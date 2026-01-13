package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/database"
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
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		http.Error(w, "Invalid set ID", http.StatusBadRequest)
		return
	}

	// Check view permission
	canView, err := h.assetHandler.canViewSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Access denied", http.StatusForbidden)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var statuses []models.AssetStatus
	for rows.Next() {
		var status models.AssetStatus
		var description sql.NullString

		err := rows.Scan(
			&status.ID, &status.SetID, &status.Name, &status.Color, &description,
			&status.IsDefault, &status.DisplayOrder, &status.CreatedAt, &status.UpdatedAt,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if description.Valid {
			status.Description = description.String
		}

		statuses = append(statuses, status)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statuses)
}

// GetAssetStatus returns a single asset status
func (h *AssetStatusHandler) GetAssetStatus(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	statusID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid status ID", http.StatusBadRequest)
		return
	}

	// Get the status to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", statusID).Scan(&setID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset status not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check view permission
	canView, err := h.assetHandler.canViewSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Access denied", http.StatusForbidden)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if description.Valid {
		status.Description = description.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
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
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		http.Error(w, "Invalid set ID", http.StatusBadRequest)
		return
	}

	// Check admin permission
	canAdmin, err := h.assetHandler.canAdminSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canAdmin {
		http.Error(w, "Admin permission required", http.StatusForbidden)
		return
	}

	var req CreateAssetStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	var statusID int64
	err = h.db.QueryRow(`
		INSERT INTO asset_statuses (set_id, name, color, description, is_default, display_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, setID, req.Name, req.Color, req.Description, req.IsDefault, req.DisplayOrder, now, now).Scan(&statusID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
	json.NewEncoder(w).Encode(status)
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
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	statusID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid status ID", http.StatusBadRequest)
		return
	}

	// Get the status to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", statusID).Scan(&setID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset status not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check admin permission
	canAdmin, err := h.assetHandler.canAdminSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canAdmin {
		http.Error(w, "Admin permission required", http.StatusForbidden)
		return
	}

	var req UpdateAssetStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	now := time.Now()

	// If setting as default, unset other defaults first
	if req.IsDefault != nil && *req.IsDefault {
		_, err = h.db.ExecWrite("UPDATE asset_statuses SET is_default = false WHERE set_id = ? AND id != ?", setID, statusID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Asset status not found", http.StatusNotFound)
		return
	}

	// Return updated status
	var status models.AssetStatus
	var description sql.NullString
	h.db.QueryRow(`
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
	json.NewEncoder(w).Encode(status)
}

// DeleteAssetStatus deletes an asset status
func (h *AssetStatusHandler) DeleteAssetStatus(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	statusID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid status ID", http.StatusBadRequest)
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
		http.Error(w, "Asset status not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check admin permission
	canAdmin, err := h.assetHandler.canAdminSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canAdmin {
		http.Error(w, "Admin permission required", http.StatusForbidden)
		return
	}

	// Prevent deletion if assets use this status
	if assetCount > 0 {
		http.Error(w, "Cannot delete status with existing assets. Reassign assets first.", http.StatusConflict)
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM asset_statuses WHERE id = ?", statusID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Asset status not found", http.StatusNotFound)
		return
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
