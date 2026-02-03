package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
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

// GetAllForChannel returns all asset reports for a specific channel
func (h *AssetReportHandler) GetAllForChannel(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.Atoi(r.PathValue("channel_id"))
	if err != nil {
		respondInvalidID(w, r, "channel_id")
		return
	}

	query := `
		SELECT ar.id, ar.channel_id, ar.asset_set_id, ar.name, ar.description,
		       ar.cql_query, ar.icon, ar.color, ar.display_order, ar.is_active,
		       ar.column_config, ar.visibility_group_ids, ar.visibility_org_ids,
		       ar.created_at, ar.updated_at,
		       c.name as channel_name, ams.name as asset_set_name
		FROM asset_reports ar
		LEFT JOIN channels c ON ar.channel_id = c.id
		LEFT JOIN asset_management_sets ams ON ar.asset_set_id = ams.id
		WHERE ar.channel_id = ?
		ORDER BY ar.display_order, ar.name`

	rows, err := h.db.Query(query, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	var assetReports []models.AssetReport
	for rows.Next() {
		var ar models.AssetReport
		var columnConfig, visibilityGroupIDs, visibilityOrgIDs *string
		err := rows.Scan(&ar.ID, &ar.ChannelID, &ar.AssetSetID, &ar.Name, &ar.Description,
			&ar.CQLQuery, &ar.Icon, &ar.Color, &ar.DisplayOrder, &ar.IsActive,
			&columnConfig, &visibilityGroupIDs, &visibilityOrgIDs,
			&ar.CreatedAt, &ar.UpdatedAt,
			&ar.ChannelName, &ar.AssetSetName)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		ar.ColumnConfig = deserializeStringArray(columnConfig)
		ar.VisibilityGroupIDs = deserializeIntArray(visibilityGroupIDs)
		ar.VisibilityOrgIDs = deserializeIntArray(visibilityOrgIDs)
		assetReports = append(assetReports, ar)
	}

	if assetReports == nil {
		assetReports = []models.AssetReport{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assetReports)
}

// Get returns a specific asset report by ID
func (h *AssetReportHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var ar models.AssetReport
	var columnConfig, visibilityGroupIDs, visibilityOrgIDs *string
	err = h.db.QueryRow(`
		SELECT ar.id, ar.channel_id, ar.asset_set_id, ar.name, ar.description,
		       ar.cql_query, ar.icon, ar.color, ar.display_order, ar.is_active,
		       ar.column_config, ar.visibility_group_ids, ar.visibility_org_ids,
		       ar.created_at, ar.updated_at,
		       c.name as channel_name, ams.name as asset_set_name
		FROM asset_reports ar
		LEFT JOIN channels c ON ar.channel_id = c.id
		LEFT JOIN asset_management_sets ams ON ar.asset_set_id = ams.id
		WHERE ar.id = ?
	`, id).Scan(&ar.ID, &ar.ChannelID, &ar.AssetSetID, &ar.Name, &ar.Description,
		&ar.CQLQuery, &ar.Icon, &ar.Color, &ar.DisplayOrder, &ar.IsActive,
		&columnConfig, &visibilityGroupIDs, &visibilityOrgIDs,
		&ar.CreatedAt, &ar.UpdatedAt,
		&ar.ChannelName, &ar.AssetSetName)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	ar.ColumnConfig = deserializeStringArray(columnConfig)
	ar.VisibilityGroupIDs = deserializeIntArray(visibilityGroupIDs)
	ar.VisibilityOrgIDs = deserializeIntArray(visibilityOrgIDs)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ar)
}

// Create creates a new asset report
func (h *AssetReportHandler) Create(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.Atoi(r.PathValue("channel_id"))
	if err != nil {
		respondInvalidID(w, r, "channel_id")
		return
	}

	var ar models.AssetReport
	if err := json.NewDecoder(r.Body).Decode(&ar); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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
	if ar.DisplayOrder == 0 {
		// Get next display order
		var maxOrder int
		h.db.QueryRow("SELECT COALESCE(MAX(display_order), 0) FROM asset_reports WHERE channel_id = ?", ar.ChannelID).Scan(&maxOrder)
		ar.DisplayOrder = maxOrder + 1
	}

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO asset_reports (channel_id, asset_set_id, name, description, cql_query, icon, color, display_order, is_active, column_config, visibility_group_ids, visibility_org_ids, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, ar.ChannelID, ar.AssetSetID, ar.Name, ar.Description, ar.CQLQuery, ar.Icon, ar.Color, ar.DisplayOrder, ar.IsActive,
		serializeStringArray(ar.ColumnConfig), serializeIntArray(ar.VisibilityGroupIDs), serializeIntArray(ar.VisibilityOrgIDs), now, now).Scan(&id)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			respondConflict(w, r, "Asset report with this name already exists for this channel")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Return the created asset report
	var columnConfig, visibilityGroupIDs, visibilityOrgIDs *string
	err = h.db.QueryRow(`
		SELECT ar.id, ar.channel_id, ar.asset_set_id, ar.name, ar.description,
		       ar.cql_query, ar.icon, ar.color, ar.display_order, ar.is_active,
		       ar.column_config, ar.visibility_group_ids, ar.visibility_org_ids,
		       ar.created_at, ar.updated_at,
		       c.name as channel_name, ams.name as asset_set_name
		FROM asset_reports ar
		LEFT JOIN channels c ON ar.channel_id = c.id
		LEFT JOIN asset_management_sets ams ON ar.asset_set_id = ams.id
		WHERE ar.id = ?
	`, id).Scan(&ar.ID, &ar.ChannelID, &ar.AssetSetID, &ar.Name, &ar.Description,
		&ar.CQLQuery, &ar.Icon, &ar.Color, &ar.DisplayOrder, &ar.IsActive,
		&columnConfig, &visibilityGroupIDs, &visibilityOrgIDs,
		&ar.CreatedAt, &ar.UpdatedAt,
		&ar.ChannelName, &ar.AssetSetName)
	ar.ColumnConfig = deserializeStringArray(columnConfig)
	ar.VisibilityGroupIDs = deserializeIntArray(visibilityGroupIDs)
	ar.VisibilityOrgIDs = deserializeIntArray(visibilityOrgIDs)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ar)
}

