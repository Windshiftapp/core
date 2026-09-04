package v2

import (
	"errors"
	"io"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type assetSetCreate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}
type assetSetPatch struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsDefault   *bool   `json:"is_default"`
}
type assetTypeCreate struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	Color        string `json:"color"`
	DisplayOrder int    `json:"display_order"`
	IsActive     *bool  `json:"is_active"`
}
type assetTypePatch struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	Icon         *string `json:"icon"`
	Color        *string `json:"color"`
	DisplayOrder *int    `json:"display_order"`
	IsActive     *bool   `json:"is_active"`
}
type assetCategoryCreate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    *int   `json:"parent_id"`
}
type assetCategoryPatch struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}
type assetCategoryMove struct {
	ParentID *int `json:"parent_id"`
}
type assetStatusCreate struct {
	Name         string `json:"name"`
	Color        string `json:"color"`
	Description  string `json:"description"`
	IsDefault    bool   `json:"is_default"`
	DisplayOrder int    `json:"display_order"`
}
type assetStatusPatch struct {
	Name         *string `json:"name"`
	Color        *string `json:"color"`
	Description  *string `json:"description"`
	IsDefault    *bool   `json:"is_default"`
	DisplayOrder *int    `json:"display_order"`
}
type assetInput struct {
	AssetTypeID       int            `json:"asset_type_id"`
	CategoryID        *int           `json:"category_id"`
	StatusID          *int           `json:"status_id"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	AssetTag          string         `json:"asset_tag"`
	CustomFieldValues map[string]any `json:"custom_field_values"`
}
type assetPatchInput struct {
	AssetTypeID       Optional[int]            `json:"asset_type_id"`
	CategoryID        Optional[int]            `json:"category_id"`
	StatusID          Optional[int]            `json:"status_id"`
	Title             Optional[string]         `json:"title"`
	Description       Optional[string]         `json:"description"`
	AssetTag          Optional[string]         `json:"asset_tag"`
	CustomFieldValues Optional[map[string]any] `json:"custom_field_values"`
}
type assetTypeFieldsInput struct {
	Fields []struct {
		CustomFieldID int  `json:"custom_field_id"`
		IsRequired    bool `json:"is_required"`
		DisplayOrder  int  `json:"display_order"`
	} `json:"fields"`
}
type assetRoleInput struct {
	UserID  *int `json:"user_id"`
	GroupID *int `json:"group_id"`
	RoleID  int  `json:"role_id"`
}
type everyoneRoleInput struct {
	RoleID *int `json:"role_id"`
}
type assetRoleAssignmentResponse struct {
	Assigned bool `json:"assigned"`
}
type assetLinkInput struct {
	LinkTypeID int    `json:"link_type_id"`
	TargetType string `json:"target_type"`
	TargetID   int    `json:"target_id"`
}
type assetImportSuggestionsInput struct {
	UploadID  string `json:"upload_id"`
	HasHeader bool   `json:"has_header"`
	Delimiter string `json:"delimiter"`
}
type assetImportUploadForm struct {
	File      []byte `json:"file"`
	HasHeader bool   `json:"has_header"`
	Delimiter string `json:"delimiter"`
}

func registerAssetRoutes(builder *routeBuilder, app *services.AssetApplicationService) {
	registerAssetSetRoutes(builder, app)
	registerAssetTypeRoutes(builder, app)
	registerAssetCategoryRoutes(builder, app)
	registerAssetStatusRoutes(builder, app)
	registerAssetCRUDRoutes(builder, app)
	registerAssetImportRoutes(builder, app)
}

func registerAssetImportRoutes(builder *routeBuilder, app *services.AssetApplicationService) {
	base := "/asset-sets/{asset_set_id}/import"
	builder.RawDocument[assetImportUploadForm, services.AssetCSVUpload](http.MethodPost, base+"/upload", http.StatusCreated, "multipart/form-data", AuthAuthenticated, []string{"assets:write"}, func(w http.ResponseWriter, r *http.Request) error {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return err
		}
		const maxBody = (50 << 20) + (1 << 20)
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		// The request body is bounded above; ParseMultipartForm's argument only controls memory use.
		//nolint:gosec // G120 does not recognize the MaxBytesReader assignment.
		if err := r.ParseMultipartForm(maxBody); err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "Invalid multipart upload")
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "file is required")
		}
		defer func() { _ = file.Close() }()
		result, err := app.UploadCSV(user.ID, setID, header.Filename, r.FormValue("has_header") != "false", r.FormValue("delimiter"), io.LimitReader(file, maxBody))
		if err != nil {
			return assetError(err)
		}
		return writeDocument(w, http.StatusCreated, result)
	})
	builder.JSON(http.MethodPost, base+"/start", http.StatusAccepted, false, AuthAuthenticated, []string{"assets:write"}, func(r *http.Request, input services.StartAssetImport) (services.AssetImportJob, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return services.AssetImportJob{}, err
		}
		result, err := app.StartImport(user.ID, setID, auditActor(r, user), input)
		return result, assetError(err)
	})
	builder.Read(base+"/jobs", AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) ([]services.AssetImportJob, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ListImportJobs(user.ID, setID)
		return result, assetError(err)
	})
	builder.Read(base+"/jobs/{job_id}", AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (services.AssetImportJob, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return services.AssetImportJob{}, err
		}
		result, err := app.GetImportJob(user.ID, setID, r.PathValue("job_id"))
		return result, assetError(err)
	})
	builder.JSON(http.MethodPost, base+"/suggest-fields", http.StatusOK, false, AuthAuthenticated, []string{"assets:write"}, func(r *http.Request, input assetImportSuggestionsInput) (services.AssetImportFieldSuggestions, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return services.AssetImportFieldSuggestions{}, err
		}
		result, err := app.SuggestImportFields(user.ID, setID, input.UploadID, input.HasHeader, input.Delimiter)
		return result, assetError(err)
	})
	builder.JSON(http.MethodPost, base+"/create-type", http.StatusCreated, false, AuthAuthenticated, []string{"assets:write"}, func(r *http.Request, input services.AssetImportTypeInput) (services.AssetImportTypeResult, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return services.AssetImportTypeResult{}, err
		}
		result, err := app.CreateTypeFromImport(user.ID, setID, auditActor(r, user), input)
		return result, assetError(err)
	})
}

func registerAssetSetRoutes(builder *routeBuilder, app *services.AssetApplicationService) {
	path := "/asset-sets"
	builder.Page(path, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) ([]models.AssetManagementSet, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		items, err := app.ListSets(user.ID)
		return pageValues(items, page), page, len(items), assetError(err)
	})
	builder.SessionJSON(http.MethodPost, path, http.StatusCreated, false, func(r *http.Request, input assetSetCreate) (*models.AssetManagementSet, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		result, err := app.CreateSet(user.ID, auditActor(r, user), models.AssetManagementSet{Name: input.Name, Description: input.Description, IsDefault: input.IsDefault})
		return result, assetError(err)
	})
	builder.Read(path+"/{asset_set_id}", AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (*models.AssetManagementSet, error) {
		user, id, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, err
		}
		result, err := app.GetSet(user.ID, id)
		return result, assetError(err)
	})
	builder.SessionJSON(http.MethodPatch, path+"/{asset_set_id}", http.StatusOK, true, func(r *http.Request, input assetSetPatch) (*models.AssetManagementSet, error) {
		user, id, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateSet(user.ID, id, auditActor(r, user), services.AssetSetPatch{Name: input.Name, Description: input.Description, IsDefault: input.IsDefault})
		return result, assetError(err)
	})
	builder.SessionCommand(http.MethodDelete, path+"/{asset_set_id}", func(r *http.Request) error {
		user, id, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return err
		}
		return assetError(app.DeleteSet(user.ID, id, auditActor(r, user)))
	})
	builder.Read("/asset-roles", AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) ([]models.AssetRole, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		result, err := app.ListRoles(user.ID)
		return result, assetError(err)
	})
	builder.Read("/asset-roles/{role_id}", AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (*models.AssetRole, error) {
		user, id, err := assetTarget(r, "role_id")
		if err != nil {
			return nil, err
		}
		result, err := app.GetRole(user.ID, id)
		return result, assetError(err)
	})
	rolesPath := path + "/{asset_set_id}/roles"
	builder.Read(rolesPath, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (services.AssetSetRoles, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return services.AssetSetRoles{}, err
		}
		result, err := app.SetRoles(user.ID, setID)
		return result, assetError(err)
	})
	builder.SessionJSON(http.MethodPost, rolesPath, http.StatusCreated, false, func(r *http.Request, input assetRoleInput) (assetRoleAssignmentResponse, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return assetRoleAssignmentResponse{}, err
		}
		err = app.AssignRole(user.ID, setID, auditActor(r, user), services.AssetRoleAssignment{UserID: input.UserID, GroupID: input.GroupID, RoleID: input.RoleID})
		err = assetError(err)
		return assetRoleAssignmentResponse{Assigned: err == nil}, err
	})
	builder.SessionCommand(http.MethodDelete, rolesPath+"/{assignment_id}", func(r *http.Request) error {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return err
		}
		assignmentID, err := pathID(r, "assignment_id")
		if err != nil {
			return err
		}
		return assetError(app.RevokeRole(user.ID, setID, assignmentID, r.URL.Query().Get("type"), auditActor(r, user)))
	})
	everyonePath := path + "/{asset_set_id}/everyone-role"
	builder.Read(everyonePath, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (*models.AssetSetEveryoneRole, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, err
		}
		result, err := app.EveryoneRole(user.ID, setID)
		return result, assetError(err)
	})
	builder.SessionJSON(http.MethodPut, everyonePath, http.StatusOK, false, func(r *http.Request, input everyoneRoleInput) (*models.AssetSetEveryoneRole, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, err
		}
		result, err := app.SetEveryoneRole(user.ID, setID, input.RoleID, auditActor(r, user))
		return result, assetError(err)
	})
}

func registerAssetTypeRoutes(builder *routeBuilder, app *services.AssetApplicationService) {
	collection := "/asset-sets/{asset_set_id}/types"
	item := "/asset-types/{asset_type_id}"
	builder.Page(collection, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) ([]models.AssetType, Pagination, int, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		items, err := app.ListTypes(user.ID, setID)
		return pageValues(items, page), page, len(items), assetError(err)
	})
	builder.SessionJSON(http.MethodPost, collection, http.StatusCreated, false, func(r *http.Request, input assetTypeCreate) (*models.AssetType, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, err
		}
		active := true
		if input.IsActive != nil {
			active = *input.IsActive
		}
		result, err := app.CreateType(user.ID, setID, auditActor(r, user), models.AssetType{Name: input.Name, Description: input.Description, Icon: input.Icon, Color: input.Color, DisplayOrder: input.DisplayOrder, IsActive: active})
		return result, assetError(err)
	})
	builder.Read(item, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (*models.AssetType, error) {
		user, id, err := assetTarget(r, "asset_type_id")
		if err != nil {
			return nil, err
		}
		result, err := app.GetType(user.ID, id)
		return result, assetError(err)
	})
	builder.SessionJSON(http.MethodPatch, item, http.StatusOK, true, func(r *http.Request, input assetTypePatch) (*models.AssetType, error) {
		user, id, err := assetTarget(r, "asset_type_id")
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateType(user.ID, id, auditActor(r, user), services.AssetTypePatch{Name: input.Name, Description: input.Description, Icon: input.Icon, Color: input.Color, DisplayOrder: input.DisplayOrder, IsActive: input.IsActive})
		return result, assetError(err)
	})
	builder.SessionCommand(http.MethodDelete, item, func(r *http.Request) error {
		user, id, err := assetTarget(r, "asset_type_id")
		if err != nil {
			return err
		}
		return assetError(app.DeleteType(user.ID, id, auditActor(r, user)))
	})
	builder.Read(item+"/fields", AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) ([]models.AssetTypeField, error) {
		user, id, err := assetTarget(r, "asset_type_id")
		if err != nil {
			return nil, err
		}
		result, err := app.TypeFields(user.ID, id)
		return result, assetError(err)
	})
	builder.SessionJSON(http.MethodPut, item+"/fields", http.StatusOK, false, func(r *http.Request, input assetTypeFieldsInput) ([]models.AssetTypeField, error) {
		user, id, err := assetTarget(r, "asset_type_id")
		if err != nil {
			return nil, err
		}
		fields := make([]repository.AssetTypeFieldAssignment, len(input.Fields))
		for i, field := range input.Fields {
			fields[i] = repository.AssetTypeFieldAssignment{CustomFieldID: field.CustomFieldID, IsRequired: field.IsRequired, DisplayOrder: field.DisplayOrder}
		}
		result, err := app.ReplaceTypeFields(user.ID, id, fields)
		return result, assetError(err)
	})
}

func registerAssetCategoryRoutes(builder *routeBuilder, app *services.AssetApplicationService) {
	collection := "/asset-sets/{asset_set_id}/categories"
	item := "/asset-categories/{category_id}"
	builder.Page(collection, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) ([]models.AssetCategory, Pagination, int, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		items, err := app.ListCategories(user.ID, setID, r.URL.Query().Get("tree") == "true")
		return pageValues(items, page), page, len(items), assetError(err)
	})
	builder.SessionJSON(http.MethodPost, collection, http.StatusCreated, false, func(r *http.Request, input assetCategoryCreate) (*models.AssetCategory, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, err
		}
		result, err := app.CreateCategory(user.ID, setID, auditActor(r, user), models.AssetCategory{Name: input.Name, Description: input.Description, ParentID: input.ParentID})
		return result, assetError(err)
	})
	builder.Read(item, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (*models.AssetCategory, error) {
		user, id, err := assetTarget(r, "category_id")
		if err != nil {
			return nil, err
		}
		result, err := app.GetCategory(user.ID, id)
		return result, assetError(err)
	})
	builder.SessionJSON(http.MethodPatch, item, http.StatusOK, true, func(r *http.Request, input assetCategoryPatch) (*models.AssetCategory, error) {
		user, id, err := assetTarget(r, "category_id")
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateCategory(user.ID, id, auditActor(r, user), services.AssetCategoryPatch{Name: input.Name, Description: input.Description})
		return result, assetError(err)
	})
	builder.SessionCommand(http.MethodDelete, item, func(r *http.Request) error {
		user, id, err := assetTarget(r, "category_id")
		if err != nil {
			return err
		}
		return assetError(app.DeleteCategory(user.ID, id, auditActor(r, user)))
	})
	builder.SessionJSON(http.MethodPost, item+"/move", http.StatusOK, false, func(r *http.Request, input assetCategoryMove) (*models.AssetCategory, error) {
		user, id, err := assetTarget(r, "category_id")
		if err != nil {
			return nil, err
		}
		result, err := app.MoveCategory(user.ID, id, input.ParentID)
		return result, assetError(err)
	})
}

func registerAssetStatusRoutes(builder *routeBuilder, app *services.AssetApplicationService) {
	collection := "/asset-sets/{asset_set_id}/statuses"
	item := "/asset-statuses/{status_id}"
	builder.Page(collection, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) ([]models.AssetStatus, Pagination, int, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		items, err := app.ListStatuses(user.ID, setID)
		return pageValues(items, page), page, len(items), assetError(err)
	})
	builder.SessionJSON(http.MethodPost, collection, http.StatusCreated, false, func(r *http.Request, input assetStatusCreate) (*models.AssetStatus, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, err
		}
		result, err := app.CreateStatus(user.ID, setID, auditActor(r, user), models.AssetStatus{Name: input.Name, Color: input.Color, Description: input.Description, IsDefault: input.IsDefault, DisplayOrder: input.DisplayOrder})
		return result, assetError(err)
	})
	builder.Read(item, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (*models.AssetStatus, error) {
		user, id, err := assetTarget(r, "status_id")
		if err != nil {
			return nil, err
		}
		result, err := app.GetStatus(user.ID, id)
		return result, assetError(err)
	})
	builder.SessionJSON(http.MethodPatch, item, http.StatusOK, true, func(r *http.Request, input assetStatusPatch) (*models.AssetStatus, error) {
		user, id, err := assetTarget(r, "status_id")
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateStatus(user.ID, id, auditActor(r, user), services.AssetStatusPatch{Name: input.Name, Color: input.Color, Description: input.Description, IsDefault: input.IsDefault, DisplayOrder: input.DisplayOrder})
		return result, assetError(err)
	})
	builder.SessionCommand(http.MethodDelete, item, func(r *http.Request) error {
		user, id, err := assetTarget(r, "status_id")
		if err != nil {
			return err
		}
		return assetError(app.DeleteStatus(user.ID, id, auditActor(r, user)))
	})
}

func registerAssetCRUDRoutes(builder *routeBuilder, app *services.AssetApplicationService) {
	collection := "/asset-sets/{asset_set_id}/assets"
	item := "/assets/{asset_id}"
	builder.Page(collection, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) ([]models.Asset, Pagination, int, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		items, total, err := app.ListAssets(user.ID, setID, repository.AssetListFilter{AssetTypeID: r.URL.Query().Get("type_id"), CategoryID: r.URL.Query().Get("category_id"), IncludeSubcategories: r.URL.Query().Get("include_subcategories") != "false", StatusID: r.URL.Query().Get("status_id"), Search: r.URL.Query().Get("search"), Limit: page.PageSize, Offset: page.Offset}, r.URL.Query().Get("ql"))
		return items, page, total, assetError(err)
	})
	builder.JSON(http.MethodPost, collection, http.StatusCreated, false, AuthAuthenticated, []string{"assets:write"}, func(r *http.Request, input assetInput) (*models.Asset, error) {
		user, setID, err := assetTarget(r, "asset_set_id")
		if err != nil {
			return nil, err
		}
		result, err := app.CreateAsset(user.ID, setID, auditActor(r, user), assetMutation(input))
		return result, assetError(err)
	})
	builder.Read(item, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (*models.Asset, error) {
		user, id, err := assetTarget(r, "asset_id")
		if err != nil {
			return nil, err
		}
		result, err := app.GetAsset(user.ID, id)
		return result, assetError(err)
	})
	builder.JSON(http.MethodPatch, item, http.StatusOK, true, AuthAuthenticated, []string{"assets:write"}, func(r *http.Request, input assetPatchInput) (*models.Asset, error) {
		user, id, err := assetTarget(r, "asset_id")
		if err != nil {
			return nil, err
		}
		patch, err := assetPatch(input)
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateAsset(user.ID, id, auditActor(r, user), patch)
		return result, assetError(err)
	})
	builder.Command(http.MethodDelete, item, AuthAuthenticated, []string{"assets:delete"}, func(r *http.Request) error {
		user, id, err := assetTarget(r, "asset_id")
		if err != nil {
			return err
		}
		return assetError(app.DeleteAsset(user.ID, id, auditActor(r, user)))
	})
	builder.JSON(http.MethodPost, "/assets/summaries", http.StatusOK, false, AuthAuthenticated, []string{"assets:read"}, func(r *http.Request, input idBatchRequest) ([]models.AssetSummary, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		ids, err := normalizeBatchIDs(input.IDs)
		if err != nil {
			return nil, err
		}
		result, err := app.AssetSummaries(user.ID, ids)
		return result, assetError(err)
	})
	builder.Read("/items/{item_id}/linked-assets", AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) ([]models.LinkedAsset, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		itemID, err := pathID(r, "item_id")
		if err != nil {
			return nil, err
		}
		result, err := app.LinkedToItem(user.ID, itemID)
		return result, assetError(err)
	})
	builder.Read(item+"/links", AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (services.EntityLinks, error) {
		user, id, err := assetTarget(r, "asset_id")
		if err != nil {
			return services.EntityLinks{}, err
		}
		result, err := app.AssetLinks(user.ID, id)
		return result, assetError(err)
	})
	builder.Read(item+"/relationship-graph", AuthAuthenticated, []string{"assets:read"}, func(r *http.Request) (models.RelationshipGraphResponse, error) {
		user, id, err := assetTarget(r, "asset_id")
		if err != nil {
			return models.RelationshipGraphResponse{}, err
		}
		result, err := app.RelationshipGraph(user.ID, id)
		return result, assetError(err)
	})
	builder.JSON(http.MethodPost, item+"/links", http.StatusCreated, false, AuthAuthenticated, []string{"assets:write"}, func(r *http.Request, input assetLinkInput) (*models.ItemLink, error) {
		user, id, err := assetTarget(r, "asset_id")
		if err != nil {
			return nil, err
		}
		result, err := app.CreateAssetLink(user.ID, id, services.CreateItemLinkParams{LinkTypeID: input.LinkTypeID, TargetType: input.TargetType, TargetID: input.TargetID})
		return result, linkError(err)
	})
}

func assetMutation(input assetInput) services.AssetMutationInput {
	return services.AssetMutationInput{AssetTypeID: input.AssetTypeID, CategoryID: input.CategoryID, StatusID: input.StatusID, Title: input.Title, Description: input.Description, AssetTag: input.AssetTag, CustomFieldValues: input.CustomFieldValues}
}
func assetPatch(input assetPatchInput) (services.AssetPatchInput, error) {
	if !input.AssetTypeID.Set && !input.CategoryID.Set && !input.StatusID.Set && !input.Title.Set && !input.Description.Set && !input.AssetTag.Set && !input.CustomFieldValues.Set {
		return services.AssetPatchInput{}, newError(http.StatusBadRequest, "invalid_request", "At least one field is required")
	}
	if input.AssetTypeID.Null || input.Title.Null || input.Description.Null || input.AssetTag.Null || input.CustomFieldValues.Null {
		return services.AssetPatchInput{}, newError(http.StatusBadRequest, "invalid_request", "Non-nullable asset fields cannot be null")
	}
	patch := services.AssetPatchInput{CategoryIDSet: input.CategoryID.Set, StatusIDSet: input.StatusID.Set}
	if input.AssetTypeID.Set {
		patch.AssetTypeID = &input.AssetTypeID.Value
	}
	if input.CategoryID.Set && !input.CategoryID.Null {
		patch.CategoryID = &input.CategoryID.Value
	}
	if input.StatusID.Set && !input.StatusID.Null {
		patch.StatusID = &input.StatusID.Value
	}
	if input.Title.Set {
		patch.Title = &input.Title.Value
	}
	if input.Description.Set {
		patch.Description = &input.Description.Value
	}
	if input.AssetTag.Set {
		patch.AssetTag = &input.AssetTag.Value
	}
	if input.CustomFieldValues.Set {
		patch.CustomFieldValues = &input.CustomFieldValues.Value
	}
	return patch, nil
}
func assetTarget(r *http.Request, name string) (*models.User, int, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	id, err := pathID(r, name)
	return user, id, err
}
func assetError(err error) error {
	if err == nil {
		return nil
	}
	var validation *services.AssetValidationError
	switch {
	case errors.Is(err, services.ErrAssetForbidden), errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Asset resource was not found")
	case errors.Is(err, services.ErrAssetImportStorageDisabled):
		return newError(http.StatusServiceUnavailable, "service_unavailable", "Asset import storage is not configured")
	case errors.Is(err, services.ErrAssetImportUploadNotFound):
		return newError(http.StatusNotFound, "not_found", "Asset import upload was not found")
	case errors.Is(err, repository.ErrDuplicateEntry):
		return newError(http.StatusConflict, "conflict", "Asset resource already exists")
	case errors.Is(err, services.ErrAssetConflict):
		return newError(http.StatusConflict, "conflict", "Asset resource is still in use")
	case errors.As(err, &validation):
		return newError(http.StatusBadRequest, "invalid_request", validation.Error())
	default:
		return internalError(err)
	}
}
