package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"windshift/internal/restapi"
	"windshift/internal/services"
)

// ItemAttachmentHandler exposes item-attachment upload and delete to bearer-
// authenticated callers (the ws CLI, MCP, third-party agents). It is a REST
// v1 adapter over ItemAttachmentService; the polymorphic cookie-auth
// AttachmentHandler.Upload/Delete routes reject crw_ tokens.
//
// Routes:
//   - POST   /rest/api/v1/items/{id}/attachments
//   - DELETE /rest/api/v1/attachments/{id}
//
// Both are gated by the items:write token scope at the route layer and by
// item.edit permission in the owning workspace inside the service. Read
// (list/download/thumbnail) stays on the existing v1 AttachmentHandler +
// ItemHandler.GetAttachments routes.
type ItemAttachmentHandler struct {
	BaseHandler
	uploads *services.ItemAttachmentService
}

// NewItemAttachmentHandler constructs the v1 item-attachment handler.
func NewItemAttachmentHandler(base BaseHandler, uploads *services.ItemAttachmentService) *ItemAttachmentHandler {
	return &ItemAttachmentHandler{BaseHandler: base, uploads: uploads}
}

// Upload handles POST /rest/api/v1/items/{id}/attachments.
//
// @Summary      Upload an attachment to a work item
// @Description  Uploads a file as an attachment on the given item. Requires
// @Description  the bearer token to carry the `items:write` scope and the
// @Description  authenticated user to hold `item.edit` in the item's
// @Description  workspace (Editor role by default). The response includes
// @Description  the attachment id which downloads via
// @Description  `/rest/api/v1/attachments/{id}/download`.
// @Tags         items
// @Accept       multipart/form-data
// @Produce      application/json
// @Security     BearerAuth
// @Param        id      path      int    true  "Item ID"
// @Param        file    formData  file   true  "File to upload"
// @Success      200     {object}  models.AttachmentUploadResponse
// @Failure      400     {object}  handlers.ErrorResponse
// @Failure      404     {object}  handlers.ErrorResponse  "item not found or you lack item.edit"
// @Router       /items/{id}/attachments [post]
func (h *ItemAttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	itemID, ok := h.ParsePathID(w, r, "id", "item ID")
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

	response, err := h.uploads.UploadItemAttachment(r.Context(), services.ItemAttachmentUploadInput{
		ItemID:           itemID,
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

// Delete handles DELETE /rest/api/v1/attachments/{id}.
//
// @Summary      Delete an attachment
// @Description  Deletes an item-scoped attachment record and its stored blob.
// @Description  Requires the `items:write` scope and `item.edit` in the
// @Description  owning item's workspace. Non-item attachments (pages,
// @Description  avatars, branding) are not removable through this endpoint.
// @Tags         items
// @Security     BearerAuth
// @Param        id   path  int  true  "Attachment ID"
// @Success      204  "Attachment deleted"
// @Failure      404   {object}  handlers.ErrorResponse  "attachment not found or you lack item.edit"
// @Router       /attachments/{id} [delete]
func (h *ItemAttachmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	attachmentID, ok := h.ParsePathID(w, r, "id", "attachment ID")
	if !ok {
		return
	}

	if err := h.uploads.DeleteItemAttachment(r.Context(), attachmentID, user.ID); err != nil {
		h.respondDeleteError(w, r, err)
		return
	}
	h.RespondNoContent(w)
}

func (h *ItemAttachmentHandler) respondUploadError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrItemAttachmentDisabled):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusServiceUnavailable, restapi.ErrCodeServiceUnavailable, "Attachments are not enabled on this server"))
	case errors.Is(err, services.ErrItemAttachmentInvalid):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, trimItemAttachmentError(err)))
	case errors.Is(err, services.ErrItemAttachmentNotFound):
		h.RespondError(w, r, restapi.ErrItemNotFound)
	default:
		h.RespondInternalError(w, r)
	}
}

func (h *ItemAttachmentHandler) respondDeleteError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrItemAttachmentNotFound):
		h.RespondNotFound(w, r)
	default:
		h.RespondInternalError(w, r)
	}
}

// trimItemAttachmentError strips the wrapped sentinel prefix so the client
// sees the human-readable reason rather than the internal error sentinel.
func trimItemAttachmentError(err error) string {
	prefix := services.ErrItemAttachmentInvalid.Error() + ": "
	msg := err.Error()
	if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
		return msg[len(prefix):]
	}
	return fmt.Sprintf("%v", err)
}
