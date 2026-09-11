package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
)

// AssetHandler exposes the asset surface on the bearer-token v1 API:
// asset CRUD (within a set), set/type/category/status reads, and CSV
// import. Defense in depth — the route layer gates on assets:* token
// scopes; this handler delegates the per-set role check (Viewer / Editor
// / Administrator with asset.view/create/edit/delete/admin keys) to
// services.AssetPermissionService. 404 (not 403) on permission failures
// so set/asset existence isn't leaked, matching the items convention.
//
// Mutations route through services.AssetService so both surfaces emit
// identical audit + automation events for the same operation; no audit
// code lives in this file.
type AssetHandler struct {
	BaseHandler
	repo         *repository.AssetRepository
	assetPerm    *services.AssetPermissionService
	assetService *services.AssetService
}

// NewAssetHandler constructs a v1 AssetHandler. The caller should pass the
// same AssetPermissionService + AssetService instances the cookie-auth
// AssetHandler uses (via legacyhandlers.AssetHandler.AssetPermissionService()
// and .AssetService()) so both surfaces share one role-check pipeline and
// one mutation/audit/automation pipeline.
func NewAssetHandler(db database.Database, permissionService *services.PermissionService, assetPerm *services.AssetPermissionService, assetService *services.AssetService) *AssetHandler {
	return &AssetHandler{
		BaseHandler:  NewBaseHandler(db, permissionService),
		repo:         repository.NewAssetRepository(db),
		assetPerm:    assetPerm,
		assetService: assetService,
	}
}

// --- helpers ---

// requireSetAccess authenticates the request, parses the set ID from the
// {setId} path param, and verifies the caller has permissionKey on that
// set. 404 on missing / no-permission.
func (h *AssetHandler) requireSetAccess(w http.ResponseWriter, r *http.Request, permissionKey string) (int, *models.User, bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return 0, nil, false
	}
	setID, ok := h.ParsePathID(w, r, "setId", "asset set ID")
	if !ok {
		return 0, nil, false
	}
	allowed, err := h.assetPerm.HasAssetSetPermission(user.ID, setID, permissionKey)
	if err != nil {
		h.RespondInternalError(w, r)
		return 0, nil, false
	}
	if !allowed {
		h.RespondError(w, r, restapi.ErrAssetSetNotFound)
		return 0, nil, false
	}
	return setID, user, true
}

// requireAssetAccess authenticates the request, parses the asset ID from
// the {id} path param, resolves the asset's set, and verifies the caller
// has permissionKey on that set. Returns the asset row (fully joined) +
// the authenticated user so callers can stamp the audit actor without
// refetching. 404 on any failure so non-visible assets are
// indistinguishable from missing ones.
func (h *AssetHandler) requireAssetAccess(w http.ResponseWriter, r *http.Request, permissionKey string) (*repository.AssetRow, *models.User, bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return nil, nil, false
	}
	assetID, ok := h.ParsePathID(w, r, "id", "asset ID")
	if !ok {
		return nil, nil, false
	}
	row, err := h.repo.FindAssetFullByID(assetID)
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, restapi.ErrAssetNotFound)
		return nil, nil, false
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return nil, nil, false
	}
	allowed, err := h.assetPerm.HasAssetSetPermission(user.ID, row.SetID, permissionKey)
	if err != nil {
		h.RespondInternalError(w, r)
		return nil, nil, false
	}
	if !allowed {
		h.RespondError(w, r, restapi.ErrAssetNotFound)
		return nil, nil, false
	}
	return row, user, true
}

// requireEntityAccess resolves an arbitrary asset-domain entity (type /
// category / status) to its set via the supplied lookup function and
// runs the role check. Used by the single-entity routes that aren't
// asset-row-shaped. notFoundErr is returned both when the entity is
// missing and when the caller lacks permission.
func (h *AssetHandler) requireEntityAccess(
	w http.ResponseWriter,
	r *http.Request,
	idLabel string,
	lookupSetID func(id int) (int, error),
	notFoundErr *restapi.APIError,
) (entityID int, ok bool) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return 0, false
	}
	entityID, ok = h.ParsePathID(w, r, "id", idLabel)
	if !ok {
		return 0, false
	}
	setID, err := lookupSetID(entityID)
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, notFoundErr)
		return 0, false
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return 0, false
	}
	allowed, err := h.assetPerm.HasAssetSetPermission(user.ID, setID, services.AssetPermissionKeyView)
	if err != nil {
		h.RespondInternalError(w, r)
		return 0, false
	}
	if !allowed {
		h.RespondError(w, r, notFoundErr)
		return 0, false
	}
	return entityID, true
}

