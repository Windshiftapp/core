package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// validateResourceBelongsToSet checks that a resource (by table name) with resourceID belongs to setID.
// Returns true if valid; writes an error response and returns false otherwise.
func (h *AssetHandler) validateResourceBelongsToSet(w http.ResponseWriter, r *http.Request, table string, resourceID, setID int, resourceName string) bool {
	var resSetID int
	err := h.db.QueryRow("SELECT set_id FROM "+table+" WHERE id = ?", resourceID).Scan(&resSetID)
	if err == sql.ErrNoRows {
		respondValidationError(w, r, resourceName+" not found")
		return false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if resSetID != setID {
		respondValidationError(w, r, resourceName+" does not belong to this set")
		return false
	}
	return true
}

// serializeCustomFields normalizes user-type fields and marshals custom field values to JSON.
// Returns (serialized *string, ok bool). Writes error response on failure.
func (h *AssetHandler) serializeCustomFields(w http.ResponseWriter, r *http.Request, customFieldValues map[string]interface{}, assetTypeID int) (*string, bool) {
	if customFieldValues == nil {
		return nil, true
	}
	if err := h.normalizeUserFieldValues(customFieldValues, assetTypeID); err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to process custom field values: %w", err))
		return nil, false
	}
	b, err := json.Marshal(customFieldValues)
	if err != nil {
		respondValidationError(w, r, "Invalid custom field values")
		return nil, false
	}
	s := string(b)
	return &s, true
}

// scanAssetRow scans a single asset row with all joined fields.
func scanAssetRow(s interface{ Scan(...interface{}) error }) (models.Asset, error) {
	var asset models.Asset
	var description, assetTag, customFieldValuesJSON, fracIndex sql.NullString
	var categoryID, statusID sql.NullInt64
	var setName, assetTypeName, assetTypeIcon, assetTypeColor sql.NullString
	var categoryName, categoryPath, statusName, statusColor sql.NullString
	var creatorName, creatorEmail sql.NullString

	err := s.Scan(
		&asset.ID, &asset.SetID, &asset.AssetTypeID, &categoryID, &statusID, &asset.Title, &description,
		&assetTag, &customFieldValuesJSON, &fracIndex,
		&asset.CreatedBy, &asset.CreatedAt, &asset.UpdatedAt,
		&setName, &assetTypeName, &assetTypeIcon, &assetTypeColor,
		&categoryName, &categoryPath, &statusName, &statusColor,
		&creatorName, &creatorEmail, &asset.LinkedItemCount,
	)
	if err != nil {
		return asset, err
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

	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &asset.CustomFieldValues); err != nil {
			slog.Error("failed to unmarshal asset custom_field_values",
				slog.Int("asset_id", asset.ID),
				slog.String("raw", customFieldValuesJSON.String),
				slog.Any("error", err))
			asset.CustomFieldValues = make(map[string]interface{})
			asset.Warnings = append(asset.Warnings, "custom field values could not be parsed")
		}
	}

	return asset, nil
}

