package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

// ItemUpdateService handles item update operations with validation, history tracking, and event emission
type ItemUpdateService struct {
	db        database.Database
	validator *validation.ItemFieldValidator
}

// NewItemUpdateService creates a new item update service. The validator is
// pre-wired with a HierarchyService-backed cycle checker so parent_id updates
// reject self-parent and cycle-inducing moves by default.
func NewItemUpdateService(db database.Database) *ItemUpdateService {
	return &ItemUpdateService{
		db:        db,
		validator: validation.NewItemFieldValidator(db).WithCycleChecker(NewHierarchyService(db)),
	}
}

// WithPermissionService wires a PermissionService into the validator so it
// can enforce the caller's workspace view-permission on cross-workspace
// parent assignments. User-facing callers must set this; internal callers
// that don't touch parent_id may omit it.
func (s *ItemUpdateService) WithPermissionService(permService *PermissionService) *ItemUpdateService {
	s.validator = s.validator.WithPermissionChecker(permService)
	// Also enforce that the caller may assign the project_id / time_project_id
	// they pass. CanViewProject is pure-SQL plus the global-admin check on the
	// supplied permission service, so this reuses the same identity.
	s.validator = s.validator.WithProjectAccessChecker(NewTimePermissionService(s.db, permService))
	if permService == nil {
		s.validator = s.validator.WithWorkspaceAssigneeChecker(nil)
	} else {
		s.validator = s.validator.WithWorkspaceAssigneeChecker(NewWorkspaceUserResolver(s.db, permService))
	}
	return s
}

// UpdateItemRequest contains the data needed to update an item
type UpdateItemRequest struct {
	ItemID        int
	UpdateData    map[string]any
	UserID        int
	EventMetadata itemevents.Metadata
}

// FindItem loads an item through the item repository for pre-update
// authorization checks, keeping persistence access behind the service layer.
func (s *ItemUpdateService) FindItem(itemID int) (*models.Item, error) {
	return repository.NewItemRepository(s.db).FindByID(itemID)
}

// UpdateItemResult contains the result of an item update operation
type UpdateItemResult struct {
	OriginalItem  *models.Item // The item before updates (for notifications)
	Item          *models.Item // The item after updates
	StatusChanged bool
	FieldChanges  []HistoryEntry
}

// ItemUpdateTransactionHook extends the canonical update transaction with
// source-owned records that must commit atomically with the item mutation.
type ItemUpdateTransactionHook func(context.Context, database.Tx, *models.Item, *models.Item) error

type itemUpdateOptions struct {
	allowStatus             bool
	recordHistory           bool
	triggerAssignee         bool
	publish                 bool
	beforeUpdateTransaction ItemUpdateTransactionHook
	afterUpdateTransaction  ItemUpdateTransactionHook
}

// HistoryEntry represents a single field change in item history.
// It aliases the repository record so services can hand history rows straight
// to ItemRepository.RecordHistory/RecordHistoryBatch without converting.
type HistoryEntry = repository.HistoryEntry

// UpdateItem updates an item with validation, transaction safety, and history tracking.
//
// Workflow and workspace moves must not go through this path. Each has a
// dedicated service that enforces its authorization, mapping, and cleanup
// invariants, so reject those fields before opening a transaction.
func (s *ItemUpdateService) UpdateItem(req UpdateItemRequest) (*UpdateItemResult, error) {
	return s.updateItem(context.Background(), req, itemUpdateOptions{
		recordHistory:   true,
		triggerAssignee: true,
		publish:         true,
	})
}