// --- asset entities ---

// List handles GET /rest/api/v1/asset-sets/{setId}/assets.
//
// @Summary      List assets in a set
// @Description  Paginated list of assets in the set, filtered by optional type/category/status/search query params. Returns 404 if the set is missing or the caller can't view it.
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Param        setId         path      int     true   "Asset set ID"
// @Param        type_id       query     int     false  "Filter by asset type ID"
// @Param        category_id   query     int     false  "Filter by asset category ID"
// @Param        status_id     query     int     false  "Filter by asset status ID"
// @Param        q             query     string  false  "Free-text title search"
// @Param        page          query     int     false  "Page (1-indexed)"
// @Param        limit         query     int     false  "Page size"
// @Success      200  {object}  restapi.PaginatedResponse
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset set not found"
// @Router       /asset-sets/{setId}/assets [get]
func (h *AssetHandler) List(w http.ResponseWriter, r *http.Request) {
	setID, _, ok := h.requireSetAccess(w, r, services.AssetPermissionKeyView)
	if !ok {
		return
	}
	pag := h.ParsePagination(r)
	filter := repository.AssetListFilter{
		SetID:                setID,
		AssetTypeID:          r.URL.Query().Get("type_id"),
		CategoryID:           r.URL.Query().Get("category_id"),
		IncludeSubcategories: r.URL.Query().Get("include_subcategories") != "false",
		StatusID:             r.URL.Query().Get("status_id"),
		Search:               r.URL.Query().Get("q"),
		Limit:                pag.Limit,
		Offset:               pag.Offset,
	}
	total, err := h.repo.CountAssets(filter)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	rows, err := h.repo.ListAssets(filter)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	items := make([]dto.AssetResponse, 0, len(rows))
	baseURL := getBaseURL(r)
	for _, row := range rows {
		asset := repository.AssetRowToModel(row)
		items = append(items, dto.MapAssetToResponse(&asset, baseURL))
	}
	h.RespondPaginated(w, items, pag, total)
}

// Get handles GET /rest/api/v1/assets/{id}.
//
// @Summary      Get an asset
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Asset ID"
// @Success      200  {object}  dto.AssetResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset not found"
// @Router       /assets/{id} [get]
func (h *AssetHandler) Get(w http.ResponseWriter, r *http.Request) {
	row, _, ok := h.requireAssetAccess(w, r, services.AssetPermissionKeyView)
	if !ok {
		return
	}
	asset := repository.AssetRowToModel(*row)
	h.RespondOK(w, dto.MapAssetToResponse(&asset, getBaseURL(r)))
}

// Create handles POST /rest/api/v1/asset-sets/{setId}/assets.
//
// @Summary      Create an asset
// @Tags         assets
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        setId  path  int                     true  "Asset set ID"
// @Param        body   body  dto.AssetCreateRequest  true  "Create payload"
// @Success      201    {object}  dto.AssetResponse
// @Failure      400    {object}  restapi.ErrorResponse
// @Failure      404    {object}  restapi.ErrorResponse  "Asset set not found"
// @Router       /asset-sets/{setId}/assets [post]
func (h *AssetHandler) Create(w http.ResponseWriter, r *http.Request) {
	setID, user, ok := h.requireSetAccess(w, r, services.AssetPermissionKeyCreate)
	if !ok {
		return
	}
	var req dto.AssetCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	if !h.ValidateRequiredString(w, r, req.Title, "title") {
		return
	}
	if req.AssetTypeID <= 0 {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "asset_type_id is required"))
		return
	}
	if ok := h.validateResourceBelongsToSet(w, r, "asset_types", req.AssetTypeID, setID, "Asset type"); !ok {
		return
	}
	if req.CategoryID != nil {
		if ok := h.validateResourceBelongsToSet(w, r, "asset_categories", *req.CategoryID, setID, "Asset category"); !ok {
			return
		}
	}
	if req.StatusID != nil {
		if ok := h.validateResourceBelongsToSet(w, r, "asset_statuses", *req.StatusID, setID, "Asset status"); !ok {
			return
		}
	}
	cfJSON, err := encodeCustomFieldValues(req.CustomFieldValues)
	if err != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid custom_field_values"))
		return
	}
	asset, err := h.assetService.CreateAsset(
		services.NewAuditActorFromRequest(r, user, middleware.GetAPIToken(r.Context()), "bearer"),
		repository.CreateAssetInput{
			SetID:                 setID,
			AssetTypeID:           req.AssetTypeID,
			CategoryID:            req.CategoryID,
			StatusID:              req.StatusID,
			Title:                 strings.TrimSpace(req.Title),
			Description:           req.Description,
			AssetTag:              req.AssetTag,
			CustomFieldValuesJSON: cfJSON,
			CreatedBy:             user.ID,
			CreatedAt:             time.Now().UTC(),
		},
		req.CustomFieldValues,
	)
	if ve, ok := services.IsAssetValidationError(err); ok {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, ve.Msg))
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondCreated(w, dto.MapAssetToResponse(asset, getBaseURL(r)))
}

