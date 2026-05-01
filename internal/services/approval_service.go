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
type ApprovalService struct {
	db              database.Database
	permService     *PermissionService
	leaveRepo       *repository.LeaveRepository
	workflowService *WorkflowService

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
	var isPersonal bool
	if err := s.db.QueryRow(`SELECT is_personal FROM workspaces WHERE id = ?`, workspaceID).Scan(&isPersonal); err == nil && isPersonal {
		return nil, nil
	}

	var approvalSetID *int

	if itemTypeID != nil {
		err := s.db.QueryRow(`
			SELECT COALESCE(csit.approval_set_id, cs.approval_set_id) as approval_set_id
			FROM workspace_configuration_sets wcs
			JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
			LEFT JOIN configuration_set_item_types csit
				ON cs.id = csit.configuration_set_id AND csit.item_type_id = ?
			WHERE wcs.workspace_id = ?
		`, *itemTypeID, workspaceID).Scan(&approvalSetID)
		if err == nil && approvalSetID != nil {
			return approvalSetID, nil
		}
	}

	err := s.db.QueryRow(`
		SELECT cs.approval_set_id
		FROM workspace_configuration_sets wcs
		JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
		WHERE wcs.workspace_id = ?
	`, workspaceID).Scan(&approvalSetID)
	if err == nil && approvalSetID != nil {
		return approvalSetID, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if itemTypeID != nil {
		err = s.db.QueryRow(`
			SELECT COALESCE(csit.approval_set_id, cs.approval_set_id) as approval_set_id
			FROM configuration_sets cs
			LEFT JOIN configuration_set_item_types csit
				ON cs.id = csit.configuration_set_id AND csit.item_type_id = ?
			WHERE cs.is_default = true
		`, *itemTypeID).Scan(&approvalSetID)
		if err == nil && approvalSetID != nil {
			return approvalSetID, nil
		}
	}

	err = s.db.QueryRow(`SELECT approval_set_id FROM configuration_sets WHERE is_default = true`).Scan(&approvalSetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return approvalSetID, nil
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

	var ass models.ApprovalSetStatus
	err = s.db.QueryRow(`
		SELECT id, approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode, created_at
		FROM approval_set_statuses
		WHERE approval_set_id = ? AND status_id = ?
	`, *approvalSetID, statusID).Scan(
		&ass.ID, &ass.ApprovalSetID, &ass.StatusID,
		&ass.ApproveTransitionID, &ass.DenyTransitionID,
		&ass.StepMode, &ass.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ass, nil
}

// ----------------------------------------------------------------------------
// RequestApproval: open a new pending approval request.
// ----------------------------------------------------------------------------

// RequestApproval opens a new approval request for the item. The caller (typically
// PerformTransition's post-commit hook) is responsible for ensuring no pending
// request already exists; the unique partial index uq_approval_requests_one_open_per_item
// enforces this at the DB layer as defense in depth.
func (s *ApprovalService) RequestApproval(ctx context.Context, itemID, statusID, triggeredByUserID int) (*models.ApprovalRequest, error) {
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

	steps, err := s.loadSteps(ass.ID)
	if err != nil {
		return nil, fmt.Errorf("load steps: %w", err)
	}
	if len(steps) == 0 {
		return nil, nil // misconfigured set; treat as no-op
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`
		INSERT INTO approval_requests (item_id, approval_set_status_id, status_id, triggered_by_user_id, status, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?)
	`, itemID, ass.ID, statusID, triggeredByUserID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("insert approval_request: %w", err)
	}
	requestID64, _ := res.LastInsertId()
	requestID := int(requestID64)

	// Create step instances per template, in order. For sequential mode only step 0 starts now.
	now := time.Now()
	stepInstanceIDs := make([]int, len(steps))
	for i, step := range steps {
		startedAt := sql.NullTime{}
		status := models.ApprovalStepStatusPending
		if ass.StepMode == models.ApprovalStepModeParallel || i == 0 {
			startedAt = sql.NullTime{Time: now, Valid: true}
		}

		var dueAt sql.NullTime
		if step.EscalationAfterHours != nil && startedAt.Valid {
			dueAt = sql.NullTime{Time: now.Add(time.Duration(*step.EscalationAfterHours) * time.Hour), Valid: true}
		}

		r, err := tx.Exec(`
			INSERT INTO approval_step_instances (approval_request_id, approval_step_id, display_order, status, escalation_due_at, started_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, requestID, step.ID, step.DisplayOrder, status, dueAt, startedAt)
		if err != nil {
			return nil, fmt.Errorf("insert step instance: %w", err)
		}
		sid64, _ := r.LastInsertId()
		stepInstanceIDs[i] = int(sid64)
	}

	// Resolve and snapshot the approver pool for steps that are now started.
	for i, step := range steps {
		startedNow := ass.StepMode == models.ApprovalStepModeParallel || i == 0
		if !startedNow {
			continue
		}
		if err := s.resolveAndSnapshotApprovers(tx, stepInstanceIDs[i], step, item, triggeredByUserID); err != nil {
			return nil, fmt.Errorf("resolve approvers (step %d): %w", step.DisplayOrder, err)
		}
	}

	// Audit row for the request opening.
	if err := writeDecision(tx, requestID, nil, nil, nil, models.ApprovalDecisionRequested, "", nil, map[string]any{
		"triggered_by_user_id": triggeredByUserID,
		"approval_set_status":  ass.ID,
		"step_mode":            ass.StepMode,
		"step_count":           len(steps),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var req models.ApprovalRequest
	err = tx.QueryRow(`
		SELECT id, item_id, approval_set_status_id, status_id, triggered_by_user_id, status, created_at, completed_at
		FROM approval_requests WHERE id = ?
	`, requestID).Scan(
		&req.ID, &req.ItemID, &req.ApprovalSetStatusID, &req.StatusID,
		&req.TriggeredByUserID, &req.Status, &req.CreatedAt, &req.CompletedAt,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load request: %w", err)
	}
	if req.Status != models.ApprovalRequestStatusPending {
		return nil, nil, fmt.Errorf("approval request %d is not pending (status=%s)", requestID, req.Status)
	}

	stepInstance, err := s.findActiveStepForActor(tx, requestID, actor)
	if err != nil {
		return nil, nil, err
	}
	if stepInstance == nil {
		return nil, nil, fmt.Errorf("actor is not an active approver of request %d", requestID)
	}

	step, err := s.loadStep(tx, stepInstance.ApprovalStepID)
	if err != nil {
		return nil, nil, err
	}

	// Self-approval guard only applies to internal users — triggered_by_user_id
	// is users-only, so a customer-actor can never collide with it.
	if actor.UserID != nil && !step.AllowSelfApproval && *actor.UserID == req.TriggeredByUserID && decision != models.ApprovalDecisionComment {
		return nil, nil, fmt.Errorf("self-approval is not allowed for this step")
	}

	priorRequestStatus := req.Status

	// "Effective" actor user id passed downstream to CommitTransition's
	// item_history INSERT (which has a NOT NULL FK to users). For internal
	// actors this is just their id; for customer actors we fall back to the
	// requestor so the FK is satisfied — actor attribution on the approval
	// decision row stays accurate via actor_portal_customer_id.
	var effectiveActorUserID int
	if actor.UserID != nil {
		effectiveActorUserID = *actor.UserID
	} else {
		effectiveActorUserID = req.TriggeredByUserID
	}

	// Comment-only decisions are recorded but don't affect quorum.
	if decision == models.ApprovalDecisionComment {
		commentDecision, err := writeDecisionRet(tx, requestID, &stepInstance.ID, actor.UserID, actor.PortalCustomerID, decision, comment, nil, nil)
		if err != nil {
			return nil, nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, nil, err
		}
		out, err := s.GetRequest(requestID)
		if err != nil {
			return nil, nil, err
		}
		s.emitDecisionEvents(commentDecision, out, priorRequestStatus, nil, effectiveActorUserID)
		return commentDecision, out, nil
	}

	// Insert the vote (uq_approval_decisions_one_vote_per_actor enforces no double-voting).
	decisionRow, err := writeDecisionRet(tx, requestID, &stepInstance.ID, actor.UserID, actor.PortalCustomerID, decision, comment, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	// Recompute step state based on votes vs quorum.
	stepNewStatus, err := s.evaluateStepStatus(tx, stepInstance.ID, step)
	if err != nil {
		return nil, nil, err
	}
	if stepNewStatus != stepInstance.Status {
		if _, err := tx.Exec(`UPDATE approval_step_instances SET status = ?, completed_at = ? WHERE id = ?`,
			stepNewStatus, time.Now(), stepInstance.ID); err != nil {
			return nil, nil, fmt.Errorf("update step status: %w", err)
		}
	}

	// If the step is now complete, drive the request state machine.
	var newlyStartedStepInstanceID *int
	if stepNewStatus == models.ApprovalStepStatusApproved || stepNewStatus == models.ApprovalStepStatusRejected {
		nextID, err := s.advanceRequestAfterStep(ctx, tx, &req, stepInstance, stepNewStatus, effectiveActorUserID, itemRepo)
		if err != nil {
			return nil, nil, err
		}
		newlyStartedStepInstanceID = nextID
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	out, err := s.GetRequest(requestID)
	if err != nil {
		return nil, nil, err
	}
	s.emitDecisionEvents(decisionRow, out, priorRequestStatus, newlyStartedStepInstanceID, effectiveActorUserID)
	return decisionRow, out, nil
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
	ass, err := s.loadApprovalSetStatus(tx, req.ApprovalSetStatusID)
	if err != nil {
		return nil, err
	}

	if ass.StepMode == models.ApprovalStepModeParallel {
		return nil, s.evaluateParallelRequestState(ctx, tx, req, ass, stepInstance, stepStatus, actorUserID, itemRepo)
	}

	// Sequential mode.
	if stepStatus == models.ApprovalStepStatusRejected {
		return nil, s.finalizeRequest(ctx, tx, req, ass, models.ApprovalRequestStatusRejected, ass.DenyTransitionID, actorUserID, itemRepo)
	}

	// stepStatus == approved — start the next pending step or finalize.
	var nextStepInstanceID int
	var nextStepID int
	err = tx.QueryRow(`
		SELECT id, approval_step_id FROM approval_step_instances
		WHERE approval_request_id = ? AND display_order > ? AND status = 'pending'
		ORDER BY display_order
		LIMIT 1
	`, req.ID, stepInstance.DisplayOrder).Scan(&nextStepInstanceID, &nextStepID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, s.finalizeRequest(ctx, tx, req, ass, models.ApprovalRequestStatusApproved, ass.ApproveTransitionID, actorUserID, itemRepo)
	}
	if err != nil {
		return nil, fmt.Errorf("find next step: %w", err)
	}

	now := time.Now()
	nextStep, err := s.loadStep(tx, nextStepID)
	if err != nil {
		return nil, err
	}
	var dueAt sql.NullTime
	if nextStep.EscalationAfterHours != nil {
		dueAt = sql.NullTime{Time: now.Add(time.Duration(*nextStep.EscalationAfterHours) * time.Hour), Valid: true}
	}
	if _, err := tx.Exec(`UPDATE approval_step_instances SET started_at = ?, escalation_due_at = ? WHERE id = ?`,
		now, dueAt, nextStepInstanceID); err != nil {
		return nil, fmt.Errorf("start next step: %w", err)
	}

	item, err := itemRepo.FindByID(req.ItemID)
	if err != nil {
		return nil, fmt.Errorf("reload item: %w", err)
	}
	if err := s.resolveAndSnapshotApprovers(tx, nextStepInstanceID, *nextStep, item, req.TriggeredByUserID); err != nil {
		return nil, fmt.Errorf("snapshot next-step approvers: %w", err)
	}
	return &nextStepInstanceID, nil
}

// finalizeRequest commits the configured approve/deny transition and marks
// the request as approved/rejected.
func (s *ApprovalService) finalizeRequest(_ context.Context, tx database.Tx, req *models.ApprovalRequest, ass *models.ApprovalSetStatus, finalStatus string, transitionID, actorUserID int, itemRepo *repository.ItemRepository) error {
	// Look up the destination status from the chosen transition.
	var toStatusID, fromStatusID int
	if err := tx.QueryRow(`SELECT from_status_id, to_status_id FROM workflow_transitions WHERE id = ?`, transitionID).
		Scan(&fromStatusID, &toStatusID); err != nil {
		return fmt.Errorf("load transition: %w", err)
	}

	now := time.Now()
	if _, err := tx.Exec(`UPDATE approval_requests SET status = ?, completed_at = ? WHERE id = ?`,
		finalStatus, now, req.ID); err != nil {
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

	if _, err := writeDecisionRet(tx, req.ID, nil, nil, nil, models.ApprovalDecisionCompleted, "", nil, map[string]any{
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
		// Skip every still-pending peer step in the same tx so the audit log is
		// truthful: peers stop being actionable the moment one rejects.
		now := time.Now()
		if _, err := tx.Exec(`
			UPDATE approval_step_instances
			SET status = ?, completed_at = ?
			WHERE approval_request_id = ? AND status = 'pending' AND id <> ?
		`, models.ApprovalStepStatusSkipped, now, req.ID, stepInstance.ID); err != nil {
			return fmt.Errorf("skip peer steps: %w", err)
		}
		return s.finalizeRequest(ctx, tx, req, ass, models.ApprovalRequestStatusRejected, ass.DenyTransitionID, actorUserID, itemRepo)
	}

	// stepStatus == approved — finalize iff every step is approved.
	var pending, total int
	if err := tx.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN status NOT IN ('approved') THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM approval_step_instances WHERE approval_request_id = ?
	`, req.ID).Scan(&pending, &total); err != nil {
		return fmt.Errorf("count step states: %w", err)
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
func (s *ApprovalService) Cancel(ctx context.Context, requestID, actorUserID int, comment, reason string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	if err := tx.QueryRow(`SELECT status FROM approval_requests WHERE id = ?`, requestID).Scan(&status); err != nil {
		return err
	}
	if status != models.ApprovalRequestStatusPending {
		return nil // already finalized; nothing to do
	}

	if _, err := tx.Exec(`UPDATE approval_requests SET status = ?, completed_at = ? WHERE id = ?`,
		models.ApprovalRequestStatusCancelled, time.Now(), requestID); err != nil {
		return err
	}
	actor := &actorUserID
	if actorUserID == 0 {
		actor = nil
	}
	if _, err := writeDecisionRet(tx, requestID, nil, actor, nil, models.ApprovalDecisionCancel, comment, nil, map[string]any{
		"reason": reason,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if s.eventCoordinator != nil {
		req, err := s.GetRequest(requestID)
		if err == nil && req != nil {
			item, _ := repository.NewItemRepository(s.db).FindByIDWithDetails(req.ItemID)
			if item != nil {
				s.eventCoordinator.EmitApprovalCancelled(req, item, reason, actorUserID)
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

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var si models.ApprovalStepInstance
	err = tx.QueryRow(`
		SELECT id, approval_request_id, approval_step_id, display_order, status,
		       escalation_due_at, escalation_count, last_escalated_at, started_at, completed_at
		FROM approval_step_instances WHERE id = ?
	`, stepInstanceID).Scan(
		&si.ID, &si.ApprovalRequestID, &si.ApprovalStepID, &si.DisplayOrder, &si.Status,
		&si.EscalationDueAt, &si.EscalationCount, &si.LastEscalatedAt, &si.StartedAt, &si.CompletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if si.Status != models.ApprovalStepStatusPending {
		return nil // already finalized
	}

	step, err := s.loadStep(tx, si.ApprovalStepID)
	if err != nil {
		return err
	}

	var req models.ApprovalRequest
	if err := tx.QueryRow(`
		SELECT id, item_id, approval_set_status_id, status_id, triggered_by_user_id, status, created_at, completed_at
		FROM approval_requests WHERE id = ?
	`, si.ApprovalRequestID).Scan(
		&req.ID, &req.ItemID, &req.ApprovalSetStatusID, &req.StatusID,
		&req.TriggeredByUserID, &req.Status, &req.CreatedAt, &req.CompletedAt,
	); err != nil {
		return err
	}
	if req.Status != models.ApprovalRequestStatusPending {
		return nil
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
		newApproverIDs, err := s.escalateReassign(tx, &si, step, &req, actor, reason)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		if s.eventCoordinator != nil {
			out, _ := s.GetRequest(req.ID)
			item, _ := itemRepo.FindByIDWithDetails(req.ItemID)
			if out != nil && item != nil {
				var refreshed *models.ApprovalStepInstance
				for i := range out.StepInstances {
					if out.StepInstances[i].ID == si.ID {
						refreshed = &out.StepInstances[i]
						break
					}
				}
				if refreshed != nil {
					s.eventCoordinator.EmitApprovalEscalated(out, refreshed, action, newApproverIDs, item, actorUserID)
				}
			}
		}
		return nil

	case models.ApprovalEscalationActionSkipStep:
		now := time.Now()
		if _, err := tx.Exec(`
			UPDATE approval_step_instances SET status = ?, completed_at = ?, escalation_due_at = NULL
			WHERE id = ? AND status = 'pending'
		`, models.ApprovalStepStatusEscalated, now, si.ID); err != nil {
			return err
		}
		if _, err := writeDecisionRet(tx, req.ID, &si.ID, actor, nil, models.ApprovalDecisionEscalate, "", nil, map[string]any{
			"reason":     reason,
			"action":     action,
			"resolution": "skip_step",
		}); err != nil {
			return err
		}
		// Treat the step as approved for advancement purposes — same downstream
		// behavior as a real quorum approval.
		newID, err := s.advanceRequestAfterStep(ctx, tx, &req, &si, models.ApprovalStepStatusApproved, actorUserID, itemRepo)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		s.emitEscalationCompletion(req.ID, action, nil, newID, actorUserID)
		return nil

	case models.ApprovalEscalationActionAutoReject:
		now := time.Now()
		if _, err := tx.Exec(`
			UPDATE approval_step_instances SET status = ?, completed_at = ?, escalation_due_at = NULL
			WHERE id = ? AND status = 'pending'
		`, models.ApprovalStepStatusRejected, now, si.ID); err != nil {
			return err
		}
		if _, err := writeDecisionRet(tx, req.ID, &si.ID, actor, nil, models.ApprovalDecisionEscalate, "", nil, map[string]any{
			"reason":     reason,
			"action":     action,
			"resolution": "auto_reject",
		}); err != nil {
			return err
		}
		if _, err := s.advanceRequestAfterStep(ctx, tx, &req, &si, models.ApprovalStepStatusRejected, actorUserID, itemRepo); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		s.emitEscalationCompletion(req.ID, action, nil, nil, actorUserID)
		return nil
	}
	return fmt.Errorf("unknown escalation_action %q", action)
}

// escalateReassign swaps the approver pool to the configured escalation target.
// Returns the list of newly-active approver user IDs.
func (s *ApprovalService) escalateReassign(tx database.Tx, si *models.ApprovalStepInstance, step *models.ApprovalStep, req *models.ApprovalRequest, actor *int, reason string) ([]int, error) {
	if step.EscalationTargetSource == "" {
		return nil, fmt.Errorf("escalation_action=reassign requires escalation_target_source")
	}

	priorPool, err := loadActiveApproverIDs(tx, si.ID)
	if err != nil {
		return nil, err
	}

	// Tombstone the prior pool. is_active flips so the snapshot history is preserved.
	if _, err := tx.Exec(`UPDATE approval_step_approvers SET is_active = 0 WHERE approval_step_instance_id = ? AND is_active = 1`, si.ID); err != nil {
		return nil, fmt.Errorf("deactivate prior pool: %w", err)
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
	// resolveAndSnapshotApprovers writes new approval_step_approvers rows and
	// applies on-leave handling. Skip-on-leave-empty escalation here would be
	// recursive, so we accept whatever the new pool resolves to (possibly
	// empty if both target and substitutes are unavailable).
	if err := s.resolveAndSnapshotApprovers(tx, si.ID, probe, item, req.TriggeredByUserID); err != nil {
		return nil, fmt.Errorf("resolve escalation target: %w", err)
	}

	newPool, err := loadActiveApproverIDs(tx, si.ID)
	if err != nil {
		return nil, err
	}

	// Increment count, update last_escalated_at, re-arm escalation_due_at if
	// the chain isn't capped out.
	now := time.Now()
	newCount := si.EscalationCount + 1
	var newDue sql.NullTime
	if step.EscalationAfterHours != nil {
		if step.MaxEscalations == nil || newCount < *step.MaxEscalations {
			newDue = sql.NullTime{Time: now.Add(time.Duration(*step.EscalationAfterHours) * time.Hour), Valid: true}
		}
	}
	if _, err := tx.Exec(`
		UPDATE approval_step_instances
		SET escalation_count = ?, last_escalated_at = ?, escalation_due_at = ?
		WHERE id = ? AND status = 'pending'
	`, newCount, now, newDue, si.ID); err != nil {
		return nil, fmt.Errorf("update escalation counters: %w", err)
	}
	si.EscalationCount = newCount

	if _, err := writeDecisionRet(tx, req.ID, &si.ID, actor, nil, models.ApprovalDecisionEscalate, "", nil, map[string]any{
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
	// Always emit Escalated as a step-level signal. Find the just-escalated
	// step (highest completed_at, status=escalated|rejected).
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

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stepInstance, err := s.findActiveStepForActor(tx, requestID, actorFromUser(actorUserID))
	if err != nil {
		return err
	}
	if stepInstance == nil {
		return fmt.Errorf("user %d is not an active approver of request %d", actorUserID, requestID)
	}

	// Tombstone actor's row in this step.
	if _, err := tx.Exec(`
		UPDATE approval_step_approvers
		SET is_active = 0
		WHERE approval_step_instance_id = ? AND user_id = ? AND is_active = 1
	`, stepInstance.ID, actorUserID); err != nil {
		return err
	}

	// Insert delegated approver, recording substituted_for_user_id for traceability.
	subOrig := actorUserID
	if _, err := tx.Exec(`
		INSERT INTO approval_step_approvers
			(approval_step_instance_id, user_id, source_role_id, source_group_id, substituted_for_user_id, is_active, created_at)
		VALUES (?, ?, NULL, NULL, ?, 1, ?)
	`, stepInstance.ID, toUserID, subOrig, time.Now()); err != nil {
		return err
	}

	delegated := toUserID
	if _, err := writeDecisionRet(tx, requestID, &stepInstance.ID, &actorUserID, nil,
		models.ApprovalDecisionDelegate, comment, &delegated, map[string]any{
			"from_user_id": actorUserID,
			"to_user_id":   toUserID,
		}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if s.eventCoordinator != nil {
		out, _ := s.GetRequest(requestID)
		if out != nil {
			item, _ := repository.NewItemRepository(s.db).FindByIDWithDetails(out.ItemID)
			if item != nil {
				for i := range out.StepInstances {
					if out.StepInstances[i].ID == stepInstance.ID {
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
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var si models.ApprovalStepInstance
	err = tx.QueryRow(`
		SELECT id, approval_request_id, approval_step_id, display_order, status,
		       escalation_due_at, escalation_count, last_escalated_at, started_at, completed_at
		FROM approval_step_instances WHERE id = ?
	`, stepInstanceID).Scan(
		&si.ID, &si.ApprovalRequestID, &si.ApprovalStepID, &si.DisplayOrder, &si.Status,
		&si.EscalationDueAt, &si.EscalationCount, &si.LastEscalatedAt, &si.StartedAt, &si.CompletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("step instance not found")
	}
	if err != nil {
		return err
	}
	if si.Status != models.ApprovalStepStatusPending {
		return errors.New("step instance is not pending")
	}

	var req models.ApprovalRequest
	if err := tx.QueryRow(`
		SELECT id, item_id, approval_set_status_id, status_id, triggered_by_user_id, status, created_at, completed_at
		FROM approval_requests WHERE id = ?
	`, si.ApprovalRequestID).Scan(
		&req.ID, &req.ItemID, &req.ApprovalSetStatusID, &req.StatusID,
		&req.TriggeredByUserID, &req.Status, &req.CreatedAt, &req.CompletedAt,
	); err != nil {
		return err
	}

	step, err := s.loadStep(tx, si.ApprovalStepID)
	if err != nil {
		return err
	}

	priorPool, err := loadActiveApproverIDs(tx, si.ID)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`UPDATE approval_step_approvers SET is_active = 0 WHERE approval_step_instance_id = ? AND is_active = 1`, si.ID); err != nil {
		return err
	}

	item, err := repository.NewItemRepository(s.db).FindByID(req.ItemID)
	if err != nil {
		return err
	}
	if err := s.resolveAndSnapshotApprovers(tx, si.ID, *step, item, req.TriggeredByUserID); err != nil {
		return err
	}

	newPool, err := loadActiveApproverIDs(tx, si.ID)
	if err != nil {
		return err
	}
	newCustomerPool, err := loadActiveApproverPortalCustomerIDs(tx, si.ID)
	if err != nil {
		return err
	}

	actor := &actorUserID
	if actorUserID == 0 {
		actor = nil
	}
	if _, err := writeDecisionRet(tx, req.ID, &si.ID, actor, nil, models.ApprovalDecisionReassign, comment, nil, map[string]any{
		"reason":             "refresh_approvers",
		"prior_pool":         priorPool,
		"new_pool":           newPool,
		"new_pool_customers": newCustomerPool,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if s.eventCoordinator != nil {
		out, _ := s.GetRequest(req.ID)
		itm, _ := repository.NewItemRepository(s.db).FindByIDWithDetails(req.ItemID)
		if out != nil && itm != nil {
			for i := range out.StepInstances {
				if out.StepInstances[i].ID == si.ID {
					s.eventCoordinator.EmitApprovalStepStarted(out, &out.StepInstances[i], newPool, newCustomerPool, itm, actorUserID)
					break
				}
			}
		}
	}
	return nil
}

// loadActiveApproverIDs returns active approver user IDs for a step instance.
// Portal-customer-only rows (user_id NULL) are skipped — use
// loadActiveApproverPortalCustomerIDs for those.
func loadActiveApproverIDs(tx database.Tx, stepInstanceID int) ([]int, error) {
	rows, err := tx.Query(`
		SELECT user_id FROM approval_step_approvers
		WHERE approval_step_instance_id = ? AND is_active = 1 AND user_id IS NOT NULL ORDER BY user_id
	`, stepInstanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int
	for rows.Next() {
		var uid int
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, nil
}

// loadActiveApproverPortalCustomerIDs is the portal-customer counterpart to
// loadActiveApproverIDs.
func loadActiveApproverPortalCustomerIDs(tx database.Tx, stepInstanceID int) ([]int, error) {
	rows, err := tx.Query(`
		SELECT portal_customer_id FROM approval_step_approvers
		WHERE approval_step_instance_id = ? AND is_active = 1 AND portal_customer_id IS NOT NULL ORDER BY portal_customer_id
	`, stepInstanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int
	for rows.Next() {
		var cid int
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		out = append(out, cid)
	}
	return out, nil
}

// ----------------------------------------------------------------------------
// Gating helpers used by WorkflowService.PerformTransition
// ----------------------------------------------------------------------------

// IsTransitionGatedByApproval returns the pending approval request ID if the
// requested (from→to) transition is the configured approve_transition_id or
// deny_transition_id of an in-flight pending approval on this item. Returns
// nil if not gated.
func (s *ApprovalService) IsTransitionGatedByApproval(itemID, fromStatusID, toStatusID int) (*int, error) {
	var requestID int
	err := s.db.QueryRow(`
		SELECT ar.id
		FROM approval_requests ar
		JOIN approval_set_statuses ass ON ass.id = ar.approval_set_status_id
		JOIN workflow_transitions wt
			ON wt.id IN (ass.approve_transition_id, ass.deny_transition_id)
		WHERE ar.item_id = ? AND ar.status = 'pending'
		  AND wt.from_status_id = ? AND wt.to_status_id = ?
		LIMIT 1
	`, itemID, fromStatusID, toStatusID).Scan(&requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &requestID, nil
}

// MaybeOpenForStatusEntry opens a new approval request iff the (workspace,
// item-type, status) tuple resolves to an approval_set_status. If no approval
// is configured for the destination status, returns (nil, nil) — safe to call
// for every transition.
func (s *ApprovalService) MaybeOpenForStatusEntry(ctx context.Context, itemID, statusID, actorUserID int) (*models.ApprovalRequest, error) {
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
	return s.RequestApproval(ctx, itemID, statusID, actorUserID)
}

// ----------------------------------------------------------------------------
// Listing helpers
// ----------------------------------------------------------------------------

// GetPendingForItem returns the single pending approval request for an item, or nil.
func (s *ApprovalService) GetPendingForItem(itemID int) (*models.ApprovalRequest, error) {
	var id int
	err := s.db.QueryRow(`SELECT id FROM approval_requests WHERE item_id = ? AND status = 'pending'`, itemID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.GetRequest(id)
}

// GetTimelineForItem returns all approval requests for an item, ordered by created_at.
func (s *ApprovalService) GetTimelineForItem(itemID int) ([]*models.ApprovalRequest, error) {
	rows, err := s.db.Query(`SELECT id FROM approval_requests WHERE item_id = ? ORDER BY created_at`, itemID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*models.ApprovalRequest
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
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

// GetForPortalCustomer is the customer-flavored counterpart to GetForUser.
// Returns approval requests where the portal customer is in the active pool.
func (s *ApprovalService) GetForPortalCustomer(customerID int, status string) ([]*models.ApprovalRequest, error) {
	return s.getForActor("portal_customer_id", customerID, status)
}

// getForActor is the polymorphic backbone — column names are hard-coded
// to one of the two known options to avoid SQL injection.
func (s *ApprovalService) getForActor(actorColumn string, actorID int, status string) ([]*models.ApprovalRequest, error) {
	if status == "" {
		status = models.ApprovalRequestStatusPending
	}
	if actorColumn != "user_id" && actorColumn != "portal_customer_id" {
		return nil, fmt.Errorf("invalid actor column %q", actorColumn)
	}
	q := fmt.Sprintf(`
		SELECT DISTINCT ar.id
		FROM approval_requests ar
		JOIN approval_step_instances asi ON asi.approval_request_id = ar.id AND asi.status = 'pending'
		JOIN approval_step_approvers asa ON asa.approval_step_instance_id = asi.id AND asa.is_active = 1
		WHERE ar.status = ? AND asa.%s = ?
		ORDER BY ar.id DESC
	`, actorColumn)
	rows, err := s.db.Query(q, status, actorID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*models.ApprovalRequest
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
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
	var req models.ApprovalRequest
	err := s.db.QueryRow(`
		SELECT id, item_id, approval_set_status_id, status_id, triggered_by_user_id, status, created_at, completed_at
		FROM approval_requests WHERE id = ?
	`, requestID).Scan(
		&req.ID, &req.ItemID, &req.ApprovalSetStatusID, &req.StatusID,
		&req.TriggeredByUserID, &req.Status, &req.CreatedAt, &req.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	// Step instances + approvers
	stepRows, err := s.db.Query(`
		SELECT id, approval_request_id, approval_step_id, display_order, status,
		       escalation_due_at, escalation_count, last_escalated_at, started_at, completed_at
		FROM approval_step_instances WHERE approval_request_id = ? ORDER BY display_order
	`, requestID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stepRows.Close() }()
	for stepRows.Next() {
		var si models.ApprovalStepInstance
		if err := stepRows.Scan(
			&si.ID, &si.ApprovalRequestID, &si.ApprovalStepID, &si.DisplayOrder, &si.Status,
			&si.EscalationDueAt, &si.EscalationCount, &si.LastEscalatedAt, &si.StartedAt, &si.CompletedAt,
		); err != nil {
			return nil, err
		}

		appRows, err := s.db.Query(`
			SELECT id, approval_step_instance_id, user_id, portal_customer_id, source_role_id, source_group_id,
			       substituted_for_user_id, is_active, created_at
			FROM approval_step_approvers WHERE approval_step_instance_id = ? ORDER BY id
		`, si.ID)
		if err != nil {
			return nil, err
		}
		for appRows.Next() {
			var a models.ApprovalStepApprover
			if err := appRows.Scan(&a.ID, &a.ApprovalStepInstanceID, &a.UserID, &a.PortalCustomerID,
				&a.SourceRoleID, &a.SourceGroupID, &a.SubstitutedForUserID, &a.IsActive, &a.CreatedAt); err != nil {
				_ = appRows.Close()
				return nil, err
			}
			si.Approvers = append(si.Approvers, a)
		}
		_ = appRows.Close()

		req.StepInstances = append(req.StepInstances, si)
	}

	// Decisions
	decRows, err := s.db.Query(`
		SELECT id, approval_request_id, approval_step_instance_id, actor_user_id, actor_portal_customer_id,
		       decision, COALESCE(comment, ''), delegated_to_user_id, COALESCE(metadata, ''), created_at
		FROM approval_decisions WHERE approval_request_id = ? ORDER BY created_at, id
	`, requestID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = decRows.Close() }()
	for decRows.Next() {
		var d models.ApprovalDecision
		var metadata string
		if err := decRows.Scan(
			&d.ID, &d.ApprovalRequestID, &d.ApprovalStepInstanceID, &d.ActorUserID, &d.ActorPortalCustomerID,
			&d.Decision, &d.Comment, &d.DelegatedToUserID, &metadata, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		if metadata != "" {
			d.Metadata = json.RawMessage(metadata)
		}
		req.Decisions = append(req.Decisions, d)
	}
	return &req, nil
}

// ----------------------------------------------------------------------------
// Internals
// ----------------------------------------------------------------------------

func (s *ApprovalService) loadSteps(approvalSetStatusID int) ([]models.ApprovalStep, error) {
	rows, err := s.db.Query(`
		SELECT id, approval_set_status_id, display_order, name,
		       quorum_mode, quorum_count, quorum_percent, rejection_policy,
		       approver_source, approver_field_identifier, approver_field_id,
		       approver_role_id, approver_group_id, approver_user_id, allow_self_approval,
		       on_leave_strategy,
		       escalation_after_hours, escalation_action, escalation_target_source,
		       escalation_target_field_identifier, escalation_target_field_id,
		       escalation_target_role_id, escalation_target_group_id, escalation_target_user_id,
		       max_escalations, created_at
		FROM approval_steps WHERE approval_set_status_id = ? ORDER BY display_order, id
	`, approvalSetStatusID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []models.ApprovalStep
	for rows.Next() {
		var s models.ApprovalStep
		var allowSelf int
		var fieldIdent, action, escTargSrc, escFieldIdent sql.NullString
		if err := rows.Scan(
			&s.ID, &s.ApprovalSetStatusID, &s.DisplayOrder, &s.Name,
			&s.QuorumMode, &s.QuorumCount, &s.QuorumPercent, &s.RejectionPolicy,
			&s.ApproverSource, &fieldIdent, &s.ApproverFieldID,
			&s.ApproverRoleID, &s.ApproverGroupID, &s.ApproverUserID, &allowSelf,
			&s.OnLeaveStrategy,
			&s.EscalationAfterHours, &action, &escTargSrc,
			&escFieldIdent, &s.EscalationTargetFieldID,
			&s.EscalationTargetRoleID, &s.EscalationTargetGroupID, &s.EscalationTargetUserID,
			&s.MaxEscalations, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		s.AllowSelfApproval = allowSelf != 0
		s.ApproverFieldIdentifier = fieldIdent.String
		s.EscalationAction = action.String
		s.EscalationTargetSource = escTargSrc.String
		s.EscalationTargetFieldIdentifier = escFieldIdent.String
		out = append(out, s)
	}
	return out, nil
}

func (s *ApprovalService) loadStep(tx database.Tx, stepID int) (*models.ApprovalStep, error) {
	var step models.ApprovalStep
	var allowSelf int
	var fieldIdent, action, escTargSrc, escFieldIdent sql.NullString
	err := tx.QueryRow(`
		SELECT id, approval_set_status_id, display_order, name,
		       quorum_mode, quorum_count, quorum_percent, rejection_policy,
		       approver_source, approver_field_identifier, approver_field_id,
		       approver_role_id, approver_group_id, approver_user_id, allow_self_approval,
		       on_leave_strategy,
		       escalation_after_hours, escalation_action, escalation_target_source,
		       escalation_target_field_identifier, escalation_target_field_id,
		       escalation_target_role_id, escalation_target_group_id, escalation_target_user_id,
		       max_escalations, created_at
		FROM approval_steps WHERE id = ?
	`, stepID).Scan(
		&step.ID, &step.ApprovalSetStatusID, &step.DisplayOrder, &step.Name,
		&step.QuorumMode, &step.QuorumCount, &step.QuorumPercent, &step.RejectionPolicy,
		&step.ApproverSource, &fieldIdent, &step.ApproverFieldID,
		&step.ApproverRoleID, &step.ApproverGroupID, &step.ApproverUserID, &allowSelf,
		&step.OnLeaveStrategy,
		&step.EscalationAfterHours, &action, &escTargSrc,
		&escFieldIdent, &step.EscalationTargetFieldID,
		&step.EscalationTargetRoleID, &step.EscalationTargetGroupID, &step.EscalationTargetUserID,
		&step.MaxEscalations, &step.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	step.AllowSelfApproval = allowSelf != 0
	step.ApproverFieldIdentifier = fieldIdent.String
	step.EscalationAction = action.String
	step.EscalationTargetSource = escTargSrc.String
	step.EscalationTargetFieldIdentifier = escFieldIdent.String
	return &step, nil
}

func (s *ApprovalService) loadApprovalSetStatus(tx database.Tx, id int) (*models.ApprovalSetStatus, error) {
	var ass models.ApprovalSetStatus
	err := tx.QueryRow(`
		SELECT id, approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode, created_at
		FROM approval_set_statuses WHERE id = ?
	`, id).Scan(&ass.ID, &ass.ApprovalSetID, &ass.StatusID,
		&ass.ApproveTransitionID, &ass.DenyTransitionID, &ass.StepMode, &ass.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ass, nil
}

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
func (s *ApprovalService) findActiveStepForActor(tx database.Tx, requestID int, actor approvalActor) (*models.ApprovalStepInstance, error) {
	if !actor.isSet() {
		return nil, nil
	}
	var si models.ApprovalStepInstance
	var err error
	if actor.UserID != nil {
		err = tx.QueryRow(`
			SELECT asi.id, asi.approval_request_id, asi.approval_step_id, asi.display_order, asi.status,
			       asi.escalation_due_at, asi.escalation_count, asi.last_escalated_at, asi.started_at, asi.completed_at
			FROM approval_step_instances asi
			JOIN approval_step_approvers asa ON asa.approval_step_instance_id = asi.id AND asa.is_active = 1 AND asa.user_id = ?
			WHERE asi.approval_request_id = ? AND asi.status = 'pending'
			ORDER BY asi.display_order
			LIMIT 1
		`, *actor.UserID, requestID).Scan(
			&si.ID, &si.ApprovalRequestID, &si.ApprovalStepID, &si.DisplayOrder, &si.Status,
			&si.EscalationDueAt, &si.EscalationCount, &si.LastEscalatedAt, &si.StartedAt, &si.CompletedAt,
		)
	} else {
		err = tx.QueryRow(`
			SELECT asi.id, asi.approval_request_id, asi.approval_step_id, asi.display_order, asi.status,
			       asi.escalation_due_at, asi.escalation_count, asi.last_escalated_at, asi.started_at, asi.completed_at
			FROM approval_step_instances asi
			JOIN approval_step_approvers asa ON asa.approval_step_instance_id = asi.id AND asa.is_active = 1 AND asa.portal_customer_id = ?
			WHERE asi.approval_request_id = ? AND asi.status = 'pending'
			ORDER BY asi.display_order
			LIMIT 1
		`, *actor.PortalCustomerID, requestID).Scan(
			&si.ID, &si.ApprovalRequestID, &si.ApprovalStepID, &si.DisplayOrder, &si.Status,
			&si.EscalationDueAt, &si.EscalationCount, &si.LastEscalatedAt, &si.StartedAt, &si.CompletedAt,
		)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &si, nil
}

// evaluateStepStatus returns the new step status based on votes vs quorum.
// The current status (passed implicitly via the step instance) is not consulted —
// caller compares to the prior status to decide whether to write an UPDATE.
func (s *ApprovalService) evaluateStepStatus(tx database.Tx, stepInstanceID int, step *models.ApprovalStep) (string, error) {
	// Pool size = active approvers.
	var poolSize int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM approval_step_approvers WHERE approval_step_instance_id = ? AND is_active = 1`, stepInstanceID).Scan(&poolSize); err != nil {
		return "", err
	}
	if poolSize == 0 {
		// No eligible approvers — slice 9 will escalate. For now, leave pending.
		return models.ApprovalStepStatusPending, nil
	}

	var approves, rejects int
	if err := tx.QueryRow(`
		SELECT COALESCE(SUM(CASE WHEN decision = 'approve' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN decision = 'reject'  THEN 1 ELSE 0 END), 0)
		FROM approval_decisions WHERE approval_step_instance_id = ?
	`, stepInstanceID).Scan(&approves, &rejects); err != nil {
		return "", err
	}

	// Rejection short-circuit.
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

	// Approval threshold.
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
		// Round up.
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
func (s *ApprovalService) resolveAndSnapshotApprovers(tx database.Tx, stepInstanceID int, step models.ApprovalStep, item *models.Item, triggeredByUserID int) error {
	rawUsers, err := s.resolveApproverSource(tx, step, item, triggeredByUserID)
	if err != nil {
		return err
	}

	finalPool := make([]resolvedApprover, 0, len(rawUsers))
	for _, ra := range rawUsers {
		// On-leave handling: only applies to internal users. Portal customers
		// don't have UserLeavePeriod records in v1 — they pass through unchanged
		// and admins should pick on_leave_strategy='keep' for steps that may
		// resolve to customers.
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
						// Audit row for the substitution.
						if _, err := writeDecisionRet(tx, requestIDForStep(tx, stepInstanceID), &stepInstanceID, nil, nil,
							models.ApprovalDecisionSubstitute, "", nil, map[string]any{
								"original_user_id":   subOrig,
								"substitute_user_id": sub,
								"reason":             "active_leave",
							}); err != nil {
							return err
						}
						continue
					}
					// No substitute → drop (slice 9 will escalate when pool is empty).
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

	// Insert snapshot rows. Exactly one of user_id / portal_customer_id is
	// non-NULL per row, satisfying the CHECK constraint.
	for _, ra := range finalPool {
		var userID, portalCustomerID interface{}
		if ra.isCustomer() {
			portalCustomerID = ra.PortalCustomerID
		} else {
			userID = ra.UserID
		}
		if _, err := tx.Exec(`
			INSERT INTO approval_step_approvers (approval_step_instance_id, user_id, portal_customer_id, source_role_id, source_group_id, substituted_for_user_id, is_active, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, ?)
		`, stepInstanceID, userID, portalCustomerID, ra.SourceRoleID, ra.SourceGroupID, ra.SubstitutedForUserID, time.Now()); err != nil {
			return fmt.Errorf("insert approver: %w", err)
		}
	}
	return nil
}

// resolveApproverSource turns the configured source into a list of user IDs
// (with provenance metadata). Custom-field user IDs are read from item.CustomFieldValues.
func (s *ApprovalService) resolveApproverSource(tx database.Tx, step models.ApprovalStep, item *models.Item, triggeredByUserID int) ([]resolvedApprover, error) {
	switch step.ApproverSource {
	case models.ApprovalSourceCreator:
		// Polymorphic: an item can be created by either an internal user OR
		// a portal customer. Prefer the portal customer when set — that's the
		// "customer reviews their own request" flow. Fall back to creator_id
		// for items created by internal users.
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
		// All whitelisted fields are user-id columns on items.
		query := fmt.Sprintf(`SELECT %s FROM items WHERE id = ?`, step.ApproverFieldIdentifier)
		if err := tx.QueryRow(query, item.ID).Scan(&userID); err != nil {
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
		// Resolve users with this workspace role, both directly via user_workspace_roles
		// and indirectly via group_workspace_roles + group_members.
		rows, err := tx.Query(`
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
		return out, nil

	case models.ApprovalSourceGroup:
		if step.ApproverGroupID == nil {
			return nil, errors.New("group source requires approver_group_id")
		}
		rows, err := tx.Query(`SELECT user_id FROM group_members WHERE group_id = ?`, *step.ApproverGroupID)
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
		// Some custom fields stringify ids; tolerate.
		var n int
		_, err := fmt.Sscanf(strings.TrimSpace(val), "%d", &n)
		if err == nil && n > 0 {
			return []resolvedApprover{{UserID: n}}
		}
	}
	return nil
}

// requestIDForStep is a helper used during snapshotting to look up the parent
// request id for an audit row when it isn't in scope as a variable.
func requestIDForStep(tx database.Tx, stepInstanceID int) int {
	var id int
	_ = tx.QueryRow(`SELECT approval_request_id FROM approval_step_instances WHERE id = ?`, stepInstanceID).Scan(&id)
	return id
}

// writeDecision writes an audit row inside a tx and ignores the inserted id.
// Pass nil for both actor params when the actor is the system (e.g. sweeper).
func writeDecision(tx database.Tx, requestID int, stepInstanceID, actorUserID, actorPortalCustomerID *int, decision, comment string, delegatedToUserID *int, metadata map[string]any) error {
	_, err := writeDecisionRet(tx, requestID, stepInstanceID, actorUserID, actorPortalCustomerID, decision, comment, delegatedToUserID, metadata)
	return err
}

// writeDecisionRet inserts a row into approval_decisions. Exactly one (or
// neither, for system actors) of actorUserID / actorPortalCustomerID is set.
func writeDecisionRet(tx database.Tx, requestID int, stepInstanceID, actorUserID, actorPortalCustomerID *int, decision, comment string, delegatedToUserID *int, metadata map[string]any) (*models.ApprovalDecision, error) {
	var metaJSON []byte
	if metadata != nil {
		var err error
		metaJSON, err = json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal decision metadata: %w", err)
		}
	}
	res, err := tx.Exec(`
		INSERT INTO approval_decisions
			(approval_request_id, approval_step_instance_id, actor_user_id, actor_portal_customer_id, decision, comment, delegated_to_user_id, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, requestID, stepInstanceID, actorUserID, actorPortalCustomerID, decision, comment, delegatedToUserID, string(metaJSON), time.Now())
	if err != nil {
		return nil, fmt.Errorf("insert decision: %w", err)
	}
	id64, _ := res.LastInsertId()
	d := &models.ApprovalDecision{
		ID:                     int(id64),
		ApprovalRequestID:      requestID,
		ApprovalStepInstanceID: stepInstanceID,
		ActorUserID:            actorUserID,
		ActorPortalCustomerID:  actorPortalCustomerID,
		Decision:               decision,
		Comment:                comment,
		DelegatedToUserID:      delegatedToUserID,
		CreatedAt:              time.Now(),
	}
	if metaJSON != nil {
		d.Metadata = json.RawMessage(metaJSON)
	}
	return d, nil
}