func (s *ItemUpdateService) updateItem(ctx context.Context, req UpdateItemRequest, opts itemUpdateOptions) (*UpdateItemResult, error) {
	if err := validateItemUpdateRequest(ctx, req, opts); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	update := itemUpdateOperation{service: s, ctx: ctx, req: req, opts: opts, tx: tx}
	if err := update.apply(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return update.result()
}

// AddMilestone atomically adds one milestone to an item while preserving the
// rest of the set. It returns changed=false for a duplicate attachment so
// retries do not repeat history or downstream effects.
func (s *ItemUpdateService) AddMilestone(req UpdateItemRequest, milestoneID int) (*UpdateItemResult, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, false, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	originalItem, err := s.loadItemInTx(tx, req.ItemID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load item: %w", err)
	}
	oldIDs, alreadyAttached, err := loadItemMilestoneIDs(tx, req.ItemID, milestoneID)
	if err != nil {
		return nil, false, err
	}
	newIDs := append(append([]int(nil), oldIDs...), milestoneID)
	sort.Ints(newIDs)
	if alreadyAttached {
		return s.finishUnchangedMilestoneUpdate(tx, originalItem, req.ItemID, false)
	}
	if err := validation.ValidatePlanningAssignments(tx, originalItem.WorkspaceID, newIDs, originalItem.IterationID); err != nil {
		return nil, false, fmt.Errorf("validation failed: %w", err)
	}

	now := time.Now()
	inserted, err := insertItemMilestone(tx, req.ItemID, milestoneID, now)
	if err != nil {
		return nil, false, err
	}
	if !inserted {
		return s.finishUnchangedMilestoneUpdate(tx, originalItem, req.ItemID, true)
	}
	history, err := s.recordMilestoneAddition(tx, req, originalItem, oldIDs, newIDs, now)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	updatedItem, err := s.loadItemWithJoins(req.ItemID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load updated item: %w", err)
	}
	PublishItemChange(updatedItem.ID, ItemChangeUpdated)
	return &UpdateItemResult{
		OriginalItem: originalItem,
		Item:         updatedItem,
		FieldChanges: history,
	}, true, nil
}

func loadItemMilestoneIDs(tx database.Tx, itemID, targetID int) (ids []int, found bool, err error) {
	rows, err := tx.Query(`
		SELECT milestone_id
		FROM item_milestones
		WHERE item_id = ?
		ORDER BY milestone_id
	`, itemID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read existing item milestones: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, false, fmt.Errorf("failed to scan existing item milestone: %w", err)
		}
		ids = append(ids, id)
		found = found || id == targetID
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("failed to iterate existing item milestones: %w", err)
	}
	return ids, found, nil
}

func (s *ItemUpdateService) finishUnchangedMilestoneUpdate(
	tx database.Tx,
	originalItem *models.Item,
	itemID int,
	concurrent bool,
) (*UpdateItemResult, bool, error) {
	transactionKind := "duplicate milestone"
	loadKind := "unchanged"
	if concurrent {
		transactionKind = "concurrent duplicate"
		loadKind = "concurrently updated"
	}
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return nil, false, fmt.Errorf("failed to close %s transaction: %w", transactionKind, err)
	}
	current, err := s.loadItemWithJoins(itemID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load %s item: %w", loadKind, err)
	}
	return &UpdateItemResult{OriginalItem: originalItem, Item: current}, false, nil
}

func insertItemMilestone(tx database.Tx, itemID, milestoneID int, now time.Time) (bool, error) {
	result, err := tx.Exec(`
		INSERT INTO item_milestones (item_id, milestone_id, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(item_id, milestone_id) DO NOTHING
	`, itemID, milestoneID, now)
	if err != nil {
		return false, fmt.Errorf("failed to attach milestone %d: %w", milestoneID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to inspect milestone attachment: %w", err)
	}
	return rowsAffected > 0, nil
}

func (s *ItemUpdateService) recordMilestoneAddition(
	tx database.Tx,
	req UpdateItemRequest,
	originalItem *models.Item,
	oldIDs, newIDs []int,
	now time.Time,
) ([]HistoryEntry, error) {
	if _, err := tx.Exec(`
		UPDATE items
		SET updated_at = ?, last_active_at = ?
		WHERE id = ?
	`, now, now, req.ItemID); err != nil {
		return nil, fmt.Errorf("failed to update item activity time: %w", err)
	}
	history := []HistoryEntry{{
		ItemID:    req.ItemID,
		UserID:    req.UserID,
		FieldName: "milestones",
		OldValue:  joinIntsCSV(oldIDs),
		NewValue:  joinIntsCSV(newIDs),
		ChangedAt: now,
	}}
	if err := s.recordItemHistory(tx, history); err != nil {
		return nil, fmt.Errorf("failed to record history: %w", err)
	}
	metadata := mergeItemEventMetadata(req.EventMetadata, itemEventMetadata(req.UserID, "application", nil))
	if metadata.OccurredAt.IsZero() {
		metadata.OccurredAt = now
	}
	if _, err := itemevents.NewRecorder(s.db).Updated(
		context.Background(), tx, originalItem, itemHistoryEventChanges(history), metadata,
	); err != nil {
		return nil, err
	}
	return history, nil
}

func hasAnyItemUpdateField(updateData map[string]any, fields ...string) bool {
	for _, field := range fields {
		if _, ok := updateData[field]; ok {
			return true
		}
	}
	return false
}

func planningMilestoneIDsForUpdate(tx database.Tx, itemID int, item *models.Item, changed bool) ([]int, error) {
	if changed {
		ids := make([]int, 0, len(item.Milestones))
		for _, milestone := range item.Milestones {
			ids = append(ids, milestone.ID)
		}
		return ids, nil
	}

	rows, err := tx.Query("SELECT milestone_id FROM item_milestones WHERE item_id = ? ORDER BY milestone_id", itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to load item milestones for scope validation: %w", err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan item milestone for scope validation: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate item milestones for scope validation: %w", err)
	}
	return ids, nil
}

