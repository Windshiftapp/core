package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// wouldCreateHierarchyCycle reports whether making newParentID the parent of
// something in ancestorCandidateID's subtree would create a cycle. It walks
// parent_id from newParentID upwards; if ancestorCandidateID is encountered
// (or equals newParentID), a cycle would result. Bounded walk (30 steps)
// matches the hierarchy depth ceiling.
func wouldCreateHierarchyCycle(db database.Database, ancestorCandidateID, newParentID int) (bool, error) {
	const maxWalk = 30
	itemRepo := repository.NewItemRepository(db)
	current := newParentID
	for i := 0; i < maxWalk; i++ {
		if current == ancestorCandidateID {
			return true, nil
		}
		parent, err := itemRepo.GetParentID(current)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return false, nil
			}
			return false, fmt.Errorf("failed to walk hierarchy: %w", err)
		}
		if parent == nil {
			return false, nil
		}
		current = *parent
	}
	// Walk exhausted the depth ceiling without terminating — treat as a cycle
	// (an existing cycle or a hierarchy already deeper than the ceiling).
	return true, nil
}

// requireItemViewByWorkspace authenticates the user, looks up the item's workspace,
// and verifies view permission. Returns the user and true on success; writes an
// error response and returns nil/false on failure.
func (h *ItemHandler) requireItemViewByWorkspace(w http.ResponseWriter, r *http.Request, itemID int) (*models.User, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, false
	}

	workspaceID, err := repository.NewItemRepository(h.db).GetWorkspaceID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "item")
			return nil, false
		}
		respondInternalError(w, r, fmt.Errorf("failed to fetch item: %w", err))
		return nil, false
	}

	canView, permErr := h.canViewItem(user.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, fmt.Errorf("permission check failed: %w", permErr))
		return nil, false
	}
	if !canView {
		respondNotFound(w, r, "Item")
		return nil, false
	}

	return user, true
}

// GetAncestors returns all ancestors of an item (for breadcrumbs)
func (h *ItemHandler) GetAncestors(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := h.requireItemViewByWorkspace(w, r, id)
	if !ok {
		return
	}

	ancestors, err := h.hierarchyService.GetAncestors(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Apply permission filtering to ancestors as well
	filteredAncestors, err := h.filterItemsByPermissions(user.ID, ancestors)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("permission check failed: %w", err))
		return
	}

	// Load labels
	if err := LoadLabelsForItems(h.db, filteredAncestors); err != nil {
		slog.Warn("failed to load labels for ancestors", slog.Any("error", err))
	}

	respondJSONOK(w, filteredAncestors)
}

// GetDescendantsNew returns all descendants using the new hierarchy service
func (h *ItemHandler) GetDescendantsNew(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := h.requireItemViewByWorkspace(w, r, id)
	if !ok {
		return
	}

	// Optional depth limit
	var err error
	maxDepth := 0
	if maxDepthStr := r.URL.Query().Get("max_depth"); maxDepthStr != "" {
		maxDepth, err = strconv.Atoi(maxDepthStr)
		if err != nil || maxDepth < 0 {
			respondValidationError(w, r, "Invalid max_depth parameter")
			return
		}
	}

	descendants, err := h.hierarchyService.GetDescendants(id, maxDepth)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Apply permission filtering
	filteredDescendants, err := h.filterItemsByPermissions(user.ID, descendants)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("permission check failed: %w", err))
		return
	}

	// Load labels
	if err := LoadLabelsForItems(h.db, filteredDescendants); err != nil {
		slog.Warn("failed to load labels for descendants", slog.Any("error", err))
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
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get the root item
	repo := repository.NewItemRepository(h.db)
	rootItem, err := repo.FindByID(id)
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to fetch item: %w", err))
		return
	}

	// Check if user has permission to view item's workspace
	canView, permErr := h.canViewItem(user.ID, rootItem.WorkspaceID)
	if permErr != nil {
		respondInternalError(w, r, fmt.Errorf("permission check failed: %w", permErr))
		return
	}
	if !canView {
		respondNotFound(w, r, "Item")
		return
	}

	// Get all descendants
	descendants, err := h.hierarchyService.GetDescendants(id, 0)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Apply permission filtering
	filteredDescendants, err := h.filterItemsByPermissions(user.ID, descendants)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("permission check failed: %w", err))
		return
	}

	// Load labels for root item and descendants
	allItems := append([]models.Item{*rootItem}, filteredDescendants...)
	if err := LoadLabelsForItems(h.db, allItems); err != nil {
		slog.Warn("failed to load labels for tree", slog.Any("error", err))
	}
	*rootItem = allItems[0]
	copy(filteredDescendants, allItems[1:])

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

	user, ok := h.requireItemViewByWorkspace(w, r, id)
	if !ok {
		return
	}

	children, err := h.hierarchyService.GetChildren(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Apply permission filtering
	filteredChildren, err := h.filterItemsByPermissions(user.ID, children)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("permission check failed: %w", err))
		return
	}

	// Load labels
	if err := LoadLabelsForItems(h.db, filteredChildren); err != nil {
		slog.Warn("failed to load labels for children", slog.Any("error", err))
	}

	respondJSONOK(w, filteredChildren)
}
