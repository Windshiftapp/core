package handlers

import (
	"errors"
	"net/http"
	"time"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// GetAssetSets returns all asset sets the user has access to
func (h *AssetHandler) GetAssetSets(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	isAdmin, _ := h.permissionService.HasGlobalPermission(currentUser.ID, "system.admin")

	sets, err := h.repo.ListSetsForUser(currentUser.ID, isAdmin)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	for i := range sets {
		if isAdmin {
			sets[i].UserPermission = AssetRoleAdministrator
		} else {
			sets[i].UserPermission, _ = h.getUserSetRoleName(currentUser.ID, sets[i].ID)
		}
	}

	respondJSONOK(w, sets)
}

// GetAssetSet returns a single asset set
func (h *AssetHandler) GetAssetSet(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	setID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	canView, err := h.canViewSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondNotFound(w, r, "asset set")
		return
	}

	set, err := h.repo.GetSetByID(setID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	set.UserPermission, _ = h.getUserSetRoleName(currentUser.ID, setID)

	respondJSONOK(w, set)
}

// CreateAssetSetRequest represents the request body for creating an asset set
type CreateAssetSetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// CreateAssetSet creates a new asset management set
func (h *AssetHandler) CreateAssetSet(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check if user has asset.manage permission or is system admin
	hasPermission, err := h.permissionService.HasGlobalPermission(currentUser.ID, "system.admin")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !hasPermission {
		hasPermission, err = h.permissionService.HasGlobalPermission(currentUser.ID, "asset.manage")
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}
	if !hasPermission {
		respondForbidden(w, r)
		return
	}

	req, ok := decodeJSON[CreateAssetSetRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	if req.IsDefault {
		if err := h.repo.ClearDefaultSet(); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	now := time.Now()
	newSet := models.AssetManagementSet{
		Name:        req.Name,
		Description: req.Description,
		IsDefault:   req.IsDefault,
		CreatedBy:   &currentUser.ID,
	}
	setID, err := h.repo.CreateSet(&newSet)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	adminRoleID, err := h.repo.GetAssetRoleIDByName(AssetRoleAdministrator)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if err := h.repo.AssignUserRole(setID, currentUser.ID, adminRoleID, currentUser.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.createDefaultStatuses(setID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetSetCreate, logger.ResourceAssetSet, &setID, req.Name)

	respondJSONCreated(w, models.AssetManagementSet{
		ID:             setID,
		Name:           req.Name,
		Description:    req.Description,
		IsDefault:      req.IsDefault,
		CreatedBy:      &currentUser.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
		UserPermission: AssetRoleAdministrator,
	})
}

// UpdateAssetSetRequest represents the request body for updating an asset set
type UpdateAssetSetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// UpdateAssetSet updates an asset management set
func (h *AssetHandler) UpdateAssetSet(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.requireSetAdminByID(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[UpdateAssetSetRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	if req.IsDefault {
		if err := h.repo.ClearDefaultSetExcept(setID); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	err := h.repo.UpdateSet(&models.AssetManagementSet{
		ID:          setID,
		Name:        req.Name,
		Description: req.Description,
		IsDefault:   req.IsDefault,
	})
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetSetUpdate, logger.ResourceAssetSet, &setID, req.Name)

	set, err := h.repo.GetAssetSetCoreByID(setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	set.UserPermission = AssetRoleAdministrator

	respondJSONOK(w, set)
}

// DeleteAssetSet deletes an asset management set
func (h *AssetHandler) DeleteAssetSet(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	setID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Only system admins can delete sets
	isAdmin, err := h.permissionService.HasGlobalPermission(currentUser.ID, "system.admin")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !isAdmin {
		respondAdminRequired(w, r)
		return
	}

	if err := h.repo.HardDeleteSet(setID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "set")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetSetDelete, logger.ResourceAssetSet, &setID, "")

	w.WriteHeader(http.StatusNoContent)
}
