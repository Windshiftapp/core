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

// AssetTypeHandler handles asset type operations
type AssetTypeHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	assetHandler      *AssetHandler // Reuse permission checking methods
}

// NewAssetTypeHandler creates a new asset type handler
func NewAssetTypeHandler(db database.Database, permissionService *services.PermissionService) *AssetTypeHandler {
	return &AssetTypeHandler{
		db:                db,
		permissionService: permissionService,
		assetHandler:      NewAssetHandler(db, permissionService),
	}
}

// GetAssetTypes returns all asset types for a set
func (h *AssetTypeHandler) GetAssetTypes(w http.ResponseWriter, r *http.Request) {
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
		SELECT at.id, at.set_id, at.name, at.description, at.icon, at.color,
		       at.display_order, at.is_active, at.created_at, at.updated_at,
		       ams.name as set_name,
		       (SELECT COUNT(*) FROM assets WHERE asset_type_id = at.id) as asset_count
		FROM asset_types at
		LEFT JOIN asset_management_sets ams ON at.set_id = ams.id
		WHERE at.set_id = ?
		ORDER BY at.display_order, at.name
	`

	rows, err := h.db.Query(query, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var types []models.AssetType
	for rows.Next() {
		var assetType models.AssetType
		var description, setName sql.NullString

		err := rows.Scan(
			&assetType.ID, &assetType.SetID, &assetType.Name, &description,
			&assetType.Icon, &assetType.Color, &assetType.DisplayOrder,
			&assetType.IsActive, &assetType.CreatedAt, &assetType.UpdatedAt,
			&setName, &assetType.AssetCount,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if description.Valid {
			assetType.Description = description.String
		}
		if setName.Valid {
			assetType.SetName = setName.String
		}

		types = append(types, assetType)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(types)
}

// GetAssetType returns a single asset type
func (h *AssetTypeHandler) GetAssetType(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	typeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the type to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", typeID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_type")
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

	var assetType models.AssetType
	var description, setName sql.NullString

	err = h.db.QueryRow(`
		SELECT at.id, at.set_id, at.name, at.description, at.icon, at.color,
		       at.display_order, at.is_active, at.created_at, at.updated_at,
		       ams.name as set_name,
		       (SELECT COUNT(*) FROM assets WHERE asset_type_id = at.id) as asset_count
		FROM asset_types at
		LEFT JOIN asset_management_sets ams ON at.set_id = ams.id
		WHERE at.id = ?
	`, typeID).Scan(
		&assetType.ID, &assetType.SetID, &assetType.Name, &description,
		&assetType.Icon, &assetType.Color, &assetType.DisplayOrder,
		&assetType.IsActive, &assetType.CreatedAt, &assetType.UpdatedAt,
		&setName, &assetType.AssetCount,
	)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if description.Valid {
		assetType.Description = description.String
	}
	if setName.Valid {
		assetType.SetName = setName.String
	}

	// Get fields for this type
	assetType.Fields, err = h.getTypeFields(typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(assetType)
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

	// Check admin permission (only admins can create types)
	canAdmin, err := h.assetHandler.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondAdminRequired(w, r)
		return
	}

	var req CreateAssetTypeRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Default values
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

	var typeID int64
	err = h.db.QueryRow(`
		INSERT INTO asset_types (set_id, name, description, icon, color, display_order, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, setID, req.Name, req.Description, req.Icon, req.Color, req.DisplayOrder, isActive, now, now).Scan(&typeID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	assetType := models.AssetType{
		ID:           int(typeID),
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(assetType)
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
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	typeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the type to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", typeID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_type")
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

	var req UpdateAssetTypeRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	now := time.Now()

	// Build update query based on provided fields
	query := "UPDATE asset_types SET name = ?, description = ?, icon = ?, color = ?, display_order = ?, updated_at = ?"
	args := []interface{}{req.Name, req.Description, req.Icon, req.Color, req.DisplayOrder, now}

	if req.IsActive != nil {
		query += ", is_active = ?"
		args = append(args, *req.IsActive)
	}

	query += " WHERE id = ?"
	args = append(args, typeID)

	result, err := h.db.ExecWrite(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "asset_type")
		return
	}

	// Return updated type
	var assetType models.AssetType
	_ = h.db.QueryRow(`
		SELECT id, set_id, name, description, icon, color, display_order, is_active, created_at, updated_at
		FROM asset_types WHERE id = ?
	`, typeID).Scan(
		&assetType.ID, &assetType.SetID, &assetType.Name, &assetType.Description,
		&assetType.Icon, &assetType.Color, &assetType.DisplayOrder, &assetType.IsActive,
		&assetType.CreatedAt, &assetType.UpdatedAt,
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(assetType)
}

// DeleteAssetType deletes an asset type
func (h *AssetTypeHandler) DeleteAssetType(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	typeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the type to check set permissions and asset count
	var setID, assetCount int
	err = h.db.QueryRow(`
		SELECT set_id, (SELECT COUNT(*) FROM assets WHERE asset_type_id = ?) as asset_count
		FROM asset_types WHERE id = ?
	`, typeID, typeID).Scan(&setID, &assetCount)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_type")
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

	// Prevent deletion if assets exist
	if assetCount > 0 {
		respondConflict(w, r, "Cannot delete type with existing assets. Delete or reassign assets first.")
		return
	}

	// Delete type fields first
	_, err = h.db.ExecWrite("DELETE FROM asset_type_fields WHERE asset_type_id = ?", typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM asset_types WHERE id = ?", typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "asset_type")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTypeFields returns fields for an asset type
func (h *AssetTypeHandler) GetTypeFields(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	typeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the type to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", typeID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_type")
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

	fields, err := h.getTypeFields(typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fields)
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
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	typeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the type to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", typeID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_type")
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

	var req UpdateTypeFieldsRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing field assignments
	_, err = tx.Exec("DELETE FROM asset_type_fields WHERE asset_type_id = ?", typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Insert new field assignments
	now := time.Now()
	for _, field := range req.Fields {
		_, err = tx.Exec(`
			INSERT INTO asset_type_fields (asset_type_id, custom_field_id, is_required, display_order, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, typeID, field.CustomFieldID, field.IsRequired, field.DisplayOrder, now)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return updated fields
	fields, err := h.getTypeFields(typeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fields)
}

// getTypeFields is a helper to get fields for an asset type
func (h *AssetTypeHandler) getTypeFields(typeID int) ([]models.AssetTypeField, error) {
	rows, err := h.db.Query(`
		SELECT atf.id, atf.asset_type_id, atf.custom_field_id, atf.is_required, atf.display_order, atf.created_at,
		       cfd.name as field_name, cfd.field_type, cfd.description as field_description, cfd.options
		FROM asset_type_fields atf
		JOIN custom_field_definitions cfd ON atf.custom_field_id = cfd.id
		WHERE atf.asset_type_id = ?
		ORDER BY atf.display_order, cfd.name
	`, typeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var fields []models.AssetTypeField
	for rows.Next() {
		var field models.AssetTypeField
		var fieldDescription, options sql.NullString

		err := rows.Scan(
			&field.ID, &field.AssetTypeID, &field.CustomFieldID, &field.IsRequired,
			&field.DisplayOrder, &field.CreatedAt,
			&field.FieldName, &field.FieldType, &fieldDescription, &options,
		)
		if err != nil {
			return nil, err
		}

		if fieldDescription.Valid {
			field.FieldDescription = fieldDescription.String
		}
		if options.Valid {
			field.Options = options.String
		}

		fields = append(fields, field)
	}

	return fields, nil
}
