package v2

import (
	"context"
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type conditionSetApplication interface {
	List(*int) ([]models.ConditionSet, error)
	Get(int) (*models.ConditionSet, error)
	Create(services.AuditActor, models.ConditionSet) (*models.ConditionSet, error)
	Patch(services.AuditActor, int, services.ConditionSetPatch) (*models.ConditionSet, error)
	Delete(services.AuditActor, int) error
}

type governanceApplication interface {
	ListApprovalSets(context.Context, *int) ([]models.ApprovalSet, error)
	GetApprovalSet(context.Context, int) (*models.ApprovalSet, error)
	CreateApprovalSet(context.Context, services.AuditActor, models.ApprovalSet) (*models.ApprovalSet, error)
	PatchApprovalSet(context.Context, services.AuditActor, int, services.ApprovalSetPatch) (*models.ApprovalSet, error)
	DeleteApprovalSet(context.Context, services.AuditActor, int) error
	ListItemApprovals(context.Context, int, int, int, int) ([]*models.ApprovalRequest, int, error)
	ListMyApprovals(context.Context, int, string, int, int) ([]*models.ApprovalRequest, int, error)
	GetApproval(context.Context, int, int) (*models.ApprovalRequest, error)
	DecideApproval(context.Context, services.AuditActor, int, string, string) (*services.ApprovalDecisionResult, error)
	CancelApproval(context.Context, services.AuditActor, int, string) (*models.ApprovalRequest, error)
	DelegateApproval(context.Context, services.AuditActor, int, int, string) (*models.ApprovalRequest, error)
	RefreshApprovalApprovers(context.Context, services.AuditActor, int, int, string) (*models.ApprovalRequest, error)
	EscalateApproval(context.Context, services.AuditActor, int, int) (*models.ApprovalRequest, error)
	GetTransitionGovernance(context.Context, int) (*services.TransitionGovernance, error)
}

func registerGovernanceRoutes(builder *routeBuilder, conditions conditionSetApplication, governance governanceApplication) {
	builder.Page("/condition-sets", AuthAuthenticated, []string{"workflows:read"}, listConditionSets(conditions))
	builder.JSON(http.MethodPost, "/condition-sets", http.StatusCreated, false, AuthAuthenticated, []string{"workflows:write"}, createConditionSet(conditions))
	builder.Read("/condition-sets/{condition_set_id}", AuthAuthenticated, []string{"workflows:read"}, getConditionSet(conditions))
	builder.JSON(http.MethodPatch, "/condition-sets/{condition_set_id}", http.StatusOK, true, AuthAuthenticated, []string{"workflows:write"}, patchConditionSet(conditions))
	builder.Command(http.MethodDelete, "/condition-sets/{condition_set_id}", AuthAuthenticated, []string{"workflows:write"}, deleteConditionSet(conditions))
	builder.Read("/workflows/{workflow_id}/condition-sets", AuthAuthenticated, []string{"workflows:read"}, listWorkflowConditionSets(conditions))
	builder.Read("/transitions/{transition_id}/governance", AuthAuthenticated, []string{"workflows:read"}, getTransitionGovernance(governance))
	builder.Page("/approval-sets", AuthAuthenticated, []string{"workflows:read"}, listApprovalSets(governance))
	builder.JSON(http.MethodPost, "/approval-sets", http.StatusCreated, false, AuthAuthenticated, []string{"workflows:write"}, createApprovalSet(governance))
	builder.Read("/approval-sets/{approval_set_id}", AuthAuthenticated, []string{"workflows:read"}, getApprovalSet(governance))
	builder.JSON(http.MethodPatch, "/approval-sets/{approval_set_id}", http.StatusOK, true, AuthAuthenticated, []string{"workflows:write"}, patchApprovalSet(governance))
	builder.Command(http.MethodDelete, "/approval-sets/{approval_set_id}", AuthAuthenticated, []string{"workflows:write"}, deleteApprovalSet(governance))
	builder.Read("/workflows/{workflow_id}/approval-sets", AuthAuthenticated, []string{"workflows:read"}, listWorkflowApprovalSets(governance))
	builder.Page("/items/{item_id}/approvals", AuthAuthenticated, []string{"approvals:read"}, listItemApprovals(governance))
	builder.Page("/approvals/mine", AuthAuthenticated, []string{"approvals:read"}, listMyApprovals(governance))
	builder.Read("/approvals/{approval_id}", AuthAuthenticated, []string{"approvals:read"}, getApproval(governance))
	builder.JSON(http.MethodPost, "/approvals/{approval_id}/decisions", http.StatusOK, false, AuthAuthenticated, []string{"approvals:write"}, decideApproval(governance))
	builder.JSON(http.MethodPost, "/approvals/{approval_id}/cancellation", http.StatusOK, false, AuthAuthenticated, []string{"approvals:write"}, cancelApproval(governance))
	builder.JSON(http.MethodPost, "/approvals/{approval_id}/delegations", http.StatusOK, false, AuthAuthenticated, []string{"approvals:write"}, delegateApproval(governance))
	builder.JSON(http.MethodPost, "/approvals/{approval_id}/steps/{step_id}/approver-refreshes", http.StatusOK, false, AuthAuthenticated, []string{"approvals:write"}, refreshApprovalApprovers(governance))
	builder.Action(http.MethodPost, "/approvals/{approval_id}/steps/{step_id}/escalations", http.StatusOK, AuthAuthenticated, []string{"approvals:write"}, escalateApproval(governance))
}

type conditionSetPatchRequest struct {
	Name                 Optional[string]                       `json:"name"`
	Description          Optional[string]                       `json:"description"`
	WorkflowID           Optional[int]                          `json:"workflow_id"`
	TransitionConditions Optional[[]models.TransitionCondition] `json:"transition_conditions"`
}

type approvalSetPatchRequest struct {
	Name        Optional[string]                     `json:"name"`
	Description Optional[string]                     `json:"description"`
	WorkflowID  Optional[int]                        `json:"workflow_id"`
	SetStatuses Optional[[]models.ApprovalSetStatus] `json:"set_statuses"`
}

type approvalCommentRequest struct {
	Comment string `json:"comment"`
}
type approvalDecisionRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}
type approvalDelegationRequest struct {
	ToUserID int    `json:"to_user_id"`
	Comment  string `json:"comment"`
}

