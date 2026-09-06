package v2

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/contextkeys"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type catalogReader interface {
	ListWorkspaces(int, services.CatalogPageParams) ([]models.Workspace, int, error)
	ListWorkspaceTemplates(context.Context, int) ([]models.WorkspaceTemplateSummary, error)
	GetWorkspace(userID, workspaceID int) (*models.Workspace, error)
	VisitWorkspace(userID, workspaceID int) (*models.Workspace, error)
	ListWorkspaceStatuses(userID, workspaceID int, itemTypeID *int) ([]services.StatusResult, error)
	ListWorkspaceItemTypes(userID, workspaceID int) ([]services.ItemTypeResult, error)
	ListWorkspaceWorkflows(userID, workspaceID int) ([]services.WorkflowResult, error)
	ListWorkspacePriorities(userID, workspaceID int) ([]services.PriorityResult, error)
	ListAssignableUsers(context.Context, int, int) ([]models.User, error)
	ListUsers(int, services.CatalogPageParams) ([]models.User, int, error)
	GetUser(userID, targetID int) (*models.User, error)
	ListLabels(userID, workspaceID int) ([]models.Label, error)
	GetLabel(userID, workspaceID, labelID int) (*models.Label, error)
	ListItemTemplates(userID, workspaceID int, itemTypeID *int) (services.ItemTemplateListResult, error)
	GetItemTemplate(userID, workspaceID, templateID int) (*models.ItemTemplate, error)
}

type workspaceApplication interface {
	Create(context.Context, services.AuditActor, services.CreateWorkspaceParams) (*models.Workspace, error)
	Update(services.AuditActor, services.UpdateWorkspaceParams) (*models.Workspace, error)
	Delete(services.AuditActor, int) error
}

type itemTemplateApplication interface {
	Create(services.AuditActor, int, services.ItemTemplateInput) (*models.ItemTemplate, error)
	Update(services.AuditActor, int, int, services.ItemTemplatePatch) (*models.ItemTemplate, error)
	Delete(services.AuditActor, int, int) error
}

func registerScopedCatalogRoutes(builder *routeBuilder, catalog catalogReader, workspaces workspaceApplication, templates itemTemplateApplication) {
	builder.Page("/workspaces", AuthAuthenticated, []string{"workspaces:read"}, listWorkspaces(catalog))
	builder.JSON(http.MethodPost, "/workspaces", http.StatusCreated, false, AuthAuthenticated, []string{"workspaces:write"}, createWorkspace(workspaces))
	builder.Read("/workspace-templates", AuthAuthenticated, []string{"workspaces:read"}, listWorkspaceTemplates(catalog))
	builder.Read("/workspaces/{workspace_id}", AuthAuthenticated, []string{"workspaces:read"}, getWorkspace(catalog))
	builder.JSON(http.MethodPatch, "/workspaces/{workspace_id}", http.StatusOK, true, AuthAuthenticated, []string{"workspaces:write"}, updateWorkspace(workspaces))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}", AuthAuthenticated, []string{"workspaces:delete"}, deleteWorkspace(workspaces))
	builder.Read("/workspaces/{workspace_id}/statuses", AuthAuthenticated, []string{"workspaces:read"}, listWorkspaceStatuses(catalog))
	builder.Read("/workspaces/{workspace_id}/item-types", AuthAuthenticated, []string{"workspaces:read"}, listWorkspaceItemTypes(catalog))
	builder.Read("/workspaces/{workspace_id}/workflows", AuthAuthenticated, []string{"workspaces:read"}, listWorkspaceWorkflows(catalog))
	builder.Read("/workspaces/{workspace_id}/priorities", AuthAuthenticated, []string{"workspaces:read"}, listWorkspacePriorities(catalog))
	builder.Read("/workspaces/{workspace_id}/assignable-users", AuthAuthenticated, []string{"users:read"}, listAssignableUsers(catalog))
	builder.Page("/users", AuthAuthenticated, []string{"users:read"}, listUsers(catalog))
	builder.Read("/users/{user_id}", AuthAuthenticated, []string{"users:read"}, getUser(catalog))
	builder.Read("/workspaces/{workspace_id}/labels", AuthAuthenticated, []string{"items:read"}, listLabels(catalog))
	builder.Read("/workspaces/{workspace_id}/labels/{label_id}", AuthAuthenticated, []string{"items:read"}, getLabel(catalog))
	builder.RawResponse[itemTemplateListDocument](http.MethodGet, "/workspaces/{workspace_id}/item-templates", http.StatusOK, "application/json", AuthAuthenticated, []string{"item-templates:read"}, listItemTemplates(catalog))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/item-templates", http.StatusCreated, false, AuthAuthenticated, []string{"item-templates:write"}, createItemTemplate(templates))
	builder.Read("/workspaces/{workspace_id}/item-templates/{template_id}", AuthAuthenticated, []string{"item-templates:read"}, getItemTemplate(catalog))
	builder.JSON(http.MethodPatch, "/workspaces/{workspace_id}/item-templates/{template_id}", http.StatusOK, true, AuthAuthenticated, []string{"item-templates:write"}, updateItemTemplate(templates))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/item-templates/{template_id}", AuthAuthenticated, []string{"item-templates:write"}, deleteItemTemplate(templates))
}

