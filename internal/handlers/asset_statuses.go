package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

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
		assetHandler:      NewAssetHandler(db, permissionService),
	}
}

// GetAssetStatuses returns all asset statuses for a set
func (h *AssetStatusHandler) GetAssetStatuses(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	// Check view permission
	canView, err := h.assetHandler.canViewSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondForbidden(w, r)
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
		var status models.AssetStatus
		var description sql.NullString

		err := rows.Scan(
			&status.ID, &status.SetID, &status.Name, &status.Color, &description,
			&status.IsDefault, &status.DisplayOrder, &status.CreatedAt, &status.UpdatedAt,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if description.Valid {
			status.Description = description.String
		}

		statuses = append(statuses, status)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(statuses)
}

// GetAssetStatus returns a single asset status
func (h *AssetStatusHandler) GetAssetStatus(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	statusID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the status to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", statusID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_status")
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
		respondForbidden(w, r)
		return
	}

	var status models.AssetStatus
	var description sql.NullString

	err = h.db.QueryRow(`
		SELECT id, set_id, name, color, description, is_default, display_order, created_at, updated_at
		FROM asset_statuses
		WHERE id = ?
	`, statusID).Scan(
		&status.ID, &status.SetID, &status.Name, &status.Color, &description,
		&status.IsDefault, &status.DisplayOrder, &status.CreatedAt, &status.UpdatedAt,
	)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if description.Valid {
		status.Description = description.String
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
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
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	// Check admin permission
	canAdmin, err := h.assetHandler.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondAdminRequired(w, r)
		return
	}

	var req CreateAssetStatusRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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

	if currentUser != nil {
		id := int(statusID)
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionAssetStatusCreate,
			ResourceType: logger.ResourceAssetStatus,
			ResourceID:   &id,
			ResourceName: req.Name,
			Success:      true,
		})
	}

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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(status)
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
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	statusID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the status to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", statusID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_status")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check admin permission
	canAdmin, err := h.assetHandler.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondAdminRequired(w, r)
		return
	}

	var req UpdateAssetStatusRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	now := time.Now()

	// If setting as default, unset other defaults first
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

	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionAssetStatusUpdate,
			ResourceType: logger.ResourceAssetStatus,
			ResourceID:   &statusID,
			ResourceName: req.Name,
			Success:      true,
		})
	}

	// Return updated status
	var status models.AssetStatus
	var description sql.NullString
	_ = h.db.QueryRow(`
		SELECT id, set_id, name, color, description, is_default, display_order, created_at, updated_at
		FROM asset_statuses WHERE id = ?
	`, statusID).Scan(
		&status.ID, &status.SetID, &status.Name, &status.Color, &description,
		&status.IsDefault, &status.DisplayOrder, &status.CreatedAt, &status.UpdatedAt,
	)
	if description.Valid {
		status.Description = description.String
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// DeleteAssetStatus deletes an asset status
func (h *AssetStatusHandler) DeleteAssetStatus(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	statusID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the status to check set permissions and asset count
	var setID int
	var assetCount int
	err = h.db.QueryRow(`
		SELECT set_id, (SELECT COUNT(*) FROM assets WHERE status_id = ?) as asset_count
		FROM asset_statuses WHERE id = ?
	`, statusID, statusID).Scan(&setID, &assetCount)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_status")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check admin permission
	canAdmin, err := h.assetHandler.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondAdminRequired(w, r)
		return
	}

	// Prevent deletion if assets use this status
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

	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionAssetStatusDelete,
			ResourceType: logger.ResourceAssetStatus,
			ResourceID:   &statusID,
			Success:      true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// CreateDefaultStatuses creates default statuses for a new asset set
func (h *AssetStatusHandler) CreateDefaultStatuses(setID int) error {
	now := time.Now()
	defaultStatuses := []struct {
		Name         string
		Color        string
		IsDefault    bool
		DisplayOrder int
	}{
		{"Active", "#22c55e", true, 0},
		{"Inactive", "#6b7280", false, 1},
		{"Maintenance", "#f59e0b", false, 2},
		{"Retired", "#ef4444", false, 3},
	}

	for _, s := range defaultStatuses {
		_, err := h.db.ExecWrite(`
			INSERT INTO asset_statuses (set_id, name, color, is_default, display_order, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, setID, s.Name, s.Color, s.IsDefault, s.DisplayOrder, now, now)
		if err != nil {
			return err
		}
	}

	return nil
}
