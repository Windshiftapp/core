package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
)

// formFieldPlaceholderRe matches ${identifier} placeholders that form-mode
// asset reports use to inject submitted field values into the CQL query.
// Identifiers can be alphanumeric, underscore, or dash (e.g. ${asset_tag},
// ${cf_5}). The body of the placeholder is replaced with a CQL string literal
// built from the submitted value, with " and \ escaped per the tokenizer.
var formFieldPlaceholderRe = regexp.MustCompile(`\$\{([A-Za-z0-9_\-]+)\}`)

// substituteFormFields replaces ${identifier} placeholders in a CQL query
// with quoted, escaped string literals built from the submitted form values.
// Missing values become an empty string literal — the caller is expected to
// have validated required fields before this runs.
func substituteFormFields(query string, values map[string]string) string {
	if !strings.Contains(query, "${") {
		return query
	}
	return formFieldPlaceholderRe.ReplaceAllStringFunc(query, func(match string) string {
		// match looks like "${name}" — strip the wrapper.
		name := match[2 : len(match)-1]
		raw := values[name]
		// Tokenizer treats `\` as a generic escape (any next rune passes through),
		// so `\\` and `\"` are sufficient to keep the value bound inside the literal.
		escaped := strings.ReplaceAll(raw, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	})
}

// readFormParams pulls the params object from a POST body for form-mode
// asset report execution. Body shape: {"params": {"field_id": "value", ...}}.
// Returns an empty map for direct-mode (GET) requests or empty bodies.
// Values are coerced to strings — numbers, booleans, and strings all flow
// into CQL as quoted literals (the tokenizer handles type coercion at compare
// time for status/priority/etc.).
func readFormParams(r *http.Request) (map[string]string, error) {
	if r.Method != http.MethodPost || r.Body == nil || r.ContentLength == 0 {
		return map[string]string{}, nil
	}
	var body struct {
		Params map[string]interface{} `json:"params"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(body.Params))
	for k, v := range body.Params {
		if v == nil {
			continue
		}
		switch tv := v.(type) {
		case string:
			out[k] = tv
		case float64:
			// JSON numbers decode as float64 — render without trailing zeros.
			out[k] = strconv.FormatFloat(tv, 'f', -1, 64)
		case bool:
			out[k] = strconv.FormatBool(tv)
		default:
			b, _ := json.Marshal(tv)
			out[k] = string(b)
		}
	}
	return out, nil
}

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
		ID                 int
		ChannelID          int
		AssetSetID         int
		CQLQuery           string
		IsActive           bool
		RunMode            string
		ColumnConfig       sql.NullString
		VisibilityGroupIDs sql.NullString
		VisibilityOrgIDs   sql.NullString
	}
	err = h.db.QueryRowContext(ctx, `
		SELECT id, channel_id, asset_set_id, cql_query, is_active, run_mode, column_config,
		       visibility_group_ids, visibility_org_ids
		FROM asset_reports WHERE id = ?
	`, reportID).Scan(&report.ID, &report.ChannelID, &report.AssetSetID, &report.CQLQuery, &report.IsActive, &report.RunMode, &report.ColumnConfig,
		&report.VisibilityGroupIDs, &report.VisibilityOrgIDs)

	if errors.Is(err, sql.ErrNoRows) {
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

	// Visibility check — must mirror GetAssetReports so the list and execute paths
	// agree on who can see this report. Return 404 (not 403) so report existence
	// is not disclosed to unauthorized callers.
	vc := h.getPortalVisibilityContext(ctx, r, channel.ID)
	if !vc.isAdmin {
		ar := models.AssetReport{
			VisibilityGroupIDs: unmarshalIntIDs(report.VisibilityGroupIDs),
			VisibilityOrgIDs:   unmarshalIntIDs(report.VisibilityOrgIDs),
		}
		if !ar.IsVisibleTo(vc.userGroupIDs, vc.customerOrgID) {
			respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
			return
		}
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

	// Form mode (run_mode='form') wraps a CQL query with ${field} placeholders
	// and only runs once the customer has submitted form values via POST. The
	// values are validated against the configured field schema and substituted
	// into the query before context-function substitution. Direct mode skips
	// this step — its CQL is self-contained.
	cqlQuery := report.CQLQuery
	if report.RunMode == "form" {
		formValues, formErr := readFormParams(r)
		if formErr != nil {
			respondValidationError(w, r, "Invalid request body: "+formErr.Error())
			return
		}
		// Required-field check: load the schema and reject when any required
		// field is missing or blank. This blocks empty submissions from
		// leaking the unrestricted query result.
		fields, ferr := h.loadAssetReportFields(ctx, reportID)
		if ferr != nil {
			respondInternalError(w, r, ferr)
			return
		}
		// Empty form submission (no params at all) on a form-mode report is
		// the "show me the form" state — return an empty result and the column
		// config so the FE can render headers without running the query.
		if len(formValues) == 0 && r.Method == http.MethodGet {
			respondJSONOK(w, map[string]interface{}{
				"assets":                   []interface{}{},
				"columns":                  decodeColumnConfig(report.ColumnConfig),
				"total":                    0,
				"page":                     1,
				"per_page":                 25,
				"total_pages":              0,
				"awaiting_form_submission": true,
			})
			return
		}
		for _, f := range fields {
			if f.IsRequired {
				v, ok := formValues[f.FieldIdentifier]
				if !ok || strings.TrimSpace(v) == "" {
					respondValidationError(w, r, "Missing required field: "+f.FieldIdentifier)
					return
				}
			}
		}
		cqlQuery = substituteFormFields(cqlQuery, formValues)
	}

	// Replace CQL context functions with actual values via the shared substitution helper.
	// For the portal context, currentUser() resolves to the user_id linked to the portal
	// customer (if any) — falling back to a portal:<customerID> sentinel that won't match
	// real user IDs.
	fnCtx := cql.FunctionContext{
		CustomerID:     portalCustomerID,
		OrganisationID: customerOrgID,
	}
	if portalCustomerID != nil {
		var userID sql.NullInt64
		_ = h.db.QueryRowContext(ctx, `SELECT user_id FROM portal_customers WHERE id = ?`, *portalCustomerID).Scan(&userID)
		if userID.Valid {
			uid := int(userID.Int64)
			fnCtx.UserID = &uid
		} else if strings.Contains(cqlQuery, "currentUser()") {
			// No linked user: replace with sentinel that won't match real user IDs.
			cqlQuery = strings.ReplaceAll(cqlQuery, "currentUser()", fmt.Sprintf("portal:%d", *portalCustomerID))
		}
	}
	cqlQuery = cql.SubstituteFunctions(cqlQuery, fnCtx)

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
		workspaceMap, wsErr := repository.NewWorkspaceRepository(h.db).ListNameKeyToIDMap()
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

	columns := decodeColumnConfig(report.ColumnConfig)

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
		       at.name as asset_type_name, ast.name as status_name, ast.color as status_color,
		       ac.name as category_name
		FROM assets a
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
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
		CategoryName      *string                `json:"category_name,omitempty"`
	}

	var assets []AssetResult
	for rows.Next() {
		var asset AssetResult
		var assetTypeID, statusID, categoryID sql.NullInt64
		var customFieldValuesStr sql.NullString
		var assetTypeName, statusName, statusColor, categoryName sql.NullString

		err := rows.Scan(&asset.ID, &asset.Title, &asset.AssetTag, &assetTypeID, &statusID, &categoryID,
			&customFieldValuesStr, &asset.CreatedAt, &asset.UpdatedAt,
			&assetTypeName, &statusName, &statusColor, &categoryName)
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
		if categoryName.Valid {
			asset.CategoryName = &categoryName.String
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
		if ids := unmarshalIntIDs(visibilityGroupIDs); ids != nil {
			ar.VisibilityGroupIDs = ids
		}
		if ids := unmarshalIntIDs(visibilityOrgIDs); ids != nil {
			ar.VisibilityOrgIDs = ids
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

// decodeColumnConfig parses the JSON-encoded column_config column, falling
// back to the default columns when the column is null/empty/malformed. The
// defaults match the frontend's logical column names (AssetReportTable.svelte)
// so getCellValue can resolve each one without further mapping.
func decodeColumnConfig(raw sql.NullString) []string {
	if raw.Valid && raw.String != "" {
		var cols []string
		if err := json.Unmarshal([]byte(raw.String), &cols); err == nil && len(cols) > 0 {
			return cols
		}
	}
	return []string{"title", "asset_tag", "status"}
}

// loadAssetReportFields fetches the field schema for a form-mode asset report.
// Used by both ExecuteAssetReport (to validate required fields before query
// substitution) and GetAssetReportFields (to surface the schema to the
// portal-side form renderer).
func (h *PortalHandler) loadAssetReportFields(ctx context.Context, assetReportID int) ([]models.AssetReportField, error) {
	rows, err := h.db.QueryContext(ctx, `
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
		ORDER BY arf.step_number, arf.display_order, arf.id
	`, assetReportID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []models.AssetReportField
	for rows.Next() {
		var f models.AssetReportField
		if err := rows.Scan(&f.ID, &f.AssetReportID, &f.FieldIdentifier, &f.FieldType,
			&f.DisplayOrder, &f.IsRequired, &f.DisplayName, &f.Description,
			&f.StepNumber, &f.VirtualFieldType, &f.VirtualFieldOptions,
			&f.CreatedAt, &f.UpdatedAt,
			&f.FieldName, &f.FieldLabel); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// GetAssetReportFields returns the form-field schema for a form-mode asset
// report, gated by the same visibility check as the list and execute paths.
// Direct-mode reports have no field schema; the response is an empty array.
func (h *PortalHandler) GetAssetReportFields(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	reportIDStr := r.PathValue("id")
	reportID, err := strconv.Atoi(reportIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	channel := portalResult.channel

	var report struct {
		ChannelID          int
		IsActive           bool
		VisibilityGroupIDs sql.NullString
		VisibilityOrgIDs   sql.NullString
	}
	err = h.db.QueryRowContext(ctx, `
		SELECT channel_id, is_active, visibility_group_ids, visibility_org_ids
		FROM asset_reports WHERE id = ?
	`, reportID).Scan(&report.ChannelID, &report.IsActive, &report.VisibilityGroupIDs, &report.VisibilityOrgIDs)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (report.ChannelID != channel.ID || !report.IsActive)) {
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	vc := h.getPortalVisibilityContext(ctx, r, channel.ID)
	if !vc.isAdmin {
		ar := models.AssetReport{
			VisibilityGroupIDs: unmarshalIntIDs(report.VisibilityGroupIDs),
			VisibilityOrgIDs:   unmarshalIntIDs(report.VisibilityOrgIDs),
		}
		if !ar.IsVisibleTo(vc.userGroupIDs, vc.customerOrgID) {
			respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
			return
		}
	}

	fields, err := h.loadAssetReportFields(ctx, reportID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if fields == nil {
		fields = []models.AssetReportField{}
	}
	respondJSONOK(w, fields)
}
