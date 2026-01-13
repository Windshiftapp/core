package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/validation"
)

// ItemUpdateService handles item update operations with validation, history tracking, and event emission
type ItemUpdateService struct {
	db        database.Database
	validator *validation.ItemFieldValidator
}

// NewItemUpdateService creates a new item update service
func NewItemUpdateService(db database.Database) *ItemUpdateService {
	return &ItemUpdateService{
		db:        db,
		validator: validation.NewItemFieldValidator(db),
	}
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

// UpdateItem updates an item with validation, transaction safety, and history tracking
func (s *ItemUpdateService) UpdateItem(req UpdateItemRequest) (*UpdateItemResult, error) {
	// Load existing item
	originalItem, err := s.loadItem(req.ItemID)
	if err != nil {
		return nil, fmt.Errorf("failed to load item: %w", err)
	}

	// Create a copy for updates
	existingItem := *originalItem

	// Apply validation and updates
	if err := s.validator.ValidateAndApplyUpdates(&existingItem, req.UpdateData, req.UserID); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Convert custom field values to JSON for database storage
	customFieldValuesJSON, err := validation.ConvertCustomFieldValuesToJSON(existingItem.CustomFieldValues)
	if err != nil {
		return nil, err
	}

	// Start transaction for atomic update + history recording
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback() // Will be ignored if transaction is committed

	// Update the item in database
	now := time.Now()
	_, err = tx.Exec(`
		UPDATE items
		SET workspace_id = ?, title = ?, description = ?, status_id = ?, priority_id = ?, due_date = ?,
		    milestone_id = ?, iteration_id = ?, project_id = ?, inherit_project = ?, assignee_id = ?, creator_id = ?,
		    custom_field_values = ?, parent_id = ?, related_work_item_id = ?, updated_at = ?
		WHERE id = ?
	`, existingItem.WorkspaceID, existingItem.Title, existingItem.Description,
		existingItem.StatusID, existingItem.PriorityID, existingItem.DueDate, existingItem.MilestoneID,
		existingItem.IterationID, existingItem.ProjectID, existingItem.InheritProject, existingItem.AssigneeID,
		existingItem.CreatorID, customFieldValuesJSON, existingItem.ParentID, existingItem.RelatedWorkItemID,
		now, req.ItemID)

	if err != nil {
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	// Generate and record history entries
	history := s.compareAndGenerateHistory(originalItem, &existingItem, req.UserID)
	if err := s.recordItemHistory(tx, history); err != nil {
		return nil, fmt.Errorf("failed to record history: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
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

// loadItem loads an item by ID with all fields
func (s *ItemUpdateService) loadItem(itemID int) (*models.Item, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, milestoneID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID, relatedWorkItemID sql.NullInt64
	var dueDate sql.NullTime

	err := s.db.QueryRow(`
		SELECT id, workspace_id, workspace_item_number, item_type_id, title, description, status_id,
		       priority_id, due_date, is_task, milestone_id, iteration_id, project_id, inherit_project,
		       assignee_id, creator_id, custom_field_values, parent_id, related_work_item_id,
		       created_at, updated_at
		FROM items WHERE id = ?
	`, itemID).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &item.IsTask, &milestoneID, &iterationID,
		&projectID, &item.InheritProject, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
		&relatedWorkItemID, &item.CreatedAt, &item.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item not found")
	}
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if itemTypeID.Valid {
		val := int(itemTypeID.Int64)
		item.ItemTypeID = &val
	}
	if parentID.Valid {
		val := int(parentID.Int64)
		item.ParentID = &val
	}
	if milestoneID.Valid {
		val := int(milestoneID.Int64)
		item.MilestoneID = &val
	}
	if iterationID.Valid {
		val := int(iterationID.Int64)
		item.IterationID = &val
	}
	if statusID.Valid {
		val := int(statusID.Int64)
		item.StatusID = &val
	}
	if priorityID.Valid {
		val := int(priorityID.Int64)
		item.PriorityID = &val
	}
	if dueDate.Valid {
		item.DueDate = &dueDate.Time
	}
	if projectID.Valid {
		val := int(projectID.Int64)
		item.ProjectID = &val
	}
	if assigneeID.Valid {
		val := int(assigneeID.Int64)
		item.AssigneeID = &val
	}
	if creatorID.Valid {
		val := int(creatorID.Int64)
		item.CreatorID = &val
	}
	if relatedWorkItemID.Valid {
		val := int(relatedWorkItemID.Int64)
		item.RelatedWorkItemID = &val
	}

	// Parse custom field values
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
			item.CustomFieldValues = make(map[string]interface{})
		}
	} else {
		item.CustomFieldValues = make(map[string]interface{})
	}

	return &item, nil
}

// loadItemWithJoins loads an item with all joined data for response
func (s *ItemUpdateService) loadItemWithJoins(itemID int) (*models.Item, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var milestoneID, statusID, priorityID, projectID sql.NullInt64
	var milestoneName, projectName sql.NullString
	var assigneeID, creatorID sql.NullInt64
	var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail sql.NullString
	var priorityName, priorityIcon, priorityColor sql.NullString

	err := s.db.QueryRow(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title, i.description, i.status_id, i.priority_id,
		       i.is_task, i.milestone_id, i.project_id, i.inherit_project, i.assignee_id, i.creator_id,
		       i.custom_field_values, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       m.name as milestone_name, proj.name as project_name,
		       assignee.first_name || ' ' || assignee.last_name as assignee_name, assignee.email as assignee_email, assignee.avatar_url as assignee_avatar,
		       creator.first_name || ' ' || creator.last_name as creator_name, creator.email as creator_email,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		WHERE i.id = ?
	`, itemID).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &item.Title, &item.Description,
		&statusID, &priorityID, &item.IsTask, &milestoneID, &projectID, &item.InheritProject,
		&assigneeID, &creatorID, &customFieldValuesJSON, &item.CreatedAt, &item.UpdatedAt,
		&item.WorkspaceName, &item.WorkspaceKey, &milestoneName, &projectName,
		&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail,
		&priorityName, &priorityIcon, &priorityColor,
	)

	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if milestoneID.Valid {
		val := int(milestoneID.Int64)
		item.MilestoneID = &val
	}
	if milestoneName.Valid {
		item.MilestoneName = milestoneName.String
	}
	if statusID.Valid {
		val := int(statusID.Int64)
		item.StatusID = &val
	}
	if priorityID.Valid {
		val := int(priorityID.Int64)
		item.PriorityID = &val
	}
	if priorityName.Valid {
		item.PriorityName = priorityName.String
	}
	if priorityIcon.Valid {
		item.PriorityIcon = priorityIcon.String
	}
	if priorityColor.Valid {
		item.PriorityColor = priorityColor.String
	}
	if projectID.Valid {
		val := int(projectID.Int64)
		item.ProjectID = &val
	}
	if projectName.Valid {
		item.ProjectName = projectName.String
	}
	if assigneeID.Valid {
		val := int(assigneeID.Int64)
		item.AssigneeID = &val
	}
	if creatorID.Valid {
		val := int(creatorID.Int64)
		item.CreatorID = &val
	}
	if assigneeName.Valid {
		item.AssigneeName = assigneeName.String
	}
	if assigneeEmail.Valid {
		item.AssigneeEmail = assigneeEmail.String
	}
	if assigneeAvatar.Valid {
		item.AssigneeAvatar = assigneeAvatar.String
	}
	if creatorName.Valid {
		item.CreatorName = creatorName.String
	}
	if creatorEmail.Valid {
		item.CreatorEmail = creatorEmail.String
	}

	// Parse custom field values
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
			item.CustomFieldValues = make(map[string]interface{})
		}
	} else {
		item.CustomFieldValues = make(map[string]interface{})
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
	addHistory("milestone_id", intPtrToString(original.MilestoneID), intPtrToString(updated.MilestoneID))
	addHistory("iteration_id", intPtrToString(original.IterationID), intPtrToString(updated.IterationID))
	addHistory("project_id", intPtrToString(original.ProjectID), intPtrToString(updated.ProjectID))
	addHistory("assignee_id", intPtrToString(original.AssigneeID), intPtrToString(updated.AssigneeID))
	addHistory("creator_id", intPtrToString(original.CreatorID), intPtrToString(updated.CreatorID))
	addHistory("parent_id", intPtrToString(original.ParentID), intPtrToString(updated.ParentID))

	// Compare due date
	addHistory("due_date", timePtrToString(original.DueDate), timePtrToString(updated.DueDate))

	// Compare workspace_id (simple int)
	if original.WorkspaceID != updated.WorkspaceID {
		addHistory("workspace_id", fmt.Sprintf("%d", original.WorkspaceID), fmt.Sprintf("%d", updated.WorkspaceID))
	}

	// Compare inherit_project (bool)
	if original.InheritProject != updated.InheritProject {
		addHistory("inherit_project", fmt.Sprintf("%t", original.InheritProject), fmt.Sprintf("%t", updated.InheritProject))
	}

	return history
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
