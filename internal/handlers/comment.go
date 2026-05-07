package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
	"windshift/internal/webhook"
)

// CommentHandler handles comment-related HTTP requests
type CommentHandler struct {
	db                  database.Database
	permissionService   *services.PermissionService
	activityTracker     *services.ActivityTracker
	mentionService      *services.MentionService // Mention service for processing @mentions (optional, can be nil)
	notificationService interface {
		EmitEvent(event *services.NotificationEvent)
	} // Notification service for async notification processing (optional, can be nil)
	webhookSender    *webhook.WebhookSender   // Webhook sender for dispatching webhook events (optional, can be nil)
	commentService   *services.CommentService // CommentService for unified comment creation logic
	issueSyncService interface {
		PushCommentToGitHub(ctx context.Context, itemID int, commentID int, authorID int, commentBody string)
		PushCommentUpdateToGitHub(ctx context.Context, commentID int, authorID int, newBody string)
	} // Issue sync service for pushing comments to GitHub (optional, can be nil)
	approvalService *services.ApprovalService // Approval service for approver-derived item-view fallback (optional, can be nil)
}

// NewCommentHandler creates a new comment handler
func NewCommentHandler(db database.Database, permissionService *services.PermissionService, activityTracker *services.ActivityTracker, notificationService interface {
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

// SetIssueSyncService sets the issue sync service for pushing comments to GitHub
func (h *CommentHandler) SetIssueSyncService(svc interface {
	PushCommentToGitHub(ctx context.Context, itemID int, commentID int, authorID int, commentBody string)
	PushCommentUpdateToGitHub(ctx context.Context, commentID int, authorID int, newBody string)
}) {
	h.issueSyncService = svc
}

// SetApprovalService wires the approval service so that comment-read endpoints
// fall back to approver-pool membership when the caller lacks workspace
// item.view (mirrors the documented exception in approvals.go's Decide).
func (h *CommentHandler) SetApprovalService(ap *services.ApprovalService) {
	h.approvalService = ap
}

// GetComments handles GET /api/items/{id}/comments
func (h *CommentHandler) GetComments(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Require authentication
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get item's workspace_id for permission check
	workspaceID, err := repository.NewItemRepository(h.db).GetWorkspaceID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to fetch item: %w", err))
		return
	}

	// Check if user has permission to view items in this workspace. Active
	// approvers without workspace item.view are allowed through so they can
	// read the comment thread for context before deciding.
	canView, err := h.canViewItemAsActor(user.ID, itemID, workspaceID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("permission check failed: %w", err))
		return
	}
	if !canView {
		respondNotFound(w, r, "Item")
		return
	}

	// Approval-engine comment text (approval_decisions.comment) is unioned in
	// here so it surfaces in the same feed users already read. Synthetic rows
	// use a negated id so they can't be edited or deleted via the existing
	// id-based handlers, and source='approval' lets the UI tag them.
	query := `
		SELECT c.id, c.item_id, c.author_id, c.portal_customer_id, c.content, c.is_private, c.created_at, c.updated_at,
		       u.first_name, u.last_name, u.email, u.avatar_url,
		       pc.name as customer_name, pc.email as customer_email,
		       'human' AS source, COALESCE(u.is_agent, FALSE) AS is_agent
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		LEFT JOIN portal_customers pc ON c.portal_customer_id = pc.id
		WHERE c.item_id = ?
		UNION ALL
		SELECT
		       -d.id AS id,
		       ar.item_id,
		       d.actor_user_id AS author_id,
		       NULL AS portal_customer_id,
		       d.comment AS content,
		       FALSE AS is_private,
		       d.created_at AS created_at,
		       d.created_at AS updated_at,
		       u.first_name, u.last_name, u.email, u.avatar_url,
		       NULL AS customer_name, NULL AS customer_email,
		       'approval' AS source, COALESCE(u.is_agent, FALSE) AS is_agent
		FROM approval_decisions d
		JOIN approval_requests ar ON ar.id = d.approval_request_id
		LEFT JOIN users u ON u.id = d.actor_user_id
		WHERE ar.item_id = ? AND d.comment IS NOT NULL AND d.comment <> ''
		ORDER BY created_at DESC
	`

	rows, err := h.db.Query(query, itemID, itemID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to fetch comments: %w", err))
		return
	}
	defer func() { _ = rows.Close() }()

	var comments []models.Comment
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to scan comment: %w", err))
			return
		}

		comments = append(comments, comment)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, fmt.Errorf("error reading comments: %w", err))
		return
	}

	respondJSONOK(w, comments)
}

