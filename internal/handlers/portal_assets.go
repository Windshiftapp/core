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
	"windshift/internal/sanitize"
	"windshift/internal/services"
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
func readFormParams(w http.ResponseWriter, r *http.Request) (map[string]string, error) {
	if r.Method != http.MethodPost || r.Body == nil || r.ContentLength == 0 {
		return map[string]string{}, nil
	}
	var body struct {
		Params map[string]any `json:"params"`
	}
	dec := newJSONDecoder(w, r)
	dec.DisallowUnknownFields()
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

// assetReportBindingAvailable revalidates form bindings on public reads. This
// covers legacy rows and later portal/configuration-set drift even though new
// management writes are already validated.
func (h *PortalHandler) assetReportBindingAvailable(config *models.ChannelConfig, runMode string, itemTypeID, workspaceID *int) (bool, error) {
	if runMode != "form" {
		return true, nil
	}
	if itemTypeID == nil || *itemTypeID <= 0 {
		return false, nil
	}
	itemTypeExists, err := repository.NewAssetReportRepository(h.db).ItemTypeExists(*itemTypeID)
	if err != nil {
		return false, err
	}
	if !itemTypeExists {
		return false, nil
	}
	if workspaceID == nil {
		return true, nil
	}
	if *workspaceID <= 0 || !containsID(config.PortalWorkspaceIDs, *workspaceID) {
		return false, nil
	}
	return services.IsItemTypeAllowedInWorkspace(h.db, *workspaceID, *itemTypeID)
}

// resolvePortalAssetReport resolves the portal and loads an asset report
// through the gate shared by the field and execute paths: the report must
// belong to the resolved channel, be active, have a usable binding, and be
// visible to the caller. GetAssetReports mirrors the same gate when listing.
// Every failure writes a 404 (not 403) so report existence is not disclosed
// to unauthorized callers. Callers defer the returned cancel.
func (h *PortalHandler) resolvePortalAssetReport(
	w http.ResponseWriter,
	r *http.Request,
	timeout time.Duration,
) (context.Context, context.CancelFunc, *models.AssetReport, bool) {
	reportID, ok := requireIDParam(w, r, "id")
	if !ok {
		return nil, func() {}, nil, false
	}

	ctx, cancel, channel, config, ok := h.resolvePortalBySlugTimeout(w, r, timeout)
	if !ok {
		return nil, cancel, nil, false
	}

	report, err := repository.NewAssetReportRepository(h.db).GetByID(reportID)
	if errors.Is(err, repository.ErrNotFound) {
		cancel()
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
		return nil, cancel, nil, false
	}
	if err != nil {
		cancel()
		respondInternalError(w, r, err)
		return nil, cancel, nil, false
	}
	if report.ChannelID != channel.ID || !report.IsActive {
		cancel()
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
		return nil, cancel, nil, false
	}
	bindingAvailable, err := h.assetReportBindingAvailable(&config, report.RunMode, report.ItemTypeID, report.WorkspaceID)
	if err != nil {
		cancel()
		respondInternalError(w, r, err)
		return nil, cancel, nil, false
	}
	if !bindingAvailable {
		cancel()
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
		return nil, cancel, nil, false
	}

	vc := h.getPortalVisibilityContext(ctx, r, channel.ID)
	if !vc.isAdmin && !report.IsVisibleTo(vc.userGroupIDs, vc.customerOrgID) {
		cancel()
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Asset report not found"))
		return nil, cancel, nil, false
	}
	return ctx, cancel, report, true
}

// ExecuteAssetReport executes a CQL query for an asset report and returns the assets
func (h *PortalHandler) ExecuteAssetReport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, report, ok := h.resolvePortalAssetReport(w, r, 30*time.Second)
	if !ok {
		return
	}
	defer cancel()

	var portalCustomerID *int
	var customerOrgID *int
	portalCustomerID, _ = h.getPortalCustomerID(ctx, r, report.ChannelID)

	//nolint:misspell // British spelling used in database
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
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		formValues, formErr := readFormParams(w, r)
		if formErr != nil {
			if isRequestBodyTooLarge(formErr) {
				respondRequestTooLarge(w, r)
				return
			}
			respondValidationError(w, r, "Invalid request body: "+formErr.Error())
			return
		}
		// Required-field check: load the schema and reject when any required
		// field is missing or blank. This blocks empty submissions from
		// leaking the unrestricted query result.
		fields, ferr := h.loadAssetReportFields(report.ID)
		if ferr != nil {
			respondInternalError(w, r, ferr)
			return
		}
		allowedFields := make(map[string]struct{}, len(fields))
		for _, field := range fields {
			allowedFields[field.FieldIdentifier] = struct{}{}
		}
		for identifier := range formValues {
			if _, allowed := allowedFields[identifier]; !allowed {
				respondValidationError(w, r, "Unknown form field: "+identifier)
				return
			}
		}
		// Empty form submission (no params at all) on a form-mode report is
		// the "show me the form" state — return an empty result and the column
		// config so the FE can render headers without running the query.
		if len(formValues) == 0 && r.Method == http.MethodGet {
			respondJSONOK(w, map[string]any{
				"assets":                   []any{},
				"columns":                  report.ColumnConfig,
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
		userID, userErr := repository.NewPortalCustomerRepository(h.db).UserID(ctx, *portalCustomerID)
		if userErr != nil && !errors.Is(userErr, repository.ErrNotFound) {
			respondInternalError(w, r, userErr)
			return
		}
		if userID != nil {
			fnCtx.UserID = userID
		} else if strings.Contains(cqlQuery, "currentUser()") {
			// No linked user: replace with sentinel that won't match real user IDs.
			cqlQuery = strings.ReplaceAll(cqlQuery, "currentUser()", fmt.Sprintf("portal:%d", *portalCustomerID))
		}
	}
	cqlQuery = cql.SubstituteFunctions(cqlQuery, fnCtx)

	// Evaluate CQL (if any) to a SQL fragment against the assets table.
	var cqlSQL string
	var cqlArgs []any
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
		itemCustomFieldMap, icErr := repository.NewItemRepository(h.db).GetCQLCustomFieldMap()
		if icErr != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load item custom field mapping: %w", icErr))
			return
		}
		evaluator := cql.NewAssetEvaluator(setMap, workspaceMap, customFieldMap, itemCustomFieldMap, h.db.GetDriverName())
		var evalErr error
		cqlSQL, cqlArgs, evalErr = evaluator.EvaluateToSQL(cqlQuery)
		if evalErr != nil {
			respondValidationError(w, r, "CQL query error: "+evalErr.Error())
			return
		}
	}

	page := 1
	perPage := 25
	var err error
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

	columns := report.ColumnConfig

	// The report's column_config is the only mechanism limiting which custom
	// fields a portal customer may see (asset custom fields have no
	// per-field portal-visibility flag), so derive the set of permitted custom
	// field keys from the cf_<id> columns and project the stored JSON down to
	// them — never serialize fields the report did not opt into.
	allowedCustomFieldKeys := allowedPortalCustomFieldKeys(columns)

	assetRepo := repository.NewAssetRepository(h.db)
	assets, err := assetRepo.ListPortalReportAssets(ctx, report.AssetSetID, cqlSQL, cqlArgs, perPage, offset, allowedCustomFieldKeys)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	total, err := assetRepo.CountPortalReportAssets(ctx, report.AssetSetID, cqlSQL, cqlArgs)
	if err != nil {
		slog.Warn("failed to get asset count", slog.Any("error", err))
	}

	response := map[string]any{
		"assets":      assets,
		"columns":     columns,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + perPage - 1) / perPage,
	}

	respondJSONOK(w, response)
}

