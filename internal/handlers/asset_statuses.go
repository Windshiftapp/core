package handlers

import (
	"errors"
	"net/http"
	"time"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// AssetStatusHandler handles asset status operations
type AssetStatusHandler struct {
	repo         *repository.AssetRepository
	assetHandler *AssetHandler
	auditor      *logger.Auditor
}

// NewAssetStatusHandler creates a new asset status handler
func NewAssetStatusHandler(repo *repository.AssetRepository, assetHandler *AssetHandler, auditor *logger.Auditor) *AssetStatusHandler {
	return &AssetStatusHandler{
		repo:         repo,
		assetHandler: assetHandler,
		auditor:      auditor,
	}
}

// GetAssetStatuses returns all asset statuses for a set
func (h *AssetStatusHandler) GetAssetStatuses(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.assetHandler.requireSetViewAccess(w, r)
	if !ok {
		return
	}

	statuses, err := h.repo.FindAssetStatusesForSet(setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, statuses)
}

// requireStatusSetID authenticates, parses the "id" param, and looks up the owning set_id.
func (h *AssetStatusHandler) requireStatusSetID(w http.ResponseWriter, r *http.Request) (user *models.User, statusID, setID int, ok bool) {
	user, ok = RequireAuth(w, r)
	if !ok {
		return nil, 0, 0, false
	}
	statusID, ok = requireIDParam(w, r, "id")
	if !ok {
		return nil, 0, 0, false
	}
	setID, err := h.repo.GetAssetStatusSetID(statusID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_status")
		return nil, 0, 0, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, 0, 0, false
	}
	return user, statusID, setID, true
}

// requireStatusAdminAccess calls requireStatusSetID and then verifies the user has admin permission on the set.
func (h *AssetStatusHandler) requireStatusAdminAccess(w http.ResponseWriter, r *http.Request) (user *models.User, statusID, setID int, ok bool) {
	currentUser, statusID, setID, ok := h.requireStatusSetID(w, r)
	if !ok {
		return nil, 0, 0, false
	}
	canAdmin, err := h.assetHandler.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, 0, 0, false
	}
	if !canAdmin {
		respondAdminRequired(w, r)
		return nil, 0, 0, false
	}
	return currentUser, statusID, setID, true
}

// GetAssetStatus returns a single asset status
func (h *AssetStatusHandler) GetAssetStatus(w http.ResponseWriter, r *http.Request) {
	currentUser, statusID, setID, ok := h.requireStatusSetID(w, r)
	if !ok {
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

	status, err := h.repo.FindAssetStatusByID(statusID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_status")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, status)
}

// CreateAssetStatusRequest represents the request body for creating an asset status
type CreateAssetStatusRequest struct {
	Name         string `json:"name"`
	Color        string `json:"color"`
	Description  string `json:"description"`
	IsDefault    bool   `json:"is_default"`
	DisplayOrder int    `json:"display_order"`
}

// CreateAssetStatus creates a new asset status
func (h *AssetStatusHandler) CreateAssetStatus(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.assetHandler.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[CreateAssetStatusRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	if req.Color == "" {
		req.Color = "#6b7280"
	}

	if req.IsDefault {
		if err := h.repo.ClearDefaultStatuses(setID); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	now := time.Now()
	status := models.AssetStatus{
		SetID:        setID,
		Name:         req.Name,
		Color:        req.Color,
		Description:  req.Description,
		IsDefault:    req.IsDefault,
		DisplayOrder: req.DisplayOrder,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	id, err := h.repo.CreateAssetStatus(&status)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	status.ID = id
	h.auditor.Log(r, currentUser, logger.ActionAssetStatusCreate, logger.ResourceAssetStatus, &id, req.Name)

	respondJSONCreated(w, status)
}

// UpdateAssetStatusRequest represents the request body for updating an asset status
type UpdateAssetStatusRequest struct {
	Name         string `json:"name"`
	Color        string `json:"color"`
	Description  string `json:"description"`
	IsDefault    *bool  `json:"is_default"`
	DisplayOrder int    `json:"display_order"`
}

// UpdateAssetStatus updates an existing asset status
func (h *AssetStatusHandler) UpdateAssetStatus(w http.ResponseWriter, r *http.Request) {
	currentUser, statusID, setID, ok := h.requireStatusAdminAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[UpdateAssetStatusRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	if req.IsDefault != nil && *req.IsDefault {
		if err := h.repo.ClearDefaultStatusesExcept(setID, statusID); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	err := h.repo.UpdateAssetStatus(statusID, repository.AssetStatusUpdate{
		Name:         req.Name,
		Color:        req.Color,
		Description:  req.Description,
		DisplayOrder: req.DisplayOrder,
		IsDefault:    req.IsDefault,
	})
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_status")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.auditor.Log(r, currentUser, logger.ActionAssetStatusUpdate, logger.ResourceAssetStatus, &statusID, req.Name)

	status, err := h.repo.FindAssetStatusByID(statusID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, status)
}

// DeleteAssetStatus deletes an asset status
func (h *AssetStatusHandler) DeleteAssetStatus(w http.ResponseWriter, r *http.Request) {
	currentUser, statusID, _, ok := h.requireStatusAdminAccess(w, r)
	if !ok {
		return
	}

	assetCount, err := h.repo.CountAssetsUsingStatus(statusID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if assetCount > 0 {
		respondConflict(w, r, "Cannot delete status with existing assets. Reassign assets first.")
		return
	}

	if err := h.repo.DeleteAssetStatus(statusID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "asset_status")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	h.auditor.Log(r, currentUser, logger.ActionAssetStatusDelete, logger.ResourceAssetStatus, &statusID, "")

	w.WriteHeader(http.StatusNoContent)
}