// CreateComment handles POST /api/items/{id}/comments
func (h *CommentHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Require authentication
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var reqBody struct {
		Content   string `json:"content"`
		AuthorID  int    `json:"author_id"`
		IsPrivate bool   `json:"is_private"`
	}

	if err = json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
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
	workspaceID, err := repository.NewItemRepository(h.db).GetWorkspaceID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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
		respondNotFound(w, r, "Item")
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
		var result *services.CreateCommentResult
		result, err = h.commentService.Create(services.CreateCommentParams{
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

	// Push comment to GitHub if issue sync is configured
	if h.issueSyncService != nil && !reqBody.IsPrivate {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.issueSyncService.PushCommentToGitHub(ctx, itemID, int(commentID), reqBody.AuthorID, reqBody.Content)
		}()
	}

	// Fetch the created comment with author details for response
	comment, err := h.getCommentByID(int(commentID))
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to fetch created comment: %w", err))
		return
	}

	respondJSONCreated(w, comment)
}

// commentEditContext holds all data needed for comment edit/delete operations.
type commentEditContext struct {
	CommentID           int
	ItemID              int
	AuthorID            int
	WorkspaceID         int
	ItemTitle           string
	WorkspaceItemNumber int
	WorkspaceKey        string
	AssigneeID          sql.NullInt64
	CreatorID           sql.NullInt64
	User                *models.User
}

// requireCommentEditAccess validates comment ID, authenticates the user,
// fetches comment + item details, and checks author or edit-others permission.
func (h *CommentHandler) requireCommentEditAccess(w http.ResponseWriter, r *http.Request) (*commentEditContext, bool) {
	commentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return nil, false
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return nil, false
	}

	var itemID, authorID int
	err = h.db.QueryRow("SELECT item_id, author_id FROM comments WHERE id = ?", commentID).Scan(&itemID, &authorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "comment")
			return nil, false
		}
		respondInternalError(w, r, fmt.Errorf("failed to fetch comment: %w", err))
		return nil, false
	}

	ctx := &commentEditContext{
		CommentID: commentID,
		ItemID:    itemID,
		AuthorID:  authorID,
		User:      user,
	}

	item, err := repository.NewItemRepository(h.db).FindByIDWithDetails(itemID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to fetch item workspace: %w", err))
		return nil, false
	}
	ctx.WorkspaceID = item.WorkspaceID
	ctx.ItemTitle = item.Title
	ctx.WorkspaceItemNumber = item.WorkspaceItemNumber
	ctx.WorkspaceKey = item.WorkspaceKey
	if item.AssigneeID != nil {
		ctx.AssigneeID = sql.NullInt64{Int64: int64(*item.AssigneeID), Valid: true}
	}
	if item.CreatorID != nil {
		ctx.CreatorID = sql.NullInt64{Int64: int64(*item.CreatorID), Valid: true}
	}

	isAuthor := user.ID == authorID
	if !isAuthor {
		canEditOthers, permErr := h.canEditOthersComments(user.ID, ctx.WorkspaceID)
		if permErr != nil {
			respondInternalError(w, r, fmt.Errorf("permission check failed: %w", permErr))
			return nil, false
		}
		if !canEditOthers {
			respondNotFound(w, r, "Item")
			return nil, false
		}
	}

	return ctx, true
}

