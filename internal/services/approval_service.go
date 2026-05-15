package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ApprovalService is the asynchronous sibling of ConditionService. Where
// conditions/validators evaluate at transition time, approvals open a stateful
// request that one or more approvers decide over time.
//
// The lifecycle:
//
//  1. An item enters a status that has an approval_set_status row.
//  2. PerformTransition (after CommitTransition) calls RequestApproval, which
//     creates an approval_requests row, materializes step instances per the
//     approval_steps template, and snapshots the approver pool for the active
//     step (with on-leave handling via LeaveRepository).
//  3. Each approver POSTs /approvals/{id}/decide. ApprovalService.Decide records
//     a decision, advances/completes the step based on the configured quorum,
//     and on final outcome calls WorkflowService.CommitTransition with the
//     configured approve/deny transition's to_status_id.
//  4. If the user transitions out of the approval-bound status via a non-gated
//     transition, the pending request is canceled with reason "left_status".
//
// The configured approve/deny transitions cannot be invoked directly by users —
// PerformTransition rejects those attempts with code "approval_must_decide".
// ErrApprovalNotFound is returned when an approval request or related approval resource is not found.
var ErrApprovalNotFound = sql.ErrNoRows

type ApprovalService struct {
	db              database.Database
	permService     *PermissionService
	leaveRepo       *repository.LeaveRepository
	workflowService *WorkflowService

	runtimeRepo  *repository.ApprovalRepository
	templateRepo *repository.ApprovalSetRepository

	// eventCoordinator is set via SetEventCoordinator at startup; nil in tests
	// that exercise gating only and don't care about notifications.
	eventCoordinator *EventCoordinator
}

// NewApprovalService constructs an ApprovalService. EventCoordinator is wired
// via SetEventCoordinator after construction (mirrors CommentService pattern).
func NewApprovalService(db database.Database, permService *PermissionService, leaveRepo *repository.LeaveRepository, workflowService *WorkflowService) *ApprovalService {
	return &ApprovalService{
		db:              db,
		permService:     permService,
		leaveRepo:       leaveRepo,
		workflowService: workflowService,
		runtimeRepo:     repository.NewApprovalRepository(db),
		templateRepo:    repository.NewApprovalSetRepository(db),
	}
}

// SetEventCoordinator wires the EventCoordinator for emitting approval events.
func (s *ApprovalService) SetEventCoordinator(ec *EventCoordinator) {
	s.eventCoordinator = ec
}

// ----------------------------------------------------------------------------
// Resolution: which approval-set-status applies to an item?
// ----------------------------------------------------------------------------

// GetApprovalSetIDForItem mirrors ConditionService.GetConditionSetIDForItem:
// item-type override → workspace config-set default → global default.
// Returns (nil, nil) for personal workspaces or when no approval set is configured.
func (s *ApprovalService) GetApprovalSetIDForItem(workspaceID int, itemTypeID *int) (*int, error) {
	ctx := context.Background()
	isPersonal, err := s.templateRepo.IsWorkspacePersonal(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if isPersonal {
		return nil, nil
	}
	return s.templateRepo.ResolveForWorkspace(ctx, workspaceID, itemTypeID)
}

// GetApprovalSetStatusForItem returns the approval_set_status (template row)
// that applies to an item entering statusID, or nil if no approval gates the entry.
func (s *ApprovalService) GetApprovalSetStatusForItem(workspaceID int, itemTypeID *int, statusID int) (*models.ApprovalSetStatus, error) {
	approvalSetID, err := s.GetApprovalSetIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}
	if approvalSetID == nil {
		return nil, nil
	}
	return s.templateRepo.FindActiveStatusBySetAndStatus(context.Background(), *approvalSetID, statusID)
}

// ----------------------------------------------------------------------------
// RequestApproval: open a new pending approval request.
// ----------------------------------------------------------------------------

// RequestApproval opens a new approval request for the item. The caller (typically
// PerformTransition's post-commit hook) is responsible for ensuring no pending
// request already exists; the unique partial index uq_approval_requests_one_open_per_item
// enforces this at the DB layer as defense in depth.
func (s *ApprovalService) RequestApproval(ctx context.Context, itemID, statusID, fromStatusID, triggeredByUserID int) (*models.ApprovalRequest, error) {
	item, err := repository.NewItemRepository(s.db).FindByID(itemID)
	if err != nil {
		return nil, fmt.Errorf("load item: %w", err)
	}
	ass, err := s.GetApprovalSetStatusForItem(item.WorkspaceID, item.ItemTypeID, statusID)
	if err != nil {
		return nil, fmt.Errorf("resolve approval set: %w", err)
	}
	if ass == nil {
		return nil, nil
	}

	steps, err := s.templateRepo.FindStepsByStatusID(ctx, ass.ID)
	if err != nil {
		return nil, fmt.Errorf("load steps: %w", err)
	}
	if len(steps) == 0 {
		return nil, nil // misconfigured set; treat as no-op
	}

	requestID, err := database.WithTxResult(s.db, func(tx database.Tx) (int, error) {
		var fromStatusPtr *int
		if fromStatusID > 0 {
			fromStatusPtr = &fromStatusID
		}
		reqID, err := s.runtimeRepo.CreateRequest(ctx, tx, itemID, ass.ID, statusID, fromStatusPtr, triggeredByUserID)
		if err != nil {
			return 0, fmt.Errorf("insert approval_request: %w", err)
		}

		now := time.Now()
		stepInstanceIDs := make([]int, len(steps))
		for i, step := range steps {
			startedAt := sql.NullTime{}
			if ass.StepMode == models.ApprovalStepModeParallel || i == 0 {
				startedAt = sql.NullTime{Time: now, Valid: true}
			}
			var dueAt sql.NullTime
			if step.EscalationAfterHours != nil && startedAt.Valid {
				dueAt = sql.NullTime{Time: now.Add(time.Duration(*step.EscalationAfterHours) * time.Hour), Valid: true}
			}
			sid, err := s.runtimeRepo.CreateStepInstance(ctx, tx, reqID, step.ID, step.DisplayOrder, startedAt, dueAt)
			if err != nil {
				return 0, fmt.Errorf("insert step instance: %w", err)
			}
			stepInstanceIDs[i] = sid
		}

		for i, step := range steps {
			startedNow := ass.StepMode == models.ApprovalStepModeParallel || i == 0
			if !startedNow {
				continue
			}
			if err := s.resolveAndSnapshotApprovers(ctx, tx, stepInstanceIDs[i], step, item, triggeredByUserID); err != nil {
				return 0, fmt.Errorf("resolve approvers (step %d): %w", step.DisplayOrder, err)
			}
		}

		if _, err := s.runtimeRepo.WriteDecision(ctx, tx, reqID, nil, nil, nil, models.ApprovalDecisionRequested, "", nil, map[string]any{
			"triggered_by_user_id": triggeredByUserID,
			"approval_set_status":  ass.ID,
			"step_mode":            ass.StepMode,
			"step_count":           len(steps),
		}); err != nil {
			return 0, err
		}
		return reqID, nil
	})
	if err != nil {
		return nil, err
	}

	req, err := s.GetRequest(requestID)
	if err != nil {
		return nil, err
	}

	// Notifications: broadcast that the request opened, and (for sequential mode)
	// page the active step's approver pool. For parallel mode, page every step
	// instance's pool.
	if s.eventCoordinator != nil {
		fullItem, _ := repository.NewItemRepository(s.db).FindByIDWithDetails(itemID)
		if fullItem != nil {
			s.eventCoordinator.EmitApprovalRequested(req, fullItem, triggeredByUserID)
			for i := range req.StepInstances {
				si := &req.StepInstances[i]
				if si.Status != models.ApprovalStepStatusPending || si.StartedAt == nil {
					continue
				}
				userIDs := approverUserIDs(si.Approvers)
				customerIDs := approverPortalCustomerIDs(si.Approvers)
				s.eventCoordinator.EmitApprovalStepStarted(req, si, userIDs, customerIDs, fullItem, triggeredByUserID)
			}
		}
	}
	return req, nil
}