// Update handles PUT /rest/api/v1/assets/{id}. Partial — only fields present
// in the body are written; everything else is preserved from the current row.
//
// @Summary      Update an asset
// @Tags         assets
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int                     true  "Asset ID"
// @Param        body  body  dto.AssetUpdateRequest  true  "Partial-update payload"
// @Success      200   {object}  dto.AssetResponse
// @Failure      404   {object}  restapi.ErrorResponse  "Asset not found"
// @Router       /assets/{id} [put]
func (h *AssetHandler) Update(w http.ResponseWriter, r *http.Request) {
	row, user, ok := h.requireAssetAccess(w, r, services.AssetPermissionKeyEdit)
	if !ok {
		return
	}
	var req dto.AssetUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	current := repository.AssetRowToModel(*row)
	in := repository.UpdateAssetInput{
		AssetTypeID: current.AssetTypeID,
		CategoryID:  current.CategoryID,
		StatusID:    current.StatusID,
		Title:       current.Title,
		Description: current.Description,
		AssetTag:    current.AssetTag,
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "title cannot be blank"))
			return
		}
		in.Title = title
	}
	if req.Description != nil {
		in.Description = *req.Description
	}
	if req.AssetTag != nil {
		in.AssetTag = *req.AssetTag
	}
	if req.AssetTypeID != nil {
		if *req.AssetTypeID <= 0 {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "asset_type_id must be positive"))
			return
		}
		if ok := h.validateResourceBelongsToSet(w, r, "asset_types", *req.AssetTypeID, row.SetID, "Asset type"); !ok {
			return
		}
		in.AssetTypeID = *req.AssetTypeID
	}
	if req.CategoryID != nil {
		if *req.CategoryID == 0 {
			in.CategoryID = nil
		} else {
			if ok := h.validateResourceBelongsToSet(w, r, "asset_categories", *req.CategoryID, row.SetID, "Asset category"); !ok {
				return
			}
			cid := *req.CategoryID
			in.CategoryID = &cid
		}
	}
	if req.StatusID != nil {
		if *req.StatusID == 0 {
			in.StatusID = nil
		} else {
			if ok := h.validateResourceBelongsToSet(w, r, "asset_statuses", *req.StatusID, row.SetID, "Asset status"); !ok {
				return
			}
			sid := *req.StatusID
			in.StatusID = &sid
		}
	}
	// Validate supplied custom fields; preserve stored values when omitted.
	var suppliedCustomFields map[string]any
	if req.CustomFieldValues != nil {
		cfJSON, err := encodeCustomFieldValues(*req.CustomFieldValues)
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid custom_field_values"))
			return
		}
		in.CustomFieldValuesJSON = cfJSON
		suppliedCustomFields = *req.CustomFieldValues
	} else if current.CustomFieldValues != nil {
		cfJSON, err := encodeCustomFieldValues(current.CustomFieldValues)
		if err == nil {
			in.CustomFieldValuesJSON = cfJSON
		}
	}
	snap := row.UpdateSnapshot()
	asset, err := h.assetService.UpdateAsset(
		services.NewAuditActorFromRequest(r, user, middleware.GetAPIToken(r.Context()), "bearer"),
		row.ID,
		snap,
		in,
		suppliedCustomFields,
	)
	if errors.Is(err, services.ErrAssetVersionConflict) {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, err.Error()))
		return
	}
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, restapi.ErrAssetNotFound)
		return
	}
	if ve, ok := services.IsAssetValidationError(err); ok {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, ve.Msg))
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, dto.MapAssetToResponse(asset, getBaseURL(r)))
}

