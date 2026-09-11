package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

// ItemTypeChangeService owns the DB-touching logic for the item-type change
// workflow. The HTTP handler only orchestrates auth and response shaping;
// every direct query lives here so the handler boundary guard stays clean.
type ItemTypeChangeService struct {
	db               database.Database
	workflowService  *WorkflowService
	conditionService *ConditionService
	itemRepo         *repository.ItemRepository
	approvalSetRepo  *repository.ApprovalSetRepository
}

// NewItemTypeChangeService constructs a service. The optional condition service
// can be wired via WithConditionService so workspace-level overrides apply.
func NewItemTypeChangeService(db database.Database) *ItemTypeChangeService {
	return &ItemTypeChangeService{
		db:              db,
		workflowService: NewWorkflowService(db),
		itemRepo:        repository.NewItemRepository(db),
		approvalSetRepo: repository.NewApprovalSetRepository(db),
	}
}

// WithConditionService wires a shared ConditionService instance so cached
// condition-set lookups stay consistent across handlers.
func (s *ItemTypeChangeService) WithConditionService(cs *ConditionService) *ItemTypeChangeService {
	s.conditionService = cs
	return s
}

// ItemTypeChangeAnalysis describes the impact of changing an item's type, and
// is returned to clients to drive the migration UI.
type ItemTypeChangeAnalysis struct {
	ItemID                 int                `json:"item_id"`
	CurrentItemTypeID      *int               `json:"current_item_type_id"`
	CurrentItemTypeName    string             `json:"current_item_type_name"`
	TargetItemTypeID       int                `json:"target_item_type_id"`
	TargetItemTypeName     string             `json:"target_item_type_name"`
	CurrentWorkflowID      *int               `json:"current_workflow_id"`
	TargetWorkflowID       *int               `json:"target_workflow_id"`
	CurrentStatusID        *int               `json:"current_status_id"`
	CurrentStatusName      string             `json:"current_status_name"`
	RequiresMigration      bool               `json:"requires_migration"`
	SuggestedStatusID      *int               `json:"suggested_status_id,omitempty"`
	SuggestedStatusName    string             `json:"suggested_status_name,omitempty"`
	AvailableStatuses      []StatusChangeInfo `json:"available_statuses"`
	CanChangeWithoutStatus bool               `json:"can_change_without_status"`
}

// StatusChangeInfo is the minimal status descriptor surfaced in an analysis.
type StatusChangeInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Analyze produces the analysis for an item against a target type.
func (s *ItemTypeChangeService) Analyze(item *models.Item, targetTypeID int) (*ItemTypeChangeAnalysis, error) {
	target, err := s.loadItemTypeTarget(targetTypeID)
	if err != nil {
		return nil, err
	}
	if err := s.validateItemTypeAllowedForWorkspace(item.WorkspaceID, targetTypeID); err != nil {
		return nil, err
	}
	if err := s.validateItemTypeHierarchyCompatibility(item, targetTypeID); err != nil {
		return nil, err
	}

	currentWorkflowID, err := s.workflowService.GetWorkflowIDForItem(item.WorkspaceID, item.ItemTypeID)
	if err != nil {
		return nil, err
	}
	targetIDCopy := targetTypeID
	targetWorkflowID, err := s.workflowService.GetWorkflowIDForItem(item.WorkspaceID, &targetIDCopy)
	if err != nil {
		return nil, err
	}

	resp := &ItemTypeChangeAnalysis{
		ItemID:                 item.ID,
		CurrentItemTypeID:      item.ItemTypeID,
		CurrentItemTypeName:    item.ItemTypeName,
		TargetItemTypeID:       target.ID,
		TargetItemTypeName:     target.Name,
		CurrentWorkflowID:      currentWorkflowID,
		TargetWorkflowID:       targetWorkflowID,
		CurrentStatusID:        item.StatusID,
		CurrentStatusName:      item.StatusName,
		CanChangeWithoutStatus: true,
	}

	if targetWorkflowID == nil || item.StatusID == nil {
		return resp, nil
	}

	available, err := s.listWorkflowStatuses(*targetWorkflowID)
	if err != nil {
		return nil, err
	}
	resp.AvailableStatuses = available

	inTargetWorkflow, err := s.IsStatusInWorkflow(*item.StatusID, *targetWorkflowID)
	if err != nil {
		return nil, err
	}
	if inTargetWorkflow {
		return resp, nil
	}

	resp.RequiresMigration = true
	resp.CanChangeWithoutStatus = false
	suggestedID, suggestedName := s.suggestStatus(item.StatusName, *targetWorkflowID, available)
	resp.SuggestedStatusID = suggestedID
	resp.SuggestedStatusName = suggestedName
	return resp, nil
}

