package email

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"

	"github.com/google/uuid"
)

// Processor handles email-to-item/comment conversion
type Processor struct {
	db               database.Database
	attachmentPath   string
	commentService   *services.CommentService
	eventCoordinator *services.EventCoordinator
}

// NewProcessor creates a new email processor
func NewProcessor(db database.Database, attachmentPath string) *Processor {
	return &Processor{
		db:             db,
		attachmentPath: attachmentPath,
	}
}

// SetCommentService sets the comment service for unified comment creation.
func (p *Processor) SetCommentService(cs *services.CommentService) {
	p.commentService = cs
}

// SetEventCoordinator wires the centralized event emitter so email-created
// items get the same side effects (notifications, webhooks, action triggers,
// activity tracking) as API-created items. Without this, email ingestion is
// silently missing those events.
func (p *Processor) SetEventCoordinator(ec *services.EventCoordinator) {
	p.eventCoordinator = ec
}

// ProcessEmail processes a single email, creating an item or comment.
// uidValidity is the IMAP UIDVALIDITY observed by the scheduler when this
// message was fetched; it lets us synthesize a stable dedup key for
// Message-ID-less mail without collapsing every such email onto the same
// (channel_id, ”) tracking row.
//
// Atomicity: we preclaim the tracking row (with NULL item_id/comment_id) up
// front, then create the item/comment, then finalize the tracking row with the
// resulting IDs. If item/comment creation fails we release the claim so a
// retry isn't blocked. This avoids the original race where a tracking insert
// failing after item creation could let the same email re-create the item on
// the next UID/mailbox reset.
func (p *Processor) ProcessEmail(
	ctx context.Context,
	email *ParsedEmail,
	channelID int,
	uidValidity uint32,
	config *models.ChannelConfig,
) (*ProcessingResult, error) {
	dedupKey := dedupKeyFor(email, channelID, uidValidity)

	// 1. Preclaim tracking row. INSERT ... ON CONFLICT DO NOTHING reports 0
	// rows affected when this dedup_key is already taken — that's our dedup
	// signal, replacing the older "isAlreadyProcessed" SELECT pre-check.
	claimed, err := p.preclaimTracking(ctx, email, channelID, dedupKey)
	if err != nil {
		return nil, fmt.Errorf("failed to claim tracking row: %w", err)
	}
	if !claimed {
		slog.Debug("email already processed", "message_id", email.MessageID, "dedup_key", dedupKey)
		return &ProcessingResult{Action: ActionAlreadyExists}, nil
	}

	// 2. Find or create portal customer by email
	customerID, err := p.findOrCreatePortalCustomer(ctx, email.From.Address, email.From.Name, channelID, config)
	if err != nil {
		p.releaseTrackingClaim(ctx, channelID, dedupKey)
		return nil, fmt.Errorf("failed to find/create portal customer: %w", err)
	}

	// 3. Check if this is a reply (find parent item by In-Reply-To/References)
	var parentItemID *int
	if email.IsReply() {
		parentItemID = p.findParentItem(ctx, channelID, email)
	}

	// 4. Create item or add comment
	var result *ProcessingResult
	if parentItemID != nil {
		// This is a reply - add comment to existing item
		result, err = p.addCommentFromReply(ctx, email, *parentItemID, customerID)
	} else {
		// This is a new conversation - create item
		result, err = p.createItemFromEmail(ctx, email, channelID, config, customerID)
	}

	if err != nil {
		p.releaseTrackingClaim(ctx, channelID, dedupKey)
		return nil, err
	}

	result.CustomerID = &customerID

	// 5. Finalize tracking with the created item/comment ID. If this UPDATE
	// fails the item still exists and the preclaim row continues to block
	// duplicates, so we surface the error in logs but don't fail the call.
	if err := p.finalizeTrackingClaim(ctx, channelID, dedupKey, result.ItemID, result.CommentID); err != nil {
		slog.Error("failed to finalize tracking row",
			"error", err,
			"channel_id", channelID,
			"dedup_key", dedupKey,
			"item_id", result.ItemID,
			"comment_id", result.CommentID,
		)
	}

	// 6. Handle attachments if item was created. Surface partial-ingestion
	// status on the tracking row so operators can see when attachments were
	// dropped — the item itself stays in place either way.
	if result.ItemID != nil && len(email.Attachments) > 0 {
		attRes, err := p.handleAttachments(ctx, email.Attachments, *result.ItemID)
		if err != nil {
			slog.Error("failed to handle attachments", "error", err, "item_id", result.ItemID)
			p.setAttachmentsStatus(ctx, channelID, dedupKey, "failed")
		} else {
			switch {
			case attRes.failed > 0 && attRes.saved > 0:
				p.setAttachmentsStatus(ctx, channelID, dedupKey, "partial")
			case attRes.failed > 0 && attRes.saved == 0:
				p.setAttachmentsStatus(ctx, channelID, dedupKey, "failed")
			case attRes.saved > 0:
				p.setAttachmentsStatus(ctx, channelID, dedupKey, "ok")
			}
		}
	}

	return result, nil
}

