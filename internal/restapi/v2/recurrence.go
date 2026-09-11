package v2

import (
	"errors"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerRecurrenceRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/items/{item_id}/recurrence", AuthAuthenticated, []string{"items:read"}, getRecurrence(deps))
	builder.JSON(http.MethodPost, "/items/{item_id}/recurrence", http.StatusCreated, false, AuthAuthenticated, []string{"items:write"}, createRecurrence(deps))
	builder.JSON(http.MethodPatch, "/items/{item_id}/recurrence", http.StatusOK, true, AuthAuthenticated, []string{"items:write"}, updateRecurrence(deps))
	builder.Command(http.MethodDelete, "/items/{item_id}/recurrence", AuthAuthenticated, []string{"items:write"}, deleteRecurrence(deps))
	builder.Page("/items/{item_id}/recurrence/instances", AuthAuthenticated, []string{"items:read"}, listRecurrenceInstances(deps))
	builder.Action(http.MethodPost, "/items/{item_id}/recurrence/generate", http.StatusOK, AuthAuthenticated, []string{"items:write"}, generateRecurrence(deps))
	builder.JSON(http.MethodPost, "/recurrence-rules/preview", http.StatusOK, false, AuthAuthenticated, []string{"items:read"}, previewRecurrence(deps.Recurrence))
	builder.Read("/workspaces/{workspace_id}/recurrence-rules", AuthAuthenticated, []string{"items:read"}, listWorkspaceRecurrences(deps))
}

type recurrencePatchRequest struct {
	RRule            Optional[string] `json:"rrule"`
	DtStart          Optional[string] `json:"dtstart"`
	DtEnd            Optional[string] `json:"dtend"`
	Timezone         Optional[string] `json:"timezone"`
	LeadTimeDays     Optional[int]    `json:"lead_time_days"`
	CopyAssignee     Optional[bool]   `json:"copy_assignee"`
	CopyPriority     Optional[bool]   `json:"copy_priority"`
	CopyCustomFields Optional[bool]   `json:"copy_custom_fields"`
	CopyDescription  Optional[bool]   `json:"copy_description"`
	StatusOnCreate   Optional[int]    `json:"status_on_create"`
	IsActive         Optional[bool]   `json:"is_active"`
}

