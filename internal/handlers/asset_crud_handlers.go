package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/cql"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// GetAssets returns all assets in a set with pagination and subcategory support
func (h *AssetHandler) GetAssets(w http.ResponseWriter, r *http.Request) {
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
	canView, err := h.canViewSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	// Parse pagination parameters
	limit := 25
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Build WHERE clause and args (shared between count and main query)
	whereClause := "WHERE a.set_id = ?"
	args := []interface{}{setID}
	ctePrefix := ""

	// Add filters
	if typeID := r.URL.Query().Get("type_id"); typeID != "" {
		whereClause += " AND a.asset_type_id = ?"
		args = append(args, typeID)
	}

	// Category filter with optional subcategory inclusion
	if categoryIDStr := r.URL.Query().Get("category_id"); categoryIDStr != "" {
		includeSubcats := r.URL.Query().Get("include_subcategories") != "false"
		if includeSubcats {
			// Use recursive CTE to get category and all descendants
			ctePrefix = `WITH RECURSIVE category_tree AS (
				SELECT id FROM asset_categories WHERE id = ?
				UNION ALL
				SELECT ac.id FROM asset_categories ac
				INNER JOIN category_tree ct ON ac.parent_id = ct.id
			) `
			whereClause += " AND a.category_id IN (SELECT id FROM category_tree)"
			// Prepend categoryID to args since CTE comes first
			args = append([]interface{}{categoryIDStr}, args...)
		} else {
			whereClause += " AND a.category_id = ?"
			args = append(args, categoryIDStr)
		}
	}

	if statusID := r.URL.Query().Get("status_id"); statusID != "" {
		whereClause += " AND a.status_id = ?"
		args = append(args, statusID)
	}

	if search := r.URL.Query().Get("search"); search != "" {
		whereClause += " AND (a.title LIKE ? OR a.description LIKE ? OR a.asset_tag LIKE ?)"
		searchTerm := "%" + search + "%"
		args = append(args, searchTerm, searchTerm, searchTerm)
	}

	// Check for CQL query parameter
	if cqlQuery := r.URL.Query().Get("cql"); cqlQuery != "" {
		// Build set mapping for CQL evaluation
		setMap, err := h.buildSetMap()
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load set mapping: %w", err))
			return
		}

		// Build workspace mapping for linkedOf() queries
		workspaceMap, err := h.buildWorkspaceMap()
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load workspace mapping: %w", err))
			return
		}

		// Create CQL evaluator and generate SQL
		evaluator := cql.NewAssetEvaluator(setMap, workspaceMap)
		cqlSQL, cqlArgs, err := evaluator.EvaluateToSQL(cqlQuery)
		if err != nil {
			respondValidationError(w, r, "CQL query error: "+err.Error())
			return
		}

		if cqlSQL != "" {
			whereClause += " AND (" + cqlSQL + ")"
			args = append(args, cqlArgs...)
		}
	}

	// Get total count first (include JOINs for CQL field references)
	countQuery := ctePrefix + `SELECT COUNT(*) FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		` + whereClause
	var total int
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Build main query
	query := ctePrefix + `
		SELECT a.id, a.set_id, a.asset_type_id, a.category_id, a.status_id, a.title, a.description,
		       a.asset_tag, a.custom_field_values, a.frac_index,
		       a.created_by, a.created_at, a.updated_at,
		       ams.name as set_name,
		       at.name as asset_type_name, at.icon as asset_type_icon, at.color as asset_type_color,
		       ac.name as category_name, ac.path as category_path,
		       ast.name as status_name, ast.color as status_color,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       u.email as creator_email,
		       (SELECT COUNT(*) FROM item_links WHERE (source_type = 'asset' AND source_id = a.id) OR (target_type = 'asset' AND target_id = a.id)) as linked_item_count
		FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		` + whereClause + `
		ORDER BY a.frac_index, a.title
		LIMIT ? OFFSET ?
	`
	// Add pagination args
	queryArgs := append(args, limit, offset)

	rows, err := h.db.Query(query, queryArgs...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	var assets []models.Asset
	for rows.Next() {
		var asset models.Asset
		var description, assetTag, customFieldValuesJSON, fracIndex sql.NullString
		var categoryID, statusID sql.NullInt64
		var setName, assetTypeName, assetTypeIcon, assetTypeColor sql.NullString
		var categoryName, categoryPath, statusName, statusColor sql.NullString
		var creatorName, creatorEmail sql.NullString

		err := rows.Scan(
			&asset.ID, &asset.SetID, &asset.AssetTypeID, &categoryID, &statusID, &asset.Title, &description,
			&assetTag, &customFieldValuesJSON, &fracIndex,
			&asset.CreatedBy, &asset.CreatedAt, &asset.UpdatedAt,
			&setName, &assetTypeName, &assetTypeIcon, &assetTypeColor,
			&categoryName, &categoryPath, &statusName, &statusColor,
			&creatorName, &creatorEmail, &asset.LinkedItemCount,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		asset.CategoryID = utils.NullInt64ToPtr(categoryID)
		asset.StatusID = utils.NullInt64ToPtr(statusID)
		asset.Description = description.String
		asset.AssetTag = assetTag.String
		asset.FracIndex = utils.NullStringToPtr(fracIndex)
		asset.SetName = setName.String
		asset.AssetTypeName = assetTypeName.String
		asset.AssetTypeIcon = assetTypeIcon.String
		asset.AssetTypeColor = assetTypeColor.String
		asset.CategoryName = categoryName.String
		asset.CategoryPath = categoryPath.String
		asset.StatusName = statusName.String
		asset.StatusColor = statusColor.String
		asset.CreatorName = creatorName.String
		asset.CreatorEmail = creatorEmail.String

		// Deserialize custom field values
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &asset.CustomFieldValues); err != nil {
				asset.CustomFieldValues = make(map[string]interface{})
			}
		}

		assets = append(assets, asset)
	}

	// Enrich user-type custom fields with current user data
	for i := range assets {
		if err := h.enrichUserCustomFields(&assets[i]); err != nil {
			// Log error but don't fail the request
			continue
		}
	}

	// Return paginated response
	response := map[string]interface{}{
		"assets": assets,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetAsset returns a single asset
func (h *AssetHandler) GetAsset(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// First get the asset to check set permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check view permission
	canView, err := h.canViewSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	var asset models.Asset
	var description, assetTag, customFieldValuesJSON, fracIndex sql.NullString
	var categoryID, statusID sql.NullInt64
	var setName, assetTypeName, assetTypeIcon, assetTypeColor sql.NullString
	var categoryName, categoryPath, statusName, statusColor sql.NullString
	var creatorName, creatorEmail sql.NullString

	err = h.db.QueryRow(`
		SELECT a.id, a.set_id, a.asset_type_id, a.category_id, a.status_id, a.title, a.description,
		       a.asset_tag, a.custom_field_values, a.frac_index,
		       a.created_by, a.created_at, a.updated_at,
		       ams.name as set_name,
		       at.name as asset_type_name, at.icon as asset_type_icon, at.color as asset_type_color,
		       ac.name as category_name, ac.path as category_path,
		       ast.name as status_name, ast.color as status_color,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       u.email as creator_email,
		       (SELECT COUNT(*) FROM item_links WHERE (source_type = 'asset' AND source_id = a.id) OR (target_type = 'asset' AND target_id = a.id)) as linked_item_count
		FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		WHERE a.id = ?
	`, assetID).Scan(
		&asset.ID, &asset.SetID, &asset.AssetTypeID, &categoryID, &statusID, &asset.Title, &description,
		&assetTag, &customFieldValuesJSON, &fracIndex,
		&asset.CreatedBy, &asset.CreatedAt, &asset.UpdatedAt,
		&setName, &assetTypeName, &assetTypeIcon, &assetTypeColor,
		&categoryName, &categoryPath, &statusName, &statusColor,
		&creatorName, &creatorEmail, &asset.LinkedItemCount,
	)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	asset.CategoryID = utils.NullInt64ToPtr(categoryID)
	asset.StatusID = utils.NullInt64ToPtr(statusID)
	asset.Description = description.String
	asset.AssetTag = assetTag.String
	asset.FracIndex = utils.NullStringToPtr(fracIndex)
	asset.SetName = setName.String
	asset.AssetTypeName = assetTypeName.String
	asset.AssetTypeIcon = assetTypeIcon.String
	asset.AssetTypeColor = assetTypeColor.String
	asset.CategoryName = categoryName.String
	asset.CategoryPath = categoryPath.String
	asset.StatusName = statusName.String
	asset.StatusColor = statusColor.String
	asset.CreatorName = creatorName.String
	asset.CreatorEmail = creatorEmail.String

	// Deserialize custom field values
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &asset.CustomFieldValues); err != nil {
			asset.CustomFieldValues = make(map[string]interface{})
		}
	}

	// Enrich user-type custom fields with current user data
	if err := h.enrichUserCustomFields(&asset); err != nil {
		// Log error but don't fail the request
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(asset)
}

