package v2

import (
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type catalogMutationApplication interface {
	CreateItemType(services.AuditActor, models.ItemType) (*models.ItemType, error)
	PatchItemType(services.AuditActor, int, services.ItemTypePatch) (*models.ItemType, error)
	DeleteItemType(services.AuditActor, int) error
	CreatePriority(services.AuditActor, models.Priority) (*models.Priority, error)
	PatchPriority(services.AuditActor, int, services.PriorityPatch) (*models.Priority, error)
	DeletePriority(services.AuditActor, int) error
	CreateStatus(services.AuditActor, models.Status) (*models.Status, error)
	PatchStatus(services.AuditActor, int, services.StatusPatch) (*models.Status, error)
	DeleteStatus(services.AuditActor, int) error
	CreateStatusCategory(services.AuditActor, models.StatusCategory) (*models.StatusCategory, error)
	PatchStatusCategory(services.AuditActor, int, services.StatusCategoryPatch) (*models.StatusCategory, error)
	DeleteStatusCategory(services.AuditActor, int) error
	CreateWorkflow(services.AuditActor, models.Workflow) (*services.WorkflowResult, error)
	PatchWorkflow(services.AuditActor, int, services.WorkflowPatch) (*services.WorkflowResult, error)
	DeleteWorkflow(services.AuditActor, int) error
	ReplaceWorkflowTransitions(services.AuditActor, int, []models.WorkflowTransition) ([]services.WorkflowTransitionResult, error)
	ListLinkTypes(bool) ([]models.LinkType, error)
	GetLinkType(int) (*models.LinkType, error)
	CreateLinkType(services.AuditActor, models.LinkType) (*models.LinkType, error)
	PatchLinkType(services.AuditActor, int, services.LinkTypePatch) (*models.LinkType, error)
	DeleteLinkType(services.AuditActor, int) error
}

type itemTypePatchRequest struct {
	Name                Optional[string] `json:"name"`
	Description         Optional[string] `json:"description"`
	Icon                Optional[string] `json:"icon"`
	Color               Optional[string] `json:"color"`
	IsDefault           Optional[bool]   `json:"is_default"`
	HierarchyLevel      Optional[int]    `json:"hierarchy_level"`
	SortOrder           Optional[int]    `json:"sort_order"`
	ConfigurationSetIDs Optional[[]int]  `json:"configuration_set_ids"`
}

type priorityPatchRequest struct {
	Name                Optional[string] `json:"name"`
	Description         Optional[string] `json:"description"`
	Icon                Optional[string] `json:"icon"`
	Color               Optional[string] `json:"color"`
	IsDefault           Optional[bool]   `json:"is_default"`
	SortOrder           Optional[int]    `json:"sort_order"`
	ConfigurationSetIDs Optional[[]int]  `json:"configuration_set_ids"`
}

type statusPatchRequest struct {
	Name        Optional[string] `json:"name"`
	Description Optional[string] `json:"description"`
	CategoryID  Optional[int]    `json:"category_id"`
	IsDefault   Optional[bool]   `json:"is_default"`
}

type statusCategoryPatchRequest struct {
	Name        Optional[string] `json:"name"`
	Description Optional[string] `json:"description"`
	Color       Optional[string] `json:"color"`
	IsDefault   Optional[bool]   `json:"is_default"`
	IsCompleted Optional[bool]   `json:"is_completed"`
}

type workflowPatchRequest struct {
	Name        Optional[string] `json:"name"`
	Description Optional[string] `json:"description"`
	IsDefault   Optional[bool]   `json:"is_default"`
}

type workflowTransitionsRequest struct {
	Transitions []models.WorkflowTransition `json:"transitions"`
}

type linkTypePatchRequest struct {
	Name               Optional[string]   `json:"name"`
	Description        Optional[string]   `json:"description"`
	ForwardLabel       Optional[string]   `json:"forward_label"`
	ReverseLabel       Optional[string]   `json:"reverse_label"`
	Color              Optional[string]   `json:"color"`
	Active             Optional[bool]     `json:"active"`
	AllowedEntityTypes Optional[[]string] `json:"allowed_entity_types"`
}

func createItemType(app catalogMutationApplication) jsonOperation[models.ItemType, itemTypeDTO] {
	return func(r *http.Request, input models.ItemType) (itemTypeDTO, error) {
		user, err := principal(r)
		if err != nil {
			return itemTypeDTO{}, err
		}
		item, err := app.CreateItemType(auditActor(r, user), input)
		if err != nil {
			return itemTypeDTO{}, catalogMutationError(err)
		}
		return itemTypeFromModel(*item), nil
	}
}

func patchItemType(app catalogMutationApplication) jsonOperation[itemTypePatchRequest, itemTypeDTO] {
	return func(r *http.Request, input itemTypePatchRequest) (itemTypeDTO, error) {
		user, id, err := catalogPrincipalAndID(r, "item_type_id")
		if err != nil {
			return itemTypeDTO{}, err
		}
		item, err := app.PatchItemType(auditActor(r, user), id, services.ItemTypePatch{
			Name: optionalValue(input.Name), Description: optionalValue(input.Description), Icon: optionalValue(input.Icon), Color: optionalValue(input.Color),
			IsDefault: optionalValue(input.IsDefault), HierarchyLevel: optionalValue(input.HierarchyLevel), SortOrder: optionalValue(input.SortOrder), ConfigurationSetIDs: optionalSlice(input.ConfigurationSetIDs),
		})
		if err != nil {
			return itemTypeDTO{}, catalogMutationError(err)
		}
		return itemTypeFromModel(*item), nil
	}
}

func deleteItemType(app catalogMutationApplication) commandOperation {
	return catalogDelete(app, "item_type_id", func(app catalogMutationApplication, actor services.AuditActor, id int) error {
		return app.DeleteItemType(actor, id)
	})
}

func createPriority(app catalogMutationApplication) jsonOperation[models.Priority, priorityDTO] {
	return func(r *http.Request, input models.Priority) (priorityDTO, error) {
		user, err := principal(r)
		if err != nil {
			return priorityDTO{}, err
		}
		item, err := app.CreatePriority(auditActor(r, user), input)
		if err != nil {
			return priorityDTO{}, catalogMutationError(err)
		}
		return priorityFromModel(*item), nil
	}
}

func patchPriority(app catalogMutationApplication) jsonOperation[priorityPatchRequest, priorityDTO] {
	return func(r *http.Request, input priorityPatchRequest) (priorityDTO, error) {
		user, id, err := catalogPrincipalAndID(r, "priority_id")
		if err != nil {
			return priorityDTO{}, err
		}
		item, err := app.PatchPriority(auditActor(r, user), id, services.PriorityPatch{Name: optionalValue(input.Name), Description: optionalValue(input.Description), Icon: optionalValue(input.Icon), Color: optionalValue(input.Color), IsDefault: optionalValue(input.IsDefault), SortOrder: optionalValue(input.SortOrder), ConfigurationSetIDs: optionalSlice(input.ConfigurationSetIDs)})
		if err != nil {
			return priorityDTO{}, catalogMutationError(err)
		}
		return priorityFromModel(*item), nil
	}
}

func deletePriority(app catalogMutationApplication) commandOperation {
	return catalogDelete(app, "priority_id", func(app catalogMutationApplication, actor services.AuditActor, id int) error {
		return app.DeletePriority(actor, id)
	})
}

func createStatus(app catalogMutationApplication) jsonOperation[models.Status, statusDTO] {
	return func(r *http.Request, input models.Status) (statusDTO, error) {
		user, err := principal(r)
		if err != nil {
			return statusDTO{}, err
		}
		item, err := app.CreateStatus(auditActor(r, user), input)
		if err != nil {
			return statusDTO{}, catalogMutationError(err)
		}
		return statusFromModel(*item), nil
	}
}

func patchStatus(app catalogMutationApplication) jsonOperation[statusPatchRequest, statusDTO] {
	return func(r *http.Request, input statusPatchRequest) (statusDTO, error) {
		user, id, err := catalogPrincipalAndID(r, "status_id")
		if err != nil {
			return statusDTO{}, err
		}
		item, err := app.PatchStatus(auditActor(r, user), id, services.StatusPatch{Name: optionalValue(input.Name), Description: optionalValue(input.Description), CategoryID: optionalValue(input.CategoryID), IsDefault: optionalValue(input.IsDefault)})
		if err != nil {
			return statusDTO{}, catalogMutationError(err)
		}
		return statusFromModel(*item), nil
	}
}

func deleteStatus(app catalogMutationApplication) commandOperation {
	return catalogDelete(app, "status_id", func(app catalogMutationApplication, actor services.AuditActor, id int) error {
		return app.DeleteStatus(actor, id)
	})
}

func createStatusCategory(app catalogMutationApplication) jsonOperation[models.StatusCategory, statusCategoryDTO] {
	return func(r *http.Request, input models.StatusCategory) (statusCategoryDTO, error) {
		user, err := principal(r)
		if err != nil {
			return statusCategoryDTO{}, err
		}
		item, err := app.CreateStatusCategory(auditActor(r, user), input)
		if err != nil {
			return statusCategoryDTO{}, catalogMutationError(err)
		}
		return statusCategoryFromModel(*item), nil
	}
}

func patchStatusCategory(app catalogMutationApplication) jsonOperation[statusCategoryPatchRequest, statusCategoryDTO] {
	return func(r *http.Request, input statusCategoryPatchRequest) (statusCategoryDTO, error) {
		user, id, err := catalogPrincipalAndID(r, "category_id")
		if err != nil {
			return statusCategoryDTO{}, err
		}
		item, err := app.PatchStatusCategory(auditActor(r, user), id, services.StatusCategoryPatch{Name: optionalValue(input.Name), Description: optionalValue(input.Description), Color: optionalValue(input.Color), IsDefault: optionalValue(input.IsDefault), IsCompleted: optionalValue(input.IsCompleted)})
		if err != nil {
			return statusCategoryDTO{}, catalogMutationError(err)
		}
		return statusCategoryFromModel(*item), nil
	}
}

func deleteStatusCategory(app catalogMutationApplication) commandOperation {
	return catalogDelete(app, "category_id", func(app catalogMutationApplication, actor services.AuditActor, id int) error {
		return app.DeleteStatusCategory(actor, id)
	})
}

func createWorkflow(app catalogMutationApplication) jsonOperation[models.Workflow, workflowDTO] {
	return func(r *http.Request, input models.Workflow) (workflowDTO, error) {
		user, err := principal(r)
		if err != nil {
			return workflowDTO{}, err
		}
		item, err := app.CreateWorkflow(auditActor(r, user), input)
		if err != nil {
			return workflowDTO{}, catalogMutationError(err)
		}
		return workflowFromResult(*item), nil
	}
}

func patchWorkflow(app catalogMutationApplication) jsonOperation[workflowPatchRequest, workflowDTO] {
	return func(r *http.Request, input workflowPatchRequest) (workflowDTO, error) {
		user, id, err := catalogPrincipalAndID(r, "workflow_id")
		if err != nil {
			return workflowDTO{}, err
		}
		item, err := app.PatchWorkflow(auditActor(r, user), id, services.WorkflowPatch{Name: optionalValue(input.Name), Description: optionalValue(input.Description), IsDefault: optionalValue(input.IsDefault)})
		if err != nil {
			return workflowDTO{}, catalogMutationError(err)
		}
		return workflowFromResult(*item), nil
	}
}

func deleteWorkflow(app catalogMutationApplication) commandOperation {
	return catalogDelete(app, "workflow_id", func(app catalogMutationApplication, actor services.AuditActor, id int) error {
		return app.DeleteWorkflow(actor, id)
	})
}

func replaceWorkflowTransitions(app catalogMutationApplication) jsonOperation[workflowTransitionsRequest, []workflowTransitionDTO] {
	return func(r *http.Request, input workflowTransitionsRequest) ([]workflowTransitionDTO, error) {
		user, id, err := catalogPrincipalAndID(r, "workflow_id")
		if err != nil {
			return nil, err
		}
		items, err := app.ReplaceWorkflowTransitions(auditActor(r, user), id, input.Transitions)
		if err != nil {
			return nil, catalogMutationError(err)
		}
		return workflowTransitionsFromResults(items), nil
	}
}

func catalogDelete(app catalogMutationApplication, pathName string, operation func(catalogMutationApplication, services.AuditActor, int) error) commandOperation {
	return func(r *http.Request) error {
		user, id, err := catalogPrincipalAndID(r, pathName)
		if err != nil {
			return err
		}
		return catalogMutationError(operation(app, auditActor(r, user), id))
	}
}

func catalogPrincipalAndID(r *http.Request, name string) (*models.User, int, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	id, err := pathID(r, name)
	return user, id, err
}

func optionalSlice[T any](value Optional[[]T]) *[]T {
	if !value.Set || value.Null {
		return nil
	}
	return &value.Value
}

func itemTypeFromModel(item models.ItemType) itemTypeDTO {
	return itemTypeDTO{ID: item.ID, BuiltinKey: nullableString(item.BuiltinKey), Name: item.Name, DisplayName: item.Name, Description: item.Description, DisplayDescription: item.Description, Icon: item.Icon, Color: item.Color, HierarchyLevel: item.HierarchyLevel, SortOrder: item.SortOrder, IsDefault: item.IsDefault}
}
func priorityFromModel(item models.Priority) priorityDTO {
	return priorityDTO{ID: item.ID, BuiltinKey: nullableString(item.BuiltinKey), Name: item.Name, DisplayName: item.Name, Description: item.Description, DisplayDescription: item.Description, Icon: item.Icon, Color: item.Color, SortOrder: item.SortOrder, IsDefault: item.IsDefault}
}
func statusCategoryFromModel(item models.StatusCategory) statusCategoryDTO {
	return statusCategoryDTO{ID: item.ID, BuiltinKey: nullableString(item.BuiltinKey), Name: item.Name, DisplayName: item.Name, Description: item.Description, DisplayDescription: item.Description, Color: item.Color, IsDefault: item.IsDefault, IsCompleted: item.IsCompleted}
}
func statusFromModel(item models.Status) statusDTO {
	return statusDTO{ID: item.ID, BuiltinKey: nullableString(item.BuiltinKey), Name: item.Name, DisplayName: item.Name, Description: item.Description, DisplayDescription: item.Description, IsDefault: item.IsDefault, Category: statusCategoryDTO{ID: item.CategoryID, BuiltinKey: nullableString(item.CategoryBuiltinKey), Name: item.CategoryName, DisplayName: item.CategoryName, Description: item.CategoryDescription, DisplayDescription: item.CategoryDescription, Color: item.CategoryColor, IsDefault: item.CategoryIsDefault, IsCompleted: item.IsCompleted}}
}

func workflowTransitionsFromResults(results []services.WorkflowTransitionResult) []workflowTransitionDTO {
	items := make([]workflowTransitionDTO, len(results))
	for i, result := range results {
		item := workflowTransitionDTO{ID: result.ID, FromAllStatuses: result.FromAllStatuses, To: transitionStatusDTO{ID: result.ToStatusID, BuiltinKey: nullableString(result.ToStatusBuiltinKey), Name: result.ToStatusName, CategoryBuiltinKey: nullableString(result.ToCategoryBuiltinKey), CategoryName: result.ToCategoryName, CategoryColor: result.ToCategoryColor}}
		if result.FromStatusID != nil {
			item.From = &transitionStatusDTO{ID: *result.FromStatusID, BuiltinKey: nullableString(result.FromStatusBuiltinKey), Name: result.FromStatusName, CategoryBuiltinKey: nullableString(result.FromCategoryBuiltinKey), CategoryName: result.FromCategoryName, CategoryColor: result.FromCategoryColor}
		}
		items[i] = item
	}
	return items
}

func catalogMutationError(err error) error {
	if err == nil {
		return nil
	}
	var serviceErr *services.ServiceError
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Catalog resource was not found")
	case errors.Is(err, services.ErrCatalogForbidden):
		return newError(http.StatusForbidden, "forbidden", "System administration permission is required")
	case errors.As(err, &serviceErr):
		return newError(serviceErr.StatusCode, catalogErrorCode(serviceErr.StatusCode), serviceErr.Message)
	case errors.Is(err, repository.ErrTransitionToStatusRequired), errors.Is(err, repository.ErrTransitionToStatusNotFound), errors.Is(err, repository.ErrTransitionFromStatusNotFound):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}

func catalogErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	default:
		return "catalog_error"
	}
}
