package services

import (
	"context"
	"errors"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	ErrGovernanceNotFound   = errors.New("governance resource not found")
	ErrGovernanceForbidden  = errors.New("governance operation forbidden")
	ErrGovernanceConflict   = errors.New("governance resource conflict")
	ErrGovernanceValidation = errors.New("governance validation failed")
)

type GovernanceValidationError struct{ Message string }

func (e *GovernanceValidationError) Error() string { return e.Message }
func (e *GovernanceValidationError) Unwrap() error { return ErrGovernanceValidation }

type ApprovalSetPatch struct {
	Name, Description *string
	WorkflowID        *int
	SetStatuses       *[]models.ApprovalSetStatus
}

type ApprovalDecisionResult struct {
	Decision *models.ApprovalDecision `json:"decision"`
	Request  *models.ApprovalRequest  `json:"request"`
}

type TransitionConditionTouch struct {
	ConditionSetID   int    `json:"condition_set_id"`
	ConditionSetName string `json:"condition_set_name"`
	ConditionCount   int    `json:"condition_count"`
}

type TransitionApprovalDriver struct {
	ApprovalSetID       int    `json:"approval_set_id"`
	ApprovalSetName     string `json:"approval_set_name"`
	ApprovalSetStatusID int    `json:"approval_set_status_id"`
	Role                string `json:"role"`
}

type TransitionGovernance struct {
	TransitionID    int                        `json:"transition_id"`
	FromStatusID    *int                       `json:"from_status_id"`
	ToStatusID      int                        `json:"to_status_id"`
	FromStatusName  string                     `json:"from_status_name"`
	ToStatusName    string                     `json:"to_status_name"`
	Conditions      []TransitionConditionTouch `json:"conditions"`
	ApprovalDrivers []TransitionApprovalDriver `json:"approval_drivers"`
}

// GovernanceApplicationService owns approval authorization and workflow-governance composition.
type GovernanceApplicationService struct {
	db           database.Database
	permissions  *PermissionService
	approvalSets *ApprovalSetService
	approvals    *ApprovalService
	items        *repository.ItemRepository
	transitions  *repository.TransitionRepository
}

func NewGovernanceApplicationService(db database.Database, permissions *PermissionService, approvalSets *ApprovalSetService, approvals *ApprovalService) *GovernanceApplicationService {
	return &GovernanceApplicationService{db: db, permissions: permissions, approvalSets: approvalSets, approvals: approvals, items: repository.NewItemRepository(db), transitions: repository.NewTransitionRepository(db)}
}

func (s *GovernanceApplicationService) ListApprovalSets(ctx context.Context, workflowID *int) ([]models.ApprovalSet, error) {
	return s.approvalSets.List(ctx, workflowID)
}
func (s *GovernanceApplicationService) GetApprovalSet(ctx context.Context, id int) (*models.ApprovalSet, error) {
	return s.approvalSets.GetByID(ctx, id)
}

func (s *GovernanceApplicationService) CreateApprovalSet(ctx context.Context, actor AuditActor, input models.ApprovalSet) (*models.ApprovalSet, error) {
	if err := s.requireSystemAdmin(actor.UserID); err != nil {
		return nil, err
	}
	sanitizeApprovalSet(&input)
	if input.Name == "" {
		return nil, governanceValidation("name is required")
	}
	item, err := s.approvalSets.Create(ctx, input)
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionApprovalSetCreate, logger.ResourceApprovalSet, &item.ID, item.Name, nil)
	}
	return item, governanceServiceError(err)
}

func (s *GovernanceApplicationService) PatchApprovalSet(ctx context.Context, actor AuditActor, id int, patch ApprovalSetPatch) (*models.ApprovalSet, error) {
	if err := s.requireSystemAdmin(actor.UserID); err != nil {
		return nil, err
	}
	current, err := s.approvalSets.GetByID(ctx, id)
	if err != nil {
		return nil, governanceServiceError(err)
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	if patch.WorkflowID != nil && *patch.WorkflowID != current.WorkflowID {
		return nil, governanceValidation("workflow_id cannot be changed")
	}
	if patch.SetStatuses != nil {
		current.SetStatuses = *patch.SetStatuses
	}
	sanitizeApprovalSet(current)
	if current.Name == "" {
		return nil, governanceValidation("name is required")
	}
	item, err := s.approvalSets.Update(ctx, id, *current)
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionApprovalSetUpdate, logger.ResourceApprovalSet, &id, item.Name, nil)
	}
	return item, governanceServiceError(err)
}

func (s *GovernanceApplicationService) DeleteApprovalSet(ctx context.Context, actor AuditActor, id int) error {
	if err := s.requireSystemAdmin(actor.UserID); err != nil {
		return err
	}
	name, err := s.approvalSets.Delete(ctx, id)
	if err != nil {
		return governanceServiceError(err)
	}
	emitServiceAudit(s.db, actor, logger.ActionApprovalSetDelete, logger.ResourceApprovalSet, &id, name, nil)
	return nil
}

