package email

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"

	"uuid"
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
	if email == nil {
		return nil, fmt.Errorf("email is required")
	}
	sender := strings.TrimSpace(email.From.Address)
	parsedSender, senderErr := mail.ParseAddress(sender)
	if senderErr != nil || parsedSender.Address == "" {
		return nil, fmt.Errorf("email has no valid sender address")
	}
	if config == nil {
		return nil, fmt.Errorf("channel config is required")
	}
	email.From.Address = parsedSender.Address
	dedupKey := dedupKeyFor(email, channelID, uidValidity)

	// 1. Preclaim tracking row. INSERT ... ON CONFLICT DO NOTHING reports 0
	// rows affected when this dedup_key is already taken — that's our dedup
	// signal, replacing the older "isAlreadyProcessed" SELECT pre-check.
	claim, err := p.preclaimTracking(ctx, email, channelID, dedupKey)
	if err != nil {
		return nil, fmt.Errorf("failed to claim tracking row: %w", err)
	}
	if claim == trackingClaimCompleted {
		slog.Debug("email already processed", "message_id", email.MessageID, "dedup_key", dedupKey)
		return &ProcessingResult{Action: ActionAlreadyExists}, nil
	}
	if claim == trackingClaimInProgress {
		slog.Debug("email tracking claim is still in progress", "message_id", email.MessageID, "dedup_key", dedupKey)
		return &ProcessingResult{Action: ActionDeferred}, nil
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
		result, err = p.addCommentFromReply(email, *parentItemID, customerID)
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

// dedupKeyFor uses stable bare Message-IDs or channel-scoped UIDVALIDITY/UID
// keys. UID resets can reprocess Message-ID-less mail, which cannot otherwise
// be distinguished across UID spaces.
func dedupKeyFor(email *ParsedEmail, channelID int, uidValidity uint32) string {
	if email.MessageID != "" {
		// Preserve legacy bare keys while canonicalizing Message-ID elsewhere.
		return bareMessageID(email.MessageID)
	}
	return fmt.Sprintf("synth:%d:%d:%d", channelID, uidValidity, email.UID)
}

// findOrCreatePortalCustomer resolves the sender to a portal customer by
// email, creating one on first contact, and grants channel access.
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

	customerID, created, err := repository.NewPortalCustomerRepository(p.db).FindOrCreateByEmail(ctx, name, email)
	if err != nil {
		return 0, err
	}
	if created {
		slog.Info("created portal customer from email", "customer_id", customerID, "email", email)
	}

	if err := p.grantChannelAccess(ctx, customerID, channelID, email, config); err != nil {
		return 0, err
	}
	return customerID, nil
}

