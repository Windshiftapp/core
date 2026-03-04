package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"windshift/internal/models"
)

// ItemListParams contains parameters for listing items
type ItemListParams struct {
	WorkspaceIDs []int
	Filters      ItemFilters
	Pagination   PaginationParams
	SortBy       string
	SortAsc      bool
}

// ItemFilters contains optional filters for item queries
type ItemFilters struct {
	WorkspaceID   *int
	StatusID      *int
	PriorityID    *int
	AssigneeID    *int
	CreatorID     *int
	ItemTypeID    *int
	MilestoneID   *int
	IterationID   *int
	ParentID      *int    // nil = any, 0 = root items only
	ParentIDIsSet bool    // true if ParentID filter should be applied
	Level         *int    // Hierarchy level filter
	MaxLevel      *int    // Maximum hierarchy level filter
	CreatedSince  *string // ISO date string
	QLQuery       string  // Custom QL query
	QLArgs        []interface{}
	StatusIDs     []int  // Multi-value status filter (for backlog + search)
	PriorityIDs   []int  // Multi-value priority filter
	TextQuery     string // LIKE search on title/description
	ItemKeyQuery  string // Workspace key pattern match (e.g. "OK-40")
	ItemID        *int   // Filter by specific item ID
}

// PaginationParams contains pagination parameters
type PaginationParams struct {
	Page   int
	Limit  int
	Offset int
}

// allowedSortColumns maps user-provided sort field names to safe SQL column references
var allowedSortColumns = map[string]string{
	"created_at":  "i.created_at",
	"updated_at":  "i.updated_at",
	"title":       "i.title",
	"due_date":    "i.due_date",
	"priority_id": "i.priority_id",
	"status_id":   "i.status_id",
	"rank":        "i.rank",
	"frac_index":  "i.frac_index",
}

