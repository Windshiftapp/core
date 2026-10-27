package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/fileserve"
	"windshift/internal/markdown"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/validation"
)

// resolvePortalRequest authorizes a request owner or active approver.
// Approver access permits reading and commenting only and ends with the pending
// approval step; isOwner distinguishes the two so write endpoints can refuse
// approvers. On success callers must defer cancel.
func (h *PortalHandler) resolvePortalRequest(w http.ResponseWriter, r *http.Request) (itemID int, config models.ChannelConfig, internalUserID *int, portalCustomerID *int, isOwner bool, ctx context.Context, cancel context.CancelFunc, ok bool) { //nolint:gocritic // multiple results needed for this complex guard
	itemID, itemOK := requireIDParam(w, r, "itemId")
	if !itemOK {
		return 0, models.ChannelConfig{}, nil, nil, false, nil, nil, false
	}

	ctx, cancel, channel, config, portalOK := h.resolvePortalBySlug(w, r)
	if !portalOK {
		return 0, models.ChannelConfig{}, nil, nil, false, nil, nil, false
	}

	// Get auth info from context (middleware already validated)
	internalUserID, portalCustomerID = h.getAuthFromContext(r)

	// Owner branch.
	isOwner, err := h.portalService.VerifyRequestOwnership(ctx, itemID, channel.ID, internalUserID, portalCustomerID)
	if err != nil {
		cancel()
		respondInternalError(w, r, err)
		return 0, models.ChannelConfig{}, nil, nil, false, nil, nil, false
	}
	if isOwner {
		return itemID, config, internalUserID, portalCustomerID, true, ctx, cancel, true
	}

	// Active-approver branch. Only consulted when ownership failed; approvers
	// who are also creators have already returned via the owner branch.
	// channel.ID is passed in so approver-derived access does not leak across
	// portal channels (an approver on item X in channel A must not be able to
	// read X via channel B's portal slug).
	if h.approvalService != nil {
		isApprover, aerr := h.callerIsActiveApproverOnItem(ctx, itemID, channel.ID, internalUserID, portalCustomerID)
		if aerr != nil {
			cancel()
			respondInternalError(w, r, aerr)
			return 0, models.ChannelConfig{}, nil, nil, false, nil, nil, false
		}
		if isApprover {
			// Approver access must not outlive the portal's served-workspace
			// configuration. Without this the fallback grants access to an item
			// whose workspace was removed from the portal.
			served, serr := h.portalService.PortalServesRequestWorkspace(ctx, itemID, channel.ID)
			if serr != nil {
				cancel()
				respondInternalError(w, r, serr)
				return 0, models.ChannelConfig{}, nil, nil, false, nil, nil, false
			}
			if served {
				return itemID, config, internalUserID, portalCustomerID, false, ctx, cancel, true
			}
		}
	}

	cancel()
	respondNotFound(w, r, "item")
	return 0, models.ChannelConfig{}, nil, nil, false, nil, nil, false
}

