package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// scanCategoryRow scans a single asset category row with all joined fields.
func scanCategoryRow(s interface{ Scan(...interface{}) error }) (models.AssetCategory, error) {
	var cat models.AssetCategory
	var description, path, fracIndex, setName, parentName sql.NullString
	var parentID sql.NullInt64

	err := s.Scan(
		&cat.ID, &cat.SetID, &cat.Name, &description, &parentID, &path,
		&cat.HasChildren, &cat.ChildrenCount, &cat.DescendantsCount, &fracIndex,
		&cat.CreatedAt, &cat.UpdatedAt,
		&setName, &parentName, &cat.AssetCount,
	)
	if err != nil {
		return cat, err
	}

	cat.Description = description.String
	cat.ParentID = utils.NullInt64ToPtr(parentID)
	cat.Path = path.String
	cat.FracIndex = utils.NullStringToPtr(fracIndex)
	cat.SetName = setName.String
	cat.ParentName = parentName.String

	return cat, nil
}

// scanCategoryBasicRow scans a category row without set_name, parent_name, or asset_count columns.
func scanCategoryBasicRow(s interface{ Scan(...interface{}) error }) (models.AssetCategory, error) {
	var cat models.AssetCategory
	var description, path, fracIndex sql.NullString
	var parentID sql.NullInt64

	err := s.Scan(
		&cat.ID, &cat.SetID, &cat.Name, &description, &parentID, &path,
		&cat.HasChildren, &cat.ChildrenCount, &cat.DescendantsCount, &fracIndex,
		&cat.CreatedAt, &cat.UpdatedAt,
	)
	if err != nil {
		return cat, err
	}

	cat.Description = description.String
	cat.ParentID = utils.NullInt64ToPtr(parentID)
	cat.Path = path.String
	cat.FracIndex = utils.NullStringToPtr(fracIndex)

	return cat, nil
}

// AssetCategoryHandler handles asset category operations
type AssetCategoryHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	assetHandler      *AssetHandler // Reuse permission checking methods
}

// NewAssetCategoryHandler creates a new asset category handler
func NewAssetCategoryHandler(db database.Database, permissionService *services.PermissionService) *AssetCategoryHandler {
	return &AssetCategoryHandler{
		db:                db,
		permissionService: permissionService,
		assetHandler:      NewAssetHandler(db, permissionService, ""),
	}
}

// requireCategoryEditAccess authenticates the user, extracts the category ID from the "id" route
// param, looks up the category's set, and checks edit permission. Returns the user, category ID,
// set ID, and true on success; writes the appropriate error response and returns false on failure.
func (h *AssetCategoryHandler) requireCategoryEditAccess(w http.ResponseWriter, r *http.Request) (user *models.User, categoryID, setID int, ok bool) {
	var currentUser *models.User
	currentUser, ok = RequireAuth(w, r)
	if !ok {
		return nil, 0, 0, false
	}

	categoryID, ok = requireIDParam(w, r, "id")
	if !ok {
		return nil, 0, 0, false
	}

	err := h.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", categoryID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "category")
		return nil, 0, 0, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, 0, 0, false
	}

	canEdit, err := h.assetHandler.canEditSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, 0, 0, false
	}
	if !canEdit {
		respondNotFound(w, r, "asset set")
		return nil, 0, 0, false
	}

	return currentUser, categoryID, setID, true
}

