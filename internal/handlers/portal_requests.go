package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/services"
	"windshift/internal/utils"
)

// resolvePortalRequest is a shared preamble for handlers that operate on a
// specific portal request. It parses the item ID, resolves the channel from
// the slug, authenticates the caller, and authorizes access — accepting either
// of two roles:
//
//  1. Owner: the caller created the request (internal user or portal customer).
//  2. Active approver: the caller is in an is_active=true approver row of a
//     pending step on this request. This mirrors the documented exception in
//     /api/approvals/{id}/decide — an approver added explicitly to a step
//     must be able to read the request they're deciding on, even when they're
//     not the original requester.
//
// Approver-derived access is read-only at the data layer: it permits reading
// detail/comments and adding a comment on the request thread (which is what
// external reviewers expect when asking the requester for clarification),
// but mutating endpoints that create or transform the request itself (e.g.
// SubmitToPortal) must not use this helper.
//
// Active-pool membership is a snapshot: once the step closes (is_active=0) or
// the request is no longer pending, approver-derived access disappears.
//
// On failure writes the appropriate HTTP error response and returns ok=false.
// Callers must defer cancel() when ok is true.
func (h *PortalHandler) resolvePortalRequest(w http.ResponseWriter, r *http.Request) (itemID int, internalUserID *int, portalCustomerID *int, ctx context.Context, cancel context.CancelFunc, ok bool) { //nolint:gocritic // multiple results needed for this complex guard
	slug := r.PathValue("slug")
	itemIDStr := r.PathValue("itemId")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "itemId")
		return 0, nil, nil, nil, nil, false
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		cancel()
		respondNotFound(w, r, "portal")
		return 0, nil, nil, nil, nil, false
	}
	channel := portalResult.channel

	// Get auth info from context (middleware already validated)
	internalUserID, portalCustomerID = h.getAuthFromContext(r)

	// Owner branch.
	isOwner, err := h.portalService.VerifyRequestOwnership(ctx, itemID, channel.ID, internalUserID, portalCustomerID)
	if err != nil {
		cancel()
		respondInternalError(w, r, err)
		return 0, nil, nil, nil, nil, false
	}
	if isOwner {
		return itemID, internalUserID, portalCustomerID, ctx, cancel, true
	}

	// Active-approver branch. Only consulted when ownership failed; approvers
	// who are also creators have already returned via the owner branch.
	if h.approvalService != nil {
		isApprover, aerr := h.callerIsActiveApproverOnItem(itemID, internalUserID, portalCustomerID)
		if aerr != nil {
			cancel()
			respondInternalError(w, r, aerr)
			return 0, nil, nil, nil, nil, false
		}
		if isApprover {
			return itemID, internalUserID, portalCustomerID, ctx, cancel, true
		}
	}

	cancel()
	respondNotFound(w, r, "item")
	return 0, nil, nil, nil, nil, false
}

// callerIsActiveApproverOnItem checks the approver pool for whichever auth
// principal is set (internal user or portal customer). Returns false if both
// are nil, which preserves the 404 path.
func (h *PortalHandler) callerIsActiveApproverOnItem(itemID int, internalUserID, portalCustomerID *int) (bool, error) {
	if h.approvalService == nil {
		return false, nil
	}
	if internalUserID != nil {
		ok, err := h.approvalService.UserHasActivePoolMembershipOnItem(*internalUserID, itemID)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	if portalCustomerID != nil {
		ok, err := h.approvalService.PortalCustomerHasActivePoolMembershipOnItem(*portalCustomerID, itemID)
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
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	channel := portalResult.channel

	// Get auth info from context (middleware already validated)
	internalUserID, portalCustomerID := h.getAuthFromContext(r)

	// Use service to get requests based on auth type
	var requests []services.PortalRequestSummary
	if internalUserID != nil {
		requests, err = h.portalService.GetRequestsByCreatorID(ctx, *internalUserID, channel.ID)
	} else {
		requests, err = h.portalService.GetRequestsByPortalCustomerID(ctx, *portalCustomerID, channel.ID)
	}

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, requests)
}

// GetRequestDetail returns detailed information about a specific request
func (h *PortalHandler) GetRequestDetail(w http.ResponseWriter, r *http.Request) {
	itemID, _, _, ctx, cancel, ok := h.resolvePortalRequest(w, r)
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

	respondJSONOK(w, detail)
}

// GetRequestComments returns comments for a specific request
func (h *PortalHandler) GetRequestComments(w http.ResponseWriter, r *http.Request) {
	itemID, _, _, ctx, cancel, ok := h.resolvePortalRequest(w, r)
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

	respondJSONOK(w, comments)
}

// AddRequestComment adds a comment to a request from a portal customer or internal user
func (h *PortalHandler) AddRequestComment(w http.ResponseWriter, r *http.Request) {
	itemID, internalUserID, portalCustomerID, ctx, cancel, ok := h.resolvePortalRequest(w, r)
	if !ok {
		return
	}
	defer cancel()

	// Parse comment content
	var commentData struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&commentData); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if strings.TrimSpace(commentData.Content) == "" {
		respondValidationError(w, r, "Comment content is required")
		return
	}

	// Sanitize comment content to prevent XSS (strips HTML tags + dangerous Markdown URLs)
	sanitizedContent := utils.SanitizeCommentContent(commentData.Content)

	// Insert comment based on auth type
	var err error
	now := time.Now()
	var commentID int64
	var authorName, authorAvatar string
	var responseAuthorID *int
	var responsePortalCustomerID *int

	if internalUserID != nil {
		// Internal user: use author_id
		insertQuery := `
			INSERT INTO comments (item_id, author_id, content, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?) RETURNING id
		`
		err = h.db.QueryRowContext(ctx, insertQuery, itemID, *internalUserID, sanitizedContent, now, now).Scan(&commentID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Fetch the user's name for the response
		nameQuery := `SELECT COALESCE(first_name || ' ' || last_name, 'Unknown'), COALESCE(avatar_url, '') FROM users WHERE id = ?`
		if scanErr := h.db.QueryRowContext(ctx, nameQuery, *internalUserID).Scan(&authorName, &authorAvatar); scanErr != nil {
			authorName = "Unknown"
			authorAvatar = ""
		}
		responseAuthorID = internalUserID
	} else {
		// Portal customer: use portal_customer_id
		insertQuery := `
			INSERT INTO comments (item_id, portal_customer_id, content, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?) RETURNING id
		`
		err = h.db.QueryRowContext(ctx, insertQuery, itemID, *portalCustomerID, sanitizedContent, now, now).Scan(&commentID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Fetch the portal customer's name for the response
		nameQuery := `SELECT COALESCE(name, 'Unknown') FROM portal_customers WHERE id = ?`
		if scanErr := h.db.QueryRowContext(ctx, nameQuery, *portalCustomerID).Scan(&authorName); scanErr != nil {
			authorName = "Unknown"
		}
		authorAvatar = ""
		responsePortalCustomerID = portalCustomerID
	}

	// Return the created comment
	response := map[string]interface{}{
		"id":            commentID,
		"item_id":       itemID,
		"content":       sanitizedContent,
		"created_at":    now,
		"updated_at":    now,
		"author_name":   authorName,
		"author_avatar": authorAvatar,
	}
	if responseAuthorID != nil {
		response["author_id"] = *responseAuthorID
	}
	if responsePortalCustomerID != nil {
		response["portal_customer_id"] = *responsePortalCustomerID
	}

	respondJSONCreated(w, response)
}
