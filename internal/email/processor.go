package email

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"

	"github.com/google/uuid"
)

// Processor handles email-to-item/comment conversion
type Processor struct {
	db             database.Database
	attachmentPath string
	commentService *services.CommentService
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

// ProcessEmail processes a single email, creating an item or comment
func (p *Processor) ProcessEmail(
	ctx context.Context,
	email *ParsedEmail,
	channelID int,
	config *models.ChannelConfig,
) (*ProcessingResult, error) {
	// 1. Check if already processed (dedup by Message-ID)
	if email.MessageID != "" {
		exists, err := p.isAlreadyProcessed(ctx, channelID, email.MessageID)
		if err != nil {
			return nil, fmt.Errorf("failed to check if email already processed: %w", err)
		}
		if exists {
			slog.Debug("email already processed", "message_id", email.MessageID)
			return &ProcessingResult{Action: ActionAlreadyExists}, nil
		}
	}

	// 2. Find or create portal customer by email
	customerID, err := p.findOrCreatePortalCustomer(ctx, email.From.Address, email.From.Name, channelID, config)
	if err != nil {
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
		return nil, err
	}

	result.CustomerID = &customerID

	// 5. Handle attachments if item was created
	if result.ItemID != nil && len(email.Attachments) > 0 {
		err = p.handleAttachments(ctx, email.Attachments, *result.ItemID)
		if err != nil {
			slog.Error("failed to handle attachments", "error", err, "item_id", result.ItemID)
			// Continue - attachments are not critical
		}
	}

	// 6. Track processed email
	err = p.recordProcessedEmail(ctx, email, channelID, result.ItemID, result.CommentID)
	if err != nil {
		slog.Error("failed to record processed email", "error", err)
		// Continue - tracking is not critical
	}

	return result, nil
}

// isAlreadyProcessed checks if an email with this Message-ID was already processed
func (p *Processor) isAlreadyProcessed(ctx context.Context, channelID int, messageID string) (bool, error) {
	var count int
	err := p.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM email_message_tracking
		WHERE channel_id = ? AND message_id = ?
	`, channelID, messageID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
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
		p.grantChannelAccess(ctx, customerID, channelID, config)
		return customerID, nil
	}

	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to query portal customer: %w", err)
	}

	// Create new customer
	var id int64
	err = p.db.QueryRow(`
		INSERT INTO portal_customers (name, email, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, name, email).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create portal customer: %w", err)
	}

	customerID = int(id)
	p.grantChannelAccess(ctx, customerID, channelID, config)

	slog.Info("created portal customer from email", "customer_id", customerID, "email", email)

	return customerID, nil
}

// grantChannelAccess grants the portal customer access to the channel and connected portal
func (p *Processor) grantChannelAccess(ctx context.Context, customerID, channelID int, config *models.ChannelConfig) {
	// Grant access to email channel
	_, _ = p.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO portal_customer_channels (portal_customer_id, channel_id)
		VALUES (?, ?)
	`, customerID, channelID)

	// Grant access to connected portal if configured
	if config.EmailConnectedPortalID != nil {
		_, _ = p.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO portal_customer_channels (portal_customer_id, channel_id)
			VALUES (?, ?)
		`, customerID, *config.EmailConnectedPortalID)
	}
}

// findParentItem looks up the original item from In-Reply-To or References headers
func (p *Processor) findParentItem(ctx context.Context, channelID int, email *ParsedEmail) *int {
	threadIDs := email.GetThreadIDs()

	for _, messageID := range threadIDs {
		var itemID int
		err := p.db.QueryRowContext(ctx, `
			SELECT item_id FROM email_message_tracking
			WHERE channel_id = ? AND message_id = ? AND item_id IS NOT NULL
		`, channelID, messageID).Scan(&itemID)

		if err == nil {
			slog.Debug("found parent item for reply", "message_id", messageID, "item_id", itemID)
			return &itemID
		}
	}

	return nil
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

	// Verify the item type exists and belongs to workspace's configuration set
	valid, err := p.validateItemType(*config.EmailItemTypeID, config.EmailWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate item type: %w", err)
	}
	if !valid {
		return nil, fmt.Errorf("item type %d is not valid for workspace %d", *config.EmailItemTypeID, config.EmailWorkspaceID)
	}

	// Get initial status for the workspace
	initialStatus := p.getInitialStatus(config.EmailWorkspaceID)

	// Prepare item parameters
	params := services.ItemCreationParams{
		WorkspaceID:             config.EmailWorkspaceID,
		Title:                   email.GetSubjectForItem(),
		Description:             StripSignature(email.GetBodyText()),
		Status:                  initialStatus,
		ItemTypeID:              config.EmailItemTypeID,
		Priority:                "medium",
		CreatorPortalCustomerID: &customerID,
		ChannelID:               &channelID,
	}

	// Apply default priority if configured
	if config.EmailDefaultPriorityID != nil {
		params.Priority = p.getPriorityByID(*config.EmailDefaultPriorityID)
	}

	// Create the item
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
	if err != nil && err != sql.ErrNoRows {
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

// handleAttachments saves email attachments to the item
func (p *Processor) handleAttachments(ctx context.Context, attachments []Attachment, itemID int) error {
	if p.attachmentPath == "" {
		return nil // Attachments not enabled — silently skip
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
		return nil
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
			return fmt.Errorf("failed to create attachment directory: %w", err)
		}

		// Save file
		filePath := filepath.Join(dir, uniqueFilename)
		if err := os.WriteFile(filePath, att.Data, 0o600); err != nil {
			return fmt.Errorf("failed to write attachment: %w", err)
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
			slog.Error("failed to create attachment record", "error", err, "filename", att.Filename)
			continue
		}

		slog.Debug("saved attachment", "filename", att.Filename, "item_id", itemID)
	}

	return nil
}

// recordProcessedEmail stores a record of the processed email
func (p *Processor) recordProcessedEmail(
	ctx context.Context,
	email *ParsedEmail,
	channelID int,
	itemID, commentID *int,
) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO email_message_tracking (
			channel_id, message_id, in_reply_to, from_email, from_name, subject,
			item_id, comment_id, direction, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'inbound', CURRENT_TIMESTAMP)
	`,
		channelID,
		email.MessageID,
		nullString(email.InReplyTo),
		email.From.Address,
		nullString(email.From.Name),
		nullString(email.Subject),
		itemID,
		commentID,
	)
	return err
}

// getInitialStatus gets the initial status for a workspace
func (p *Processor) getInitialStatus(_ int) string {
	var status string
	err := p.db.QueryRow(`
		SELECT s.name FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE sc.name = 'To Do'
		ORDER BY s.id ASC
		LIMIT 1
	`).Scan(&status)

	if err != nil {
		return "Open" // Default fallback
	}
	return status
}

// getPriorityByID gets priority name by ID
func (p *Processor) getPriorityByID(priorityID int) string {
	var name string
	err := p.db.QueryRow(`SELECT name FROM priorities WHERE id = ?`, priorityID).Scan(&name)
	if err != nil {
		return "medium"
	}
	return strings.ToLower(name)
}

// validateItemType checks if the item type exists
func (p *Processor) validateItemType(itemTypeID, _ int) (bool, error) {
	var count int
	// Just check that the item type exists - consistent with regular item creation
	err := p.db.QueryRow(`SELECT COUNT(*) FROM item_types WHERE id = ?`, itemTypeID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// nullString returns nil for empty strings
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
