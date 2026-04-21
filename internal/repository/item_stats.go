package repository

import (
	"database/sql"
	"fmt"
	"time"
)

// --- Workspace statistics aggregations --------------------------------------

// WorkspaceStatsAssignment is one row of the assignment-distribution facet
// returned by ComputeWorkspaceItemStats. UserID is nil for unassigned items.
type WorkspaceStatsAssignment struct {
	UserID    *int
	UserName  string
	FirstName string
	LastName  string
	ItemCount int
}

// WorkspaceStatsProject is one row of the project-statistics facet.
type WorkspaceStatsProject struct {
	ProjectID      *int
	ProjectName    string
	ProjectColor   string
	ItemCount      int
	CompletedCount int
}

// WorkspaceItemStats bundles the five item-scoped aggregations the workspace
// stats endpoint needs. Collections count and milestone progress live
// elsewhere and are composed into the handler's response separately.
type WorkspaceItemStats struct {
	TotalItems             int
	ItemsByStatusCategory  map[string]int
	AssignmentDistribution []WorkspaceStatsAssignment
	ProjectStatistics      []WorkspaceStatsProject
	PriorityBreakdown      map[string]int
}

// ComputeWorkspaceItemStats runs the five item aggregations the workspace
// stats dashboard depends on (total, status-category breakdown, assignment
// distribution over `since`, project statistics over `since`, priority
// breakdown over `since`). All five share the workspace-id filter and the
// optional VQL-derived `filterSQL` + `filterArgs`.
func (r *ItemRepository) ComputeWorkspaceItemStats(workspaceID int, filterSQL string, filterArgs []interface{}, since time.Time) (*WorkspaceItemStats, error) {
	stats := &WorkspaceItemStats{
		ItemsByStatusCategory:  make(map[string]int),
		AssignmentDistribution: []WorkspaceStatsAssignment{},
		ProjectStatistics:      []WorkspaceStatsProject{},
		PriorityBreakdown:      make(map[string]int),
	}

	// 1. Total items
	totalQuery := `
		SELECT COUNT(*)
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.workspace_id = ?`
	totalArgs := []interface{}{workspaceID}
	if filterSQL != "" {
		totalQuery += " AND (" + filterSQL + ")"
		totalArgs = append(totalArgs, filterArgs...)
	}
	if err := r.db.QueryRow(totalQuery, totalArgs...).Scan(&stats.TotalItems); err != nil {
		return nil, fmt.Errorf("count workspace items: %w", err)
	}

	// 2. Items by status category
	statusQuery := `
		SELECT sc.name, COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?`
	statusArgs := []interface{}{workspaceID}
	if filterSQL != "" {
		statusQuery += " AND (" + filterSQL + ")"
		statusArgs = append(statusArgs, filterArgs...)
	}
	statusQuery += ` GROUP BY sc.name`
	rows, err := r.db.Query(statusQuery, statusArgs...)
	if err != nil {
		return nil, fmt.Errorf("status category breakdown: %w", err)
	}
	for rows.Next() {
		var categoryName sql.NullString
		var count int
		if err := rows.Scan(&categoryName, &count); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan status breakdown: %w", err)
		}
		if categoryName.Valid {
			stats.ItemsByStatusCategory[categoryName.String] = count
		}
	}
	_ = rows.Close()

	// 3. Assignment distribution (since cutoff)
	assignmentQuery := `
		SELECT
			i.assignee_id,
			COALESCE(u.username, 'Unassigned') as user_name,
			COALESCE(u.first_name, '') as first_name,
			COALESCE(u.last_name, '') as last_name,
			COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`
	assignmentArgs := []interface{}{workspaceID, since}
	if filterSQL != "" {
		assignmentQuery += " AND (" + filterSQL + ")"
		assignmentArgs = append(assignmentArgs, filterArgs...)
	}
	assignmentQuery += `
		GROUP BY i.assignee_id, u.username, u.first_name, u.last_name
		ORDER BY item_count DESC
		LIMIT 10`
	assignRows, err := r.db.Query(assignmentQuery, assignmentArgs...)
	if err != nil {
		return nil, fmt.Errorf("assignment distribution: %w", err)
	}
	for assignRows.Next() {
		var row WorkspaceStatsAssignment
		var assigneeID sql.NullInt64
		if err := assignRows.Scan(&assigneeID, &row.UserName, &row.FirstName, &row.LastName, &row.ItemCount); err != nil {
			_ = assignRows.Close()
			return nil, fmt.Errorf("scan assignment row: %w", err)
		}
		assignNullableInt(&row.UserID, assigneeID)
		stats.AssignmentDistribution = append(stats.AssignmentDistribution, row)
	}
	_ = assignRows.Close()

	// 4. Project statistics (since cutoff)
	projectQuery := `
		SELECT
			tp.id,
			tp.name,
			tp.color,
			COUNT(i.id) as item_count,
			SUM(CASE WHEN COALESCE(sc.is_completed, FALSE) = TRUE THEN 1 ELSE 0 END) as completed_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?
		  AND i.time_project_id IS NOT NULL`
	projectArgs := []interface{}{workspaceID, since}
	if filterSQL != "" {
		projectQuery += " AND (" + filterSQL + ")"
		projectArgs = append(projectArgs, filterArgs...)
	}
	projectQuery += `
		GROUP BY tp.id, tp.name, tp.color
		ORDER BY item_count DESC
		LIMIT 10`
	projectRows, err := r.db.Query(projectQuery, projectArgs...)
	if err != nil {
		return nil, fmt.Errorf("project statistics: %w", err)
	}
	for projectRows.Next() {
		var row WorkspaceStatsProject
		var projectID sql.NullInt64
		var projectColor sql.NullString
		if err := projectRows.Scan(&projectID, &row.ProjectName, &projectColor, &row.ItemCount, &row.CompletedCount); err != nil {
			_ = projectRows.Close()
			return nil, fmt.Errorf("scan project row: %w", err)
		}
		assignNullableInt(&row.ProjectID, projectID)
		row.ProjectColor = projectColor.String
		stats.ProjectStatistics = append(stats.ProjectStatistics, row)
	}
	_ = projectRows.Close()

	// 5. Priority breakdown (since cutoff)
	priorityQuery := `
		SELECT
			COALESCE(pri.name, 'None') as priority,
			COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`
	priorityArgs := []interface{}{workspaceID, since}
	if filterSQL != "" {
		priorityQuery += " AND (" + filterSQL + ")"
		priorityArgs = append(priorityArgs, filterArgs...)
	}
	priorityQuery += ` GROUP BY pri.name`
	priorityRows, err := r.db.Query(priorityQuery, priorityArgs...)
	if err != nil {
		return nil, fmt.Errorf("priority breakdown: %w", err)
	}
	for priorityRows.Next() {
		var priority string
		var count int
		if err := priorityRows.Scan(&priority, &count); err != nil {
			_ = priorityRows.Close()
			return nil, fmt.Errorf("scan priority row: %w", err)
		}
		stats.PriorityBreakdown[priority] = count
	}
	_ = priorityRows.Close()

	return stats, nil
}

