package services

import (
	"context"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ApprovalSetService owns template CRUD for approval sets — the asynchronous
// sibling of ConditionService for asynchronous gates. Mirrors ChannelService:
// the service holds the transaction; the repo owns the SQL.
type ApprovalSetService struct {
	db   database.Database
	repo *repository.ApprovalSetRepository
}

// NewApprovalSetService constructs an ApprovalSetService.
func NewApprovalSetService(db database.Database) *ApprovalSetService {
	return &ApprovalSetService{
		db:   db,
		repo: repository.NewApprovalSetRepository(db),
	}
}

// Sentinel errors so handlers can map to the right HTTP status without
// scraping error strings.
var (
	// ErrApprovalSetValidation indicates the input failed template-level
	// validation. Wrap with fmt.Errorf("...: %w", ErrApprovalSetValidation)
	// or use NewApprovalSetValidation for a typed message.
	ErrApprovalSetValidation = errors.New("approval set validation failed")

	// ErrApprovalSetInUseByConfigSet means a configuration_set or
	// configuration_set_item_types row references this approval set; delete
	// is refused.
	ErrApprovalSetInUseByConfigSet = errors.New("approval set is in use by one or more configuration sets")

	// ErrApprovalSetHasPendingRequests means at least one pending
	// approval_request points at a status of this set; delete is refused.
	ErrApprovalSetHasPendingRequests = errors.New("approval set has pending approval requests")
)

// ApprovalSetValidationError carries the human-readable detail behind a
// validation failure. Errors.Is(err, ErrApprovalSetValidation) is true for
// any wrapped instance.
type ApprovalSetValidationError struct{ Msg string }

func (e *ApprovalSetValidationError) Error() string { return e.Msg }
func (e *ApprovalSetValidationError) Unwrap() error { return ErrApprovalSetValidation }
func newApprovalSetValidation(format string, args ...interface{}) error {
	return &ApprovalSetValidationError{Msg: fmt.Sprintf(format, args...)}
}

// ============================================================================
// Reads
// ============================================================================

// List returns approval sets, optionally filtered by workflow ID. Each
// returned set is populated with WorkflowName + GatedStatuses; nested
// SetStatuses/Steps are NOT loaded (use GetByID for the full graph).
func (s *ApprovalSetService) List(ctx context.Context, workflowID *int) ([]models.ApprovalSet, error) {
	sets, err := s.repo.FindAll(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if err := s.attachGatedStatuses(ctx, sets); err != nil {
		return nil, err
	}
	if sets == nil {
		sets = []models.ApprovalSet{}
	}
	return sets, nil
}

// GetByID returns a single approval set with its full graph (set-statuses,
// each with steps). Returns repository.ErrNotFound if no set matches.
func (s *ApprovalSetService) GetByID(ctx context.Context, id int) (*models.ApprovalSet, error) {
	return s.repo.FindByID(ctx, id)
}

// FindDriversForTransition returns approval sets that drive the given
// transition (their approve_transition_id or deny_transition_id). Powers the
// transition-governance override-warning UI.
func (s *ApprovalSetService) FindDriversForTransition(ctx context.Context, transitionID int) ([]repository.ApprovalSetTransitionDriver, error) {
	return s.repo.FindDriversForTransition(ctx, transitionID)
}

// ============================================================================
// Mutations
// ============================================================================

// Create creates a new approval set with nested set-statuses and steps.
// workflow_id must reference an existing workflow. Returns the persisted set
// with its full graph populated.
func (s *ApprovalSetService) Create(ctx context.Context, input models.ApprovalSet) (*models.ApprovalSet, error) {
	if input.WorkflowID == 0 {
		return nil, newApprovalSetValidation("workflow_id is required")
	}
	exists, err := s.repo.WorkflowExists(ctx, input.WorkflowID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, newApprovalSetValidation("Workflow not found")
	}
	if err := s.validateSetStatuses(ctx, input.WorkflowID, input.SetStatuses); err != nil {
		return nil, err
	}

	createdID, err := database.WithTxResult(s.db, func(tx database.Tx) (int, error) {
		setCopy := input
		id, err := s.repo.CreateSet(ctx, tx, &setCopy)
		if err != nil {
			return 0, err
		}
		if err := s.persistSetStatuses(ctx, tx, id, input.SetStatuses); err != nil {
			return 0, err
		}
		return id, nil
	})
	if err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, createdID)
}

// Update replaces an approval set's name/description and its nested
// set-statuses + steps. workflow_id is immutable. Returns the updated set
// with its full graph populated.
func (s *ApprovalSetService) Update(ctx context.Context, id int, input models.ApprovalSet) (*models.ApprovalSet, error) {
	existingWorkflowID, err := s.repo.GetWorkflowIDForSet(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.WorkflowID != 0 && input.WorkflowID != existingWorkflowID {
		return nil, newApprovalSetValidation("Cannot change workflow_id of an existing approval set")
	}
	if err := s.validateSetStatuses(ctx, existingWorkflowID, input.SetStatuses); err != nil {
		return nil, err
	}

	if err := database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.UpdateSet(ctx, tx, id, input.Name, input.Description); err != nil {
			return err
		}
		// Soft-archive: drop unreferenced rows, flip the rest to is_active=0,
		// then insert fresh active rows. In-flight requests' FK to the
		// archived snapshot is preserved.
		if err := s.repo.DeleteUnreferencedStatuses(ctx, tx, id); err != nil {
			return err
		}
		if err := s.repo.DeactivateActiveStatuses(ctx, tx, id); err != nil {
			return err
		}
		return s.persistSetStatuses(ctx, tx, id, input.SetStatuses)
	}); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// Delete deletes an approval set. Refuses (returns sentinel error) if the
// set is referenced by any configuration_set or has any pending requests.
// Returns the deleted set's name so callers can record it in audit logs.
func (s *ApprovalSetService) Delete(ctx context.Context, id int) (string, error) {
	name, err := s.repo.GetSetName(ctx, id)
	if err != nil {
		return "", err
	}

	inUse, err := s.repo.CountReferencingConfigSets(ctx, id)
	if err != nil {
		return "", err
	}
	if inUse > 0 {
		return "", ErrApprovalSetInUseByConfigSet
	}

	pending, err := s.repo.CountPendingRequestsForSet(ctx, id)
	if err != nil {
		return "", err
	}
	if pending > 0 {
		return "", fmt.Errorf("%w: %d pending request(s)", ErrApprovalSetHasPendingRequests, pending)
	}

	if err := database.WithTx(s.db, func(tx database.Tx) error {
		return s.repo.DeleteSet(ctx, tx, id)
	}); err != nil {
		return "", err
	}
	return name, nil
}

// ============================================================================
// Helpers
// ============================================================================

func (s *ApprovalSetService) attachGatedStatuses(ctx context.Context, sets []models.ApprovalSet) error {
	if len(sets) == 0 {
		return nil
	}
	ids := make([]int, len(sets))
	for i := range sets {
		ids[i] = sets[i].ID
	}
	gated, err := s.repo.FindGatedStatusesForSets(ctx, ids)
	if err != nil {
		return err
	}
	for i := range sets {
		if rows, ok := gated[sets[i].ID]; ok {
			sets[i].GatedStatuses = rows
		}
	}
	return nil
}

func (s *ApprovalSetService) persistSetStatuses(ctx context.Context, tx database.Tx, approvalSetID int, setStatuses []models.ApprovalSetStatus) error {
	for _, ass := range setStatuses {
		assCopy := ass
		assCopy.ApprovalSetID = approvalSetID
		assID, err := s.repo.CreateStatus(ctx, tx, &assCopy)
		if err != nil {
			return err
		}
		for _, step := range ass.Steps {
			if err := s.repo.CreateStep(ctx, tx, assID, step); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateSetStatuses runs the full template-validation pass: per-status
// invariants (transitions originate at the configured status, both transitions
// differ, etc.) plus per-step invariants delegated to validateApprovalStep.
func (s *ApprovalSetService) validateSetStatuses(ctx context.Context, workflowID int, setStatuses []models.ApprovalSetStatus) error {
	seenStatus := make(map[int]bool, len(setStatuses))
	for _, ass := range setStatuses {
		if ass.StatusID == 0 {
			return newApprovalSetValidation("status_id is required for each approval_set_status")
		}
		if seenStatus[ass.StatusID] {
			return newApprovalSetValidation("duplicate status_id %d in approval set", ass.StatusID)
		}
		seenStatus[ass.StatusID] = true

		if ass.ApproveTransitionID == 0 || ass.DenyTransitionID == 0 {
			return newApprovalSetValidation("approve_transition_id and deny_transition_id are required")
		}
		if ass.ApproveTransitionID == ass.DenyTransitionID {
			return newApprovalSetValidation("approve and deny transitions must differ")
		}
		if ass.StepMode != models.ApprovalStepModeSequential && ass.StepMode != models.ApprovalStepModeParallel {
			return newApprovalSetValidation("step_mode must be 'sequential' or 'parallel'")
		}

		if err := s.checkTransitionFromStatus(ctx, ass.ApproveTransitionID, workflowID, ass.StatusID, "approve_transition_id"); err != nil {
			return err
		}
		if err := s.checkTransitionFromStatus(ctx, ass.DenyTransitionID, workflowID, ass.StatusID, "deny_transition_id"); err != nil {
			return err
		}

		if len(ass.Steps) == 0 {
			return newApprovalSetValidation("an approval_set_status must have at least one step")
		}
		for _, step := range ass.Steps {
			if err := validateApprovalStep(step); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ApprovalSetService) checkTransitionFromStatus(ctx context.Context, transitionID, workflowID, fromStatusID int, fieldName string) error {
	ok, err := s.repo.TransitionExistsFromStatus(ctx, transitionID, workflowID, fromStatusID)
	if err != nil {
		return err
	}
	if !ok {
		return newApprovalSetValidation("%s does not exist on this workflow as a transition out of the configured status", fieldName)
	}
	return nil
}

// validateApprovalStep enforces step-level template invariants: quorum mode
// validity, rejection policy, on-leave strategy, escalation action vocabulary,
// and approver-source completeness.
func validateApprovalStep(step models.ApprovalStep) error {
	if step.Name == "" {
		return newApprovalSetValidation("each step must have a name")
	}
	switch step.QuorumMode {
	case models.ApprovalQuorumModeAny, models.ApprovalQuorumModeAll:
		// ok
	case models.ApprovalQuorumModeCount:
		if step.QuorumCount == nil || *step.QuorumCount < 1 {
			return newApprovalSetValidation("quorum_mode 'count' requires quorum_count >= 1")
		}
	case models.ApprovalQuorumModePercent:
		if step.QuorumPercent == nil || *step.QuorumPercent < 1 || *step.QuorumPercent > 100 {
			return newApprovalSetValidation("quorum_mode 'percent' requires quorum_percent in [1,100]")
		}
	default:
		return newApprovalSetValidation("quorum_mode must be one of any|all|count|percent")
	}

	switch step.RejectionPolicy {
	case "", models.ApprovalRejectionPolicyAnyFails, models.ApprovalRejectionPolicyQuorumRequired:
		// ok
	default:
		return newApprovalSetValidation("rejection_policy must be 'any_rejection_fails' or 'requires_quorum_to_fail'")
	}

	switch step.OnLeaveStrategy {
	case "", models.ApprovalOnLeaveUseSubstitute, models.ApprovalOnLeaveSkip, models.ApprovalOnLeaveKeep:
		// ok
	default:
		return newApprovalSetValidation("on_leave_strategy must be 'use_substitute', 'skip', or 'keep'")
	}

	if step.EscalationAction != "" {
		switch step.EscalationAction {
		case models.ApprovalEscalationActionReassign, models.ApprovalEscalationActionSkipStep, models.ApprovalEscalationActionAutoReject:
		default:
			return newApprovalSetValidation("escalation_action must be 'reassign', 'skip_step', or 'auto_reject'")
		}
	}

	switch step.ApproverSource {
	case models.ApprovalSourceCreator, models.ApprovalSourceAssignee, models.ApprovalSourceCurrentUser:
		// no extra fields required
	case models.ApprovalSourceUser:
		if step.ApproverUserID == nil || *step.ApproverUserID == 0 {
			return newApprovalSetValidation("approver_source 'user' requires approver_user_id")
		}
	case models.ApprovalSourceRegularField:
		if _, ok := models.AllowedRegularApproverFields[step.ApproverFieldIdentifier]; !ok {
			return newApprovalSetValidation("approver_field_identifier %q is not in the regular-field whitelist", step.ApproverFieldIdentifier)
		}
	case models.ApprovalSourceCustomField:
		if step.ApproverFieldID == nil || *step.ApproverFieldID == 0 {
			return newApprovalSetValidation("approver_source 'custom_field' requires approver_field_id")
		}
	case models.ApprovalSourceRole:
		if step.ApproverRoleID == nil || *step.ApproverRoleID == 0 {
			return newApprovalSetValidation("approver_source 'role' requires approver_role_id")
		}
	case models.ApprovalSourceGroup:
		if step.ApproverGroupID == nil || *step.ApproverGroupID == 0 {
			return newApprovalSetValidation("approver_source 'group' requires approver_group_id")
		}
	default:
		return newApprovalSetValidation("approver_source must be one of creator|assignee|current_user|user|regular_field|custom_field|role|group")
	}

	if step.EscalationTargetSource != "" {
		probe := step
		probe.ApproverSource = step.EscalationTargetSource
		probe.ApproverFieldIdentifier = step.EscalationTargetFieldIdentifier
		probe.ApproverFieldID = step.EscalationTargetFieldID
		probe.ApproverRoleID = step.EscalationTargetRoleID
		probe.ApproverGroupID = step.EscalationTargetGroupID
		probe.ApproverUserID = step.EscalationTargetUserID
		switch probe.ApproverSource {
		case models.ApprovalSourceCreator, models.ApprovalSourceAssignee, models.ApprovalSourceCurrentUser:
		case models.ApprovalSourceUser:
			if probe.ApproverUserID == nil || *probe.ApproverUserID == 0 {
				return newApprovalSetValidation("escalation_target_source 'user' requires escalation_target_user_id")
			}
		case models.ApprovalSourceRegularField:
			if _, ok := models.AllowedRegularApproverFields[probe.ApproverFieldIdentifier]; !ok {
				return newApprovalSetValidation("escalation_target_field_identifier is not in the regular-field whitelist")
			}
		case models.ApprovalSourceCustomField:
			if probe.ApproverFieldID == nil || *probe.ApproverFieldID == 0 {
				return newApprovalSetValidation("escalation_target_source 'custom_field' requires escalation_target_field_id")
			}
		case models.ApprovalSourceRole:
			if probe.ApproverRoleID == nil || *probe.ApproverRoleID == 0 {
				return newApprovalSetValidation("escalation_target_source 'role' requires escalation_target_role_id")
			}
		case models.ApprovalSourceGroup:
			if probe.ApproverGroupID == nil || *probe.ApproverGroupID == 0 {
				return newApprovalSetValidation("escalation_target_source 'group' requires escalation_target_group_id")
			}
		default:
			return newApprovalSetValidation("escalation_target_source must be a valid source vocabulary value")
		}
	}
	return nil
}
