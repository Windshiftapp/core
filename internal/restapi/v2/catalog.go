package v2

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/models"
	"windshift/internal/objecttranslation"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type statusReader interface {
	ListStatuses() ([]services.StatusResult, error)
	GetStatus(int) (*services.StatusResult, error)
	ListCategories() ([]services.StatusCategoryResult, error)
	GetCategory(int) (*services.StatusCategoryResult, error)
}

type workflowReader interface {
	List() ([]services.WorkflowResult, error)
	GetByID(int) (*services.WorkflowResult, error)
	GetTransitions(int) ([]services.WorkflowTransitionResult, error)
}

type configurationReader interface {
	ListItemTypes() ([]services.ItemTypeResult, error)
	GetItemType(int) (*services.ItemTypeResult, error)
	ListPriorities() ([]services.PriorityResult, error)
	GetPriority(int) (*services.PriorityResult, error)
	ListCustomFields() ([]services.CustomFieldResult, error)
	ListCustomFieldsWithMeta() ([]services.CustomFieldResult, services.CustomFieldCatalogMeta, error)
	GetCustomField(int) (*services.CustomFieldResult, error)
}

func registerCatalogRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/query-language/catalog", AuthAuthenticated, []string{"items:read"}, queryLanguageCompletionCatalog(deps.Configuration))
	builder.Read("/statuses", AuthAuthenticated, []string{"statuses:read"}, listStatuses(deps.Statuses, deps.ObjectTranslations))
	builder.JSON(http.MethodPost, "/statuses", http.StatusCreated, false, AuthAuthenticated, []string{"statuses:write"}, createStatus(deps.CatalogMutations))
	builder.Read("/statuses/{status_id}", AuthAuthenticated, []string{"statuses:read"}, getStatus(deps.Statuses, deps.ObjectTranslations))
	builder.JSON(http.MethodPatch, "/statuses/{status_id}", http.StatusOK, true, AuthAuthenticated, []string{"statuses:write"}, patchStatus(deps.CatalogMutations))
	builder.Command(http.MethodDelete, "/statuses/{status_id}", AuthAuthenticated, []string{"statuses:write"}, deleteStatus(deps.CatalogMutations))
	builder.Read("/status-categories", AuthAuthenticated, []string{"statuses:read"}, listStatusCategories(deps.Statuses, deps.ObjectTranslations))
	builder.JSON(http.MethodPost, "/status-categories", http.StatusCreated, false, AuthAuthenticated, []string{"statuses:write"}, createStatusCategory(deps.CatalogMutations))
	builder.Read("/status-categories/{category_id}", AuthAuthenticated, []string{"statuses:read"}, getStatusCategory(deps.Statuses, deps.ObjectTranslations))
	builder.JSON(http.MethodPatch, "/status-categories/{category_id}", http.StatusOK, true, AuthAuthenticated, []string{"statuses:write"}, patchStatusCategory(deps.CatalogMutations))
	builder.Command(http.MethodDelete, "/status-categories/{category_id}", AuthAuthenticated, []string{"statuses:write"}, deleteStatusCategory(deps.CatalogMutations))
	builder.Read("/workflows", AuthAuthenticated, []string{"workflows:read"}, listWorkflows(deps.Workflows, deps.ObjectTranslations))
	builder.JSON(http.MethodPost, "/workflows", http.StatusCreated, false, AuthAuthenticated, []string{"workflows:write"}, createWorkflow(deps.CatalogMutations))
	builder.Read("/workflows/{workflow_id}", AuthAuthenticated, []string{"workflows:read"}, getWorkflow(deps.Workflows, deps.ObjectTranslations))
	builder.JSON(http.MethodPatch, "/workflows/{workflow_id}", http.StatusOK, true, AuthAuthenticated, []string{"workflows:write"}, patchWorkflow(deps.CatalogMutations))
	builder.Command(http.MethodDelete, "/workflows/{workflow_id}", AuthAuthenticated, []string{"workflows:write"}, deleteWorkflow(deps.CatalogMutations))
	builder.Read("/workflows/{workflow_id}/transitions", AuthAuthenticated, []string{"workflows:read"}, listWorkflowTransitions(deps.Workflows))
	builder.JSON(http.MethodPut, "/workflows/{workflow_id}/transitions", http.StatusOK, false, AuthAuthenticated, []string{"workflows:write"}, replaceWorkflowTransitions(deps.CatalogMutations))
	builder.Read("/item-types", AuthAuthenticated, []string{"item-types:read"}, listItemTypes(deps.Configuration, deps.ObjectTranslations))
	builder.JSON(http.MethodPost, "/item-types", http.StatusCreated, false, AuthAuthenticated, []string{"item-types:write"}, createItemType(deps.CatalogMutations))
	builder.Read("/item-types/{item_type_id}", AuthAuthenticated, []string{"item-types:read"}, getItemType(deps.Configuration, deps.ObjectTranslations))
	builder.JSON(http.MethodPatch, "/item-types/{item_type_id}", http.StatusOK, true, AuthAuthenticated, []string{"item-types:write"}, patchItemType(deps.CatalogMutations))
	builder.Command(http.MethodDelete, "/item-types/{item_type_id}", AuthAuthenticated, []string{"item-types:write"}, deleteItemType(deps.CatalogMutations))
	builder.Read("/priorities", AuthAuthenticated, []string{"priorities:read"}, listPriorities(deps.Configuration, deps.ObjectTranslations))
	builder.JSON(http.MethodPost, "/priorities", http.StatusCreated, false, AuthAuthenticated, []string{"priorities:write"}, createPriority(deps.CatalogMutations))
	builder.Read("/priorities/{priority_id}", AuthAuthenticated, []string{"priorities:read"}, getPriority(deps.Configuration, deps.ObjectTranslations))
	builder.JSON(http.MethodPatch, "/priorities/{priority_id}", http.StatusOK, true, AuthAuthenticated, []string{"priorities:write"}, patchPriority(deps.CatalogMutations))
	builder.Command(http.MethodDelete, "/priorities/{priority_id}", AuthAuthenticated, []string{"priorities:write"}, deletePriority(deps.CatalogMutations))
	builder.Metadata("/custom-fields", AuthAuthenticated, []string{"custom-fields:read"}, listCustomFields(deps.Configuration))
	builder.Read("/custom-fields/{custom_field_id}", AuthAuthenticated, []string{"custom-fields:read"}, getCustomField(deps.Configuration))
}