// dedupKeyFor returns the stable per-channel dedup key for an email. When
// MessageID is non-empty we use it directly so a server re-delivery (same
// Message-ID, new UID) still matches. When it's empty we synthesize a
// channel-scoped key from UIDVALIDITY+UID — IMAP guarantees uniqueness of
// (uidvalidity, uid) within a mailbox, so this distinguishes Message-ID-less
// emails from each other and survives normal polling. A UIDVALIDITY reset or
// mailbox restore will change the synth key and cause the message to be
// reprocessed; that is acceptable because we can't tell synthetically-keyed
// emails apart across UID-space resets.
func dedupKeyFor(email *ParsedEmail, channelID int, uidValidity uint32) string {
	if email.MessageID != "" {
		return email.MessageID
	}
	return fmt.Sprintf("synth:%d:%d:%d", channelID, uidValidity, email.UID)
}

// findOrCreatePortalCustomer finds an existing portal customer or creates a new one
func (p *Processor) findOrCreatePortalCustomer(
	ctx context.Context,
	email, name string,
	channelID int,
	config *models.ChannelConfig,
) (int, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)

	if name == "" {
		// Extract name from email if not provided
		parts := strings.Split(email, "@")
		if len(parts) > 0 {
			name = parts[0]
		}
	}

	// Try to find existing customer by email
	var customerID int
	err := p.db.QueryRow(`
		SELECT id FROM portal_customers WHERE LOWER(email) = ?
	`, email).Scan(&customerID)

	if err == nil {
		// Customer exists
		p.grantChannelAccess(ctx, customerID, channelID, email, config)
		return customerID, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("failed to query portal customer: %w", err)
	}

	// Create new customer. Use ON CONFLICT DO NOTHING RETURNING id so a
	// concurrent inserter (another process in a multi-instance deployment, or
	// an admin creating a customer with the same address) doesn't make us fail
	// with a unique-constraint error — instead we fall through and re-select
	// the row the winner created.
	var id int64
	err = p.db.QueryRow(`
		INSERT INTO portal_customers (name, email, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, name, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// Lost the race; another inserter committed first. Re-read their row.
		if err = p.db.QueryRow(`SELECT id FROM portal_customers WHERE LOWER(email) = ?`, email).Scan(&customerID); err != nil {
			return 0, fmt.Errorf("failed to re-select portal customer after conflict: %w", err)
		}
		p.grantChannelAccess(ctx, customerID, channelID, email, config)
		return customerID, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to create portal customer: %w", err)
	}

	customerID = int(id)
	p.grantChannelAccess(ctx, customerID, channelID, email, config)

	slog.Info("created portal customer from email", "customer_id", customerID, "email", email)

	return customerID, nil
}

// grantChannelAccess grants the portal customer access to the email channel
// and, when configured, the connected portal channel. Granting access to the
// connected portal is gated by that portal's own PortalAllowedDomains and
// PortalRegistrationMode so email ingestion can't bypass portal-side policy
// (e.g. a "manual registration only" portal must not auto-admit arbitrary
// senders just because they emailed the ingest channel).
func (p *Processor) grantChannelAccess(ctx context.Context, customerID, channelID int, senderEmail string, config *models.ChannelConfig) {
	// Grant access to email channel
	_, _ = p.db.ExecContext(ctx, `
		INSERT INTO portal_customer_channels (portal_customer_id, channel_id)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING
	`, customerID, channelID)

	if config.EmailConnectedPortalID == nil {
		return
	}
	portalID := *config.EmailConnectedPortalID
	if !p.connectedPortalAdmitsEmail(ctx, portalID, senderEmail) {
		slog.Info("portal policy rejects sender; skipping connected-portal access",
			"customer_id", customerID,
			"portal_channel_id", portalID,
			"sender", senderEmail,
		)
		return
	}
	_, _ = p.db.ExecContext(ctx, `
		INSERT INTO portal_customer_channels (portal_customer_id, channel_id)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING
	`, customerID, portalID)
}

// connectedPortalAdmitsEmail returns true when the connected portal's policy
// would admit this sender. Domain allow-list and registration-mode mirror the
// portal_auth.go login path. A missing/unreadable portal config falls back to
// rejecting the access grant: we'd rather under-grant than over-grant.
func (p *Processor) connectedPortalAdmitsEmail(ctx context.Context, portalChannelID int, senderEmail string) bool {
	var configJSON string
	if err := p.db.QueryRowContext(ctx, `SELECT COALESCE(config, '') FROM channels WHERE id = ?`, portalChannelID).Scan(&configJSON); err != nil {
		slog.Warn("failed to load connected portal config; denying access grant",
			"error", err, "portal_channel_id", portalChannelID)
		return false
	}
	if configJSON == "" {
		return true
	}
	var pCfg models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &pCfg); err != nil {
		slog.Warn("failed to parse connected portal config; denying access grant",
			"error", err, "portal_channel_id", portalChannelID)
		return false
	}

	// Domain allow-list: empty means allow all.
	if len(pCfg.PortalAllowedDomains) > 0 {
		at := strings.LastIndex(senderEmail, "@")
		if at < 0 || at == len(senderEmail)-1 {
			return false
		}
		domain := senderEmail[at+1:]
		allowed := false
		for _, d := range pCfg.PortalAllowedDomains {
			if strings.EqualFold(strings.TrimSpace(d), domain) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	// Manual registration: only existing portal_customer_channels rows admit.
	if pCfg.PortalRegistrationMode == "manual" {
		var hasAccess bool
		if err := p.db.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM portal_customer_channels pcc
				JOIN portal_customers pc ON pc.id = pcc.portal_customer_id
				WHERE LOWER(pc.email) = ? AND pcc.channel_id = ?
			)
		`, senderEmail, portalChannelID).Scan(&hasAccess); err != nil {
			slog.Warn("failed to check manual portal access; denying grant",
				"error", err, "portal_channel_id", portalChannelID)
			return false
		}
		return hasAccess
	}

	return true
}