// CreateAssetRequest represents the request body for creating an asset
type CreateAssetRequest struct {
	AssetTypeID       int                    `json:"asset_type_id"`
	CategoryID        *int                   `json:"category_id,omitempty"`
	StatusID          *int                   `json:"status_id,omitempty"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	AssetTag          string                 `json:"asset_tag,omitempty"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
}

// CreateAsset creates a new asset
func (h *AssetHandler) CreateAsset(w http.ResponseWriter, r *http.Request) {
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

	// Check edit permission
	canEdit, err := h.canEditSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canEdit {
		respondForbidden(w, r)
		return
	}

	var req CreateAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Title == "" {
		respondValidationError(w, r, "Title is required")
		return
	}

	if req.AssetTypeID == 0 {
		respondValidationError(w, r, "Asset type is required")
		return
	}

	// Validate asset type belongs to this set
	var typeSetID int
	err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", req.AssetTypeID).Scan(&typeSetID)
	if err == sql.ErrNoRows {
		respondValidationError(w, r, "Asset type not found")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if typeSetID != setID {
		respondValidationError(w, r, "Asset type does not belong to this set")
		return
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.SanitizeDescription(req.Description)

	// Validate category if provided
	if req.CategoryID != nil {
		var catSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", *req.CategoryID).Scan(&catSetID)
		if err == sql.ErrNoRows {
			respondValidationError(w, r, "Category not found")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if catSetID != setID {
			respondValidationError(w, r, "Category does not belong to this set")
			return
		}
	}

	// Handle status_id - get default if not provided
	var statusID *int
	if req.StatusID != nil {
		// Validate status belongs to this set
		var statusSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", *req.StatusID).Scan(&statusSetID)
		if err == sql.ErrNoRows {
			respondValidationError(w, r, "Status not found")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if statusSetID != setID {
			respondValidationError(w, r, "Status does not belong to this set")
			return
		}
		statusID = req.StatusID
	} else {
		// Get default status for this set
		var defaultStatusID int
		err = h.db.QueryRow("SELECT id FROM asset_statuses WHERE set_id = ? AND is_default = true LIMIT 1", setID).Scan(&defaultStatusID)
		if err == nil {
			statusID = &defaultStatusID
		}
		// If no default status found, statusID will be nil which is okay
	}

	now := time.Now()

	// Normalize user-type custom field values to store just the ID
	if req.CustomFieldValues != nil {
		if err := h.normalizeUserFieldValues(req.CustomFieldValues, req.AssetTypeID); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to process custom field values: %w", err))
			return
		}
	}

	// Serialize custom field values
	var customFieldValuesJSON string
	if req.CustomFieldValues != nil {
		customFieldValuesBytes, err := json.Marshal(req.CustomFieldValues)
		if err != nil {
			respondValidationError(w, r, "Invalid custom field values")
			return
		}
		customFieldValuesJSON = string(customFieldValuesBytes)
	}

	var assetID int64
	err = h.db.QueryRow(`
		INSERT INTO assets (set_id, asset_type_id, category_id, status_id, title, description, asset_tag, custom_field_values, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, setID, req.AssetTypeID, req.CategoryID, statusID, req.Title, req.Description, req.AssetTag, customFieldValuesJSON, currentUser.ID, now, now).Scan(&assetID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return created asset
	asset := models.Asset{
		ID:                int(assetID),
		SetID:             setID,
		AssetTypeID:       req.AssetTypeID,
		CategoryID:        req.CategoryID,
		StatusID:          statusID,
		Title:             req.Title,
		Description:       req.Description,
		AssetTag:          req.AssetTag,
		CustomFieldValues: req.CustomFieldValues,
		CreatedBy:         &currentUser.ID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(asset)
}

// UpdateAssetRequest represents the request body for updating an asset
type UpdateAssetRequest struct {
	AssetTypeID       int                    `json:"asset_type_id"`
	CategoryID        *int                   `json:"category_id,omitempty"`
	StatusID          *int                   `json:"status_id,omitempty"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	AssetTag          string                 `json:"asset_tag,omitempty"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
}

// UpdateAsset updates an existing asset
func (h *AssetHandler) UpdateAsset(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get asset to check permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check edit permission
	canEdit, err := h.canEditSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canEdit {
		respondForbidden(w, r)
		return
	}

	var req UpdateAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Title == "" {
		respondValidationError(w, r, "Title is required")
		return
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.SanitizeDescription(req.Description)

	// Validate asset type if changing
	if req.AssetTypeID != 0 {
		var typeSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", req.AssetTypeID).Scan(&typeSetID)
		if err == sql.ErrNoRows {
			respondValidationError(w, r, "Asset type not found")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if typeSetID != setID {
			respondValidationError(w, r, "Asset type does not belong to this set")
			return
		}
	}

	// Validate category if provided
	if req.CategoryID != nil {
		var catSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_categories WHERE id = ?", *req.CategoryID).Scan(&catSetID)
		if err == sql.ErrNoRows {
			respondValidationError(w, r, "Category not found")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if catSetID != setID {
			respondValidationError(w, r, "Category does not belong to this set")
			return
		}
	}

	// Validate status_id if provided
	if req.StatusID != nil {
		var statusSetID int
		err = h.db.QueryRow("SELECT set_id FROM asset_statuses WHERE id = ?", *req.StatusID).Scan(&statusSetID)
		if err == sql.ErrNoRows {
			respondValidationError(w, r, "Status not found")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if statusSetID != setID {
			respondValidationError(w, r, "Status does not belong to this set")
			return
		}
	}

	now := time.Now()

	// Normalize user-type custom field values to store just the ID
	if req.CustomFieldValues != nil {
		if err := h.normalizeUserFieldValues(req.CustomFieldValues, req.AssetTypeID); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to process custom field values: %w", err))
			return
		}
	}

	// Serialize custom field values
	var customFieldValuesJSON string
	if req.CustomFieldValues != nil {
		customFieldValuesBytes, err := json.Marshal(req.CustomFieldValues)
		if err != nil {
			respondValidationError(w, r, "Invalid custom field values")
			return
		}
		customFieldValuesJSON = string(customFieldValuesBytes)
	}

	result, err := h.db.ExecWrite(`
		UPDATE assets
		SET asset_type_id = ?, category_id = ?, status_id = ?, title = ?, description = ?,
		    asset_tag = ?, custom_field_values = ?, updated_at = ?
		WHERE id = ?
	`, req.AssetTypeID, req.CategoryID, req.StatusID, req.Title, req.Description, req.AssetTag, customFieldValuesJSON, now, assetID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "asset")
		return
	}

	// Return updated asset
	asset := models.Asset{
		ID:                assetID,
		SetID:             setID,
		AssetTypeID:       req.AssetTypeID,
		CategoryID:        req.CategoryID,
		StatusID:          req.StatusID,
		Title:             req.Title,
		Description:       req.Description,
		AssetTag:          req.AssetTag,
		CustomFieldValues: req.CustomFieldValues,
		UpdatedAt:         now,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(asset)
}

// DeleteAsset deletes an asset
func (h *AssetHandler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get asset to check permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check edit permission (edit permission allows delete)
	canEdit, err := h.canEditSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canEdit {
		respondForbidden(w, r)
		return
	}

	// Delete related links first
	_, err = h.db.ExecWrite("DELETE FROM item_links WHERE (source_type = 'asset' AND source_id = ?) OR (target_type = 'asset' AND target_id = ?)", assetID, assetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM assets WHERE id = ?", assetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "asset")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
