package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/models"
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
//
// @Summary      Get a comment by ID
// @Description  Returns 404 (not 403) when the caller cannot view the comment's parent workspace — workspace existence is never leaked.
// @Tags         comments
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Comment ID"
// @Success      200  {object}  dto.CommentResponse
// @Failure      400  {object}  restapi.ErrorResponse  "Invalid comment ID"
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      403  {object}  restapi.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  restapi.ErrorResponse  "Comment not found or not visible to caller"
// @Failure      500  {object}  restapi.ErrorResponse
// @Router       /comments/{id} [get]
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

// requireEditableComment authenticates the caller, parses the comment ID from
// the path, and enforces edit permission. Returns (0, nil, false) after
// writing an HTTP response on failure.
func (h *CommentHandler) requireEditableComment(w http.ResponseWriter, r *http.Request) (int, *models.User, bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return 0, nil, false
	}
	commentID, ok := h.ParsePathID(w, r, "id", "comment ID")
	if !ok {
		return 0, nil, false
	}
	if !h.checkCommentEditPermission(w, r, commentID, user.ID) {
		return 0, nil, false
	}
	return commentID, user, true
}

// Update handles PUT /rest/api/v1/comments/{id}
//
// @Summary      Update a comment
// @Description  The caller must be the comment author or hold edit permission on the comment's workspace.
// @Tags         comments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                       true  "Comment ID"
// @Param        body  body      dto.CommentUpdateRequest  true  "Updated content"
// @Success      200   {object}  dto.CommentResponse
// @Failure      400   {object}  restapi.ErrorResponse  "Invalid comment ID, request body, or empty content"
// @Failure      401   {object}  restapi.ErrorResponse
// @Failure      403   {object}  restapi.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  restapi.ErrorResponse  "Comment not found or caller cannot edit it"
// @Failure      500   {object}  restapi.ErrorResponse
// @Router       /comments/{id} [put]
func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	commentID, user, ok := h.requireEditableComment(w, r)
	if !ok {
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
//
// @Summary      Delete a comment
// @Description  The caller must be the comment author or hold edit permission on the comment's workspace.
// @Tags         comments
// @Security     BearerAuth
// @Param        id   path  int  true  "Comment ID"
// @Success      204  "Comment deleted"
// @Failure      400  {object}  restapi.ErrorResponse  "Invalid comment ID"
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      403  {object}  restapi.ErrorResponse  "Token lacks the items:delete scope"
// @Failure      404  {object}  restapi.ErrorResponse  "Comment not found or caller cannot delete it"
// @Failure      500  {object}  restapi.ErrorResponse
// @Router       /comments/{id} [delete]
func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	commentID, _, ok := h.requireEditableComment(w, r)
	if !ok {
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