// Delete handles DELETE /rest/api/v1/assets/{id}. Removes the asset and
// any item_links rows pointing at it.
//
// @Summary      Delete an asset
// @Tags         assets
// @Security     BearerAuth
// @Param        id  path  int  true  "Asset ID"
// @Success      204
// @Failure      404  {object}  restapi.ErrorResponse  "Asset not found"
// @Router       /assets/{id} [delete]
func (h *AssetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	row, user, ok := h.requireAssetAccess(w, r, services.AssetPermissionKeyDelete)
	if !ok {
		return
	}
	err := h.assetService.DeleteAsset(services.NewAuditActorFromRequest(r, user, middleware.GetAPIToken(r.Context()), "bearer"), row.ID)
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, restapi.ErrAssetNotFound)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondNoContent(w)
}

// ImportCSV handles POST /rest/api/v1/asset-sets/{setId}/assets/import.
// Synchronous one-shot: parses the multipart CSV, creates rows in-band,
// and returns a summary in the AssetImportJob shape. Each header column is
// matched against built-in fields ("title", "description", "asset_tag")
// and falls back to a case-insensitive match against the asset type's
// custom field names. Rows missing the required title are counted as
// errors but don't abort the import — partial-success is reported in
// the response (errors_rows > 0).
//
// @Summary      Import assets from CSV (sync, one-shot)
// @Description  multipart/form-data: file=<csv>, asset_type_id=N (required), status_id=M (optional), category_id=K (optional). The CSV must have a header row.
// @Tags         assets
// @Accept       mpfd
// @Produce      json
// @Security     BearerAuth
// @Param        setId         path      int   true   "Asset set ID"
// @Param        file          formData  file  true   "CSV file"
// @Param        asset_type_id formData  int   true   "Asset type id"
// @Param        status_id     formData  int   false  "Default status id"
// @Param        category_id   formData  int   false  "Default category id"
// @Success      201  {object}  dto.AssetImportJobResponse
// @Failure      400  {object}  restapi.ErrorResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset set not found"
// @Router       /asset-sets/{setId}/assets/import [post]
func (h *AssetHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	setID, user, ok := h.requireSetAccess(w, r, services.AssetPermissionKeyCreate)
	if !ok {
		return
	}
	// 32 MiB cap is generous for one-shot CSV imports; rows > that should
	// use the (future) async batch flow rather than this endpoint.
	const maxImportBytes = 32 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	// #nosec G120 -- the body is already capped by MaxBytesReader above; the int arg is the in-memory threshold, not the upper bound
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid multipart body"))
		return
	}
	assetTypeID, err := strconv.Atoi(r.FormValue("asset_type_id"))
	if err != nil || assetTypeID <= 0 {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "asset_type_id is required"))
		return
	}
	if !h.validateResourceBelongsToSet(w, r, "asset_types", assetTypeID, setID, "Asset type") {
		return
	}

	var statusID *int
	if s := r.FormValue("status_id"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			if !h.validateResourceBelongsToSet(w, r, "asset_statuses", v, setID, "Asset status") {
				return
			}
			statusID = &v
		}
	}
	var categoryID *int
	if c := r.FormValue("category_id"); c != "" {
		if v, err := strconv.Atoi(c); err == nil && v > 0 {
			if !h.validateResourceBelongsToSet(w, r, "asset_categories", v, setID, "Asset category") {
				return
			}
			categoryID = &v
		}
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "file is required"))
		return
	}
	defer func() { _ = file.Close() }()

	filename := ""
	if header != nil {
		filename = header.Filename
	}
	summary, err := h.assetService.ImportAssetsCSV(
		services.NewAuditActorFromRequest(r, user, middleware.GetAPIToken(r.Context()), "bearer"),
		setID,
		assetTypeID,
		services.ImportCSVDefaults{StatusID: statusID, CategoryID: categoryID},
		file,
		filename,
	)
	if ve, ok := services.IsAssetValidationError(err); ok {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, ve.Msg))
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	startedAt := summary.StartedAt
	completedAt := summary.CompletedAt
	job := dto.AssetImportJobResponse{
		SetID:         summary.SetID,
		AssetTypeID:   summary.AssetTypeID,
		Status:        summary.Status,
		TotalRows:     summary.TotalRows,
		ProcessedRows: summary.ProcessedRows,
		CreatedRows:   summary.CreatedRows,
		ErrorRows:     summary.ErrorRows,
		ErrorMessage:  summary.ErrorMessage,
		CreatedAt:     startedAt,
		StartedAt:     &startedAt,
		CompletedAt:   &completedAt,
	}
	h.RespondCreated(w, job)
}

