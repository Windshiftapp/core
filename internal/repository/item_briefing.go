package repository

import (
	"fmt"
	"time"
)

// This file hosts the read queries that feed the daily-briefing digest. They
// previously lived as inline SQL in internal/scheduler/briefing_scheduler.go;
// moving them here keeps the scheduler a thin orchestrator and puts the item
// joins next to the rest of the item data-access layer. The result structs are
// intentionally denormalized for digest rendering — callers format the lines.

// BriefingChange is a single recent item-history change. OldValue/NewValue are
// the raw stored values (often a *_id); the caller resolves them to display
// names via the lookup maps.
type BriefingChange struct {
	FieldName string
	OldValue  string
	NewValue  string
	ItemKey   string
	Title     string
	ChangedBy string
	ChangedAt time.Time
}

// userItemScope returns the SQL predicate and arguments that restrict an item
// join to the work a user is responsible for: items assigned to them, plus
// every item in their personal workspaces. The predicate starts with " AND" so
// callers append it to an existing WHERE clause. It is empty when userID is not
// positive, which keeps the workspace-wide feed for callers that want it.
func userItemScope(personalWorkspaceIDs []int, userID int) (clause string, args []any) {
	if userID <= 0 {
		return "", nil
	}
	clause = " AND (i.assignee_id = ?"
	args = []any{userID}
	if pph, pArgs := inPlaceholders(personalWorkspaceIDs); pph != "" {
		clause += fmt.Sprintf(" OR i.workspace_id IN (%s)", pph)
		args = append(args, pArgs...)
	}
	return clause + ")", args
}

// RecentItemChanges returns up to limit item-history changes across the given
// workspaces since the supplied instant, most recent first. Returns nil when
// no workspaces are supplied.
func (r *ItemRepository) RecentItemChanges(workspaceIDs []int, since time.Time, limit int) ([]BriefingChange, error) {
	return r.recentItemChanges(workspaceIDs, nil, 0, since, limit)
}

// RecentItemChangesForUser is RecentItemChanges restricted to the items the
// user is responsible for. The briefing uses this form: a workspace-wide feed
// is identical for every member of a workspace, so users without assigned
// items would otherwise receive the same recap of someone else's work.
func (r *ItemRepository) RecentItemChangesForUser(workspaceIDs, personalWorkspaceIDs []int, userID int, since time.Time, limit int) ([]BriefingChange, error) {
	return r.recentItemChanges(workspaceIDs, personalWorkspaceIDs, userID, since, limit)
}

