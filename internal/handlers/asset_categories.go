package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// AssetCategoryHandler handles asset category operations
type AssetCategoryHandler struct {
	db                database.Database
	repo              *repository.AssetRepository
	permissionService *services.PermissionService
	assetHandler      *AssetHandler // Reuse permission checking methods
}

// NewAssetCategoryHandler creates a new asset category handler
func NewAssetCategoryHandler(db database.Database, permissionService *services.PermissionService) *AssetCategoryHandler {
	return &AssetCategoryHandler{
		db:                db,
		repo:              repository.NewAssetRepository(db),
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

	setID, err := h.repo.GetAssetCategorySetID(categoryID)
	if errors.Is(err, repository.ErrNotFound) {
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

	isTree := r.URL.Query().Get("tree") == "true"

	categories, err := h.repo.FindAssetCategoriesForSet(setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if isTree {
		respondJSONOK(w, h.buildCategoryTree(categories))
		return
	}

	respondJSONOK(w, categories)
}

// buildCategoryTree builds a hierarchical tree from flat category list
func (h *AssetCategoryHandler) buildCategoryTree(categories []models.AssetCategory) []models.AssetCategory {
	catMap := make(map[int]*models.AssetCategory)
	childrenMap := make(map[int][]int)

	for i := range categories {
		categories[i].Children = []models.AssetCategory{}
		catMap[categories[i].ID] = &categories[i]
		if categories[i].ParentID != nil {
			childrenMap[*categories[i].ParentID] = append(childrenMap[*categories[i].ParentID], categories[i].ID)
		}
	}

	var buildSubtree func(id int) models.AssetCategory
	buildSubtree = func(id int) models.AssetCategory {
		cat := *catMap[id]
		cat.Children = []models.AssetCategory{}
		for _, childID := range childrenMap[id] {
			cat.Children = append(cat.Children, buildSubtree(childID))
		}
		return cat
	}

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

	setID, err := h.repo.GetAssetCategorySetID(categoryID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "category")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	canView, err := h.assetHandler.canViewSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondNotFound(w, r, "asset set")
		return
	}

	cat, err := h.repo.FindAssetCategoryByID(categoryID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "category")
		return
	}
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

	req, ok := decodeJSON[CreateCategoryRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	if req.ParentID != nil {
		parentSetID, err := h.repo.GetAssetCategorySetID(*req.ParentID)
		if errors.Is(err, repository.ErrNotFound) {
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

	id, createdAt, err := h.repo.CreateAssetCategory(repository.CreateAssetCategoryInput{
		SetID:       setID,
		Name:        req.Name,
		Description: req.Description,
		ParentID:    req.ParentID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetCategoryCreate, logger.ResourceAssetCategory, &id, req.Name)

	respondJSONCreated(w, models.AssetCategory{
		ID:          id,
		SetID:       setID,
		Name:        req.Name,
		Description: req.Description,
		ParentID:    req.ParentID,
		Path:        "/",
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	})
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

	err := h.repo.UpdateAssetCategoryNameDescription(categoryID, req.Name, req.Description)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "category")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetCategoryUpdate, logger.ResourceAssetCategory, &categoryID, req.Name)

	cat, err := h.repo.GetAssetCategoryCoreByID(categoryID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

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

	setID, hasChildren, parentID, assetCount, err := h.repo.GetAssetCategoryDeletionInfo(categoryID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "category")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	canEdit, err := h.assetHandler.canEditSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canEdit {
		respondNotFound(w, r, "asset set")
		return
	}

	if hasChildren {
		respondConflict(w, r, "Cannot delete category with children. Delete children first.")
		return
	}
	if assetCount > 0 {
		respondConflict(w, r, "Cannot delete category with assets. Move or delete assets first.")
		return
	}

	if err := h.repo.DeleteAssetCategory(categoryID, parentID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "category")
			return
		}
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

	oldParentID, _ := h.repo.GetAssetCategoryParentID(categoryID)

	req, ok := decodeJSON[MoveCategoryRequest](w, r)
	if !ok {
		return
	}

	if req.ParentID != nil {
		if *req.ParentID == categoryID {
			respondValidationError(w, r, "Cannot move category to itself")
			return
		}

		parentSetID, err := h.repo.GetAssetCategorySetID(*req.ParentID)
		if errors.Is(err, repository.ErrNotFound) {
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

		isDescendant, err := h.repo.IsAssetCategoryDescendantOf(*req.ParentID, categoryID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if isDescendant {
			respondValidationError(w, r, "Cannot move category to one of its descendants")
			return
		}
	}

	if err := h.repo.MoveAssetCategory(categoryID, oldParentID, req.ParentID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	cat, err := h.repo.GetAssetCategoryCoreByID(categoryID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, cat)
}
