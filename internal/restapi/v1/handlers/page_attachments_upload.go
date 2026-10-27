package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"windshift/internal/restapi"
	"windshift/internal/services"
)

// PageAttachmentUploadHandler exposes page-attachment uploads to bearer-
// authenticated callers (the ws CLI, integrations). It is a REST v1 adapter
// over PageAttachmentUploadService; the cookie-auth attachment handler no
// longer sits in this bearer-token path.
type PageAttachmentUploadHandler struct {
	BaseHandler
	uploads *services.PageAttachmentUploadService
}

// NewPageAttachmentUploadHandler constructs the v1 page-attachment upload
// handler.
func NewPageAttachmentUploadHandler(base BaseHandler, uploads *services.PageAttachmentUploadService) *PageAttachmentUploadHandler {
	return &PageAttachmentUploadHandler{BaseHandler: base, uploads: uploads}
}

// Upload handles POST /rest/api/v1/workspaces/{id}/pages/{pageId}/attachments.
//
// @Summary      Upload an attachment to a workspace knowledge page
// @Description  Uploads a file as an attachment on the given page. Requires
// @Description  the bearer token to carry the `pages:write` scope and the
// @Description  authenticated user to have `page.edit` on the page (Editor
// @Description  role on the workspace by default). The response includes
// @Description  the attachment id which can be embedded in markdown as
// @Description  `![alt](/api/attachments/{id}/download)`.
// @Tags         pages
// @Accept       multipart/form-data
// @Produce      application/json
// @Security     BearerAuth
// @Param        id      path      int    true  "Workspace ID"
// @Param        pageId  path      int    true  "Page ID"
// @Param        file    formData  file   true  "File to upload"
// @Success      200     {object}  models.AttachmentUploadResponse
// @Failure      400     {object}  handlers.ErrorResponse
// @Failure      404     {object}  handlers.ErrorResponse  "page not found or you lack page.edit"
// @Router       /workspaces/{id}/pages/{pageId}/attachments [post]
func (h *PageAttachmentUploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	pageID, ok := h.ParsePathID(w, r, "pageId", "page ID")
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil { // #nosec G120 -- MaxBytesReader caps the body; this is the memory threshold.
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Failed to parse form data: "+err.Error()))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Failed to get file from form: "+err.Error()))
		return
	}
	defer func() { _ = file.Close() }()

	fileData, err := io.ReadAll(file)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	response, err := h.uploads.UploadPageAttachment(r.Context(), services.PageAttachmentUploadInput{
		PageID:           pageID,
		UploaderID:       user.ID,
		OriginalFilename: fileHeader.Filename,
		FileData:         fileData,
		FileSize:         int64(len(fileData)),
	})
	if err != nil {
		h.respondUploadError(w, r, err)
		return
	}

	h.RespondOK(w, response)
}

func (h *PageAttachmentUploadHandler) respondUploadError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrPageAttachmentUploadDisabled):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusServiceUnavailable, restapi.ErrCodeServiceUnavailable, "Attachments are not enabled on this server"))
	case errors.Is(err, services.ErrPageAttachmentUploadInvalid):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, trimWrappedUploadError(err)))
	case errors.Is(err, services.ErrPageAttachmentUploadNotFound):
		h.RespondNotFound(w, r)
	default:
		h.RespondInternalError(w, r)
	}
}

func trimWrappedUploadError(err error) string {
	prefix := services.ErrPageAttachmentUploadInvalid.Error() + ": "
	msg := err.Error()
	if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
		return msg[len(prefix):]
	}
	return fmt.Sprintf("%v", err)
}