// findParentItem looks up the original item from In-Reply-To or References headers.
//
// Thread-hijack defense: the In-Reply-To / References headers are entirely
// attacker-controlled. If we trusted them naively, anyone who leaks or guesses
// a Message-ID used on a channel could post a "reply" onto that item from a
// new email address, exposing private conversations to a third party. We match
// only when the sender is demonstrably part of the thread — either a prior
// participant on that tracked thread (their address appeared as from_email
// on an earlier tracked message for the same item) or the original creator
// of the item via the portal_customer linkage.
func (p *Processor) findParentItem(ctx context.Context, channelID int, email *ParsedEmail) *int {
	threadIDs := email.GetThreadIDs()
	senderEmail := normalizedEmail(email.From.Address)

	for _, messageID := range threadIDs {
		var itemID int
		err := p.db.QueryRowContext(ctx, `
			SELECT item_id FROM email_message_tracking
			WHERE channel_id = ? AND message_id = ? AND item_id IS NOT NULL
		`, channelID, messageID).Scan(&itemID)
		if err != nil {
			continue
		}
		if !p.senderIsThreadParticipant(ctx, itemID, channelID, senderEmail) {
			slog.Warn("ignoring reply: sender is not a known thread participant",
				"item_id", itemID,
				"message_id", messageID,
				"sender", senderEmail,
			)
			continue
		}
		slog.Debug("found parent item for reply", "message_id", messageID, "item_id", itemID)
		return &itemID
	}

	return nil
}