func listConditionSets(app conditionSetApplication) pageOperation[models.ConditionSet] {
	return func(r *http.Request) ([]models.ConditionSet, Pagination, int, error) {
		page, err := ParsePage(r)
		if err != nil {
			return nil, page, 0, err
		}
		workflowID, err := optionalPositiveQueryID(r, "workflow_id")
		if err != nil {
			return nil, page, 0, err
		}
		items, err := app.List(workflowID)
		if err != nil {
			return nil, page, 0, governanceError(err)
		}
		slice, total := paginate(items, page)
		return slice, page, total, nil
	}
}
func listWorkflowConditionSets(app conditionSetApplication) readOperation[[]models.ConditionSet] {
	return func(r *http.Request) ([]models.ConditionSet, error) {
		id, err := pathID(r, "workflow_id")
		if err != nil {
			return nil, err
		}
		items, err := app.List(&id)
		return items, governanceError(err)
	}
}
func getConditionSet(app conditionSetApplication) readOperation[models.ConditionSet] {
	return func(r *http.Request) (models.ConditionSet, error) {
		id, err := pathID(r, "condition_set_id")
		if err != nil {
			return models.ConditionSet{}, err
		}
		item, err := app.Get(id)
		if err != nil {
			return models.ConditionSet{}, governanceError(err)
		}
		return *item, nil
	}
}
func createConditionSet(app conditionSetApplication) jsonOperation[models.ConditionSet, models.ConditionSet] {
	return func(r *http.Request, input models.ConditionSet) (models.ConditionSet, error) {
		user, err := principal(r)
		if err != nil {
			return models.ConditionSet{}, err
		}
		item, err := app.Create(auditActor(r, user), input)
		if err != nil {
			return models.ConditionSet{}, governanceError(err)
		}
		return *item, nil
	}
}
func patchConditionSet(app conditionSetApplication) jsonOperation[conditionSetPatchRequest, models.ConditionSet] {
	return func(r *http.Request, input conditionSetPatchRequest) (models.ConditionSet, error) {
		user, id, err := governancePrincipalAndID(r, "condition_set_id")
		if err != nil {
			return models.ConditionSet{}, err
		}
		item, err := app.Patch(auditActor(r, user), id, services.ConditionSetPatch{Name: optionalValue(input.Name), Description: optionalValue(input.Description), WorkflowID: optionalValue(input.WorkflowID), TransitionConditions: optionalSlice(input.TransitionConditions)})
		if err != nil {
			return models.ConditionSet{}, governanceError(err)
		}
		return *item, nil
	}
}
func deleteConditionSet(app conditionSetApplication) commandOperation {
	return func(r *http.Request) error {
		user, id, err := governancePrincipalAndID(r, "condition_set_id")
		if err != nil {
			return err
		}
		return governanceError(app.Delete(auditActor(r, user), id))
	}
}