type statusCategoryDTO struct {
	ID                 int     `json:"id"`
	BuiltinKey         *string `json:"builtin_key"`
	Name               string  `json:"name"`
	DisplayName        string  `json:"display_name"`
	Description        string  `json:"description"`
	DisplayDescription string  `json:"display_description"`
	Color              string  `json:"color"`
	IsDefault          bool    `json:"is_default"`
	IsCompleted        bool    `json:"is_completed"`
}

type statusDTO struct {
	ID                 int               `json:"id"`
	BuiltinKey         *string           `json:"builtin_key"`
	Name               string            `json:"name"`
	DisplayName        string            `json:"display_name"`
	Description        string            `json:"description"`
	DisplayDescription string            `json:"display_description"`
	IsDefault          bool              `json:"is_default"`
	Category           statusCategoryDTO `json:"category"`
}

func listStatuses(reader statusReader, localizer objectLocalizer) readOperation[[]statusDTO] {
	return func(r *http.Request) ([]statusDTO, error) {
		results, err := reader.ListStatuses()
		if err != nil {
			return nil, internalError(err)
		}
		items := make([]statusDTO, len(results))
		for i := range results {
			items[i] = statusFromResult(results[i])
		}
		if err := localizeCatalog(r, localizer, "status", &items); err != nil {
			return nil, err
		}
		return items, nil
	}
}

func getStatus(reader statusReader, localizer objectLocalizer) readOperation[statusDTO] {
	return func(r *http.Request) (statusDTO, error) {
		id, err := pathID(r, "status_id")
		if err != nil {
			return statusDTO{}, err
		}
		result, err := reader.GetStatus(id)
		if err != nil {
			return statusDTO{}, readError(err, "Status was not found")
		}
		item := statusFromResult(*result)
		if err := localizeCatalog(r, localizer, "status", &item); err != nil {
			return statusDTO{}, err
		}
		return item, nil
	}
}

func statusFromResult(result services.StatusResult) statusDTO {
	return statusDTO{
		ID: result.ID, BuiltinKey: nullableString(result.BuiltinKey), Name: result.Name,
		DisplayName: result.Name, Description: result.Description, DisplayDescription: result.Description, IsDefault: result.IsDefault,
		Category: statusCategoryDTO{
			ID: result.CategoryID, BuiltinKey: nullableString(result.CategoryBuiltinKey),
			Name: result.CategoryName, DisplayName: result.CategoryName, Description: result.CategoryDescription, DisplayDescription: result.CategoryDescription,
			Color: result.CategoryColor, IsDefault: result.CategoryIsDefault,
			IsCompleted: result.IsCompleted,
		},
	}
}

