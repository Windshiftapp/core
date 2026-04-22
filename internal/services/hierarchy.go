package services

import (
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// maxHierarchyDepth caps every recursive hierarchy walk — both the CTE-based
// traversals below and the cycle-detection in WouldCreateCycle. 30 comfortably
// covers realistic roadmap/epic/story/subtask trees while capping worst-case
// scan cost and preventing a stored cycle from looping the DB forever.
const maxHierarchyDepth = 30

// HierarchyService handles all hierarchy-related operations using only parent_id
type HierarchyService struct {
	db database.Database
}

// NewHierarchyService creates a new hierarchy service
func NewHierarchyService(db database.Database) *HierarchyService {
	return &HierarchyService{db: db}
}

// WouldCreateCycle reports whether assigning newParentID as the parent of
// something in ancestorCandidateID's subtree (or of ancestorCandidateID itself)
// would create a cycle. It walks parent_id upward from newParentID; if
// ancestorCandidateID is encountered — or equals newParentID — a cycle would
// result. Self-parent (newParentID == ancestorCandidateID) is reported as a
// cycle. If the walk exhausts maxHierarchyDepth without reaching a root, the
// hierarchy is either already cyclic or deeper than our ceiling, so we
// fail-closed and return (true, nil).
//
// This overload reads outside of a transaction. Callers that are about to
// mutate parent_id must use WouldCreateCycleTx so the check and the write
// are atomic.
func (h *HierarchyService) WouldCreateCycle(ancestorCandidateID, newParentID int) (bool, error) {
	itemRepo := repository.NewItemRepository(h.db)
	return walkForCycle(ancestorCandidateID, newParentID, itemRepo.GetParentID)
}

// WouldCreateCycleTx is the transaction-scoped variant of WouldCreateCycle.
// It walks using SELECT ... FOR UPDATE (on Postgres) so the rows being
// examined are locked for the rest of the transaction; paired with writing
// the new parent_id in the same transaction this closes the TOCTOU window
// where two concurrent reparents could each pass their cycle check and
// together create a cycle.
func (h *HierarchyService) WouldCreateCycleTx(tx database.Tx, ancestorCandidateID, newParentID int) (bool, error) {
	itemRepo := repository.NewItemRepository(h.db)
	return walkForCycle(ancestorCandidateID, newParentID, func(id int) (*int, error) {
		return itemRepo.GetParentIDTx(tx, id)
	})
}

func walkForCycle(ancestorCandidateID, newParentID int, getParent func(int) (*int, error)) (bool, error) {
	current := newParentID
	for i := 0; i < maxHierarchyDepth; i++ {
		if current == ancestorCandidateID {
			return true, nil
		}
		parent, err := getParent(current)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return false, nil
			}
			return false, fmt.Errorf("failed to walk hierarchy: %w", err)
		}
		if parent == nil {
			return false, nil
		}
		current = *parent
	}
	return true, nil
}