type workspaceDTO struct {
	ID                      int     `json:"id"`
	Name                    string  `json:"name"`
	Key                     string  `json:"key"`
	Description             string  `json:"description"`
	Active                  bool    `json:"active"`
	TimeProjectID           *int    `json:"time_project_id"`
	TimeProjectName         string  `json:"time_project_name"`
	TimeProjectCategories   []int   `json:"time_project_categories"`
	IsPersonal              bool    `json:"is_personal"`
	OwnerID                 *int    `json:"owner_id"`
	IsTemplate              bool    `json:"is_template"`
	InternalCommentsEnabled bool    `json:"internal_comments_enabled"`
	Icon                    string  `json:"icon"`
	Color                   string  `json:"color"`
	AvatarURL               *string `json:"avatar_url"`
	DefaultView             string  `json:"default_view"`
	ConfigurationSetID      *int64  `json:"configuration_set_id"`
	CreatedAt               string  `json:"created_at"`
	UpdatedAt               string  `json:"updated_at"`
}

type workspaceTemplateDTO struct {
	ID                   int    `json:"id"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	Icon                 string `json:"icon"`
	Color                string `json:"color"`
	ConfigurationSetName string `json:"configuration_set_name"`
	TemplateCount        int    `json:"template_count"`
	ItemCount            int    `json:"item_count"`
}

type workspaceCreateRequest struct {
	Name                string  `json:"name"`
	Key                 string  `json:"key"`
	Description         string  `json:"description"`
	Active              *bool   `json:"active"`
	TimeProjectID       *int    `json:"time_project_id"`
	IsPersonal          bool    `json:"is_personal"`
	OwnerID             *int    `json:"owner_id"`
	Icon                string  `json:"icon"`
	Color               string  `json:"color"`
	AvatarURL           *string `json:"avatar_url"`
	DefaultView         string  `json:"default_view"`
	TemplateWorkspaceID *int    `json:"template_workspace_id"`
}

type workspacePatchRequest struct {
	Name                    Optional[string] `json:"name"`
	Key                     Optional[string] `json:"key"`
	Description             Optional[string] `json:"description"`
	Active                  Optional[bool]   `json:"active"`
	TimeProjectID           Optional[int]    `json:"time_project_id"`
	IsPersonal              Optional[bool]   `json:"is_personal"`
	OwnerID                 Optional[int]    `json:"owner_id"`
	Icon                    Optional[string] `json:"icon"`
	Color                   Optional[string] `json:"color"`
	AvatarURL               Optional[string] `json:"avatar_url"`
	DefaultView             Optional[string] `json:"default_view"`
	InternalCommentsEnabled Optional[bool]   `json:"internal_comments_enabled"`
	TimeProjectCategories   Optional[[]int]  `json:"time_project_categories"`
	IsTemplate              Optional[bool]   `json:"is_template"`
}

func listWorkspaces(catalog catalogReader) pageOperation[workspaceDTO] {
	return func(r *http.Request) ([]workspaceDTO, Pagination, int, error) {
		page, err := ParsePagination(r, map[string]bool{"name": true, "key": true, "created_at": true}, "name")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		items, total, err := catalog.ListWorkspaces(user.ID, catalogPage(page))
		if err != nil {
			return nil, Pagination{}, 0, scopedReadError(err, "Workspace was not found")
		}
		result := make([]workspaceDTO, len(items))
		for i := range items {
			result[i] = workspaceFromModel(&items[i])
		}
		return result, page, total, nil
	}
}

func listWorkspaceTemplates(catalog catalogReader) readOperation[[]workspaceTemplateDTO] {
	return func(r *http.Request) ([]workspaceTemplateDTO, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		items, err := catalog.ListWorkspaceTemplates(r.Context(), user.ID)
		if err != nil {
			return nil, internalError(err)
		}
		result := make([]workspaceTemplateDTO, len(items))
		for i, item := range items {
			result[i] = workspaceTemplateDTO(item)
		}
		return result, nil
	}
}

func getWorkspace(catalog catalogReader) readOperation[workspaceDTO] {
	return func(r *http.Request) (workspaceDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return workspaceDTO{}, err
		}
		workspace, err := catalog.VisitWorkspace(user.ID, workspaceID)
		if err != nil {
			return workspaceDTO{}, scopedReadError(err, "Workspace was not found")
		}
		return workspaceFromModel(workspace), nil
	}
}

func createWorkspace(workspaces workspaceApplication) jsonOperation[workspaceCreateRequest, workspaceDTO] {
	return func(r *http.Request, input workspaceCreateRequest) (workspaceDTO, error) {
		user, err := principal(r)
		if err != nil {
			return workspaceDTO{}, err
		}
		workspace, err := workspaces.Create(r.Context(), auditActor(r, user), services.CreateWorkspaceParams{
			Name: input.Name, Key: input.Key, Description: input.Description, Active: input.Active,
			TimeProjectID: input.TimeProjectID, IsPersonal: input.IsPersonal, OwnerID: input.OwnerID,
			Icon: input.Icon, Color: input.Color, AvatarURL: input.AvatarURL, DefaultView: input.DefaultView,
			TemplateWorkspaceID: input.TemplateWorkspaceID,
		})
		if err != nil {
			return workspaceDTO{}, workspaceMutationError(err)
		}
		return workspaceFromModel(workspace), nil
	}
}

func updateWorkspace(workspaces workspaceApplication) jsonOperation[workspacePatchRequest, workspaceDTO] {
	return func(r *http.Request, input workspacePatchRequest) (workspaceDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return workspaceDTO{}, err
		}
		if workspacePatchHasInvalidNull(input) {
			return workspaceDTO{}, newError(http.StatusBadRequest, "invalid_request", "Only nullable workspace fields may be null")
		}
		params := services.UpdateWorkspaceParams{
			ID: workspaceID, Name: optionalValue(input.Name), Key: optionalValue(input.Key),
			Description: optionalValue(input.Description), Active: optionalValue(input.Active),
			TimeProjectID: services.NullableUpdate[int]{Present: input.TimeProjectID.Set, Value: optionalNullableValue(input.TimeProjectID)},
			IsPersonal:    optionalValue(input.IsPersonal),
			OwnerID:       services.NullableUpdate[int]{Present: input.OwnerID.Set, Value: optionalNullableValue(input.OwnerID)},
			Icon:          optionalValue(input.Icon), Color: optionalValue(input.Color),
			AvatarURL:   services.NullableUpdate[string]{Present: input.AvatarURL.Set, Value: optionalNullableValue(input.AvatarURL)},
			DefaultView: optionalValue(input.DefaultView), InternalCommentsEnabled: optionalValue(input.InternalCommentsEnabled),
			TimeProjectCategories: optionalValue(input.TimeProjectCategories), IsTemplate: optionalValue(input.IsTemplate),
		}
		workspace, err := workspaces.Update(auditActor(r, user), params)
		if err != nil {
			return workspaceDTO{}, workspaceMutationError(err)
		}
		return workspaceFromModel(workspace), nil
	}
}

func deleteWorkspace(workspaces workspaceApplication) commandOperation {
	return func(r *http.Request) error {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return err
		}
		return workspaceMutationError(workspaces.Delete(auditActor(r, user), workspaceID))
	}
}

func workspacePatchHasInvalidNull(input workspacePatchRequest) bool {
	return input.Name.Null || input.Key.Null || input.Description.Null || input.Active.Null ||
		input.IsPersonal.Null || input.Icon.Null || input.Color.Null || input.DefaultView.Null ||
		input.InternalCommentsEnabled.Null || input.TimeProjectCategories.Null || input.IsTemplate.Null
}

func workspaceMutationError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, services.ErrTemplateWorkspaceNotFound):
		return newError(http.StatusNotFound, "not_found", "Workspace was not found")
	case errors.Is(err, services.ErrWorkspaceMutationForbidden):
		return newError(http.StatusForbidden, "forbidden", "Workspace operation is not permitted")
	case errors.Is(err, services.ErrWorkspaceMutationInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, repository.ErrDuplicateEntry):
		return newError(http.StatusConflict, "conflict", "Workspace key already exists")
	case errors.Is(err, services.ErrWorkspaceHasProtectedIntegrationLinks):
		return newError(http.StatusConflict, "conflict", "Remove all protected integration links from this workspace before deleting it.")
	case errors.Is(err, services.ErrInvalidWorkspaceTemplate), errors.Is(err, services.ErrWorkspaceTemplateTooLarge), errors.Is(err, services.ErrPersonalWorkspaceTemplate):
		return newError(http.StatusUnprocessableEntity, "unprocessable_entity", err.Error())
	default:
		return internalError(err)
	}
}

func listWorkspaceStatuses(catalog catalogReader) readOperation[[]statusDTO] {
	return func(r *http.Request) ([]statusDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		itemTypeID, err := optionalPositiveQuery(r, "item_type_id")
		if err != nil {
			return nil, err
		}
		items, err := catalog.ListWorkspaceStatuses(user.ID, workspaceID, itemTypeID)
		if err != nil {
			return nil, scopedReadError(err, "Workspace or item type was not found")
		}
		result := make([]statusDTO, len(items))
		for i := range items {
			result[i] = statusFromResult(items[i])
		}
		return result, nil
	}
}

func listWorkspaceItemTypes(catalog catalogReader) readOperation[[]itemTypeDTO] {
	return func(r *http.Request) ([]itemTypeDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		items, err := catalog.ListWorkspaceItemTypes(user.ID, workspaceID)
		if err != nil {
			return nil, scopedReadError(err, "Workspace was not found")
		}
		result := make([]itemTypeDTO, len(items))
		for i := range items {
			result[i] = itemTypeFromResult(items[i])
		}
		return result, nil
	}
}

func listWorkspaceWorkflows(catalog catalogReader) readOperation[[]workflowDTO] {
	return func(r *http.Request) ([]workflowDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		items, err := catalog.ListWorkspaceWorkflows(user.ID, workspaceID)
		if err != nil {
			return nil, scopedReadError(err, "Workspace was not found")
		}
		result := make([]workflowDTO, len(items))
		for i := range items {
			result[i] = workflowFromResult(items[i])
		}
		return result, nil
	}
}

func listWorkspacePriorities(catalog catalogReader) readOperation[[]priorityDTO] {
	return func(r *http.Request) ([]priorityDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		items, err := catalog.ListWorkspacePriorities(user.ID, workspaceID)
		if err != nil {
			return nil, scopedReadError(err, "Workspace was not found")
		}
		result := make([]priorityDTO, len(items))
		for i := range items {
			result[i] = priorityFromResult(items[i])
		}
		return result, nil
	}
}

type userSummaryDTO struct {
	ID            int     `json:"id"`
	Username      string  `json:"username"`
	FirstName     string  `json:"first_name"`
	LastName      string  `json:"last_name"`
	FullName      string  `json:"full_name"`
	IsActive      bool    `json:"is_active"`
	IsAgent       bool    `json:"is_agent"`
	AgentPresence string  `json:"agent_presence"`
	AvatarURL     *string `json:"avatar_url"`
	CreatedAt     string  `json:"created_at"`
}

func listAssignableUsers(catalog catalogReader) readOperation[[]userSummaryDTO] {
	return func(r *http.Request) ([]userSummaryDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		items, err := catalog.ListAssignableUsers(r.Context(), user.ID, workspaceID)
		if err != nil {
			return nil, scopedReadError(err, "Workspace was not found")
		}
		return userSummaries(items), nil
	}
}

func listUsers(catalog catalogReader) pageOperation[userSummaryDTO] {
	return func(r *http.Request) ([]userSummaryDTO, Pagination, int, error) {
		page, err := ParsePagination(r, map[string]bool{"username": true, "full_name": true, "created_at": true}, "username")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		items, total, err := catalog.ListUsers(user.ID, catalogPage(page))
		if err != nil {
			return nil, Pagination{}, 0, scopedReadError(err, "User was not found")
		}
		return userSummaries(items), page, total, nil
	}
}

func getUser(catalog catalogReader) readOperation[userSummaryDTO] {
	return func(r *http.Request) (userSummaryDTO, error) {
		user, err := principal(r)
		if err != nil {
			return userSummaryDTO{}, err
		}
		userID, err := pathID(r, "user_id")
		if err != nil {
			return userSummaryDTO{}, err
		}
		item, err := catalog.GetUser(user.ID, userID)
		if err != nil {
			return userSummaryDTO{}, scopedReadError(err, "User was not found")
		}
		return userSummaryFromModel(item), nil
	}
}

type labelDTO struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func listLabels(catalog catalogReader) readOperation[[]labelDTO] {
	return func(r *http.Request) ([]labelDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		items, err := catalog.ListLabels(user.ID, workspaceID)
		if err != nil {
			return nil, scopedReadError(err, "Workspace was not found")
		}
		result := make([]labelDTO, len(items))
		for i := range items {
			result[i] = labelFromModel(&items[i])
		}
		return result, nil
	}
}

func getLabel(catalog catalogReader) readOperation[labelDTO] {
	return func(r *http.Request) (labelDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return labelDTO{}, err
		}
		labelID, err := pathID(r, "label_id")
		if err != nil {
			return labelDTO{}, err
		}
		item, err := catalog.GetLabel(user.ID, workspaceID, labelID)
		if err != nil {
			return labelDTO{}, scopedReadError(err, "Label was not found")
		}
		return labelFromModel(item), nil
	}
}

type itemTemplateDTO struct {
	ID              int    `json:"id"`
	WorkspaceID     int    `json:"workspace_id"`
	Name            string `json:"name"`
	DescriptionBody string `json:"description_body"`
	Mode            string `json:"mode"`
	IsActive        bool   `json:"is_active"`
	ItemTypeIDs     []int  `json:"item_type_ids"`
	CreatedBy       *int   `json:"created_by"`
	UpdatedBy       *int   `json:"updated_by"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type itemTemplateListDocument struct {
	Data                []itemTemplateDTO `json:"data"`
	MandatoryTemplateID *int              `json:"mandatory_template_id"`
}

type itemTemplateCreateRequest struct {
	Name            string `json:"name"`
	DescriptionBody string `json:"description_body"`
	Mode            string `json:"mode"`
	IsActive        *bool  `json:"is_active"`
	ItemTypeIDs     []int  `json:"item_type_ids"`
}

type itemTemplatePatchRequest struct {
	Name            Optional[string] `json:"name"`
	DescriptionBody Optional[string] `json:"description_body"`
	Mode            Optional[string] `json:"mode"`
	IsActive        Optional[bool]   `json:"is_active"`
	ItemTypeIDs     Optional[[]int]  `json:"item_type_ids"`
}

func listItemTemplates(catalog catalogReader) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return err
		}
		itemTypeID, err := optionalPositiveQuery(r, "item_type_id")
		if err != nil {
			return err
		}
		items, err := catalog.ListItemTemplates(user.ID, workspaceID, itemTypeID)
		if err != nil {
			return scopedReadError(err, "Workspace or item type was not found")
		}
		result := make([]itemTemplateDTO, len(items.Items))
		for i := range items.Items {
			result[i] = itemTemplateFromModel(&items.Items[i])
		}
		return writeJSON(w, http.StatusOK, itemTemplateListDocument{
			Data: result, MandatoryTemplateID: items.MandatoryTemplateID,
		})
	}
}