// FindAllWithDetails retrieves items with all joined data, supporting filters and pagination
func (r *ItemRepository) FindAllWithDetails(params ItemListParams) ([]models.Item, int, error) {
	// Build the SELECT clause
	selectClause := `SELECT
		i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description, i.status_id, i.priority_id, i.due_date, i.is_task,
		i.milestone_id, i.iteration_id, i.project_id, i.inherit_project, i.time_project_id, i.assignee_id, i.creator_id, i.custom_field_values, i.calendar_data, i.parent_id,
		i.frac_index, i.created_at, i.updated_at,
		w.name as workspace_name, w.key as workspace_key, it.name as item_type_name,
		p.title as parent_title, m.name as milestone_name, iter.name as iteration_name, proj.name as project_name, tp.name as time_project_name,
		assignee.first_name || ' ' || assignee.last_name as assignee_name, assignee.email as assignee_email, assignee.avatar_url as assignee_avatar,
		creator.first_name || ' ' || creator.last_name as creator_name, creator.email as creator_email,
		st.name as status_name, pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color
	`

	fromClause := `FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN items p ON i.parent_id = p.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
	`

	whereClause, args := r.buildWhereClause(params)

	// Get total count
	countQuery := "SELECT COUNT(DISTINCT i.id) " + fromClause + whereClause
	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count items: %w", err)
	}

	// Build ORDER BY clause
	orderByClause := r.buildOrderByClause(params.SortBy, params.SortAsc)

	// Build pagination
	limit := params.Pagination.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := params.Pagination.Offset
	if offset < 0 {
		offset = 0
	}

	// Execute query
	fullQuery := selectClause + fromClause + whereClause + orderByClause + fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	rows, err := r.db.Query(fullQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items, err := r.scanItemList(rows)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// Search searches items by title and description with text matching.
// It delegates to FindAllWithDetails using TextQuery/ItemKeyQuery filters.
func (r *ItemRepository) Search(query string, workspaceIDs []int, pagination PaginationParams) ([]models.Item, int, error) {
	if len(workspaceIDs) == 0 {
		return []models.Item{}, 0, nil
	}

	// Detect workspace key pattern (e.g. "OK-40")
	filters := ItemFilters{}
	parts := strings.Split(strings.ToUpper(query), "-")
	isKeyPattern := len(parts) == 2 && parts[0] != "" && parts[1] != ""
	if isKeyPattern {
		if _, err := strconv.Atoi(parts[1]); err == nil {
			filters.ItemKeyQuery = query
		} else {
			filters.TextQuery = query
		}
	} else {
		filters.TextQuery = query
	}

	return r.FindAllWithDetails(ItemListParams{
		WorkspaceIDs: workspaceIDs,
		Filters:      filters,
		Pagination:   pagination,
		SortBy:       "updated_at",
	})
}

// buildWhereClause constructs the WHERE clause and arguments for item queries
func (r *ItemRepository) buildWhereClause(params ItemListParams) (whereClause string, args []interface{}) {
	whereClause = "WHERE 1=1"

	// Filter by accessible workspaces
	if len(params.WorkspaceIDs) > 0 {
		placeholders := make([]string, len(params.WorkspaceIDs))
		for i, id := range params.WorkspaceIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.workspace_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	// Apply QL query if provided
	if params.Filters.QLQuery != "" {
		whereClause += " AND (" + params.Filters.QLQuery + ")"
		args = append(args, params.Filters.QLArgs...)
	}

	// Apply individual filters
	if params.Filters.WorkspaceID != nil {
		whereClause += " AND i.workspace_id = ?"
		args = append(args, *params.Filters.WorkspaceID)
	}

	if params.Filters.StatusID != nil {
		whereClause += " AND i.status_id = ?"
		args = append(args, *params.Filters.StatusID)
	}

	if params.Filters.PriorityID != nil {
		whereClause += " AND i.priority_id = ?"
		args = append(args, *params.Filters.PriorityID)
	}

	if params.Filters.AssigneeID != nil {
		whereClause += " AND i.assignee_id = ?"
		args = append(args, *params.Filters.AssigneeID)
	}

	if params.Filters.CreatorID != nil {
		whereClause += " AND i.creator_id = ?"
		args = append(args, *params.Filters.CreatorID)
	}

	if params.Filters.ItemTypeID != nil {
		whereClause += " AND i.item_type_id = ?"
		args = append(args, *params.Filters.ItemTypeID)
	}

	if params.Filters.MilestoneID != nil {
		whereClause += " AND i.milestone_id = ?"
		args = append(args, *params.Filters.MilestoneID)
	}

	if params.Filters.IterationID != nil {
		whereClause += " AND i.iteration_id = ?"
		args = append(args, *params.Filters.IterationID)
	}

	// Handle parent_id filter
	if params.Filters.ParentIDIsSet {
		if params.Filters.ParentID == nil || *params.Filters.ParentID == 0 {
			whereClause += " AND i.parent_id IS NULL"
		} else {
			whereClause += " AND i.parent_id = ?"
			args = append(args, *params.Filters.ParentID)
		}
	}

	if params.Filters.Level != nil {
		whereClause += " AND COALESCE(it.hierarchy_level, 0) = ?"
		args = append(args, *params.Filters.Level)
	}

	if params.Filters.MaxLevel != nil {
		whereClause += " AND COALESCE(it.hierarchy_level, 0) <= ?"
		args = append(args, *params.Filters.MaxLevel)
	}

	if params.Filters.CreatedSince != nil {
		whereClause += " AND i.created_at >= ?"
		args = append(args, *params.Filters.CreatedSince)
	}

	// Multi-value status filter
	if len(params.Filters.StatusIDs) > 0 {
		placeholders := make([]string, len(params.Filters.StatusIDs))
		for i, id := range params.Filters.StatusIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.status_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	// Multi-value priority filter
	if len(params.Filters.PriorityIDs) > 0 {
		placeholders := make([]string, len(params.Filters.PriorityIDs))
		for i, id := range params.Filters.PriorityIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.priority_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	// Text search on title/description
	if params.Filters.TextQuery != "" {
		whereClause += " AND (LOWER(i.title) LIKE LOWER(?) OR LOWER(i.description) LIKE LOWER(?))"
		searchPattern := "%" + params.Filters.TextQuery + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// Workspace key pattern match (e.g. "OK-40")
	if params.Filters.ItemKeyQuery != "" {
		parts := strings.Split(strings.ToUpper(params.Filters.ItemKeyQuery), "-")
		if len(parts) == 2 {
			if num, err := strconv.Atoi(parts[1]); err == nil && num > 0 {
				whereClause += " AND (LOWER(w.key) = LOWER(?) AND i.workspace_item_number = ?)"
				args = append(args, parts[0], num)
			}
		}
	}

	// Filter by specific item ID
	if params.Filters.ItemID != nil {
		whereClause += " AND i.id = ?"
		args = append(args, *params.Filters.ItemID)
	}

	return whereClause, args
}

// buildOrderByClause constructs the ORDER BY clause
func (r *ItemRepository) buildOrderByClause(sortBy string, sortAsc bool) string {
	if sortBy == "created_at" {
		return " ORDER BY i.created_at DESC"
	}

	if col, ok := allowedSortColumns[sortBy]; ok {
		direction := "DESC"
		if sortAsc {
			direction = "ASC"
		}
		return fmt.Sprintf(" ORDER BY %s %s", col, direction)
	}

	// Default: prioritize frac_index over creation time
	return ` ORDER BY
		CASE WHEN i.frac_index IS NULL THEN 1 ELSE 0 END,
		i.frac_index ASC,
		i.created_at DESC`
}

// scanItemList scans rows into a slice of items
func (r *ItemRepository) scanItemList(rows *sql.Rows) ([]models.Item, error) {
	var items []models.Item

	for rows.Next() {
		var item models.Item
		var customFieldValuesJSON, calendarDataJSON sql.NullString
		var itemTypeID, parentID, milestoneID, iterationID, projectID, timeProjectID, assigneeID, creatorID, statusID, priorityID sql.NullInt64
		var dueDate sql.NullTime
		var itemTypeName, parentTitle, milestoneName, iterationName, projectName, timeProjectName sql.NullString
		var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail, statusName sql.NullString
		var priorityName, priorityIcon, priorityColor sql.NullString
		var fracIndex sql.NullString
		var inheritProject bool

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
			&statusID, &priorityID, &dueDate, &item.IsTask, &milestoneID, &iterationID, &projectID, &inheritProject, &timeProjectID, &assigneeID, &creatorID, &customFieldValuesJSON, &calendarDataJSON, &parentID,
			&fracIndex, &item.CreatedAt, &item.UpdatedAt, &item.WorkspaceName, &item.WorkspaceKey, &itemTypeName, &parentTitle, &milestoneName, &iterationName, &projectName, &timeProjectName,
			&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail, &statusName, &priorityName, &priorityIcon, &priorityColor,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}

		// Handle nullable fields
		assignNullableInt(&item.ItemTypeID, itemTypeID)
		assignNullableInt(&item.ParentID, parentID)
		assignNullableInt(&item.MilestoneID, milestoneID)
		assignNullableInt(&item.IterationID, iterationID)
		assignNullableInt(&item.StatusID, statusID)
		assignNullableInt(&item.ProjectID, projectID)
		assignNullableInt(&item.TimeProjectID, timeProjectID)
		assignNullableInt(&item.PriorityID, priorityID)
		assignNullableInt(&item.AssigneeID, assigneeID)
		assignNullableInt(&item.CreatorID, creatorID)

		if dueDate.Valid {
			item.DueDate = &dueDate.Time
		}

		item.InheritProject = inheritProject
		assignNullableString(&item.ItemTypeName, itemTypeName)
		assignNullableString(&item.ParentTitle, parentTitle)
		assignNullableString(&item.MilestoneName, milestoneName)
		assignNullableString(&item.IterationName, iterationName)
		assignNullableString(&item.StatusName, statusName)
		assignNullableString(&item.ProjectName, projectName)
		assignNullableString(&item.TimeProjectName, timeProjectName)
		assignNullableString(&item.PriorityName, priorityName)
		assignNullableString(&item.PriorityIcon, priorityIcon)
		assignNullableString(&item.PriorityColor, priorityColor)
		assignNullableString(&item.AssigneeName, assigneeName)
		assignNullableString(&item.AssigneeEmail, assigneeEmail)
		assignNullableString(&item.AssigneeAvatar, assigneeAvatar)
		assignNullableString(&item.CreatorName, creatorName)
		assignNullableString(&item.CreatorEmail, creatorEmail)

		if fracIndex.Valid {
			item.FracIndex = &fracIndex.String
		}

		// Parse custom field values JSON
		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
				item.CustomFieldValues = make(map[string]interface{})
			}
		} else {
			item.CustomFieldValues = make(map[string]interface{})
		}

		// Parse calendar data JSON
		if calendarDataJSON.Valid && calendarDataJSON.String != "" {
			if err := json.Unmarshal([]byte(calendarDataJSON.String), &item.CalendarData); err != nil {
				item.CalendarData = []models.CalendarScheduleEntry{}
			}
		} else {
			item.CalendarData = []models.CalendarScheduleEntry{}
		}

		items = append(items, item)
	}

	if items == nil {
		items = []models.Item{}
	}

	return items, nil
}

// GetBacklogStatusIDs returns status IDs for backlog items.
// It first checks board_configurations for the workspace, then falls back to non-completed statuses.
func (r *ItemRepository) GetBacklogStatusIDs(workspaceID int) ([]int, error) {
	// First, check if there's a board configuration with backlog_status_ids
	if workspaceID > 0 {
		var backlogStatusIDsJSON sql.NullString
		err := r.db.QueryRow(`
			SELECT backlog_status_ids
			FROM board_configurations
			WHERE workspace_id = ?`, workspaceID).Scan(&backlogStatusIDsJSON)

		if err == nil && backlogStatusIDsJSON.Valid && backlogStatusIDsJSON.String != "" {
			var statusIDs []int
			if err := json.Unmarshal([]byte(backlogStatusIDsJSON.String), &statusIDs); err != nil {
				return nil, fmt.Errorf("failed to parse backlog configuration: %w", err)
			}
			if len(statusIDs) > 0 {
				return statusIDs, nil
			}
		}
	}

	// Fall back to global non-completed statuses
	rows, err := r.db.Query(`
		SELECT DISTINCT s.id
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE COALESCE(sc.is_completed, FALSE) = FALSE`)
	if err != nil {
		return nil, fmt.Errorf("failed to query backlog statuses: %w", err)
	}
	defer rows.Close()

	var statusIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan backlog status: %w", err)
		}
		statusIDs = append(statusIDs, id)
	}

	return statusIDs, rows.Err()
}