// grantChannelAccess grants the portal customer access to the email channel
// and, when configured, the connected portal channel. Granting access to the
// connected portal is gated by that portal's own PortalAllowedDomains and
// PortalRegistrationMode so email ingestion can't bypass portal-side policy
// (e.g. a "manual registration only" portal must not auto-admit arbitrary
// senders just because they emailed the ingest channel).
func (p *Processor) grantChannelAccess(ctx context.Context, customerID, channelID int, senderEmail string, config *models.ChannelConfig) error {
	repo := repository.NewPortalCustomerRepository(p.db)
	// Grant access to email channel
	if _, err := repo.EnsureChannelAccess(customerID, channelID); err != nil {
		return fmt.Errorf("grant customer access to email channel: %w", err)
	}

	if config.EmailConnectedPortalID == nil {
		return nil
	}
	portalID := *config.EmailConnectedPortalID
	if !p.connectedPortalAdmitsEmail(ctx, portalID, senderEmail) {
		slog.Info("portal policy rejects sender; skipping connected-portal access",
			"customer_id", customerID,
			"portal_channel_id", portalID,
			"sender", senderEmail,
		)
		return nil
	}
	if _, err := repo.EnsureChannelAccess(customerID, portalID); err != nil {
		return fmt.Errorf("grant customer access to connected portal: %w", err)
	}
	return nil
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

	// Unknown registration modes fail closed; empty is the legacy spelling of
	// open. A typo must never auto-register an email sender.
	if pCfg.PortalRegistrationMode != "" && pCfg.PortalRegistrationMode != "open" && pCfg.PortalRegistrationMode != "manual" {
		return false
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
		canonicalID := canonicalMessageID(messageID)
		bareID := bareMessageID(messageID)
		if canonicalID == "" {
			continue
		}
		err := p.db.QueryRowContext(ctx, `
			SELECT item_id FROM email_message_tracking
			WHERE channel_id = ? AND message_id IN (?, ?) AND item_id IS NOT NULL
		`, channelID, canonicalID, bareID).Scan(&itemID)
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
		WHERE item_id = ? AND channel_id = ? AND LOWER(from_email) = ?
	`, itemID, channelID, senderEmail).Scan(&priorCount); err == nil && priorCount > 0 {
		return true
	}
	// Original creator via portal customer.
	if creatorEmail, err := repository.NewItemRepository(p.db).GetPortalCreatorEmail(itemID, channelID); err == nil {
		if normalizedEmail(creatorEmail) == senderEmail {
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
	if config.EmailDefaultPriorityID != nil {
		allowed, err := services.IsPriorityAllowedInWorkspace(p.db, config.EmailWorkspaceID, *config.EmailDefaultPriorityID)
		if err != nil {
			return nil, fmt.Errorf("failed to check priority restriction: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("priority %d is not allowed in workspace %d", *config.EmailDefaultPriorityID, config.EmailWorkspaceID)
		}
	}

	// Build item parameters. Leave StatusID nil so services.CreateItem resolves
	// it from the workspace workflow. PriorityID is supplied only when the
	// channel explicitly configured a workspace-valid override; otherwise the
	// shared service resolves the workspace default.
	params := services.ItemCreationParams{
		WorkspaceID:             config.EmailWorkspaceID,
		Title:                   sanitize.PlainTextField.Sanitize(email.GetSubjectForItem()),
		Description:             sanitize.Comment.Sanitize(StripSignature(email.GetBodyText())),
		ItemTypeID:              config.EmailItemTypeID,
		PriorityID:              config.EmailDefaultPriorityID,
		CreatorPortalCustomerID: &customerID,
		ChannelID:               &channelID,
		EventMetadata:           itemevents.PortalCustomer(customerID, "email"),
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

	// All comment writes go through CommentService (notifications, mentions,
	// webhooks, email reply handling, and the item-change publish). It is always
	// wired in production. Inbound From is not an authentication credential, so
	// even a portal customer linked to a user remains a portal identity here.
	if p.commentService == nil {
		return nil, fmt.Errorf("comment service not configured")
	}
	result, err := p.commentService.Create(services.CreateCommentParams{
		ItemID:           itemID,
		AuthorID:         0,
		PortalCustomerID: &customerID,
		Content:          content,
		EventMetadata:    itemevents.PortalCustomer(customerID, "email"),
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
		relPath := filepath.Join("items", fmt.Sprintf("%d", itemID), uniqueFilename)
		filePath := filepath.Join(p.attachmentPath, relPath)
		if err := writeFileAtomic(filePath, att.Data, 0o600); err != nil {
			slog.Error("failed to write attachment", "error", err, "filename", att.Filename, "path", filePath)
			out.failed++
			continue
		}

		// Create attachment record. Store the path relative to attachmentPath;
		// download handlers also tolerate older absolute rows.
		now := time.Now()
		_, err := p.db.ExecWriteContext(ctx, `
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
	if _, err := p.db.ExecWriteContext(ctx, `
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

const trackingClaimStaleAfter = 5 * time.Minute

type trackingClaimState uint8

const (
	trackingClaimAcquired trackingClaimState = iota
	trackingClaimCompleted
	trackingClaimInProgress
)

// preclaimTracking inserts the tracking row up front (NULL item_id/comment_id)
// so duplicate detection happens before item creation, not after. A process
// crash can leave that preclaim behind forever, so an incomplete claim older
// than the processing lease + request budget is atomically reclaimed. Returns
// The result distinguishes ownership from a completed duplicate and a live
// unfinished claim so callers do not advance the mailbox watermark too early.
func (p *Processor) preclaimTracking(
	ctx context.Context,
	email *ParsedEmail,
	channelID int,
	dedupKey string,
) (trackingClaimState, error) {
	res, err := p.db.ExecWriteContext(ctx, `
		INSERT INTO email_message_tracking (
			channel_id, message_id, dedup_key, in_reply_to, from_email, from_name, subject,
			item_id, comment_id, direction, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, 'inbound', CURRENT_TIMESTAMP)
		ON CONFLICT(channel_id, dedup_key) DO UPDATE SET
			message_id = excluded.message_id,
			in_reply_to = excluded.in_reply_to,
			from_email = excluded.from_email,
			from_name = excluded.from_name,
			subject = excluded.subject,
			processed_at = CURRENT_TIMESTAMP
		WHERE email_message_tracking.item_id IS NULL
		  AND email_message_tracking.comment_id IS NULL
		  AND email_message_tracking.processed_at < ?
	`,
		channelID,
		email.MessageID,
		dedupKey,
		nullString(email.InReplyTo),
		email.From.Address,
		nullString(email.From.Name),
		nullString(email.Subject),
		time.Now().Add(-trackingClaimStaleAfter),
	)
	if err != nil {
		return trackingClaimInProgress, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return trackingClaimInProgress, err
	}
	if rows > 0 {
		return trackingClaimAcquired, nil
	}

	var itemID, commentID sql.NullInt64
	if err := p.db.QueryRowContext(ctx, `
		SELECT item_id, comment_id
		FROM email_message_tracking
		WHERE channel_id = ? AND dedup_key = ?
	`, channelID, dedupKey).Scan(&itemID, &commentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return trackingClaimInProgress, nil
		}
		return trackingClaimInProgress, err
	}
	if itemID.Valid || commentID.Valid {
		return trackingClaimCompleted, nil
	}
	return trackingClaimInProgress, nil
}

// releaseTrackingClaim removes a preclaim row whose downstream item/comment
// creation failed, so a retry isn't permanently blocked by an orphan. Best
// effort — a failure here just leaves the row in place, which makes the email
// look already-processed on retry (operator can re-trigger).
func (p *Processor) releaseTrackingClaim(ctx context.Context, channelID int, dedupKey string) {
	if _, err := p.db.ExecWriteContext(ctx, `
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
	_, err := p.db.ExecWriteContext(ctx, `
		UPDATE email_message_tracking
		SET item_id = ?, comment_id = ?
		WHERE channel_id = ? AND dedup_key = ? AND item_id IS NULL AND comment_id IS NULL
	`, itemID, commentID, channelID, dedupKey)
	return err
}

// nullString returns nil for empty strings
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