func getItemTemplate(catalog catalogReader) readOperation[itemTemplateDTO] {
	return func(r *http.Request) (itemTemplateDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return itemTemplateDTO{}, err
		}
		templateID, err := pathID(r, "template_id")
		if err != nil {
			return itemTemplateDTO{}, err
		}
		item, err := catalog.GetItemTemplate(user.ID, workspaceID, templateID)
		if err != nil {
			return itemTemplateDTO{}, scopedReadError(err, "Item template was not found")
		}
		return itemTemplateFromModel(item), nil
	}
}

func createItemTemplate(templates itemTemplateApplication) jsonOperation[itemTemplateCreateRequest, itemTemplateDTO] {
	return func(r *http.Request, input itemTemplateCreateRequest) (itemTemplateDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return itemTemplateDTO{}, err
		}
		result, err := templates.Create(auditActor(r, user), workspaceID, services.ItemTemplateInput{
			Name: input.Name, DescriptionBody: input.DescriptionBody, Mode: input.Mode,
			IsActive: input.IsActive, ItemTypeIDs: input.ItemTypeIDs,
		})
		if err != nil {
			return itemTemplateDTO{}, itemTemplateMutationError(err)
		}
		return itemTemplateFromModel(result), nil
	}
}

func updateItemTemplate(templates itemTemplateApplication) jsonOperation[itemTemplatePatchRequest, itemTemplateDTO] {
	return func(r *http.Request, input itemTemplatePatchRequest) (itemTemplateDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return itemTemplateDTO{}, err
		}
		templateID, err := pathID(r, "template_id")
		if err != nil {
			return itemTemplateDTO{}, err
		}
		if input.Name.Null || input.DescriptionBody.Null || input.Mode.Null || input.IsActive.Null || input.ItemTypeIDs.Null {
			return itemTemplateDTO{}, newError(http.StatusBadRequest, "invalid_request", "Item template fields may not be null")
		}
		result, err := templates.Update(auditActor(r, user), workspaceID, templateID, services.ItemTemplatePatch{
			Name: optionalValue(input.Name), DescriptionBody: optionalValue(input.DescriptionBody),
			Mode: optionalValue(input.Mode), IsActive: optionalValue(input.IsActive), ItemTypeIDs: optionalValue(input.ItemTypeIDs),
		})
		if err != nil {
			return itemTemplateDTO{}, itemTemplateMutationError(err)
		}
		return itemTemplateFromModel(result), nil
	}
}

