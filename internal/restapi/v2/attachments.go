package v2

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"windshift/internal/fileserve"
	"windshift/internal/models"
	"windshift/internal/services"
)

type attachmentUploadForm struct {
	File []byte `json:"file"`
}

func registerAttachmentRoutes(builder *routeBuilder, deps Deps) {
	attachments := deps.Attachments
	builder.Page("/items/{item_id}/attachments", AuthAuthenticated, []string{"items:read"}, listItemAttachments(attachments))
	builder.RawDocument[attachmentUploadForm, models.Attachment](http.MethodPost, "/items/{item_id}/attachments", http.StatusCreated, "multipart/form-data", AuthAuthenticated, []string{"items:write"}, uploadItemAttachment(attachments))
	builder.RawDocument[attachmentUploadForm, models.Attachment](http.MethodPost, "/workspaces/{workspace_id}/pages/{page_id}/attachments", http.StatusCreated, "multipart/form-data", AuthAuthenticated, []string{"pages:write"}, uploadPageAttachment(deps))
	builder.Read("/attachments/{attachment_id}", AuthAuthenticated, []string{"items:read"}, getItemAttachment(attachments))
	builder.RawResponse[[]byte](http.MethodGet, "/attachments/{attachment_id}/content", http.StatusOK, "application/octet-stream", AuthAuthenticated, []string{"items:read"}, serveItemAttachment(attachments, false))
	builder.RawResponse[[]byte](http.MethodGet, "/attachments/{attachment_id}/thumbnail", http.StatusOK, "application/octet-stream", AuthAuthenticated, []string{"items:read"}, serveItemAttachment(attachments, true))
	builder.Command(http.MethodDelete, "/attachments/{attachment_id}", AuthAuthenticated, []string{"items:write"}, deleteItemAttachment(attachments))
}

func uploadPageAttachment(deps Deps) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		user, _, pageID, err := requirePage(r, deps, services.PageOpEdit)
		if err != nil {
			return err
		}
		const maxBody = int64(32 << 20)
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		// The request body is bounded above; ParseMultipartForm's argument only controls memory use.
		//nolint:gosec // G120 does not recognize the MaxBytesReader assignment.
		if err := r.ParseMultipartForm(maxBody); err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "Invalid multipart upload")
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "file is required")
		}
		defer func() { _ = file.Close() }()
		data, err := io.ReadAll(file)
		if err != nil {
			return internalError(err)
		}
		result, err := deps.PageAttachments.UploadPageAttachment(services.PageAttachmentUploadInput{
			PageID: pageID, UploaderID: user.ID, OriginalFilename: header.Filename,
			FileData: data, FileSize: int64(len(data)),
		})
		if err != nil {
			return pageAttachmentError(err)
		}
		return writeDocument(w, http.StatusCreated, result.Attachment)
	}
}

func listItemAttachments(attachments attachmentApplication) pageOperation[models.Attachment] {
	return func(r *http.Request) ([]models.Attachment, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		itemID, err := pathID(r, "item_id")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		result, total, err := attachments.ListItemAttachments(user.ID, itemID, page.PageSize, page.Offset)
		return result, page, total, attachmentError(err)
	}
}

func getItemAttachment(attachments attachmentApplication) readOperation[models.Attachment] {
	return func(r *http.Request) (models.Attachment, error) {
		user, err := principal(r)
		if err != nil {
			return models.Attachment{}, err
		}
		attachmentID, err := pathID(r, "attachment_id")
		if err != nil {
			return models.Attachment{}, err
		}
		attachment, err := attachments.GetItemAttachment(user.ID, attachmentID)
		if attachment == nil {
			return models.Attachment{}, attachmentError(err)
		}
		return *attachment, attachmentError(err)
	}
}

func uploadItemAttachment(attachments attachmentApplication) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		itemID, err := pathID(r, "item_id")
		if err != nil {
			return err
		}
		policy, err := attachments.UploadPolicy()
		if err != nil {
			return internalError(err)
		}
		if !policy.Enabled || policy.MaxFileSize <= 0 {
			return attachmentError(services.ErrItemAttachmentDisabled)
		}
		maxBody := policy.MaxFileSize + (1 << 20)
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		// The request body is bounded above; ParseMultipartForm's argument only controls memory use.
		//nolint:gosec // G120 does not recognize the MaxBytesReader assignment.
		if err := r.ParseMultipartForm(maxBody); err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "Invalid multipart upload")
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "file is required")
		}
		defer func() { _ = file.Close() }()
		data, err := io.ReadAll(io.LimitReader(file, policy.MaxFileSize+1))
		if err != nil {
			return internalError(err)
		}
		if int64(len(data)) > policy.MaxFileSize {
			return newError(http.StatusRequestEntityTooLarge, "payload_too_large", "Attachment exceeds the configured size limit")
		}
		result, err := attachments.UploadItemAttachment(services.ItemAttachmentUploadInput{ItemID: itemID, UploaderID: user.ID, OriginalFilename: header.Filename, FileData: data, FileSize: int64(len(data))})
		if err != nil {
			return attachmentError(err)
		}
		return writeDocument(w, http.StatusCreated, result.Attachment)
	}
}

func serveItemAttachment(attachments attachmentApplication, thumbnail bool) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		attachmentID, err := pathID(r, "attachment_id")
		if err != nil {
			return err
		}
		binary, err := attachments.OpenItemAttachment(user.ID, attachmentID, thumbnail)
		if err != nil {
			return attachmentError(err)
		}
		defer func() { _ = binary.File.Close() }()
		info, err := binary.File.Stat()
		if err != nil {
			return internalError(err)
		}
		w.Header().Set("Content-Type", binary.MimeType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("ETag", fmt.Sprintf("\"attachment-%d-%d\"", attachmentID, binary.FileSize))
		if thumbnail {
			w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		} else {
			w.Header().Set("Content-Disposition", fileserve.ContentDisposition("attachment", binary.OriginalFilename))
		}
		http.ServeContent(w, r, binary.OriginalFilename, info.ModTime(), binary.File)
		return nil
	}
}

func deleteItemAttachment(attachments attachmentApplication) commandOperation {
	return func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		attachmentID, err := pathID(r, "attachment_id")
		if err != nil {
			return err
		}
		return attachmentError(attachments.DeleteItemAttachment(attachmentID, user.ID))
	}
}

func attachmentError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrItemAttachmentNotFound):
		return newError(http.StatusNotFound, "not_found", "Attachment was not found")
	case errors.Is(err, services.ErrItemAttachmentDisabled):
		return newError(http.StatusServiceUnavailable, "service_unavailable", "Attachments are not enabled")
	case errors.Is(err, services.ErrItemAttachmentInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}

func pageAttachmentError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrPageAttachmentUploadNotFound):
		return newError(http.StatusNotFound, "not_found", "Page was not found")
	case errors.Is(err, services.ErrPageAttachmentUploadDisabled):
		return newError(http.StatusServiceUnavailable, "service_unavailable", "Attachments are not enabled")
	case errors.Is(err, services.ErrPageAttachmentUploadInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