// loadItemInTx loads an item inside a transaction. On Postgres, the repository
// appends FOR UPDATE to lock the row while validation and mutation run.
func (s *ItemUpdateService) loadItemInTx(tx database.Tx, itemID int) (*models.Item, error) {
	item, err := repository.NewItemRepository(s.db).FindByIDForUpdate(tx, itemID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("item not found")
	}
	return item, err
}

// loadItemWithJoins loads an item with all joined data for response.
func (s *ItemUpdateService) loadItemWithJoins(itemID int) (*models.Item, error) {
	return repository.NewItemRepository(s.db).FindByIDWithDetails(itemID)
}

// hasStatusChanged checks if the status changed between two items
func (s *ItemUpdateService) hasStatusChanged(original, updated *models.Item) bool {
	if original.StatusID == nil && updated.StatusID != nil {
		return true
	}
	if original.StatusID != nil && updated.StatusID == nil {
		return true
	}
	if original.StatusID != nil && updated.StatusID != nil && *original.StatusID != *updated.StatusID {
		return true
	}
	return false
}

// compareAndGenerateHistory compares two items and generates history entries for changed fields
func (s *ItemUpdateService) compareAndGenerateHistory(original, updated *models.Item, userID int) []HistoryEntry {
	var history []HistoryEntry
	now := time.Now()

	// Helper to add history entry
	addHistory := func(fieldName, oldValue, newValue string) {
		if oldValue != newValue {
			history = append(history, HistoryEntry{
				ItemID:    updated.ID,
				UserID:    userID,
				FieldName: fieldName,
				OldValue:  oldValue,
				NewValue:  newValue,
				ChangedAt: now,
			})
		}
	}

	// Compare simple string fields
	addHistory("title", original.Title, updated.Title)
	addHistory("description", original.Description, updated.Description)

	// Compare nullable ID fields
	addHistory("status_id", intPtrToString(original.StatusID), intPtrToString(updated.StatusID))
	addHistory("priority_id", intPtrToString(original.PriorityID), intPtrToString(updated.PriorityID))
	addHistory("iteration_id", intPtrToString(original.IterationID), intPtrToString(updated.IterationID))
	addHistory("project_id", intPtrToString(original.ProjectID), intPtrToString(updated.ProjectID))
	addHistory("time_project_id", intPtrToString(original.TimeProjectID), intPtrToString(updated.TimeProjectID))
	addHistory("assignee_id", intPtrToString(original.AssigneeID), intPtrToString(updated.AssigneeID))
	addHistory("creator_id", intPtrToString(original.CreatorID), intPtrToString(updated.CreatorID))
	addHistory("parent_id", intPtrToString(original.ParentID), intPtrToString(updated.ParentID))
	addHistory("related_work_item_id", intPtrToString(original.RelatedWorkItemID), intPtrToString(updated.RelatedWorkItemID))
	if original.IsTask != updated.IsTask {
		addHistory("is_task", fmt.Sprintf("%t", original.IsTask), fmt.Sprintf("%t", updated.IsTask))
	}

	// Compare date fields
	addHistory("due_date", timePtrToString(original.DueDate), timePtrToString(updated.DueDate))
	addHistory("start_date", timePtrToString(original.StartDate), timePtrToString(updated.StartDate))
	addHistory("end_date", timePtrToString(original.EndDate), timePtrToString(updated.EndDate))

	// Compare workspace_id (simple int)
	if original.WorkspaceID != updated.WorkspaceID {
		addHistory("workspace_id", fmt.Sprintf("%d", original.WorkspaceID), fmt.Sprintf("%d", updated.WorkspaceID))
	}

	// Compare inherit_project (bool)
	if original.InheritProject != updated.InheritProject {
		addHistory("inherit_project", fmt.Sprintf("%t", original.InheritProject), fmt.Sprintf("%t", updated.InheritProject))
	}

	// Compare story_points (float64 pointer)
	addHistory("story_points", float64PtrToString(original.StoryPoints), float64PtrToString(updated.StoryPoints))

	// Compare estimate_minutes (int pointer)
	addHistory("estimate_minutes", intPtrToString(original.EstimateMinutes), intPtrToString(updated.EstimateMinutes))

	// Compare custom field values
	allKeys := make(map[string]struct{})
	for k := range original.CustomFieldValues {
		allKeys[k] = struct{}{}
	}
	for k := range updated.CustomFieldValues {
		allKeys[k] = struct{}{}
	}
	for k := range allKeys {
		oldVal := customFieldValueToString(original.CustomFieldValues[k])
		newVal := customFieldValueToString(updated.CustomFieldValues[k])
		addHistory("cf_"+k, oldVal, newVal)
	}

	return history
}