// GetAncestors returns all ancestors of an item (from root to direct parent).
// The recursive walk is capped at maxHierarchyDepth so a stored cycle can't
// loop the DB.
func (h *HierarchyService) GetAncestors(itemID int) ([]models.Item, error) {
	query := `
		WITH RECURSIVE ancestors AS (
			-- Base case: get the item itself
			SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.is_task,
			       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
			       i.created_at, i.updated_at,
			       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name, it.color as item_type_color, it.icon as item_type_icon,
			       0 as level
			FROM items i
			JOIN workspaces w ON i.workspace_id = w.id
			LEFT JOIN item_types it ON i.item_type_id = it.id
			WHERE i.id = ?

			UNION ALL

			-- Recursive case: get parent of current item
			SELECT p.id, p.workspace_id, p.workspace_item_number, p.item_type_id, p.title, p.description, p.is_task,
			       p.milestone_id, p.assignee_id, p.creator_id, p.custom_field_values, p.parent_id,
			       p.created_at, p.updated_at,
			       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name, it.color as item_type_color, it.icon as item_type_icon,
			       a.level + 1 as level
			FROM items p
			JOIN workspaces w ON p.workspace_id = w.id
			LEFT JOIN item_types it ON p.item_type_id = it.id
			JOIN ancestors a ON p.id = a.parent_id
			WHERE a.level < ?
		)
		SELECT id, workspace_id, workspace_item_number, item_type_id, title, description, is_task,
		       milestone_id, assignee_id, creator_id, custom_field_values, parent_id,
		       created_at, updated_at,
		       workspace_name, workspace_key, item_type_name, item_type_color, item_type_icon, level
		FROM ancestors
		WHERE id != ? -- Exclude the original item
		ORDER BY level DESC -- Root first, then down to direct parent
	`

	rows, err := h.db.Query(query, itemID, maxHierarchyDepth, itemID)
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
		var workspaceName, workspaceKey, itemTypeName, itemTypeColor, itemTypeIcon sql.NullString
		var level int

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description, &item.IsTask,
			&milestoneID, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
			&item.CreatedAt, &item.UpdatedAt,
			&workspaceName, &workspaceKey, &itemTypeName, &itemTypeColor, &itemTypeIcon, &level,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ancestor: %w", err)
		}

		// Handle nullable fields
		item.ItemTypeID = nullInt64ToIntPtr(itemTypeID)
		item.MilestoneID = nullInt64ToIntPtr(milestoneID)
		item.AssigneeID = nullInt64ToIntPtr(assigneeID)
		item.CreatorID = nullInt64ToIntPtr(creatorID)
		item.ParentID = nullInt64ToIntPtr(parentID)
		item.WorkspaceName = nullStringToString(workspaceName)
		item.WorkspaceKey = nullStringToString(workspaceKey)
		item.ItemTypeName = nullStringToString(itemTypeName)
		parseItemCustomFieldValues(&item, customFieldValuesJSON)

		ancestors = append(ancestors, item)
	}

	return ancestors, rows.Err()
}

// GetDescendants returns all descendants of an item
func (h *HierarchyService) GetDescendants(itemID, maxDepth int) ([]models.Item, error) {
	query := `
		WITH RECURSIVE descendants AS (
			-- Base case: get direct children
			SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.is_task,
			       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
			       i.created_at, i.updated_at,
			       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name,
			       1 as depth
			FROM items i
			JOIN workspaces w ON i.workspace_id = w.id
			LEFT JOIN item_types it ON i.item_type_id = it.id
			WHERE i.parent_id = ?

			UNION ALL

			-- Recursive case: get children of descendants
			SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.is_task,
			       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
			       i.created_at, i.updated_at,
			       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name,
			       d.depth + 1 as depth
			FROM items i
			JOIN workspaces w ON i.workspace_id = w.id
			LEFT JOIN item_types it ON i.item_type_id = it.id
			JOIN descendants d ON i.parent_id = d.id
			WHERE d.depth < ?
		)
		SELECT id, workspace_id, workspace_item_number, item_type_id, title, description, is_task,
		       milestone_id, assignee_id, creator_id, custom_field_values, parent_id,
		       created_at, updated_at,
		       workspace_name, workspace_key, item_type_name, depth
		FROM descendants
		ORDER BY depth ASC, created_at ASC
	`

	// Clamp maxDepth to the shared hierarchy ceiling so a client cannot
	// request an arbitrarily deep recursive CTE and a stored cycle can't
	// run away.
	if maxDepth <= 0 || maxDepth > maxHierarchyDepth {
		maxDepth = maxHierarchyDepth
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
		var workspaceName, workspaceKey, itemTypeName sql.NullString
		var depth int

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description, &item.IsTask,
			&milestoneID, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
			&item.CreatedAt, &item.UpdatedAt,
			&workspaceName, &workspaceKey, &itemTypeName, &depth,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan descendant: %w", err)
		}

		// Handle nullable fields
		item.ItemTypeID = nullInt64ToIntPtr(itemTypeID)
		item.MilestoneID = nullInt64ToIntPtr(milestoneID)
		item.AssigneeID = nullInt64ToIntPtr(assigneeID)
		item.CreatorID = nullInt64ToIntPtr(creatorID)
		item.ParentID = nullInt64ToIntPtr(parentID)
		item.WorkspaceName = nullStringToString(workspaceName)
		item.WorkspaceKey = nullStringToString(workspaceKey)
		item.ItemTypeName = nullStringToString(itemTypeName)
		parseItemCustomFieldValues(&item, customFieldValuesJSON)

		descendants = append(descendants, item)
	}

	return descendants, rows.Err()
}