// allowedPortalCustomFieldKeys derives the custom-field keys a report opts
// into from its cf_<id> columns. Asset custom fields have no per-field portal
// visibility flag, so a column_config entry is the only thing that authorizes
// serializing a custom field to a portal visitor.
func allowedPortalCustomFieldKeys(columns []string) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, col := range columns {
		if strings.HasPrefix(col, "cf_") {
			allowed[strings.TrimPrefix(col, "cf_")] = struct{}{}
		}
	}
	return allowed
}

// portalExposedSetReports groups the active, visible asset reports this visitor
// can reach on the portal by asset set, dropping any set without a
// portal-access grant. A grant alone never exposes a set: the set must also be
// referenced by a report this visitor can actually see.
func (h *PortalHandler) portalExposedSetReports(
	channel models.Channel,
	config models.ChannelConfig,
	vc portalVisibilityContext,
) (map[int][]models.AssetReport, error) {
	reports, err := repository.NewAssetReportRepository(h.db).ListByChannel(channel.ID)
	if err != nil {
		return nil, err
	}
	assetRepo := repository.NewAssetRepository(h.db)
	bySet := make(map[int][]models.AssetReport)
	for _, ar := range reports {
		if !ar.IsActive {
			continue
		}
		bindingAvailable, err := h.assetReportBindingAvailable(&config, ar.RunMode, ar.ItemTypeID, ar.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if !bindingAvailable {
			continue
		}
		if !vc.isAdmin && !ar.IsVisibleTo(vc.userGroupIDs, vc.customerOrgID) {
			continue
		}
		enabled, err := assetRepo.IsSetPortalEnabled(ar.AssetSetID)
		if err != nil {
			return nil, err
		}
		if !enabled {
			continue
		}
		bySet[ar.AssetSetID] = append(bySet[ar.AssetSetID], ar)
	}
	return bySet, nil
}

// portalReportCQLSQL evaluates a report's CQL with the current portal context
// functions substituted and returns the SQL fragment plus args. An empty CQL
// yields an empty fragment that matches every asset in the set.
func (h *PortalHandler) portalReportCQLSQL(
	ctx context.Context,
	report *models.AssetReport,
	portalCustomerID, customerOrgID *int,
) (cqlSQL string, arguments []any, err error) {
	query := report.CQLQuery
	if strings.TrimSpace(query) == "" {
		return "", nil, nil
	}

	fnCtx := cql.FunctionContext{CustomerID: portalCustomerID, OrganisationID: customerOrgID}
	if portalCustomerID != nil {
		userID, err := repository.NewPortalCustomerRepository(h.db).UserID(ctx, *portalCustomerID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return "", nil, err
		}
		if userID != nil {
			fnCtx.UserID = userID
		} else if strings.Contains(query, "currentUser()") {
			query = strings.ReplaceAll(query, "currentUser()", fmt.Sprintf("portal:%d", *portalCustomerID))
		}
	}
	query = cql.SubstituteFunctions(query, fnCtx)

	assetRepo := repository.NewAssetRepository(h.db)
	setMap, err := assetRepo.GetCQLSetMap()
	if err != nil {
		return "", nil, fmt.Errorf("failed to load set mapping: %w", err)
	}
	workspaceMap, err := repository.NewWorkspaceRepository(h.db).ListNameKeyToIDMap()
	if err != nil {
		return "", nil, fmt.Errorf("failed to load workspace mapping: %w", err)
	}
	customFieldMap, err := assetRepo.GetCQLCustomFieldMap(report.AssetSetID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load custom field mapping: %w", err)
	}
	itemCustomFieldMap, err := repository.NewItemRepository(h.db).GetCQLCustomFieldMap()
	if err != nil {
		return "", nil, fmt.Errorf("failed to load item custom field mapping: %w", err)
	}
	evaluator := cql.NewAssetEvaluator(setMap, workspaceMap, customFieldMap, itemCustomFieldMap, h.db.GetDriverName())
	return evaluator.EvaluateToSQL(query)
}

// resolvePortalAsset authorizes and projects a single asset. It returns nil
// when the asset is missing, its set has no portal grant, or no visible report
// on this portal includes it — the caller must not distinguish those cases.
func (h *PortalHandler) resolvePortalAsset(
	ctx context.Context,
	reportsBySet map[int][]models.AssetReport,
	assetID int,
	portalCustomerID, customerOrgID *int,
) (*repository.PortalReportAsset, error) {
	assetSetID, err := repository.NewAssetRepository(h.db).GetAssetSetID(assetID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	reports := reportsBySet[assetSetID]
	for i := range reports {
		report := &reports[i]
		cqlSQL, cqlArgs, err := h.portalReportCQLSQL(ctx, report, portalCustomerID, customerOrgID)
		if err != nil {
			return nil, err
		}
		query := cqlSQL
		args := append([]any{}, cqlArgs...)
		if query != "" {
			query += " AND a.id = ?"
		} else {
			query = "a.id = ?"
		}
		args = append(args, assetID)
		assets, err := repository.NewAssetRepository(h.db).ListPortalReportAssets(ctx, report.AssetSetID, query, args, 1, 0, allowedPortalCustomFieldKeys(report.ColumnConfig))
		if err != nil {
			return nil, err
		}
		if len(assets) > 0 {
			return &assets[0], nil
		}
	}
	return nil, nil
}

// portalAssetAccess carries the resolved portal context shared by the asset
// read endpoints: the visible reports grouped by set, and the current
// customer identity used by CQL context functions.
type portalAssetAccess struct {
	ctx              context.Context
	reportsBySet     map[int][]models.AssetReport
	portalCustomerID *int
	customerOrgID    *int
}

// portalAssetRequestContext resolves the portal, its visibility context, and
// the current customer identity shared by the portal asset read endpoints.
// It writes the response and returns ok=false on failure.
func (h *PortalHandler) portalAssetRequestContext(w http.ResponseWriter, r *http.Request) (portalAssetAccess, context.CancelFunc, bool) {
	ctx, cancel, channel, config, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return portalAssetAccess{}, cancel, false
	}
	vc := h.getPortalVisibilityContext(ctx, r, channel.ID)
	reportsBySet, err := h.portalExposedSetReports(channel, config, vc)
	if err != nil {
		cancel()
		respondInternalError(w, r, err)
		return portalAssetAccess{}, cancel, false
	}
	portalCustomerID, _ := h.getPortalCustomerID(ctx, r, channel.ID)
	var customerOrgID *int
	if portalCustomerID != nil {
		customerOrgID = h.getPortalCustomerOrgID(ctx, *portalCustomerID)
	}
	return portalAssetAccess{
		ctx:              ctx,
		reportsBySet:     reportsBySet,
		portalCustomerID: portalCustomerID,
		customerOrgID:    customerOrgID,
	}, cancel, true
}

// GetPortalAsset returns one asset for a portal visitor through the
// portal-access + visible-report gate. Missing and unauthorized ids both
// return 404 so the endpoint is not an existence oracle.
func (h *PortalHandler) GetPortalAsset(w http.ResponseWriter, r *http.Request) {
	assetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	access, cancel, ok := h.portalAssetRequestContext(w, r)
	if !ok {
		return
	}
	defer cancel()

	asset, err := h.resolvePortalAsset(access.ctx, access.reportsBySet, assetID, access.portalCustomerID, access.customerOrgID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if asset == nil {
		respondNotFound(w, r, "asset")
		return
	}
	respondJSONOK(w, asset)
}

// PortalAssetSummaries resolves a batch of asset ids, silently omitting ids
// that are missing or not authorized for this portal. Used to prefill asset
// fields without turning a bare id into a name probe.
func (h *PortalHandler) PortalAssetSummaries(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var body struct {
		IDs []int `json:"ids"`
	}
	dec := newJSONDecoder(w, r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		respondValidationError(w, r, "Invalid request body: "+err.Error())
		return
	}
	if len(body.IDs) > 100 {
		respondValidationError(w, r, "Too many asset ids")
		return
	}

	access, cancel, ok := h.portalAssetRequestContext(w, r)
	if !ok {
		return
	}
	defer cancel()

	assets := make([]repository.PortalReportAsset, 0, len(body.IDs))
	for _, id := range body.IDs {
		if id <= 0 {
			continue
		}
		asset, err := h.resolvePortalAsset(access.ctx, access.reportsBySet, id, access.portalCustomerID, access.customerOrgID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if asset != nil {
			assets = append(assets, *asset)
		}
	}
	respondJSONOK(w, map[string]any{"assets": assets})
}

// SearchPortalAssets searches assets limited to portal-enabled sets this
// visitor can reach through a visible report, then re-checks each hit against
// those reports' CQL so the search cannot reveal assets a report hides.
func (h *PortalHandler) SearchPortalAssets(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		respondJSONOK(w, map[string]any{"assets": []any{}})
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	access, cancel, ok := h.portalAssetRequestContext(w, r)
	if !ok {
		return
	}
	defer cancel()

	setIDs := make([]int, 0, len(access.reportsBySet))
	for setID := range access.reportsBySet {
		setIDs = append(setIDs, setID)
	}
	matches, err := repository.NewAssetRepository(h.db).Search(query, setIDs, limit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	assets := make([]repository.PortalReportAsset, 0, len(matches))
	for _, match := range matches {
		asset, err := h.resolvePortalAsset(access.ctx, access.reportsBySet, match.ID, access.portalCustomerID, access.customerOrgID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if asset != nil {
			assets = append(assets, *asset)
		}
	}
	respondJSONOK(w, map[string]any{"assets": assets})
}

// GetAssetReports returns asset reports for a portal, filtered by visibility
func (h *PortalHandler) GetAssetReports(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, config, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return
	}
	defer cancel()

	vc := h.getPortalVisibilityContext(ctx, r, channel.ID)
	assetReports, err := h.loadPortalAssetReports(ctx, channel, config, vc)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, assetReports)
}

// allowedRowActionSources are the scalar asset fields a row action may pass
// through its link. Anything else is dropped before the config is exposed.
var allowedRowActionSources = map[string]bool{
	"asset_id":  true,
	"asset_tag": true,
	"title":     true,
}

// visibleRequestTypeIDs returns the active request types on a channel that the
// current portal visitor may open. Row actions targeting anything outside this
// set are dropped so a link never exposes a hidden form or its id.
func (h *PortalHandler) visibleRequestTypeIDs(ctx context.Context, channelID int, vc portalVisibilityContext) (map[int]bool, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT id, visibility_group_ids, visibility_org_ids
		FROM request_types
		WHERE channel_id = ? AND is_active = true
	`, channelID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	visible := make(map[int]bool)
	for rows.Next() {
		var id int
		var groupIDs, orgIDs sql.NullString
		if err := rows.Scan(&id, &groupIDs, &orgIDs); err != nil {
			return nil, err
		}
		groups, err := unmarshalIntIDs(groupIDs)
		if err != nil {
			return nil, fmt.Errorf("parse request type %d visibility groups: %w", id, err)
		}
		orgs, err := unmarshalIntIDs(orgIDs)
		if err != nil {
			return nil, fmt.Errorf("parse request type %d visibility organizations: %w", id, err)
		}
		rt := models.RequestType{VisibilityGroupIDs: groups, VisibilityOrgIDs: orgs}
		if vc.isAdmin || rt.IsVisibleTo(vc.userGroupIDs, vc.customerOrgID) {
			visible[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return visible, nil
}

// filterVisibleRowActions copies the row actions whose target request type is
// visible to the visitor, scrubbing the display fields and dropping malformed
// or unsupported entries.
func filterVisibleRowActions(actions []models.AssetReportRowAction, visibleTypes map[int]bool) []models.AssetReportRowAction {
	filtered := []models.AssetReportRowAction{}
	for _, action := range actions {
		if action.RequestTypeID <= 0 || !visibleTypes[action.RequestTypeID] {
			continue
		}
		if !allowedRowActionSources[action.Source] {
			continue
		}
		action.Label = strings.TrimSpace(action.Label)
		sanitize.Apply(&action.Label, sanitize.PlainTextField)
		sanitize.Apply(&action.TargetField, sanitize.ShortIdentifier)
		if action.Label == "" || action.TargetField == "" {
			continue
		}
		filtered = append(filtered, action)
	}
	return filtered
}

func (h *PortalHandler) loadPortalAssetReports(ctx context.Context, channel models.Channel, config models.ChannelConfig, vc portalVisibilityContext) ([]models.PublicAssetReport, error) {
	reports, err := repository.NewAssetReportRepository(h.db).ListByChannel(channel.ID)
	if err != nil {
		return nil, err
	}

	// Computed once, lazily, only when a report actually carries row actions.
	var visibleTypes map[int]bool

	assetReports := []models.PublicAssetReport{}
	for _, ar := range reports {
		if !ar.IsActive {
			continue
		}
		bindingAvailable, err := h.assetReportBindingAvailable(&config, ar.RunMode, ar.ItemTypeID, ar.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if !bindingAvailable {
			continue
		}

		// Admin users see all; others see only visible ones
		if vc.isAdmin || ar.IsVisibleTo(vc.userGroupIDs, vc.customerOrgID) {
			publicReport := models.PublicAssetReport{
				ID:           ar.ID,
				Name:         ar.Name,
				Description:  ar.Description,
				Icon:         ar.Icon,
				Color:        ar.Color,
				DisplayOrder: ar.DisplayOrder,
				ColumnConfig: ar.ColumnConfig,
				RunMode:      ar.RunMode,
				ItemTypeID:   ar.ItemTypeID,
				WorkspaceID:  ar.WorkspaceID,
			}
			if ar.Config != nil && *ar.Config != "" {
				var config models.RequestTypeConfig
				if err := json.Unmarshal([]byte(*ar.Config), &config); err != nil {
					slog.Warn("ignoring invalid public asset report config",
						slog.String("component", "portal_assets"), slog.Int("asset_report_id", ar.ID), slog.Any("error", err))
				} else {
					publicConfig := models.PublicAssetReportConfig{
						SuccessMessage:   config.SuccessMessage,
						SubmitButtonText: config.SubmitButtonText,
					}
					if len(config.RowActions) > 0 {
						if visibleTypes == nil {
							visibleTypes, err = h.visibleRequestTypeIDs(ctx, channel.ID, vc)
							if err != nil {
								return nil, err
							}
						}
						publicConfig.RowActions = filterVisibleRowActions(config.RowActions, visibleTypes)
					}
					if publicConfig.SuccessMessage != "" || publicConfig.SubmitButtonText != "" || len(publicConfig.RowActions) > 0 {
						publicReport.Config = &publicConfig
					}
				}
			}
			assetReports = append(assetReports, publicReport)
		}
	}
	return assetReports, nil
}

// GetRequestTypeFields returns fields for a request type (portal-aware authentication)
// Accepts either internal session OR portal customer session
func (h *PortalHandler) GetRequestTypeFields(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, _, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return
	}
	defer cancel()

	requestTypeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load the request type through the shared visibility gate; without it,
	// hidden request types can be enumerated by guessing IDs because the
	// fields endpoint would reveal their form structure.
	if _, ok := h.resolveVisibleRequestType(ctx, w, r, channel.ID, requestTypeID); !ok {
		return
	}

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
	ctx, cancel, channel, _, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return
	}
	defer cancel()

	// Resolve visibility context so we only return custom fields for request
	// types / asset reports the caller is allowed to see. Without this the
	// endpoint leaks field names, descriptions, and option vocabularies for
	// hidden request types — same gate GetRequestTypes / GetAssetReports use.
	vc := h.getPortalVisibilityContext(ctx, r, channel.ID)

	fields, err := h.portalService.GetCustomFieldsForChannel(ctx, channel.ID, vc.userGroupIDs, vc.customerOrgID, vc.isAdmin)
	if err != nil {
		slog.Error("failed to get custom fields for channel", slog.String("component", "portal"), slog.Int("channel_id", channel.ID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}

// loadAssetReportFields fetches the field schema for a form-mode asset report.
// Used by both ExecuteAssetReport (to validate required fields before query
// substitution) and GetAssetReportFields (to surface the schema to the
// portal-side form renderer).
func (h *PortalHandler) loadAssetReportFields(assetReportID int) ([]models.AssetReportField, error) {
	repo := repository.NewAssetReportRepository(h.db)
	report, err := repo.GetByID(assetReportID)
	if err != nil {
		return nil, err
	}
	allowedCustomFieldIDs := make(map[string]struct{})
	if report.ItemTypeID != nil && report.WorkspaceID != nil {
		allowedCustomFieldIDs, err = services.AllowedCreateScreenCustomFieldIdentifiers(h.db, *report.WorkspaceID, *report.ItemTypeID)
		if err != nil {
			return nil, err
		}
	}
	fields, err := repo.ListFields(assetReportID)
	if err != nil {
		return nil, err
	}
	out := make([]models.AssetReportField, 0, len(fields))
	for _, f := range fields {
		if f.FieldType == "custom" {
			if _, allowed := allowedCustomFieldIDs[f.FieldIdentifier]; !allowed {
				continue
			}
		}
		out = append(out, f)
	}
	return out, nil
}

// GetAssetReportFields returns the form-field schema for a form-mode asset
// report, gated by the same visibility check as the list and execute paths.
// Direct-mode reports have no field schema; the response is an empty array.
func (h *PortalHandler) GetAssetReportFields(w http.ResponseWriter, r *http.Request) {
	_, cancel, report, ok := h.resolvePortalAssetReport(w, r, 10*time.Second)
	if !ok {
		return
	}
	defer cancel()

	if report.RunMode != "form" {
		respondJSONOK(w, []models.AssetReportField{})
		return
	}

	fields, err := h.loadAssetReportFields(report.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if fields == nil {
		fields = []models.AssetReportField{}
	}
	respondJSONOK(w, fields)
}