func (s *GovernanceApplicationService) ListItemApprovals(ctx context.Context, userID, itemID, limit, offset int) ([]*models.ApprovalRequest, int, error) {
	allowed, err := s.canViewItemAsApprover(ctx, userID, itemID)
	if err != nil {
		return nil, 0, err
	}
	if !allowed {
		return nil, 0, ErrGovernanceNotFound
	}
	items, total, err := s.approvals.GetTimelineForItemPage(ctx, itemID, limit, offset)
	if items == nil && err == nil {
		items = []*models.ApprovalRequest{}
	}
	return items, total, err
}

func (s *GovernanceApplicationService) ListMyApprovals(ctx context.Context, userID int, status string, limit, offset int) ([]*models.ApprovalRequest, int, error) {
	items, total, err := s.approvals.GetForUserPage(ctx, userID, status, limit, offset)
	if items == nil && err == nil {
		items = []*models.ApprovalRequest{}
	}
	return items, total, err
}

func (s *GovernanceApplicationService) GetApproval(ctx context.Context, userID, requestID int) (*models.ApprovalRequest, error) {
	request, err := s.approvals.GetRequest(ctx, requestID)
	if err != nil {
		return nil, governanceServiceError(err)
	}
	if !s.canViewApproval(userID, request) {
		return nil, ErrGovernanceNotFound
	}
	return request, nil
}

func (s *GovernanceApplicationService) DecideApproval(ctx context.Context, actor AuditActor, requestID int, decision, comment string) (*ApprovalDecisionResult, error) {
	comment = sanitize.RichText.Sanitize(comment)
	switch decision {
	case models.ApprovalDecisionApprove, models.ApprovalDecisionReject, models.ApprovalDecisionComment:
	default:
		return nil, governanceValidation("decision must be approve, reject, or comment")
	}
	if _, err := s.approvals.GetItemIDForRequest(ctx, requestID); err != nil {
		return nil, governanceServiceError(err)
	}
	result, request, err := s.approvals.Decide(ctx, requestID, actor.UserID, decision, comment, DecideOptions{})
	if err != nil {
		return nil, governanceValidation(err.Error())
	}
	emitServiceAudit(s.db, actor, logger.ActionApprovalDecide, logger.ResourceApprovalRequest, &requestID, decision, nil)
	return &ApprovalDecisionResult{Decision: result, Request: request}, nil
}

func (s *GovernanceApplicationService) CancelApproval(ctx context.Context, actor AuditActor, requestID int, comment string) (*models.ApprovalRequest, error) {
	comment = sanitize.RichText.Sanitize(comment)
	request, err := s.approvals.GetRequest(ctx, requestID)
	if err != nil {
		return nil, governanceServiceError(err)
	}
	authorized := request.TriggeredByUserID == actor.UserID
	if !authorized {
		workspaceID, err := s.items.GetWorkspaceID(request.ItemID)
		if err != nil {
			return nil, ErrGovernanceNotFound
		}
		authorized, err = s.permissions.HasWorkspacePermission(actor.UserID, workspaceID, models.PermissionItemEdit)
		if err != nil {
			return nil, err
		}
	}
	if !authorized {
		return nil, ErrGovernanceNotFound
	}
	if request.Status != models.ApprovalRequestStatusPending {
		return nil, ErrGovernanceConflict
	}
	if err := s.approvals.Cancel(ctx, requestID, actor.UserID, comment, "manual"); err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionApprovalCancel, logger.ResourceApprovalRequest, &requestID, "", nil)
	return s.approvals.GetRequest(ctx, requestID)
}

func (s *GovernanceApplicationService) DelegateApproval(ctx context.Context, actor AuditActor, requestID, toUserID int, comment string) (*models.ApprovalRequest, error) {
	if toUserID <= 0 {
		return nil, governanceValidation("to_user_id is required")
	}
	itemID, err := s.approvals.GetItemIDForRequest(ctx, requestID)
	if err != nil {
		return nil, governanceServiceError(err)
	}
	allowed, err := s.canViewItemAsApprover(ctx, actor.UserID, itemID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrGovernanceNotFound
	}
	comment = sanitize.RichText.Sanitize(comment)
	if err := s.approvals.Delegate(ctx, requestID, actor.UserID, toUserID, comment); err != nil {
		return nil, governanceValidation(err.Error())
	}
	emitServiceAudit(s.db, actor, logger.ActionApprovalDelegate, logger.ResourceApprovalRequest, &requestID, "", nil)
	return s.approvals.GetRequest(ctx, requestID)
}

func (s *GovernanceApplicationService) RefreshApprovalApprovers(ctx context.Context, actor AuditActor, requestID, stepID int, comment string) (*models.ApprovalRequest, error) {
	if err := s.requireApprovalItemPermission(ctx, actor.UserID, requestID, models.PermissionItemEdit); err != nil {
		return nil, err
	}
	belongs, err := s.approvals.StepInstanceBelongsToRequest(ctx, stepID, requestID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, ErrGovernanceNotFound
	}
	comment = sanitize.RichText.Sanitize(comment)
	if err := s.approvals.RefreshApprovers(ctx, stepID, actor.UserID, comment); err != nil {
		return nil, governanceValidation(err.Error())
	}
	emitServiceAudit(s.db, actor, logger.ActionApprovalRefresh, logger.ResourceApprovalRequest, &requestID, "", nil)
	return s.approvals.GetRequest(ctx, requestID)
}

