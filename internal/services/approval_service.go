package services

import (
	"context"
	"database/sql"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ErrApprovalNotFound is returned when an approval request or related approval resource is not found.
var ErrApprovalNotFound = sql.ErrNoRows

// ApprovalService owns stateful approvals, unlike synchronous conditions. It
// snapshots approvers, commits configured decision transitions, and cancels a
// pending request when an item leaves its approval-bound status. Users cannot
// invoke approve or deny transitions directly.
type ApprovalService struct {
	db              database.Database
	leaveRepo       *repository.LeaveRepository
	workflowService *WorkflowService

	runtimeRepo  *repository.ApprovalRepository
	templateRepo *repository.ApprovalSetRepository

	// Set at startup; nil for notification-free gating tests.
	eventCoordinator *EventCoordinator
}

// NewApprovalService constructs an approval service; events are wired separately.
func NewApprovalService(db database.Database, leaveRepo *repository.LeaveRepository, workflowService *WorkflowService) *ApprovalService {
	return &ApprovalService{
		db:              db,
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
func (s *ApprovalService) GetApprovalSetIDForItem(ctx context.Context, workspaceID int, itemTypeID *int) (*int, error) {
	if s == nil {
		return nil, nil
	}
	return s.templateRepo.ResolveForWorkspace(ctx, workspaceID, itemTypeID)
}

// GetApprovalSetStatusForItem returns the approval_set_status (template row)
// that applies to an item entering statusID, or nil if no approval gates the entry.
func (s *ApprovalService) GetApprovalSetStatusForItem(ctx context.Context, workspaceID int, itemTypeID *int, statusID int) (*models.ApprovalSetStatus, error) {
	approvalSetID, err := s.GetApprovalSetIDForItem(ctx, workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}
	if approvalSetID == nil {
		return nil, nil
	}
	return s.templateRepo.FindActiveStatusBySetAndStatus(ctx, *approvalSetID, statusID)
}

// ----------------------------------------------------------------------------
// RequestApproval: open a new pending approval request.
// ----------------------------------------------------------------------------

// RequestApproval opens a new approval request for the item. The caller (typically
// PerformTransition's post-commit hook) is responsible for ensuring no pending
// request already exists; the unique partial index uq_approval_requests_one_open_per_item
// enforces this at the DB layer as defense in depth.
