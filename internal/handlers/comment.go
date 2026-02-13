package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
	"windshift/internal/webhook"
)

// CommentHandler handles comment-related HTTP requests
type CommentHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	activityTracker   *services.ActivityTracker
	mentionService    *services.MentionService // Mention service for processing @mentions (optional, can be nil)
	notificationService interface{
		EmitEvent(event *services.NotificationEvent)
	} // Notification service for async notification processing (optional, can be nil)
	webhookSender  *webhook.WebhookSender     // Webhook sender for dispatching webhook events (optional, can be nil)
	commentService *services.CommentService   // CommentService for unified comment creation logic
}

// NewCommentHandler creates a new comment handler
func NewCommentHandler(db database.Database, permissionService *services.PermissionService, activityTracker *services.ActivityTracker, notificationService interface{
	EmitEvent(event *services.NotificationEvent)
}) *CommentHandler {
	return &CommentHandler{
		db:                  db,
		permissionService:   permissionService,
		activityTracker:     activityTracker,
		notificationService: notificationService,
	}
}

// SetWebhookSender sets the webhook sender for dispatching webhook events
func (h *CommentHandler) SetWebhookSender(sender *webhook.WebhookSender) {
	h.webhookSender = sender
}

// SetMentionService sets the mention service for processing @mentions
func (h *CommentHandler) SetMentionService(mentionService *services.MentionService) {
	h.mentionService = mentionService
}

// SetCommentService sets the comment service for unified comment creation
func (h *CommentHandler) SetCommentService(commentService *services.CommentService) {
	h.commentService = commentService
}