func (s *GovernanceApplicationService) EscalateApproval(ctx context.Context, actor AuditActor, requestID, stepID int) (*models.ApprovalRequest, error) {
	if err := s.requireApprovalItemPermission(ctx, actor.UserID, requestID, models.PermissionWorkspaceAdmin); err != nil {
		return nil, err
	}
	belongs, err := s.approvals.StepInstanceBelongsToRequest(ctx, stepID, requestID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, ErrGovernanceNotFound
	}
	if err := s.approvals.Escalate(ctx, stepID, actor.UserID, "manual"); err != nil {
		return nil, governanceValidation(err.Error())
	}
	emitServiceAudit(s.db, actor, logger.ActionApprovalEscalate, logger.ResourceApprovalRequest, &requestID, "manual", nil)
	return s.approvals.GetRequest(ctx, requestID)
}

func (s *GovernanceApplicationService) GetTransitionGovernance(ctx context.Context, transitionID int) (*TransitionGovernance, error) {
	transition, err := s.transitions.GetWithStatusNames(transitionID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrGovernanceNotFound
	}
	if err != nil {
		return nil, err
	}
	result := &TransitionGovernance{TransitionID: transition.ID, FromStatusID: transition.FromStatusID, ToStatusID: transition.ToStatusID, FromStatusName: transition.FromStatusName, ToStatusName: transition.ToStatusName, Conditions: []TransitionConditionTouch{}, ApprovalDrivers: []TransitionApprovalDriver{}}
	touches, err := s.transitions.ListConditionSetTouches(transitionID)
	if err != nil {
		return nil, err
	}
	for _, item := range touches {
		result.Conditions = append(result.Conditions, TransitionConditionTouch{ConditionSetID: item.ConditionSetID, ConditionSetName: item.ConditionSetName, ConditionCount: item.ConditionCount})
	}
	drivers, err := s.approvalSets.FindDriversForTransition(ctx, transitionID)
	if err != nil {
		return nil, err
	}
	for _, item := range drivers {
		result.ApprovalDrivers = append(result.ApprovalDrivers, TransitionApprovalDriver{ApprovalSetID: item.ApprovalSetID, ApprovalSetName: item.ApprovalSetName, ApprovalSetStatusID: item.ApprovalSetStatusID, Role: item.Role})
	}
	return result, nil
}

func (s *GovernanceApplicationService) requireSystemAdmin(userID int) error {
	allowed, err := s.permissions.IsSystemAdmin(userID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrGovernanceForbidden
	}
	return nil
}

func (s *GovernanceApplicationService) canViewItemAsApprover(ctx context.Context, userID, itemID int) (bool, error) {
	workspaceID, err := s.items.GetWorkspaceID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	if err != nil {
		return false, err
	}
	if allowed {
		return true, nil
	}
	return s.approvals.UserHasActivePoolMembershipOnItem(ctx, userID, itemID, nil)
}

func (s *GovernanceApplicationService) canViewApproval(userID int, request *models.ApprovalRequest) bool {
	workspaceID, err := s.items.GetWorkspaceID(request.ItemID)
	if err == nil {
		allowed, _ := s.permissions.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
		if allowed {
			return true
		}
	}
	for _, step := range request.StepInstances {
		for _, approver := range step.Approvers {
			if approver.UserID != nil && *approver.UserID == userID {
				return true
			}
		}
	}
	return request.TriggeredByUserID == userID
}

func (s *GovernanceApplicationService) requireApprovalItemPermission(ctx context.Context, userID, requestID int, permission string) error {
	itemID, err := s.approvals.GetItemIDForRequest(ctx, requestID)
	if err != nil {
		return governanceServiceError(err)
	}
	workspaceID, err := s.items.GetWorkspaceID(itemID)
	if err != nil {
		return ErrGovernanceNotFound
	}
	allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrGovernanceNotFound
	}
	return nil
}

func sanitizeApprovalSet(item *models.ApprovalSet) {
	sanitize.ApplyAll(sanitize.Pair{Target: &item.Name, Policy: sanitize.PlainTextField}, sanitize.Pair{Target: &item.Description, Policy: sanitize.RichText})
}
func governanceValidation(message string) error { return &GovernanceValidationError{Message: message} }
func governanceServiceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, ErrApprovalNotFound):
		return ErrGovernanceNotFound
	case errors.Is(err, ErrApprovalSetInUseByConfigSet), errors.Is(err, ErrApprovalSetValidation):
		return governanceValidation(err.Error())
	case errors.Is(err, ErrApprovalSetHasPendingRequests):
		return ErrGovernanceConflict
	default:
		return err
	}
}
