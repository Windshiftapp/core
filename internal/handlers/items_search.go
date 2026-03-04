package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// Search filter limits to prevent abuse
const (
	maxSearchQueryLength = 500 // Maximum characters for search query
	maxWorkspaceFilters  = 50  // Maximum number of workspace IDs in filter
	maxStatusFilters     = 20  // Maximum number of statuses in filter
	maxPriorityFilters   = 10  // Maximum number of priorities in filter
)

// Search items across workspaces with advanced filtering
func (h *ItemHandler) Search(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get user from context
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get accessible workspace IDs (includes active workspaces and inactive ones where user has admin access)
	accessibleWorkspaceIDs, err := h.getAccessibleWorkspaceIDs(user)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// If user has no accessible workspaces, return empty list
	if len(accessibleWorkspaceIDs) == 0 {
		respondJSONOK(w, []models.Item{})
		return
	}

	// Get search parameters with input validation
	textQuery := r.URL.Query().Get("q")
	workspaceIDs := r.URL.Query()["workspace_id"] // Allow multiple workspace IDs
	statuses := r.URL.Query()["status"]           // Allow multiple statuses
	priorities := r.URL.Query()["priority"]       // Allow multiple priorities

	// Validate and sanitize inputs
	if len(textQuery) > maxSearchQueryLength {
		respondValidationError(w, r, fmt.Sprintf("Search query too long (max %d characters)", maxSearchQueryLength))
		return
	}

	// Validate workspace IDs are numeric
	for _, workspaceID := range workspaceIDs {
		if workspaceID == "" {
			continue
		}
		if _, err = strconv.Atoi(workspaceID); err != nil {
			respondValidationError(w, r, "Invalid workspace ID format")
			return
		}
	}

	// Limit array sizes to prevent abuse
	if len(workspaceIDs) > maxWorkspaceFilters {
		respondValidationError(w, r, fmt.Sprintf("Too many workspace filters (max %d)", maxWorkspaceFilters))
		return
	}
	if len(statuses) > maxStatusFilters {
		respondValidationError(w, r, fmt.Sprintf("Too many status filters (max %d)", maxStatusFilters))
		return
	}
	if len(priorities) > maxPriorityFilters {
		respondValidationError(w, r, fmt.Sprintf("Too many priority filters (max %d)", maxPriorityFilters))
		return
	}

	// Intersect requested workspace IDs with accessible ones
	finalWorkspaceIDs := accessibleWorkspaceIDs
	if len(workspaceIDs) > 0 {
		requestedIDs := make(map[int]bool)
		for _, wsID := range workspaceIDs {
			if wsID != "" {
				var id int
				if id, err = strconv.Atoi(wsID); err == nil {
					requestedIDs[id] = true
				}
			}
		}
		finalWorkspaceIDs = []int{}
		for _, id := range accessibleWorkspaceIDs {
			if requestedIDs[id] {
				finalWorkspaceIDs = append(finalWorkspaceIDs, id)
			}
		}
	}

	// Parse status IDs (numeric)
	var statusIDs []int
	for _, s := range statuses {
		if s != "" {
			if id, err := strconv.Atoi(s); err == nil {
				statusIDs = append(statusIDs, id)
			}
		}
	}

	// Parse priority IDs (numeric)
	var priorityIDs []int
	for _, p := range priorities {
		if p != "" {
			if id, err := strconv.Atoi(p); err == nil {
				priorityIDs = append(priorityIDs, id)
			}
		}
	}

	// Parse limit
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			respondValidationError(w, r, "Invalid limit format")
			return
		}
		switch {
		case parsedLimit < 1:
			limit = 1
		case parsedLimit > 1000:
			limit = 1000
		default:
			limit = parsedLimit
		}
	}

	// Call service
	items, _, err := h.itemCRUD.SearchWithFilters(services.SearchParams{
		TextQuery:    textQuery,
		WorkspaceIDs: finalWorkspaceIDs,
		StatusIDs:    statusIDs,
		PriorityIDs:  priorityIDs,
		Pagination: services.PaginationParams{
			Limit: limit,
		},
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Filter items based on user permissions
	filteredItems, err := h.filterItemsByPermissions(user.ID, items)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, filteredItems)
}