func listStatusCategories(reader statusReader, localizer objectLocalizer) readOperation[[]statusCategoryDTO] {
	return func(r *http.Request) ([]statusCategoryDTO, error) {
		results, err := reader.ListCategories()
		if err != nil {
			return nil, internalError(err)
		}
		items := make([]statusCategoryDTO, len(results))
		for i := range results {
			items[i] = statusCategoryFromResult(results[i])
		}
		if err := localizeCatalog(r, localizer, "status_category", &items); err != nil {
			return nil, err
		}
		return items, nil
	}
}

func getStatusCategory(reader statusReader, localizer objectLocalizer) readOperation[statusCategoryDTO] {
	return func(r *http.Request) (statusCategoryDTO, error) {
		id, err := pathID(r, "category_id")
		if err != nil {
			return statusCategoryDTO{}, err
		}
		result, err := reader.GetCategory(id)
		if err != nil {
			return statusCategoryDTO{}, readError(err, "Status category was not found")
		}
		item := statusCategoryFromResult(*result)
		if err := localizeCatalog(r, localizer, "status_category", &item); err != nil {
			return statusCategoryDTO{}, err
		}
		return item, nil
	}
}

func statusCategoryFromResult(result services.StatusCategoryResult) statusCategoryDTO {
	return statusCategoryDTO{
		ID: result.ID, BuiltinKey: nullableString(result.BuiltinKey), Name: result.Name,
		DisplayName: result.Name, Description: result.Description, DisplayDescription: result.Description, Color: result.Color, IsDefault: result.IsDefault,
		IsCompleted: result.IsCompleted,
	}
}

type workflowDTO struct {
	ID                 int     `json:"id"`
	BuiltinKey         *string `json:"builtin_key"`
	Name               string  `json:"name"`
	DisplayName        string  `json:"display_name"`
	Description        string  `json:"description"`
	DisplayDescription string  `json:"display_description"`
	IsDefault          bool    `json:"is_default"`
}

func listWorkflows(reader workflowReader, localizer objectLocalizer) readOperation[[]workflowDTO] {
	return func(r *http.Request) ([]workflowDTO, error) {
		results, err := reader.List()
		if err != nil {
			return nil, internalError(err)
		}
		items := make([]workflowDTO, len(results))
		for i := range results {
			items[i] = workflowFromResult(results[i])
		}
		if err := localizeCatalog(r, localizer, "workflow", &items); err != nil {
			return nil, err
		}
		return items, nil
	}
}

func getWorkflow(reader workflowReader, localizer objectLocalizer) readOperation[workflowDTO] {
	return func(r *http.Request) (workflowDTO, error) {
		id, err := pathID(r, "workflow_id")
		if err != nil {
			return workflowDTO{}, err
		}
		result, err := reader.GetByID(id)
		if err != nil {
			return workflowDTO{}, readError(err, "Workflow was not found")
		}
		item := workflowFromResult(*result)
		if err := localizeCatalog(r, localizer, "workflow", &item); err != nil {
			return workflowDTO{}, err
		}
		return item, nil
	}
}

func workflowFromResult(result services.WorkflowResult) workflowDTO {
	return workflowDTO{
		ID: result.ID, BuiltinKey: nullableString(result.BuiltinKey), Name: result.Name,
		DisplayName: result.Name, Description: result.Description, DisplayDescription: result.Description, IsDefault: result.IsDefault,
	}
}

type transitionStatusDTO struct {
	ID                 int     `json:"id"`
	BuiltinKey         *string `json:"builtin_key"`
	Name               string  `json:"name"`
	CategoryBuiltinKey *string `json:"category_builtin_key"`
	CategoryName       string  `json:"category_name"`
	CategoryColor      string  `json:"category_color"`
}

type workflowTransitionDTO struct {
	ID              int                  `json:"id"`
	FromAllStatuses bool                 `json:"from_all_statuses"`
	From            *transitionStatusDTO `json:"from"`
	To              transitionStatusDTO  `json:"to"`
}