func deleteItemTemplate(templates itemTemplateApplication) commandOperation {
	return func(r *http.Request) error {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return err
		}
		templateID, err := pathID(r, "template_id")
		if err != nil {
			return err
		}
		return itemTemplateMutationError(templates.Delete(auditActor(r, user), workspaceID, templateID))
	}
}

func itemTemplateMutationError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Item template was not found")
	case errors.Is(err, services.ErrItemTemplateNameRequired), errors.Is(err, services.ErrItemTemplateTypeNotFound),
		errors.Is(err, repository.ErrInvalidTemplateMode), errors.Is(err, repository.ErrMandatoryRequiresOneType):
		return newError(http.StatusBadRequest, "validation_failed", err.Error())
	case errors.Is(err, repository.ErrDuplicateEntry), errors.Is(err, repository.ErrMandatoryConflict):
		return newError(http.StatusConflict, "conflict", err.Error())
	default:
		return internalError(err)
	}
}

func workspaceFromModel(workspace *models.Workspace) workspaceDTO {
	categories := workspace.TimeProjectCategories
	if categories == nil {
		categories = []int{}
	}
	return workspaceDTO{
		ID: workspace.ID, Name: workspace.Name, Key: workspace.Key, Description: workspace.Description,
		Active: workspace.Active, TimeProjectID: workspace.TimeProjectID, IsPersonal: workspace.IsPersonal,
		TimeProjectName: workspace.TimeProjectName, TimeProjectCategories: categories,
		OwnerID: workspace.OwnerID, IsTemplate: workspace.IsTemplate,
		InternalCommentsEnabled: workspace.InternalCommentsEnabled, Icon: workspace.Icon, Color: workspace.Color,
		AvatarURL: workspace.AvatarURL, DefaultView: workspace.DefaultView, ConfigurationSetID: workspace.ConfigurationSetID,
		CreatedAt: timestamp(workspace.CreatedAt), UpdatedAt: timestamp(workspace.UpdatedAt),
	}
}