// CountDescendants returns the total number of descendants for an item.
// The recursive walk is capped at maxHierarchyDepth so a stored cycle can't
// loop the DB.
func (h *HierarchyService) CountDescendants(itemID int) (int, error) {
	query := `
		WITH RECURSIVE descendants AS (
			-- Base case: get direct children
			SELECT id, parent_id, 1 as depth
			FROM items
			WHERE parent_id = ?

			UNION ALL

			-- Recursive case: get children of descendants
			SELECT i.id, i.parent_id, d.depth + 1
			FROM items i
			JOIN descendants d ON i.parent_id = d.id
			WHERE d.depth < ?
		)
		SELECT COUNT(*) FROM descendants
	`

	var count int
	err := h.db.QueryRow(query, itemID, maxHierarchyDepth).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count descendants: %w", err)
	}

	return count, nil
}

// GetChildren returns direct children of an item
func (h *HierarchyService) GetChildren(itemID int) ([]models.Item, error) {
	query := `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.is_task,
		       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name
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
		var workspaceName, workspaceKey, itemTypeName sql.NullString

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description, &item.IsTask,
			&milestoneID, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
			&item.CreatedAt, &item.UpdatedAt,
			&workspaceName, &workspaceKey, &itemTypeName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan child: %w", err)
		}

		// Handle nullable fields
		item.ItemTypeID = nullInt64ToIntPtr(itemTypeID)
		item.MilestoneID = nullInt64ToIntPtr(milestoneID)
		item.AssigneeID = nullInt64ToIntPtr(assigneeID)
		item.CreatorID = nullInt64ToIntPtr(creatorID)
		item.ParentID = nullInt64ToIntPtr(parentID)
		item.WorkspaceName = nullStringToString(workspaceName)
		item.WorkspaceKey = nullStringToString(workspaceKey)
		item.ItemTypeName = nullStringToString(itemTypeName)
		parseItemCustomFieldValues(&item, customFieldValuesJSON)

		children = append(children, item)
	}

	return children, rows.Err()
}

// GetRoot returns the root item for a given item (walks up to top level).
// The walk is capped at maxHierarchyDepth so a stored cycle can't loop the
// DB; exhaustion surfaces as an error rather than a silent nil so callers
// can't confuse it with "no parent".
func (h *HierarchyService) GetRoot(itemID int) (*models.Item, error) {
	query := `
		WITH RECURSIVE path_to_root AS (
			-- Base case: start with the given item
			SELECT id, parent_id, 0 as depth
			FROM items
			WHERE id = ?

			UNION ALL

			-- Recursive case: walk up to parent
			SELECT i.id, i.parent_id, p.depth + 1
			FROM items i
			JOIN path_to_root p ON i.id = p.parent_id
			WHERE p.depth < ?
		)
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.is_task,
		       i.milestone_id, i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.id IN (SELECT id FROM path_to_root) AND i.parent_id IS NULL
	`

	var item models.Item
	var itemTypeID, milestoneID, assigneeID, creatorID sql.NullInt64
	var customFieldValuesJSON sql.NullString
	var parentID sql.NullInt64
	var workspaceName, workspaceKey, itemTypeName sql.NullString

	err := h.db.QueryRow(query, itemID, maxHierarchyDepth).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description, &item.IsTask,
		&milestoneID, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
		&item.CreatedAt, &item.UpdatedAt,
		&workspaceName, &workspaceKey, &itemTypeName,
	)
	if err == sql.ErrNoRows {
		// No row with parent_id IS NULL found within the depth cap. Either
		// the walk hit the ceiling (likely cyclic) or every ancestor up to
		// the cap has a parent. Either way this is not a well-formed
		// hierarchy — surface it instead of silently returning nil.
		return nil, fmt.Errorf("hierarchy walk exceeded %d levels without finding a root (item %d, likely cyclic)", maxHierarchyDepth, itemID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find root: %w", err)
	}

	// Handle nullable fields
	item.ItemTypeID = nullInt64ToIntPtr(itemTypeID)
	item.MilestoneID = nullInt64ToIntPtr(milestoneID)
	item.AssigneeID = nullInt64ToIntPtr(assigneeID)
	item.CreatorID = nullInt64ToIntPtr(creatorID)
	item.ParentID = nullInt64ToIntPtr(parentID)
	item.WorkspaceName = nullStringToString(workspaceName)
	item.WorkspaceKey = nullStringToString(workspaceKey)
	item.ItemTypeName = nullStringToString(itemTypeName)
	parseItemCustomFieldValues(&item, customFieldValuesJSON)

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
