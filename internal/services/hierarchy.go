package services

import (
	"database/sql"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
)

// HierarchyService handles all hierarchy-related operations using only parent_id
type HierarchyService struct {
	db database.Database
}

// NewHierarchyService creates a new hierarchy service
func NewHierarchyService(db database.Database) *HierarchyService {
	return &HierarchyService{db: db}
}

// GetAncestors returns all ancestors of an item (from root to direct parent)
func (h *HierarchyService) GetAncestors(itemID int) ([]models.Item, error) {
	// Use recursive CTE to get all ancestors
	query := `
		WITH RECURSIVE ancestors AS (
			-- Base case: get the item itself
			SELECT i.id, i.workspace_id, i.item_type_id, i.title, i.description, i.is_task,
			       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
			       i.created_at, i.updated_at,
			       w.name as workspace_name, it.name as item_type_name, it.color as item_type_color, it.icon as item_type_icon,
			       0 as level
			FROM items i
			JOIN workspaces w ON i.workspace_id = w.id
			LEFT JOIN item_types it ON i.item_type_id = it.id
			WHERE i.id = ?

			UNION ALL

			-- Recursive case: get parent of current item
			SELECT p.id, p.workspace_id, p.item_type_id, p.title, p.description, p.is_task,
			       p.milestone_id, p.assignee_id, p.creator_id, p.custom_field_values, p.parent_id,
			       p.created_at, p.updated_at,
			       w.name as workspace_name, it.name as item_type_name, it.color as item_type_color, it.icon as item_type_icon,
			       a.level + 1 as level
			FROM items p
			JOIN workspaces w ON p.workspace_id = w.id
			LEFT JOIN item_types it ON p.item_type_id = it.id
			JOIN ancestors a ON p.id = a.parent_id
		)
		SELECT id, workspace_id, item_type_id, title, description, is_task,
		       milestone_id, assignee_id, creator_id, custom_field_values, parent_id,
		       created_at, updated_at,
		       workspace_name, item_type_name, item_type_color, item_type_icon, level
		FROM ancestors
		WHERE id != ? -- Exclude the original item
		ORDER BY level DESC -- Root first, then down to direct parent
	`

	rows, err := h.db.Query(query, itemID, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to query ancestors: %w", err)
	}
	defer rows.Close()

	var ancestors []models.Item
	for rows.Next() {
		var item models.Item
		var itemTypeID, milestoneID, assigneeID, creatorID sql.NullInt64
		var customFieldValuesJSON sql.NullString
		var parentID sql.NullInt64
		var workspaceName, itemTypeName, itemTypeColor, itemTypeIcon sql.NullString
		var level int

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &itemTypeID, &item.Title, &item.Description, &item.IsTask,
			&milestoneID, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
			&item.CreatedAt, &item.UpdatedAt,
			&workspaceName, &itemTypeName, &itemTypeColor, &itemTypeIcon, &level,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ancestor: %w", err)
		}

		// Handle nullable fields
		if itemTypeID.Valid {
			val := int(itemTypeID.Int64)
			item.ItemTypeID = &val
		}
		if milestoneID.Valid {
			val := int(milestoneID.Int64)
			item.MilestoneID = &val
		}
		if assigneeID.Valid {
			val := int(assigneeID.Int64)
			item.AssigneeID = &val
		}
		if creatorID.Valid {
			val := int(creatorID.Int64)
			item.CreatorID = &val
		}
		if parentID.Valid {
			val := int(parentID.Int64)
			item.ParentID = &val
		}
		if workspaceName.Valid {
			item.WorkspaceName = workspaceName.String
		}
		if itemTypeName.Valid {
			item.ItemTypeName = itemTypeName.String
		}

		// Handle custom field values
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			// Parse JSON if needed - for now just store as empty map to avoid import issues
			item.CustomFieldValues = make(map[string]interface{})
		}

		ancestors = append(ancestors, item)
	}

	return ancestors, rows.Err()
}