func userSummaries(users []models.User) []userSummaryDTO {
	result := make([]userSummaryDTO, len(users))
	for i := range users {
		result[i] = userSummaryFromModel(&users[i])
	}
	return result
}

func userSummaryFromModel(user *models.User) userSummaryDTO {
	var avatarURL *string
	if user.AvatarURL != "" {
		avatarURL = &user.AvatarURL
	}
	return userSummaryDTO{
		ID: user.ID, Username: user.Username, FirstName: user.FirstName, LastName: user.LastName,
		FullName: user.FullName, IsActive: user.IsActive, IsAgent: user.IsAgent,
		AgentPresence: user.AgentPresence, AvatarURL: avatarURL, CreatedAt: timestamp(user.CreatedAt),
	}
}

func labelFromModel(label *models.Label) labelDTO {
	return labelDTO{
		ID: label.ID, Name: label.Name, Color: label.Color,
		CreatedAt: timestamp(label.CreatedAt), UpdatedAt: timestamp(label.UpdatedAt),
	}
}

func itemTemplateFromModel(item *models.ItemTemplate) itemTemplateDTO {
	itemTypeIDs := item.ItemTypeIDs
	if itemTypeIDs == nil {
		itemTypeIDs = []int{}
	}
	return itemTemplateDTO{
		ID: item.ID, WorkspaceID: item.WorkspaceID, Name: item.Name,
		DescriptionBody: item.DescriptionBody, Mode: item.Mode, IsActive: item.IsActive,
		ItemTypeIDs: itemTypeIDs, CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy,
		CreatedAt: timestamp(item.CreatedAt), UpdatedAt: timestamp(item.UpdatedAt),
	}
}