// approverUserIDs extracts the internal user IDs from the active approvers of a
// step. Portal-customer approvers are skipped here — call approverPortalCustomerIDs
// for those.
func approverUserIDs(approvers []models.ApprovalStepApprover) []int {
	out := make([]int, 0, len(approvers))
	for _, a := range approvers {
		if a.IsActive && a.UserID != nil {
			out = append(out, *a.UserID)
		}
	}
	return out
}

// approverPortalCustomerIDs returns the active portal-customer ids of a step.
func approverPortalCustomerIDs(approvers []models.ApprovalStepApprover) []int {
	out := make([]int, 0, len(approvers))
	for _, a := range approvers {
		if a.IsActive && a.PortalCustomerID != nil {
			out = append(out, *a.PortalCustomerID)
		}
	}
	return out
}

// ----------------------------------------------------------------------------
// Decide: record an approver's decision and advance the state machine.
// ----------------------------------------------------------------------------

// DecideOptions are optional inputs to Decide.
type DecideOptions struct {
	// ItemRepo can be supplied to avoid re-instantiating it; nil is fine.
	ItemRepo *repository.ItemRepository
}

// Decide records a decision against the active step of an approval request. On
// final outcome (approve or reject), it commits the configured transition via
// WorkflowService.CommitTransition inside the same tx.
//
// decision must be one of: ApprovalDecisionApprove, ApprovalDecisionReject,
// ApprovalDecisionComment. (Delegate / cancel / refresh-approvers are separate
// methods so each can carry its own validation.)
func (s *ApprovalService) Decide(ctx context.Context, requestID, actorUserID int, decision, comment string, opts DecideOptions) (*models.ApprovalDecision, *models.ApprovalRequest, error) {
	return s.decideAs(ctx, requestID, actorFromUser(actorUserID), decision, comment, opts)
}

// DecideAsCustomer is the portal-side entry point: a portal customer decides
// on an approval where they're in the active pool. Same state machine as the
// internal Decide, but actor attribution flows through actor_portal_customer_id
// and the downstream CommitTransition uses the request's triggered_by_user_id
// for item_history attribution (since item_history requires a user actor).
func (s *ApprovalService) DecideAsCustomer(ctx context.Context, requestID, actorPortalCustomerID int, decision, comment string, opts DecideOptions) (*models.ApprovalDecision, *models.ApprovalRequest, error) {
	return s.decideAs(ctx, requestID, actorFromCustomer(actorPortalCustomerID), decision, comment, opts)
}

func (s *ApprovalService) decideAs(ctx context.Context, requestID int, actor approvalActor, decision, comment string, opts DecideOptions) (*models.ApprovalDecision, *models.ApprovalRequest, error) {
	switch decision {
	case models.ApprovalDecisionApprove, models.ApprovalDecisionReject, models.ApprovalDecisionComment:
		// ok
	default:
		return nil, nil, fmt.Errorf("invalid decision %q", decision)
	}

	itemRepo := opts.ItemRepo
	if itemRepo == nil {
		itemRepo = repository.NewItemRepository(s.db)
	}

	type decideOutcome struct {
		decision                   *models.ApprovalDecision
		priorRequestStatus         string
		newlyStartedStepInstanceID *int
		effectiveActorUserID       int
	}

	out, err := database.WithTxResult(s.db, func(tx database.Tx) (decideOutcome, error) {
		var zero decideOutcome
		req, err := s.runtimeRepo.LoadRequestByIDInTx(ctx, tx, requestID)
		if err != nil {
			return zero, fmt.Errorf("load request: %w", err)
		}
		if req.Status != models.ApprovalRequestStatusPending {
			return zero, fmt.Errorf("approval request %d is not pending (status=%s)", requestID, req.Status)
		}

		stepInstance, err := s.findActiveStepForActor(ctx, tx, requestID, actor)
		if err != nil {
			return zero, err
		}
		if stepInstance == nil {
			return zero, fmt.Errorf("actor is not an active approver of request %d", requestID)
		}

		step, err := s.templateRepo.FindStepByIDInTx(ctx, tx, stepInstance.ApprovalStepID)
		if err != nil {
			return zero, err
		}

		// Self-approval guard only applies to internal users — triggered_by_user_id
		// is users-only, so a customer-actor can never collide with it.
		if actor.UserID != nil && !step.AllowSelfApproval && *actor.UserID == req.TriggeredByUserID && decision != models.ApprovalDecisionComment {
			return zero, fmt.Errorf("self-approval is not allowed for this step")
		}

		priorRequestStatus := req.Status

		var effectiveActorUserID int
		if actor.UserID != nil {
			effectiveActorUserID = *actor.UserID
		} else {
			effectiveActorUserID = req.TriggeredByUserID
		}

		if decision == models.ApprovalDecisionComment {
			commentDecision, err := s.runtimeRepo.WriteDecision(ctx, tx, requestID, &stepInstance.ID, actor.UserID, actor.PortalCustomerID, decision, comment, nil, nil)
			if err != nil {
				return zero, err
			}
			return decideOutcome{
				decision:             commentDecision,
				priorRequestStatus:   priorRequestStatus,
				effectiveActorUserID: effectiveActorUserID,
			}, nil
		}

		decisionRow, err := s.runtimeRepo.WriteDecision(ctx, tx, requestID, &stepInstance.ID, actor.UserID, actor.PortalCustomerID, decision, comment, nil, nil)
		if err != nil {
			return zero, err
		}

		stepNewStatus, err := s.evaluateStepStatus(ctx, tx, stepInstance.ID, step)
		if err != nil {
			return zero, err
		}
		if stepNewStatus != stepInstance.Status {
			if err := s.runtimeRepo.UpdateStepInstanceStatusComplete(ctx, tx, stepInstance.ID, stepNewStatus); err != nil {
				return zero, err
			}
		}

		var newlyStartedStepInstanceID *int
		if stepNewStatus == models.ApprovalStepStatusApproved || stepNewStatus == models.ApprovalStepStatusRejected {
			nextID, err := s.advanceRequestAfterStep(ctx, tx, req, stepInstance, stepNewStatus, effectiveActorUserID, itemRepo)
			if err != nil {
				return zero, err
			}
			newlyStartedStepInstanceID = nextID
		}

		return decideOutcome{
			decision:                   decisionRow,
			priorRequestStatus:         priorRequestStatus,
			newlyStartedStepInstanceID: newlyStartedStepInstanceID,
			effectiveActorUserID:       effectiveActorUserID,
		}, nil
	})
	if err != nil {
		return nil, nil, err
	}

	full, err := s.GetRequest(requestID)
	if err != nil {
		return nil, nil, err
	}
	s.emitDecisionEvents(out.decision, full, out.priorRequestStatus, out.newlyStartedStepInstanceID, out.effectiveActorUserID)
	return out.decision, full, nil
}