// GetComments handles GET /api/items/{id}/comments
func (h *CommentHandler) GetComments(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get item's workspace_id for permission check
	var workspaceID int
	err = h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to fetch item: %w", err))
		return
	}

	// Check if user has permission to view items in this workspace
	canView, err := h.canViewItem(user.ID, workspaceID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("permission check failed: %w", err))
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	query := `
		SELECT c.id, c.item_id, c.author_id, c.portal_customer_id, c.content, c.is_private, c.created_at, c.updated_at,
		       u.first_name, u.last_name, u.email, u.avatar_url,
		       pc.name as customer_name, pc.email as customer_email
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		LEFT JOIN portal_customers pc ON c.portal_customer_id = pc.id
		WHERE c.item_id = ?
		ORDER BY c.created_at DESC
	`

	rows, err := h.db.Query(query, itemID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to fetch comments: %w", err))
		return
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var comment models.Comment
		var authorID, portalCustomerID sql.NullInt64
		var firstName, lastName sql.NullString
		var email, avatarURL sql.NullString
		var customerName, customerEmail sql.NullString

		err := rows.Scan(
			&comment.ID, &comment.ItemID, &authorID, &portalCustomerID, &comment.Content, &comment.IsPrivate,
			&comment.CreatedAt, &comment.UpdatedAt,
			&firstName, &lastName, &email, &avatarURL,
			&customerName, &customerEmail,
		)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to scan comment: %w", err))
			return
		}

		// Set author or portal customer ID
		comment.AuthorID = utils.NullInt64ToPtr(authorID)
		comment.PortalCustomerID = utils.NullInt64ToPtr(portalCustomerID)

		// Construct author name - prefer user info, fall back to portal customer
		if firstName.Valid && lastName.Valid {
			comment.AuthorName = strings.TrimSpace(firstName.String + " " + lastName.String)
		} else if firstName.Valid {
			comment.AuthorName = firstName.String
		} else if lastName.Valid {
			comment.AuthorName = lastName.String
		} else if customerName.Valid {
			comment.AuthorName = customerName.String
		} else {
			comment.AuthorName = "Unknown User"
		}

		// Set email - prefer user email, fall back to portal customer
		if email.Valid {
			comment.AuthorEmail = email.String
		} else if customerEmail.Valid {
			comment.AuthorEmail = customerEmail.String
		}

		comment.AuthorAvatar = avatarURL.String

		comments = append(comments, comment)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, fmt.Errorf("error reading comments: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

// CreateComment handles POST /api/items/{id}/comments
func (h *CommentHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	var reqBody struct {
		Content   string `json:"content"`
		AuthorID  int    `json:"author_id"`
		IsPrivate bool   `json:"is_private"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if strings.TrimSpace(reqBody.Content) == "" {
		respondValidationError(w, r, "Content is required")
		return
	}

	if reqBody.AuthorID <= 0 {
		respondValidationError(w, r, "Author ID is required")
		return
	}

	// Get item's workspace_id for permission check
	var workspaceID int
	err = h.db.QueryRow(`SELECT workspace_id FROM items WHERE id = ?`, itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to fetch item: %w", err))
		return
	}

	// Check if user has permission to comment on items in this workspace
	canComment, err := h.canCommentOnItem(user.ID, workspaceID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("permission check failed: %w", err))
		return
	}
	if !canComment {
		respondForbidden(w, r)
		return
	}

	// Verify the author exists
	var exists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", reqBody.AuthorID).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to verify author: %w", err))
		return
	}
	if !exists {
		respondNotFound(w, r, "user")
		return
	}

	// Use CommentService if available, otherwise fall back to legacy inline logic
	var commentID int64
	if h.commentService != nil {
		result, err := h.commentService.Create(services.CreateCommentParams{
			ItemID:      itemID,
			AuthorID:    reqBody.AuthorID,
			Content:     reqBody.Content,
			IsPrivate:   reqBody.IsPrivate,
			ActorUserID: user.ID,
		})
		if err != nil {
			slog.Error("failed to create comment via service", slog.String("component", "comment"), slog.Any("error", err))
			respondInternalError(w, r, fmt.Errorf("failed to create comment: %w", err))
			return
		}
		commentID = result.CommentID
	} else {
		// Legacy fallback: direct DB insert without side effects
		// This path should not be used in production - CommentService should always be set
		slog.Warn("commentService is nil, using legacy comment creation without notifications/mentions/webhooks",
			slog.String("component", "comment"),
			slog.Int("item_id", itemID))

		sanitizedContent := utils.SanitizeCommentContent(reqBody.Content)
		now := time.Now()
		err = h.db.QueryRow(`
			INSERT INTO comments (item_id, author_id, content, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?) RETURNING id
		`, itemID, reqBody.AuthorID, sanitizedContent, now, now).Scan(&commentID)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to create comment: %w", err))
			return
		}
	}

	// Fetch the created comment with author details for response
	comment, err := h.getCommentByID(int(commentID))
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to fetch created comment: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(comment)
}

// UpdateComment handles PUT /api/comments/{id}
func (h *CommentHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	commentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get comment details to check author and workspace
	var itemID, authorID int
	err = h.db.QueryRow("SELECT item_id, author_id FROM comments WHERE id = ?", commentID).Scan(&itemID, &authorID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "comment")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to fetch comment: %w", err))
		return
	}

	// Get item details for permission check and notifications
	var workspaceID int
	var itemTitle string
	var workspaceItemNumber int
	var workspaceKey string
	var assigneeID, creatorID sql.NullInt64
	err = h.db.QueryRow(`
		SELECT i.workspace_id, i.title, i.workspace_item_number, w.key, i.assignee_id, i.creator_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, itemID).Scan(&workspaceID, &itemTitle, &workspaceItemNumber, &workspaceKey, &assigneeID, &creatorID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to fetch item workspace: %w", err))
		return
	}

	// Check if user is the comment author OR has permission to edit others' comments
	isAuthor := user.ID == authorID
	if !isAuthor {
		canEditOthers, err := h.canEditOthersComments(user.ID, workspaceID)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("permission check failed: %w", err))
			return
		}
		if !canEditOthers {
			respondForbidden(w, r)
			return
		}
	}

	var reqBody struct {
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if strings.TrimSpace(reqBody.Content) == "" {
		respondValidationError(w, r, "Content is required")
		return
	}

	// Sanitize comment content to prevent XSS (strips HTML tags + dangerous Markdown URLs)
	sanitizedContent := utils.SanitizeCommentContent(reqBody.Content)

	// Update the comment
	now := time.Now()
	result, err := h.db.ExecWrite(`
		UPDATE comments
		SET content = ?, updated_at = ?
		WHERE id = ?
	`, sanitizedContent, now, commentID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to update comment: %w", err))
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check update result: %w", err))
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "comment")
		return
	}

	// Fetch the updated comment
	comment, err := h.getCommentByID(commentID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to fetch updated comment: %w", err))
		return
	}

	// Emit notification event
	if h.notificationService != nil && user != nil {
		assigneeIDPtr := utils.NullInt64ToPtr(assigneeID)
		creatorIDPtr := utils.NullInt64ToPtr(creatorID)

		h.notificationService.EmitEvent(&services.NotificationEvent{
			EventType:   models.EventCommentUpdated,
			WorkspaceID: workspaceID,
			ActorUserID: user.ID,
			ItemID:      itemID,
			AssigneeID:  assigneeIDPtr,
			CreatorID:   creatorIDPtr,
			Title:       "Comment Updated",
			TemplateData: map[string]interface{}{
				"item.title": itemTitle,
				"item.id":    itemID,
				"user.name":  user.Username,
			},
		})
	}

	// Process @mentions in updated comment content (handles diff - adds new mentions, removes old ones)
	if h.mentionService != nil {
		if err := h.mentionService.ProcessMentions(services.ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    commentID,
			Content:     reqBody.Content,
			ItemID:      itemID,
			WorkspaceID: workspaceID,
			ActorUserID: user.ID,
		}); err != nil {
			slog.Warn("failed to process mentions", slog.String("component", "comment"), slog.Any("error", err))
			// Don't fail the request if mention processing fails
		}
	}

	// Dispatch webhook event for comment update
	if h.webhookSender != nil {
		itemRepo := repository.NewItemRepository(h.db)
		if item, err := itemRepo.FindByIDWithDetails(itemID); err == nil {
			go h.webhookSender.DispatchEvent("comment.updated", item)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comment)
}

