package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// serializeStringArray converts a slice of strings to a JSON string pointer
// Returns nil if the slice is empty or nil
func serializeStringArray(strs []string) *string {
	if len(strs) == 0 {
		return nil
	}
	data, err := json.Marshal(strs)
	if err != nil {
		return nil
	}
	s := string(data)
	return &s
}

// deserializeStringArray converts a JSON string pointer to a slice of strings
// Returns nil if the string is nil or empty
func deserializeStringArray(s *string) []string {
	if s == nil || *s == "" {
		return nil
	}
	var strs []string
	if err := json.Unmarshal([]byte(*s), &strs); err != nil {
		return nil
	}
	return strs
}

type AssetReportHandler struct {
	db database.Database
}

func NewAssetReportHandler(db database.Database) *AssetReportHandler {
	return &AssetReportHandler{db: db}
}

// assetReportSelectQuery is the shared SELECT used to fetch asset reports with joined names.
const assetReportSelectQuery = `
	SELECT ar.id, ar.channel_id, ar.asset_set_id, ar.name, ar.description,
	       ar.cql_query, ar.icon, ar.color, ar.display_order, ar.is_active,
	       ar.column_config, ar.visibility_group_ids, ar.visibility_org_ids,
	       ar.run_mode, ar.item_type_id, ar.workspace_id, ar.config,
	       ar.created_at, ar.updated_at,
	       c.name as channel_name, ams.name as asset_set_name,
	       it.name as item_type_name
	FROM asset_reports ar
	LEFT JOIN channels c ON ar.channel_id = c.id
	LEFT JOIN asset_management_sets ams ON ar.asset_set_id = ams.id
	LEFT JOIN item_types it ON ar.item_type_id = it.id`

// scanAssetReport scans a single asset report row (works with both *sql.Row and *sql.Rows).
func scanAssetReport(scanner interface{ Scan(...interface{}) error }) (models.AssetReport, error) {
	var ar models.AssetReport
	var columnConfig, visibilityGroupIDs, visibilityOrgIDs, config *string
	var itemTypeName sql.NullString
	err := scanner.Scan(&ar.ID, &ar.ChannelID, &ar.AssetSetID, &ar.Name, &ar.Description,
		&ar.CQLQuery, &ar.Icon, &ar.Color, &ar.DisplayOrder, &ar.IsActive,
		&columnConfig, &visibilityGroupIDs, &visibilityOrgIDs,
		&ar.RunMode, &ar.ItemTypeID, &ar.WorkspaceID, &config,
		&ar.CreatedAt, &ar.UpdatedAt,
		&ar.ChannelName, &ar.AssetSetName,
		&itemTypeName)
	if err != nil {
		return ar, err
	}
	ar.ColumnConfig = deserializeStringArray(columnConfig)
	ar.VisibilityGroupIDs = deserializeIntArray(visibilityGroupIDs)
	ar.VisibilityOrgIDs = deserializeIntArray(visibilityOrgIDs)
	ar.Config = config
	if itemTypeName.Valid {
		ar.ItemTypeName = itemTypeName.String
	}
	return ar, nil
}

// loadAssetReportByID fetches a single asset report by ID using the standard joined query.
func (h *AssetReportHandler) loadAssetReportByID(id int) (models.AssetReport, error) {
	return scanAssetReport(h.db.QueryRow(assetReportSelectQuery+" WHERE ar.id = ?", id))
}