// GetCategories returns all categories for a set (optionally as tree)
func (h *AssetCategoryHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.assetHandler.requireSetViewAccess(w, r)
	if !ok {
		return
	}

	// Check if tree format is requested
	isTree := r.URL.Query().Get("tree") == "true"

	query := `
		SELECT ac.id, ac.set_id, ac.name, ac.description, ac.parent_id, ac.path,
		       ac.has_children, ac.children_count, ac.descendants_count, ac.frac_index,
		       ac.created_at, ac.updated_at,
		       ams.name as set_name,
		       pc.name as parent_name,
		       (SELECT COUNT(*) FROM assets WHERE category_id = ac.id) as asset_count
		FROM asset_categories ac
		LEFT JOIN asset_management_sets ams ON ac.set_id = ams.id
		LEFT JOIN asset_categories pc ON ac.parent_id = pc.id
		WHERE ac.set_id = ?
		ORDER BY ac.frac_index, ac.name
	`

	rows, err := h.db.Query(query, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var categories []models.AssetCategory
	for rows.Next() {
		cat, err := scanCategoryRow(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		categories = append(categories, cat)
	}

	if isTree {
		// Build tree structure
		tree := h.buildCategoryTree(categories)
		respondJSONOK(w, tree)
		return
	}

	respondJSONOK(w, categories)
}

// buildCategoryTree builds a hierarchical tree from flat category list
func (h *AssetCategoryHandler) buildCategoryTree(categories []models.AssetCategory) []models.AssetCategory {
	// Create maps for lookup and parent-child relationships
	catMap := make(map[int]*models.AssetCategory)
	childrenMap := make(map[int][]int) // parent_id -> child_ids

	for i := range categories {
		categories[i].Children = []models.AssetCategory{}
		catMap[categories[i].ID] = &categories[i]

		if categories[i].ParentID != nil {
			childrenMap[*categories[i].ParentID] = append(childrenMap[*categories[i].ParentID], categories[i].ID)
		}
	}

	// Recursive function to build subtree
	var buildSubtree func(id int) models.AssetCategory
	buildSubtree = func(id int) models.AssetCategory {
		cat := *catMap[id]
		cat.Children = []models.AssetCategory{}
		for _, childID := range childrenMap[id] {
			cat.Children = append(cat.Children, buildSubtree(childID))
		}
		return cat
	}

	// Build roots
	var roots []models.AssetCategory
	for i := range categories {
		if categories[i].ParentID == nil {
			roots = append(roots, buildSubtree(categories[i].ID))
		}
	}

	return roots
}

// GetCategory returns a single category
func (h *AssetCategoryHandler) GetCategory(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	categoryID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Get the category to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", categoryID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "category")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check view permission
	canView, err := h.assetHandler.canViewSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondNotFound(w, r, "asset set")
		return
	}

	cat, err := scanCategoryRow(h.db.QueryRow(`
		SELECT ac.id, ac.set_id, ac.name, ac.description, ac.parent_id, ac.path,
		       ac.has_children, ac.children_count, ac.descendants_count, ac.frac_index,
		       ac.created_at, ac.updated_at,
		       ams.name as set_name,
		       pc.name as parent_name,
		       (SELECT COUNT(*) FROM assets WHERE category_id = ac.id) as asset_count
		FROM asset_categories ac
		LEFT JOIN asset_management_sets ams ON ac.set_id = ams.id
		LEFT JOIN asset_categories pc ON ac.parent_id = pc.id
		WHERE ac.id = ?
	`, categoryID))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, cat)
}

// CreateCategoryRequest represents the request body for creating a category
type CreateCategoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    *int   `json:"parent_id,omitempty"`
}

