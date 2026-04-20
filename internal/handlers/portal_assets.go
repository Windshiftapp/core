package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
)

// ExecuteAssetReport executes a CQL query for an asset report and returns the assets
func (h *PortalHandler) ExecuteAssetReport(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	reportIDStr := r.PathValue("id")
	reportID, err := strconv.Atoi(reportIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	channel := portalResult.channel

	// Get the asset report
	var report struct {
		ID           int
		ChannelID    int
		AssetSetID   int
		CQLQuery     string
		IsActive     bool
		ColumnConfig sql.NullString
	}
	err = h.db.QueryRowContext(ctx, `
		SELECT id, channel_id, asset_set_id, cql_query, is_active, column_config
		FROM asset_reports WHERE id = ?
	`, reportID).Scan(&report.ID, &report.ChannelID, &report.AssetSetID, &report.CQLQuery, &report.IsActive, &report.ColumnConfig)

	if err == sql.ErrNoRows {
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Verify report belongs to this channel
	if report.ChannelID != channel.ID {
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
		return
	}

	// Verify report is active
	if !report.IsActive {
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
		return
	}

	// Get portal customer ID for CQL function replacements
	var portalCustomerID *int
	var customerOrgID *int
	portalCustomerID, _ = h.getPortalCustomerID(ctx, r)

	//nolint:misspell // British spelling used in database
	// Get organisation ID for this customer if authenticated
	if portalCustomerID != nil {
		customerOrgID = h.getPortalCustomerOrgID(ctx, *portalCustomerID)
	}

	// Replace CQL functions with actual values
	cqlQuery := report.CQLQuery

	// Replace currentUser() in CQL query with actual user ID
	if portalCustomerID != nil && strings.Contains(cqlQuery, "currentUser()") {
		// Get the user_id linked to this portal customer (if any)
		var userID sql.NullInt64
		_ = h.db.QueryRowContext(ctx, `SELECT user_id FROM portal_customers WHERE id = ?`, *portalCustomerID).Scan(&userID)
		if userID.Valid {
			cqlQuery = strings.ReplaceAll(cqlQuery, "currentUser()", fmt.Sprintf("%d", userID.Int64))
		} else {
			// If no linked user, use portal customer ID with negative sign to differentiate
			cqlQuery = strings.ReplaceAll(cqlQuery, "currentUser()", fmt.Sprintf("portal:%d", *portalCustomerID))
		}
	}

	// Replace currentCustomer() with portal customer ID
	if portalCustomerID != nil && strings.Contains(cqlQuery, "currentCustomer()") {
		cqlQuery = strings.ReplaceAll(cqlQuery, "currentCustomer()", fmt.Sprintf("%d", *portalCustomerID))
	}

	//nolint:misspell // British spelling used in database
	// Replace currentOrganisation() with customer organisation ID
	if customerOrgID != nil && strings.Contains(cqlQuery, "currentOrganisation()") {
		cqlQuery = strings.ReplaceAll(cqlQuery, "currentOrganisation()", fmt.Sprintf("%d", *customerOrgID))
	}

	// Evaluate CQL (if any) to a SQL fragment against the assets table.
	var cqlSQL string
	var cqlArgs []interface{}
	if strings.TrimSpace(cqlQuery) != "" {
		assetRepo := repository.NewAssetRepository(h.db)
		setMap, setMapErr := assetRepo.GetCQLSetMap()
		if setMapErr != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load set mapping: %w", setMapErr))
			return
		}
		workspaceMap, wsErr := buildAssetCQLWorkspaceMap(h.db)
		if wsErr != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load workspace mapping: %w", wsErr))
			return
		}
		customFieldMap, cfErr := assetRepo.GetCQLCustomFieldMap(report.AssetSetID)
		if cfErr != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load custom field mapping: %w", cfErr))
			return
		}
		evaluator := cql.NewAssetEvaluator(setMap, workspaceMap, customFieldMap, h.db.GetDriverName())
		var evalErr error
		cqlSQL, cqlArgs, evalErr = evaluator.EvaluateToSQL(cqlQuery)
		if evalErr != nil {
			respondValidationError(w, r, "CQL query error: "+evalErr.Error())
			return
		}
	}

	// Parse pagination parameters
	page := 1
	perPage := 25
	if p := r.URL.Query().Get("page"); p != "" {
		var pInt int
		if pInt, err = strconv.Atoi(p); err == nil && pInt > 0 {
			page = pInt
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		var ppInt int
		if ppInt, err = strconv.Atoi(pp); err == nil && ppInt > 0 && ppInt <= 100 {
			perPage = ppInt
		}
	}
	offset := (page - 1) * perPage

	// Parse column config
	var columns []string
	if report.ColumnConfig.Valid && report.ColumnConfig.String != "" {
		_ = json.Unmarshal([]byte(report.ColumnConfig.String), &columns)
	}
	if len(columns) == 0 {
		columns = []string{"title", "asset_tag", "status_id"}
	}

	// Build the query for assets, scoped to the report's set and optionally filtered by CQL.
	whereClause := "a.set_id = ?"
	queryArgs := []interface{}{report.AssetSetID}
	if cqlSQL != "" {
		whereClause += " AND (" + cqlSQL + ")"
		queryArgs = append(queryArgs, cqlArgs...)
	}

	query := `
		SELECT a.id, a.title, a.asset_tag, a.asset_type_id, a.status_id, a.category_id,
		       a.custom_field_values, a.created_at, a.updated_at,
		       at.name as asset_type_name, ast.name as status_name, ast.color as status_color
		FROM assets a
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		WHERE ` + whereClause + `
		ORDER BY a.created_at DESC
		LIMIT ? OFFSET ?
	`
	queryArgs = append(queryArgs, perPage, offset)

	rows, err := h.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type AssetResult struct {
		ID                int                    `json:"id"`
		Title             string                 `json:"title"`
		AssetTag          string                 `json:"asset_tag"`
		AssetTypeID       *int                   `json:"asset_type_id,omitempty"`
		StatusID          *int                   `json:"status_id,omitempty"`
		CategoryID        *int                   `json:"category_id,omitempty"`
		CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
		CreatedAt         time.Time              `json:"created_at"`
		UpdatedAt         time.Time              `json:"updated_at"`
		AssetTypeName     *string                `json:"asset_type_name,omitempty"`
		StatusName        *string                `json:"status_name,omitempty"`
		StatusColor       *string                `json:"status_color,omitempty"`
	}

	var assets []AssetResult
	for rows.Next() {
		var asset AssetResult
		var assetTypeID, statusID, categoryID sql.NullInt64
		var customFieldValuesStr sql.NullString
		var assetTypeName, statusName, statusColor sql.NullString

		err := rows.Scan(&asset.ID, &asset.Title, &asset.AssetTag, &assetTypeID, &statusID, &categoryID,
			&customFieldValuesStr, &asset.CreatedAt, &asset.UpdatedAt,
			&assetTypeName, &statusName, &statusColor)
		if err != nil {
			continue
		}

		if assetTypeID.Valid {
			id := int(assetTypeID.Int64)
			asset.AssetTypeID = &id
		}
		if statusID.Valid {
			id := int(statusID.Int64)
			asset.StatusID = &id
		}
		if categoryID.Valid {
			id := int(categoryID.Int64)
			asset.CategoryID = &id
		}
		if customFieldValuesStr.Valid && customFieldValuesStr.String != "" {
			_ = json.Unmarshal([]byte(customFieldValuesStr.String), &asset.CustomFieldValues)
		}
		if assetTypeName.Valid {
			asset.AssetTypeName = &assetTypeName.String
		}
		if statusName.Valid {
			asset.StatusName = &statusName.String
		}
		if statusColor.Valid {
			asset.StatusColor = &statusColor.String
		}

		assets = append(assets, asset)
	}

	if assets == nil {
		assets = []AssetResult{}
	}

	// Get total count honoring the same CQL filter.
	countArgs := []interface{}{report.AssetSetID}
	countQuery := `SELECT COUNT(*) FROM assets a WHERE a.set_id = ?`
	if cqlSQL != "" {
		countQuery += " AND (" + cqlSQL + ")"
		countArgs = append(countArgs, cqlArgs...)
	}
	var total int
	if err := h.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		slog.Warn("failed to get asset count", slog.Any("error", err))
	}

	// Build response
	response := map[string]interface{}{
		"assets":      assets,
		"columns":     columns,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + perPage - 1) / perPage,
	}

	respondJSONOK(w, response)
}

