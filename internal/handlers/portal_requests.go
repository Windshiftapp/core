package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"windshift/internal/markdown"
	"windshift/internal/models"
	"windshift/internal/validation"
)

// resolvePortalRequest authorizes a request owner or active approver.
// Approver access permits reading and commenting only and ends with the pending
// approval step. On success callers must defer cancel.
func (h *PortalHandler) resolvePortalRequest(w http.ResponseWriter, r *http.Request) (itemID int, config models.ChannelConfig, internalUserID *int, portalCustomerID *int, ctx context.Context, cancel context.CancelFunc, ok bool) { //nolint:gocritic // multiple results needed for this complex guard
	itemID, itemOK := requireIDParam(w, r, "itemId")
	if !itemOK {
		return 0, models.ChannelConfig{}, nil, nil, nil, nil, false
	}

	ctx, cancel, channel, config, portalOK := h.resolvePortalBySlug(w, r)
	if !portalOK {
		return 0, models.ChannelConfig{}, nil, nil, nil, nil, false
	}

	// Get auth info from context (middleware already validated)
	internalUserID, portalCustomerID = h.getAuthFromContext(r)

	// Owner branch.
	isOwner, err := h.portalService.VerifyRequestOwnership(ctx, itemID, channel.ID, internalUserID, portalCustomerID)
	if err != nil {
		cancel()
		respondInternalError(w, r, err)
		return 0, models.ChannelConfig{}, nil, nil, nil, nil, false
	}
	if isOwner {
		return itemID, config, internalUserID, portalCustomerID, ctx, cancel, true
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
			return 0, models.ChannelConfig{}, nil, nil, nil, nil, false
		}
		if isApprover {
			return itemID, config, internalUserID, portalCustomerID, ctx, cancel, true
		}
	}

	cancel()
	respondNotFound(w, r, "item")
	return 0, models.ChannelConfig{}, nil, nil, nil, nil, false
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
	itemID, config, _, _, ctx, cancel, ok := h.resolvePortalRequest(w, r)
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
	itemID, config, _, _, ctx, cancel, ok := h.resolvePortalRequest(w, r)
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
	itemID, _, internalUserID, portalCustomerID, ctx, cancel, ok := h.resolvePortalRequest(w, r)
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