func listApprovalSets(app governanceApplication) pageOperation[models.ApprovalSet] {
	return func(r *http.Request) ([]models.ApprovalSet, Pagination, int, error) {
		page, err := ParsePage(r)
		if err != nil {
			return nil, page, 0, err
		}
		workflowID, err := optionalPositiveQueryID(r, "workflow_id")
		if err != nil {
			return nil, page, 0, err
		}
		items, err := app.ListApprovalSets(r.Context(), workflowID)
		if err != nil {
			return nil, page, 0, governanceError(err)
		}
		slice, total := paginate(items, page)
		return slice, page, total, nil
	}
}
func listWorkflowApprovalSets(app governanceApplication) readOperation[[]models.ApprovalSet] {
	return func(r *http.Request) ([]models.ApprovalSet, error) {
		id, err := pathID(r, "workflow_id")
		if err != nil {
			return nil, err
		}
		items, err := app.ListApprovalSets(r.Context(), &id)
		return items, governanceError(err)
	}
}
func getApprovalSet(app governanceApplication) readOperation[models.ApprovalSet] {
	return func(r *http.Request) (models.ApprovalSet, error) {
		id, err := pathID(r, "approval_set_id")
		if err != nil {
			return models.ApprovalSet{}, err
		}
		item, err := app.GetApprovalSet(r.Context(), id)
		if err != nil {
			return models.ApprovalSet{}, governanceError(err)
		}
		return *item, nil
	}
}
func createApprovalSet(app governanceApplication) jsonOperation[models.ApprovalSet, models.ApprovalSet] {
	return func(r *http.Request, input models.ApprovalSet) (models.ApprovalSet, error) {
		user, err := principal(r)
		if err != nil {
			return models.ApprovalSet{}, err
		}
		item, err := app.CreateApprovalSet(r.Context(), auditActor(r, user), input)
		if err != nil {
			return models.ApprovalSet{}, governanceError(err)
		}
		return *item, nil
	}
}
func patchApprovalSet(app governanceApplication) jsonOperation[approvalSetPatchRequest, models.ApprovalSet] {
	return func(r *http.Request, input approvalSetPatchRequest) (models.ApprovalSet, error) {
		user, id, err := governancePrincipalAndID(r, "approval_set_id")
		if err != nil {
			return models.ApprovalSet{}, err
		}
		item, err := app.PatchApprovalSet(r.Context(), auditActor(r, user), id, services.ApprovalSetPatch{Name: optionalValue(input.Name), Description: optionalValue(input.Description), WorkflowID: optionalValue(input.WorkflowID), SetStatuses: optionalSlice(input.SetStatuses)})
		if err != nil {
			return models.ApprovalSet{}, governanceError(err)
		}
		return *item, nil
	}
}
func deleteApprovalSet(app governanceApplication) commandOperation {
	return func(r *http.Request) error {
		user, id, err := governancePrincipalAndID(r, "approval_set_id")
		if err != nil {
			return err
		}
		return governanceError(app.DeleteApprovalSet(r.Context(), auditActor(r, user), id))
	}
}