// senderIsThreadParticipant reports whether senderEmail is allowed to post
// onto the given item via an email reply.
func (p *Processor) senderIsThreadParticipant(ctx context.Context, itemID, channelID int, senderEmail string) bool {
	if senderEmail == "" {
		return false
	}
	// Prior participant on this thread (inbound or outbound).
	var priorCount int
	if err := p.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM email_message_tracking
		WHERE item_id = ? AND LOWER(from_email) = ?
	`, itemID, senderEmail).Scan(&priorCount); err == nil && priorCount > 0 {
		return true
	}
	// Original creator via portal customer.
	var creatorEmail sql.NullString
	if err := p.db.QueryRowContext(ctx, `
		SELECT pc.email
		FROM items i
		JOIN portal_customers pc ON pc.id = i.creator_portal_customer_id
		WHERE i.id = ? AND i.channel_id = ?
	`, itemID, channelID).Scan(&creatorEmail); err == nil && creatorEmail.Valid {
		if normalizedEmail(creatorEmail.String) == senderEmail {
			return true
		}
	}
	return false
}

// normalizedEmail lowercases and trims, matching how the processor stores and
// compares email addresses elsewhere (see findOrCreatePortalCustomer).
func normalizedEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// createItemFromEmail creates a new item from an email
func (p *Processor) createItemFromEmail( //nolint:unparam // ctx reserved for future use
	ctx context.Context,
	email *ParsedEmail,
	channelID int,
	config *models.ChannelConfig,
	customerID int,
) (*ProcessingResult, error) {
	_ = ctx
	if config.EmailWorkspaceID == 0 {
		return nil, fmt.Errorf("no workspace configured for email channel")
	}

	// Validate item type is configured
	if config.EmailItemTypeID == nil || *config.EmailItemTypeID == 0 {
		return nil, fmt.Errorf("no item type configured for email channel: EmailItemTypeID is required")
	}

	// Verify the item type is allowed in this workspace's configuration set.
	// This mirrors the REST handler (restapi/v1/handlers/items.go) so email-
	// created items go through the same validation as API-created ones.
	allowed, err := services.IsItemTypeAllowedInWorkspace(p.db, config.EmailWorkspaceID, *config.EmailItemTypeID)
	if err != nil {
		return nil, fmt.Errorf("failed to check item type restriction: %w", err)
	}
	if !allowed {
		return nil, fmt.Errorf("item type %d is not allowed in workspace %d", *config.EmailItemTypeID, config.EmailWorkspaceID)
	}

	// Build item parameters. Leave StatusID/PriorityID nil so services.CreateItem
	// resolves them from the workspace's workflow and default-priority config —
	// the same path the REST API uses. A prior implementation tried to resolve
	// these here with a global query, which failed for custom workflows.
	params := services.ItemCreationParams{
		WorkspaceID:             config.EmailWorkspaceID,
		Title:                   utils.StripHTMLTags(email.GetSubjectForItem()),
		Description:             utils.SanitizeCommentContent(StripSignature(email.GetBodyText())),
		ItemTypeID:              config.EmailItemTypeID,
		PriorityID:              config.EmailDefaultPriorityID,
		CreatorPortalCustomerID: &customerID,
		ChannelID:               &channelID,
	}

	// Create the item via the shared service entry point.
	itemID, err := services.CreateItem(p.db, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create item: %w", err)
	}

	slog.Info("created item from email",
		"item_id", itemID,
		"subject", email.Subject,
		"from", email.From.Address,
	)

	id := int(itemID)

	// Emit the same side effects (notifications, webhooks, action triggers,
	// activity tracking) the REST item-create path runs. Without this, email-
	// ingested items silently bypass watchers and automation hooks. We pass
	// actorUserID=0 because the email sender is a portal customer, not an
	// internal user — the coordinator handles that gracefully.
	if p.eventCoordinator != nil {
		fullItem, fetchErr := repository.NewItemRepository(p.db).FindByIDWithDetails(id)
		if fetchErr != nil {
			slog.Warn("failed to load full item for event emission, skipping side effects",
				"error", fetchErr, "item_id", id)
		} else if fullItem != nil {
			p.eventCoordinator.EmitItemCreated(fullItem, 0)
		}
	}

	return &ProcessingResult{
		Action: ActionItemCreated,
		ItemID: &id,
	}, nil
}

// addCommentFromReply adds a comment to an existing item from an email reply
func (p *Processor) addCommentFromReply(
	ctx context.Context,
	email *ParsedEmail,
	itemID int,
	customerID int,
) (*ProcessingResult, error) {
	// Extract reply content (strip quoted text)
	content := StripSignature(ExtractReplyContent(email.GetBodyText()))

	if strings.TrimSpace(content) == "" {
		// No new content - skip
		return &ProcessingResult{
			Action: ActionSkipped,
			ItemID: &itemID,
		}, nil
	}

	// Get user ID from portal customer (if linked)
	var linkedUserID int
	err := p.db.QueryRowContext(ctx, `
		SELECT user_id FROM portal_customers WHERE id = ? AND user_id IS NOT NULL
	`, customerID).Scan(&linkedUserID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("failed to get user_id for portal customer", "error", err)
	}

	// Use CommentService for unified comment creation (notifications, mentions, webhooks, email reply handling)
	if p.commentService != nil {
		var result *services.CreateCommentResult
		result, err = p.commentService.Create(services.CreateCommentParams{
			ItemID:           itemID,
			AuthorID:         linkedUserID,
			PortalCustomerID: &customerID,
			Content:          content,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create comment: %w", err)
		}

		commentID := int(result.CommentID)

		slog.Info("added comment from email reply",
			"comment_id", commentID,
			"item_id", itemID,
			"from", email.From.Address,
		)

		return &ProcessingResult{
			Action:    ActionCommentAdded,
			ItemID:    &itemID,
			CommentID: &commentID,
		}, nil
	}

	// Fallback: direct DB insert (should not be used in production)
	slog.Warn("commentService not set in email processor, using direct DB insert",
		"item_id", itemID)

	now := time.Now()
	var commentIDInt64 int64
	if linkedUserID != 0 {
		err = p.db.QueryRowContext(ctx, `
			INSERT INTO comments (item_id, author_id, content, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?) RETURNING id
		`, itemID, linkedUserID, content, now, now).Scan(&commentIDInt64)
	} else {
		err = p.db.QueryRowContext(ctx, `
			INSERT INTO comments (item_id, portal_customer_id, content, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?) RETURNING id
		`, itemID, customerID, content, now, now).Scan(&commentIDInt64)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	commentID := int(commentIDInt64)

	slog.Info("added comment from email reply",
		"comment_id", commentID,
		"item_id", itemID,
		"from", email.From.Address,
	)

	return &ProcessingResult{
		Action:    ActionCommentAdded,
		ItemID:    &itemID,
		CommentID: &commentID,
	}, nil
}

// attachmentResult counts the per-email attachment outcome so callers can
// surface partial-ingestion status on the tracking row. Size/MIME rejections
// are config decisions (not failures) and don't increment failed.
type attachmentResult struct {
	saved  int
	failed int
}

// handleAttachments saves email attachments to the item. Returns counts of
// successfully stored vs. write/insert-failed attachments so the caller can
// stamp an attachments_status on the tracking row. Returns a non-nil error
// only for fatal setup errors (mkdir, settings load mishaps) — per-attachment
// failures are absorbed into result.failed.
func (p *Processor) handleAttachments(ctx context.Context, attachments []Attachment, itemID int) (attachmentResult, error) {
	var out attachmentResult
	if p.attachmentPath == "" {
		return out, nil // Attachments not enabled — silently skip
	}

	// Load attachment settings
	var maxFileSize int64
	var allowedMimeJSON string
	var enabled bool
	err := p.db.QueryRowContext(ctx, `
		SELECT max_file_size, allowed_mime_types, enabled
		FROM attachment_settings ORDER BY id DESC LIMIT 1
	`).Scan(&maxFileSize, &allowedMimeJSON, &enabled)
	if err != nil {
		// No settings row = use defaults (enabled, 50MB, all types)
		maxFileSize = 52428800
		enabled = true
	}
	if !enabled {
		return out, nil
	}

	// Parse allowed MIME types
	var allowedTypes []string
	if allowedMimeJSON != "" {
		_ = json.Unmarshal([]byte(allowedMimeJSON), &allowedTypes)
	}

	for _, att := range attachments {
		// Check size limit
		if att.Size > maxFileSize {
			slog.Debug("skipping attachment: exceeds max size", "filename", att.Filename, "size", att.Size)
			continue
		}
		// Check MIME allowlist
		if len(allowedTypes) > 0 {
			allowed := false
			for _, t := range allowedTypes {
				if strings.HasPrefix(att.ContentType, t) {
					allowed = true
					break
				}
			}
			if !allowed {
				slog.Debug("skipping attachment: MIME type not allowed", "filename", att.Filename, "type", att.ContentType)
				continue
			}
		}

		// Generate unique filename
		ext := filepath.Ext(att.Filename)
		uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)

		// Create directory if needed
		dir := filepath.Join(p.attachmentPath, "items", fmt.Sprintf("%d", itemID))
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return out, fmt.Errorf("failed to create attachment directory: %w", err)
		}

		// Write to a .tmp sibling and rename into place so a partial write never
		// leaves a truncated file that the UI would later serve. If the DB insert
		// that follows fails, delete the file so we don't orphan it on disk.
		filePath := filepath.Join(dir, uniqueFilename)
		if err := writeFileAtomic(filePath, att.Data, 0o600); err != nil {
			slog.Error("failed to write attachment", "error", err, "filename", att.Filename, "path", filePath)
			out.failed++
			continue
		}

		// Relative path for DB record
		relPath := filepath.Join("items", fmt.Sprintf("%d", itemID), uniqueFilename)

		// Create attachment record
		now := time.Now()
		_, err := p.db.ExecContext(ctx, `
			INSERT INTO attachments (item_id, filename, original_filename, file_path, mime_type, file_size, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, itemID, uniqueFilename, att.Filename, relPath, att.ContentType, att.Size, now)
		if err != nil {
			slog.Error("failed to create attachment record, deleting orphaned file",
				"error", err, "filename", att.Filename, "path", filePath)
			if rmErr := os.Remove(filePath); rmErr != nil {
				slog.Warn("failed to remove orphaned attachment file", "path", filePath, "error", rmErr)
			}
			out.failed++
			continue
		}

		slog.Debug("saved attachment", "filename", att.Filename, "item_id", itemID)
		out.saved++
	}

	return out, nil
}