func listWorkflowTransitions(reader workflowReader) readOperation[[]workflowTransitionDTO] {
	return func(r *http.Request) ([]workflowTransitionDTO, error) {
		id, err := pathID(r, "workflow_id")
		if err != nil {
			return nil, err
		}
		if _, err := reader.GetByID(id); err != nil {
			return nil, readError(err, "Workflow was not found")
		}
		results, err := reader.GetTransitions(id)
		if err != nil {
			return nil, internalError(err)
		}
		items := make([]workflowTransitionDTO, len(results))
		for i, result := range results {
			item := workflowTransitionDTO{
				ID: result.ID, FromAllStatuses: result.FromAllStatuses,
				To: transitionStatusDTO{
					ID: result.ToStatusID, BuiltinKey: nullableString(result.ToStatusBuiltinKey),
					Name: result.ToStatusName, CategoryBuiltinKey: nullableString(result.ToCategoryBuiltinKey),
					CategoryName: result.ToCategoryName, CategoryColor: result.ToCategoryColor,
				},
			}
			if result.FromStatusID != nil {
				item.From = &transitionStatusDTO{
					ID: *result.FromStatusID, BuiltinKey: nullableString(result.FromStatusBuiltinKey),
					Name: result.FromStatusName, CategoryBuiltinKey: nullableString(result.FromCategoryBuiltinKey),
					CategoryName: result.FromCategoryName, CategoryColor: result.FromCategoryColor,
				}
			}
			items[i] = item
		}
		return items, nil
	}
}

type itemTypeDTO struct {
	ID                 int     `json:"id"`
	BuiltinKey         *string `json:"builtin_key"`
	Name               string  `json:"name"`
	DisplayName        string  `json:"display_name"`
	Description        string  `json:"description"`
	DisplayDescription string  `json:"display_description"`
	Icon               string  `json:"icon"`
	Color              string  `json:"color"`
	HierarchyLevel     int     `json:"hierarchy_level"`
	SortOrder          int     `json:"sort_order"`
	IsDefault          bool    `json:"is_default"`
}

func listItemTypes(reader configurationReader, localizer objectLocalizer) readOperation[[]itemTypeDTO] {
	return func(r *http.Request) ([]itemTypeDTO, error) {
		results, err := reader.ListItemTypes()
		if err != nil {
			return nil, internalError(err)
		}
		items := make([]itemTypeDTO, len(results))
		for i := range results {
			items[i] = itemTypeFromResult(results[i])
		}
		if err := localizeCatalog(r, localizer, "item_type", &items); err != nil {
			return nil, err
		}
		return items, nil
	}
}

func getItemType(reader configurationReader, localizer objectLocalizer) readOperation[itemTypeDTO] {
	return func(r *http.Request) (itemTypeDTO, error) {
		id, err := pathID(r, "item_type_id")
		if err != nil {
			return itemTypeDTO{}, err
		}
		result, err := reader.GetItemType(id)
		if err != nil {
			return itemTypeDTO{}, readError(err, "Item type was not found")
		}
		item := itemTypeFromResult(*result)
		if err := localizeCatalog(r, localizer, "item_type", &item); err != nil {
			return itemTypeDTO{}, err
		}
		return item, nil
	}
}

func itemTypeFromResult(result services.ItemTypeResult) itemTypeDTO {
	return itemTypeDTO{
		ID: result.ID, BuiltinKey: nullableString(result.BuiltinKey), Name: result.Name,
		DisplayName: result.Name, Description: result.Description, DisplayDescription: result.Description, Icon: result.Icon, Color: result.Color,
		HierarchyLevel: result.HierarchyLevel, SortOrder: result.SortOrder, IsDefault: result.IsDefault,
	}
}

type priorityDTO struct {
	ID                 int     `json:"id"`
	BuiltinKey         *string `json:"builtin_key"`
	Name               string  `json:"name"`
	DisplayName        string  `json:"display_name"`
	Description        string  `json:"description"`
	DisplayDescription string  `json:"display_description"`
	Icon               string  `json:"icon"`
	Color              string  `json:"color"`
	SortOrder          int     `json:"sort_order"`
	IsDefault          bool    `json:"is_default"`
}

func listPriorities(reader configurationReader, localizer objectLocalizer) readOperation[[]priorityDTO] {
	return func(r *http.Request) ([]priorityDTO, error) {
		results, err := reader.ListPriorities()
		if err != nil {
			return nil, internalError(err)
		}
		items := make([]priorityDTO, len(results))
		for i := range results {
			items[i] = priorityFromResult(results[i])
		}
		if err := localizeCatalog(r, localizer, "priority", &items); err != nil {
			return nil, err
		}
		return items, nil
	}
}

func getPriority(reader configurationReader, localizer objectLocalizer) readOperation[priorityDTO] {
	return func(r *http.Request) (priorityDTO, error) {
		id, err := pathID(r, "priority_id")
		if err != nil {
			return priorityDTO{}, err
		}
		result, err := reader.GetPriority(id)
		if err != nil {
			return priorityDTO{}, readError(err, "Priority was not found")
		}
		item := priorityFromResult(*result)
		if err := localizeCatalog(r, localizer, "priority", &item); err != nil {
			return priorityDTO{}, err
		}
		return item, nil
	}
}