func principal(r *http.Request) (*models.User, error) {
	user, ok := r.Context().Value(contextkeys.User).(*models.User)
	if !ok || user == nil {
		return nil, newError(http.StatusUnauthorized, "authentication_required", "Authentication is required")
	}
	return user, nil
}

func principalAndWorkspace(r *http.Request) (*models.User, int, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	workspaceID, err := pathID(r, "workspace_id")
	if err != nil {
		return nil, 0, err
	}
	return user, workspaceID, nil
}

func optionalPositiveQuery(r *http.Request, name string) (*int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		apiErr := newError(http.StatusBadRequest, "invalid_request", name+" must be a positive integer")
		apiErr.Details = map[string]any{"field": name}
		return nil, apiErr
	}
	return &value, nil
}

func scopedReadError(err error, notFoundMessage string) error {
	switch {
	case errors.Is(err, services.ErrCatalogNotFound):
		return newError(http.StatusNotFound, "not_found", notFoundMessage)
	case errors.Is(err, services.ErrCatalogForbidden):
		return newError(http.StatusForbidden, "insufficient_permission", "The principal lacks the required permission")
	default:
		return internalError(err)
	}
}

func catalogPage(page Pagination) services.CatalogPageParams {
	return services.CatalogPageParams{
		Limit: page.PageSize, Offset: page.Offset, Sort: page.Sort, Desc: page.Desc,
	}
}

func timestamp(value time.Time) string { return value.UTC().Format(time.RFC3339) }
