package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/constants"
	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

// initialStatusCacheEntry holds a cached initial status ID with expiry
type initialStatusCacheEntry struct {
	statusID  *int
	expiresAt time.Time
}

const initialStatusCacheTTL = 5 * time.Minute

// initialStatusSweepInterval is the minimum gap between opportunistic sweeps
// of expired initialStatusCache entries. We don't run a background goroutine
// for this — WorkflowService is constructed both per-request and at server
// scope, and a per-request goroutine would itself leak. Amortizing the sweep
// across cache-miss calls keeps the long-lived (server-scoped) instance
// bounded without adding lifecycle plumbing on the short-lived ones.
const initialStatusSweepInterval = time.Minute

// WorkflowService provides centralized workflow lookup logic with proper fallback chain
type WorkflowService struct {
	db                 database.Database
	initialStatusCache sync.Map // key: string "ws:{id}:it:{id|nil}" → value: *initialStatusCacheEntry
	// lastSweepUnixNano is the most recent time evictExpired ran, stored as
	// time.Time.UnixNano. Atomic so concurrent callers can throttle the
	// sweep without taking a lock.
	lastSweepUnixNano atomic.Int64
}

// StatusTransitionOption is a status option exposed by a workflow transition.
type StatusTransitionOption = repository.StatusTransitionOption

// NewWorkflowService creates a new workflow service
func NewWorkflowService(db database.Database) *WorkflowService {
	return &WorkflowService{db: db}
}

// GetInitialStatusIDCached returns the initial status ID for a workspace+itemType,
// using an in-memory cache to avoid repeated DB lookups.
func (s *WorkflowService) GetInitialStatusIDCached(workspaceID int, itemTypeID *int) (*int, error) {
	key := initialStatusCacheKey(workspaceID, itemTypeID)

	// Check cache
	if val, ok := s.initialStatusCache.Load(key); ok {
		entry, ok := val.(*initialStatusCacheEntry)
		if !ok {
			return nil, fmt.Errorf("invalid cache entry type")
		}
		if time.Now().Before(entry.expiresAt) {
			return entry.statusID, nil
		}
		// Expired, delete and fall through
		s.initialStatusCache.Delete(key)
	}

	// Cache miss: sweep any other expired entries before paying for the DB
	// lookup. Throttled to one sweep per initialStatusSweepInterval across
	// callers; for short-lived per-request instances this is a no-op.
	s.maybeSweepInitialStatusCache(time.Now())

	// Cache miss: resolve via DB
	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}

	var statusID *int
	if workflowID != nil {
		statusID, err = s.GetInitialStatusID(*workflowID)
		if err != nil {
			return nil, err
		}
	}

	// Store in cache
	s.initialStatusCache.Store(key, &initialStatusCacheEntry{
		statusID:  statusID,
		expiresAt: time.Now().Add(initialStatusCacheTTL),
	})

	return statusID, nil
}

// InvalidateInitialStatusCache clears the initial status cache.
// Call this when workflow configuration changes.
func (s *WorkflowService) InvalidateInitialStatusCache() {
	s.initialStatusCache.Range(func(key, _ any) bool {
		s.initialStatusCache.Delete(key)
		return true
	})
}

// maybeSweepInitialStatusCache deletes expired entries from initialStatusCache
// if at least initialStatusSweepInterval has elapsed since the last sweep.
// Uses an atomic CAS so concurrent callers don't double-sweep. now is passed
// in so tests can drive the throttle deterministically.
func (s *WorkflowService) maybeSweepInitialStatusCache(now time.Time) {
	prev := s.lastSweepUnixNano.Load()
	nowNs := now.UnixNano()
	if prev != 0 && nowNs-prev < int64(initialStatusSweepInterval) {
		return
	}
	if !s.lastSweepUnixNano.CompareAndSwap(prev, nowNs) {
		return
	}
	s.initialStatusCache.Range(func(key, val any) bool {
		entry, ok := val.(*initialStatusCacheEntry)
		if !ok || now.After(entry.expiresAt) {
			s.initialStatusCache.Delete(key)
		}
		return true
	})
}

func initialStatusCacheKey(workspaceID int, itemTypeID *int) string {
	if itemTypeID != nil {
		return fmt.Sprintf("ws:%d:it:%d", workspaceID, *itemTypeID)
	}
	return fmt.Sprintf("ws:%d:it:nil", workspaceID)
}

