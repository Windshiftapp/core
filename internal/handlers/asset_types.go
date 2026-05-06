package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

// AssetTypeHandler handles asset type operations
type AssetTypeHandler struct {
	repo         *repository.AssetRepository
	assetHandler *AssetHandler // Reuse permission checking methods
	auditor      *logger.Auditor
}

// NewAssetTypeHandler creates a new asset type handler
func NewAssetTypeHandler(repo *repository.AssetRepository, assetHandler *AssetHandler, auditor *logger.Auditor) *AssetTypeHandler {
	return &AssetTypeHandler{
		repo:         repo,
		assetHandler: assetHandler,
		auditor:      auditor,
	}
}

// GetAssetTypes returns all asset types for a set
func (h *AssetTypeHandler) GetAssetTypes(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.assetHandler.requireSetViewAccess(w, r)
	if !ok {
		return
	}

	types, err := h.repo.FindAssetTypesForSet(setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, types)
}

// requireAssetTypeAccess authenticates the user, parses the type ID from "id" path param,
// and looks up the set_id for the asset type. Returns false if any check fails.
func (h *AssetTypeHandler) requireAssetTypeAccess(w http.ResponseWriter, r *http.Request) (typeID, setID int, user *models.User, ok bool) {
	user = utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return 0, 0, nil, false
	}

	typeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return 0, 0, nil, false
	}

	setID, err = h.repo.GetAssetTypeSetID(typeID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_type")
		return 0, 0, nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return 0, 0, nil, false
	}

	return typeID, setID, user, true
}

// requireAssetTypeViewAccess authenticates the user, resolves the asset type's
// set, and verifies view permission on that set. Returns the type ID on success.
func (h *AssetTypeHandler) requireAssetTypeViewAccess(w http.ResponseWriter, r *http.Request) (typeID int, ok bool) {
	typeID, setID, user, ok := h.requireAssetTypeAccess(w, r)
	if !ok {
		return 0, false
	}

	canView, err := h.assetHandler.canViewSet(user.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return 0, false
	}
	if !canView {
		respondNotFound(w, r, "asset set")
		return 0, false
	}
	return typeID, true
}

// requireAssetTypeAdminAccess authenticates the user, resolves the asset type's
// set, and verifies admin permission on that set. Returns the type ID and user on success.
func (h *AssetTypeHandler) requireAssetTypeAdminAccess(w http.ResponseWriter, r *http.Request) (typeID int, user *models.User, ok bool) {
	typeID, setID, user, ok := h.requireAssetTypeAccess(w, r)
	if !ok {
		return 0, nil, false
	}

	canAdmin, err := h.assetHandler.canAdminSet(user.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return 0, nil, false
	}
	if !canAdmin {
		respondAdminRequired(w, r)
		return 0, nil, false
	}

	return typeID, user, true
}

// GetAssetType returns a single asset type
func (h *AssetTypeHandler) GetAssetType(w http.ResponseWriter, r *http.Request) {
	typeID, ok := h.requireAssetTypeViewAccess(w, r)
	if !ok {
		return
	}

	assetType, err := h.repo.FindAssetTypeByID(typeID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	assetType.Fields, err = h.repo.FindAssetTypeFields(typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, assetType)
}

// CreateAssetTypeRequest represents the request body for creating an asset type
type CreateAssetTypeRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	Color        string `json:"color"`
	DisplayOrder int    `json:"display_order"`
	IsActive     *bool  `json:"is_active"`
}

// CreateAssetType creates a new asset type
func (h *AssetTypeHandler) CreateAssetType(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.assetHandler.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[CreateAssetTypeRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	if req.Icon == "" {
		req.Icon = "Box"
	}
	if req.Color == "" {
		req.Color = "#6b7280"
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	now := time.Now()
	assetType := models.AssetType{
		SetID:        setID,
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Color:        req.Color,
		DisplayOrder: req.DisplayOrder,
		IsActive:     isActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	id, err := h.repo.CreateAssetType(&assetType)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	assetType.ID = id
	h.auditor.Log(r, currentUser, logger.ActionAssetTypeCreate, logger.ResourceAssetType, &id, req.Name)

	respondJSONCreated(w, assetType)
}

// UpdateAssetTypeRequest represents the request body for updating an asset type
type UpdateAssetTypeRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	Color        string `json:"color"`
	DisplayOrder int    `json:"display_order"`
	IsActive     *bool  `json:"is_active"`
}

// UpdateAssetType updates an existing asset type
func (h *AssetTypeHandler) UpdateAssetType(w http.ResponseWriter, r *http.Request) {
	typeID, currentUser, ok := h.requireAssetTypeAdminAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[UpdateAssetTypeRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	err := h.repo.UpdateAssetType(typeID, repository.AssetTypeUpdate{
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Color:        req.Color,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.IsActive,
	})
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.auditor.Log(r, currentUser, logger.ActionAssetTypeUpdate, logger.ResourceAssetType, &typeID, req.Name)

	assetType, err := h.repo.GetAssetTypeCoreByID(typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, assetType)
}

// DeleteAssetType deletes an asset type
func (h *AssetTypeHandler) DeleteAssetType(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	typeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	setID, assetCount, err := h.repo.GetAssetTypeSetAndCount(typeID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	canAdmin, err := h.assetHandler.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondAdminRequired(w, r)
		return
	}

	if assetCount > 0 {
		respondConflict(w, r, "Cannot delete type with existing assets. Delete or reassign assets first.")
		return
	}

	if err := h.repo.DeleteAssetType(typeID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "asset_type")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	h.auditor.Log(r, currentUser, logger.ActionAssetTypeDelete, logger.ResourceAssetType, &typeID, "")

	w.WriteHeader(http.StatusNoContent)
}

// GetTypeFields returns fields for an asset type
func (h *AssetTypeHandler) GetTypeFields(w http.ResponseWriter, r *http.Request) {
	typeID, ok := h.requireAssetTypeViewAccess(w, r)
	if !ok {
		return
	}

	fields, err := h.repo.FindAssetTypeFields(typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}

// UpdateTypeFieldsRequest represents the request body for updating type fields
type UpdateTypeFieldsRequest struct {
	Fields []struct {
		CustomFieldID int  `json:"custom_field_id"`
		IsRequired    bool `json:"is_required"`
		DisplayOrder  int  `json:"display_order"`
	} `json:"fields"`
}

// UpdateTypeFields updates the custom fields for an asset type
func (h *AssetTypeHandler) UpdateTypeFields(w http.ResponseWriter, r *http.Request) {
	typeID, _, ok := h.requireAssetTypeAdminAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[UpdateTypeFieldsRequest](w, r)
	if !ok {
		return
	}

	assignments := make([]repository.AssetTypeFieldAssignment, len(req.Fields))
	for i, f := range req.Fields {
		assignments[i] = repository.AssetTypeFieldAssignment{
			CustomFieldID: f.CustomFieldID,
			IsRequired:    f.IsRequired,
			DisplayOrder:  f.DisplayOrder,
		}
	}

	if err := h.repo.ReplaceAssetTypeFields(typeID, assignments); err != nil {
		respondInternalError(w, r, err)
		return
	}

	fields, err := h.repo.FindAssetTypeFields(typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}
