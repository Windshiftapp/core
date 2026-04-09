package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ItemRepository provides data access methods for items
type ItemRepository struct {
	db database.Database
}

// NewItemRepository creates a new item repository
func NewItemRepository(db database.Database) *ItemRepository {
	return &ItemRepository{db: db}
}

// FindByID loads an item by ID with all fields (no joins)
func (r *ItemRepository) FindByID(id int) (*models.Item, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, milestoneID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID, relatedWorkItemID sql.NullInt64
	var dueDate, startDate, endDate sql.NullTime
	var storyPoints sql.NullFloat64

	err := r.db.QueryRow(`
		SELECT id, workspace_id, workspace_item_number, item_type_id, title, description, status_id,
		       priority_id, due_date, start_date, end_date, is_task, milestone_id, iteration_id, project_id, inherit_project,
		       assignee_id, creator_id, custom_field_values, parent_id, related_work_item_id,
		       story_points, frac_index, created_at, updated_at
		FROM items WHERE id = ?
	`, id).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &milestoneID, &iterationID,
		&projectID, &item.InheritProject, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
		&relatedWorkItemID, &storyPoints, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find item: %w", err)
	}

	// Handle nullable fields
	assignNullableInt(&item.ItemTypeID, itemTypeID)
	assignNullableInt(&item.ParentID, parentID)
	assignNullableInt(&item.StatusID, statusID)
	assignNullableInt(&item.PriorityID, priorityID)
	assignNullableInt(&item.MilestoneID, milestoneID)
	assignNullableInt(&item.IterationID, iterationID)
	assignNullableInt(&item.ProjectID, projectID)
	assignNullableInt(&item.AssigneeID, assigneeID)
	assignNullableInt(&item.CreatorID, creatorID)
	assignNullableInt(&item.RelatedWorkItemID, relatedWorkItemID)

	if dueDate.Valid {
		item.DueDate = &dueDate.Time
	}
	if startDate.Valid {
		item.StartDate = &startDate.Time
	}
	if endDate.Valid {
		item.EndDate = &endDate.Time
	}
	if storyPoints.Valid {
		item.StoryPoints = &storyPoints.Float64
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

// ItemWithWorkspaceStatus includes workspace active status for permission checks
type ItemWithWorkspaceStatus struct {
	*models.Item
	WorkspaceActive bool
}

// FindByIDWithDetails loads an item with all joined data
// This is the consolidated method for the ~30 duplicate JOIN queries throughout items.go
func (r *ItemRepository) FindByIDWithDetails(id int) (*models.Item, error) {
	result, err := r.FindByIDWithWorkspaceStatus(id)
	if err != nil {
		return nil, err
	}
	return result.Item, nil
}

// FindByIDWithWorkspaceStatus loads an item with all joined data including workspace active status
func (r *ItemRepository) FindByIDWithWorkspaceStatus(id int) (*ItemWithWorkspaceStatus, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, milestoneID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID, timeProjectID sql.NullInt64
	var dueDate, startDate, endDate sql.NullTime
	var workspaceActive bool

	// Joined data
	var milestoneName, projectName, iterationName, timeProjectName, parentTitle sql.NullString
	var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail sql.NullString
	var priorityName, priorityIcon, priorityColor sql.NullString
	var statusName sql.NullString
	var itemTypeName sql.NullString
	// Related work item data (for personal tasks)
	var relatedWorkItemID sql.NullInt64
	var relatedWorkItemTitle, relatedWorkItemWorkspaceKey sql.NullString
	var relatedWorkItemWorkspaceID, relatedWorkItemNumber sql.NullInt64
	// Portal-specific fields
	var creatorPortalCustomerID, channelID, requestTypeID sql.NullInt64

	var storyPoints sql.NullFloat64

	err := r.db.QueryRow(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.start_date, i.end_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.inherit_project, i.time_project_id, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.story_points, i.frac_index, i.created_at, i.updated_at,
		       i.creator_portal_customer_id, i.channel_id, i.request_type_id,
		       w.name as workspace_name, w.key as workspace_key, w.active as workspace_active,
		       m.name as milestone_name,
		       iter.name as iteration_name,
		       proj.name as project_name,
		       tp.name as time_project_name,
		       p.title as parent_title,
		       assignee.first_name || ' ' || assignee.last_name as assignee_name, assignee.email as assignee_email, assignee.avatar_url as assignee_avatar,
		       creator.first_name || ' ' || creator.last_name as creator_name, creator.email as creator_email,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name,
		       it.name as item_type_name,
		       i.related_work_item_id,
		       rw.title as related_work_item_title,
		       rw_ws.key as related_work_item_workspace_key,
		       rw.workspace_id as related_work_item_workspace_id,
		       rw.workspace_item_number as related_work_item_number
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN items p ON i.parent_id = p.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN items rw ON i.related_work_item_id = rw.id
		LEFT JOIN workspaces rw_ws ON rw.workspace_id = rw_ws.id
		WHERE i.id = ?
	`, id).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &milestoneID, &iterationID,
		&projectID, &item.InheritProject, &timeProjectID, &assigneeID, &creatorID, &customFieldValuesJSON,
		&parentID, &storyPoints, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
		&creatorPortalCustomerID, &channelID, &requestTypeID,
		&item.WorkspaceName, &item.WorkspaceKey, &workspaceActive,
		&milestoneName, &iterationName, &projectName, &timeProjectName, &parentTitle,
		&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail,
		&priorityName, &priorityIcon, &priorityColor,
		&statusName,
		&itemTypeName,
		&relatedWorkItemID,
		&relatedWorkItemTitle,
		&relatedWorkItemWorkspaceKey,
		&relatedWorkItemWorkspaceID,
		&relatedWorkItemNumber,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find item with details: %w", err)
	}

	// Handle nullable ID fields
	assignNullableInt(&item.ItemTypeID, itemTypeID)
	assignNullableInt(&item.ParentID, parentID)
	assignNullableInt(&item.StatusID, statusID)
	assignNullableInt(&item.PriorityID, priorityID)
	assignNullableInt(&item.MilestoneID, milestoneID)
	assignNullableInt(&item.IterationID, iterationID)
	assignNullableInt(&item.ProjectID, projectID)
	assignNullableInt(&item.TimeProjectID, timeProjectID)
	assignNullableInt(&item.AssigneeID, assigneeID)
	assignNullableInt(&item.CreatorID, creatorID)

	// Portal-specific fields
	assignNullableInt(&item.CreatorPortalCustomerID, creatorPortalCustomerID)
	assignNullableInt(&item.ChannelID, channelID)
	assignNullableInt(&item.RequestTypeID, requestTypeID)

	if dueDate.Valid {
		item.DueDate = &dueDate.Time
	}
	if startDate.Valid {
		item.StartDate = &startDate.Time
	}
	if endDate.Valid {
		item.EndDate = &endDate.Time
	}
	if storyPoints.Valid {
		item.StoryPoints = &storyPoints.Float64
	}

	// Handle nullable string fields from joins
	assignNullableString(&item.MilestoneName, milestoneName)
	assignNullableString(&item.IterationName, iterationName)
	assignNullableString(&item.ProjectName, projectName)
	assignNullableString(&item.TimeProjectName, timeProjectName)
	assignNullableString(&item.ParentTitle, parentTitle)
	assignNullableString(&item.AssigneeName, assigneeName)
	assignNullableString(&item.AssigneeEmail, assigneeEmail)
	assignNullableString(&item.AssigneeAvatar, assigneeAvatar)
	assignNullableString(&item.CreatorName, creatorName)
	assignNullableString(&item.CreatorEmail, creatorEmail)
	assignNullableString(&item.PriorityName, priorityName)
	assignNullableString(&item.PriorityIcon, priorityIcon)
	assignNullableString(&item.PriorityColor, priorityColor)
	assignNullableString(&item.StatusName, statusName)
	assignNullableString(&item.ItemTypeName, itemTypeName)

	// Handle related work item fields (for personal tasks)
	assignNullableInt(&item.RelatedWorkItemID, relatedWorkItemID)
	assignNullableString(&item.RelatedWorkItemTitle, relatedWorkItemTitle)
	assignNullableString(&item.RelatedWorkItemWorkspaceKey, relatedWorkItemWorkspaceKey)
	if relatedWorkItemWorkspaceID.Valid {
		item.RelatedWorkItemWorkspaceID = int(relatedWorkItemWorkspaceID.Int64)
	}
	if relatedWorkItemNumber.Valid {
		item.RelatedWorkItemNumber = int(relatedWorkItemNumber.Int64)
	}

	// Parse custom field values
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
			item.CustomFieldValues = make(map[string]interface{})
		}
	} else {
		item.CustomFieldValues = make(map[string]interface{})
	}

	return &ItemWithWorkspaceStatus{Item: &item, WorkspaceActive: workspaceActive}, nil
}

// GetWorkspaceID returns just the workspace_id for an item (frequently needed for permission checks)
func (r *ItemRepository) GetWorkspaceID(itemID int) (int, error) {
	var workspaceID int
	err := r.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace id: %w", err)
	}
	return workspaceID, nil
}

// GetNextWorkspaceItemNumber returns the next item number for a workspace (atomic increment)
func (r *ItemRepository) GetNextWorkspaceItemNumber(tx database.Tx, workspaceID int) (int, error) {
	var nextNumber int
	err := tx.QueryRow(`
		SELECT COALESCE(MAX(workspace_item_number), 0) + 1
		FROM items
		WHERE workspace_id = ?
	`, workspaceID).Scan(&nextNumber)
	if err != nil {
		return 0, fmt.Errorf("failed to get next item number: %w", err)
	}
	return nextNumber, nil
}

// Create inserts a new item and returns its ID
func (r *ItemRepository) Create(tx database.Tx, item *models.Item) (int, error) {
	customFieldValuesJSON, err := marshalCustomFields(item.CustomFieldValues)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	var id int64
	err = tx.QueryRow(`
		INSERT INTO items (
			workspace_id, workspace_item_number, item_type_id, title, description, status_id,
			priority_id, due_date, start_date, end_date, is_task, milestone_id, iteration_id, project_id, inherit_project,
			assignee_id, creator_id, custom_field_values, parent_id, related_work_item_id,
			story_points, frac_index, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		item.WorkspaceID, item.WorkspaceItemNumber, item.ItemTypeID, item.Title, item.Description,
		item.StatusID, item.PriorityID, item.DueDate, item.StartDate, item.EndDate, item.IsTask, item.MilestoneID,
		item.IterationID, item.ProjectID, item.InheritProject, item.AssigneeID, item.CreatorID,
		customFieldValuesJSON, item.ParentID, item.RelatedWorkItemID,
		item.StoryPoints, item.FracIndex, now, now,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create item: %w", err)
	}

	return int(id), nil
}

// Update updates an existing item
func (r *ItemRepository) Update(tx database.Tx, item *models.Item) error {
	customFieldValuesJSON, err := marshalCustomFields(item.CustomFieldValues)
	if err != nil {
		return err
	}

	now := time.Now()
	_, err = tx.Exec(`
		UPDATE items
		SET workspace_id = ?, title = ?, description = ?, status_id = ?, priority_id = ?,
		    due_date = ?, start_date = ?, end_date = ?, milestone_id = ?, iteration_id = ?, project_id = ?, inherit_project = ?,
		    assignee_id = ?, creator_id = ?, custom_field_values = ?, parent_id = ?,
		    related_work_item_id = ?, story_points = ?, updated_at = ?
		WHERE id = ?
	`,
		item.WorkspaceID, item.Title, item.Description, item.StatusID, item.PriorityID,
		item.DueDate, item.StartDate, item.EndDate, item.MilestoneID, item.IterationID, item.ProjectID, item.InheritProject,
		item.AssigneeID, item.CreatorID, customFieldValuesJSON, item.ParentID,
		item.RelatedWorkItemID, item.StoryPoints, now, item.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

// allowedItemColumns is the whitelist of columns that UpdateFields may touch.
var allowedItemColumns = map[string]bool{
	"title": true, "description": true, "status_id": true, "priority_id": true,
	"due_date": true, "start_date": true, "end_date": true,
	"milestone_id": true, "iteration_id": true, "project_id": true, "inherit_project": true,
	"assignee_id": true, "creator_id": true, "custom_field_values": true,
	"parent_id": true, "related_work_item_id": true, "item_type_id": true,
	"frac_index": true, "is_task": true, "time_project_id": true,
	"story_points": true,
}

// UpdateFields updates only the specified columns of an item.
// Keys must be valid item column names; unknown keys return an error.
func (r *ItemRepository) UpdateFields(tx database.Tx, itemID int, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(fields)+1)
	args := make([]interface{}, 0, len(fields)+2)

	for col, val := range fields {
		if !allowedItemColumns[col] {
			return fmt.Errorf("unknown item column: %s", col)
		}
		setClauses = append(setClauses, col+" = ?")
		args = append(args, val)
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, time.Now())
	args = append(args, itemID)

	query := "UPDATE items SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	_, err := tx.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update item fields: %w", err)
	}
	return nil
}

// Delete removes an item by ID
func (r *ItemRepository) Delete(tx database.Tx, id int) error {
	_, err := tx.Exec("DELETE FROM items WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}
	return nil
}

// DeleteItemLinks removes all links where the item is source or target
func (r *ItemRepository) DeleteItemLinks(tx database.Tx, itemID int) error {
	_, err := tx.Exec(`
		DELETE FROM item_links
		WHERE (source_type = 'item' AND source_id = ?) OR (target_type = 'item' AND target_id = ?)
	`, itemID, itemID)
	if err != nil {
		return fmt.Errorf("failed to delete item links: %w", err)
	}
	return nil
}

// ClearWorklogItemReferences clears item references from worklogs
func (r *ItemRepository) ClearWorklogItemReferences(tx database.Tx, itemID int) error {
	_, err := tx.Exec("UPDATE time_worklogs SET item_id = NULL WHERE item_id = ?", itemID)
	if err != nil {
		return fmt.Errorf("failed to clear worklog references: %w", err)
	}
	return nil
}

// GetParentID returns the parent_id for an item
func (r *ItemRepository) GetParentID(itemID int) (*int, error) {
	var parentID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT parent_id FROM items WHERE id = ?
	`, itemID).Scan(&parentID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get parent id: %w", err)
	}

	var result *int
	if parentID.Valid {
		val := int(parentID.Int64)
		result = &val
	}
	return result, nil
}

// Exists checks if an item exists
func (r *ItemRepository) Exists(id int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM items WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check item existence: %w", err)
	}
	return exists, nil
}

// Helper functions

func assignNullableInt(dest **int, src sql.NullInt64) {
	if src.Valid {
		val := int(src.Int64)
		*dest = &val
	}
}

func assignNullableString(dest *string, src sql.NullString) {
	if src.Valid {
		*dest = src.String
	}
}

func marshalCustomFields(customFields map[string]interface{}) (sql.NullString, error) {
	if len(customFields) == 0 {
		return sql.NullString{Valid: false}, nil
	}

	data, err := json.Marshal(customFields)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to marshal custom fields: %w", err)
	}

	return sql.NullString{String: string(data), Valid: true}, nil
}