// IsStatusInWorkflow returns true when statusID participates in any transition
// of workflowID (either as origin or destination).
func (s *ItemTypeChangeService) IsStatusInWorkflow(statusID, workflowID int) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM workflow_transitions
			WHERE workflow_id = ? AND (from_status_id = ? OR to_status_id = ?)
		)
	`, workflowID, statusID, statusID).Scan(&exists)
	return exists, err
}

// ValidateStatusMapping rejects type changes that would bypass approval-bound
// or condition-gated transitions in the target workflow.
func (s *ItemTypeChangeService) ValidateStatusMapping(ctx context.Context, item *models.Item, targetTypeID int, targetWorkflowID, targetStatusID *int) error {
	if targetStatusID == nil || targetWorkflowID == nil || item.StatusID == nil {
		return nil
	}
	if *targetStatusID == *item.StatusID {
		return nil
	}

	pending, err := s.itemHasPendingApproval(item.ID)
	if err != nil {
		return err
	}
	if pending {
		return &validation.ValidationError{Field: "item_type_id", Message: "Cannot change item type while an approval is pending"}
	}

	approvalBound, err := s.statusIsApprovalBound(ctx, item.WorkspaceID, targetTypeID, *targetStatusID)
	if err != nil {
		return err
	}
	if approvalBound {
		return &validation.ValidationError{Field: "target_status_id", Message: "Target status requires approval in the target item type; item type change is blocked"}
	}

	initialStatusID, err := s.workflowService.GetInitialStatusID(*targetWorkflowID)
	if err != nil {
		return err
	}
	if initialStatusID != nil && *initialStatusID == *targetStatusID {
		return nil
	}

	transitionID, err := s.findWorkflowTransitionID(*targetWorkflowID, *item.StatusID, *targetStatusID)
	if err != nil {
		return err
	}
	if transitionID == nil {
		return &validation.ValidationError{Field: "target_status_id", Message: "Target status is not reachable by a direct transition in the target item type workflow"}
	}

	conditionSetID, err := s.resolveConditionSetIDForItemType(item.WorkspaceID, targetTypeID)
	if err != nil {
		return err
	}
	if conditionSetID != nil {
		hasConditions, err := s.transitionHasConditions(*conditionSetID, *transitionID)
		if err != nil {
			return err
		}
		if hasConditions {
			return &validation.ValidationError{Field: "target_status_id", Message: "Target status transition has conditions in the target item type; item type change is blocked"}
		}
	}

	return nil
}

// ApplyChange runs the item-type change atomically: it updates the item fields
// and records the history rows. Callers must have already validated inputs via
// Analyze + ValidateStatusMapping.
func (s *ItemTypeChangeService) ApplyChange(itemID, userID, targetTypeID int, nextStatusID *int, original *models.Item) ([]HistoryEntry, error) {
	finalStatusID := original.StatusID
	if nextStatusID != nil {
		finalStatusID = nextStatusID
	}
	if err := validation.ValidateTaskState(s.db, original.WorkspaceID, userID, original.IsTask, finalStatusID); err != nil {
		return nil, err
	}
	now := time.Now()
	fields := map[string]any{"item_type_id": targetTypeID}
	if nextStatusID != nil {
		fields["status_id"] = *nextStatusID
	}

	history := []HistoryEntry{{
		ItemID:    itemID,
		UserID:    userID,
		FieldName: "item_type_id",
		OldValue:  intPtrHistoryValue(original.ItemTypeID),
		NewValue:  fmt.Sprintf("%d", targetTypeID),
		ChangedAt: now,
	}}
	if nextStatusID != nil {
		history = append(history, HistoryEntry{
			ItemID:    itemID,
			UserID:    userID,
			FieldName: "status_id",
			OldValue:  intPtrHistoryValue(original.StatusID),
			NewValue:  fmt.Sprintf("%d", *nextStatusID),
			ChangedAt: now,
		})
	}

	err := database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.itemRepo.UpdateFields(tx, itemID, fields); err != nil {
			return err
		}
		if err := s.itemRepo.RecordHistoryBatch(tx, history); err != nil {
			return fmt.Errorf("record item history: %w", err)
		}
		updated := *original
		updated.ItemTypeID = &targetTypeID
		if nextStatusID != nil {
			updated.StatusID = nextStatusID
		}
		metadata := itemevents.User(userID, "application")
		metadata.OccurredAt = now
		recorder := itemevents.NewRecorder(s.db)
		changes := itemevents.Changes(original, &updated)
		if nextStatusID != nil {
			if _, err := recorder.StatusChanged(context.Background(), tx, &updated, original.StatusID, nextStatusID, changes, metadata); err != nil {
				return err
			}
			return nil
		}
		_, err := recorder.Updated(context.Background(), tx, &updated, changes, metadata)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Live-update publish (WI-483): the type change (and optional status change)
	// committed. A status kind triggers the status-aware reload on the client.
	kind := ItemChangeUpdated
	if nextStatusID != nil {
		kind = ItemChangeStatus
	}
	PublishItemChange(itemID, kind)

	return history, nil
}

type itemTypeTargetDetails struct {
	ID             int
	Name           string
	HierarchyLevel int
}

func (s *ItemTypeChangeService) loadItemTypeTarget(id int) (*itemTypeTargetDetails, error) {
	var out itemTypeTargetDetails
	err := s.db.QueryRow(`SELECT id, name, COALESCE(hierarchy_level, 0) FROM item_types WHERE id = ?`, id).Scan(&out.ID, &out.Name, &out.HierarchyLevel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &validation.ValidationError{Field: "target_item_type_id", Message: "Item type not found"}
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *ItemTypeChangeService) validateItemTypeAllowedForWorkspace(workspaceID, targetTypeID int) error {
	allowed, err := repository.NewConfigurationSetRepository(s.db).ItemTypeAllowed(workspaceID, targetTypeID)
	if err != nil {
		return err
	}
	if !allowed {
		return &validation.ValidationError{Field: "target_item_type_id", Message: "Item type is not allowed in this workspace"}
	}
	return nil
}

func (s *ItemTypeChangeService) validateItemTypeHierarchyCompatibility(item *models.Item, targetTypeID int) error {
	err := validation.ValidateItemTypePlacement(s.db, item.ID, targetTypeID, item.ParentID)
	var placementErr *validation.ValidationError
	if errors.As(err, &placementErr) {
		return &validation.ValidationError{
			Field:   "target_item_type_id",
			Message: placementErr.Message,
		}
	}
	return err
}

func (s *ItemTypeChangeService) listWorkflowStatuses(workflowID int) ([]StatusChangeInfo, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT s.id, s.name
		FROM workflow_transitions wt
		JOIN statuses s ON wt.from_status_id = s.id OR wt.to_status_id = s.id
		WHERE wt.workflow_id = ?
		ORDER BY s.name
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []StatusChangeInfo{}
	for rows.Next() {
		var info StatusChangeInfo
		if err := rows.Scan(&info.ID, &info.Name); err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, rows.Err()
}

func (s *ItemTypeChangeService) suggestStatus(currentName string, workflowID int, available []StatusChangeInfo) (statusID *int, statusName string) {
	currentNorm := strings.ToLower(strings.TrimSpace(currentName))
	for _, status := range available {
		if strings.ToLower(strings.TrimSpace(status.Name)) == currentNorm && currentNorm != "" {
			id := status.ID
			return &id, status.Name
		}
	}
	initialID, err := s.workflowService.GetInitialStatusID(workflowID)
	if err == nil && initialID != nil {
		for _, status := range available {
			if status.ID == *initialID {
				id := status.ID
				return &id, status.Name
			}
		}
	}
	return nil, ""
}

func (s *ItemTypeChangeService) itemHasPendingApproval(itemID int) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM approval_requests WHERE item_id = ? AND status = 'pending')`, itemID).Scan(&exists)
	return exists, err
}