// --- asset sets ---

// ListSets handles GET /rest/api/v1/asset-sets.
//
// @Summary      List asset sets visible to the caller
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}  dto.AssetSetResponse
// @Failure      401  {object}  restapi.ErrorResponse
// @Router       /asset-sets [get]
func (h *AssetHandler) ListSets(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	isAdmin, _ := h.PermissionService.HasGlobalPermission(user.ID, "system.admin")
	sets, err := h.repo.ListSetsForUser(user.ID, isAdmin)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	baseURL := getBaseURL(r)
	out := make([]dto.AssetSetResponse, 0, len(sets))
	for i := range sets {
		out = append(out, dto.MapAssetSetToResponse(&sets[i], baseURL))
	}
	h.RespondOK(w, out)
}

// GetSet handles GET /rest/api/v1/asset-sets/{setId}.
//
// @Summary      Get an asset set
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Param        setId  path  int  true  "Asset set ID"
// @Success      200  {object}  dto.AssetSetResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset set not found"
// @Router       /asset-sets/{setId} [get]
func (h *AssetHandler) GetSet(w http.ResponseWriter, r *http.Request) {
	setID, _, ok := h.requireSetAccess(w, r, services.AssetPermissionKeyView)
	if !ok {
		return
	}
	set, err := h.repo.GetSetByID(setID)
	if errors.Is(err, repository.ErrNotFound) || set == nil {
		h.RespondError(w, r, restapi.ErrAssetSetNotFound)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, dto.MapAssetSetToResponse(set, getBaseURL(r)))
}

// --- asset types (read-only on v1) ---