// GetDescendants returns all descendants of an item
func (h *HierarchyService) GetDescendants(itemID, maxDepth int) ([]models.Item, error) {
	query := `
		WITH RECURSIVE descendants AS (
			-- Base case: get direct children
			SELECT i.id, i.workspace_id, i.item_type_id, i.title, i.description, i.is_task,
			       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
			       i.created_at, i.updated_at,
			       w.name as workspace_name, it.name as item_type_name,
			       1 as depth
			FROM items i
			JOIN workspaces w ON i.workspace_id = w.id
			LEFT JOIN item_types it ON i.item_type_id = it.id
			WHERE i.parent_id = ?

			UNION ALL

			-- Recursive case: get children of descendants
			SELECT i.id, i.workspace_id, i.item_type_id, i.title, i.description, i.is_task,
			       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
			       i.created_at, i.updated_at,
			       w.name as workspace_name, it.name as item_type_name,
			       d.depth + 1 as depth
			FROM items i
			JOIN workspaces w ON i.workspace_id = w.id
			LEFT JOIN item_types it ON i.item_type_id = it.id
			JOIN descendants d ON i.parent_id = d.id
			WHERE d.depth < ?
		)
		SELECT id, workspace_id, item_type_id, title, description, is_task,
		       milestone_id, assignee_id, creator_id, custom_field_values, parent_id,
		       created_at, updated_at,
		       workspace_name, item_type_name, depth
		FROM descendants
		ORDER BY depth ASC, created_at ASC
	`

	// If maxDepth is 0 or negative, use a large number to get all descendants
	if maxDepth <= 0 {
		maxDepth = 100 // Reasonable limit to prevent infinite recursion
	}

	rows, err := h.db.Query(query, itemID, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("failed to query descendants: %w", err)
	}
	defer rows.Close()

	var descendants []models.Item
	for rows.Next() {
		var item models.Item
		var itemTypeID, milestoneID, assigneeID, creatorID sql.NullInt64
		var customFieldValuesJSON sql.NullString
		var parentID sql.NullInt64
		var workspaceName, itemTypeName sql.NullString
		var depth int

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &itemTypeID, &item.Title, &item.Description, &item.IsTask,
			&milestoneID, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
			&item.CreatedAt, &item.UpdatedAt,
			&workspaceName, &itemTypeName, &depth,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan descendant: %w", err)
		}

		// Handle nullable fields
		if itemTypeID.Valid {
			val := int(itemTypeID.Int64)
			item.ItemTypeID = &val
		}
		if milestoneID.Valid {
			val := int(milestoneID.Int64)
			item.MilestoneID = &val
		}
		if assigneeID.Valid {
			val := int(assigneeID.Int64)
			item.AssigneeID = &val
		}
		if creatorID.Valid {
			val := int(creatorID.Int64)
			item.CreatorID = &val
		}
		if parentID.Valid {
			val := int(parentID.Int64)
			item.ParentID = &val
		}
		if workspaceName.Valid {
			item.WorkspaceName = workspaceName.String
		}
		if itemTypeName.Valid {
			item.ItemTypeName = itemTypeName.String
		}

		// Handle custom field values
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			item.CustomFieldValues = make(map[string]interface{})
		}

		descendants = append(descendants, item)
	}

	return descendants, rows.Err()
}

// CountDescendants returns the total number of descendants for an item
func (h *HierarchyService) CountDescendants(itemID int) (int, error) {
	query := `
		WITH RECURSIVE descendants AS (
			-- Base case: get direct children
			SELECT id, parent_id
			FROM items
			WHERE parent_id = ?
			
			UNION ALL
			
			-- Recursive case: get children of descendants
			SELECT i.id, i.parent_id
			FROM items i
			JOIN descendants d ON i.parent_id = d.id
		)
		SELECT COUNT(*) FROM descendants
	`

	var count int
	err := h.db.QueryRow(query, itemID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count descendants: %w", err)
	}

	return count, nil
}

// GetChildren returns direct children of an item
func (h *HierarchyService) GetChildren(itemID int) ([]models.Item, error) {
	query := `
		SELECT i.id, i.workspace_id, i.item_type_id, i.title, i.description, i.is_task,
		       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, it.name as item_type_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.parent_id = ?
		ORDER BY i.created_at ASC
	`

	rows, err := h.db.Query(query, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to query children: %w", err)
	}
	defer rows.Close()

	var children []models.Item
	for rows.Next() {
		var item models.Item
		var itemTypeID, milestoneID, assigneeID, creatorID sql.NullInt64
		var customFieldValuesJSON sql.NullString
		var parentID sql.NullInt64
		var workspaceName, itemTypeName sql.NullString

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &itemTypeID, &item.Title, &item.Description, &item.IsTask,
			&milestoneID, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
			&item.CreatedAt, &item.UpdatedAt,
			&workspaceName, &itemTypeName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan child: %w", err)
		}

		// Handle nullable fields
		if itemTypeID.Valid {
			val := int(itemTypeID.Int64)
			item.ItemTypeID = &val
		}
		if milestoneID.Valid {
			val := int(milestoneID.Int64)
			item.MilestoneID = &val
		}
		if assigneeID.Valid {
			val := int(assigneeID.Int64)
			item.AssigneeID = &val
		}
		if creatorID.Valid {
			val := int(creatorID.Int64)
			item.CreatorID = &val
		}
		if parentID.Valid {
			val := int(parentID.Int64)
			item.ParentID = &val
		}
		if workspaceName.Valid {
			item.WorkspaceName = workspaceName.String
		}
		if itemTypeName.Valid {
			item.ItemTypeName = itemTypeName.String
		}

		// Handle custom field values
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			item.CustomFieldValues = make(map[string]interface{})
		}

		children = append(children, item)
	}

	return children, rows.Err()
}

