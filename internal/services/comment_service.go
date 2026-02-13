package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

// WebhookDispatcher is an interface for dispatching webhook events.
// This avoids an import cycle with the webhook package.
type WebhookDispatcher interface {
	DispatchEvent(eventType string, item *models.Item)
}

// EmailReplyHandler is an interface for handling outbound email replies on comment creation.
// This avoids an import cycle with the email reply service.
type EmailReplyHandler interface {
	HandleCommentCreated(params HandleCommentParams) error
}

// HandleCommentParams contains the parameters for handling a comment creation event.
type HandleCommentParams struct {
	CommentID        int
	ItemID           int
	AuthorID         int
	PortalCustomerID *int
	Content          string
	IsPrivate        bool
}

// CommentService encapsulates comment creation logic used by both HTTP handlers
// and action automation service.
type CommentService struct {
	db                  database.Database
	activityTracker     *ActivityTracker
	notificationService *NotificationService
	mentionService      *MentionService
	webhookSender       WebhookDispatcher
	emailReplyService   EmailReplyHandler
}

// CreateCommentParams contains the parameters for creating a comment.
type CreateCommentParams struct {
	ItemID           int
	AuthorID         int        // Internal user (0 if portal customer without linked user)
	PortalCustomerID *int       // Portal customer (nil if internal user)
	Content          string     // Raw content (will be sanitized)
	IsPrivate        bool       // For action automation private notes
	ActorUserID      int        // User performing the action (for notifications, 0 for portal customers)
	CreatedAt        *time.Time // Optional: override created_at (e.g. for imports preserving original timestamps)
}

// CreateCommentResult contains the result of creating a comment.
type CreateCommentResult struct {
	Comment   *models.Comment
	CommentID int64
}

// NewCommentService creates a new CommentService.
func NewCommentService(db database.Database) *CommentService {
	return &CommentService{
		db: db,
	}
}

// SetActivityTracker sets the activity tracker for tracking comment activity.
func (s *CommentService) SetActivityTracker(tracker *ActivityTracker) {
	s.activityTracker = tracker
}

// SetNotificationService sets the notification service for emitting comment events.
func (s *CommentService) SetNotificationService(ns *NotificationService) {
	s.notificationService = ns
}

// SetMentionService sets the mention service for processing @mentions.
func (s *CommentService) SetMentionService(ms *MentionService) {
	s.mentionService = ms
}

// SetWebhookSender sets the webhook sender for dispatching webhook events.
func (s *CommentService) SetWebhookSender(ws WebhookDispatcher) {
	s.webhookSender = ws
}

// SetEmailReplyService sets the email reply service for sending threaded replies to portal customers.
func (s *CommentService) SetEmailReplyService(ers EmailReplyHandler) {
	s.emailReplyService = ers
}