func (r *ItemRepository) recentItemChanges(workspaceIDs, personalWorkspaceIDs []int, userID int, since time.Time, limit int) ([]BriefingChange, error) {
	wsIn, args := inPlaceholders(workspaceIDs)
	if wsIn == "" {
		return nil, nil
	}
	args = append(args, since)
	scope, scopeArgs := userItemScope(personalWorkspaceIDs, userID)
	args = append(args, scopeArgs...)
	query := fmt.Sprintf(`SELECT ih.field_name, COALESCE(ih.old_value, ''), COALESCE(ih.new_value, ''), ih.changed_at,
		w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
		COALESCE(NULLIF(TRIM(u.first_name || ' ' || u.last_name), ''), u.username, pc.name, 'System') as changed_by
		FROM item_history ih
		JOIN items i ON ih.item_id = i.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN users u ON ih.user_id = u.id
		LEFT JOIN portal_customers pc ON ih.actor_portal_customer_id = pc.id
		WHERE i.workspace_id IN (%s) AND ih.changed_at >= ?%s
		ORDER BY ih.changed_at DESC LIMIT %d`, wsIn, scope, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query recent item changes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []BriefingChange
	for rows.Next() {
		var c BriefingChange
		if err := rows.Scan(&c.FieldName, &c.OldValue, &c.NewValue, &c.ChangedAt, &c.ItemKey, &c.Title, &c.ChangedBy); err != nil {
			return nil, fmt.Errorf("scan recent item change: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// BriefingComment is a single recent public comment, denormalized for the
// digest. Content is the full stored body; the caller truncates for display.
type BriefingComment struct {
	Content   string
	ItemKey   string
	Title     string
	Author    string
	CreatedAt time.Time
}

// RecentComments returns up to limit public comments on items in the given
// workspaces since the supplied instant, most recent first. Private comments
// are excluded. Returns nil when no workspaces are supplied.
func (r *ItemRepository) RecentComments(workspaceIDs []int, since time.Time, limit int) ([]BriefingComment, error) {
	return r.recentComments(workspaceIDs, nil, 0, since, limit)
}

// RecentCommentsForUser is RecentComments restricted to the items the user is
// responsible for. The briefing uses this form so a comment on a colleague's
// item does not appear as the user's own recap.
func (r *ItemRepository) RecentCommentsForUser(workspaceIDs, personalWorkspaceIDs []int, userID int, since time.Time, limit int) ([]BriefingComment, error) {
	return r.recentComments(workspaceIDs, personalWorkspaceIDs, userID, since, limit)
}

func (r *ItemRepository) recentComments(workspaceIDs, personalWorkspaceIDs []int, userID int, since time.Time, limit int) ([]BriefingComment, error) {
	wsIn, args := inPlaceholders(workspaceIDs)
	if wsIn == "" {
		return nil, nil
	}
	args = append(args, since)
	scope, scopeArgs := userItemScope(personalWorkspaceIDs, userID)
	args = append(args, scopeArgs...)
	query := fmt.Sprintf(`SELECT c.content, c.created_at,
		w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
		COALESCE(u.first_name || ' ' || u.last_name, 'Unknown') as author
		FROM comments c
		JOIN items i ON c.item_id = i.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN users u ON c.author_id = u.id
		WHERE i.workspace_id IN (%s) AND c.created_at >= ? AND c.is_private = false%s
		ORDER BY c.created_at DESC LIMIT %d`, wsIn, scope, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query recent comments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []BriefingComment
	for rows.Next() {
		var c BriefingComment
		if err := rows.Scan(&c.Content, &c.CreatedAt, &c.ItemKey, &c.Title, &c.Author); err != nil {
			return nil, fmt.Errorf("scan recent comment: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// BriefingOpenItem is a single open (non-completed) item for the digest, with
// status/priority/milestone/iteration already resolved to names. Date columns
// are kept as raw strings ("" when absent) for direct rendering.
type BriefingOpenItem struct {
	WorkspaceKey        string
	ItemNumber          int
	Title               string
	Status              string
	Priority            string
	DueDate             string
	MilestoneName       string
	MilestoneTargetDate string
	IterationName       string
	IterationEndDate    string
}

// OpenItemsForUser returns up to limit non-completed items the user should see
// in their briefing: items assigned to the user within the accessible
// workspaces, plus every item in the user's personal workspaces. Ordered by due
// date ascending with undated items last. Returns nil when no workspaces are
// supplied or userID is not positive.
func (r *ItemRepository) OpenItemsForUser(workspaceIDs, personalWorkspaceIDs []int, userID, limit int) ([]BriefingOpenItem, error) {
	wsIn, args := inPlaceholders(workspaceIDs)
	if wsIn == "" || userID <= 0 {
		return nil, nil
	}
	scope, scopeArgs := userItemScope(personalWorkspaceIDs, userID)
	args = append(args, scopeArgs...)

	query := fmt.Sprintf(`SELECT w.key, i.workspace_item_number, i.title,
		COALESCE(st.name, ''), COALESCE(p.name, ''), COALESCE(CAST(i.due_date AS TEXT), ''),
		COALESCE(m.name, ''), COALESCE(CAST(m.target_date AS TEXT), ''),
		COALESCE(iter.name, ''), COALESCE(CAST(iter.end_date AS TEXT), '')
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN item_milestones im ON im.item_id = i.id
		LEFT JOIN milestones m ON m.id = im.milestone_id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		WHERE i.workspace_id IN (%s)%s
		AND COALESCE(sc.is_completed, FALSE) = FALSE
		ORDER BY i.due_date ASC NULLS LAST LIMIT %d`, wsIn, scope, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query open items for user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []BriefingOpenItem
	for rows.Next() {
		var it BriefingOpenItem
		if err := rows.Scan(&it.WorkspaceKey, &it.ItemNumber, &it.Title, &it.Status, &it.Priority,
			&it.DueDate, &it.MilestoneName, &it.MilestoneTargetDate, &it.IterationName, &it.IterationEndDate); err != nil {
			return nil, fmt.Errorf("scan open item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
