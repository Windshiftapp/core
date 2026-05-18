package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// AttachmentHandler serves attachment binaries on the v1 surface.
//
// List lookups stay on ItemHandler.GetAttachments (already registered on
// /items/{id}/attachments). This handler exists so bearer-token callers can
// reach the bytes themselves — the legacy /api/attachments/{id}/download
// route explicitly rejects bearer tokens.
type AttachmentHandler struct {
	BaseHandler
	attachmentPath string
}

// NewAttachmentHandler constructs the v1 attachment handler. attachmentPath is
// the configured base directory for attachment storage; when empty, the
// Download handler responds with a service-unavailable error.
func NewAttachmentHandler(db database.Database, permissionService *services.PermissionService, attachmentPath string) *AttachmentHandler {
	return &AttachmentHandler{
		BaseHandler:    NewBaseHandler(db, permissionService),
		attachmentPath: attachmentPath,
	}
}

// Download handles GET /rest/api/v1/attachments/{id}/download
//
// @Summary      Download an attachment's binary contents
// @Tags         items
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        id   path      int  true  "Attachment ID"
// @Success      200  {file}    binary
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Attachment not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /attachments/{id}/download [get]
func (h *AttachmentHandler) Download(w http.ResponseWriter, r *http.Request) {
	if h.attachmentPath == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusServiceUnavailable, restapi.ErrCodeServiceUnavailable, "Attachments are not enabled on this server"))
		return
	}

	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	attachmentID, ok := h.ParsePathID(w, r, "id", "attachment ID")
	if !ok {
		return
	}

	var (
		itemID           sql.NullInt64
		entityType       sql.NullString
		filename         string
		originalFilename string
		filePath         string
		mimeType         string
		fileSize         int64
	)
	err := h.DB.QueryRow(`
		SELECT item_id, entity_type, filename, original_filename, file_path, mime_type, file_size
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(&itemID, &entityType, &filename, &originalFilename, &filePath, &mimeType, &fileSize)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		slog.Error("attachment lookup failed", slog.String("component", "v1/attachments"), slog.Any("error", err))
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	// Only item-scoped attachments are exposed on this route. test_result and
	// other entity types still have their own paths off the cookie-auth
	// surface; routing them through items:read here would let an items token
	// pull bytes from unrelated resources.
	if entityType.Valid && entityType.String != "" && entityType.String != "item" {
		restapi.RespondError(w, r, restapi.ErrItemNotFound)
		return
	}
	if !itemID.Valid {
		restapi.RespondError(w, r, restapi.ErrItemNotFound)
		return
	}

	workspaceID, err := repository.NewItemRepository(h.DB).GetWorkspaceID(int(itemID.Int64))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	canView, err := h.Perms.CanViewWorkspace(user.ID, workspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrItemNotFound)
		return
	}

	// Path traversal guard: the resolved file path must live under the
	// configured attachment directory. Mirrors the legacy handler's check.
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	absBase, err := filepath.Abs(h.attachmentPath)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	if !strings.HasPrefix(absPath, absBase+string(os.PathSeparator)) {
		slog.Warn("attachment path traversal blocked", slog.String("component", "v1/attachments"), slog.Int("attachment_id", attachmentID))
		restapi.RespondError(w, r, restapi.ErrItemNotFound)
		return
	}

	file, err := os.Open(absPath) //nolint:gosec // path was just validated against the configured base
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		slog.Error("failed to open attachment file", slog.String("component", "v1/attachments"), slog.Any("error", err))
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	// CLI consumers always want a download; force the disposition rather than
	// inheriting the legacy "inline for safe MIME types" branch (the legacy
	// handler serves the same files to browsers, which is the only case where
	// inline display matters).
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", originalFilename))

	if _, err := io.Copy(w, file); err != nil {
		slog.Error("failed to stream attachment", slog.String("component", "v1/attachments"), slog.Any("error", err))
	}
}
