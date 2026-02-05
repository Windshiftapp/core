package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/restapi/v1/shared"
	"windshift/internal/services"
)

// CommentHandler handles public API requests for standalone comments
type CommentHandler struct {
	db             database.Database
	perms          *shared.PermissionHelper
	commentService *services.CommentService
}

// NewCommentHandler creates a new comment handler
func NewCommentHandler(db database.Database, permissionService *services.PermissionService) *CommentHandler {
	return &CommentHandler{
		db:             db,
		perms:          shared.NewPermissionHelper(db, permissionService),
		commentService: services.NewCommentService(db),
	}
}

// SetCommentService allows injecting a configured comment service
func (h *CommentHandler) SetCommentService(cs *services.CommentService) {
	h.commentService = cs
}

// Get handles GET /rest/api/v1/comments/{id}
func (h *CommentHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	commentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid comment ID"))
		return
	}

	// Use service to get comment
	commentWithDetails, err := h.commentService.Get(commentID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	// Check permission
	canView, err := h.perms.CanViewWorkspace(user.ID, commentWithDetails.WorkspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	// Convert to DTO response
	comment := dto.CommentResponse{
		ID:        commentWithDetails.ID,
		ItemID:    commentWithDetails.ItemID,
		Content:   commentWithDetails.Content,
		CreatedAt: commentWithDetails.CreatedAt,
		UpdatedAt: commentWithDetails.UpdatedAt,
	}
	if commentWithDetails.AuthorName != "" {
		comment.Author = &dto.UserSummary{
			FullName: commentWithDetails.AuthorName,
			Email:    commentWithDetails.AuthorEmail,
		}
	}

	restapi.RespondOK(w, comment)
}

// Update handles PUT /rest/api/v1/comments/{id}
func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	commentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid comment ID"))
		return
	}

	// Get comment to check ownership using service
	authorID, err := h.commentService.GetAuthorID(commentID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	workspaceID, err := h.commentService.GetWorkspaceIDForComment(commentID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check if user is author or has admin permission
	if authorID != user.ID {
		canEdit, permErr := h.perms.CanEditWorkspace(user.ID, workspaceID)
		if permErr != nil || !canEdit {
			restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
			return
		}
	}

	var req dto.CommentUpdateRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "content is required"))
		return
	}

	// Use service to update comment
	updatedComment, err := h.commentService.Update(commentID, req.Content, user.ID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Convert to DTO response
	comment := dto.CommentResponse{
		ID:        updatedComment.ID,
		ItemID:    updatedComment.ItemID,
		Content:   updatedComment.Content,
		CreatedAt: updatedComment.CreatedAt,
		UpdatedAt: updatedComment.UpdatedAt,
	}
	if updatedComment.AuthorName != "" {
		comment.Author = &dto.UserSummary{
			FullName: updatedComment.AuthorName,
			Email:    updatedComment.AuthorEmail,
		}
	}

	restapi.RespondOK(w, comment)
}

// Delete handles DELETE /rest/api/v1/comments/{id}
func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	commentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid comment ID"))
		return
	}

	// Get comment to check ownership using service
	authorID, err := h.commentService.GetAuthorID(commentID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	workspaceID, err := h.commentService.GetWorkspaceIDForComment(commentID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check if user is author or has admin permission
	if authorID != user.ID {
		canEdit, permErr := h.perms.CanEditWorkspace(user.ID, workspaceID)
		if permErr != nil || !canEdit {
			restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
			return
		}
	}

	// Use service to delete comment
	err = h.commentService.Delete(commentID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondNoContent(w)
}