// GetAllForChannel returns all asset reports for a specific channel
func (h *AssetReportHandler) GetAllForChannel(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	rows, err := h.db.Query(assetReportSelectQuery+" WHERE ar.channel_id = ? ORDER BY ar.display_order, ar.name", channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var assetReports []models.AssetReport
	for rows.Next() {
		ar, err := scanAssetReport(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		assetReports = append(assetReports, ar)
	}

	if assetReports == nil {
		assetReports = []models.AssetReport{}
	}

	respondJSONOK(w, assetReports)
}

// Get returns a specific asset report by ID
func (h *AssetReportHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ar, err := h.loadAssetReportByID(id)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, ar)
}

// Create creates a new asset report
func (h *AssetReportHandler) Create(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	var err error

	ar, ok := decodeJSON[models.AssetReport](w, r)
	if !ok {
		return
	}

	// Set channel ID from URL
	ar.ChannelID = channelID

	// Validate required fields
	if strings.TrimSpace(ar.Name) == "" {
		respondValidationError(w, r, "Asset report name is required")
		return
	}
	if ar.AssetSetID == 0 {
		respondValidationError(w, r, "Asset set ID is required")
		return
	}
	if strings.TrimSpace(ar.CQLQuery) == "" {
		respondValidationError(w, r, "QL query is required")
		return
	}

	// Verify channel exists
	var channelExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?)", ar.ChannelID).Scan(&channelExists)
	if err != nil || !channelExists {
		respondBadRequest(w, r, "Channel not found")
		return
	}

	// Verify asset set exists
	var assetSetExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_management_sets WHERE id = ?)", ar.AssetSetID).Scan(&assetSetExists)
	if err != nil || !assetSetExists {
		respondBadRequest(w, r, "Asset set not found")
		return
	}

	// Set default values if not provided
	if ar.Icon == "" {
		ar.Icon = "Table2"
	}
	if ar.Color == "" {
		ar.Color = "#6b7280"
	}
	if ar.RunMode == "" {
		ar.RunMode = "direct"
	}
	if ar.RunMode != "direct" && ar.RunMode != "form" {
		respondValidationError(w, r, "Invalid run_mode")
		return
	}
	if ar.DisplayOrder == 0 {
		// Get next display order
		var maxOrder int
		if err := h.db.QueryRow("SELECT COALESCE(MAX(display_order), 0) FROM asset_reports WHERE channel_id = ?", ar.ChannelID).Scan(&maxOrder); err != nil {
			slog.Warn("failed to get max display order for asset reports", slog.Any("error", err))
		}
		ar.DisplayOrder = maxOrder + 1
	}

	// Check uniqueness before insert
	var nameExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_reports WHERE name = ? AND channel_id = ?)", ar.Name, ar.ChannelID).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Asset report with this name already exists for this channel")
		return
	}

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO asset_reports (channel_id, asset_set_id, name, description, cql_query, icon, color, display_order, is_active, column_config, visibility_group_ids, visibility_org_ids, run_mode, item_type_id, workspace_id, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, ar.ChannelID, ar.AssetSetID, ar.Name, ar.Description, ar.CQLQuery, ar.Icon, ar.Color, ar.DisplayOrder, ar.IsActive,
		serializeStringArray(ar.ColumnConfig), serializeIntArray(ar.VisibilityGroupIDs), serializeIntArray(ar.VisibilityOrgIDs),
		ar.RunMode, ar.ItemTypeID, ar.WorkspaceID, ar.Config, now, now).Scan(&id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Asset report with this name already exists for this channel")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Return the created asset report
	ar, err = h.loadAssetReportByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "asset_report_create",
			ResourceType: "asset_report",
			ResourceID:   &ar.ID,
			ResourceName: ar.Name,
			Details: map[string]interface{}{
				"channel_id":   ar.ChannelID,
				"asset_set_id": ar.AssetSetID,
				"icon":         ar.Icon,
				"color":        ar.Color,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, ar)
}

