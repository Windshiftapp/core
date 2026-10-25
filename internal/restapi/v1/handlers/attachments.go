package handlers

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"windshift/internal/blobstore"
	"windshift/internal/database"
	"windshift/internal/fileserve"
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
	storage *blobstore.Store
	repo    *repository.AttachmentRepository
}

// NewAttachmentHandler constructs the v1 attachment handler. storage selects
// the configured attachment backend; when disabled, the handlers respond with a
// service-unavailable error.
func NewAttachmentHandler(db database.Database, permissionService *services.PermissionService, storage *blobstore.Store) *AttachmentHandler {
	return &AttachmentHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		storage:     storage,
		repo:        repository.NewAttachmentRepository(db),
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
	if !h.storage.Enabled() {
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

	record, err := h.repo.GetItemDownloadRecord(attachmentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		slog.Error("attachment lookup failed", slog.String("component", "v1/attachments"), slog.Any("error", err))
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	canView, err := h.Perms.CanViewWorkspace(user.ID, record.WorkspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrItemNotFound)
		return
	}

	// Open the blob through the configured backend. The local backend keeps its
	// os.Root confinement; relative email-ingestion rows resolve against the
	// same root.
	file, err := h.storage.Open(r.Context(), record.FilePath)
	if err != nil {
		if errors.Is(err, blobstore.ErrOutsideRoot) {
			slog.Warn("attachment path traversal blocked", slog.String("component", "v1/attachments"), slog.Int("attachment_id", attachmentID), slog.String("file_path", record.FilePath))
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		if errors.Is(err, blobstore.ErrNotFound) {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		slog.Error("failed to open attachment file", slog.String("component", "v1/attachments"), slog.Any("error", err))
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", record.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size(), 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	// CLI consumers always want a download; force the disposition rather than
	// inheriting the legacy "inline for safe MIME types" branch (the legacy
	// handler serves the same files to browsers, which is the only case where
	// inline display matters).
	w.Header().Set("Content-Disposition", fileserve.ContentDisposition("attachment", record.OriginalFilename))

	if _, err := io.Copy(w, file); err != nil {
		slog.Error("failed to stream attachment", slog.String("component", "v1/attachments"), slog.Any("error", err))
	}
}

// Thumbnail handles GET /rest/api/v1/attachments/{id}/thumbnail
//
// @Summary      Download an attachment thumbnail
// @Tags         items
// @Produce      image/jpeg
// @Security     BearerAuth
// @Param        id   path      int  true  "Attachment ID"
// @Success      200  {file}    binary
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Attachment not found, has no thumbnail, or is not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /attachments/{id}/thumbnail [get]
func (h *AttachmentHandler) Thumbnail(w http.ResponseWriter, r *http.Request) {
	if !h.storage.Enabled() {
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

	record, err := h.repo.GetItemThumbnailRecord(attachmentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		slog.Error("attachment thumbnail lookup failed", slog.String("component", "v1/attachments"), slog.Any("error", err))
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	canView, err := h.Perms.CanViewWorkspace(user.ID, record.WorkspaceID)
	if err != nil || !canView {
		restapi.RespondError(w, r, restapi.ErrItemNotFound)
		return
	}

	file, err := h.storage.Open(r.Context(), record.ThumbnailPath)
	if err != nil {
		if errors.Is(err, blobstore.ErrOutsideRoot) {
			slog.Warn("attachment thumbnail path traversal blocked", slog.String("component", "v1/attachments"), slog.Int("attachment_id", attachmentID), slog.String("thumbnail_path", record.ThumbnailPath))
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		if errors.Is(err, blobstore.ErrNotFound) {
			restapi.RespondError(w, r, restapi.ErrItemNotFound)
			return
		}
		slog.Error("failed to open attachment thumbnail", slog.String("component", "v1/attachments"), slog.Any("error", err))
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size(), 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "public, max-age=31536000")

	if _, err := io.Copy(w, file); err != nil {
		slog.Error("failed to stream attachment thumbnail", slog.String("component", "v1/attachments"), slog.Any("error", err))
	}
}