// callerIsActiveApproverOnItem checks the approver pool for whichever auth
// principal is set (internal user or portal customer). Returns false if both
// are nil, which preserves the 404 path. The lookup is scoped to channelID so
// portal flows never grant cross-channel approver-derived access.
func (h *PortalHandler) callerIsActiveApproverOnItem(ctx context.Context, itemID, channelID int, internalUserID, portalCustomerID *int) (bool, error) {
	if h.approvalService == nil {
		return false, nil
	}
	if internalUserID != nil {
		ok, err := h.approvalService.UserHasActivePoolMembershipOnItem(ctx, *internalUserID, itemID, &channelID)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	if portalCustomerID != nil {
		ok, err := h.approvalService.PortalCustomerHasActivePoolMembershipOnItem(ctx, *portalCustomerID, itemID, &channelID)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// GetMyRequests returns all requests submitted by the authenticated portal customer through this portal
func (h *PortalHandler) GetMyRequests(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, _, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return
	}
	defer cancel()

	requests, err := h.loadMyPortalRequests(ctx, r, channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, requests)
}

// GetRequestDetail returns detailed information about a specific request
func (h *PortalHandler) GetRequestDetail(w http.ResponseWriter, r *http.Request) {
	itemID, config, _, _, _, ctx, cancel, ok := h.resolvePortalRequest(w, r)
	if !ok {
		return
	}
	defer cancel()

	// Get the request details
	detail, err := h.portalService.GetRequestDetail(ctx, itemID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if detail == nil {
		respondNotFound(w, r, "item")
		return
	}

	// Page links in the description resolve to in-portal KB articles when the
	// page is published through this portal; otherwise they lose the anchor so
	// customers never see a dead or existence-leaking link. Both the raw
	// markdown (rendered client-side through the shared read-only editor) and
	// the sanitized HTML form are rewritten.
	resolver := h.portalKBPageLinkResolver(config)
	detail.Description = markdown.RewritePageLinksInMarkdown(detail.Description, resolver)
	detail.DescriptionHTML = markdown.RewritePageLinks(detail.DescriptionHTML, resolver)

	respondJSONOK(w, detail)
}

// GetRequestComments returns comments for a specific request
func (h *PortalHandler) GetRequestComments(w http.ResponseWriter, r *http.Request) {
	itemID, config, _, _, _, ctx, cancel, ok := h.resolvePortalRequest(w, r)
	if !ok {
		return
	}
	defer cancel()

	// Use service to get comments
	comments, err := h.portalService.GetRequestComments(ctx, itemID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Rewrite agent-inserted page links the same way as the description:
	// published-through-this-portal pages become in-portal article links,
	// anything else degrades to plain text. Both the raw markdown (rendered
	// client-side through the shared read-only editor) and the sanitized HTML
	// form are rewritten.
	resolver := h.portalKBPageLinkResolver(config)
	for i := range comments {
		comments[i].Content = markdown.RewritePageLinksInMarkdown(comments[i].Content, resolver)
		comments[i].ContentHTML = markdown.RewritePageLinks(comments[i].ContentHTML, resolver)
	}

	respondJSONOK(w, comments)
}

// AddRequestComment adds a comment to a request from a portal customer or internal user
func (h *PortalHandler) AddRequestComment(w http.ResponseWriter, r *http.Request) {
	itemID, _, internalUserID, portalCustomerID, _, ctx, cancel, ok := h.resolvePortalRequest(w, r)
	if !ok {
		return
	}
	defer cancel()

	// Parse comment content
	var commentData struct {
		Content string `json:"content"`
	}
	if !decodeChannelRequest(w, r, &commentData, false) {
		return
	}

	if strings.TrimSpace(commentData.Content) == "" {
		respondValidationError(w, r, "Comment content is required")
		return
	}

	comment, err := h.portalService.CreateRequestComment(ctx, itemID, commentData.Content, internalUserID, portalCustomerID)
	if err != nil {
		var validationErr *validation.ValidationError
		if errors.As(err, &validationErr) {
			respondValidationError(w, r, validationErr.Message)
			return
		}
		respondInternalError(w, r, err)
		return
	}
	contentHTML, err := markdown.Render(comment.Content)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("render portal comment: %w", err))
		return
	}

	// Return the created comment
	response := map[string]any{
		"id":            comment.ID,
		"item_id":       comment.ItemID,
		"content":       comment.Content,
		"content_html":  contentHTML,
		"created_at":    comment.CreatedAt,
		"updated_at":    comment.UpdatedAt,
		"author_name":   comment.AuthorName,
		"author_avatar": comment.AuthorAvatar,
	}
	if comment.AuthorID != nil {
		response["author_id"] = *comment.AuthorID
	}
	if comment.PortalCustomerID != nil {
		response["portal_customer_id"] = *comment.PortalCustomerID
	}

	respondJSONCreated(w, response)
}

// GetRequestAttachments lists the files attached to a request. Owners and
// active approvers may list; ownership was resolved by resolvePortalRequest.
func (h *PortalHandler) GetRequestAttachments(w http.ResponseWriter, r *http.Request) {
	itemID, _, _, _, _, _, cancel, ok := h.resolvePortalRequest(w, r)
	if !ok {
		return
	}
	defer cancel()

	attachments, _, err := h.requestAttachmentService().ListPortalRequestAttachments(itemID, 200, 0)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, attachments)
}

// AddRequestAttachment stores a file uploaded by the request owner. Approvers
// have read-only portal access, so uploads are owner-only. The upload shares
// the submission rate limiter and the same validation, storage, and limits as
// the authenticated item attachment surface. Portal-customer uploads are
// attributed to the customer end to end (attachment record + item history);
// internal-owner uploads keep user attribution.
func (h *PortalHandler) AddRequestAttachment(w http.ResponseWriter, r *http.Request) {
	itemID, _, internalUserID, portalCustomerID, isOwner, _, cancel, ok := h.resolvePortalRequest(w, r)
	if !ok {
		return
	}
	defer cancel()

	if !isOwner {
		respondNotFound(w, r, "item")
		return
	}

	svc := h.requestAttachmentService()
	if svc == nil {
		respondServiceUnavailable(w, r, "Attachments are not enabled on this server")
		return
	}

	// Cap the request before parsing, mirroring the internal upload endpoint.
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	// #nosec G120 -- the body is already capped by MaxBytesReader above; the int arg is the in-memory threshold, not the upper bound
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if isRequestBodyTooLarge(err) {
			respondRequestTooLarge(w, r)
			return
		}
		respondBadRequest(w, r, "Failed to parse form data")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, r, "Failed to get file from form")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("read portal attachment: %w", err))
		return
	}

	var uploaderID int
	if internalUserID != nil {
		uploaderID = *internalUserID
	}
	input := services.ItemAttachmentUploadInput{
		ItemID:           itemID,
		UploaderID:       uploaderID,
		OriginalFilename: header.Filename,
		FileData:         data,
		FileSize:         int64(len(data)),
	}
	if portalCustomerID != nil {
		input.UploaderPortalCustomerID = portalCustomerID
	}
	response, err := svc.UploadPublicFormAttachment(r.Context(), input)
	if err != nil {
		h.respondPortalUploadError(w, r, err)
		return
	}

	respondJSONCreated(w, response)
}

