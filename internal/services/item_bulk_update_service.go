package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

const (
	MaxBulkItemUpdates = 500
	MaxBulkItemPatches = 5000
)

var (
	ErrBulkItemNotFound   = errors.New("one or more items were not found")
	ErrBulkItemForbidden  = errors.New("one or more items are not editable")
	ErrBulkItemLimit      = errors.New("bulk item limit exceeded")
	ErrBulkPatchLimit     = errors.New("bulk patch item limit exceeded")
	ErrBulkFieldsRequired = errors.New("at least one field is required")
	ErrBulkDuplicateItem  = errors.New("an item may only appear once in a bulk patch")
)

// Bulk-edit accepts the same safe fields as the single-item update surface,
// excluding workflow status, item type, workspace moves, ordering, and joined
// milestone sets, all of which have dedicated semantics/endpoints.
var bulkEditableItemFields = map[string]struct{}{
	"title": {}, "description": {}, "priority_id": {},
	"due_date": {}, "start_date": {}, "end_date": {},
	"iteration_id": {}, "project_id": {}, "inherit_project": {}, "time_project_id": {},
	"assignee_id": {}, "team_id": {}, "creator_id": {}, "parent_id": {}, "related_work_item_id": {},
	"story_points": {}, "estimate_minutes": {}, "custom_field_values": {}, "is_task": {},
}

type BulkItemFieldError struct {
	Field string
}

func (e *BulkItemFieldError) Error() string {
	return fmt.Sprintf("field %q is not editable through bulk update", e.Field)
}

type BulkUpdateItemsRequest struct {
	ItemIDs            []int
	Fields             map[string]any
	UserID             int
	EventMetadata      itemevents.Metadata
	AuthorizeWorkspace func(workspaceID int) (bool, error)
}

type BulkUpdateItemsResult struct {
	Results        []UpdateItemResult
	RequestedCount int
	UpdatedCount   int
	UnchangedCount int
	SQLStatements  int
	Duration       time.Duration
}

type BulkItemPatch struct {
	ItemID int
	Fields map[string]any
}

type BulkPatchItemsRequest struct {
	Patches            []BulkItemPatch
	UserID             int
	EventMetadata      itemevents.Metadata
	AuthorizeWorkspace func(workspaceID int) (bool, error)
}

// BulkUpdateItems applies one field patch to every requested item in a single
// transaction. Any missing item, permission denial, or validation failure
// rolls the entire operation back. Since the operation only assigns values
// (never increments/appends), retries are naturally idempotent; unchanged
// items produce no write, history, or side effect result.
func (s *ItemUpdateService) BulkUpdateItems(ctx context.Context, req BulkUpdateItemsRequest) (*BulkUpdateItemsResult, error) {
	ids := uniquePositiveSortedIDs(req.ItemIDs)
	if len(ids) == 0 {
		return nil, ErrBulkItemNotFound
	}
	if len(ids) > MaxBulkItemUpdates {
		return nil, ErrBulkItemLimit
	}
	if len(req.Fields) == 0 {
		return nil, ErrBulkFieldsRequired
	}
	patches := make([]BulkItemPatch, len(ids))
	for i, id := range ids {
		patches[i] = BulkItemPatch{ItemID: id, Fields: req.Fields}
	}
	return s.BulkPatchItems(ctx, BulkPatchItemsRequest{
		Patches: patches, UserID: req.UserID, EventMetadata: req.EventMetadata,
		AuthorizeWorkspace: req.AuthorizeWorkspace,
	})
}