func (s *ItemTypeChangeService) statusIsApprovalBound(ctx context.Context, workspaceID, itemTypeID, statusID int) (bool, error) {
	itemTypePtr := itemTypeID
	approvalSetID, err := s.approvalSetRepo.ResolveForWorkspace(ctx, workspaceID, &itemTypePtr)
	if err != nil || approvalSetID == nil {
		return false, err
	}
	var exists bool
	err = s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM approval_set_statuses
			WHERE approval_set_id = ? AND status_id = ? AND is_active = true
		)
	`, *approvalSetID, statusID).Scan(&exists)
	return exists, err
}

func (s *ItemTypeChangeService) resolveConditionSetIDForItemType(workspaceID, itemTypeID int) (*int, error) {
	id := itemTypeID
	if s.conditionService != nil {
		return s.conditionService.GetConditionSetIDForItem(workspaceID, &id)
	}
	return NewConditionService(s.db, nil, nil).GetConditionSetIDForItem(workspaceID, &id)
}

func (s *ItemTypeChangeService) findWorkflowTransitionID(workflowID, fromStatusID, toStatusID int) (*int, error) {
	var id int
	err := s.db.QueryRow(`
		SELECT id FROM workflow_transitions
		WHERE workflow_id = ?
		  AND to_status_id = ?
		  AND (from_status_id = ? OR from_all_statuses = TRUE)
		ORDER BY CASE WHEN from_status_id IS NULL THEN 1 ELSE 0 END, display_order
		LIMIT 1
	`, workflowID, toStatusID, fromStatusID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s *ItemTypeChangeService) transitionHasConditions(conditionSetID, transitionID int) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM condition_set_transitions cst
			JOIN conditions c ON c.condition_set_transition_id = cst.id
			WHERE cst.condition_set_id = ? AND cst.transition_id = ?
		)
	`, conditionSetID, transitionID).Scan(&exists)
	return exists, err
}

func intPtrHistoryValue(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}
