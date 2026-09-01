package services

import (
	"context"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

type itemUpdateOperation struct {
	service           *ItemUpdateService
	ctx               context.Context
	req               UpdateItemRequest
	opts              itemUpdateOptions
	tx                database.Tx
	original          *models.Item
	updated           models.Item
	now               time.Time
	milestoneOldIDs   []int
	milestoneNewIDs   []int
	hasMilestoneIDs   bool
	milestonesChanged bool
	history           []HistoryEntry
}

func validateItemUpdateRequest(ctx context.Context, req UpdateItemRequest, opts itemUpdateOptions) error {
	if ctx == nil {
		return fmt.Errorf("item update requires a context")
	}
	if _, ok := req.UpdateData["status_id"]; ok && !opts.allowStatus {
		return &validation.ValidationError{
			Field:   "status_id",
			Message: "must be changed via the transition endpoint, not item update",
		}
	}
	if _, ok := req.UpdateData["item_type_id"]; ok {
		return &validation.ValidationError{
			Field:   "item_type_id",
			Message: "must be changed via the item type change endpoint, not item update",
		}
	}
	if _, ok := req.UpdateData["workspace_id"]; ok {
		return &validation.ValidationError{
			Field:   "workspace_id",
			Message: "must be changed via the workspace move endpoint, not item update",
		}
	}
	return nil
}

func (u *itemUpdateOperation) apply() error {
	original, err := u.service.loadItemInTx(u.tx, u.req.ItemID)
	if err != nil {
		return fmt.Errorf("failed to load item: %w", err)
	}
	u.original = original
	u.updated = *original
	if err := u.service.validator.ValidateAndApplyUpdates(&u.updated, u.req.UpdateData, u.req.UserID); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	if err := u.validatePlanningAssignments(); err != nil {
		return err
	}
	if u.opts.beforeUpdateTransaction != nil {
		if err := u.opts.beforeUpdateTransaction(u.ctx, u.tx, u.original, &u.updated); err != nil {
			return err
		}
	}

	u.now = time.Now()
	if err := repository.NewItemRepository(u.service.db).Update(u.tx, &u.updated); err != nil {
		return err
	}
	if err := u.replaceMilestones(); err != nil {
		return err
	}
	if u.opts.afterUpdateTransaction != nil {
		if err := u.opts.afterUpdateTransaction(u.ctx, u.tx, u.original, &u.updated); err != nil {
			return err
		}
	}
	return u.recordHistoryAndEvent()
}

func (u *itemUpdateOperation) validatePlanningAssignments() error {
	_, milestonesChanged := u.req.UpdateData["milestone_ids"]
	if !milestonesChanged && !hasAnyItemUpdateField(u.req.UpdateData, "iteration_id", "workspace_id") {
		return nil
	}
	milestoneIDs, err := planningMilestoneIDsForUpdate(u.tx, u.req.ItemID, &u.updated, milestonesChanged)
	if err != nil {
		return err
	}
	if err := validation.ValidatePlanningAssignments(
		u.tx,
		u.updated.WorkspaceID,
		milestoneIDs,
		u.updated.IterationID,
	); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

func (u *itemUpdateOperation) replaceMilestones() error {
	if _, ok := u.req.UpdateData["milestone_ids"]; !ok {
		return nil
	}
	u.hasMilestoneIDs = true

	oldIDs, err := readItemMilestoneIDsForHistory(u.tx, u.req.ItemID)
	if err != nil {
		return err
	}
	u.milestoneOldIDs = oldIDs
	if _, err := u.tx.Exec("DELETE FROM item_milestones WHERE item_id = ?", u.req.ItemID); err != nil {
		return fmt.Errorf("failed to clear item_milestones: %w", err)
	}
	for _, milestone := range u.updated.Milestones {
		u.milestoneNewIDs = append(u.milestoneNewIDs, milestone.ID)
		if _, err := u.tx.Exec(
			"INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?)",
			u.req.ItemID,
			milestone.ID,
			u.now,
		); err != nil {
			return fmt.Errorf("failed to attach milestone %d: %w", milestone.ID, err)
		}
	}
	u.milestonesChanged = joinIntsCSV(u.milestoneOldIDs) != joinIntsCSV(u.milestoneNewIDs)
	return nil
}

func readItemMilestoneIDsForHistory(tx database.Tx, itemID int) ([]int, error) {
	rows, err := tx.Query("SELECT milestone_id FROM item_milestones WHERE item_id = ? ORDER BY milestone_id", itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to read existing item_milestones: %w", err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan existing milestone id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate existing milestone ids: %w", err)
	}
	return ids, nil
}

func (u *itemUpdateOperation) recordHistoryAndEvent() error {
	eventHistory := u.service.compareAndGenerateHistory(u.original, &u.updated, u.req.UserID)
	if u.opts.recordHistory {
		u.history = append(u.history, eventHistory...)
	}
	if u.hasMilestoneIDs && u.milestonesChanged {
		entry := HistoryEntry{
			ItemID: u.req.ItemID, UserID: u.req.UserID, FieldName: "milestones",
			OldValue: joinIntsCSV(u.milestoneOldIDs), NewValue: joinIntsCSV(u.milestoneNewIDs),
			ChangedAt: u.now,
		}
		eventHistory = append(eventHistory, entry)
		if u.opts.recordHistory {
			u.history = append(u.history, entry)
		}
	}
	if u.opts.recordHistory {
		if err := u.service.recordItemHistory(u.tx, u.history); err != nil {
			return fmt.Errorf("failed to record history: %w", err)
		}
	}
	if len(eventHistory) == 0 {
		return nil
	}
	return u.recordEvent()
}

func (u *itemUpdateOperation) recordEvent() error {
	metadata := mergeItemEventMetadata(u.req.EventMetadata, itemEventMetadata(u.req.UserID, "application", nil))
	if metadata.OccurredAt.IsZero() {
		metadata.OccurredAt = u.now
	}
	changes := itemevents.Changes(u.original, &u.updated)
	if u.hasMilestoneIDs && u.milestonesChanged {
		changes = append(changes, itemevents.FieldChange{
			Field: "milestones", OldValue: u.milestoneOldIDs, NewValue: u.milestoneNewIDs,
		})
	}
	recorder := itemevents.NewRecorder(u.service.db)
	if u.service.hasStatusChanged(u.original, &u.updated) {
		_, err := recorder.StatusChanged(
			u.ctx, u.tx, &u.updated, u.original.StatusID, u.updated.StatusID, changes, metadata,
		)
		return err
	}
	_, err := recorder.Updated(u.ctx, u.tx, &u.updated, changes, metadata)
	return err
}

func (u *itemUpdateOperation) result() (*UpdateItemResult, error) {
	updatedItem, err := u.service.loadItemWithJoins(u.req.ItemID)
	if err != nil {
		return nil, fmt.Errorf("failed to load updated item: %w", err)
	}
	statusChanged := u.service.hasStatusChanged(u.original, updatedItem)
	if u.opts.triggerAssignee {
		maybeTriggerAssigneeRun(
			updatedItem.WorkspaceID,
			updatedItem.ID,
			u.original.AssigneeID,
			updatedItem.AssigneeID,
			u.req.UserID,
		)
	}
	if u.opts.publish {
		publishUpdatedItem(u.original, updatedItem, statusChanged)
	}
	return &UpdateItemResult{
		OriginalItem: u.original, Item: updatedItem,
		StatusChanged: statusChanged, FieldChanges: u.history,
	}, nil
}

func publishUpdatedItem(original, updated *models.Item, statusChanged bool) {
	changeKind := ItemChangeUpdated
	if statusChanged {
		changeKind = ItemChangeStatus
	}
	PublishItemChange(updated.ID, changeKind)
	oldParent, newParent := original.ParentID, updated.ParentID
	reparented := (oldParent == nil) != (newParent == nil) ||
		(oldParent != nil && newParent != nil && *oldParent != *newParent)
	if !reparented {
		return
	}
	if oldParent != nil {
		PublishItemChange(*oldParent, ItemChangeUpdated)
	}
	if newParent != nil {
		PublishItemChange(*newParent, ItemChangeUpdated)
	}
}