// Update updates an existing asset report
func (h *AssetReportHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the old asset report for audit logging
	var oldAR models.AssetReport
	var oldColumnConfig, oldVisibilityGroupIDs, oldVisibilityOrgIDs *string
	err = h.db.QueryRow(`
		SELECT ar.id, ar.channel_id, ar.asset_set_id, ar.name, ar.description,
		       ar.cql_query, ar.icon, ar.color, ar.display_order, ar.is_active,
		       ar.column_config, ar.visibility_group_ids, ar.visibility_org_ids,
		       ar.created_at, ar.updated_at,
		       c.name as channel_name, ams.name as asset_set_name
		FROM asset_reports ar
		LEFT JOIN channels c ON ar.channel_id = c.id
		LEFT JOIN asset_management_sets ams ON ar.asset_set_id = ams.id
		WHERE ar.id = ?
	`, id).Scan(&oldAR.ID, &oldAR.ChannelID, &oldAR.AssetSetID, &oldAR.Name, &oldAR.Description,
		&oldAR.CQLQuery, &oldAR.Icon, &oldAR.Color, &oldAR.DisplayOrder, &oldAR.IsActive,
		&oldColumnConfig, &oldVisibilityGroupIDs, &oldVisibilityOrgIDs,
		&oldAR.CreatedAt, &oldAR.UpdatedAt,
		&oldAR.ChannelName, &oldAR.AssetSetName)
	oldAR.ColumnConfig = deserializeStringArray(oldColumnConfig)
	oldAR.VisibilityGroupIDs = deserializeIntArray(oldVisibilityGroupIDs)
	oldAR.VisibilityOrgIDs = deserializeIntArray(oldVisibilityOrgIDs)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var ar models.AssetReport
	if err := json.NewDecoder(r.Body).Decode(&ar); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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

	// Verify asset set exists
	var assetSetExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_management_sets WHERE id = ?)", ar.AssetSetID).Scan(&assetSetExists)
	if err != nil || !assetSetExists {
		respondBadRequest(w, r, "Asset set not found")
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE asset_reports
		SET asset_set_id = ?, name = ?, description = ?, cql_query = ?, icon = ?, color = ?, display_order = ?, is_active = ?,
		    column_config = ?, visibility_group_ids = ?, visibility_org_ids = ?, updated_at = ?
		WHERE id = ?
	`, ar.AssetSetID, ar.Name, ar.Description, ar.CQLQuery, ar.Icon, ar.Color, ar.DisplayOrder, ar.IsActive,
		serializeStringArray(ar.ColumnConfig), serializeIntArray(ar.VisibilityGroupIDs), serializeIntArray(ar.VisibilityOrgIDs), now, id)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			respondConflict(w, r, "Asset report with this name already exists for this channel")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Return the updated asset report
	var columnConfig, visibilityGroupIDs, visibilityOrgIDs *string
	err = h.db.QueryRow(`
		SELECT ar.id, ar.channel_id, ar.asset_set_id, ar.name, ar.description,
		       ar.cql_query, ar.icon, ar.color, ar.display_order, ar.is_active,
		       ar.column_config, ar.visibility_group_ids, ar.visibility_org_ids,
		       ar.created_at, ar.updated_at,
		       c.name as channel_name, ams.name as asset_set_name
		FROM asset_reports ar
		LEFT JOIN channels c ON ar.channel_id = c.id
		LEFT JOIN asset_management_sets ams ON ar.asset_set_id = ams.id
		WHERE ar.id = ?
	`, id).Scan(&ar.ID, &ar.ChannelID, &ar.AssetSetID, &ar.Name, &ar.Description,
		&ar.CQLQuery, &ar.Icon, &ar.Color, &ar.DisplayOrder, &ar.IsActive,
		&columnConfig, &visibilityGroupIDs, &visibilityOrgIDs,
		&ar.CreatedAt, &ar.UpdatedAt,
		&ar.ChannelName, &ar.AssetSetName)
	ar.ColumnConfig = deserializeStringArray(columnConfig)
	ar.VisibilityGroupIDs = deserializeIntArray(visibilityGroupIDs)
	ar.VisibilityOrgIDs = deserializeIntArray(visibilityOrgIDs)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldAR.Name != ar.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldAR.Name,
				"new": ar.Name,
			}
		}
		if oldAR.AssetSetID != ar.AssetSetID {
			details["asset_set_changed"] = map[string]interface{}{
				"old": oldAR.AssetSetID,
				"new": ar.AssetSetID,
			}
		}
		if oldAR.Icon != ar.Icon {
			details["icon_changed"] = map[string]interface{}{
				"old": oldAR.Icon,
				"new": ar.Icon,
			}
		}
		if oldAR.Color != ar.Color {
			details["color_changed"] = map[string]interface{}{
				"old": oldAR.Color,
				"new": ar.Color,
			}
		}

		logger.LogAudit(h.db, logger.AuditEvent{
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ar)
}

// Delete deletes an asset report
func (h *AssetReportHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

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
	var configStr string
	err = h.db.QueryRow("SELECT config FROM channels WHERE id = ?", channelID).Scan(&configStr)
	if err == nil && configStr != "" {
		var config models.ChannelConfig
		if err := json.Unmarshal([]byte(configStr), &config); err == nil {
			// Remove the asset report ID from all portal sections
			modified := false
			for i := range config.PortalSections {
				newIDs := []int{}
				for _, arID := range config.PortalSections[i].AssetReportIDs {
					if arID != id {
						newIDs = append(newIDs, arID)
					} else {
						modified = true
					}
				}
				config.PortalSections[i].AssetReportIDs = newIDs
			}

			// Update the config if we made changes
			if modified {
				updatedConfigJSON, err := json.Marshal(config)
				if err == nil {
					_, _ = h.db.ExecWrite("UPDATE channels SET config = ?, updated_at = ? WHERE id = ?",
						string(updatedConfigJSON), time.Now(), channelID)
				}
			}
		}
	}

	// Delete the asset report
	_, err = h.db.ExecWrite("DELETE FROM asset_reports WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
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
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Verify asset report exists
	var exists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_reports WHERE id = ?)", id).Scan(&exists)
	if err != nil || !exists {
		respondNotFound(w, r, "asset_report")
		return
	}

	// Parse visibility request
	var req struct {
		GroupIDs []int `json:"group_ids"`
		OrgIDs   []int `json:"org_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Update visibility columns
	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE asset_reports
		SET visibility_group_ids = ?, visibility_org_ids = ?, updated_at = ?
		WHERE id = ?
	`, serializeIntArray(req.GroupIDs), serializeIntArray(req.OrgIDs), now, id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated asset report
	var ar models.AssetReport
	var columnConfig, visibilityGroupIDs, visibilityOrgIDs *string
	err = h.db.QueryRow(`
		SELECT ar.id, ar.channel_id, ar.asset_set_id, ar.name, ar.description,
		       ar.cql_query, ar.icon, ar.color, ar.display_order, ar.is_active,
		       ar.column_config, ar.visibility_group_ids, ar.visibility_org_ids,
		       ar.created_at, ar.updated_at,
		       c.name as channel_name, ams.name as asset_set_name
		FROM asset_reports ar
		LEFT JOIN channels c ON ar.channel_id = c.id
		LEFT JOIN asset_management_sets ams ON ar.asset_set_id = ams.id
		WHERE ar.id = ?
	`, id).Scan(&ar.ID, &ar.ChannelID, &ar.AssetSetID, &ar.Name, &ar.Description,
		&ar.CQLQuery, &ar.Icon, &ar.Color, &ar.DisplayOrder, &ar.IsActive,
		&columnConfig, &visibilityGroupIDs, &visibilityOrgIDs,
		&ar.CreatedAt, &ar.UpdatedAt,
		&ar.ChannelName, &ar.AssetSetName)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	ar.ColumnConfig = deserializeStringArray(columnConfig)
	ar.VisibilityGroupIDs = deserializeIntArray(visibilityGroupIDs)
	ar.VisibilityOrgIDs = deserializeIntArray(visibilityOrgIDs)

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "asset_report_visibility_update",
			ResourceType: "asset_report",
			ResourceID:   &ar.ID,
			ResourceName: ar.Name,
			Details: map[string]interface{}{
				"visibility_group_ids": req.GroupIDs,
				"visibility_org_ids":   req.OrgIDs,
			},
			Success: true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ar)
}
