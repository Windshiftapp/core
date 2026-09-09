package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
)

const maxItemHierarchyDepth = 30

// defaultTimeRollupMaxItems caps how many items contribute to a time-rollup
// aggregation. Sized so the IN-list stays well under SQLite's default
// SQLITE_MAX_VARIABLE_NUMBER (32766) even with a handful of extra params.
const defaultTimeRollupMaxItems = 500

// GetItemTypeAndHierarchyLevel returns an item's item_type_id (nil when unset)
// and the hierarchy level of that type (0 when the type has none). Returns
// ErrNotFound when the item does not exist.
func (r *ItemRepository) GetItemTypeAndHierarchyLevel(itemID int) (typeID *int, level int, err error) {
	var itemTypeID sql.NullInt64
	err = r.db.QueryRow(`
		SELECT i.item_type_id, COALESCE(it.hierarchy_level, 0)
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.id = ?
	`, itemID).Scan(&itemTypeID, &level)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("get item type hierarchy level: %w", err)
	}
	assignNullableInt(&typeID, itemTypeID)
	return typeID, level, nil
}

// GetChildren returns direct children of an item
func (r *ItemRepository) GetChildren(parentID int) ([]*models.Item, error) {
	return r.GetChildrenContext(context.Background(), parentID)
}

// GetChildrenContext is the request-aware form of GetChildren.
func (r *ItemRepository) GetChildrenContext(ctx context.Context, parentID int) ([]*models.Item, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.iteration_id,
		       i.project_id, i.inherit_project, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name, sc.color as status_color,
		       it.name as item_type_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
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

// GetDescendants returns all descendants of an item using a capped recursive CTE.
func (r *ItemRepository) GetDescendants(parentID int) ([]*models.Item, error) {
	return r.GetDescendantsWithMaxDepth(parentID, maxItemHierarchyDepth)
}

// GetDescendantsWithMaxDepth returns descendants up to maxDepth levels deep.
func (r *ItemRepository) GetDescendantsWithMaxDepth(parentID, maxDepth int) ([]*models.Item, error) {
	return r.GetDescendantsWithMaxDepthContext(context.Background(), parentID, maxDepth)
}

// GetDescendantsWithMaxDepthContext is the request-aware form of
// GetDescendantsWithMaxDepth.
func (r *ItemRepository) GetDescendantsWithMaxDepthContext(ctx context.Context, parentID, maxDepth int) ([]*models.Item, error) {
	if maxDepth <= 0 || maxDepth > maxItemHierarchyDepth {
		maxDepth = maxItemHierarchyDepth
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT id, parent_id, 1 as level
			FROM items
			WHERE parent_id = ?
			UNION ALL
			SELECT i.id, i.parent_id, d.level + 1
			FROM items i
			INNER JOIN descendants d ON i.parent_id = d.id
			WHERE d.level < ?
		)
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.iteration_id,
		       i.project_id, i.inherit_project, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name, sc.color as status_color,
		       it.name as item_type_name,
		       d.level
		FROM items i
		INNER JOIN descendants d ON i.id = d.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		ORDER BY d.level, i.frac_index
	`, parentID, maxDepth)
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
			SELECT i.id, i.parent_id, 0 as level, it.hierarchy_level
			FROM items i
			LEFT JOIN item_types it ON i.item_type_id = it.id
			WHERE i.id = ?
			UNION ALL
			SELECT i.id, i.parent_id, a.level + 1, it.hierarchy_level
			FROM items i
			LEFT JOIN item_types it ON i.item_type_id = it.id
			INNER JOIN ancestors a ON i.id = a.parent_id
			WHERE a.level < ?
			  AND COALESCE(a.hierarchy_level, -999) != 0
		)
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.iteration_id,
		       i.project_id, i.inherit_project, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name, sc.color as status_color,
		       it.name as item_type_name
		FROM items i
		INNER JOIN ancestors a ON i.id = a.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE a.level > 0
		ORDER BY a.level DESC
	`, itemID, maxItemHierarchyDepth)
	if err != nil {
		return nil, fmt.Errorf("failed to get ancestors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanItemsWithDetails(rows)
}

// GetAncestorsForHierarchy returns ancestors using the lightweight projection
// consumed by HierarchyService. It intentionally avoids status/priority joins
// so cycle-safety tests and callers with minimal hierarchy fixtures do not need
// the full item-detail schema.
func (r *ItemRepository) GetAncestorsForHierarchy(itemID, maxDepth int) ([]models.Item, error) {
	return r.GetAncestorsForHierarchyContext(context.Background(), itemID, maxDepth)
}

// GetAncestorsForHierarchyContext is the request-aware form of
// GetAncestorsForHierarchy.
func (r *ItemRepository) GetAncestorsForHierarchyContext(ctx context.Context, itemID, maxDepth int) ([]models.Item, error) {
	if maxDepth <= 0 || maxDepth > maxItemHierarchyDepth {
		maxDepth = maxItemHierarchyDepth
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.is_task,
			       i.assignee_id, i.creator_id, i.custom_field_values, i.parent_id,
			       i.created_at, i.updated_at,
			       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name, it.color as item_type_color, it.icon as item_type_icon,
			       0 as level, it.hierarchy_level
			FROM items i
			JOIN workspaces w ON i.workspace_id = w.id
			LEFT JOIN item_types it ON i.item_type_id = it.id
			WHERE i.id = ?

			UNION ALL

			SELECT p.id, p.workspace_id, p.workspace_item_number, p.item_type_id, p.title, p.description, p.is_task,
			       p.assignee_id, p.creator_id, p.custom_field_values, p.parent_id,
			       p.created_at, p.updated_at,
			       w.name as workspace_name, w.key as workspace_key, it.name as item_type_name, it.color as item_type_color, it.icon as item_type_icon,
			       a.level + 1 as level, it.hierarchy_level
			FROM items p
			JOIN workspaces w ON p.workspace_id = w.id
			LEFT JOIN item_types it ON p.item_type_id = it.id
			JOIN ancestors a ON p.id = a.parent_id
			WHERE a.level < ?
			  AND COALESCE(a.hierarchy_level, -999) != 0
		)
		SELECT id, workspace_id, workspace_item_number, item_type_id, title, description, is_task,
		       assignee_id, creator_id, custom_field_values, parent_id,
		       created_at, updated_at,
		       workspace_name, workspace_key, item_type_name, item_type_color, item_type_icon, level
		FROM ancestors
		WHERE id != ?
		ORDER BY level DESC
	`, itemID, maxDepth, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to query ancestors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ancestors []models.Item
	for rows.Next() {
		var item models.Item
		var itemTypeID, assigneeID, creatorID, parentID sql.NullInt64
		var customFieldValuesJSON sql.NullString
		var workspaceName, workspaceKey, itemTypeName, itemTypeColor, itemTypeIcon sql.NullString
		var level int
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description, &item.IsTask,
			&assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
			&item.CreatedAt, &item.UpdatedAt,
			&workspaceName, &workspaceKey, &itemTypeName, &itemTypeColor, &itemTypeIcon, &level,
		); err != nil {
			return nil, fmt.Errorf("failed to scan ancestor: %w", err)
		}
		_ = level
		_ = itemTypeColor
		_ = itemTypeIcon
		assignNullableInt(&item.ItemTypeID, itemTypeID)
		assignNullableInt(&item.AssigneeID, assigneeID)
		assignNullableInt(&item.CreatorID, creatorID)
		assignNullableInt(&item.ParentID, parentID)
		assignNullableString(&item.WorkspaceName, workspaceName)
		assignNullableString(&item.WorkspaceKey, workspaceKey)
		assignNullableString(&item.ItemTypeName, itemTypeName)
		item.CustomFieldValues = parseCustomFieldsJSON(customFieldValuesJSON)
		ancestors = append(ancestors, item)
	}
	return ancestors, rows.Err()
}

// GetRootItems returns all root items (no parent) for a workspace
func (r *ItemRepository) GetRootItems(workspaceID int) ([]*models.Item, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.iteration_id,
		       i.project_id, i.inherit_project, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name, sc.color as status_color,
		       it.name as item_type_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
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

// GetTimeRollup sums estimate_minutes and time_worklogs.duration_minutes for
// the item and its descendants up to maxDepth levels deep, capped at maxItems
// total items. The caller-supplied root item counts as level 0. When the
// descendant set hits maxItems the result's Truncated flag is set and only
// the first maxItems contribute to the sums.
//
// The root item's view permission is checked by the handler; per-descendant
// permission filtering is intentionally skipped here — we only expose
// aggregate counts, not per-item data.
func (r *ItemRepository) GetTimeRollup(itemID, maxDepth, maxItems int) (*models.TimeRollup, error) {
	if maxDepth <= 0 || maxDepth > maxItemHierarchyDepth {
		maxDepth = maxItemHierarchyDepth
	}
	if maxItems <= 0 {
		maxItems = defaultTimeRollupMaxItems
	}

	// Step 1: collect ids + estimate_minutes for the root + descendants.
	// Fetch maxItems+1 so we can detect whether the result was truncated
	// without a second COUNT query.
	rows, err := r.db.Query(`
		WITH RECURSIVE tree(id, est, level) AS (
			SELECT id, COALESCE(estimate_minutes, 0), 0
			FROM items WHERE id = ?
			UNION ALL
			SELECT i.id, COALESCE(i.estimate_minutes, 0), t.level + 1
			FROM items i
			INNER JOIN tree t ON i.parent_id = t.id
			WHERE t.level < ?
		)
		SELECT id, est FROM tree LIMIT ?
	`, itemID, maxDepth, maxItems+1)
	if err != nil {
		return nil, fmt.Errorf("failed to query time rollup tree: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type idEst struct {
		id  int
		est int64
	}
	rowsData := make([]idEst, 0, maxItems+1)
	for rows.Next() {
		var id int
		var est sql.NullInt64
		if err := rows.Scan(&id, &est); err != nil {
			return nil, fmt.Errorf("failed to scan time rollup row: %w", err)
		}
		var estVal int64
		if est.Valid {
			estVal = est.Int64
		}
		rowsData = append(rowsData, idEst{id: id, est: estVal})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate time rollup rows: %w", err)
	}

	truncated := len(rowsData) > maxItems
	if truncated {
		rowsData = rowsData[:maxItems]
	}

	rollup := &models.TimeRollup{
		ItemCount: len(rowsData),
		Truncated: truncated,
	}

	if len(rowsData) == 0 {
		return rollup, nil
	}

	ids := make([]int, len(rowsData))
	for i, r := range rowsData {
		ids[i] = r.id
		rollup.TotalEstimateMinutes += r.est
	}

	// Step 2: sum logged minutes across the included items. The IN-list is
	// bounded by maxItems (default 500) which is well within SQL parameter
	// limits on both SQLite and Postgres.
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(
		"SELECT COALESCE(SUM(duration_minutes), 0) FROM time_worklogs WHERE item_id IN (%s)",
		placeholders,
	)
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	if err := r.db.QueryRow(query, args...).Scan(&rollup.TotalLoggedMinutes); err != nil {
		return nil, fmt.Errorf("failed to sum worklog duration: %w", err)
	}

	return rollup, nil
}

// GetDescendantIDs returns just the IDs of all descendants (for bulk operations like delete)
func (r *ItemRepository) GetDescendantIDs(parentID int) ([]int, error) {
	rows, err := r.db.Query(`
		WITH RECURSIVE descendants AS (
			SELECT id, 1 as level FROM items WHERE parent_id = ?
			UNION ALL
			SELECT i.id, d.level + 1 FROM items i
			INNER JOIN descendants d ON i.parent_id = d.id
			WHERE d.level < ?
		)
		SELECT id FROM descendants
	`, parentID, maxItemHierarchyDepth)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate descendant ids: %w", err)
	}

	return ids, nil
}

// GetItemHierarchyLevel returns the hierarchy level of an item's type, or nil
// when the item does not exist, has no type, or the type has no level set.
func (r *ItemRepository) GetItemHierarchyLevel(itemID int) (*int, error) {
	var level sql.NullInt64
	err := r.db.QueryRow(`
		SELECT it.hierarchy_level
		FROM items p
		LEFT JOIN item_types it ON p.item_type_id = it.id
		WHERE p.id = ?
	`, itemID).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get item hierarchy level: %w", err)
	}
	var out *int
	assignNullableInt(&out, level)
	return out, nil
}

// CountChildrenWithHierarchyLevelNot returns how many direct children of an
// item have a typed hierarchy level different from the given level. Children
// without a type are not counted.
func (r *ItemRepository) CountChildrenWithHierarchyLevelNot(parentID, level int) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM items c
		JOIN item_types it ON c.item_type_id = it.id
		WHERE c.parent_id = ? AND it.hierarchy_level != ?
	`, parentID, level).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count children by hierarchy level: %w", err)
	}
	return count, nil
}