type recurrenceDTO struct {
	ID                  int        `json:"id"`
	TemplateItemID      int        `json:"template_item_id"`
	WorkspaceID         int        `json:"workspace_id"`
	RRule               string     `json:"rrule"`
	DtStart             time.Time  `json:"dtstart"`
	DtEnd               *time.Time `json:"dtend"`
	Timezone            string     `json:"timezone"`
	LeadTimeDays        int        `json:"lead_time_days"`
	LastGeneratedUntil  *time.Time `json:"last_generated_until"`
	NextGenerationCheck *time.Time `json:"next_generation_check"`
	CopyAssignee        bool       `json:"copy_assignee"`
	CopyPriority        bool       `json:"copy_priority"`
	CopyCustomFields    bool       `json:"copy_custom_fields"`
	CopyDescription     bool       `json:"copy_description"`
	StatusOnCreate      *int       `json:"status_on_create"`
	IsActive            bool       `json:"is_active"`
	CreatedBy           *int       `json:"created_by"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	TemplateTitle       string     `json:"template_title"`
	WorkspaceName       string     `json:"workspace_name"`
	WorkspaceKey        string     `json:"workspace_key"`
	CreatorName         string     `json:"creator_name"`
	InstanceCount       int        `json:"instance_count"`
	NextOccurrence      *time.Time `json:"next_occurrence"`
}

type recurrenceInstanceDTO struct {
	ID               int       `json:"id"`
	RecurrenceRuleID int       `json:"recurrence_rule_id"`
	InstanceItemID   int       `json:"instance_item_id"`
	ScheduledDate    time.Time `json:"scheduled_date"`
	SequenceNumber   int       `json:"sequence_number"`
	CreatedAt        time.Time `json:"created_at"`
	ItemTitle        string    `json:"item_title"`
	ItemStatus       string    `json:"item_status"`
}

type recurrenceGenerationDTO struct {
	GeneratedCount int `json:"generated_count"`
}

func getRecurrence(deps Deps) readOperation[*recurrenceDTO] {
	return func(r *http.Request) (*recurrenceDTO, error) {
		item, err := requireItem(r, deps, deps.Access.CanViewWorkspace)
		if err != nil {
			return nil, err
		}
		rule, err := deps.Recurrence.Get(item.ID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, internalError(err)
		}
		return recurrenceFromModel(rule), nil
	}
}

func createRecurrence(deps Deps) jsonOperation[models.CreateRecurrenceRequest, recurrenceDTO] {
	return func(r *http.Request, input models.CreateRecurrenceRequest) (recurrenceDTO, error) {
		user, err := principal(r)
		if err != nil {
			return recurrenceDTO{}, err
		}
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return recurrenceDTO{}, err
		}
		rule, err := deps.Recurrence.Create(item.ID, item.WorkspaceID, user.ID, input, auditActor(r, user))
		if err != nil {
			return recurrenceDTO{}, recurrenceError(err)
		}
		return *recurrenceFromModel(rule), nil
	}
}

func updateRecurrence(deps Deps) jsonOperation[recurrencePatchRequest, recurrenceDTO] {
	return func(r *http.Request, input recurrencePatchRequest) (recurrenceDTO, error) {
		user, err := principal(r)
		if err != nil {
			return recurrenceDTO{}, err
		}
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return recurrenceDTO{}, err
		}
		update, err := recurrenceUpdate(input)
		if err != nil {
			return recurrenceDTO{}, err
		}
		rule, err := deps.Recurrence.UpdateWithPatch(item.ID, update, auditActor(r, user))
		if err != nil {
			return recurrenceDTO{}, recurrenceError(err)
		}
		return *recurrenceFromModel(rule), nil
	}
}

func deleteRecurrence(deps Deps) commandOperation {
	return func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return err
		}
		return recurrenceError(deps.Recurrence.Delete(item.ID, auditActor(r, user)))
	}
}

func listRecurrenceInstances(deps Deps) pageOperation[recurrenceInstanceDTO] {
	return func(r *http.Request) ([]recurrenceInstanceDTO, Pagination, int, error) {
		item, err := requireItem(r, deps, deps.Access.CanViewWorkspace)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		result, err := deps.Recurrence.ListInstances(item.ID, page.PageSize, page.Offset)
		if err != nil {
			return nil, Pagination{}, 0, recurrenceError(err)
		}
		items := make([]recurrenceInstanceDTO, len(result.Items))
		for i, instance := range result.Items {
			items[i] = recurrenceInstanceFromModel(instance)
		}
		return items, page, result.Total, nil
	}
}

func generateRecurrence(deps Deps) actionOperation[recurrenceGenerationDTO] {
	return func(r *http.Request) (recurrenceGenerationDTO, error) {
		user, err := principal(r)
		if err != nil {
			return recurrenceGenerationDTO{}, err
		}
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return recurrenceGenerationDTO{}, err
		}
		count, err := deps.Recurrence.ForceGenerate(item.ID, auditActor(r, user))
		if err != nil {
			return recurrenceGenerationDTO{}, recurrenceError(err)
		}
		return recurrenceGenerationDTO{GeneratedCount: count}, nil
	}
}

func previewRecurrence(application recurrenceApplication) jsonOperation[models.RRulePreviewRequest, services.RecurrencePreview] {
	return func(_ *http.Request, input models.RRulePreviewRequest) (services.RecurrencePreview, error) {
		preview, err := application.Preview(input)
		if err != nil {
			return services.RecurrencePreview{}, recurrenceError(err)
		}
		return *preview, nil
	}
}

func listWorkspaceRecurrences(deps Deps) readOperation[[]recurrenceDTO] {
	return func(r *http.Request) ([]recurrenceDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		if err := requireWorkspace(deps.Access.CanViewWorkspace, user.ID, workspaceID); err != nil {
			return nil, err
		}
		rules, err := deps.Recurrence.ListByWorkspace(workspaceID)
		if err != nil {
			return nil, internalError(err)
		}
		result := make([]recurrenceDTO, len(rules))
		for i, rule := range rules {
			result[i] = *recurrenceFromModel(rule)
		}
		return result, nil
	}
}

func recurrenceUpdate(input recurrencePatchRequest) (services.RecurrenceUpdate, error) {
	if input.RRule.Null || input.DtStart.Null || input.Timezone.Null || input.LeadTimeDays.Null ||
		input.CopyAssignee.Null || input.CopyPriority.Null || input.CopyCustomFields.Null ||
		input.CopyDescription.Null || input.IsActive.Null {
		return services.RecurrenceUpdate{}, newError(http.StatusBadRequest, "invalid_request", "Only dtend and status_on_create may be null")
	}
	return services.RecurrenceUpdate{
		RRule: optionalValue(input.RRule), DtStart: optionalValue(input.DtStart),
		DtEndSet: input.DtEnd.Set, DtEnd: optionalNullableValue(input.DtEnd),
		Timezone: optionalValue(input.Timezone), LeadTimeDays: optionalValue(input.LeadTimeDays),
		CopyAssignee: optionalValue(input.CopyAssignee), CopyPriority: optionalValue(input.CopyPriority),
		CopyCustomFields: optionalValue(input.CopyCustomFields), CopyDescription: optionalValue(input.CopyDescription),
		StatusOnCreateSet: input.StatusOnCreate.Set, StatusOnCreate: optionalNullableValue(input.StatusOnCreate),
		IsActive: optionalValue(input.IsActive),
	}, nil
}

func optionalValue[T any](value Optional[T]) *T {
	if !value.Set {
		return nil
	}
	return &value.Value
}

func optionalNullableValue[T any](value Optional[T]) *T {
	if !value.Set || value.Null {
		return nil
	}
	return &value.Value
}

func recurrenceFromModel(rule *models.RecurrenceRule) *recurrenceDTO {
	return &recurrenceDTO{
		ID: rule.ID, TemplateItemID: rule.TemplateItemID, WorkspaceID: rule.WorkspaceID,
		RRule: rule.RRule, DtStart: rule.DtStart, DtEnd: rule.DtEnd, Timezone: rule.Timezone,
		LeadTimeDays: rule.LeadTimeDays, LastGeneratedUntil: rule.LastGeneratedUntil,
		NextGenerationCheck: rule.NextGenerationCheck, CopyAssignee: rule.CopyAssignee,
		CopyPriority: rule.CopyPriority, CopyCustomFields: rule.CopyCustomFields,
		CopyDescription: rule.CopyDescription, StatusOnCreate: rule.StatusOnCreate,
		IsActive: rule.IsActive, CreatedBy: rule.CreatedBy, CreatedAt: rule.CreatedAt,
		UpdatedAt: rule.UpdatedAt, TemplateTitle: rule.TemplateTitle,
		WorkspaceName: rule.WorkspaceName, WorkspaceKey: rule.WorkspaceKey,
		CreatorName: rule.CreatorName, InstanceCount: rule.InstanceCount,
		NextOccurrence: rule.NextOccurrence,
	}
}

func recurrenceInstanceFromModel(instance *models.RecurrenceInstance) recurrenceInstanceDTO {
	return recurrenceInstanceDTO{
		ID: instance.ID, RecurrenceRuleID: instance.RecurrenceRuleID,
		InstanceItemID: instance.InstanceItemID, ScheduledDate: instance.ScheduledDate,
		SequenceNumber: instance.SequenceNumber, CreatedAt: instance.CreatedAt,
		ItemTitle: instance.ItemTitle, ItemStatus: instance.ItemStatus,
	}
}

func recurrenceError(err error) error {
	if err == nil {
		return nil
	}
	if validation, ok := services.AsRecurrenceValidationError(err); ok {
		return newError(http.StatusBadRequest, "invalid_request", validation.Message)
	}
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Recurrence rule was not found")
	case errors.Is(err, services.ErrRecurrenceConflict):
		return newError(http.StatusConflict, "conflict", "Recurrence rule already exists for this item")
	case errors.Is(err, services.ErrRecurrenceWorkspaceLimit):
		return newError(http.StatusConflict, "conflict", services.RecurrenceWorkspaceLimitMessage())
	default:
		return internalError(err)
	}
}