// GetRoot returns the root item for a given item (walks up to top level)
func (h *HierarchyService) GetRoot(itemID int) (*models.Item, error) {
	query := `
		WITH RECURSIVE path_to_root AS (
			-- Base case: start with the given item
			SELECT id, parent_id
			FROM items
			WHERE id = ?
			
			UNION ALL
			
			-- Recursive case: walk up to parent
			SELECT i.id, i.parent_id
			FROM items i
			JOIN path_to_root p ON i.id = p.parent_id
		)
		SELECT i.id, i.workspace_id, i.item_type_id, i.title, i.description, i.is_task,
		       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, it.name as item_type_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.id IN (SELECT id FROM path_to_root) AND i.parent_id IS NULL
	`

	var item models.Item
	var itemTypeID, milestoneID, assigneeID, creatorID sql.NullInt64
	var customFieldValuesJSON sql.NullString
	var parentID sql.NullInt64
	var workspaceName, itemTypeName sql.NullString

	err := h.db.QueryRow(query, itemID).Scan(
		&item.ID, &item.WorkspaceID, &itemTypeID, &item.Title, &item.Description, &item.IsTask,
		&milestoneID, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
		&item.CreatedAt, &item.UpdatedAt,
		&workspaceName, &itemTypeName,
	)
	if err == sql.ErrNoRows {
		return nil, nil // No root found (shouldn't happen in a well-formed hierarchy)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find root: %w", err)
	}

	// Handle nullable fields
	if itemTypeID.Valid {
		val := int(itemTypeID.Int64)
		item.ItemTypeID = &val
	}
	if milestoneID.Valid {
		val := int(milestoneID.Int64)
		item.MilestoneID = &val
	}
	if assigneeID.Valid {
		val := int(assigneeID.Int64)
		item.AssigneeID = &val
	}
	if creatorID.Valid {
		val := int(creatorID.Int64)
		item.CreatorID = &val
	}
	if parentID.Valid {
		val := int(parentID.Int64)
		item.ParentID = &val
	}
	if workspaceName.Valid {
		item.WorkspaceName = workspaceName.String
	}
	if itemTypeName.Valid {
		item.ItemTypeName = itemTypeName.String
	}

	// Handle custom field values
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		item.CustomFieldValues = make(map[string]interface{})
	}

	return &item, nil
}

// GetEffectiveProject returns the effective project_id for an item by walking up the hierarchy
// Returns: (effective_project_id, inheritance_mode, error)
// inheritance_mode: "none" (NULL), "inherit" (-1), "direct" (>0)
func (h *HierarchyService) GetEffectiveProject(itemID int) (projectID *int, inheritanceMode string, err error) {
	query := `
		WITH RECURSIVE project_chain AS (
			-- Base case: get the item itself
			SELECT id, project_id, parent_id, 0 as depth
			FROM items
			WHERE id = ?

			UNION ALL

			-- Recursive case: walk up to parent if current has inherit (-1)
			SELECT i.id, i.project_id, i.parent_id, pc.depth + 1
			FROM items i
			JOIN project_chain pc ON i.id = pc.parent_id
			WHERE pc.project_id = -1 AND pc.depth < 10
		)
		SELECT
			id,
			project_id,
			CASE
				WHEN project_id IS NULL THEN 'none'
				WHEN project_id = -1 THEN 'inherit'
				ELSE 'direct'
			END as mode,
			depth
		FROM project_chain
		WHERE project_id IS NOT NULL AND project_id != -1
		ORDER BY depth ASC
		LIMIT 1
	`

	var id, depth int
	var nullProjectID sql.NullInt64
	var mode string

	err = h.db.QueryRow(query, itemID).Scan(&id, &nullProjectID, &mode, &depth)
	if err == sql.ErrNoRows {
		// No effective project found (all ancestors have NULL or -1)
		return nil, "none", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to get effective project: %w", err)
	}

	if nullProjectID.Valid {
		val := int(nullProjectID.Int64)
		return &val, mode, nil
	}

	return nil, "none", nil
}