// emitDecisionEvents fires post-commit notifications for a Decide call:
// always EmitApprovalDecided; if a new step just started (sequential mode
// advance only — parallel mode opens all steps at request time), emit
// EmitApprovalStepStarted for that step; if the request finalized, emit
// EmitApprovalCompleted.
func (s *ApprovalService) emitDecisionEvents(decision *models.ApprovalDecision, req *models.ApprovalRequest, priorRequestStatus string, newlyStartedStepInstanceID *int, actorUserID int) {
	if s.eventCoordinator == nil || req == nil {
		return
	}
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(req.ItemID)
	if err != nil || item == nil {
		return
	}
	s.eventCoordinator.EmitApprovalDecided(req, decision, item)

	if newlyStartedStepInstanceID != nil {
		for i := range req.StepInstances {
			si := &req.StepInstances[i]
			if si.ID != *newlyStartedStepInstanceID {
				continue
			}
			s.eventCoordinator.EmitApprovalStepStarted(req, si, approverUserIDs(si.Approvers), approverPortalCustomerIDs(si.Approvers), item, actorUserID)
			break
		}
	}

	if req.Status != priorRequestStatus &&
		(req.Status == models.ApprovalRequestStatusApproved || req.Status == models.ApprovalRequestStatusRejected) {
		s.eventCoordinator.EmitApprovalCompleted(req, item, actorUserID)
	}
}

// advanceRequestAfterStep drives request-level state when a step terminates.
// Returns the ID of a newly-started step instance (sequential mode advance),
// or nil if no new step started.
//
// Sequential mode: on step approve, start the next step (or finalize if last);
// on step reject, finalize the request as rejected.
// Parallel mode: on step approve, finalize as approved iff every step is
// approved; on step reject, finalize as rejected and skip any still-pending
// peer steps in the same tx.
func (s *ApprovalService) advanceRequestAfterStep(ctx context.Context, tx database.Tx, req *models.ApprovalRequest, stepInstance *models.ApprovalStepInstance, stepStatus string, actorUserID int, itemRepo *repository.ItemRepository) (*int, error) {
	ass, err := s.templateRepo.FindStatusByIDInTx(ctx, tx, req.ApprovalSetStatusID)
	if err != nil {
		return nil, err
	}

	if ass.StepMode == models.ApprovalStepModeParallel {
		return nil, s.evaluateParallelRequestState(ctx, tx, req, ass, stepInstance, stepStatus, actorUserID, itemRepo)
	}

	if stepStatus == models.ApprovalStepStatusRejected {
		return nil, s.finalizeRequest(ctx, tx, req, ass, models.ApprovalRequestStatusRejected, ass.DenyTransitionID, actorUserID, itemRepo)
	}

	nextStepInstanceID, nextStepID, found, err := s.runtimeRepo.FindNextPendingStep(ctx, tx, req.ID, stepInstance.DisplayOrder)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, s.finalizeRequest(ctx, tx, req, ass, models.ApprovalRequestStatusApproved, ass.ApproveTransitionID, actorUserID, itemRepo)
	}

	now := time.Now()
	nextStep, err := s.templateRepo.FindStepByIDInTx(ctx, tx, nextStepID)
	if err != nil {
		return nil, err
	}
	var dueAt sql.NullTime
	if nextStep.EscalationAfterHours != nil {
		dueAt = sql.NullTime{Time: now.Add(time.Duration(*nextStep.EscalationAfterHours) * time.Hour), Valid: true}
	}
	if err := s.runtimeRepo.StartStepInstance(ctx, tx, nextStepInstanceID, now, dueAt); err != nil {
		return nil, err
	}

	item, err := itemRepo.FindByID(req.ItemID)
	if err != nil {
		return nil, fmt.Errorf("reload item: %w", err)
	}
	if err := s.resolveAndSnapshotApprovers(ctx, tx, nextStepInstanceID, *nextStep, item, req.TriggeredByUserID); err != nil {
		return nil, fmt.Errorf("snapshot next-step approvers: %w", err)
	}
	return &nextStepInstanceID, nil
}

// finalizeRequest commits the configured approve/deny transition and marks
// the request as approved/rejected.
func (s *ApprovalService) finalizeRequest(ctx context.Context, tx database.Tx, req *models.ApprovalRequest, ass *models.ApprovalSetStatus, finalStatus string, transitionID, actorUserID int, itemRepo *repository.ItemRepository) error {
	_, toStatusID, err := s.runtimeRepo.GetTransitionEndpoints(ctx, tx, transitionID)
	if err != nil {
		return fmt.Errorf("load transition: %w", err)
	}

	if err := s.runtimeRepo.UpdateRequestStatusComplete(ctx, tx, req.ID, finalStatus); err != nil {
		return fmt.Errorf("finalize request: %w", err)
	}

	// item_history.user_id is NOT NULL with an FK to users — when the actor is
	// the system (e.g. sweeper-driven auto_reject), fall back to the requestor
	// so the history attribution stays valid. The audit decision row still
	// carries actor=NULL for the true "system" provenance.
	historyActor := actorUserID
	if historyActor == 0 {
		historyActor = req.TriggeredByUserID
	}
	if err := s.workflowService.CommitTransition(tx, itemRepo, req.ItemID, ass.StatusID, toStatusID, historyActor); err != nil {
		return fmt.Errorf("commit driven transition: %w", err)
	}

	if _, err := s.runtimeRepo.WriteDecision(ctx, tx, req.ID, nil, nil, nil, models.ApprovalDecisionCompleted, "", nil, map[string]any{
		"final_status":  finalStatus,
		"transition_id": transitionID,
		"to_status_id":  toStatusID,
	}); err != nil {
		return err
	}
	return nil
}

