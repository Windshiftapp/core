package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
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
		assetHandler:      NewAssetHandler(db, permissionService, ""),
	}
}

// GetAssetTypes returns all asset types for a set
func (h *AssetTypeHandler) GetAssetTypes(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.assetHandler.requireSetViewAccess(w, r)
	if !ok {
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
		assetType, err := scanAssetType(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		types = append(types, assetType)
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

	err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", typeID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_type")
		return 0, 0, nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return 0, 0, nil, false
	}

	return typeID, setID, user, true
}

// scanAssetType scans a full asset type row (with nullable description and set_name)
// from any scanner (sql.Row or sql.Rows). The column order must match the standard
// asset-type SELECT: id, set_id, name, description, icon, color, display_order,
// is_active, created_at, updated_at, set_name, asset_count.
func scanAssetType(scanner interface {
	Scan(dest ...interface{}) error
}) (models.AssetType, error) {
	var at models.AssetType
	var description, setName sql.NullString

	err := scanner.Scan(
		&at.ID, &at.SetID, &at.Name, &description,
		&at.Icon, &at.Color, &at.DisplayOrder,
		&at.IsActive, &at.CreatedAt, &at.UpdatedAt,
		&setName, &at.AssetCount,
	)
	if err != nil {
		return at, err
	}

	if description.Valid {
		at.Description = description.String
	}
	if setName.Valid {
		at.SetName = setName.String
	}
	return at, nil
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
	typeID, setID, currentUser, ok := h.requireAssetTypeAccess(w, r)
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

	assetType, err := scanAssetType(h.db.QueryRow(`
		SELECT at.id, at.set_id, at.name, at.description, at.icon, at.color,
		       at.display_order, at.is_active, at.created_at, at.updated_at,
		       ams.name as set_name,
		       (SELECT COUNT(*) FROM assets WHERE asset_type_id = at.id) as asset_count
		FROM asset_types at
		LEFT JOIN asset_management_sets ams ON at.set_id = ams.id
		WHERE at.id = ?
	`, typeID))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get fields for this type
	assetType.Fields, err = h.getTypeFields(typeID)
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
	var err error

	req, ok := decodeJSON[CreateAssetTypeRequest](w, r)
	if !ok {
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

	id := int(typeID)
	logAudit(h.db, r, currentUser, logger.ActionAssetTypeCreate, logger.ResourceAssetType, &id, req.Name)

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

	logAudit(h.db, r, currentUser, logger.ActionAssetTypeUpdate, logger.ResourceAssetType, &typeID, req.Name)

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

	// Get the type to check set permissions and asset count
	var setID, assetCount int
	err := h.db.QueryRow(`
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

	logAudit(h.db, r, currentUser, logger.ActionAssetTypeDelete, logger.ResourceAssetType, &typeID, "")

	w.WriteHeader(http.StatusNoContent)
}

// GetTypeFields returns fields for an asset type
func (h *AssetTypeHandler) GetTypeFields(w http.ResponseWriter, r *http.Request) {
	typeID, setID, currentUser, ok := h.requireAssetTypeAccess(w, r)
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

	fields, err := h.getTypeFields(typeID)
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

	respondJSONOK(w, fields)
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
