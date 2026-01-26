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

// GetTree returns the item and all its descendants as a nested tree structure
func (h *ItemHandler) GetTree(w http.ResponseWriter, r *http.Request) {
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

	// Get the root item
	repo := repository.NewItemRepository(h.db)
	rootItem, err := repo.FindByID(id)
	if err != nil {
		if err == repository.ErrNotFound {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to view item's workspace
	canView, permErr := h.canViewItem(user.ID, rootItem.WorkspaceID)
	if permErr != nil {
		http.Error(w, "Permission check failed: "+permErr.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Insufficient permissions to view this item", http.StatusForbidden)
		return
	}

	// Get all descendants
	descendants, err := h.hierarchyService.GetDescendants(id, 0)
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

	// Build tree structure
	tree := h.buildItemTree(rootItem, filteredDescendants)

	respondJSONOK(w, tree)
}

// ItemTreeNode represents an item with its children in a tree structure
type ItemTreeNode struct {
	*models.Item
	Children []*ItemTreeNode `json:"children"`
}

// buildItemTree constructs a nested tree from a root item and its descendants
func (h *ItemHandler) buildItemTree(root *models.Item, descendants []models.Item) *ItemTreeNode {
	// Create a map for quick lookup
	nodeMap := make(map[int]*ItemTreeNode)

	// Create node for root
	rootNode := &ItemTreeNode{
		Item:     root,
		Children: make([]*ItemTreeNode, 0),
	}
	nodeMap[root.ID] = rootNode

	// Create nodes for all descendants
	for i := range descendants {
		item := &descendants[i]
		nodeMap[item.ID] = &ItemTreeNode{
			Item:     item,
			Children: make([]*ItemTreeNode, 0),
		}
	}

	// Build tree by linking children to parents
	for _, item := range descendants {
		if item.ParentID != nil {
			if parentNode, exists := nodeMap[*item.ParentID]; exists {
				parentNode.Children = append(parentNode.Children, nodeMap[item.ID])
			}
		}
	}

	return rootNode
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