// GetAssetReports returns asset reports for a portal, filtered by visibility
func (h *PortalHandler) GetAssetReports(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	channel := portalResult.channel

	// Query all asset reports for this channel
	query := `
		SELECT ar.id, ar.channel_id, ar.asset_set_id, ar.name, ar.description,
		       ar.cql_query, ar.icon, ar.color, ar.display_order, ar.is_active,
		       ar.column_config, ar.visibility_group_ids, ar.visibility_org_ids,
		       ar.created_at, ar.updated_at,
		       ams.name as asset_set_name
		FROM asset_reports ar
		LEFT JOIN asset_management_sets ams ON ar.asset_set_id = ams.id
		WHERE ar.channel_id = ? AND ar.is_active = true
		ORDER BY ar.display_order, ar.name`

	rows, err := h.db.QueryContext(ctx, query, channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	// Get visibility context for filtering
	vc := h.getPortalVisibilityContext(ctx, r, channel.ID)

	var assetReports []models.AssetReport
	for rows.Next() {
		var ar models.AssetReport
		var columnConfig, visibilityGroupIDs, visibilityOrgIDs sql.NullString
		err := rows.Scan(&ar.ID, &ar.ChannelID, &ar.AssetSetID, &ar.Name, &ar.Description,
			&ar.CQLQuery, &ar.Icon, &ar.Color, &ar.DisplayOrder, &ar.IsActive,
			&columnConfig, &visibilityGroupIDs, &visibilityOrgIDs,
			&ar.CreatedAt, &ar.UpdatedAt,
			&ar.AssetSetName)
		if err != nil {
			continue
		}

		// Deserialize arrays
		if columnConfig.Valid && columnConfig.String != "" {
			var cols []string
			if err := json.Unmarshal([]byte(columnConfig.String), &cols); err == nil {
				ar.ColumnConfig = cols
			}
		}
		if visibilityGroupIDs.Valid && visibilityGroupIDs.String != "" {
			var ids []int
			if err := json.Unmarshal([]byte(visibilityGroupIDs.String), &ids); err == nil {
				ar.VisibilityGroupIDs = ids
			}
		}
		if visibilityOrgIDs.Valid && visibilityOrgIDs.String != "" {
			var ids []int
			if err := json.Unmarshal([]byte(visibilityOrgIDs.String), &ids); err == nil {
				ar.VisibilityOrgIDs = ids
			}
		}

		// Admin users see all; others see only visible ones
		if vc.isAdmin || ar.IsVisibleTo(vc.userGroupIDs, vc.customerOrgID) {
			assetReports = append(assetReports, ar)
		}
	}

	if assetReports == nil {
		assetReports = []models.AssetReport{}
	}

	respondJSONOK(w, assetReports)
}

// GetRequestTypeFields returns fields for a request type (portal-aware authentication)
// Accepts either internal session OR portal customer session
func (h *PortalHandler) GetRequestTypeFields(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	requestTypeIDStr := r.PathValue("id")
	requestTypeID, err := strconv.Atoi(requestTypeIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}

	// Verify request type belongs to this channel
	valid, err := h.portalService.ValidateRequestTypeBelongsToChannel(ctx, requestTypeID, portalResult.channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !valid {
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Request type not found"))
		return
	}

	// Get fields from service
	fields, err := h.portalService.GetRequestTypeFields(ctx, requestTypeID)
	if err != nil {
		slog.Error("failed to get request type fields", slog.String("component", "portal"), slog.Int("request_type_id", requestTypeID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}

// GetCustomFields returns custom field definitions used by this portal's request types
// Accepts either internal session OR portal customer session
func (h *PortalHandler) GetCustomFields(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}

	// Get custom fields used by this channel's request types
	fields, err := h.portalService.GetCustomFieldsForChannel(ctx, portalResult.channel.ID)
	if err != nil {
		slog.Error("failed to get custom fields for channel", slog.String("component", "portal"), slog.Int("channel_id", portalResult.channel.ID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}
