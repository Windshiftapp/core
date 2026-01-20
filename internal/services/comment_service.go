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

// CommentService encapsulates comment creation logic used by both HTTP handlers
// and action automation service.
type CommentService struct {
	db                  database.Database
	activityTracker     *ActivityTracker
	notificationService *NotificationService
	mentionService      *MentionService
	webhookSender       WebhookDispatcher
}

// CreateCommentParams contains the parameters for creating a comment.
type CreateCommentParams struct {
	ItemID      int
	AuthorID    int
	Content     string    // Raw content (will be sanitized)
	IsPrivate   bool      // For action automation private notes
	ActorUserID int       // User performing the action (for notifications)
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

// Create creates a new comment with all associated side effects:
// activity tracking, notifications, mentions, and webhooks.
func (s *CommentService) Create(params CreateCommentParams) (*CreateCommentResult, error) {
	// 1. Sanitize content (XSS prevention)
	sanitizedContent := utils.StripHTMLTags(params.Content)

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
	var commentID int64
	err = s.db.QueryRow(`
		INSERT INTO comments (item_id, author_id, content, is_private, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, params.ItemID, params.AuthorID, sanitizedContent, params.IsPrivate, now, now).Scan(&commentID)
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

		// Get actor username for notification
		var actorName string
		_ = s.db.QueryRow("SELECT username FROM users WHERE id = ?", params.ActorUserID).Scan(&actorName)
		if actorName == "" {
			actorName = fmt.Sprintf("User #%d", params.ActorUserID)
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

	// 8. Return created comment
	return &CreateCommentResult{
		CommentID: commentID,
	}, nil
}