// GetWorkflowIDForItem returns workflow ID with proper fallback chain:
// 1. Item type-specific override (configuration_set_item_types.workflow_id) - can be NULL
// 2. Config set default (configuration_sets.workflow_id) - can be NULL
// 3. Global default workflow (workflows.is_default = true) - final fallback
//
// Returns nil only if NO workflow exists at any level
func (s *WorkflowService) GetWorkflowIDForItem(workspaceID int, itemTypeID *int) (*int, error) {
	resolved, err := repository.NewConfigurationSetRepository(s.db).ResolveForWorkspace(context.Background(), workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}
	if resolved != nil && resolved.IsPersonal {
		return nil, nil
	}
	if resolved != nil && resolved.WorkflowID != nil {
		return resolved.WorkflowID, nil
	}

	// Final fallback: global default workflow
	var defaultID int
	err = s.db.QueryRow(`SELECT id FROM workflows WHERE is_default = true LIMIT 1`).Scan(&defaultID)
	if err == nil {
		return &defaultID, nil
	}

	// No workflow configured anywhere
	return nil, nil
}

// GetDefaultWorkflowID returns the global default workflow ID.
func (s *WorkflowService) GetDefaultWorkflowID() (*int, error) {
	var defaultID int
	err := s.db.QueryRow(`SELECT id FROM workflows WHERE is_default = true LIMIT 1`).Scan(&defaultID)
	if err != nil {
		return nil, err
	}
	return &defaultID, nil
}

// GetStatusName returns a status name. Missing statuses return an empty name
// for compatibility with the previous transition-list handler behavior.
func (s *WorkflowService) GetStatusName(statusID int64) (string, error) {
	return repository.NewStatusRepository(s.db).GetName(statusID)
}

// GetStatusTransitionOption returns display metadata for a status.
func (s *WorkflowService) GetStatusTransitionOption(statusID int64) (*StatusTransitionOption, error) {
	return repository.NewStatusRepository(s.db).GetTransitionOption(statusID)
}

// ListAvailableTransitionOptions returns the direct workflow transitions from a status.
func (s *WorkflowService) ListAvailableTransitionOptions(workflowID int, fromStatusID int64) ([]StatusTransitionOption, error) {
	return repository.NewStatusRepository(s.db).ListAvailableTransitionOptions(workflowID, fromStatusID)
}

