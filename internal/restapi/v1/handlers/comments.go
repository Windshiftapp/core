package handlers

import (
	"database/sql"
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
	db    database.Database
	perms *shared.PermissionHelper
}

// NewCommentHandler creates a new comment handler
func NewCommentHandler(db database.Database, permissionService *services.PermissionService) *CommentHandler {
	return &CommentHandler{
		db:    db,
		perms: shared.NewPermissionHelper(db, permissionService),
	}
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

	// Get comment with item info for permission check
	var comment dto.CommentResponse
	var itemID, workspaceID int
	var authorName, authorEmail sql.NullString

	err = h.db.QueryRow(`
		SELECT c.id, c.item_id, c.author_id, c.content, c.created_at, c.updated_at,
		       u.first_name || ' ' || u.last_name as author_name, u.email as author_email,
		       i.workspace_id
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		JOIN items i ON c.item_id = i.id
		WHERE c.id = ?
	`, commentID).Scan(&comment.ID, &itemID, &comment.Author, &comment.Content,
		&comment.CreatedAt, &comment.UpdatedAt, &authorName, &authorEmail, &workspaceID)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check permission
	canView, _ := h.perms.CanViewWorkspace(user.ID, workspaceID)
	if !canView {
		restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
		return
	}

	comment.ItemID = itemID
	if authorName.Valid {
		comment.Author = &dto.UserSummary{
			FullName: authorName.String,
			Email:    authorEmail.String,
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

	// Get comment to check ownership
	var authorID, workspaceID int
	err = h.db.QueryRow(`
		SELECT c.author_id, i.workspace_id
		FROM comments c
		JOIN items i ON c.item_id = i.id
		WHERE c.id = ?
	`, commentID).Scan(&authorID, &workspaceID)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check if user is author or has admin permission
	if authorID != user.ID {
		canEdit, _ := h.perms.CanEditWorkspace(user.ID, workspaceID)
		if !canEdit {
			restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
			return
		}
	}

	var req dto.CommentUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid JSON body"))
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "content is required"))
		return
	}

	_, err = h.db.ExecWrite(`
		UPDATE comments SET content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, req.Content, commentID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Return updated comment
	var comment dto.CommentResponse
	var itemID int
	var authorName, authorEmail sql.NullString
	h.db.QueryRow(`
		SELECT c.id, c.item_id, c.author_id, c.content, c.created_at, c.updated_at,
		       u.first_name || ' ' || u.last_name as author_name, u.email as author_email
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		WHERE c.id = ?
	`, commentID).Scan(&comment.ID, &itemID, &comment.Author, &comment.Content,
		&comment.CreatedAt, &comment.UpdatedAt, &authorName, &authorEmail)

	comment.ItemID = itemID
	if authorName.Valid {
		comment.Author = &dto.UserSummary{
			FullName: authorName.String,
			Email:    authorEmail.String,
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

	// Get comment to check ownership
	var authorID, workspaceID int
	err = h.db.QueryRow(`
		SELECT c.author_id, i.workspace_id
		FROM comments c
		JOIN items i ON c.item_id = i.id
		WHERE c.id = ?
	`, commentID).Scan(&authorID, &workspaceID)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Check if user is author or has admin permission
	if authorID != user.ID {
		canEdit, _ := h.perms.CanEditWorkspace(user.ID, workspaceID)
		if !canEdit {
			restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
			return
		}
	}

	_, err = h.db.ExecWrite("DELETE FROM comments WHERE id = ?", commentID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondNoContent(w)
}