// DownloadRequestAttachment streams one request attachment to its owner or an
// active approver. The service refuses attachments bound to any other item or
// entity type, so attachment IDs cannot be replayed across requests.
func (h *PortalHandler) DownloadRequestAttachment(w http.ResponseWriter, r *http.Request) {
	itemID, _, _, _, _, _, cancel, ok := h.resolvePortalRequest(w, r)
	if !ok {
		return
	}
	defer cancel()

	attachmentID, idOK := requireIDParam(w, r, "attachmentId")
	if !idOK {
		return
	}

	binary, err := h.requestAttachmentService().OpenPortalRequestAttachment(r.Context(), attachmentID, itemID)
	if err != nil {
		if errors.Is(err, services.ErrItemAttachmentDisabled) {
			respondServiceUnavailable(w, r, "Attachments are not enabled on this server")
			return
		}
		respondNotFound(w, r, "attachment")
		return
	}
	defer func() { _ = binary.File.Close() }()

	w.Header().Set("Content-Type", binary.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(binary.File.Size(), 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	if fileserve.IsInlineSafeMimeType(binary.MimeType) {
		w.Header().Set("Content-Disposition", fileserve.ContentDisposition("inline", binary.OriginalFilename))
	} else {
		w.Header().Set("Content-Disposition", fileserve.ContentDisposition("attachment", binary.OriginalFilename))
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	}
	if _, err := io.Copy(w, binary.File); err != nil {
		slog.Error("failed to serve portal attachment", slog.String("component", "portal"), slog.Int("item_id", itemID), slog.Int("attachment_id", attachmentID), slog.Any("error", err))
	}
}

// requestAttachmentService returns the portal attachment service, or nil when
// attachment storage is disabled on this server.
func (h *PortalHandler) requestAttachmentService() *services.ItemAttachmentService {
	return h.attachments
}

// respondPortalUploadError maps item attachment upload failures to portal
// responses; denials collapse to 404 so ownership is never disclosed.
func (h *PortalHandler) respondPortalUploadError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, services.ErrItemAttachmentDisabled) {
		respondServiceUnavailable(w, r, "Attachments are not enabled on this server")
		return
	}
	if errors.Is(err, services.ErrItemAttachmentNotFound) {
		respondNotFound(w, r, "item")
		return
	}
	if errors.Is(err, services.ErrItemAttachmentInvalid) {
		respondValidationError(w, r, strings.TrimPrefix(err.Error(), services.ErrItemAttachmentInvalid.Error()+": "))
		return
	}
	respondInternalError(w, r, err)
}