// CreateCategory creates a new category
func (h *AssetCategoryHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.assetHandler.requireSetEditAccess(w, r)
	if !ok {
		return
	}
	var err error

	req, ok := decodeJSON[CreateCategoryRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Validate parent if provided
	if req.ParentID != nil {
		var parentSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", *req.ParentID).Scan(&parentSetID)
		if err == sql.ErrNoRows {
			respondValidationError(w, r, "Parent category not found")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if parentSetID != setID {
			respondValidationError(w, r, "Parent category must belong to same set")
			return
		}
	}

	now := time.Now()

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var catID int64
	err = tx.QueryRow(`
		INSERT INTO asset_categories (set_id, name, description, parent_id, path, created_at, updated_at)
		VALUES (?, ?, ?, ?, '/', ?, ?) RETURNING id
	`, setID, req.Name, req.Description, req.ParentID, now, now).Scan(&catID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update parent counts if there's a parent
	if req.ParentID != nil {
		err = h.updateParentCounts(tx, *req.ParentID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	id := int(catID)
	logAudit(h.db, r, currentUser, logger.ActionAssetCategoryCreate, logger.ResourceAssetCategory, &id, req.Name)

	cat := models.AssetCategory{
		ID:          int(catID),
		SetID:       setID,
		Name:        req.Name,
		Description: req.Description,
		ParentID:    req.ParentID,
		Path:        "/",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	respondJSONCreated(w, cat)
}

// UpdateCategoryRequest represents the request body for updating a category
type UpdateCategoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateCategory updates an existing category
func (h *AssetCategoryHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	currentUser, categoryID, _, ok := h.requireCategoryEditAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[UpdateCategoryRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	now := time.Now()

	result, err := h.db.ExecWrite(`
		UPDATE asset_categories SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Description, now, categoryID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "category")
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetCategoryUpdate, logger.ResourceAssetCategory, &categoryID, req.Name)

	// Return updated category
	cat, _ := scanCategoryBasicRow(h.db.QueryRow(`
		SELECT id, set_id, name, description, parent_id, path, has_children, children_count, descendants_count, frac_index, created_at, updated_at
		FROM asset_categories WHERE id = ?
	`, categoryID))

	respondJSONOK(w, cat)
}

// DeleteCategory deletes a category
func (h *AssetCategoryHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	categoryID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Get the category to check permissions and counts
	var setID int
	var hasChildren bool
	var assetCount int
	var parentID sql.NullInt64
	err = h.db.QueryRow(`
		SELECT set_id, has_children, parent_id, (SELECT COUNT(*) FROM assets WHERE category_id = ?) as asset_count
		FROM asset_categories WHERE id = ?
	`, categoryID, categoryID).Scan(&setID, &hasChildren, &parentID, &assetCount)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "category")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check edit permission
	canEdit, err := h.assetHandler.canEditSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canEdit {
		respondNotFound(w, r, "asset set")
		return
	}

	// Prevent deletion if has children
	if hasChildren {
		respondConflict(w, r, "Cannot delete category with children. Delete children first.")
		return
	}

	// Prevent deletion if has assets
	if assetCount > 0 {
		respondConflict(w, r, "Cannot delete category with assets. Move or delete assets first.")
		return
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec("DELETE FROM asset_categories WHERE id = ?", categoryID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "category")
		return
	}

	// Update parent counts if there was a parent
	if parentID.Valid {
		err = h.updateParentCounts(tx, int(parentID.Int64))
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetCategoryDelete, logger.ResourceAssetCategory, &categoryID, "")

	w.WriteHeader(http.StatusNoContent)
}

// MoveCategoryRequest represents the request body for moving a category
type MoveCategoryRequest struct {
	ParentID *int `json:"parent_id"` // nil means move to root
}

// MoveCategory moves a category to a new parent
func (h *AssetCategoryHandler) MoveCategory(w http.ResponseWriter, r *http.Request) {
	_, categoryID, setID, ok := h.requireCategoryEditAccess(w, r)
	if !ok {
		return
	}

	// Also need old parent ID for count updates
	var oldParentID sql.NullInt64
	_ = h.db.QueryRow("SELECT parent_id FROM asset_categories WHERE id = ?", categoryID).Scan(&oldParentID)

	var err error

	req, ok := decodeJSON[MoveCategoryRequest](w, r)
	if !ok {
		return
	}

	// Validate new parent if provided
	if req.ParentID != nil {
		// Cannot be own parent
		if *req.ParentID == categoryID {
			respondValidationError(w, r, "Cannot move category to itself")
			return
		}

		var parentSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", *req.ParentID).Scan(&parentSetID)
		if err == sql.ErrNoRows {
			respondValidationError(w, r, "New parent category not found")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if parentSetID != setID {
			respondValidationError(w, r, "New parent must belong to same set")
			return
		}

		// Check for circular reference (cannot move to a descendant)
		var isDescendant bool
		isDescendant, err = h.isDescendant(*req.ParentID, categoryID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if isDescendant {
			respondValidationError(w, r, "Cannot move category to one of its descendants")
			return
		}
	}

	now := time.Now()

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Update the category's parent
	_, err = tx.Exec("UPDATE asset_categories SET parent_id = ?, updated_at = ? WHERE id = ?",
		req.ParentID, now, categoryID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update old parent counts
	if oldParentID.Valid {
		err = h.updateParentCountsTx(tx, int(oldParentID.Int64))
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Update new parent counts
	if req.ParentID != nil {
		err = h.updateParentCountsTx(tx, *req.ParentID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return updated category
	cat, _ := scanCategoryBasicRow(h.db.QueryRow(`
		SELECT id, set_id, name, description, parent_id, path, has_children, children_count, descendants_count, frac_index, created_at, updated_at
		FROM asset_categories WHERE id = ?
	`, categoryID))

	respondJSONOK(w, cat)
}

// isDescendant checks if potentialDescendant is a descendant of categoryID
func (h *AssetCategoryHandler) isDescendant(potentialDescendant, categoryID int) (bool, error) {
	// Use recursive CTE to find all ancestors of potentialDescendant
	rows, err := h.db.Query(`
		WITH RECURSIVE ancestors AS (
			SELECT parent_id FROM asset_categories WHERE id = ?
			UNION ALL
			SELECT ac.parent_id FROM asset_categories ac
			INNER JOIN ancestors a ON ac.id = a.parent_id
			WHERE ac.parent_id IS NOT NULL
		)
		SELECT 1 FROM ancestors WHERE parent_id = ? LIMIT 1
	`, potentialDescendant, categoryID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	return rows.Next(), nil
}

// updateParentCounts updates the children counts for a parent category (using DB transaction)
func (h *AssetCategoryHandler) updateParentCounts(tx database.Tx, parentID int) error {
	return h.updateParentCountsTx(tx, parentID)
}

// updateParentCountsTx updates the children counts for a parent category
func (h *AssetCategoryHandler) updateParentCountsTx(tx database.Tx, parentID int) error {
	// Count direct children
	var childrenCount int
	err := tx.QueryRow("SELECT COUNT(*) FROM asset_categories WHERE parent_id = ?", parentID).Scan(&childrenCount)
	if err != nil {
		return err
	}

	// Update the parent
	_, err = tx.Exec(`
		UPDATE asset_categories
		SET children_count = ?, has_children = ?, updated_at = ?
		WHERE id = ?
	`, childrenCount, childrenCount > 0, time.Now(), parentID)
	if err != nil {
		return err
	}

	// Update descendants_count for all ancestors using recursive CTE
	_, err = tx.Exec(`
		WITH RECURSIVE ancestors AS (
			SELECT parent_id as id FROM asset_categories WHERE id = ? AND parent_id IS NOT NULL
			UNION ALL
			SELECT ac.parent_id as id FROM asset_categories ac
			INNER JOIN ancestors a ON ac.id = a.id
			WHERE ac.parent_id IS NOT NULL
		)
		UPDATE asset_categories
		SET descendants_count = (
			WITH RECURSIVE descendants AS (
				SELECT id FROM asset_categories WHERE parent_id = asset_categories.id
				UNION ALL
				SELECT ac.id FROM asset_categories ac
				INNER JOIN descendants d ON ac.parent_id = d.id
			)
			SELECT COUNT(*) FROM descendants
		)
		WHERE id IN (SELECT id FROM ancestors)
	`, parentID)

	return err
}