// Update updates an existing asset report
func (h *AssetReportHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Get the old asset report fields needed for audit logging
	var oldName, oldIcon, oldColor string
	var oldAssetSetID int
	err = h.db.QueryRow(`SELECT name, asset_set_id, icon, color FROM asset_reports WHERE id = ?`, id).
		Scan(&oldName, &oldAssetSetID, &oldIcon, &oldColor)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	ar, ok := decodeJSON[models.AssetReport](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if strings.TrimSpace(ar.Name) == "" {
		respondValidationError(w, r, "Asset report name is required")
		return
	}
	if ar.AssetSetID == 0 {
		respondValidationError(w, r, "Asset set ID is required")
		return
	}
	if strings.TrimSpace(ar.CQLQuery) == "" {
		respondValidationError(w, r, "QL query is required")
		return
	}

	// Verify asset set exists
	var assetSetExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_management_sets WHERE id = ?)", ar.AssetSetID).Scan(&assetSetExists)
	if err != nil || !assetSetExists {
		respondBadRequest(w, r, "Asset set not found")
		return
	}

	// Check uniqueness before update
	var nameExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_reports WHERE name = ? AND channel_id = (SELECT channel_id FROM asset_reports WHERE id = ?) AND id != ?)", ar.Name, id, id).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Asset report with this name already exists for this channel")
		return
	}

	if ar.RunMode == "" {
		ar.RunMode = "direct"
	}
	if ar.RunMode != "direct" && ar.RunMode != "form" {
		respondValidationError(w, r, "Invalid run_mode")
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE asset_reports
		SET asset_set_id = ?, name = ?, description = ?, cql_query = ?, icon = ?, color = ?, display_order = ?, is_active = ?,
		    column_config = ?, visibility_group_ids = ?, visibility_org_ids = ?,
		    run_mode = ?, item_type_id = ?, workspace_id = ?, config = ?,
		    updated_at = ?
		WHERE id = ?
	`, ar.AssetSetID, ar.Name, ar.Description, ar.CQLQuery, ar.Icon, ar.Color, ar.DisplayOrder, ar.IsActive,
		serializeStringArray(ar.ColumnConfig), serializeIntArray(ar.VisibilityGroupIDs), serializeIntArray(ar.VisibilityOrgIDs),
		ar.RunMode, ar.ItemTypeID, ar.WorkspaceID, ar.Config,
		now, id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Asset report with this name already exists for this channel")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Return the updated asset report
	ar, err = h.loadAssetReportByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldName != ar.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldName,
				"new": ar.Name,
			}
		}
		if oldAssetSetID != ar.AssetSetID {
			details["asset_set_changed"] = map[string]interface{}{
				"old": oldAssetSetID,
				"new": ar.AssetSetID,
			}
		}
		if oldIcon != ar.Icon {
			details["icon_changed"] = map[string]interface{}{
				"old": oldIcon,
				"new": ar.Icon,
			}
		}
		if oldColor != ar.Color {
			details["color_changed"] = map[string]interface{}{
				"old": oldColor,
				"new": ar.Color,
			}
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "asset_report_update",
			ResourceType: "asset_report",
			ResourceID:   &ar.ID,
			ResourceName: ar.Name,
			Details:      details,
			Success:      true,
		})
	}

	respondJSONOK(w, ar)
}

// Delete deletes an asset report
func (h *AssetReportHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Get the asset report details for audit logging
	var assetReportName string
	var channelID int
	err = h.db.QueryRow(`
		SELECT name, channel_id
		FROM asset_reports
		WHERE id = ?
	`, id).Scan(&assetReportName, &channelID)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Clean up portal sections: remove this asset report ID from all sections
	removeIDFromPortalSections(h.db, channelID, id,
		func(s *models.PortalSection) []int { return s.AssetReportIDs },
		func(s *models.PortalSection, ids []int) { s.AssetReportIDs = ids },
	)

	// Delete the asset report
	_, err = h.db.ExecWrite("DELETE FROM asset_reports WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "asset_report_delete",
			ResourceType: "asset_report",
			ResourceID:   &id,
			ResourceName: assetReportName,
			Details: map[string]interface{}{
				"channel_id": channelID,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateVisibility updates only the visibility settings for an asset report
func (h *AssetReportHandler) UpdateVisibility(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !decodeAndUpdateVisibility(w, r, h.db, "asset_reports", "asset_report", id) {
		return
	}

	// Return the updated asset report
	ar, loadErr := h.loadAssetReportByID(id)
	if loadErr != nil {
		respondInternalError(w, r, loadErr)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "asset_report_visibility_update",
			ResourceType: "asset_report",
			ResourceID:   &ar.ID,
			ResourceName: ar.Name,
			Details: map[string]interface{}{
				"visibility_group_ids": ar.VisibilityGroupIDs,
				"visibility_org_ids":   ar.VisibilityOrgIDs,
			},
			Success: true,
		})
	}

	respondJSONOK(w, ar)
}

// assetReportFieldsSelectQuery mirrors request_type_fields' select — used for form-mode fields.
const assetReportFieldsSelectQuery = `
	SELECT arf.id, arf.asset_report_id, arf.field_identifier, arf.field_type,
	       arf.display_order, arf.is_required, arf.display_name, arf.description,
	       COALESCE(arf.step_number, 1) as step_number,
	       arf.virtual_field_type, arf.virtual_field_options,
	       arf.created_at, arf.updated_at,
	       CASE
	           WHEN arf.field_type = 'virtual' THEN arf.field_identifier
	           ELSE COALESCE(cfd.name, arf.field_identifier)
	       END as field_name,
	       CASE
	           WHEN arf.display_name IS NOT NULL AND arf.display_name != '' THEN arf.display_name
	           WHEN arf.field_type = 'virtual' THEN arf.field_identifier
	           ELSE COALESCE(cfd.name, arf.field_identifier)
	       END as field_label
	FROM asset_report_fields arf
	LEFT JOIN custom_field_definitions cfd ON arf.field_type = 'custom' AND arf.field_identifier = CAST(cfd.id AS TEXT)
	WHERE arf.asset_report_id = ?
	ORDER BY arf.step_number, arf.display_order, arf.id`

// GetFields returns all fields for a form-mode asset report.
func (h *AssetReportHandler) GetFields(w http.ResponseWriter, r *http.Request) {
	assetReportID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	rows, err := h.db.Query(assetReportFieldsSelectQuery, assetReportID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var fields []models.AssetReportField
	for rows.Next() {
		var f models.AssetReportField
		err := rows.Scan(&f.ID, &f.AssetReportID, &f.FieldIdentifier, &f.FieldType,
			&f.DisplayOrder, &f.IsRequired, &f.DisplayName, &f.Description,
			&f.StepNumber, &f.VirtualFieldType, &f.VirtualFieldOptions,
			&f.CreatedAt, &f.UpdatedAt,
			&f.FieldName, &f.FieldLabel)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		fields = append(fields, f)
	}

	if fields == nil {
		fields = []models.AssetReportField{}
	}

	respondJSONOK(w, fields)
}

// UpdateFields replaces all fields for a form-mode asset report.
func (h *AssetReportHandler) UpdateFields(w http.ResponseWriter, r *http.Request) {
	assetReportID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var exists bool
	if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_reports WHERE id = ?)", assetReportID).Scan(&exists); err != nil || !exists {
		respondNotFound(w, r, "asset_report")
		return
	}

	fields, ok := decodeJSON[[]models.AssetReportField](w, r)
	if !ok {
		return
	}

	if _, err := h.db.ExecWrite("DELETE FROM asset_report_fields WHERE asset_report_id = ?", assetReportID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	now := time.Now()
	for _, f := range fields {
		step := f.StepNumber
		if step == 0 {
			step = 1
		}
		_, err := h.db.ExecWrite(`
			INSERT INTO asset_report_fields (asset_report_id, field_identifier, field_type, display_order, is_required,
			                                 display_name, description, step_number, virtual_field_type, virtual_field_options,
			                                 created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, assetReportID, f.FieldIdentifier, f.FieldType, f.DisplayOrder, f.IsRequired,
			f.DisplayName, f.Description, step, f.VirtualFieldType, f.VirtualFieldOptions, now, now)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "asset_report_fields_update",
			ResourceType: "asset_report",
			ResourceID:   &assetReportID,
			Details:      map[string]interface{}{"field_count": len(fields)},
			Success:      true,
		})
	}

	h.GetFields(w, r)
}

// GetAvailableFields returns fields available to bind on a form-mode asset report.
func (h *AssetReportHandler) GetAvailableFields(w http.ResponseWriter, r *http.Request) {
	assetReportID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var itemTypeID *int
	var workspaceID *int
	err := h.db.QueryRow("SELECT item_type_id, workspace_id FROM asset_reports WHERE id = ?", assetReportID).Scan(&itemTypeID, &workspaceID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	fields := []AvailableField{
		{Identifier: "title", Name: "Title", Type: "default"},
		{Identifier: "description", Name: "Description", Type: "default"},
	}

	if workspaceID == nil || itemTypeID == nil {
		respondJSONOK(w, fields)
		return
	}

	var createScreenID *int
	err = h.db.QueryRow(`
		SELECT csit.create_screen_id
		FROM workspace_configuration_sets wcs
		JOIN configuration_set_item_types csit
		  ON csit.configuration_set_id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ? AND csit.item_type_id = ?
		LIMIT 1
	`, *workspaceID, *itemTypeID).Scan(&createScreenID)
	if err == sql.ErrNoRows || createScreenID == nil {
		respondJSONOK(w, fields)
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	fields, err = appendCustomScreenFields(h.db, *createScreenID, fields)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}