// --- Configuration-set migration aggregations ------------------------------

// ItemStatusCount is one row of a (status_id, status_name, item_count)
// aggregation. Rows with no status yield status_id=0, status_name="" because
// the underlying queries use COALESCE.
type ItemStatusCount struct {
	StatusID   int
	StatusName string
	ItemCount  int
}

// ItemTypeStatusCount is one row of a
// (item_type_id, item_type_name, status_id, status_name, count) aggregation
// used when migration analysis differentiates by item type. ItemTypeID is nil
// when the item has no item_type_id; StatusID is 0 and StatusName is "" when
// the item has no status.
type ItemTypeStatusCount struct {
	ItemTypeID   *int
	ItemTypeName string
	StatusID     int
	StatusName   string
	ItemCount    int
}

// ListStatusCountsForWorkspaces groups items in the given workspaces by
// status_id / status_name, returning COUNT(*) per group. Used by migration
// analyzers to enumerate the statuses in use. Returns an empty slice if
// workspaceIDs is empty.
func (r *ItemRepository) ListStatusCountsForWorkspaces(workspaceIDs []int) ([]ItemStatusCount, error) {
	if len(workspaceIDs) == 0 {
		return []ItemStatusCount{}, nil
	}
	placeholders, args := inPlaceholders(workspaceIDs)
	query := `
		SELECT COALESCE(s.id, 0) as status_id, COALESCE(s.name, '') as status_name, COUNT(*) as item_count
		FROM items i
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE i.workspace_id IN (` + placeholders + `)
		GROUP BY s.id, s.name
		ORDER BY s.name`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list status counts for workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ItemStatusCount
	for rows.Next() {
		var c ItemStatusCount
		if err := rows.Scan(&c.StatusID, &c.StatusName, &c.ItemCount); err != nil {
			return nil, fmt.Errorf("scan status count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListItemTypeStatusCountsForWorkspaces groups items in the given workspaces
// by (item_type_id, status_id), returning COUNT(*) per group. Used by the
// per-item-type migration analyzer.
func (r *ItemRepository) ListItemTypeStatusCountsForWorkspaces(workspaceIDs []int) ([]ItemTypeStatusCount, error) {
	if len(workspaceIDs) == 0 {
		return []ItemTypeStatusCount{}, nil
	}
	placeholders, args := inPlaceholders(workspaceIDs)
	query := `
		SELECT i.item_type_id, COALESCE(it.name, '') as item_type_name,
		       COALESCE(s.id, 0) as status_id, COALESCE(s.name, '') as status_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE i.workspace_id IN (` + placeholders + `)
		GROUP BY i.item_type_id, it.name, s.id, s.name
		ORDER BY it.name, s.name`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list item-type/status counts for workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ItemTypeStatusCount
	for rows.Next() {
		var c ItemTypeStatusCount
		var itemTypeID sql.NullInt64
		if err := rows.Scan(&itemTypeID, &c.ItemTypeName, &c.StatusID, &c.StatusName, &c.ItemCount); err != nil {
			return nil, fmt.Errorf("scan item-type/status count: %w", err)
		}
		assignNullableInt(&c.ItemTypeID, itemTypeID)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ItemTypeCount is one row of (item_type_id, item_type_name, count). TypeID=0
// and TypeName="(No Type)" for items with no item_type_id, courtesy of COALESCE.
type ItemTypeCount struct {
	TypeID    int
	TypeName  string
	ItemCount int
}

// ListItemTypeCountsForWorkspace groups a single workspace's items by
// item_type_id, returning COUNT(*) per group. Used by the item-type migration
// analyzer.
func (r *ItemRepository) ListItemTypeCountsForWorkspace(workspaceID int) ([]ItemTypeCount, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(i.item_type_id, 0) as type_id,
		       COALESCE(it.name, '(No Type)') as type_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.workspace_id = ?
		GROUP BY i.item_type_id, it.name
		ORDER BY it.name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list item-type counts for workspace: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ItemTypeCount
	for rows.Next() {
		var c ItemTypeCount
		if err := rows.Scan(&c.TypeID, &c.TypeName, &c.ItemCount); err != nil {
			return nil, fmt.Errorf("scan item-type count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PriorityCount is one row of (priority_id, priority_name, count).
// PriorityID=0 and PriorityName="(No Priority)" for items with no priority_id.
type PriorityCount struct {
	PriorityID   int
	PriorityName string
	ItemCount    int
}

// ListPriorityCountsForWorkspace groups a single workspace's items by
// priority_id, returning COUNT(*) per group.
func (r *ItemRepository) ListPriorityCountsForWorkspace(workspaceID int) ([]PriorityCount, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(i.priority_id, 0) as priority_id,
		       COALESCE(p.name, '(No Priority)') as priority_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN priorities p ON i.priority_id = p.id
		WHERE i.workspace_id = ?
		GROUP BY i.priority_id, p.name
		ORDER BY p.name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list priority counts for workspace: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PriorityCount
	for rows.Next() {
		var c PriorityCount
		if err := rows.Scan(&c.PriorityID, &c.PriorityName, &c.ItemCount); err != nil {
			return nil, fmt.Errorf("scan priority count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListNonEmptyCustomFieldJSONForWorkspace returns the raw custom_field_values
// JSON strings for every item in the given workspace whose value is non-NULL,
// non-empty, and not the literal "{}". Used by the custom-field migration
// analyzer to count how many items reference each field.
func (r *ItemRepository) ListNonEmptyCustomFieldJSONForWorkspace(workspaceID int) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT custom_field_values FROM items
		WHERE workspace_id = ?
		  AND custom_field_values IS NOT NULL
		  AND custom_field_values != ''
		  AND custom_field_values != '{}'
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list custom field JSON: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var cfvJSON string
		if err := rows.Scan(&cfvJSON); err != nil {
			return nil, fmt.Errorf("scan custom field JSON: %w", err)
		}
		out = append(out, cfvJSON)
	}
	return out, rows.Err()
}
