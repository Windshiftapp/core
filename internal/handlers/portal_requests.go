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
	slug := r.PathValue("slug")
	itemIDStr := r.PathValue("itemId")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "itemId")
		return
	}

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

	// Use service to verify ownership
	isOwner, err := h.portalService.VerifyRequestOwnership(ctx, itemID, channel.ID, internalUserID, portalCustomerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !isOwner {
		respondNotFound(w, r, "item")
		return
	}

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
	slug := r.PathValue("slug")
	itemIDStr := r.PathValue("itemId")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "itemId")
		return
	}

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

	// Use service to verify ownership
	isOwner, err := h.portalService.VerifyRequestOwnership(ctx, itemID, channel.ID, internalUserID, portalCustomerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !isOwner {
		respondNotFound(w, r, "item")
		return
	}

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
	slug := r.PathValue("slug")
	itemIDStr := r.PathValue("itemId")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "itemId")
		return
	}

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

	// Use service to verify ownership
	isOwner, err := h.portalService.VerifyRequestOwnership(ctx, itemID, channel.ID, internalUserID, portalCustomerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !isOwner {
		respondNotFound(w, r, "item")
		return
	}

	// Parse comment content
	var commentData struct {
		Content string `json:"content"`
	}
	if err = json.NewDecoder(r.Body).Decode(&commentData); err != nil {
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