// DeleteComment handles DELETE /api/comments/{id}
func (h *CommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	commentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get comment details to check author and workspace
	var itemID, authorID int
	err = h.db.QueryRow("SELECT item_id, author_id FROM comments WHERE id = ?", commentID).Scan(&itemID, &authorID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "comment")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to fetch comment: %w", err))
		return
	}

	// Get item details for permission check and notifications
	var workspaceID int
	var itemTitle string
	var workspaceItemNumber int
	var workspaceKey string
	var assigneeID, creatorID sql.NullInt64
	err = h.db.QueryRow(`
		SELECT i.workspace_id, i.title, i.workspace_item_number, w.key, i.assignee_id, i.creator_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, itemID).Scan(&workspaceID, &itemTitle, &workspaceItemNumber, &workspaceKey, &assigneeID, &creatorID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to fetch item workspace: %w", err))
		return
	}

	// Check if user is the comment author OR has permission to edit others' comments
	isAuthor := user.ID == authorID
	if !isAuthor {
		canEditOthers, err := h.canEditOthersComments(user.ID, workspaceID)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("permission check failed: %w", err))
			return
		}
		if !canEditOthers {
			respondForbidden(w, r)
			return
		}
	}

	result, err := h.db.ExecWrite("DELETE FROM comments WHERE id = ?", commentID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to delete comment: %w", err))
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check delete result: %w", err))
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "comment")
		return
	}

	// Emit notification event
	if h.notificationService != nil && user != nil {
		assigneeIDPtr := utils.NullInt64ToPtr(assigneeID)
		creatorIDPtr := utils.NullInt64ToPtr(creatorID)

		h.notificationService.EmitEvent(&services.NotificationEvent{
			EventType:   models.EventCommentDeleted,
			WorkspaceID: workspaceID,
			ActorUserID: user.ID,
			ItemID:      itemID,
			AssigneeID:  assigneeIDPtr,
			CreatorID:   creatorIDPtr,
			Title:       "Comment Deleted",
			TemplateData: map[string]interface{}{
				"item.title": itemTitle,
				"item.id":    itemID,
				"user.name":  user.Username,
			},
		})
	}

	// Dispatch webhook event for comment deletion
	if h.webhookSender != nil {
		itemRepo := repository.NewItemRepository(h.db)
		if item, err := itemRepo.FindByIDWithDetails(itemID); err == nil {
			go h.webhookSender.DispatchEvent("comment.deleted", item)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// Helper function to get a comment by ID with author details
func (h *CommentHandler) getCommentByID(commentID int) (*models.Comment, error) {
	query := `
		SELECT c.id, c.item_id, c.author_id, c.portal_customer_id, c.content, c.is_private, c.created_at, c.updated_at,
		       u.first_name, u.last_name, u.email, u.avatar_url,
		       pc.name as customer_name, pc.email as customer_email
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		LEFT JOIN portal_customers pc ON c.portal_customer_id = pc.id
		WHERE c.id = ?
	`

	var comment models.Comment
	var authorID, portalCustomerID sql.NullInt64
	var firstName, lastName sql.NullString
	var email, avatarURL sql.NullString
	var customerName, customerEmail sql.NullString

	err := h.db.QueryRow(query, commentID).Scan(
		&comment.ID, &comment.ItemID, &authorID, &portalCustomerID, &comment.Content, &comment.IsPrivate,
		&comment.CreatedAt, &comment.UpdatedAt,
		&firstName, &lastName, &email, &avatarURL,
		&customerName, &customerEmail,
	)
	if err != nil {
		return nil, err
	}

	// Set author or portal customer ID
	comment.AuthorID = utils.NullInt64ToPtr(authorID)
	comment.PortalCustomerID = utils.NullInt64ToPtr(portalCustomerID)

	// Construct author name - prefer user info, fall back to portal customer
	if firstName.Valid && lastName.Valid {
		comment.AuthorName = strings.TrimSpace(firstName.String + " " + lastName.String)
	} else if firstName.Valid {
		comment.AuthorName = firstName.String
	} else if lastName.Valid {
		comment.AuthorName = lastName.String
	} else if customerName.Valid {
		comment.AuthorName = customerName.String
	} else {
		comment.AuthorName = "Unknown User"
	}

	// Set email - prefer user email, fall back to portal customer
	if email.Valid {
		comment.AuthorEmail = email.String
	} else if customerEmail.Valid {
		comment.AuthorEmail = customerEmail.String
	}

	comment.AuthorAvatar = avatarURL.String

	return &comment, nil
}

// Permission helper methods

// getUserFromContext extracts the user from the request context
func (h *CommentHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}

// canViewItem checks if a user can view items in a specific workspace (needed to view comments)
func (h *CommentHandler) canViewItem(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return true, nil
	}
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
}

// canCommentOnItem checks if a user can comment on items in a specific workspace
func (h *CommentHandler) canCommentOnItem(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return true, nil
	}
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemComment)
}

// canEditOthersComments checks if a user can edit other users' comments in a specific workspace
func (h *CommentHandler) canEditOthersComments(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return true, nil
	}
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionCommentEditOthers)
}