func creationHistoryEntries(item models.Item, userID int, changedAt time.Time) []HistoryEntry {
	var history []HistoryEntry
	if changedAt.IsZero() {
		changedAt = time.Now()
	}

	// Helper to add history entry (old_value is always empty for creation)
	addHistory := func(fieldName, newValue string) {
		if newValue != "" {
			history = append(history, HistoryEntry{
				ItemID:    item.ID,
				UserID:    userID,
				FieldName: fieldName,
				OldValue:  "",
				NewValue:  newValue,
				ChangedAt: changedAt,
			})
		}
	}

	// Add entries for all fields
	addHistory("title", item.Title)
	addHistory("description", item.Description)
	addHistory("item_type_id", intPtrToString(item.ItemTypeID))
	addHistory("status_id", intPtrToString(item.StatusID))
	addHistory("priority_id", intPtrToString(item.PriorityID))
	addHistory("iteration_id", intPtrToString(item.IterationID))
	addHistory("project_id", intPtrToString(item.ProjectID))
	addHistory("time_project_id", intPtrToString(item.TimeProjectID))
	addHistory("assignee_id", intPtrToString(item.AssigneeID))
	addHistory("creator_id", intPtrToString(item.CreatorID))
	addHistory("parent_id", intPtrToString(item.ParentID))
	addHistory("due_date", timePtrToString(item.DueDate))
	addHistory("start_date", timePtrToString(item.StartDate))
	addHistory("end_date", timePtrToString(item.EndDate))
	addHistory("workspace_id", fmt.Sprintf("%d", item.WorkspaceID))
	addHistory("story_points", float64PtrToString(item.StoryPoints))
	addHistory("estimate_minutes", intPtrToString(item.EstimateMinutes))
	// Only record inherit_project when set; the default (false) on root items
	// would otherwise add a no-op entry to every creation event.
	if item.InheritProject {
		addHistory("inherit_project", "true")
	}

	return history
}

// recordItemHistory records history entries in the database
func (s *ItemUpdateService) recordItemHistory(tx database.Tx, history []HistoryEntry) error {
	return repository.NewItemRepository(s.db).RecordHistoryBatch(tx, history)
}

// joinIntsCSV renders a sorted slice of ints as "1,2,3" — used as the
// old/new payload of milestones history rows.
func joinIntsCSV(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, n := range ids {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ",")
}

// Helper functions for converting values to strings for history
func intPtrToString(val *int) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%d", *val)
}

func timePtrToString(val *time.Time) string {
	if val == nil {
		return ""
	}
	return val.Format("2006-01-02")
}

func float64PtrToString(val *float64) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%g", *val)
}

func customFieldValueToString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