func priorityFromResult(result services.PriorityResult) priorityDTO {
	return priorityDTO{
		ID: result.ID, BuiltinKey: nullableString(result.BuiltinKey), Name: result.Name,
		DisplayName: result.Name, Description: result.Description, DisplayDescription: result.Description, Icon: result.Icon, Color: result.Color,
		SortOrder: result.SortOrder, IsDefault: result.IsDefault,
	}
}

type customFieldDTO struct {
	ID                             int                              `json:"id"`
	Name                           string                           `json:"name"`
	FieldType                      string                           `json:"field_type"`
	Description                    string                           `json:"description"`
	Required                       bool                             `json:"required"`
	Options                        map[string]any                   `json:"options"`
	DisplayOrder                   int                              `json:"display_order"`
	SystemDefault                  bool                             `json:"system_default"`
	AppliesToPortalCustomers       bool                             `json:"applies_to_portal_customers"`
	AppliesToCustomerOrganisations bool                             `json:"applies_to_customer_organisations"`
	AssetTypeUsages                []services.CustomFieldAssetUsage `json:"asset_type_usages"`
	Indexed                        models.CustomFieldIndexInfo      `json:"indexed"`
}

func listCustomFields(reader configurationReader) metadataOperation[[]customFieldDTO, services.CustomFieldCatalogMeta] {
	return func(*http.Request) ([]customFieldDTO, services.CustomFieldCatalogMeta, error) {
		results, meta, err := reader.ListCustomFieldsWithMeta()
		if err != nil {
			return nil, services.CustomFieldCatalogMeta{}, internalError(err)
		}
		items := make([]customFieldDTO, len(results))
		for i := range results {
			items[i], err = customFieldFromResult(results[i])
			if err != nil {
				return nil, services.CustomFieldCatalogMeta{}, internalError(err)
			}
		}
		return items, meta, nil
	}
}

func getCustomField(reader configurationReader) readOperation[customFieldDTO] {
	return func(r *http.Request) (customFieldDTO, error) {
		id, err := pathID(r, "custom_field_id")
		if err != nil {
			return customFieldDTO{}, err
		}
		result, err := reader.GetCustomField(id)
		if err != nil {
			return customFieldDTO{}, readError(err, "Custom field was not found")
		}
		item, err := customFieldFromResult(*result)
		if err != nil {
			return customFieldDTO{}, internalError(err)
		}
		return item, nil
	}
}

func customFieldFromResult(result services.CustomFieldResult) (customFieldDTO, error) {
	options, err := parseCustomFieldOptions(result.FieldType, result.Options)
	if err != nil {
		return customFieldDTO{}, fmt.Errorf("custom field %d options: %w", result.ID, err)
	}
	return customFieldDTO{
		ID: result.ID, Name: result.Name, FieldType: result.FieldType,
		Description: result.Description, Required: result.Required,
		Options: options, DisplayOrder: result.DisplayOrder,
		SystemDefault: result.SystemDefault, AppliesToPortalCustomers: result.AppliesToPortalCustomers,
		AppliesToCustomerOrganisations: result.AppliesToCustomerOrganisations,
		AssetTypeUsages:                result.AssetTypeUsages, Indexed: result.Indexed,
	}, nil
}

func parseCustomFieldOptions(fieldType, raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	if fieldType == "select" || fieldType == "multiselect" {
		options, err := models.ParseSelectOptions(raw)
		if err != nil {
			return nil, err
		}
		body, err := json.Marshal(options)
		if err != nil {
			return nil, err
		}
		var result map[string]any
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func pathID(r *http.Request, name string) (int, error) {
	raw := r.PathValue(name)
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		apiErr := newError(http.StatusBadRequest, "invalid_request", name+" must be a positive integer")
		apiErr.Details = map[string]any{"field": name}
		return 0, apiErr
	}
	return value, nil
}

func readError(err error, message string) error {
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return newError(http.StatusNotFound, "not_found", message)
	}
	var serviceErr *services.ServiceError
	if errors.As(err, &serviceErr) && serviceErr.StatusCode == http.StatusNotFound {
		return newError(http.StatusNotFound, "not_found", message)
	}
	return internalError(err)
}

func localizeCatalog(r *http.Request, localizer objectLocalizer, objectType string, value any) error {
	if localizer == nil {
		return nil
	}
	if err := localizer.LocalizeResponse(r.Context(), objecttranslation.RequestLocale(r), objectType, value); err != nil {
		return internalError(err)
	}
	return nil
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