// ListTypes handles GET /rest/api/v1/asset-sets/{setId}/types.
//
// @Summary      List asset types in a set
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Param        setId  path  int  true  "Asset set ID"
// @Success      200  {array}  dto.AssetTypeResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset set not found"
// @Router       /asset-sets/{setId}/types [get]
func (h *AssetHandler) ListTypes(w http.ResponseWriter, r *http.Request) {
	setID, _, ok := h.requireSetAccess(w, r, services.AssetPermissionKeyView)
	if !ok {
		return
	}
	types, err := h.repo.FindAssetTypesForSet(setID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	out := make([]dto.AssetTypeResponse, 0, len(types))
	for i := range types {
		fields, err := h.repo.FindAssetTypeFields(types[i].ID)
		if err != nil {
			h.RespondInternalError(w, r)
			return
		}
		types[i].Fields = fields
		out = append(out, dto.MapAssetTypeToResponse(&types[i]))
	}
	h.RespondOK(w, out)
}

// GetType handles GET /rest/api/v1/asset-types/{id}.
//
// @Summary      Get an asset type
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "Asset type ID"
// @Success      200  {object}  dto.AssetTypeResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset type not found"
// @Router       /asset-types/{id} [get]
func (h *AssetHandler) GetType(w http.ResponseWriter, r *http.Request) {
	typeID, ok := h.requireEntityAccess(w, r, "asset type ID", h.repo.GetAssetTypeSetID, restapi.ErrAssetTypeNotFound)
	if !ok {
		return
	}
	at, err := h.repo.FindAssetTypeByID(typeID)
	if errors.Is(err, repository.ErrNotFound) || at == nil {
		h.RespondError(w, r, restapi.ErrAssetTypeNotFound)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	fields, err := h.repo.FindAssetTypeFields(typeID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	at.Fields = fields
	h.RespondOK(w, dto.MapAssetTypeToResponse(at))
}

// --- asset categories (read-only on v1) ---

// ListCategories handles GET /rest/api/v1/asset-sets/{setId}/categories.
//
// @Summary      List asset categories in a set
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Param        setId  path  int  true  "Asset set ID"
// @Success      200  {array}  dto.AssetCategoryResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset set not found"
// @Router       /asset-sets/{setId}/categories [get]
func (h *AssetHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	setID, _, ok := h.requireSetAccess(w, r, services.AssetPermissionKeyView)
	if !ok {
		return
	}
	cats, err := h.repo.FindAssetCategoriesForSet(setID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	out := make([]dto.AssetCategoryResponse, 0, len(cats))
	for i := range cats {
		out = append(out, dto.MapAssetCategoryToResponse(&cats[i]))
	}
	h.RespondOK(w, out)
}

// GetCategory handles GET /rest/api/v1/asset-categories/{id}.
//
// @Summary      Get an asset category
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "Asset category ID"
// @Success      200  {object}  dto.AssetCategoryResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset category not found"
// @Router       /asset-categories/{id} [get]
func (h *AssetHandler) GetCategory(w http.ResponseWriter, r *http.Request) {
	catID, ok := h.requireEntityAccess(w, r, "asset category ID", h.repo.GetAssetCategorySetID, restapi.ErrAssetCategoryNotFound)
	if !ok {
		return
	}
	c, err := h.repo.FindAssetCategoryByID(catID)
	if errors.Is(err, repository.ErrNotFound) || c == nil {
		h.RespondError(w, r, restapi.ErrAssetCategoryNotFound)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, dto.MapAssetCategoryToResponse(c))
}

// --- asset statuses (read-only on v1) ---

// ListStatuses handles GET /rest/api/v1/asset-sets/{setId}/statuses.
//
// @Summary      List asset statuses in a set
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Param        setId  path  int  true  "Asset set ID"
// @Success      200  {array}  dto.AssetStatusResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset set not found"
// @Router       /asset-sets/{setId}/statuses [get]
func (h *AssetHandler) ListStatuses(w http.ResponseWriter, r *http.Request) {
	setID, _, ok := h.requireSetAccess(w, r, services.AssetPermissionKeyView)
	if !ok {
		return
	}
	statuses, err := h.repo.FindAssetStatusesForSet(setID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	out := make([]dto.AssetStatusResponse, 0, len(statuses))
	for i := range statuses {
		out = append(out, dto.MapAssetStatusToResponse(&statuses[i]))
	}
	h.RespondOK(w, out)
}

// GetStatus handles GET /rest/api/v1/asset-statuses/{id}.
//
// @Summary      Get an asset status
// @Tags         assets
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "Asset status ID"
// @Success      200  {object}  dto.AssetStatusResponse
// @Failure      404  {object}  restapi.ErrorResponse  "Asset status not found"
// @Router       /asset-statuses/{id} [get]
func (h *AssetHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	statusID, ok := h.requireEntityAccess(w, r, "asset status ID", h.repo.GetAssetStatusSetID, restapi.ErrAssetStatusNotFound)
	if !ok {
		return
	}
	s, err := h.repo.FindAssetStatusByID(statusID)
	if errors.Is(err, repository.ErrNotFound) || s == nil {
		h.RespondError(w, r, restapi.ErrAssetStatusNotFound)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, dto.MapAssetStatusToResponse(s))
}

// --- helpers ---

// validateResourceBelongsToSet checks that the given resource (asset_types
// / asset_categories / asset_statuses) belongs to setID. Writes a 400
// validation error on mismatch and returns false; returns true on match.
func (h *AssetHandler) validateResourceBelongsToSet(w http.ResponseWriter, r *http.Request, table string, resourceID, setID int, resourceName string) bool {
	resSetID, err := h.repo.GetResourceSetID(table, resourceID)
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, resourceName+" not found"))
		return false
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return false
	}
	if resSetID != setID {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, resourceName+" does not belong to this asset set"))
		return false
	}
	return true
}

// encodeCustomFieldValues marshals the values map for storage. Returns
// nil pointer for nil / empty maps so the column stores NULL rather than
// "null" / "{}".
func encodeCustomFieldValues(m map[string]any) (*string, error) {
	if len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}
