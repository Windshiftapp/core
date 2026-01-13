package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// GetChildren returns direct children of an item
func (h *ItemHandler) GetChildren(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get parent workspace for permission check
	repo := repository.NewItemRepository(h.db)
	parentWorkspaceID, err := repo.GetWorkspaceID(id)
	if err != nil {
		if err == repository.ErrNotFound {
			http.Error(w, "Parent item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch parent item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission
	canView, err := h.canViewItem(user.ID, parentWorkspaceID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Insufficient permissions to view this item's children", http.StatusForbidden)
		return
	}

	// Get children using repository
	children, err := repo.GetChildren(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to slice (non-pointer) for JSON encoding
	result := make([]models.Item, len(children))
	for i, child := range children {
		result[i] = *child
	}

	respondJSONOK(w, result)
}

// GetAncestors returns all ancestors of an item (for breadcrumbs)
func (h *ItemHandler) GetAncestors(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get child item to check permission
	var childWorkspaceID int
	err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", id).Scan(&childWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to view child item's workspace
	canView, permErr := h.canViewItem(user.ID, childWorkspaceID)
	if permErr != nil {
		http.Error(w, "Permission check failed: "+permErr.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Insufficient permissions to view this item's ancestors", http.StatusForbidden)
		return
	}

	ancestors, err := h.hierarchyService.GetAncestors(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Apply permission filtering to ancestors as well
	filteredAncestors, err := h.filterItemsByPermissions(user.ID, ancestors)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, filteredAncestors)
}

// GetDescendantsNew returns all descendants using the new hierarchy service
func (h *ItemHandler) GetDescendantsNew(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get parent item to check permission
	var parentWorkspaceID int
	err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", id).Scan(&parentWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to view parent item's workspace
	canView, permErr := h.canViewItem(user.ID, parentWorkspaceID)
	if permErr != nil {
		http.Error(w, "Permission check failed: "+permErr.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Insufficient permissions to view this item's descendants", http.StatusForbidden)
		return
	}

	// Optional depth limit
	maxDepth := 0
	if maxDepthStr := r.URL.Query().Get("max_depth"); maxDepthStr != "" {
		maxDepth, err = strconv.Atoi(maxDepthStr)
		if err != nil || maxDepth < 0 {
			http.Error(w, "Invalid max_depth parameter", http.StatusBadRequest)
			return
		}
	}

	descendants, err := h.hierarchyService.GetDescendants(id, maxDepth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Apply permission filtering
	filteredDescendants, err := h.filterItemsByPermissions(user.ID, descendants)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, filteredDescendants)
}

// GetChildrenNew returns direct children using the new hierarchy service
func (h *ItemHandler) GetChildrenNew(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get parent item to check permission
	var parentWorkspaceID int
	err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", id).Scan(&parentWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Parent item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch parent item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to view parent item's workspace
	canView, permErr := h.canViewItem(user.ID, parentWorkspaceID)
	if permErr != nil {
		http.Error(w, "Permission check failed: "+permErr.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Insufficient permissions to view this item's children", http.StatusForbidden)
		return
	}

	children, err := h.hierarchyService.GetChildren(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Apply permission filtering
	filteredChildren, err := h.filterItemsByPermissions(user.ID, children)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, filteredChildren)
}
