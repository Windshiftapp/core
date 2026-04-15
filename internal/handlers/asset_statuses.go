package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
)

// scanAssetStatusRow scans a single asset status row (9 columns) and maps the nullable description.
func scanAssetStatusRow(s interface{ Scan(...interface{}) error }) (models.AssetStatus, error) {
	var status models.AssetStatus
	var description sql.NullString

	err := s.Scan(
		&status.ID, &status.SetID, &status.Name, &status.Color, &description,
		&status.IsDefault, &status.DisplayOrder, &status.CreatedAt, &status.UpdatedAt,
	)
	if err != nil {
		return status, err
	}

	if description.Valid {
		status.Description = description.String
	}

	return status, nil
}

// AssetStatusHandler handles asset status operations
type AssetStatusHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	assetHandler      *AssetHandler
}

// NewAssetStatusHandler creates a new asset status handler
func NewAssetStatusHandler(db database.Database, permissionService *services.PermissionService) *AssetStatusHandler {
	return &AssetStatusHandler{
		db:                db,
		permissionService: permissionService,
		assetHandler:      NewAssetHandler(db, permissionService, ""),
	}
}

// GetAssetStatuses returns all asset statuses for a set
func (h *AssetStatusHandler) GetAssetStatuses(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.assetHandler.requireSetViewAccess(w, r)
	if !ok {
		return
	}

	query := `
		SELECT id, set_id, name, color, description, is_default, display_order, created_at, updated_at
		FROM asset_statuses
		WHERE set_id = ?
		ORDER BY display_order, name
	`

	rows, err := h.db.Query(query, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var statuses []models.AssetStatus
	for rows.Next() {
		status, err := scanAssetStatusRow(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		statuses = append(statuses, status)
	}

	respondJSONOK(w, statuses)
}

// requireStatusSetID authenticates, parses the "id" param, and looks up the owning set_id.
// Returns the user, status ID, set ID, and ok. Writes 401/400/404/500 on failure.
func (h *AssetStatusHandler) requireStatusSetID(w http.ResponseWriter, r *http.Request) (user *models.User, statusID, setID int, ok bool) {
	user, ok = RequireAuth(w, r)
	if !ok {
		return nil, 0, 0, false
	}
	statusID, ok = requireIDParam(w, r, "id")
	if !ok {
		return nil, 0, 0, false
	}
	err := h.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", statusID).Scan(&setID)
	if err == sql.ErrNoRows {
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

	status, err := scanAssetStatusRow(h.db.QueryRow(`
		SELECT id, set_id, name, color, description, is_default, display_order, created_at, updated_at
		FROM asset_statuses
		WHERE id = ?
	`, statusID))
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

	var err error
	req, ok := decodeJSON[CreateAssetStatusRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Default color
	if req.Color == "" {
		req.Color = "#6b7280"
	}

	now := time.Now()

	// If this is marked as default, unset other defaults first
	if req.IsDefault {
		_, err = h.db.ExecWrite("UPDATE asset_statuses SET is_default = false WHERE set_id = ?", setID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	var statusID int64
	err = h.db.QueryRow(`
		INSERT INTO asset_statuses (set_id, name, color, description, is_default, display_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, setID, req.Name, req.Color, req.Description, req.IsDefault, req.DisplayOrder, now, now).Scan(&statusID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	id := int(statusID)
	logAudit(h.db, r, currentUser, logger.ActionAssetStatusCreate, logger.ResourceAssetStatus, &id, req.Name)

	status := models.AssetStatus{
		ID:           int(statusID),
		SetID:        setID,
		Name:         req.Name,
		Color:        req.Color,
		Description:  req.Description,
		IsDefault:    req.IsDefault,
		DisplayOrder: req.DisplayOrder,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

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

	now := time.Now()

	// If setting as default, unset other defaults first
	var err error
	if req.IsDefault != nil && *req.IsDefault {
		_, err = h.db.ExecWrite("UPDATE asset_statuses SET is_default = false WHERE set_id = ? AND id != ?", setID, statusID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Build update query
	query := "UPDATE asset_statuses SET name = ?, color = ?, description = ?, display_order = ?, updated_at = ?"
	args := []interface{}{req.Name, req.Color, req.Description, req.DisplayOrder, now}

	if req.IsDefault != nil {
		query += ", is_default = ?"
		args = append(args, *req.IsDefault)
	}

	query += " WHERE id = ?"
	args = append(args, statusID)

	result, err := h.db.ExecWrite(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "asset_status")
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetStatusUpdate, logger.ResourceAssetStatus, &statusID, req.Name)

	// Return updated status
	status, _ := scanAssetStatusRow(h.db.QueryRow(`
		SELECT id, set_id, name, color, description, is_default, display_order, created_at, updated_at
		FROM asset_statuses WHERE id = ?
	`, statusID))

	respondJSONOK(w, status)
}

// DeleteAssetStatus deletes an asset status
func (h *AssetStatusHandler) DeleteAssetStatus(w http.ResponseWriter, r *http.Request) {
	currentUser, statusID, _, ok := h.requireStatusAdminAccess(w, r)
	if !ok {
		return
	}

	// Prevent deletion if assets use this status
	var assetCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM assets WHERE status_id = ?", statusID).Scan(&assetCount)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if assetCount > 0 {
		respondConflict(w, r, "Cannot delete status with existing assets. Reassign assets first.")
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM asset_statuses WHERE id = ?", statusID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "asset_status")
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetStatusDelete, logger.ResourceAssetStatus, &statusID, "")

	w.WriteHeader(http.StatusNoContent)
}

// CreateDefaultStatuses creates default statuses for a new asset set.
func (h *AssetStatusHandler) CreateDefaultStatuses(setID int) error {
	return createDefaultAssetStatuses(h.db, setID)
}