// evaluateParallelRequestState handles request-level state transitions for
// parallel-mode approvals. Called once per terminating step instance.
//
//   - On any step rejection: finalize the request as rejected, skip every
//     still-pending peer step in the same tx (status='skipped').
//   - On step approval: finalize as approved iff every step instance has
//     reached status='approved'. Otherwise no-op (the other steps continue).
//
// stepInstance is the just-decided step (already updated to stepStatus).
func (s *ApprovalService) evaluateParallelRequestState(ctx context.Context, tx database.Tx, req *models.ApprovalRequest, ass *models.ApprovalSetStatus, stepInstance *models.ApprovalStepInstance, stepStatus string, actorUserID int, itemRepo *repository.ItemRepository) error {
	if stepStatus == models.ApprovalStepStatusRejected {
		if err := s.runtimeRepo.SkipPendingPeerSteps(ctx, tx, req.ID, stepInstance.ID); err != nil {
			return err
		}
		return s.finalizeRequest(ctx, tx, req, ass, models.ApprovalRequestStatusRejected, ass.DenyTransitionID, actorUserID, itemRepo)
	}

	pending, total, err := s.runtimeRepo.CountStepStates(ctx, tx, req.ID)
	if err != nil {
		return err
	}
	if total == 0 {
		return errors.New("parallel approval has no step instances")
	}
	if pending == 0 {
		return s.finalizeRequest(ctx, tx, req, ass, models.ApprovalRequestStatusApproved, ass.ApproveTransitionID, actorUserID, itemRepo)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Cancel: caller-initiated termination of a pending request.
// ----------------------------------------------------------------------------

// Cancel marks a pending request canceled. reason is a short string surfaced
// in the audit log: "left_status", "manual", "superseded", etc.
//
// Cancel also reverts the item to the status it was in before the inbound
// approval-triggering transition (snapshotted in approval_requests.from_status_id),
// so the item is never left stuck in the gated status with no active gate.
// The revert is skipped — and the reason recorded in audit metadata — when:
//   - from_status_id is NULL (request pre-dates the column, or the prior
//     status was deleted; the FK is ON DELETE SET NULL), or
//   - the item has since drifted to a different status (already left the gated one).
//
// The revert calls WorkflowService.CommitTransition directly (not PerformTransition),
// so it bypasses gating logic — going backwards via a system action must not
// re-trigger an approval gate on the prior status.
func (s *ApprovalService) Cancel(ctx context.Context, requestID, actorUserID int, comment, reason string) error {
	type cancelOutcome struct {
		ran        bool
		itemID     int
		toStatusID int
		revertTo   int
	}

	outcome, err := database.WithTxResult(s.db, func(tx database.Tx) (cancelOutcome, error) {
		var out cancelOutcome
		req, err := s.runtimeRepo.LoadRequestByIDInTx(ctx, tx, requestID)
		if err != nil {
			return out, err
		}
		if req.Status != models.ApprovalRequestStatusPending {
			return out, nil // already finalized; nothing to do
		}
		out.itemID = req.ItemID
		out.toStatusID = req.StatusID

		if err := s.runtimeRepo.UpdateRequestStatusComplete(ctx, tx, requestID, models.ApprovalRequestStatusCancelled); err != nil {
			return out, err
		}

		auditMeta := map[string]any{"reason": reason}
		if req.FromStatusID == nil {
			auditMeta["skipped_revert_reason"] = "pre_migration"
		} else {
			currentStatusID, err := s.runtimeRepo.GetItemCurrentStatusID(ctx, tx, req.ItemID)
			if err != nil {
				return out, fmt.Errorf("load item status: %w", err)
			}
			if currentStatusID != req.StatusID {
				auditMeta["skipped_revert_reason"] = "status_drift"
			} else {
				out.revertTo = *req.FromStatusID
			}
		}

		if out.revertTo != 0 {
			itemRepo := repository.NewItemRepository(s.db)
			if err := s.workflowService.CommitTransition(tx, itemRepo, req.ItemID, req.StatusID, out.revertTo, actorUserID); err != nil {
				return out, fmt.Errorf("revert item status: %w", err)
			}
			auditMeta["reverted_to_status_id"] = out.revertTo
		}

		actor := &actorUserID
		if actorUserID == 0 {
			actor = nil
		}
		if _, err := s.runtimeRepo.WriteDecision(ctx, tx, requestID, nil, actor, nil, models.ApprovalDecisionCancel, comment, nil, auditMeta); err != nil {
			return out, err
		}
		out.ran = true
		return out, nil
	})
	if err != nil {
		return err
	}
	if !outcome.ran {
		return nil
	}

	if s.eventCoordinator != nil {
		req, err := s.GetRequest(requestID)
		if err == nil && req != nil {
			item, _ := repository.NewItemRepository(s.db).FindByIDWithDetails(req.ItemID)
			if item != nil {
				s.eventCoordinator.EmitApprovalCancelled(req, item, reason, actorUserID)
				if outcome.revertTo != 0 {
					oldStatus := outcome.toStatusID
					newStatus := outcome.revertTo
					s.eventCoordinator.EmitStatusChanged(item, &oldStatus, &newStatus, actorUserID)
				}
			}
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Escalate / Delegate / RefreshApprovers
// ----------------------------------------------------------------------------

// Escalate applies the configured escalation policy to a step instance.
// Called by the sweeper (actorUserID = 0, system) on timeout, or by an admin
// via POST /approvals/{id}/steps/{step_id}/escalate.
//
// Behavior depends on approval_steps.escalation_action:
//   - "reassign" (default): swap the approver pool to escalation_target_*,
//     write an audit row, increment escalation_count, re-arm escalation_due_at
//     if max_escalations is NULL or escalation_count < max_escalations.
//   - "skip_step": mark step status='escalated', advance the request as if the
//     step had approved.
//   - "auto_reject": mark step status='rejected', finalize the request as
//     rejected and fire the configured deny transition.
//
// Idempotent: if the step is no longer pending (already decided / escalated by
// another worker), Escalate is a no-op.
func (s *ApprovalService) Escalate(ctx context.Context, stepInstanceID, actorUserID int, reason string) error {
	itemRepo := repository.NewItemRepository(s.db)

	type escalateOutcome struct {
		ran                        bool
		action                     string
		newApproverIDs             []int
		newlyStartedStepInstanceID *int
		stepInstanceID             int
		requestID                  int
	}

	outcome, err := database.WithTxResult(s.db, func(tx database.Tx) (escalateOutcome, error) {
		var out escalateOutcome
		si, err := s.runtimeRepo.LoadStepInstanceByIDInTx(ctx, tx, stepInstanceID)
		if errors.Is(err, repository.ErrNotFound) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if si.Status != models.ApprovalStepStatusPending {
			return out, nil
		}

		step, err := s.templateRepo.FindStepByIDInTx(ctx, tx, si.ApprovalStepID)
		if err != nil {
			return out, err
		}

		req, err := s.runtimeRepo.LoadRequestByIDInTx(ctx, tx, si.ApprovalRequestID)
		if err != nil {
			return out, err
		}
		if req.Status != models.ApprovalRequestStatusPending {
			return out, nil
		}

		action := step.EscalationAction
		if action == "" {
			action = models.ApprovalEscalationActionReassign
		}

		var actor *int
		if actorUserID > 0 {
			actor = &actorUserID
		}

		switch action {
		case models.ApprovalEscalationActionReassign:
			newApproverIDs, err := s.escalateReassign(ctx, tx, si, step, req, actor, reason)
			if err != nil {
				return out, err
			}
			out.ran = true
			out.action = action
			out.newApproverIDs = newApproverIDs
			out.stepInstanceID = si.ID
			out.requestID = req.ID
			return out, nil

		case models.ApprovalEscalationActionSkipStep:
			if err := s.runtimeRepo.MarkStepEscalated(ctx, tx, si.ID, models.ApprovalStepStatusEscalated); err != nil {
				return out, err
			}
			if _, err := s.runtimeRepo.WriteDecision(ctx, tx, req.ID, &si.ID, actor, nil, models.ApprovalDecisionEscalate, "", nil, map[string]any{
				"reason":     reason,
				"action":     action,
				"resolution": "skip_step",
			}); err != nil {
				return out, err
			}
			newID, err := s.advanceRequestAfterStep(ctx, tx, req, si, models.ApprovalStepStatusApproved, actorUserID, itemRepo)
			if err != nil {
				return out, err
			}
			out.ran = true
			out.action = action
			out.newlyStartedStepInstanceID = newID
			out.requestID = req.ID
			return out, nil

		case models.ApprovalEscalationActionAutoReject:
			if err := s.runtimeRepo.MarkStepEscalated(ctx, tx, si.ID, models.ApprovalStepStatusRejected); err != nil {
				return out, err
			}
			if _, err := s.runtimeRepo.WriteDecision(ctx, tx, req.ID, &si.ID, actor, nil, models.ApprovalDecisionEscalate, "", nil, map[string]any{
				"reason":     reason,
				"action":     action,
				"resolution": "auto_reject",
			}); err != nil {
				return out, err
			}
			if _, err := s.advanceRequestAfterStep(ctx, tx, req, si, models.ApprovalStepStatusRejected, actorUserID, itemRepo); err != nil {
				return out, err
			}
			out.ran = true
			out.action = action
			out.requestID = req.ID
			return out, nil
		}
		return out, fmt.Errorf("unknown escalation_action %q", action)
	})
	if err != nil {
		return err
	}
	if !outcome.ran {
		return nil
	}

	if s.eventCoordinator == nil {
		return nil
	}
	switch outcome.action {
	case models.ApprovalEscalationActionReassign:
		req, _ := s.GetRequest(outcome.requestID)
		if req == nil {
			return nil
		}
		item, _ := itemRepo.FindByIDWithDetails(req.ItemID)
		if item != nil {
			for i := range req.StepInstances {
				if req.StepInstances[i].ID == outcome.stepInstanceID {
					s.eventCoordinator.EmitApprovalEscalated(req, &req.StepInstances[i], outcome.action, outcome.newApproverIDs, item, actorUserID)
					break
				}
			}
		}
	default:
		s.emitEscalationCompletion(outcome.requestID, outcome.action, nil, outcome.newlyStartedStepInstanceID, actorUserID)
	}
	return nil
}

// escalateReassign swaps the approver pool to the configured escalation target.
// Returns the list of newly-active approver user IDs.
func (s *ApprovalService) escalateReassign(ctx context.Context, tx database.Tx, si *models.ApprovalStepInstance, step *models.ApprovalStep, req *models.ApprovalRequest, actor *int, reason string) ([]int, error) {
	if step.EscalationTargetSource == "" {
		return nil, fmt.Errorf("escalation_action=reassign requires escalation_target_source")
	}

	priorPool, err := s.runtimeRepo.LoadActiveApproverUserIDs(ctx, tx, si.ID)
	if err != nil {
		return nil, err
	}

	if err := s.runtimeRepo.DeactivateApprovers(ctx, tx, si.ID); err != nil {
		return nil, err
	}

	// Resolve the escalation target as if it were the approver_source. Reuse
	// resolveApproverSource by rewriting the step into a target-shaped probe.
	probe := *step
	probe.ApproverSource = step.EscalationTargetSource
	probe.ApproverFieldIdentifier = step.EscalationTargetFieldIdentifier
	probe.ApproverFieldID = step.EscalationTargetFieldID
	probe.ApproverRoleID = step.EscalationTargetRoleID
	probe.ApproverGroupID = step.EscalationTargetGroupID
	probe.ApproverUserID = step.EscalationTargetUserID

	itemRepo := repository.NewItemRepository(s.db)
	item, err := itemRepo.FindByID(req.ItemID)
	if err != nil {
		return nil, fmt.Errorf("reload item: %w", err)
	}
	if err := s.resolveAndSnapshotApprovers(ctx, tx, si.ID, probe, item, req.TriggeredByUserID); err != nil {
		return nil, fmt.Errorf("resolve escalation target: %w", err)
	}

	newPool, err := s.runtimeRepo.LoadActiveApproverUserIDs(ctx, tx, si.ID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	newCount := si.EscalationCount + 1
	var newDue sql.NullTime
	if step.EscalationAfterHours != nil {
		if step.MaxEscalations == nil || newCount < *step.MaxEscalations {
			newDue = sql.NullTime{Time: now.Add(time.Duration(*step.EscalationAfterHours) * time.Hour), Valid: true}
		}
	}
	if err := s.runtimeRepo.UpdateEscalationCounters(ctx, tx, si.ID, newCount, now, newDue); err != nil {
		return nil, err
	}
	si.EscalationCount = newCount

	if _, err := s.runtimeRepo.WriteDecision(ctx, tx, req.ID, &si.ID, actor, nil, models.ApprovalDecisionEscalate, "", nil, map[string]any{
		"reason":           reason,
		"action":           models.ApprovalEscalationActionReassign,
		"prior_pool":       priorPool,
		"new_pool":         newPool,
		"escalation_count": newCount,
		"max_escalations":  step.MaxEscalations,
	}); err != nil {
		return nil, err
	}
	return newPool, nil
}

// emitEscalationCompletion fires the post-tx event for skip_step / auto_reject
// escalations. For reassign, EmitApprovalEscalated is called inline with the
// new pool; here we fire the request-level Completed event when applicable.
func (s *ApprovalService) emitEscalationCompletion(requestID int, action string, _ []int, newlyStartedStepInstanceID *int, actorUserID int) {
	if s.eventCoordinator == nil {
		return
	}
	out, err := s.GetRequest(requestID)
	if err != nil || out == nil {
		return
	}
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(out.ItemID)
	if err != nil || item == nil {
		return
	}
	for i := range out.StepInstances {
		si := &out.StepInstances[i]
		if si.Status == models.ApprovalStepStatusEscalated || si.Status == models.ApprovalStepStatusRejected {
			s.eventCoordinator.EmitApprovalEscalated(out, si, action, nil, item, actorUserID)
			break
		}
	}
	if newlyStartedStepInstanceID != nil {
		for i := range out.StepInstances {
			si := &out.StepInstances[i]
			if si.ID == *newlyStartedStepInstanceID {
				s.eventCoordinator.EmitApprovalStepStarted(out, si, approverUserIDs(si.Approvers), approverPortalCustomerIDs(si.Approvers), item, actorUserID)
				break
			}
		}
	}
	if out.Status == models.ApprovalRequestStatusApproved || out.Status == models.ApprovalRequestStatusRejected {
		s.eventCoordinator.EmitApprovalCompleted(out, item, actorUserID)
	}
}

// Delegate hands the actor's seat in the active step pool to another user.
// Mirrors the on-leave substitute flow but driven by the user.
func (s *ApprovalService) Delegate(ctx context.Context, requestID, actorUserID, toUserID int, comment string) error {
	if toUserID == 0 || toUserID == actorUserID {
		return errors.New("delegate target must be a different user")
	}

	stepInstanceID, err := database.WithTxResult(s.db, func(tx database.Tx) (int, error) {
		stepInstance, err := s.runtimeRepo.FindActiveStepForUser(ctx, tx, requestID, actorUserID)
		if err != nil {
			return 0, err
		}
		if stepInstance == nil {
			return 0, fmt.Errorf("user %d is not an active approver of request %d", actorUserID, requestID)
		}
		if err := s.runtimeRepo.DeactivateApproverByUser(ctx, tx, stepInstance.ID, actorUserID); err != nil {
			return 0, err
		}
		if err := s.runtimeRepo.InsertDelegatedApprover(ctx, tx, stepInstance.ID, toUserID, actorUserID); err != nil {
			return 0, err
		}
		delegated := toUserID
		if _, err := s.runtimeRepo.WriteDecision(ctx, tx, requestID, &stepInstance.ID, &actorUserID, nil,
			models.ApprovalDecisionDelegate, comment, &delegated, map[string]any{
				"from_user_id": actorUserID,
				"to_user_id":   toUserID,
			}); err != nil {
			return 0, err
		}
		return stepInstance.ID, nil
	})
	if err != nil {
		return err
	}

	if s.eventCoordinator != nil {
		out, _ := s.GetRequest(requestID)
		if out != nil {
			item, _ := repository.NewItemRepository(s.db).FindByIDWithDetails(out.ItemID)
			if item != nil {
				for i := range out.StepInstances {
					if out.StepInstances[i].ID == stepInstanceID {
						s.eventCoordinator.EmitApprovalStepStarted(out, &out.StepInstances[i], []int{toUserID}, nil, item, actorUserID)
						break
					}
				}
			}
		}
	}
	return nil
}

// RefreshApprovers re-resolves the configured approver_source for a pending
// step instance, applies on-leave handling, and replaces the snapshot. Admin
// path — useful when a source field was edited mid-flow and the admin wants
// the change to take effect.
func (s *ApprovalService) RefreshApprovers(ctx context.Context, stepInstanceID, actorUserID int, comment string) error {
	type refreshOutcome struct {
		stepInstanceID  int
		newPool         []int
		newCustomerPool []int
		requestID       int
	}

	outcome, err := database.WithTxResult(s.db, func(tx database.Tx) (refreshOutcome, error) {
		var out refreshOutcome
		si, err := s.runtimeRepo.LoadStepInstanceByIDInTx(ctx, tx, stepInstanceID)
		if errors.Is(err, repository.ErrNotFound) {
			return out, errors.New("step instance not found")
		}
		if err != nil {
			return out, err
		}
		if si.Status != models.ApprovalStepStatusPending {
			return out, errors.New("step instance is not pending")
		}

		req, err := s.runtimeRepo.LoadRequestByIDInTx(ctx, tx, si.ApprovalRequestID)
		if err != nil {
			return out, err
		}

		step, err := s.templateRepo.FindStepByIDInTx(ctx, tx, si.ApprovalStepID)
		if err != nil {
			return out, err
		}

		priorPool, err := s.runtimeRepo.LoadActiveApproverUserIDs(ctx, tx, si.ID)
		if err != nil {
			return out, err
		}

		if err := s.runtimeRepo.DeactivateApprovers(ctx, tx, si.ID); err != nil {
			return out, err
		}

		item, err := repository.NewItemRepository(s.db).FindByID(req.ItemID)
		if err != nil {
			return out, err
		}
		if err := s.resolveAndSnapshotApprovers(ctx, tx, si.ID, *step, item, req.TriggeredByUserID); err != nil {
			return out, err
		}

		newPool, err := s.runtimeRepo.LoadActiveApproverUserIDs(ctx, tx, si.ID)
		if err != nil {
			return out, err
		}
		newCustomerPool, err := s.runtimeRepo.LoadActiveApproverCustomerIDs(ctx, tx, si.ID)
		if err != nil {
			return out, err
		}

		actor := &actorUserID
		if actorUserID == 0 {
			actor = nil
		}
		if _, err := s.runtimeRepo.WriteDecision(ctx, tx, req.ID, &si.ID, actor, nil, models.ApprovalDecisionReassign, comment, nil, map[string]any{
			"reason":             "refresh_approvers",
			"prior_pool":         priorPool,
			"new_pool":           newPool,
			"new_pool_customers": newCustomerPool,
		}); err != nil {
			return out, err
		}
		out.stepInstanceID = si.ID
		out.newPool = newPool
		out.newCustomerPool = newCustomerPool
		out.requestID = req.ID
		return out, nil
	})
	if err != nil {
		return err
	}

	if s.eventCoordinator != nil {
		req, _ := s.GetRequest(outcome.requestID)
		if req == nil {
			return nil
		}
		itm, _ := repository.NewItemRepository(s.db).FindByIDWithDetails(req.ItemID)
		if itm != nil {
			for i := range req.StepInstances {
				if req.StepInstances[i].ID == outcome.stepInstanceID {
					s.eventCoordinator.EmitApprovalStepStarted(req, &req.StepInstances[i], outcome.newPool, outcome.newCustomerPool, itm, actorUserID)
					break
				}
			}
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Gating helpers used by WorkflowService.PerformTransition
// ----------------------------------------------------------------------------

// IsTransitionGatedByApproval returns the pending approval request ID if the
// requested (from→to) transition is the configured approve_transition_id or
// deny_transition_id of an in-flight pending approval on this item. Returns
// nil if not gated.
func (s *ApprovalService) IsTransitionGatedByApproval(itemID, fromStatusID, toStatusID int) (*int, error) {
	return s.runtimeRepo.FindGatedRequestForTransition(context.Background(), itemID, fromStatusID, toStatusID)
}

// PendingApprovalSummary is the compact view returned alongside available
// transitions so the picker can render a "Pending approval" affordance and so
// callers can avoid reproducing the active-pool check on the client.
type PendingApprovalSummary struct {
	ID           int    `json:"id"`
	Status       string `json:"status"`
	YouCanDecide bool   `json:"you_can_decide"`
}

// GetGatedTransitionsForItem returns the set of workflow_transition IDs that
// the user may not invoke directly because an in-flight approval owns them
// (its configured approve_transition_id and deny_transition_id), plus a compact
// summary of the pending request. Returns (nil, nil, nil) when no approval is
// pending.
func (s *ApprovalService) GetGatedTransitionsForItem(itemID, userID int) ([]int, *PendingApprovalSummary, error) {
	view, err := s.runtimeRepo.FindGatedTransitionsForItem(context.Background(), itemID, userID)
	if err != nil {
		return nil, nil, err
	}
	if view == nil {
		return nil, nil, nil
	}
	return []int{view.ApproveTransitionID, view.DenyTransitionID}, &PendingApprovalSummary{
		ID:           view.RequestID,
		Status:       view.Status,
		YouCanDecide: view.UserCanDecide,
	}, nil
}

// MaybeOpenForStatusEntry opens a new approval request iff the (workspace,
// item-type, status) tuple resolves to an approval_set_status. If no approval
// is configured for the destination status, returns (nil, nil) — safe to call
// for every transition.
func (s *ApprovalService) MaybeOpenForStatusEntry(ctx context.Context, itemID, statusID, fromStatusID, actorUserID int) (*models.ApprovalRequest, error) {
	item, err := repository.NewItemRepository(s.db).FindByID(itemID)
	if err != nil {
		return nil, err
	}
	ass, err := s.GetApprovalSetStatusForItem(item.WorkspaceID, item.ItemTypeID, statusID)
	if err != nil {
		return nil, err
	}
	if ass == nil {
		return nil, nil
	}
	return s.RequestApproval(ctx, itemID, statusID, fromStatusID, actorUserID)
}

// ----------------------------------------------------------------------------
// Listing helpers
// ----------------------------------------------------------------------------

// GetPendingForItem returns the single pending approval request for an item, or nil.
func (s *ApprovalService) GetPendingForItem(itemID int) (*models.ApprovalRequest, error) {
	id, err := s.runtimeRepo.FindPendingRequestIDForItem(context.Background(), itemID)
	if err != nil {
		return nil, err
	}
	if id == nil {
		return nil, nil
	}
	return s.GetRequest(*id)
}

// GetTimelineForItem returns all approval requests for an item, ordered by created_at.
func (s *ApprovalService) GetTimelineForItem(itemID int) ([]*models.ApprovalRequest, error) {
	ids, err := s.runtimeRepo.FindRequestIDsForItem(context.Background(), itemID)
	if err != nil {
		return nil, err
	}
	out := make([]*models.ApprovalRequest, 0, len(ids))
	for _, id := range ids {
		req, err := s.GetRequest(id)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, nil
}

// GetForUser returns approval requests where the user is in the active approver
// pool of any pending step. status filters request status (empty = "pending").
func (s *ApprovalService) GetForUser(userID int, status string) ([]*models.ApprovalRequest, error) {
	return s.getForActor("user_id", userID, status)
}

// UserHasActivePoolMembershipOnItem returns true iff the user is in an active
// approver row of a step that is currently active on a pending approval request
// for itemID. This is the gate used by approver-derived item-view access:
// when the step closes (is_active flipped to 0) or the request is no longer
// pending, the user immediately loses approver-derived access.
func (s *ApprovalService) UserHasActivePoolMembershipOnItem(userID, itemID int) (bool, error) {
	return s.runtimeRepo.ActorHasActivePoolMembershipOnItem(context.Background(), "user_id", userID, itemID)
}

// PortalCustomerHasActivePoolMembershipOnItem is the portal-customer counterpart
// to UserHasActivePoolMembershipOnItem.
func (s *ApprovalService) PortalCustomerHasActivePoolMembershipOnItem(customerID, itemID int) (bool, error) {
	return s.runtimeRepo.ActorHasActivePoolMembershipOnItem(context.Background(), "portal_customer_id", customerID, itemID)
}

// GetForPortalCustomer is the customer-flavored counterpart to GetForUser.
// Returns approval requests where the portal customer is in the active pool.
func (s *ApprovalService) GetForPortalCustomer(customerID int, status string) ([]*models.ApprovalRequest, error) {
	return s.getForActor("portal_customer_id", customerID, status)
}

func (s *ApprovalService) getForActor(actorColumn string, actorID int, status string) ([]*models.ApprovalRequest, error) {
	ids, err := s.runtimeRepo.FindRequestIDsForActor(context.Background(), actorColumn, actorID, status)
	if err != nil {
		return nil, err
	}
	out := make([]*models.ApprovalRequest, 0, len(ids))
	for _, id := range ids {
		req, err := s.GetRequest(id)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, nil
}

// GetRequest loads a request with its step instances, approvers, and decisions.
func (s *ApprovalService) GetRequest(requestID int) (*models.ApprovalRequest, error) {
	req, err := s.runtimeRepo.FindFullRequestByID(context.Background(), requestID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, sql.ErrNoRows
	}
	return req, err
}

// GetItemIDForRequest returns the item id behind an approval request, or
// sql.ErrNoRows if none. Pass-through to ApprovalRepository so handlers don't
// need a repo reference of their own.
func (s *ApprovalService) GetItemIDForRequest(ctx context.Context, requestID int) (int, error) {
	id, err := s.runtimeRepo.GetItemIDForRequest(ctx, requestID)
	if errors.Is(err, repository.ErrNotFound) {
		return 0, sql.ErrNoRows
	}
	return id, err
}

// StepInstanceBelongsToRequest reports whether a step instance belongs to the
// given approval request. Pass-through to ApprovalRepository.
func (s *ApprovalService) StepInstanceBelongsToRequest(ctx context.Context, stepInstanceID, requestID int) (bool, error) {
	return s.runtimeRepo.StepInstanceBelongsToRequest(ctx, stepInstanceID, requestID)
}

// CountPendingApproversForRole returns the number of pending approval-step
// approver rows that resolved through this workspace role. Used by the
// workspace-role delete path to refuse the request when it would orphan an
// in-flight pool.
func (s *ApprovalService) CountPendingApproversForRole(ctx context.Context, roleID int) (int, error) {
	return s.runtimeRepo.CountPendingApproversForRole(ctx, roleID)
}

// ----------------------------------------------------------------------------
// Internals
// ----------------------------------------------------------------------------

// approvalActor is the polymorphic actor for an approval action — exactly one
// of UserID / PortalCustomerID is set. Internal callers use UserID; the portal
// surface uses PortalCustomerID. System-driven actions (sweeper escalations)
// pass the zero value: both nil.
type approvalActor struct {
	UserID           *int
	PortalCustomerID *int
}

func actorFromUser(userID int) approvalActor {
	return approvalActor{UserID: &userID}
}

func actorFromCustomer(customerID int) approvalActor {
	return approvalActor{PortalCustomerID: &customerID}
}

func (a approvalActor) isSet() bool {
	return a.UserID != nil || a.PortalCustomerID != nil
}

// findActiveStepForActor returns the lowest-display-order pending step instance
// where the actor (user or portal customer) has an active approver row, or nil
// if none. Branches on whichever id is set in the actor struct.
func (s *ApprovalService) findActiveStepForActor(ctx context.Context, tx database.Tx, requestID int, actor approvalActor) (*models.ApprovalStepInstance, error) {
	if !actor.isSet() {
		return nil, nil
	}
	if actor.UserID != nil {
		return s.runtimeRepo.FindActiveStepForUser(ctx, tx, requestID, *actor.UserID)
	}
	return s.runtimeRepo.FindActiveStepForCustomer(ctx, tx, requestID, *actor.PortalCustomerID)
}

// evaluateStepStatus returns the new step status based on votes vs quorum.
// The current status (passed implicitly via the step instance) is not consulted —
// caller compares to the prior status to decide whether to write an UPDATE.
func (s *ApprovalService) evaluateStepStatus(ctx context.Context, tx database.Tx, stepInstanceID int, step *models.ApprovalStep) (string, error) {
	poolSize, err := s.runtimeRepo.CountActiveApprovers(ctx, tx, stepInstanceID)
	if err != nil {
		return "", err
	}
	if poolSize == 0 {
		return models.ApprovalStepStatusPending, nil
	}

	approves, rejects, err := s.runtimeRepo.CountVotes(ctx, tx, stepInstanceID)
	if err != nil {
		return "", err
	}

	switch step.RejectionPolicy {
	case models.ApprovalRejectionPolicyAnyFails, "":
		if rejects > 0 {
			return models.ApprovalStepStatusRejected, nil
		}
	case models.ApprovalRejectionPolicyQuorumRequired:
		if rejects >= quorumThreshold(step, poolSize) {
			return models.ApprovalStepStatusRejected, nil
		}
	}

	if approves >= quorumThreshold(step, poolSize) {
		return models.ApprovalStepStatusApproved, nil
	}
	return models.ApprovalStepStatusPending, nil
}

// quorumThreshold computes the integer threshold for a step+pool-size.
func quorumThreshold(step *models.ApprovalStep, poolSize int) int {
	switch step.QuorumMode {
	case models.ApprovalQuorumModeAny, "":
		return 1
	case models.ApprovalQuorumModeAll:
		return poolSize
	case models.ApprovalQuorumModeCount:
		if step.QuorumCount == nil || *step.QuorumCount < 1 {
			return 1
		}
		if *step.QuorumCount > poolSize {
			return poolSize
		}
		return *step.QuorumCount
	case models.ApprovalQuorumModePercent:
		if step.QuorumPercent == nil || *step.QuorumPercent < 1 {
			return 1
		}
		t := (poolSize**step.QuorumPercent + 99) / 100
		if t < 1 {
			t = 1
		}
		if t > poolSize {
			t = poolSize
		}
		return t
	default:
		return 1
	}
}

// ----------------------------------------------------------------------------
// Approver resolution + on-leave handling
// ----------------------------------------------------------------------------

// resolvedApprover is one entry produced by resolveApproverSource. Exactly
// one of UserID / PortalCustomerID is set:
//   - UserID > 0: an internal user (the historical case)
//   - PortalCustomerID > 0: a portal customer (creator polymorphism today;
//     a future "participant" custom field type would feed this too)
//
// SubstitutedForUserID applies only when on-leave handling swapped a user
// for their substitute. Customers don't have leave periods in v1 so this
// stays nil for portal-customer rows.
type resolvedApprover struct {
	UserID               int
	PortalCustomerID     int
	SourceRoleID         *int
	SourceGroupID        *int
	SubstitutedForUserID *int
}

func (r resolvedApprover) isCustomer() bool { return r.PortalCustomerID > 0 }

// resolveAndSnapshotApprovers resolves the configured approver_source for a step,
// applies on-leave handling (use_substitute / skip / keep) per UserLeavePeriod,
// and writes the snapshot rows to approval_step_approvers. If the resolved pool
// is empty, the step is left with no approvers — slice 9 will escalate;
// for now Decide returns "user is not an active approver" until manual intervention.
func (s *ApprovalService) resolveAndSnapshotApprovers(ctx context.Context, tx database.Tx, stepInstanceID int, step models.ApprovalStep, item *models.Item, triggeredByUserID int) error {
	rawUsers, err := s.resolveApproverSource(ctx, tx, step, item, triggeredByUserID)
	if err != nil {
		return err
	}

	finalPool := make([]resolvedApprover, 0, len(rawUsers))
	for _, ra := range rawUsers {
		if ra.isCustomer() {
			finalPool = append(finalPool, ra)
			continue
		}

		var onLeave bool
		if s.leaveRepo != nil {
			leave, err := s.leaveRepo.GetActiveForUser(ra.UserID)
			if err == nil && leave != nil {
				onLeave = true
				switch step.OnLeaveStrategy {
				case models.ApprovalOnLeaveUseSubstitute, "":
					if leave.SubstituteUserID != nil && *leave.SubstituteUserID != 0 {
						sub := *leave.SubstituteUserID
						subOrig := ra.UserID
						substitute := resolvedApprover{
							UserID:               sub,
							SourceRoleID:         ra.SourceRoleID,
							SourceGroupID:        ra.SourceGroupID,
							SubstitutedForUserID: &subOrig,
						}
						finalPool = append(finalPool, substitute)
						parentReqID, _ := s.runtimeRepo.GetRequestIDForStep(ctx, tx, stepInstanceID)
						if _, err := s.runtimeRepo.WriteDecision(ctx, tx, parentReqID, &stepInstanceID, nil, nil,
							models.ApprovalDecisionSubstitute, "", nil, map[string]any{
								"original_user_id":   subOrig,
								"substitute_user_id": sub,
								"reason":             "active_leave",
							}); err != nil {
							return err
						}
						continue
					}
					// No substitute configured: fall back to keeping the
					// original approver. Dropping them silently leaves the
					// pool potentially empty and the request unactionable.
					finalPool = append(finalPool, ra)
				case models.ApprovalOnLeaveSkip:
					// drop
				case models.ApprovalOnLeaveKeep:
					finalPool = append(finalPool, ra)
				}
			}
		}
		if !onLeave {
			finalPool = append(finalPool, ra)
		}
	}

	// Self-approval guard. Customers can't trigger an approval today
	// (triggered_by_user_id is users-only); they're never blocked here.
	if !step.AllowSelfApproval && triggeredByUserID != 0 {
		filtered := finalPool[:0]
		for _, ra := range finalPool {
			if !ra.isCustomer() && ra.UserID == triggeredByUserID {
				continue
			}
			filtered = append(filtered, ra)
		}
		finalPool = filtered
	}

	for _, ra := range finalPool {
		ai := repository.ApproverInsert{
			UserID:               ra.UserID,
			PortalCustomerID:     ra.PortalCustomerID,
			SourceRoleID:         ra.SourceRoleID,
			SourceGroupID:        ra.SourceGroupID,
			SubstitutedForUserID: ra.SubstitutedForUserID,
		}
		if err := s.runtimeRepo.InsertApprover(ctx, tx, stepInstanceID, ai); err != nil {
			return err
		}
	}
	return nil
}

// resolveApproverSource turns the configured source into a list of user IDs
// (with provenance metadata). The cross-domain reads (items, user_workspace_roles,
// group_members) intentionally stay inline rather than moving to ApprovalRepository
// — they belong to other domains and folding them into an approval repo would
// blur the boundaries.
func (s *ApprovalService) resolveApproverSource(ctx context.Context, tx database.Tx, step models.ApprovalStep, item *models.Item, triggeredByUserID int) ([]resolvedApprover, error) {
	switch step.ApproverSource {
	case models.ApprovalSourceCreator:
		if item.CreatorPortalCustomerID != nil && *item.CreatorPortalCustomerID != 0 {
			return []resolvedApprover{{PortalCustomerID: *item.CreatorPortalCustomerID}}, nil
		}
		if item.CreatorID != nil && *item.CreatorID != 0 {
			return []resolvedApprover{{UserID: *item.CreatorID}}, nil
		}
		return nil, nil

	case models.ApprovalSourceAssignee:
		if item.AssigneeID != nil && *item.AssigneeID != 0 {
			return []resolvedApprover{{UserID: *item.AssigneeID}}, nil
		}
		return nil, nil

	case models.ApprovalSourceCurrentUser:
		if triggeredByUserID != 0 {
			return []resolvedApprover{{UserID: triggeredByUserID}}, nil
		}
		return nil, nil

	case models.ApprovalSourceUser:
		if step.ApproverUserID != nil && *step.ApproverUserID != 0 {
			return []resolvedApprover{{UserID: *step.ApproverUserID}}, nil
		}
		return nil, nil

	case models.ApprovalSourceRegularField:
		if _, ok := models.AllowedRegularApproverFields[step.ApproverFieldIdentifier]; !ok {
			return nil, fmt.Errorf("regular_field %q is not in the approver whitelist", step.ApproverFieldIdentifier)
		}
		var userID sql.NullInt64
		query := fmt.Sprintf(`SELECT %s FROM items WHERE id = ?`, step.ApproverFieldIdentifier)
		if err := tx.QueryRowContext(ctx, query, item.ID).Scan(&userID); err != nil {
			return nil, err
		}
		if !userID.Valid || userID.Int64 == 0 {
			return nil, nil
		}
		return []resolvedApprover{{UserID: int(userID.Int64)}}, nil

	case models.ApprovalSourceCustomField:
		if step.ApproverFieldID == nil {
			return nil, errors.New("custom_field source requires approver_field_id")
		}
		raw, err := repository.NewItemRepository(s.db).GetCustomFieldValuesRaw(item.ID)
		if err != nil {
			return nil, err
		}
		if !raw.Valid || raw.String == "" {
			return nil, nil
		}
		var values map[string]interface{}
		if err := json.Unmarshal([]byte(raw.String), &values); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%d", *step.ApproverFieldID)
		v, ok := values[key]
		if !ok {
			return nil, nil
		}
		return userListFromValue(v), nil

	case models.ApprovalSourceRole:
		if step.ApproverRoleID == nil {
			return nil, errors.New("role source requires approver_role_id")
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT DISTINCT user_id FROM user_workspace_roles
			WHERE workspace_id = ? AND role_id = ?
			UNION
			SELECT DISTINCT gm.user_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gm.group_id = gwr.group_id
			WHERE gwr.workspace_id = ? AND gwr.role_id = ?
		`, item.WorkspaceID, *step.ApproverRoleID, item.WorkspaceID, *step.ApproverRoleID)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		var out []resolvedApprover
		for rows.Next() {
			var uid int
			if err := rows.Scan(&uid); err != nil {
				return nil, err
			}
			rid := *step.ApproverRoleID
			out = append(out, resolvedApprover{UserID: uid, SourceRoleID: &rid})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return out, nil

	case models.ApprovalSourceGroup:
		if step.ApproverGroupID == nil {
			return nil, errors.New("group source requires approver_group_id")
		}
		rows, err := tx.QueryContext(ctx, `SELECT user_id FROM group_members WHERE group_id = ?`, *step.ApproverGroupID)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		var out []resolvedApprover
		for rows.Next() {
			var uid int
			if err := rows.Scan(&uid); err != nil {
				return nil, err
			}
			gid := *step.ApproverGroupID
			out = append(out, resolvedApprover{UserID: uid, SourceGroupID: &gid})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported approver_source %q", step.ApproverSource)
}

// userListFromValue interprets a custom-field value as a user id or list of user ids.
func userListFromValue(v interface{}) []resolvedApprover {
	switch val := v.(type) {
	case float64:
		if val > 0 {
			return []resolvedApprover{{UserID: int(val)}}
		}
	case int:
		if val > 0 {
			return []resolvedApprover{{UserID: val}}
		}
	case []interface{}:
		var out []resolvedApprover
		for _, item := range val {
			if uid, ok := toInt(item); ok && uid > 0 {
				out = append(out, resolvedApprover{UserID: uid})
			}
		}
		return out
	case string:
		var n int
		_, err := fmt.Sscanf(strings.TrimSpace(val), "%d", &n)
		if err == nil && n > 0 {
			return []resolvedApprover{{UserID: n}}
		}
	}
	return nil
}