// BulkPatchItems applies a distinct field patch to each requested item in one
// transaction. Validation, permission, history, and post-commit side effects
// match BulkUpdateItems.
func (s *ItemUpdateService) BulkPatchItems(ctx context.Context, req BulkPatchItemsRequest) (*BulkUpdateItemsResult, error) {
	started := time.Now()
	if len(req.Patches) == 0 {
		return nil, ErrBulkItemNotFound
	}
	if len(req.Patches) > MaxBulkItemPatches {
		return nil, ErrBulkPatchLimit
	}

	fieldsByID := make(map[int]map[string]any, len(req.Patches))
	requestedIDs := make([]int, 0, len(req.Patches))
	for _, patch := range req.Patches {
		if patch.ItemID <= 0 {
			return nil, ErrBulkItemNotFound
		}
		if _, duplicate := fieldsByID[patch.ItemID]; duplicate {
			return nil, ErrBulkDuplicateItem
		}
		if len(patch.Fields) == 0 {
			return nil, ErrBulkFieldsRequired
		}
		for field := range patch.Fields {
			if _, allowed := bulkEditableItemFields[field]; !allowed {
				return nil, &BulkItemFieldError{Field: field}
			}
		}
		if err := validateBulkItemFieldTypes(patch.Fields); err != nil {
			return nil, err
		}
		fieldsByID[patch.ItemID] = patch.Fields
		requestedIDs = append(requestedIDs, patch.ItemID)
	}
	ids := uniquePositiveSortedIDs(requestedIDs)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("start bulk item patch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	repo := repository.NewItemRepository(s.db)
	originals, err := repo.FindByIDsForUpdateContext(ctx, tx, ids)
	sqlStatements := 1
	if err != nil {
		return nil, err
	}
	if len(originals) != len(ids) {
		return nil, ErrBulkItemNotFound
	}

	workspaceIDs := map[int]struct{}{}
	for _, item := range originals {
		workspaceIDs[item.WorkspaceID] = struct{}{}
	}
	for workspaceID := range workspaceIDs {
		if req.AuthorizeWorkspace == nil {
			return nil, ErrBulkItemForbidden
		}
		allowed, authErr := req.AuthorizeWorkspace(workspaceID)
		if authErr != nil {
			return nil, fmt.Errorf("authorize workspace %d: %w", workspaceID, authErr)
		}
		if !allowed {
			return nil, ErrBulkItemForbidden
		}
	}

	pending := make([]UpdateItemResult, 0, len(originals))
	eventRecords := make([]itemevents.UpdateRecord, 0, len(originals))
	for _, original := range originals {
		updated := *original
		if err := s.validator.ValidateAndApplyUpdates(&updated, fieldsByID[original.ID], req.UserID); err != nil {
			return nil, fmt.Errorf("item %d validation failed: %w", original.ID, err)
		}
		history := s.compareAndGenerateHistory(original, &updated, req.UserID)
		if len(history) == 0 {
			continue
		}
		if err := repo.Update(tx, &updated); err != nil {
			return nil, fmt.Errorf("update item %d: %w", original.ID, err)
		}
		sqlStatements++
		if err := s.recordItemHistory(tx, history); err != nil {
			return nil, fmt.Errorf("record item %d history: %w", original.ID, err)
		}
		sqlStatements += len(history)
		metadata := mergeItemEventMetadata(req.EventMetadata, itemEventMetadata(req.UserID, "application", nil))
		if metadata.OccurredAt.IsZero() {
			metadata.OccurredAt = history[0].ChangedAt
		}
		eventRecords = append(eventRecords, itemevents.UpdateRecord{
			Item: &updated, Changes: itemevents.Changes(original, &updated), Metadata: metadata,
		})
		pending = append(pending, UpdateItemResult{OriginalItem: original, FieldChanges: history})
	}
	if _, err := itemevents.NewRecorder(s.db).UpdatedBatch(ctx, tx, eventRecords); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit bulk item update: %w", err)
	}

	changedIDs := make([]int, len(pending))
	for i := range pending {
		changedIDs[i] = pending[i].OriginalItem.ID
	}
	updatedItems, err := repo.FindByIDsWithDetails(changedIDs)
	if len(changedIDs) > 0 {
		sqlStatements += 2 // detail rows plus batched milestone attachment
	}
	if err != nil {
		return nil, fmt.Errorf("load bulk-updated items: %w", err)
	}
	updatedByID := make(map[int]*models.Item, len(updatedItems))
	for _, item := range updatedItems {
		updatedByID[item.ID] = item
	}
	for i := range pending {
		updated := updatedByID[pending[i].OriginalItem.ID]
		if updated == nil {
			return nil, fmt.Errorf("bulk-updated item %d disappeared", pending[i].OriginalItem.ID)
		}
		pending[i].Item = updated
		pending[i].StatusChanged = s.hasStatusChanged(pending[i].OriginalItem, updated)
		maybeTriggerAssigneeRun(updated.WorkspaceID, updated.ID, pending[i].OriginalItem.AssigneeID, updated.AssigneeID, req.UserID)
		PublishItemChange(updated.ID, ItemChangeUpdated)
		oldParent, newParent := pending[i].OriginalItem.ParentID, updated.ParentID
		reparented := (oldParent == nil) != (newParent == nil) ||
			(oldParent != nil && newParent != nil && *oldParent != *newParent)
		if reparented {
			if oldParent != nil {
				PublishItemChange(*oldParent, ItemChangeUpdated)
			}
			if newParent != nil {
				PublishItemChange(*newParent, ItemChangeUpdated)
			}
		}
	}

	return &BulkUpdateItemsResult{
		Results:        pending,
		RequestedCount: len(ids),
		UpdatedCount:   len(pending),
		UnchangedCount: len(ids) - len(pending),
		SQLStatements:  sqlStatements,
		Duration:       time.Since(started),
	}, nil
}

func uniquePositiveSortedIDs(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			seen[id] = struct{}{}
		}
	}
	result := make([]int, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Ints(result)
	return result
}

func IsBulkItemFieldError(err error) bool {
	var fieldErr *BulkItemFieldError
	return errors.As(err, &fieldErr)
}

func IsBulkItemValidationError(err error) bool {
	var validationErr *validation.ValidationError
	return errors.As(err, &validationErr)
}

func validateBulkItemFieldTypes(fields map[string]any) error {
	for _, field := range []string{"title", "description"} {
		if value, present := fields[field]; present {
			if _, ok := value.(string); !ok {
				return &validation.ValidationError{Field: field, Message: "must be a string"}
			}
		}
	}
	for _, field := range []string{"inherit_project", "is_task"} {
		if value, present := fields[field]; present {
			if _, ok := value.(bool); !ok {
				return &validation.ValidationError{Field: field, Message: "must be a boolean"}
			}
		}
	}
	return nil
}
