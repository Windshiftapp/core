package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// ProgressItemRow is the narrow projection used by planning progress reports.
// It carries only the columns those reports render, so item descriptions,
// custom-field JSON, calendar data, and the status-transition subquery are
// never fetched.
type ProgressItemRow struct {
	ID                  int
	Title               string
	WorkspaceID         int
	WorkspaceKey        string
	WorkspaceItemNumber int
	StatusID            *int
	StatusName          sql.NullString
	StatusBuiltinKey    string
	CategoryName        sql.NullString
	CategoryBuiltinKey  string
	CategoryColor       string
	IsCompleted         sql.NullBool
	PriorityName        sql.NullString
	PriorityBuiltinKey  string
	PriorityColor       string
	AssigneeName        sql.NullString
	AssigneeAvatar      sql.NullString
}

// ListProgressItems returns one keyset page of progress-report rows ordered by
// workspace item number then id. afterNumber/afterID resume after the last row
// of the previous page (0/0 starts at the beginning). It performs no count and
// no OFFSET, so deep pages do not rescan earlier rows. The boolean reports
// whether another page may exist.
func (r *ItemRepository) ListProgressItems(ctx context.Context, params ItemListParams, afterNumber, afterID, limit int) ([]ProgressItemRow, bool, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	whereClause, args := r.buildWhereClause(params)
	cursor := ""
	if afterNumber > 0 || afterID > 0 {
		cursor = " AND (i.workspace_item_number > ? OR (i.workspace_item_number = ? AND i.id > ?))"
		args = append(args, afterNumber, afterNumber, afterID)
	}
	// Fetch one sentinel row so the caller can tell whether a further page
	// exists without issuing a separate count.
	fetch := limit + 1
	query := "WITH page_items AS (SELECT i.id, i.workspace_item_number FROM items i" +
		ItemListFilterJoins() + whereClause + cursor +
		" ORDER BY i.workspace_item_number ASC, i.id ASC LIMIT ?) " +
		`SELECT i.id, i.title, i.workspace_id, w.key, i.workspace_item_number,
		        i.status_id, st.name, COALESCE(st.builtin_key, ''),
		        sc.name, COALESCE(sc.builtin_key, ''), COALESCE(sc.color, ''), sc.is_completed,
		        pri.name, COALESCE(pri.builtin_key, ''), COALESCE(pri.color, ''),
		        assignee.first_name || ' ' || assignee.last_name, assignee.avatar_url
		 FROM items i
		 JOIN workspaces w ON i.workspace_id = w.id
		 LEFT JOIN statuses st ON i.status_id = st.id
		 LEFT JOIN status_categories sc ON st.category_id = sc.id
		 LEFT JOIN priorities pri ON i.priority_id = pri.id
		 LEFT JOIN users assignee ON i.assignee_id = assignee.id
		 JOIN page_items page ON page.id = i.id
		 ORDER BY i.workspace_item_number ASC, i.id ASC`

	rows, err := r.db.QueryContext(ctx, query, append(args, fetch)...)
	if err != nil {
		return nil, false, fmt.Errorf("query progress items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ProgressItemRow, 0, limit)
	for rows.Next() {
		var row ProgressItemRow
		if err := rows.Scan(
			&row.ID, &row.Title, &row.WorkspaceID, &row.WorkspaceKey, &row.WorkspaceItemNumber,
			&row.StatusID, &row.StatusName, &row.StatusBuiltinKey,
			&row.CategoryName, &row.CategoryBuiltinKey, &row.CategoryColor, &row.IsCompleted,
			&row.PriorityName, &row.PriorityBuiltinKey, &row.PriorityColor,
			&row.AssigneeName, &row.AssigneeAvatar,
		); err != nil {
			return nil, false, fmt.Errorf("scan progress item: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate progress items: %w", err)
	}

	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}