func listItemApprovals(app governanceApplication) pageOperation[*models.ApprovalRequest] {
	return func(r *http.Request) ([]*models.ApprovalRequest, Pagination, int, error) {
		user, id, page, err := approvalListContext(r, "item_id")
		if err != nil {
			return nil, page, 0, err
		}
		items, total, err := app.ListItemApprovals(r.Context(), user.ID, id, page.PageSize, page.Offset)
		if err != nil {
			return nil, page, 0, governanceError(err)
		}
		return items, page, total, nil
	}
}
func listMyApprovals(app governanceApplication) pageOperation[*models.ApprovalRequest] {
	return func(r *http.Request) ([]*models.ApprovalRequest, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, page, 0, err
		}
		items, total, err := app.ListMyApprovals(r.Context(), user.ID, r.URL.Query().Get("status"), page.PageSize, page.Offset)
		if err != nil {
			return nil, page, 0, governanceError(err)
		}
		return items, page, total, nil
	}
}
func getApproval(app governanceApplication) readOperation[models.ApprovalRequest] {
	return func(r *http.Request) (models.ApprovalRequest, error) {
		user, id, err := governancePrincipalAndID(r, "approval_id")
		if err != nil {
			return models.ApprovalRequest{}, err
		}
		item, err := app.GetApproval(r.Context(), user.ID, id)
		if err != nil {
			return models.ApprovalRequest{}, governanceError(err)
		}
		return *item, nil
	}
}
func decideApproval(app governanceApplication) jsonOperation[approvalDecisionRequest, services.ApprovalDecisionResult] {
	return func(r *http.Request, input approvalDecisionRequest) (services.ApprovalDecisionResult, error) {
		user, id, err := governancePrincipalAndID(r, "approval_id")
		if err != nil {
			return services.ApprovalDecisionResult{}, err
		}
		item, err := app.DecideApproval(r.Context(), auditActor(r, user), id, input.Decision, input.Comment)
		if err != nil {
			return services.ApprovalDecisionResult{}, governanceError(err)
		}
		return *item, nil
	}
}
func cancelApproval(app governanceApplication) jsonOperation[approvalCommentRequest, models.ApprovalRequest] {
	return func(r *http.Request, input approvalCommentRequest) (models.ApprovalRequest, error) {
		user, id, err := governancePrincipalAndID(r, "approval_id")
		if err != nil {
			return models.ApprovalRequest{}, err
		}
		item, err := app.CancelApproval(r.Context(), auditActor(r, user), id, input.Comment)
		if err != nil {
			return models.ApprovalRequest{}, governanceError(err)
		}
		return *item, nil
	}
}
func delegateApproval(app governanceApplication) jsonOperation[approvalDelegationRequest, models.ApprovalRequest] {
	return func(r *http.Request, input approvalDelegationRequest) (models.ApprovalRequest, error) {
		user, id, err := governancePrincipalAndID(r, "approval_id")
		if err != nil {
			return models.ApprovalRequest{}, err
		}
		item, err := app.DelegateApproval(r.Context(), auditActor(r, user), id, input.ToUserID, input.Comment)
		if err != nil {
			return models.ApprovalRequest{}, governanceError(err)
		}
		return *item, nil
	}
}
func refreshApprovalApprovers(app governanceApplication) jsonOperation[approvalCommentRequest, models.ApprovalRequest] {
	return func(r *http.Request, input approvalCommentRequest) (models.ApprovalRequest, error) {
		user, id, err := governancePrincipalAndID(r, "approval_id")
		if err != nil {
			return models.ApprovalRequest{}, err
		}
		stepID, err := pathID(r, "step_id")
		if err != nil {
			return models.ApprovalRequest{}, err
		}
		item, err := app.RefreshApprovalApprovers(r.Context(), auditActor(r, user), id, stepID, input.Comment)
		if err != nil {
			return models.ApprovalRequest{}, governanceError(err)
		}
		return *item, nil
	}
}
func escalateApproval(app governanceApplication) actionOperation[models.ApprovalRequest] {
	return func(r *http.Request) (models.ApprovalRequest, error) {
		user, id, err := governancePrincipalAndID(r, "approval_id")
		if err != nil {
			return models.ApprovalRequest{}, err
		}
		stepID, err := pathID(r, "step_id")
		if err != nil {
			return models.ApprovalRequest{}, err
		}
		item, err := app.EscalateApproval(r.Context(), auditActor(r, user), id, stepID)
		if err != nil {
			return models.ApprovalRequest{}, governanceError(err)
		}
		return *item, nil
	}
}
func getTransitionGovernance(app governanceApplication) readOperation[services.TransitionGovernance] {
	return func(r *http.Request) (services.TransitionGovernance, error) {
		id, err := pathID(r, "transition_id")
		if err != nil {
			return services.TransitionGovernance{}, err
		}
		item, err := app.GetTransitionGovernance(r.Context(), id)
		if err != nil {
			return services.TransitionGovernance{}, governanceError(err)
		}
		return *item, nil
	}
}

func approvalListContext(r *http.Request, idName string) (*models.User, int, Pagination, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, Pagination{}, err
	}
	id, err := pathID(r, idName)
	if err != nil {
		return nil, 0, Pagination{}, err
	}
	page, err := ParsePage(r)
	return user, id, page, err
}
func governancePrincipalAndID(r *http.Request, name string) (*models.User, int, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	id, err := pathID(r, name)
	return user, id, err
}
func paginate[T any](items []T, page Pagination) (pageItems []T, total int) {
	total = len(items)
	start := min(page.Offset, total)
	end := min(start+page.PageSize, total)
	return items[start:end], total
}

func governanceError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, services.ErrGovernanceNotFound):
		return newError(http.StatusNotFound, "not_found", "Governance resource was not found")
	case errors.Is(err, services.ErrGovernanceForbidden):
		return newError(http.StatusForbidden, "forbidden", "System administration permission is required")
	case errors.Is(err, services.ErrGovernanceConflict):
		return newError(http.StatusConflict, "conflict", "Governance resource is in use or no longer pending")
	case errors.Is(err, services.ErrGovernanceValidation):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
