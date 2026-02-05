package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
)

// GetChildren returns direct children of an item
func (r *ItemRepository) GetChildren(parentID int) ([]*models.Item, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.inherit_project, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name,
		       it.name as item_type_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.parent_id = ?
		ORDER BY i.frac_index
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get children: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanItemsWithDetails(rows)
}

// GetDescendants returns all descendants of an item using recursive CTE
func (r *ItemRepository) GetDescendants(parentID int) ([]*models.Item, error) {
	rows, err := r.db.Query(`
		WITH RECURSIVE descendants AS (
			SELECT id, parent_id, 1 as level
			FROM items
			WHERE parent_id = ?
			UNION ALL
			SELECT i.id, i.parent_id, d.level + 1
			FROM items i
			INNER JOIN descendants d ON i.parent_id = d.id
		)
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.inherit_project, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name,
		       it.name as item_type_name,
		       d.level
		FROM items i
		INNER JOIN descendants d ON i.id = d.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		ORDER BY d.level, i.frac_index
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get descendants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanItemsWithDetailsAndLevel(rows)
}

// GetAncestors returns all ancestors of an item (path to root)
func (r *ItemRepository) GetAncestors(itemID int) ([]*models.Item, error) {
	rows, err := r.db.Query(`
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id, 0 as level
			FROM items
			WHERE id = ?
			UNION ALL
			SELECT i.id, i.parent_id, a.level + 1
			FROM items i
			INNER JOIN ancestors a ON i.id = a.parent_id
		)
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.inherit_project, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name,
		       it.name as item_type_name
		FROM items i
		INNER JOIN ancestors a ON i.id = a.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE a.level > 0
		ORDER BY a.level DESC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ancestors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanItemsWithDetails(rows)
}

// GetRootItems returns all root items (no parent) for a workspace
func (r *ItemRepository) GetRootItems(workspaceID int) ([]*models.Item, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.inherit_project, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name,
		       it.name as item_type_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.workspace_id = ? AND i.parent_id IS NULL
		ORDER BY i.frac_index
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get root items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanItemsWithDetails(rows)
}

// GetDescendantIDs returns just the IDs of all descendants (for bulk operations like delete)
func (r *ItemRepository) GetDescendantIDs(parentID int) ([]int, error) {
	rows, err := r.db.Query(`
		WITH RECURSIVE descendants AS (
			SELECT id FROM items WHERE parent_id = ?
			UNION ALL
			SELECT i.id FROM items i
			INNER JOIN descendants d ON i.parent_id = d.id
		)
		SELECT id FROM descendants
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get descendant ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan descendant id: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// UpdateParent updates the parent_id for an item
func (r *ItemRepository) UpdateParent(tx database.Tx, itemID int, newParentID *int) error {
	var err error
	if newParentID == nil {
		_, err = tx.Exec(`UPDATE items SET parent_id = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, itemID)
	} else {
		_, err = tx.Exec(`UPDATE items SET parent_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, *newParentID, itemID)
	}
	if err != nil {
		return fmt.Errorf("failed to update parent: %w", err)
	}
	return nil
}

// Helper function to scan items with details from rows
func scanItemsWithDetails(rows *sql.Rows) ([]*models.Item, error) {
	var items []*models.Item

	for rows.Next() {
		item, err := scanItemWithDetailsRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return items, nil
}

// Helper function to scan items with details and level from rows
func scanItemsWithDetailsAndLevel(rows *sql.Rows) ([]*models.Item, error) {
	var items []*models.Item

	for rows.Next() {
		var item models.Item
		var customFieldValuesJSON sql.NullString
		var itemTypeID, parentID, statusID, milestoneID, iterationID, projectID, priorityID sql.NullInt64
		var assigneeID, creatorID sql.NullInt64
		var dueDate sql.NullTime
		var priorityName, priorityIcon, priorityColor sql.NullString
		var statusName sql.NullString
		var itemTypeName sql.NullString
		var level int // level is computed from the CTE, not stored

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
			&statusID, &priorityID, &dueDate, &item.IsTask, &milestoneID, &iterationID,
			&projectID, &item.InheritProject, &assigneeID, &creatorID, &customFieldValuesJSON,
			&parentID, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
			&item.WorkspaceName, &item.WorkspaceKey,
			&priorityName, &priorityIcon, &priorityColor,
			&statusName,
			&itemTypeName,
			&level,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan item row: %w", err)
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

		if dueDate.Valid {
			item.DueDate = &dueDate.Time
		}

		assignNullableString(&item.PriorityName, priorityName)
		assignNullableString(&item.PriorityIcon, priorityIcon)
		assignNullableString(&item.PriorityColor, priorityColor)
		assignNullableString(&item.StatusName, statusName)
		assignNullableString(&item.ItemTypeName, itemTypeName)

		// Parse custom field values
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
				item.CustomFieldValues = make(map[string]interface{})
			}
		} else {
			item.CustomFieldValues = make(map[string]interface{})
		}

		// level is computed but not stored in Item model (could be added if needed)
		_ = level
		items = append(items, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return items, nil
}

// scanItemWithDetailsRow scans a single row with item details
func scanItemWithDetailsRow(rows *sql.Rows) (*models.Item, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, milestoneID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID sql.NullInt64
	var dueDate sql.NullTime
	var priorityName, priorityIcon, priorityColor sql.NullString
	var statusName sql.NullString
	var itemTypeName sql.NullString

	err := rows.Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &item.IsTask, &milestoneID, &iterationID,
		&projectID, &item.InheritProject, &assigneeID, &creatorID, &customFieldValuesJSON,
		&parentID, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
		&item.WorkspaceName, &item.WorkspaceKey,
		&priorityName, &priorityIcon, &priorityColor,
		&statusName,
		&itemTypeName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan item row: %w", err)
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

	if dueDate.Valid {
		item.DueDate = &dueDate.Time
	}

	assignNullableString(&item.PriorityName, priorityName)
	assignNullableString(&item.PriorityIcon, priorityIcon)
	assignNullableString(&item.PriorityColor, priorityColor)
	assignNullableString(&item.StatusName, statusName)
	assignNullableString(&item.ItemTypeName, itemTypeName)

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
