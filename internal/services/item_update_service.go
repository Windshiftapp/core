package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
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
	return s
}

// UpdateItemRequest contains the data needed to update an item
type UpdateItemRequest struct {
	ItemID     int
	UpdateData map[string]interface{}
	UserID     int
}

// UpdateItemResult contains the result of an item update operation
type UpdateItemResult struct {
	OriginalItem  *models.Item // The item before updates (for notifications)
	Item          *models.Item // The item after updates
	StatusChanged bool
	FieldChanges  []HistoryEntry
}

// HistoryEntry represents a single field change in item history
type HistoryEntry struct {
	ItemID    int
	UserID    int
	FieldName string
	OldValue  string
	NewValue  string
	ChangedAt time.Time
}

// UpdateItem updates an item with validation, transaction safety, and history tracking.
//
// Workflow-status changes must NOT go through this path — callers should use
// WorkflowService.PerformTransition directly (via the HTTP transition endpoint
// or an action executor). Accepting status_id here would skip the transition
// validation + event emission pipeline, so we reject it defensively.
func (s *ItemUpdateService) UpdateItem(req UpdateItemRequest) (*UpdateItemResult, error) {
	if _, hasStatus := req.UpdateData["status_id"]; hasStatus {
		return nil, &validation.ValidationError{
			Field:   "status_id",
			Message: "must be changed via the transition endpoint, not item update",
		}
	}

	// Start transaction first so the read-modify-write is atomic
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Will be ignored if transaction is committed

	// Load existing item inside the transaction (with FOR UPDATE on Postgres for row-level locking)
	originalItem, err := s.loadItemInTx(tx, req.ItemID)
	if err != nil {
		return nil, fmt.Errorf("failed to load item: %w", err)
	}

	// Create a copy for updates
	existingItem := *originalItem

	// Apply validation and updates
	if err = s.validator.ValidateAndApplyUpdates(&existingItem, req.UpdateData, req.UserID); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Convert custom field values to JSON for database storage
	customFieldValuesJSON, err := validation.ConvertCustomFieldValuesToJSON(existingItem.CustomFieldValues)
	if err != nil {
		return nil, err
	}

	// Update the item in database
	now := time.Now()
	_, err = tx.Exec(`
		UPDATE items
		SET workspace_id = ?, title = ?, description = ?, status_id = ?, priority_id = ?, due_date = ?,
		    start_date = ?, end_date = ?,
		    iteration_id = ?, project_id = ?, inherit_project = ?, assignee_id = ?, creator_id = ?,
		    custom_field_values = ?, parent_id = ?, related_work_item_id = ?, story_points = ?, updated_at = ?
		WHERE id = ?
	`, existingItem.WorkspaceID, existingItem.Title, existingItem.Description,
		existingItem.StatusID, existingItem.PriorityID, existingItem.DueDate,
		existingItem.StartDate, existingItem.EndDate,
		existingItem.IterationID, existingItem.ProjectID, existingItem.InheritProject, existingItem.AssigneeID,
		existingItem.CreatorID, customFieldValuesJSON, existingItem.ParentID, existingItem.RelatedWorkItemID,
		existingItem.StoryPoints, now, req.ItemID)

	if err != nil {
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	// Apply milestone-set replace if the validator parsed milestone_ids. The
	// validator stashes ID-only Milestone stubs on existingItem.Milestones
	// when "milestone_ids" appeared in updateData; otherwise the field is
	// untouched and we leave the join table alone.
	var milestoneOldIDs, milestoneNewIDs []int
	hasMilestoneIDs := false
	if _, ok := req.UpdateData["milestone_ids"]; ok {
		hasMilestoneIDs = true

		// Snapshot current milestone IDs in the transaction for history.
		oldRows, err := tx.Query("SELECT milestone_id FROM item_milestones WHERE item_id = ? ORDER BY milestone_id", req.ItemID)
		if err != nil {
			return nil, fmt.Errorf("failed to read existing item_milestones: %w", err)
		}
		for oldRows.Next() {
			var mID int
			if err := oldRows.Scan(&mID); err != nil {
				_ = oldRows.Close()
				return nil, fmt.Errorf("failed to scan existing milestone id: %w", err)
			}
			milestoneOldIDs = append(milestoneOldIDs, mID)
		}
		if err := oldRows.Err(); err != nil {
			_ = oldRows.Close()
			return nil, fmt.Errorf("failed to iterate existing milestone ids: %w", err)
		}
		_ = oldRows.Close()

		if _, err := tx.Exec("DELETE FROM item_milestones WHERE item_id = ?", req.ItemID); err != nil {
			return nil, fmt.Errorf("failed to clear item_milestones: %w", err)
		}
		for _, m := range existingItem.Milestones {
			milestoneNewIDs = append(milestoneNewIDs, m.ID)
			if _, err := tx.Exec(
				"INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?)",
				req.ItemID, m.ID, now,
			); err != nil {
				return nil, fmt.Errorf("failed to attach milestone %d: %w", m.ID, err)
			}
		}
	}

	// Generate and record history entries
	history := s.compareAndGenerateHistory(originalItem, &existingItem, req.UserID)
	if hasMilestoneIDs {
		oldStr := joinIntsCSV(milestoneOldIDs)
		newStr := joinIntsCSV(milestoneNewIDs)
		if oldStr != newStr {
			history = append(history, HistoryEntry{
				ItemID:    req.ItemID,
				UserID:    req.UserID,
				FieldName: "milestones",
				OldValue:  oldStr,
				NewValue:  newStr,
				ChangedAt: now,
			})
		}
	}
	if err = s.recordItemHistory(tx, history); err != nil {
		return nil, fmt.Errorf("failed to record history: %w", err)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Load the updated item with joins for response
	updatedItem, err := s.loadItemWithJoins(req.ItemID)
	if err != nil {
		return nil, fmt.Errorf("failed to load updated item: %w", err)
	}

	// Check if status changed (for event emission)
	statusChanged := s.hasStatusChanged(originalItem, updatedItem)

	return &UpdateItemResult{
		OriginalItem:  originalItem,
		Item:          updatedItem,
		StatusChanged: statusChanged,
		FieldChanges:  history,
	}, nil
}

// loadItemInTx loads an item inside a transaction. On Postgres, appends FOR UPDATE to lock the row.
func (s *ItemUpdateService) loadItemInTx(tx database.Tx, itemID int) (*models.Item, error) {
	query := `
		SELECT id, workspace_id, workspace_item_number, item_type_id, title, description, status_id,
		       priority_id, due_date, start_date, end_date, is_task, iteration_id, project_id, inherit_project,
		       assignee_id, creator_id, custom_field_values, parent_id, related_work_item_id,
		       story_points, created_at, updated_at
		FROM items WHERE id = ?`

	if s.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}

	item, err := scanItemBaseFields(tx.QueryRow(query, itemID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("item not found")
	}
	if err != nil {
		return nil, err
	}

	return item, nil
}

// loadItemWithJoins loads an item with all joined data for response
func (s *ItemUpdateService) loadItemWithJoins(itemID int) (*models.Item, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var statusID, priorityID, projectID sql.NullInt64
	var projectName sql.NullString
	var assigneeID, creatorID sql.NullInt64
	var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail sql.NullString
	var priorityName, priorityIcon, priorityColor sql.NullString

	err := s.db.QueryRow(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title, i.description, i.status_id, i.priority_id,
		       i.is_task, i.project_id, i.inherit_project, i.assignee_id, i.creator_id,
		       i.custom_field_values, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       proj.name as project_name,
		       assignee.first_name || ' ' || assignee.last_name as assignee_name, assignee.email as assignee_email, assignee.avatar_url as assignee_avatar,
		       creator.first_name || ' ' || creator.last_name as creator_name, creator.email as creator_email,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		WHERE i.id = ?
	`, itemID).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &item.Title, &item.Description,
		&statusID, &priorityID, &item.IsTask, &projectID, &item.InheritProject,
		&assigneeID, &creatorID, &customFieldValuesJSON, &item.CreatedAt, &item.UpdatedAt,
		&item.WorkspaceName, &item.WorkspaceKey, &projectName,
		&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail,
		&priorityName, &priorityIcon, &priorityColor,
	)

	if err != nil {
		return nil, err
	}

	// Handle nullable ID fields
	item.StatusID = nullInt64ToIntPtr(statusID)
	item.PriorityID = nullInt64ToIntPtr(priorityID)
	item.ProjectID = nullInt64ToIntPtr(projectID)
	item.AssigneeID = nullInt64ToIntPtr(assigneeID)
	item.CreatorID = nullInt64ToIntPtr(creatorID)

	// Handle nullable string fields
	item.ProjectName = nullStringToString(projectName)
	item.PriorityName = nullStringToString(priorityName)
	item.PriorityIcon = nullStringToString(priorityIcon)
	item.PriorityColor = nullStringToString(priorityColor)
	item.AssigneeName = nullStringToString(assigneeName)
	item.AssigneeEmail = nullStringToString(assigneeEmail)
	item.AssigneeAvatar = nullStringToString(assigneeAvatar)
	item.CreatorName = nullStringToString(creatorName)
	item.CreatorEmail = nullStringToString(creatorEmail)

	parseItemCustomFieldValues(&item, customFieldValuesJSON)

	// Hydrate milestones for the response.
	holder := []models.Item{item}
	if err := repository.NewMilestoneAttachRepository(s.db).LoadForItems(holder); err == nil {
		item = holder[0]
	}

	return &item, nil
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
	addHistory("assignee_id", intPtrToString(original.AssigneeID), intPtrToString(updated.AssigneeID))
	addHistory("creator_id", intPtrToString(original.CreatorID), intPtrToString(updated.CreatorID))
	addHistory("parent_id", intPtrToString(original.ParentID), intPtrToString(updated.ParentID))

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

// RecordItemCreationHistory records the initial values when an item is created
// This ensures that the item history shows the creation event with initial values
func (s *ItemUpdateService) RecordItemCreationHistory(db database.Database, itemID, userID int) error {
	return s.recordItemCreationHistory(db, itemID, userID)
}

// recordItemCreationHistory records the initial values when an item is created
func (s *ItemUpdateService) recordItemCreationHistory(db database.Database, itemID, userID int) error {
	// Load the newly created item to get all its initial values
	item, err := scanItemBaseFields(db.QueryRow(`
		SELECT id, workspace_id, workspace_item_number, item_type_id, title, description, status_id,
		       priority_id, due_date, start_date, end_date, is_task, iteration_id, project_id, inherit_project,
		       assignee_id, creator_id, custom_field_values, parent_id, related_work_item_id,
		       story_points, created_at, updated_at
		FROM items WHERE id = ?
	`, itemID))

	if err != nil {
		return fmt.Errorf("failed to load created item: %w", err)
	}

	// Generate history entries for all initial values
	var history []HistoryEntry
	now := time.Now()

	// Helper to add history entry (old_value is always empty for creation)
	addHistory := func(fieldName, newValue string) {
		if newValue != "" {
			history = append(history, HistoryEntry{
				ItemID:    itemID,
				UserID:    userID,
				FieldName: fieldName,
				OldValue:  "",
				NewValue:  newValue,
				ChangedAt: now,
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
	addHistory("assignee_id", intPtrToString(item.AssigneeID))
	addHistory("creator_id", intPtrToString(item.CreatorID))
	addHistory("parent_id", intPtrToString(item.ParentID))
	addHistory("due_date", timePtrToString(item.DueDate))
	addHistory("start_date", timePtrToString(item.StartDate))
	addHistory("end_date", timePtrToString(item.EndDate))
	addHistory("workspace_id", fmt.Sprintf("%d", item.WorkspaceID))
	addHistory("story_points", float64PtrToString(item.StoryPoints))

	// Record history entries directly (no transaction needed here, caller should manage)
	for _, entry := range history {
		_, err := db.Exec(`
			INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, entry.ItemID, entry.UserID, entry.FieldName, entry.OldValue, entry.NewValue, entry.ChangedAt)
		if err != nil {
			return fmt.Errorf("failed to record creation history: %w", err)
		}
	}

	return nil
}

// recordItemHistory records history entries in the database
func (s *ItemUpdateService) recordItemHistory(tx database.Tx, history []HistoryEntry) error {
	for _, entry := range history {
		_, err := tx.Exec(`
			INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, entry.ItemID, entry.UserID, entry.FieldName, entry.OldValue, entry.NewValue, entry.ChangedAt)
		if err != nil {
			return err
		}
	}
	return nil
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

// nullInt64ToIntPtr converts a sql.NullInt64 to *int
func nullInt64ToIntPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	val := int(n.Int64)
	return &val
}

// nullTimeToTimePtr converts a sql.NullTime to *time.Time
func nullTimeToTimePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	return &n.Time
}

// nullStringToString returns the string value or empty string for a sql.NullString
func nullStringToString(n sql.NullString) string {
	return n.String
}

// parseItemCustomFieldValues parses JSON custom field values into the item
func parseItemCustomFieldValues(item *models.Item, raw sql.NullString) {
	if raw.Valid && raw.String != "" {
		if err := json.Unmarshal([]byte(raw.String), &item.CustomFieldValues); err != nil {
			item.CustomFieldValues = make(map[string]interface{})
		}
	} else {
		item.CustomFieldValues = make(map[string]interface{})
	}
}

// scanItemBaseFields scans the common item base query columns and populates nullable fields.
// The query must select: id, workspace_id, workspace_item_number, item_type_id, title, description,
// status_id, priority_id, due_date, start_date, end_date, is_task, iteration_id,
// project_id, inherit_project, assignee_id, creator_id, custom_field_values, parent_id,
// related_work_item_id, created_at, updated_at
func scanItemBaseFields(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.Item, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID, relatedWorkItemID sql.NullInt64
	var dueDate, startDate, endDate sql.NullTime
	var storyPoints sql.NullFloat64

	err := scanner.Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &iterationID,
		&projectID, &item.InheritProject, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
		&relatedWorkItemID, &storyPoints, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	item.ItemTypeID = nullInt64ToIntPtr(itemTypeID)
	item.ParentID = nullInt64ToIntPtr(parentID)
	item.IterationID = nullInt64ToIntPtr(iterationID)
	item.StatusID = nullInt64ToIntPtr(statusID)
	item.PriorityID = nullInt64ToIntPtr(priorityID)
	item.DueDate = nullTimeToTimePtr(dueDate)
	item.StartDate = nullTimeToTimePtr(startDate)
	item.EndDate = nullTimeToTimePtr(endDate)
	item.ProjectID = nullInt64ToIntPtr(projectID)
	item.AssigneeID = nullInt64ToIntPtr(assigneeID)
	item.CreatorID = nullInt64ToIntPtr(creatorID)
	item.RelatedWorkItemID = nullInt64ToIntPtr(relatedWorkItemID)
	if storyPoints.Valid {
		item.StoryPoints = &storyPoints.Float64
	}

	parseItemCustomFieldValues(&item, customFieldValuesJSON)

	return &item, nil
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

func customFieldValueToString(val interface{}) string {
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