// UpdateComment handles PUT /api/comments/{id}
func (h *CommentHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	ctx, ok := h.requireCommentEditAccess(w, r)
	if !ok {
		return
	}

	commentID := ctx.CommentID
	itemID := ctx.ItemID
	workspaceID := ctx.WorkspaceID
	itemTitle := ctx.ItemTitle
	user := ctx.User
	assigneeID := ctx.AssigneeID
	creatorID := ctx.CreatorID

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
	if h.notificationService != nil {
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

	// Push comment edit to GitHub if issue sync is configured
	if h.issueSyncService != nil && !comment.IsPrivate {
		go func() {
			syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.issueSyncService.PushCommentUpdateToGitHub(syncCtx, commentID, ctx.AuthorID, reqBody.Content)
		}()
	}

	respondJSONOK(w, comment)
}

// DeleteComment handles DELETE /api/comments/{id}
func (h *CommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	ctx, ok := h.requireCommentEditAccess(w, r)
	if !ok {
		return
	}

	commentID := ctx.CommentID
	itemID := ctx.ItemID
	workspaceID := ctx.WorkspaceID
	itemTitle := ctx.ItemTitle
	user := ctx.User
	assigneeID := ctx.AssigneeID
	creatorID := ctx.CreatorID

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

	// Clean up orphaned mention records for the deleted comment
	if h.mentionService != nil {
		_ = h.mentionService.DeleteMentionsForSource("comment", commentID)
	}

	// Emit notification event
	if h.notificationService != nil {
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

// scanComment scans a comment row (from *sql.Row or *sql.Rows) and populates
// the author name, email, and avatar fields from the joined user/customer columns.
func scanComment(scanner interface {
	Scan(dest ...interface{}) error
}) (models.Comment, error) {
	var comment models.Comment
	var authorID, portalCustomerID sql.NullInt64
	var firstName, lastName sql.NullString
	var email, avatarURL sql.NullString
	var customerName, customerEmail sql.NullString
	var isAgent sql.NullBool

	err := scanner.Scan(
		&comment.ID, &comment.ItemID, &authorID, &portalCustomerID, &comment.Content, &comment.IsPrivate,
		&comment.CreatedAt, &comment.UpdatedAt,
		&firstName, &lastName, &email, &avatarURL,
		&customerName, &customerEmail,
		&comment.Source, &isAgent,
	)
	if err != nil {
		return comment, err
	}
	comment.IsAgent = isAgent.Valid && isAgent.Bool

	comment.AuthorID = utils.NullInt64ToPtr(authorID)
	comment.PortalCustomerID = utils.NullInt64ToPtr(portalCustomerID)

	// Construct author name - prefer user info, fall back to portal customer
	switch {
	case firstName.Valid && lastName.Valid:
		comment.AuthorName = strings.TrimSpace(firstName.String + " " + lastName.String)
	case firstName.Valid:
		comment.AuthorName = firstName.String
	case lastName.Valid:
		comment.AuthorName = lastName.String
	case customerName.Valid:
		comment.AuthorName = customerName.String
	default:
		comment.AuthorName = "Unknown User"
	}

	// Set email - prefer user email, fall back to portal customer
	switch {
	case email.Valid:
		comment.AuthorEmail = email.String
	case customerEmail.Valid:
		comment.AuthorEmail = customerEmail.String
	}

	comment.AuthorAvatar = avatarURL.String

	return comment, nil
}

// Helper function to get a comment by ID with author details
func (h *CommentHandler) getCommentByID(commentID int) (*models.Comment, error) {
	query := `
		SELECT c.id, c.item_id, c.author_id, c.portal_customer_id, c.content, c.is_private, c.created_at, c.updated_at,
		       u.first_name, u.last_name, u.email, u.avatar_url,
		       pc.name as customer_name, pc.email as customer_email,
		       'human' AS source, COALESCE(u.is_agent, FALSE) AS is_agent
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		LEFT JOIN portal_customers pc ON c.portal_customer_id = pc.id
		WHERE c.id = ?
	`

	comment, err := scanComment(h.db.QueryRow(query, commentID))
	if err != nil {
		return nil, err
	}

	return &comment, nil
}

// Permission helper methods

// canViewItemAsActor checks workspace item-view permission with the approver-pool fallback so an
// active approver can read item comments to inform their decision. See
// CheckItemPermissionAsActor for the security model.
func (h *CommentHandler) canViewItemAsActor(userID, itemID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		slog.Error("permission service unavailable, denying access", slog.String("component", "comment"))
		return false, nil
	}
	return userCanViewItemAsActor(userID, itemID, workspaceID, h.permissionService, h.approvalService)
}

// canCommentOnItem checks if a user can comment on items in a specific workspace
func (h *CommentHandler) canCommentOnItem(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		slog.Error("permission service unavailable, denying access", slog.String("component", "comment"))
		return false, nil
	}
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemComment)
}

// canEditOthersComments checks if a user can edit other users' comments in a specific workspace
func (h *CommentHandler) canEditOthersComments(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		slog.Error("permission service unavailable, denying access", slog.String("component", "comment"))
		return false, nil
	}
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionCommentEditOthers)
}