// setAttachmentsStatus writes the partial-ingestion marker onto the tracking
// row so an operator inspecting the email log can see when attachments were
// dropped. Best-effort: a failure here doesn't change the item state.
func (p *Processor) setAttachmentsStatus(ctx context.Context, channelID int, dedupKey, status string) {
	if status == "" {
		return
	}
	if _, err := p.db.ExecContext(ctx, `
		UPDATE email_message_tracking
		SET attachments_status = ?
		WHERE channel_id = ? AND dedup_key = ?
	`, status, channelID, dedupKey); err != nil {
		slog.Warn("failed to set attachments_status", "error", err, "channel_id", channelID, "dedup_key", dedupKey)
	}
}

// writeFileAtomic writes data to a temp file in the same directory as path and
// renames it into place. On any failure before the rename it tries to remove
// the temp file so a crash doesn't leak a partial write.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup; harmless if the rename already consumed the file.
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil { //nolint:gosec // G703: path is attachmentPath + "items" + itemID + uuid + filepath.Ext (caller-controlled root, no traversal possible)
		cleanup()
		return err
	}
	return nil
}

// preclaimTracking inserts the tracking row up front (NULL item_id/comment_id)
// so duplicate detection happens before item creation, not after. Returns true
// when this caller owns the claim and should proceed; false when the row
// already existed (i.e. another worker or a prior run already tracked it).
func (p *Processor) preclaimTracking(
	ctx context.Context,
	email *ParsedEmail,
	channelID int,
	dedupKey string,
) (bool, error) {
	res, err := p.db.ExecContext(ctx, `
		INSERT INTO email_message_tracking (
			channel_id, message_id, dedup_key, in_reply_to, from_email, from_name, subject,
			item_id, comment_id, direction, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, 'inbound', CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
	`,
		channelID,
		email.MessageID,
		dedupKey,
		nullString(email.InReplyTo),
		email.From.Address,
		nullString(email.From.Name),
		nullString(email.Subject),
	)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// releaseTrackingClaim removes a preclaim row whose downstream item/comment
// creation failed, so a retry isn't permanently blocked by an orphan. Best
// effort — a failure here just leaves the row in place, which makes the email
// look already-processed on retry (operator can re-trigger).
func (p *Processor) releaseTrackingClaim(ctx context.Context, channelID int, dedupKey string) {
	if _, err := p.db.ExecContext(ctx, `
		DELETE FROM email_message_tracking
		WHERE channel_id = ? AND dedup_key = ? AND item_id IS NULL AND comment_id IS NULL
	`, channelID, dedupKey); err != nil {
		slog.Warn("failed to release tracking claim", "error", err, "channel_id", channelID, "dedup_key", dedupKey)
	}
}

// finalizeTrackingClaim sets the item_id/comment_id on a preclaim row once the
// downstream create has succeeded. The WHERE constrains by NULL refs to avoid
// stomping a row another worker may have completed first.
func (p *Processor) finalizeTrackingClaim(
	ctx context.Context,
	channelID int,
	dedupKey string,
	itemID, commentID *int,
) error {
	_, err := p.db.ExecContext(ctx, `
		UPDATE email_message_tracking
		SET item_id = ?, comment_id = ?
		WHERE channel_id = ? AND dedup_key = ? AND item_id IS NULL AND comment_id IS NULL
	`, itemID, commentID, channelID, dedupKey)
	return err
}

// nullString returns nil for empty strings
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