// CountDescendants returns the total number of descendants of an item. The
// recursive walk is capped at maxItemHierarchyDepth so a stored cycle can't
// loop the DB.
func (r *ItemRepository) CountDescendants(itemID int) (int, error) {
	return r.CountDescendantsContext(context.Background(), itemID)
}

// CountDescendantsContext is the request-aware form of CountDescendants.
func (r *ItemRepository) CountDescendantsContext(ctx context.Context, itemID int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT id, parent_id, 1 as depth
			FROM items
			WHERE parent_id = ?

			UNION ALL

			SELECT i.id, i.parent_id, d.depth + 1
			FROM items i
			JOIN descendants d ON i.parent_id = d.id
			WHERE d.depth < ?
		)
		SELECT COUNT(*) FROM descendants
	`, itemID, maxItemHierarchyDepth).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count descendants: %w", err)
	}
	return count, nil
}

// EffectiveProjectResolution is the result of walking an item's hierarchy to
// resolve project inheritance.
type EffectiveProjectResolution struct {
	DirectProjectID    *int // the item's own project_id (nil when unset)
	InheritProject     bool // the item's inherit_project flag
	EffectiveProjectID *int // resolved project after walking inherit_project up the chain
}

// ResolveEffectiveProject walks up an item's parent chain (capped at 10
// levels) to resolve the inherited time-tracking project.
func (r *ItemRepository) ResolveEffectiveProject(itemID int) (*EffectiveProjectResolution, error) {
	query := `
		WITH RECURSIVE effective_projects AS (
			-- Base case: the item itself
			SELECT
				i.id,
				i.project_id,
				i.inherit_project,
				i.parent_id,
				it.hierarchy_level,
				CASE
					WHEN i.inherit_project = true THEN NULL
					ELSE i.project_id
				END as effective_project_id,
				0 as depth
			FROM items i
			LEFT JOIN item_types it ON i.item_type_id = it.id
			WHERE i.id = ?

			UNION ALL

			-- Recursive case: climb up hierarchy to find inherited project
			SELECT
				ep.id,
				ep.project_id,
				ep.inherit_project,
				i.parent_id,
				it.hierarchy_level,
				CASE
					WHEN i.project_id IS NOT NULL AND i.inherit_project = false THEN i.project_id
					ELSE ep.effective_project_id
				END as effective_project_id,
				ep.depth + 1
			FROM effective_projects ep
			JOIN items i ON ep.parent_id = i.id
			LEFT JOIN item_types it ON i.item_type_id = it.id
			WHERE ep.effective_project_id IS NULL
			  AND ep.inherit_project = true
			  AND COALESCE(ep.hierarchy_level, -999) != 0
			  AND ep.depth < 10
		)
		SELECT
			project_id,
			inherit_project,
			effective_project_id
		FROM effective_projects
		WHERE id = ?
		ORDER BY depth DESC
		LIMIT 1
	`

	var out EffectiveProjectResolution
	var directProjectID, effectiveProjectID sql.NullInt64
	err := r.db.QueryRow(query, itemID, itemID).Scan(&directProjectID, &out.InheritProject, &effectiveProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate effective project: %w", err)
	}
	assignNullableInt(&out.DirectProjectID, directProjectID)
	assignNullableInt(&out.EffectiveProjectID, effectiveProjectID)
	return &out, nil
}

// SetParentDirect sets parent_id without recording history or bumping
// updated_at. Used by the Jira import, which runs without a user context and
// must preserve imported timestamps.
func (r *ItemRepository) SetParentDirect(itemID, parentID int) error {
	if _, err := r.db.ExecWrite(`UPDATE items SET parent_id = ? WHERE id = ?`, parentID, itemID); err != nil {
		return fmt.Errorf("set parent: %w", err)
	}
	return nil
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

// scanItemsWithDetailsAndLevel scans item rows that include a trailing level column.
func scanItemsWithDetailsAndLevel(rows *sql.Rows) ([]*models.Item, error) {
	var items []*models.Item
	for rows.Next() {
		var level int
		item, err := scanItemRowBase(rows, &level)
		if err != nil {
			return nil, err
		}
		_ = level
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	return items, nil
}

// scanItemWithDetailsRow scans a single row with item details (no level column).
func scanItemWithDetailsRow(rows *sql.Rows) (*models.Item, error) {
	return scanItemRowBase(rows, nil)
}

// scanItemRowBase scans a single item row with details. If level is non-nil, it is scanned as an
// additional trailing column (used by hierarchy CTE queries).
func scanItemRowBase(rows *sql.Rows, level *int) (*models.Item, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID sql.NullInt64
	var dueDate sql.NullTime
	var priorityName, priorityIcon, priorityColor sql.NullString
	var statusName, statusColor sql.NullString
	var itemTypeName sql.NullString

	dests := []any{
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &item.IsTask, &iterationID,
		&projectID, &item.InheritProject, &assigneeID, &creatorID, &customFieldValuesJSON,
		&parentID, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
		&item.WorkspaceName, &item.WorkspaceKey,
		&priorityName, &priorityIcon, &priorityColor,
		&statusName, &statusColor,
		&itemTypeName,
	}
	if level != nil {
		dests = append(dests, level)
	}

	if err := rows.Scan(dests...); err != nil {
		return nil, fmt.Errorf("failed to scan item row: %w", err)
	}

	assignNullableInt(&item.ItemTypeID, itemTypeID)
	assignNullableInt(&item.ParentID, parentID)
	assignNullableInt(&item.StatusID, statusID)
	assignNullableInt(&item.PriorityID, priorityID)
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
	assignNullableString(&item.StatusColor, statusColor)
	assignNullableString(&item.ItemTypeName, itemTypeName)

	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
			item.CustomFieldValues = make(map[string]any)
		}
	} else {
		item.CustomFieldValues = make(map[string]any)
	}

	return &item, nil
}

// FindChildrenForUpdateContext locks children of already-locked parents.
// PostgreSQL's parent foreign key makes concurrent attachments wait for those
// parent locks; SQLite serializes writers for the enclosing transaction.
func (r *ItemRepository) FindChildrenForUpdateContext(ctx context.Context, tx database.Tx, parentIDs []int) ([]*models.Item, error) {
	if len(parentIDs) == 0 {
		return []*models.Item{}, nil
	}
	placeholders, args := inPlaceholders(parentIDs)
	query := `SELECT ` + itemBaseColumns + ` FROM items WHERE parent_id IN (` + placeholders + `) ORDER BY id`
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("lock child items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []*models.Item
	for rows.Next() {
		item, err := scanItemBase(rows)
		if err != nil {
			return nil, fmt.Errorf("scan locked child: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate locked children: %w", err)
	}
	return items, nil
}

// FindSubtreeForUpdateContext locks each generation before discovering its
// children, so hierarchy changes cannot add unexamined rows to the deletion.
func (r *ItemRepository) FindSubtreeForUpdateContext(ctx context.Context, tx database.Tx, rootID int) ([]*models.Item, error) {
	root, err := r.FindByIDForUpdate(tx, rootID)
	if err != nil {
		return nil, err
	}
	items := []*models.Item{root}
	seen := map[int]bool{root.ID: true}
	pending := []int{root.ID}
	for len(pending) > 0 {
		batchSize := min(len(pending), 500)
		children, err := r.FindChildrenForUpdateContext(ctx, tx, pending[:batchSize])
		if err != nil {
			return nil, err
		}
		pending = pending[batchSize:]
		for _, child := range children {
			if seen[child.ID] {
				continue
			}
			seen[child.ID] = true
			items = append(items, child)
			pending = append(pending, child.ID)
		}
	}
	return items, nil
}