// IsValidStatusTransition checks if a status transition is allowed by the workflow
// Uses the full fallback chain to determine the correct workflow
func (s *WorkflowService) IsValidStatusTransition(workspaceID int, itemTypeID *int, fromStatusID, toStatusID int64) (bool, error) {
	// Same status is always valid
	if fromStatusID == toStatusID {
		return true, nil
	}

	// Get the workflow using proper fallback chain
	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return false, err
	}

	// No workflow configured - allow any transition
	if workflowID == nil {
		return true, nil
	}

	// Check if the transition exists in the workflow: either a direct edge or a
	// from-all row that admits every source status.
	var exists bool
	err = s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM workflow_transitions
			WHERE workflow_id = ?
			  AND to_status_id = ?
			  AND (from_status_id = ? OR from_all_statuses = TRUE)
		)
	`, *workflowID, toStatusID, fromStatusID).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("failed to check transition: %w", err)
	}

	return exists, nil
}

// ValidateCreateStatusOverride checks whether a user/API create request may set
// status_id instead of letting creation use the workflow initial status. This is
// intentionally stricter than a normal status transition: create-time placement
// may target the initial status itself, or a direct transition from the initial
// status, but it must not bypass condition/validator rules or approval gates.
func (s *WorkflowService) ValidateCreateStatusOverride(ctx context.Context, workspaceID int, itemTypeID *int, requestedStatusID int) error {
	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return err
	}
	// Workspaces without a workflow keep the legacy behavior.
	if workflowID == nil {
		return nil
	}

	initialStatusID, err := s.GetInitialStatusID(*workflowID)
	if err != nil {
		return err
	}
	if initialStatusID == nil {
		return &TransitionRejection{Code: "workflow_invalid", Message: "workflow has no initial status"}
	}
	if requestedStatusID == *initialStatusID {
		return nil
	}

	var transitionID int
	err = s.db.QueryRow(`
		SELECT id FROM workflow_transitions
		WHERE workflow_id = ? AND from_status_id = ? AND to_status_id = ?
	`, *workflowID, *initialStatusID, requestedStatusID).Scan(&transitionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &TransitionRejection{Code: "workflow_invalid", Message: "requested status is not reachable from the workflow initial status"}
		}
		return fmt.Errorf("check create status transition: %w", err)
	}

	conditionSetID, err := NewConditionService(s.db, nil, nil).GetConditionSetIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return err
	}
	if conditionSetID != nil {
		var conditionCount int
		err = s.db.QueryRow(`
			SELECT COUNT(*)
			FROM condition_set_transitions cst
			JOIN conditions c ON c.condition_set_transition_id = cst.id
			WHERE cst.condition_set_id = ?
			  AND cst.transition_id = ?
			  AND c.mode IN ('condition', 'validator')
		`, *conditionSetID, transitionID).Scan(&conditionCount)
		if err != nil {
			return fmt.Errorf("check create status conditions: %w", err)
		}
		if conditionCount > 0 {
			return &TransitionRejection{Code: "condition_blocked", Message: "requested status is gated by workflow conditions or validators"}
		}
	}

	approvalService := NewApprovalService(s.db, nil, s)
	approvalSetStatus, err := approvalService.GetApprovalSetStatusForItem(ctx, workspaceID, itemTypeID, requestedStatusID)
	if err != nil {
		return err
	}
	if approvalSetStatus != nil {
		return &TransitionRejection{Code: "approval_pending", Message: "requested status requires approval and cannot be set during creation"}
	}

	approvalSetID, err := approvalService.GetApprovalSetIDForItem(ctx, workspaceID, itemTypeID)
	if err != nil {
		return err
	}
	if approvalSetID != nil {
		var approvalTransitionCount int
		err = s.db.QueryRow(`
			SELECT COUNT(*)
			FROM approval_set_statuses
			WHERE approval_set_id = ?
			  AND is_active = true
			  AND (approve_transition_id = ? OR deny_transition_id = ?)
		`, *approvalSetID, transitionID, transitionID).Scan(&approvalTransitionCount)
		if err != nil {
			return fmt.Errorf("check create status approval transitions: %w", err)
		}
		if approvalTransitionCount > 0 {
			return &TransitionRejection{Code: "approval_must_decide", Message: "requested status uses an approval decision transition and cannot be set during creation"}
		}
	}

	return nil
}

// IsValidStatusTransitionForUser checks if a transition is allowed by workflow AND conditions.
// conditionService may be nil (in which case only workflow rules are checked).
// Only conditions whose mode is in `modes` are enforced — callers pass
// []string{"validator"} for write paths that historically only hard-block
// validator-mode, and []string{"validator", "condition"} for the dedicated
// transition endpoint where condition-mode also blocks.
// Returns (allowed, failureMessage, error). failureMessage is set when a condition with an
// error_message fails.
func (s *WorkflowService) IsValidStatusTransitionForUser(ctx context.Context, workspaceID int, itemTypeID *int, fromStatusID, toStatusID int64, userID int, item map[string]any, conditionService *ConditionService, modes []string) (allowed bool, failureMessage string, err error) {
	// Same status is always valid
	if fromStatusID == toStatusID {
		return true, "", nil
	}

	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return false, "", err
	}
	if workflowID == nil {
		return true, "", nil
	}

	// Resolve the governing transition row: a direct edge wins over a from-all
	// row so a specific edge keeps its own conditions and validators.
	var transitionID int
	err = s.db.QueryRow(`
		SELECT id FROM workflow_transitions
		WHERE workflow_id = ?
		  AND to_status_id = ?
		  AND (from_status_id = ? OR from_all_statuses = TRUE)
		ORDER BY CASE WHEN from_status_id IS NULL THEN 1 ELSE 0 END, display_order
		LIMIT 1
	`, *workflowID, toStatusID, fromStatusID).Scan(&transitionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("failed to check transition: %w", err)
	}

	// If no condition service, just check workflow validity
	if conditionService == nil {
		return true, "", nil
	}

	// Get condition set for this workspace/item type
	conditionSetID, err := conditionService.GetConditionSetIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return false, "", err
	}
	if conditionSetID == nil {
		return true, "", nil
	}

	return conditionService.EvaluateTransitionConditions(ctx, *conditionSetID, transitionID, userID, item, modes)
}

// PerformTransitionRequest is the input to WorkflowService.PerformTransition.
type PerformTransitionRequest struct {
	ItemID        int
	ToStatusID    int
	ActorUserID   int
	EventMetadata itemevents.Metadata
	// Modes selects which condition modes to enforce (e.g. []string{"validator", "condition"}
	// for user-initiated transitions, or nil/empty for automation that should only be
	// gated by workflow validity). Callers pass this through to
	// ConditionService.EvaluateTransitionConditions.
	Modes []string
}

// PerformTransitionResult is returned once a transition commits, including
// when a post-commit approval hook also returns an error.
type PerformTransitionResult struct {
	Item        *models.Item
	OldStatusID *int
	NewStatusID *int
	NoOp        bool
}

type transitionApprovalService interface {
	IsTransitionGatedByApproval(ctx context.Context, itemID, fromStatusID, toStatusID int) (*int, error)
	GetPendingForItem(ctx context.Context, itemID int) (*models.ApprovalRequest, error)
	Cancel(ctx context.Context, requestID, actorUserID int, comment, reason string) error
	MaybeOpenForStatusEntry(ctx context.Context, itemID, statusID, fromStatusID, actorUserID int) (*models.ApprovalRequest, error)
}

// TransitionRejection is returned as an error when a transition is rejected
// by workflow rules, conditions, or approvals. Callers use errors.As to
// distinguish rejections (→ HTTP 4xx) from internal errors (→ HTTP 500).
//
// Codes:
//   - "no_current_status"     — item has no status yet; cannot transition (HTTP 400)
//   - "workflow_invalid"      — transition is not in the workflow graph (HTTP 400)
//   - "condition_blocked"     — a validator/condition rejected the transition (HTTP 400)
//   - "approval_must_decide"  — transition is gated by an in-flight approval (HTTP 409)
//   - "approval_pending"      — another approval is in flight on this item (HTTP 409)
//   - "approval_rejected"     — approval finalized as rejected (HTTP 409)
//
// Details is an optional structured payload (e.g. {"approval_request_id": 42})
// surfaced to the HTTP response so the UI can deep-link to the approval.
type TransitionRejection struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *TransitionRejection) Error() string { return e.Message }

// PerformTransition is the single entry point for workflow status transitions.
// It validates the transition (workflow graph + selected condition modes +
// approval gating), writes the status change transactionally, records a
// history entry, and returns the updated item. Callers are responsible for
// emitting events (EventCoordinator for user-initiated, cascade event for
// action executors).
//
// approvalService may be nil. When non-nil:
//   - Direct user attempts at the configured approve/deny transitions of an
//     in-flight pending approval are rejected with code "approval_must_decide".
//   - After commit, if the destination status is approval-bound, a fresh
//     approval request is opened.
//   - After commit, if the source status had a pending approval request, it is
//     canceled with reason "left_status".
func (s *WorkflowService) PerformTransition(
	ctx context.Context,
	req PerformTransitionRequest,
	itemRepo *repository.ItemRepository,
	conditionService *ConditionService,
	approvalService transitionApprovalService,
) (*PerformTransitionResult, error) {
	item, err := itemRepo.FindByID(req.ItemID)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateTaskState(s.db, item.WorkspaceID, req.ActorUserID, item.IsTask, &req.ToStatusID); err != nil {
		return nil, err
	}

	missingStatus := item.StatusID == nil
	oldStatusID := constants.StatusIDOpen
	if missingStatus {
		isPersonal, err := repository.IsPersonalWorkspace(s.db, item.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("resolve item workspace: %w", err)
		}
		if !isPersonal {
			return nil, &TransitionRejection{
				Code:    "no_current_status",
				Message: "item has no current status; cannot transition",
			}
		}
	} else {
		oldStatusID = *item.StatusID
	}

	// No-op: target equals current status.
	if !missingStatus && oldStatusID == req.ToStatusID {
		return &PerformTransitionResult{
			Item:        item,
			OldStatusID: &oldStatusID,
			NewStatusID: &oldStatusID,
			NoOp:        true,
		}, nil
	}

	currentStatusSQL := sql.NullInt64{Int64: int64(oldStatusID), Valid: true}
	itemTypeSQL := sql.NullInt64{}
	if item.ItemTypeID != nil {
		itemTypeSQL = sql.NullInt64{Int64: int64(*item.ItemTypeID), Valid: true}
	}
	var itemTypeIDPtr *int
	if item.ItemTypeID != nil {
		v := *item.ItemTypeID
		itemTypeIDPtr = &v
	}

	itemCtx := BuildItemContext(s.db, req.ItemID, item.WorkspaceID, currentStatusSQL, itemTypeSQL)
	valid, failureMsg, err := s.IsValidStatusTransitionForUser(
		ctx, item.WorkspaceID, itemTypeIDPtr,
		int64(oldStatusID), int64(req.ToStatusID),
		req.ActorUserID, itemCtx, conditionService,
		req.Modes,
	)
	if err != nil {
		return nil, err
	}
	if !valid {
		msg := failureMsg
		code := "workflow_invalid"
		if msg != "" {
			code = "condition_blocked"
		} else {
			msg = "transition not allowed by workflow"
		}
		return nil, &TransitionRejection{Code: code, Message: msg}
	}

	// Approval gating: refuse direct user invocation of a gated approve/deny
	// transition. ApprovalService.Decide is the only legitimate path to those
	// commits and bypasses PerformTransition entirely.
	if approvalService != nil {
		gatingRequestID, err := approvalService.IsTransitionGatedByApproval(ctx, req.ItemID, oldStatusID, req.ToStatusID)
		if err != nil {
			return nil, fmt.Errorf("check approval gating: %w", err)
		}
		if gatingRequestID != nil {
			return nil, &TransitionRejection{
				Code:    "approval_must_decide",
				Message: "this transition is driven by an in-flight approval; decide the approval instead",
				Details: map[string]any{"approval_request_id": *gatingRequestID},
			}
		}
	}

	// Transactional write + history entry.
	if err := database.WithTx(s.db, func(tx database.Tx) error {
		metadata := mergeItemEventMetadata(req.EventMetadata, itemEventMetadata(req.ActorUserID, "application", nil))
		return s.CommitTransition(ctx, tx, itemRepo, req.ItemID, oldStatusID, req.ToStatusID, req.ActorUserID, metadata)
	}); err != nil {
		return nil, err
	}

	// Live-update publish (WI-483): the status transition committed. Reached only
	// on a real transition (the no-op case short-circuits earlier).
	PublishItemChange(req.ItemID, ItemChangeStatus)

	updated, err := itemRepo.FindByIDWithDetails(req.ItemID)
	if err != nil {
		return nil, fmt.Errorf("reload item: %w", err)
	}

	newStatusID := req.ToStatusID
	result := &PerformTransitionResult{
		Item:        updated,
		OldStatusID: &oldStatusID,
		NewStatusID: &newStatusID,
		NoOp:        false,
	}

	// Post-commit approval hooks. Cancel any pending approval (we just left the
	// approval-bound status by a non-gated transition), then open a new one if
	// the destination status is itself approval-bound.
	if approvalService != nil {
		pending, err := approvalService.GetPendingForItem(ctx, req.ItemID)
		if err != nil {
			logPostCommitApprovalError(req, oldStatusID, "get_pending", err)
			return result, fmt.Errorf("get pending approval after commit: %w", err)
		}
		if pending != nil {
			if err := approvalService.Cancel(ctx, pending.ID, req.ActorUserID, "", "left_status"); err != nil {
				logPostCommitApprovalError(req, oldStatusID, "cancel", err)
				return result, fmt.Errorf("cancel approval after commit: %w", err)
			}
		}
		if _, err := approvalService.MaybeOpenForStatusEntry(ctx, req.ItemID, req.ToStatusID, oldStatusID, req.ActorUserID); err != nil {
			logPostCommitApprovalError(req, oldStatusID, "maybe_open", err)
			return result, fmt.Errorf("open approval after commit: %w", err)
		}
	}

	return result, nil
}

func logPostCommitApprovalError(req PerformTransitionRequest, oldStatusID int, hook string, err error) {
	slog.Error("post-commit approval hook failed",
		slog.String("component", "workflow"),
		slog.String("hook", hook),
		slog.Int("item_id", req.ItemID),
		slog.Int("old_status_id", oldStatusID),
		slog.Int("new_status_id", req.ToStatusID),
		slog.Int("actor_user_id", req.ActorUserID),
		slog.Any("error", err),
	)
}

// CommitTransition writes the status change and a corresponding item_history row
// inside the caller-owned transaction. It does NOT re-run workflow validity,
// conditions, validators, or approvals — the caller is responsible for those
// gates. ApprovalService.Decide calls this directly when an approval finalizes,
// because the approval itself is the gate; PerformTransition calls it after its
// own gating logic for user-driven transitions.
func (s *WorkflowService) CommitTransition(
	ctx context.Context, tx database.Tx, itemRepo *repository.ItemRepository,
	itemID, oldStatusID, newStatusID, actorUserID int,
	metadata itemevents.Metadata,
) error {
	item, err := itemRepo.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return err
	}
	if err := validation.ValidateTaskState(tx, item.WorkspaceID, actorUserID, item.IsTask, &newStatusID); err != nil {
		return err
	}
	changedAt := time.Now()
	if err := itemRepo.UpdateFields(tx, itemID, map[string]any{
		"status_id": newStatusID,
	}); err != nil {
		return err
	}

	if err := itemRepo.RecordHistory(tx, repository.HistoryEntry{
		ItemID:    itemID,
		UserID:    actorUserID,
		FieldName: "status_id",
		OldValue:  fmt.Sprintf("%d", oldStatusID),
		NewValue:  fmt.Sprintf("%d", newStatusID),
		ChangedAt: changedAt,
	}); err != nil {
		return fmt.Errorf("record transition history: %w", err)
	}
	item, err = itemRepo.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return err
	}
	if metadata.OccurredAt.IsZero() {
		metadata.OccurredAt = changedAt
	}
	oldStatus, newStatus := oldStatusID, newStatusID
	changes := []itemevents.FieldChange{{Field: "status_id", OldValue: oldStatus, NewValue: newStatus}}
	if _, err := itemevents.NewRecorder(s.db).StatusChanged(ctx, tx, item, &oldStatus, &newStatus, changes, metadata); err != nil {
		return err
	}
	return nil
}

// IsTransitionRejection returns the TransitionRejection if err is one, else nil.
func IsTransitionRejection(err error) *TransitionRejection {
	var rej *TransitionRejection
	if errors.As(err, &rej) {
		return rej
	}
	return nil
}

// GetTransitionID returns the transition ID for a workflow transition. A
// direct edge is preferred over a from-all row targeting the same status.
func (s *WorkflowService) GetTransitionID(workflowID int, fromStatusID, toStatusID int64) (int, error) {
	var id int
	err := s.db.QueryRow(`
		SELECT id FROM workflow_transitions
		WHERE workflow_id = ?
		  AND to_status_id = ?
		  AND (from_status_id = ? OR from_all_statuses = TRUE)
		ORDER BY CASE WHEN from_status_id IS NULL THEN 1 ELSE 0 END, display_order
		LIMIT 1
	`, workflowID, toStatusID, fromStatusID).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetAvailableTransitions returns all valid status transitions from the current status
// Uses the full fallback chain to determine the correct workflow
func (s *WorkflowService) GetAvailableTransitions(workspaceID int, itemTypeID *int, currentStatusID int64) ([]StatusTransition, error) {
	// Get the workflow using proper fallback chain
	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}

	// No workflow configured - return empty (caller should handle this)
	if workflowID == nil {
		return []StatusTransition{}, nil
	}

	// Get valid transitions from the current status. From-all rows add targets
	// that have no direct edge from this status.
	rows, err := s.db.Query(`
		SELECT s.id, s.name, sc.color
		FROM workflow_transitions wt
		JOIN statuses s ON wt.to_status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE wt.workflow_id = ?
		  AND (
			wt.from_status_id = ?
			OR (
				wt.from_all_statuses = TRUE
				AND wt.to_status_id NOT IN (
					SELECT to_status_id FROM workflow_transitions WHERE workflow_id = ? AND from_status_id = ?
				)
			)
		  )
	`, *workflowID, currentStatusID, *workflowID, currentStatusID)

	if err != nil {
		return nil, fmt.Errorf("failed to query transitions: %w", err)
	}
	defer rows.Close()

	var transitions []StatusTransition
	for rows.Next() {
		var t StatusTransition
		var color sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &color); err != nil {
			continue
		}
		if color.Valid {
			t.CategoryColor = color.String
		}
		transitions = append(transitions, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transitions: %w", err)
	}

	return transitions, nil
}

// GetTransitionByName resolves a status slug (e.g. "in-review") to the target
// status ID reachable from the item's current status. Matching is
// case-insensitive on the slugified target-status name. Returns found=false
// when no reachable transition matches.
func (s *WorkflowService) GetTransitionByName(
	workspaceID int, itemTypeID *int, fromStatusID int64, nameSlug string,
) (toStatusID int64, found bool, err error) {
	normalized := slugifyStatusName(nameSlug)
	if normalized == "" {
		return 0, false, nil
	}

	transitions, err := s.GetAvailableTransitions(workspaceID, itemTypeID, fromStatusID)
	if err != nil {
		return 0, false, err
	}

	for _, t := range transitions {
		if slugifyStatusName(t.Name) == normalized {
			return int64(t.ID), true, nil
		}
	}
	return 0, false, nil
}

// slugifyStatusName normalises a status label or smart-commit command into a
// comparable slug: lowercased, non-alphanumerics replaced with single hyphens,
// leading/trailing hyphens trimmed.
func slugifyStatusName(s string) string {
	var b []byte
	prevHyphen := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			b = append(b, c+32)
			prevHyphen = false
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b = append(b, c)
			prevHyphen = false
		default:
			if !prevHyphen {
				b = append(b, '-')
				prevHyphen = true
			}
		}
	}
	for len(b) > 0 && b[len(b)-1] == '-' {
		b = b[:len(b)-1]
	}
	return string(b)
}

// StatusTransition represents a valid status transition
type StatusTransition struct {
	ID            int
	Name          string
	CategoryColor string
}

// GetInitialStatusID returns the initial status ID for a workflow
// The initial status is identified by from_status_id IS NULL (and not a
// from-all row) in workflow_transitions
func (s *WorkflowService) GetInitialStatusID(workflowID int) (*int, error) {
	var statusID int
	err := s.db.QueryRow(`
		SELECT wt.to_status_id
		FROM workflow_transitions wt
		WHERE wt.workflow_id = ?
		  AND wt.from_status_id IS NULL
		  AND wt.from_all_statuses = FALSE
		ORDER BY wt.display_order ASC
		LIMIT 1
	`, workflowID).Scan(&statusID)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // No initial status configured for this workflow
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query initial status: %w", err)
	}

	return &statusID, nil
}

// ========================================
// Read Operations for V1 API
// ========================================

// WorkflowResult represents a workflow for listing/reading.
type WorkflowResult struct {
	ID          int
	BuiltinKey  string
	Name        string
	Description string
	IsDefault   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkflowTransitionResult represents a workflow transition.
type WorkflowTransitionResult struct {
	ID                     int
	FromStatusID           *int
	FromAllStatuses        bool
	FromStatusBuiltinKey   string
	FromStatusName         string
	FromCategoryBuiltinKey string
	FromCategoryName       string
	FromCategoryColor      string
	ToStatusID             int
	ToStatusBuiltinKey     string
	ToStatusName           string
	ToCategoryBuiltinKey   string
	ToCategoryName         string
	ToCategoryColor        string
}

// List retrieves all workflows.
func (s *WorkflowService) List() ([]WorkflowResult, error) {
	workflowModels, err := repository.NewWorkflowRepository(s.db).List()
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	workflows := make([]WorkflowResult, 0, len(workflowModels))
	for _, workflow := range workflowModels {
		workflows = append(workflows, workflowResult(workflow))
	}
	return workflows, nil
}

// ListForWorkspace returns the distinct workflows effective for a workspace's
// configured item types. Item-type overrides take precedence over the
// configuration-set workflow, which in turn falls back to the global default.
// Personal workspaces have no workflow restrictions and return an empty list.
func (s *WorkflowService) ListForWorkspace(workspaceID int) ([]WorkflowResult, error) {
	workflowIDs, err := repository.NewConfigurationSetRepository(s.db).ListEffectiveWorkflowIDs(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspace workflows: %w", err)
	}
	workflowModels, err := repository.NewWorkflowRepository(s.db).ListByIDs(workflowIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace workflows: %w", err)
	}
	workflows := make([]WorkflowResult, 0, len(workflowModels))
	for _, workflow := range workflowModels {
		workflows = append(workflows, WorkflowResult{
			ID: workflow.ID, BuiltinKey: workflow.BuiltinKey, Name: workflow.Name, Description: workflow.Description,
			IsDefault: workflow.IsDefault, CreatedAt: workflow.CreatedAt, UpdatedAt: workflow.UpdatedAt,
		})
	}
	return workflows, nil
}

// GetByID retrieves a workflow by ID.
func (s *WorkflowService) GetByID(id int) (*WorkflowResult, error) {
	model, err := repository.NewWorkflowRepository(s.db).Get(id)
	if err != nil {
		return nil, fmt.Errorf("get workflow %d: %w", id, err)
	}
	result := workflowResult(*model)
	return &result, nil
}

func workflowResult(workflow models.Workflow) WorkflowResult {
	return WorkflowResult{
		ID: workflow.ID, BuiltinKey: workflow.BuiltinKey, Name: workflow.Name,
		Description: workflow.Description, IsDefault: workflow.IsDefault,
		CreatedAt: workflow.CreatedAt, UpdatedAt: workflow.UpdatedAt,
	}
}

// Exists checks if a workflow exists.
func (s *WorkflowService) Exists(id int) (bool, error) {
	_, err := repository.NewWorkflowRepository(s.db).Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check workflow: %w", err)
	}
	return true, nil
}

// scanTransitions scans rows from a workflow transition query into a slice of results.
func (s *WorkflowService) scanTransitions(rows *sql.Rows) ([]WorkflowTransitionResult, error) { //nolint:unparam // error kept for consistency
	var transitions []WorkflowTransitionResult
	for rows.Next() {
		var t WorkflowTransitionResult
		var fromStatusID sql.NullInt64
		var fromStatusBuiltinKey, fromStatusName sql.NullString
		var fromCategoryBuiltinKey, fromCategoryName, fromCategoryColor sql.NullString

		err := rows.Scan(&t.ID, &fromStatusID, &t.ToStatusID, &t.FromAllStatuses,
			&fromStatusBuiltinKey, &fromStatusName, &t.ToStatusBuiltinKey, &t.ToStatusName,
			&fromCategoryBuiltinKey, &fromCategoryName, &fromCategoryColor,
			&t.ToCategoryBuiltinKey, &t.ToCategoryName, &t.ToCategoryColor)
		if err != nil {
			continue
		}

		if fromStatusID.Valid {
			id := int(fromStatusID.Int64)
			t.FromStatusID = &id
			t.FromStatusBuiltinKey = fromStatusBuiltinKey.String
			t.FromStatusName = fromStatusName.String
			t.FromCategoryBuiltinKey = fromCategoryBuiltinKey.String
			t.FromCategoryName = fromCategoryName.String
			t.FromCategoryColor = fromCategoryColor.String
		}

		transitions = append(transitions, t)
	}

	if transitions == nil {
		transitions = []WorkflowTransitionResult{}
	}

	return transitions, nil
}

// GetTransitions retrieves all transitions for a workflow.
func (s *WorkflowService) GetTransitions(workflowID int) ([]WorkflowTransitionResult, error) {
	transitionModels, err := repository.NewWorkflowRepository(s.db).ListTransitions(workflowID)
	if err != nil {
		return nil, fmt.Errorf("get workflow transitions: %w", err)
	}
	transitions := make([]WorkflowTransitionResult, 0, len(transitionModels))
	for _, transition := range transitionModels {
		transitions = append(transitions, WorkflowTransitionResult{
			ID: transition.ID, FromStatusID: transition.FromStatusID,
			FromAllStatuses:      transition.FromAllStatuses,
			FromStatusBuiltinKey: transition.FromStatusBuiltinKey, FromStatusName: transition.FromStatusName,
			FromCategoryBuiltinKey: transition.FromCategoryBuiltinKey, FromCategoryName: transition.FromCategoryName,
			FromCategoryColor: transition.FromCategoryColor, ToStatusID: transition.ToStatusID,
			ToStatusBuiltinKey: transition.ToStatusBuiltinKey, ToStatusName: transition.ToStatusName,
			ToCategoryBuiltinKey: transition.ToCategoryBuiltinKey, ToCategoryName: transition.ToCategoryName,
			ToCategoryColor: transition.ToCategoryColor,
		})
	}
	return transitions, nil
}

// GetTransitionsFromStatus retrieves directed transitions from a given status
// ID. NULL from-status rows define item creation and are not available moves
// from an existing item.
func (s *WorkflowService) GetTransitionsFromStatus(statusID int) ([]WorkflowTransitionResult, error) {
	rows, err := s.db.Query(`
		SELECT wt.id, wt.from_status_id, wt.to_status_id, wt.from_all_statuses,
		       fs.builtin_key as from_status_builtin_key, fs.name as from_status_name,
		       COALESCE(ts.builtin_key, ''), ts.name as to_status_name,
		       fsc.builtin_key as from_category_builtin_key, fsc.name as from_category_name, fsc.color as from_category_color,
		       COALESCE(tsc.builtin_key, ''), tsc.name as to_category_name, tsc.color as to_category_color
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		LEFT JOIN status_categories fsc ON fs.category_id = fsc.id
		JOIN status_categories tsc ON ts.category_id = tsc.id
		WHERE wt.from_status_id = ?
		ORDER BY wt.display_order
	`, statusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transitions from status: %w", err)
	}
	defer rows.Close()

	return s.scanTransitions(rows)
}

// GetTransitionsForItem retrieves the transitions available from an item's
// current status in its effective workspace/item-type workflow. Statuses are
// global records and may be shared by several workflows, so filtering only by
// status ID can leak transitions that the item can never perform.
func (s *WorkflowService) GetTransitionsForItem(
	workspaceID int, itemTypeID *int, statusID int,
) ([]WorkflowTransitionResult, error) {
	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}
	if workflowID == nil {
		return []WorkflowTransitionResult{}, nil
	}

	rows, err := s.db.Query(`
		SELECT wt.id, wt.from_status_id, wt.to_status_id, wt.from_all_statuses,
		       fs.builtin_key as from_status_builtin_key, fs.name as from_status_name,
		       COALESCE(ts.builtin_key, ''), ts.name as to_status_name,
		       fsc.builtin_key as from_category_builtin_key, fsc.name as from_category_name, fsc.color as from_category_color,
		       COALESCE(tsc.builtin_key, ''), tsc.name as to_category_name, tsc.color as to_category_color
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		LEFT JOIN status_categories fsc ON fs.category_id = fsc.id
		JOIN status_categories tsc ON ts.category_id = tsc.id
		WHERE wt.workflow_id = ?
		  AND (
			wt.from_status_id = ?
			OR (
				wt.from_all_statuses = TRUE
				AND wt.to_status_id NOT IN (
					SELECT to_status_id FROM workflow_transitions WHERE workflow_id = ? AND from_status_id = ?
				)
			)
		  )
		ORDER BY CASE WHEN wt.from_all_statuses THEN 1 ELSE 0 END, wt.display_order
	`, *workflowID, statusID, *workflowID, statusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get item workflow transitions: %w", err)
	}
	defer rows.Close()

	return s.scanTransitions(rows)
}