// Create creates a new comment with all associated side effects:
// activity tracking, notifications, mentions, and webhooks.
func (s *CommentService) Create(params CreateCommentParams) (*CreateCommentResult, error) {
	// 1. Sanitize content (XSS prevention — strips HTML tags + dangerous Markdown URLs)
	sanitizedContent := utils.SanitizeCommentContent(params.Content)

	// 2. Get item details for notifications
	var workspaceID int
	var itemTitle string
	var workspaceItemNumber int
	var workspaceKey string
	var assigneeID, creatorID sql.NullInt64
	err := s.db.QueryRow(`
		SELECT i.workspace_id, i.title, i.workspace_item_number, w.key, i.assignee_id, i.creator_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, params.ItemID).Scan(&workspaceID, &itemTitle, &workspaceItemNumber, &workspaceKey, &assigneeID, &creatorID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("item not found: %d", params.ItemID)
		}
		return nil, fmt.Errorf("failed to fetch item details: %w", err)
	}

	// 3. Insert into DB
	now := time.Now()
	if params.CreatedAt != nil {
		now = *params.CreatedAt
	}

	var commentID int64
	if params.PortalCustomerID != nil && params.AuthorID == 0 {
		// Portal customer without linked user — insert with portal_customer_id
		err = s.db.QueryRow(`
			INSERT INTO comments (item_id, portal_customer_id, content, is_private, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?) RETURNING id
		`, params.ItemID, *params.PortalCustomerID, sanitizedContent, params.IsPrivate, now, now).Scan(&commentID)
	} else {
		// Internal user or portal customer with linked user
		var authorID interface{}
		if params.AuthorID != 0 {
			authorID = params.AuthorID
		}
		err = s.db.QueryRow(`
			INSERT INTO comments (item_id, author_id, content, is_private, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?) RETURNING id
		`, params.ItemID, authorID, sanitizedContent, params.IsPrivate, now, now).Scan(&commentID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	// 4. Track activity (if activityTracker != nil)
	if s.activityTracker != nil {
		if err := s.activityTracker.TrackItemActivity(params.ActorUserID, params.ItemID, ActivityComment); err != nil {
			// Don't fail the request if activity tracking fails, just log it
			slog.Warn("failed to track comment activity",
				slog.String("component", "comment_service"),
				slog.Int("item_id", params.ItemID),
				slog.Any("error", err),
			)
		}
	}

	// 5. Emit notification event (if notificationService != nil)
	if s.notificationService != nil {
		assigneeIDPtr := utils.NullInt64ToPtr(assigneeID)
		creatorIDPtr := utils.NullInt64ToPtr(creatorID)

		// Get actor name for notification
		var actorName string
		if params.PortalCustomerID != nil {
			// Portal customer — look up customer name
			_ = s.db.QueryRow("SELECT name FROM portal_customers WHERE id = ?", *params.PortalCustomerID).Scan(&actorName)
			if actorName == "" {
				actorName = "Portal Customer"
			}
		} else {
			_ = s.db.QueryRow("SELECT username FROM users WHERE id = ?", params.ActorUserID).Scan(&actorName)
			if actorName == "" {
				actorName = fmt.Sprintf("User #%d", params.ActorUserID)
			}
		}

		// Construct the item key (e.g., "TST-1")
		itemKey := fmt.Sprintf("%s-%d", workspaceKey, workspaceItemNumber)

		slog.Debug("emitting notification event for comment",
			slog.String("component", "comment_service"),
			slog.Int("item_id", params.ItemID),
			slog.Int("actor_user_id", params.ActorUserID),
		)

		s.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventCommentCreated,
			WorkspaceID: workspaceID,
			ActorUserID: params.ActorUserID,
			ItemID:      params.ItemID,
			AssigneeID:  assigneeIDPtr,
			CreatorID:   creatorIDPtr,
			Title:       "New Comment Added",
			TemplateData: map[string]interface{}{
				"item.title": itemTitle,
				"item.key":   itemKey,
				"item.id":    params.ItemID,
				"user.name":  actorName,
			},
		})
	}

	// 6. Process @mentions (if mentionService != nil)
	if s.mentionService != nil {
		if err := s.mentionService.ProcessMentions(ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    int(commentID),
			Content:     params.Content, // Use original content for mention parsing
			ItemID:      params.ItemID,
			WorkspaceID: workspaceID,
			ActorUserID: params.ActorUserID,
		}); err != nil {
			slog.Warn("failed to process mentions",
				slog.String("component", "comment_service"),
				slog.Int64("comment_id", commentID),
				slog.Any("error", err),
			)
			// Don't fail the request if mention processing fails
		}
	}

	// 7. Dispatch webhook (if webhookSender != nil)
	if s.webhookSender != nil {
		// Get full item for webhook payload
		itemRepo := repository.NewItemRepository(s.db)
		if item, err := itemRepo.FindByIDWithDetails(params.ItemID); err == nil {
			go s.webhookSender.DispatchEvent("comment.created", item)
		}
	}

	// 8. Handle outbound email reply (if emailReplyService != nil)
	if s.emailReplyService != nil {
		if err := s.emailReplyService.HandleCommentCreated(HandleCommentParams{
			CommentID:        int(commentID),
			ItemID:           params.ItemID,
			AuthorID:         params.AuthorID,
			PortalCustomerID: params.PortalCustomerID,
			Content:          sanitizedContent,
			IsPrivate:        params.IsPrivate,
		}); err != nil {
			slog.Warn("failed to handle email reply for comment",
				slog.String("component", "comment_service"),
				slog.Int64("comment_id", commentID),
				slog.Any("error", err),
			)
		}
	}

	// 9. Return created comment
	return &CreateCommentResult{
		CommentID: commentID,
	}, nil
}

// CommentWithDetails contains a comment with its related details
type CommentWithDetails struct {
	models.Comment
	WorkspaceID int
	ItemTitle   string
}

// Get retrieves a comment by ID with author details
func (s *CommentService) Get(commentID int) (*CommentWithDetails, error) {
	var comment CommentWithDetails
	var authorID sql.NullInt64
	var authorFirstName, authorLastName, authorEmail sql.NullString

	err := s.db.QueryRow(`
		SELECT c.id, c.item_id, c.author_id, c.content, c.is_private, c.created_at, c.updated_at,
		       u.first_name, u.last_name, u.email,
		       i.workspace_id, i.title
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		JOIN items i ON c.item_id = i.id
		WHERE c.id = ?
	`, commentID).Scan(
		&comment.ID, &comment.ItemID, &authorID, &comment.Content, &comment.IsPrivate,
		&comment.CreatedAt, &comment.UpdatedAt,
		&authorFirstName, &authorLastName, &authorEmail,
		&comment.WorkspaceID, &comment.ItemTitle,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("comment not found: %d", commentID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comment: %w", err)
	}

	if authorID.Valid {
		id := int(authorID.Int64)
		comment.AuthorID = &id
	}
	if authorFirstName.Valid && authorLastName.Valid {
		comment.AuthorName = fmt.Sprintf("%s %s", authorFirstName.String, authorLastName.String)
	}
	if authorEmail.Valid {
		comment.AuthorEmail = authorEmail.String
	}

	return &comment, nil
}

// Update updates a comment's content
func (s *CommentService) Update(commentID int, content string, userID int) (*models.Comment, error) {
	// Sanitize content (strips HTML tags + dangerous Markdown URLs)
	sanitizedContent := utils.SanitizeCommentContent(content)

	// Check if comment exists and get author
	var authorID int
	err := s.db.QueryRow("SELECT author_id FROM comments WHERE id = ?", commentID).Scan(&authorID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("comment not found: %d", commentID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check comment: %w", err)
	}

	// Update the comment
	now := time.Now()
	_, err = s.db.Exec(`
		UPDATE comments SET content = ?, updated_at = ? WHERE id = ?
	`, sanitizedContent, now, commentID)
	if err != nil {
		return nil, fmt.Errorf("failed to update comment: %w", err)
	}

	// Fetch and return the updated comment
	var comment models.Comment
	var authorFirstName, authorLastName, authorEmail sql.NullString
	var authorIDNull sql.NullInt64

	err = s.db.QueryRow(`
		SELECT c.id, c.item_id, c.author_id, c.content, c.is_private, c.created_at, c.updated_at,
		       u.first_name, u.last_name, u.email
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		WHERE c.id = ?
	`, commentID).Scan(
		&comment.ID, &comment.ItemID, &authorIDNull, &comment.Content, &comment.IsPrivate,
		&comment.CreatedAt, &comment.UpdatedAt,
		&authorFirstName, &authorLastName, &authorEmail,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated comment: %w", err)
	}

	if authorIDNull.Valid {
		id := int(authorIDNull.Int64)
		comment.AuthorID = &id
	}
	if authorFirstName.Valid && authorLastName.Valid {
		comment.AuthorName = fmt.Sprintf("%s %s", authorFirstName.String, authorLastName.String)
	}
	if authorEmail.Valid {
		comment.AuthorEmail = authorEmail.String
	}

	return &comment, nil
}

// Delete removes a comment
func (s *CommentService) Delete(commentID int) error {
	// Check if comment exists
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM comments WHERE id = ?)", commentID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check comment: %w", err)
	}
	if !exists {
		return fmt.Errorf("comment not found: %d", commentID)
	}

	_, err = s.db.Exec("DELETE FROM comments WHERE id = ?", commentID)
	if err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	return nil
}

// GetByItemID retrieves all comments for an item
func (s *CommentService) GetByItemID(itemID int) ([]models.Comment, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.item_id, c.author_id, c.content, c.is_private, c.created_at, c.updated_at,
		       u.first_name || ' ' || u.last_name as author_name, u.email as author_email
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		WHERE c.item_id = ?
		ORDER BY c.created_at DESC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var c models.Comment
		var authorID sql.NullInt64
		var authorName, authorEmail sql.NullString

		err := rows.Scan(
			&c.ID, &c.ItemID, &authorID, &c.Content, &c.IsPrivate,
			&c.CreatedAt, &c.UpdatedAt, &authorName, &authorEmail,
		)
		if err != nil {
			continue
		}

		if authorID.Valid {
			id := int(authorID.Int64)
			c.AuthorID = &id
		}
		if authorName.Valid {
			c.AuthorName = authorName.String
		}
		if authorEmail.Valid {
			c.AuthorEmail = authorEmail.String
		}

		comments = append(comments, c)
	}

	if comments == nil {
		comments = []models.Comment{}
	}

	return comments, nil
}

// GetWorkspaceIDForComment returns the workspace ID for a comment's item
func (s *CommentService) GetWorkspaceIDForComment(commentID int) (int, error) {
	var workspaceID int
	err := s.db.QueryRow(`
		SELECT i.workspace_id
		FROM comments c
		JOIN items i ON c.item_id = i.id
		WHERE c.id = ?
	`, commentID).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("comment not found: %d", commentID)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace ID: %w", err)
	}
	return workspaceID, nil
}

// GetAuthorID returns the author ID of a comment
func (s *CommentService) GetAuthorID(commentID int) (int, error) {
	var authorID int
	err := s.db.QueryRow("SELECT author_id FROM comments WHERE id = ?", commentID).Scan(&authorID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("comment not found: %d", commentID)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get author ID: %w", err)
	}
	return authorID, nil
}