// GetAssets returns all assets in a set with pagination and subcategory support
func (h *AssetHandler) GetAssets(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.requireSetViewAccess(w, r)
	if !ok {
		return
	}

	limit, offset := parseOffsetPagination(r, 25, 10000)

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
	if cqlQuery := r.URL.Query().Get("ql"); cqlQuery != "" {
		setMap, err := buildAssetCQLSetMap(h.db)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load set mapping: %w", err))
			return
		}

		workspaceMap, err := buildAssetCQLWorkspaceMap(h.db)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load workspace mapping: %w", err))
			return
		}

		customFieldMap, err := buildAssetCQLCustomFieldMap(h.db, setID)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load custom field mapping: %w", err))
			return
		}

		evaluator := cql.NewAssetEvaluator(setMap, workspaceMap, customFieldMap, h.db.GetDriverName())
		cqlSQL, cqlArgs, err := evaluator.EvaluateToSQL(cqlQuery)
		if err != nil {
			respondValidationError(w, r, "CQL query error: "+err.Error())
			return
		}

		if cqlSQL != "" {
			whereClause += " AND (" + cqlSQL + ")"
			args = append(args, cqlArgs...)
		}

		slog.Debug("asset query CQL",
			slog.String("cql", cqlQuery),
			slog.String("sql", cqlSQL),
			slog.Any("args", cqlArgs))
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
		slog.Error("asset count query failed",
			slog.String("query", countQuery),
			slog.Any("args", args),
			slog.Any("error", err))
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
	args = append(args, limit, offset)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		slog.Error("asset list query failed",
			slog.String("query", query),
			slog.Any("args", args),
			slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var assets []models.Asset
	for rows.Next() {
		asset, err := scanAssetRow(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return
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

	respondJSONOK(w, response)
}

// GetAsset returns a single asset
// loadFullAsset fetches a single asset with all joined/enriched fields,
// matching the shape returned by GetAsset. Shared by GET and PUT so clients
// see a consistent payload after create/update.
func (h *AssetHandler) loadFullAsset(assetID int) (models.Asset, error) {
	asset, err := scanAssetRow(h.db.QueryRow(`
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
	`, assetID))
	if err != nil {
		return asset, err
	}
	if err := h.enrichUserCustomFields(&asset); err != nil {
		slog.Debug("failed to enrich user custom fields", slog.Any("error", err))
	}
	if err := h.enrichAssetRefCustomFields(&asset); err != nil {
		slog.Debug("failed to enrich asset-ref custom fields", slog.Any("error", err))
	}
	return asset, nil
}

func (h *AssetHandler) GetAsset(w http.ResponseWriter, r *http.Request) {
	_, assetID, ok := h.requireAssetViewAccess(w, r)
	if !ok {
		return
	}

	asset, err := h.loadFullAsset(assetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, asset)
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
	currentUser, setID, ok := h.requireSetEditAccess(w, r)
	if !ok {
		return
	}
	var err error

	req, ok := decodeJSON[CreateAssetRequest](w, r)
	if !ok {
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		respondValidationError(w, r, "Title is required")
		return
	}

	if req.AssetTypeID == 0 {
		respondValidationError(w, r, "Asset type is required")
		return
	}

	if !h.validateResourceBelongsToSet(w, r, "asset_types", req.AssetTypeID, setID, "Asset type") {
		return
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.SanitizeDescription(req.Description)

	if req.CategoryID != nil {
		if !h.validateResourceBelongsToSet(w, r, "asset_categories", *req.CategoryID, setID, "Category") {
			return
		}
	}

	// Handle status_id - get default if not provided
	var statusID *int
	if req.StatusID != nil {
		if !h.validateResourceBelongsToSet(w, r, "asset_statuses", *req.StatusID, setID, "Status") {
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

	customFieldValuesJSON, ok := h.serializeCustomFields(w, r, req.CustomFieldValues, req.AssetTypeID)
	if !ok {
		return
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

	id := int(assetID)
	logAudit(h.db, r, currentUser, logger.ActionAssetCreate, logger.ResourceAsset, &id, req.Title)

	// Emit asset action event for automation
	if h.assetActionService != nil {
		h.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
			EventType:   models.AssetTriggerAssetCreated,
			SetID:       setID,
			AssetID:     id,
			ActorUserID: currentUser.ID,
			NewValues: map[string]interface{}{
				"title":         req.Title,
				"asset_type_id": req.AssetTypeID,
				"status_id":     statusID,
			},
		})
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

	respondJSONCreated(w, asset)
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
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	assetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Get asset to check permissions and capture old values for event emission
	var setID int
	var oldStatusID sql.NullInt64
	var oldAssetTypeID int
	err = h.db.QueryRow("SELECT set_id, status_id, asset_type_id FROM assets WHERE id = ?", assetID).Scan(&setID, &oldStatusID, &oldAssetTypeID)
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
		respondNotFound(w, r, "asset")
		return
	}

	req, ok := decodeJSON[UpdateAssetRequest](w, r)
	if !ok {
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		respondValidationError(w, r, "Title is required")
		return
	}

	if req.AssetTypeID <= 0 {
		respondValidationError(w, r, "asset_type_id is required")
		return
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.SanitizeDescription(req.Description)

	if !h.validateResourceBelongsToSet(w, r, "asset_types", req.AssetTypeID, setID, "Asset type") {
		return
	}

	if req.CategoryID != nil {
		if !h.validateResourceBelongsToSet(w, r, "asset_categories", *req.CategoryID, setID, "Category") {
			return
		}
	}

	if req.StatusID != nil {
		if !h.validateResourceBelongsToSet(w, r, "asset_statuses", *req.StatusID, setID, "Status") {
			return
		}
	}

	now := time.Now()

	customFieldValuesJSON, ok := h.serializeCustomFields(w, r, req.CustomFieldValues, req.AssetTypeID)
	if !ok {
		return
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

	logAudit(h.db, r, currentUser, logger.ActionAssetUpdate, logger.ResourceAsset, &assetID, req.Title)

	// Emit asset action events for automation
	if h.assetActionService != nil {
		// Determine if status changed
		oldSID := 0
		if oldStatusID.Valid {
			oldSID = int(oldStatusID.Int64)
		}
		newSID := 0
		if req.StatusID != nil {
			newSID = *req.StatusID
		}
		statusChanged := oldSID != newSID

		if statusChanged {
			h.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
				EventType:   models.AssetTriggerAssetStatusChanged,
				SetID:       setID,
				AssetID:     assetID,
				ActorUserID: currentUser.ID,
				OldValues:   map[string]interface{}{"status_id": oldSID},
				NewValues:   map[string]interface{}{"status_id": newSID},
			})
		}

		h.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
			EventType:   models.AssetTriggerAssetUpdated,
			SetID:       setID,
			AssetID:     assetID,
			ActorUserID: currentUser.ID,
			NewValues: map[string]interface{}{
				"title":         req.Title,
				"asset_type_id": req.AssetTypeID,
				"status_id":     req.StatusID,
			},
		})
	}

	asset, err := h.loadFullAsset(assetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, asset)
}

// DeleteAsset deletes an asset
func (h *AssetHandler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	currentUser, assetID, ok := h.requireAssetEditAccess(w, r)
	if !ok {
		return
	}

	var setID int
	var title string
	err := h.db.QueryRow("SELECT set_id, title FROM assets WHERE id = ?", assetID).Scan(&setID, &title)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	err = database.WithTx(h.db, func(tx database.Tx) error {
		if _, err := tx.Exec("DELETE FROM item_links WHERE (source_type = 'asset' AND source_id = ?) OR (target_type = 'asset' AND target_id = ?)", assetID, assetID); err != nil {
			return err
		}
		result, err := tx.Exec("DELETE FROM assets WHERE id = ?", assetID)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetDelete, logger.ResourceAsset, &assetID, title)

	if h.assetActionService != nil {
		h.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
			EventType:   models.AssetTriggerAssetDeleted,
			SetID:       setID,
			AssetID:     assetID,
			ActorUserID: currentUser.ID,
			OldValues: map[string]interface{}{
				"title": title,
			},
		})
	}

	w.WriteHeader(http.StatusNoContent)
}