// UpdateFracIndex updates the frac_index of an item for fractional indexing ordering
func (h *ItemHandler) UpdateFracIndex(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Parse the request body
	var fracIndexRequest struct {
		// Item ID-based ranking (uses prev/next item IDs to calculate frac_index)
		PrevItemID *int `json:"prev_item_id"` // ID of item before in current view
		NextItemID *int `json:"next_item_id"` // ID of item after in current view
	}

	if err := json.NewDecoder(r.Body).Decode(&fracIndexRequest); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Require authentication
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get workspace_id for permission check
	var workspaceID int
	err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", id).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check if user has permission to edit items in this workspace
	canEdit, permErr := h.canEditItem(user.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canEdit {
		respondNotFound(w, r, "Item")
		return
	}

	// Generate the new frac_index
	var prevFracIndex, nextFracIndex string

	// Look up frac_index values from item IDs directly (trust the caller's prev/next selection)
	if fracIndexRequest.PrevItemID != nil {
		var prevItemFracIndex sql.NullString
		err = h.db.QueryRow("SELECT frac_index FROM items WHERE id = ?", *fracIndexRequest.PrevItemID).Scan(&prevItemFracIndex)
		if err == nil && prevItemFracIndex.Valid {
			prevFracIndex = prevItemFracIndex.String
		}
	}

	if fracIndexRequest.NextItemID != nil {
		var nextItemFracIndex sql.NullString
		err = h.db.QueryRow("SELECT frac_index FROM items WHERE id = ?", *fracIndexRequest.NextItemID).Scan(&nextItemFracIndex)
		if err == nil && nextItemFracIndex.Valid {
			nextFracIndex = nextItemFracIndex.String
		}
	}

	// Defensive check: if prev and next have the same frac_index, skip update
	if prevFracIndex != "" && nextFracIndex != "" && prevFracIndex == nextFracIndex {
		// Return the item as-is without error
		h.Get(w, r)
		return
	}

	// Get the current item's frac_index to check if update is needed
	var currentFracIndex sql.NullString
	err = h.db.QueryRow("SELECT frac_index FROM items WHERE id = ?", id).Scan(&currentFracIndex)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check if the item is already in the correct position
	if currentFracIndex.Valid {
		current := currentFracIndex.String
		// Item is already correctly positioned if:
		// - It's after prev (or prev is empty)
		// - It's before next (or next is empty)
		isAfterPrev := prevFracIndex == "" || current > prevFracIndex
		isBeforeNext := nextFracIndex == "" || current < nextFracIndex

		if isAfterPrev && isBeforeNext {
			// Return the item as-is without error
			h.Get(w, r)
			return
		}
	}

	newFracIndex, err := services.KeyBetween(prevFracIndex, nextFracIndex)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update the item's frac_index
	err = services.UpdateItemFracIndex(h.db.GetDB(), id, newFracIndex)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated item
	h.Get(w, r)
}

// GetBacklogItems returns items whose statuses are not marked as completed for a workspace
func (h *ItemHandler) GetBacklogItems(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	workspaceIDParam := r.URL.Query().Get("workspace_id")
	collectionIDParam := r.URL.Query().Get("collection_id")

	// workspace_id is required when no collection_id is provided
	if workspaceIDParam == "" && collectionIDParam == "" {
		respondValidationError(w, r, "workspace_id parameter is required")
		return
	}

	var wsID int
	if workspaceIDParam != "" {
		var err error
		wsID, err = strconv.Atoi(workspaceIDParam)
		if err != nil {
			respondValidationError(w, r, "Invalid workspace_id format")
			return
		}
	}

	var collectionID int
	if collectionIDParam != "" {
		var err error
		collectionID, err = strconv.Atoi(collectionIDParam)
		if err != nil {
			respondValidationError(w, r, "Invalid collection_id parameter")
			return
		}
	}

	// Get accessible workspace IDs
	accessibleWorkspaceIDs, err := h.getAccessibleWorkspaceIDs(user)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if len(accessibleWorkspaceIDs) == 0 {
		respondJSONOK(w, []models.Item{})
		return
	}

	qlQuery := r.URL.Query().Get("ql")

	// Parse pagination parameters
	page := 1
	limit := 50
	maxLimit := 1000

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, parseErr := strconv.Atoi(pageStr); parseErr == nil && p > 0 {
			page = p
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, parseErr := strconv.Atoi(limitStr); parseErr == nil && l > 0 {
			limit = l
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}

	offset := (page - 1) * limit

	// Call service
	items, totalCount, err := h.itemCRUD.GetBacklogItems(services.BacklogParams{
		WorkspaceID:  wsID,
		CollectionID: collectionID,
		QLQuery:      qlQuery,
		WorkspaceIDs: accessibleWorkspaceIDs,
		Pagination:   services.PaginationParams{Limit: limit, Offset: offset, Page: page},
	})
	if err != nil {
		if strings.Contains(err.Error(), "QL query error:") {
			respondValidationError(w, r, err.Error())
			return
		}
		if strings.Contains(err.Error(), "collection not found") {
			respondNotFound(w, r, "collection")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Filter items based on user permissions
	filteredItems, err := h.filterItemsByPermissions(user.ID, items)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	totalPages := 0
	if limit > 0 {
		totalPages = (totalCount + limit - 1) / limit
	}

	response := models.PaginatedItemsResponse{
		Items: filteredItems,
		Pagination: models.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      totalCount,
			TotalPages: totalPages,
		},
	}
	respondJSONOK(w, response)
}
