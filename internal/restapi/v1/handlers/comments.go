package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/services"
)

// CommentHandler handles public API requests for standalone comments
type CommentHandler struct {
	BaseHandler
	commentService *services.CommentService
}

// NewCommentHandler creates a new comment handler
func NewCommentHandler(db database.Database, permissionService *services.PermissionService) *CommentHandler {
	return &CommentHandler{
		BaseHandler:    NewBaseHandler(db, permissionService),
		commentService: services.NewCommentService(db),
	}
}

// SetCommentService allows injecting a configured comment service
func (h *CommentHandler) SetCommentService(cs *services.CommentService) {
	h.commentService = cs
}

// checkCommentEditPermission verifies the user is the comment author or has workspace edit permission.
// Returns false and writes an error response on failure.
func (h *CommentHandler) checkCommentEditPermission(w http.ResponseWriter, r *http.Request, commentID, userID int) bool {
	authorID, err := h.commentService.GetAuthorID(commentID)
	if err != nil {
		h.RespondNotFound(w, r)
		return false
	}

	if authorID != userID {
		workspaceID, err := h.commentService.GetWorkspaceIDForComment(commentID)
		if err != nil {
			h.RespondInternalError(w, r)
			return false
		}

		canEdit, permErr := h.Perms.CanEditWorkspace(userID, workspaceID)
		if permErr != nil || !canEdit {
			h.RespondNotFound(w, r)
			return false
		}
	}

	return true
}

// Get handles GET /rest/api/v1/comments/{id}
func (h *CommentHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	commentID, ok := h.ParsePathID(w, r, "id", "comment ID")
	if !ok {
		return
	}

	// Use service to get comment
	commentWithDetails, err := h.commentService.Get(commentID)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	// Check permission
	canView, err := h.Perms.CanViewWorkspace(user.ID, commentWithDetails.WorkspaceID)
	if err != nil || !canView {
		h.RespondNotFound(w, r)
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

	h.RespondOK(w, comment)
}

// Update handles PUT /rest/api/v1/comments/{id}
func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	commentID, ok := h.ParsePathID(w, r, "id", "comment ID")
	if !ok {
		return
	}

	if !h.checkCommentEditPermission(w, r, commentID, user.ID) {
		return
	}

	var req dto.CommentUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	if !h.ValidateRequiredString(w, r, req.Content, "content") {
		return
	}

	// Use service to update comment
	updatedComment, err := h.commentService.Update(commentID, req.Content, user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
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

	h.RespondOK(w, comment)
}

// Delete handles DELETE /rest/api/v1/comments/{id}
func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	commentID, ok := h.ParsePathID(w, r, "id", "comment ID")
	if !ok {
		return
	}

	if !h.checkCommentEditPermission(w, r, commentID, user.ID) {
		return
	}

	// Use service to delete comment
	err := h.commentService.Delete(commentID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}
