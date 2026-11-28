package repository

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"time"

	"windshift/internal/models"
)

type SCMLinkSummary struct {
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	State     string    `json:"recorded_state"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ItemCompletionFact struct {
	Completed bool       `json:"currently_completed"`
	At        *time.Time `json:"last_completion_at,omitempty"`
	ActorID   *int       `json:"actor_user_id,omitempty"`
	ActorName string     `json:"actor_name,omitempty"`
	ActorKind string     `json:"actor_kind,omitempty"`
	HistoryID *int       `json:"history_id,omitempty"`
}

const completionEventRows = `FROM item_history h
	JOIN statuses next ON CAST(next.id AS TEXT) = h.new_value
	JOIN status_categories next_category ON next_category.id = next.category_id AND next_category.is_completed = TRUE
	LEFT JOIN statuses previous ON CAST(previous.id AS TEXT) = h.old_value
	LEFT JOIN status_categories previous_category ON previous_category.id = previous.category_id
	WHERE h.field_name = 'status_id' AND COALESCE(previous_category.is_completed, FALSE) = FALSE`

const completionEventJoin = `LEFT JOIN item_history completion ON completion.id = (
	SELECT h.id ` + completionEventRows + ` AND h.item_id = i.id
	ORDER BY h.changed_at DESC, h.id DESC LIMIT 1
)`

func (r *ItemRepository) CompletionFacts(ctx context.Context, items []models.Item) (map[int]ItemCompletionFact, error) {
	result := make(map[int]ItemCompletionFact, len(items))
	for batch := range slices.Chunk(items, 200) {
		args := make([]any, 0, len(batch))
		for _, item := range batch {
			args = append(args, item.ID)
		}
		params := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		rows, err := r.db.QueryContext(ctx, `SELECT i.id, COALESCE(current_category.is_completed, FALSE), completion.changed_at, completion.user_id, completion.id,
			COALESCE(NULLIF(TRIM(COALESCE(actor.first_name,'') || ' ' || COALESCE(actor.last_name,'')),''),actor.username,''),
			COALESCE(completion.actor_kind,'')
			FROM items i LEFT JOIN statuses current_status ON current_status.id=i.status_id
			LEFT JOIN status_categories current_category ON current_category.id=current_status.category_id
			`+completionEventJoin+" LEFT JOIN users actor ON actor.id=completion.user_id WHERE i.id IN ("+params+")", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int
			var record ItemCompletionFact
			var at sql.NullTime
			if err := rows.Scan(&id, &record.Completed, &at, &record.ActorID, &record.HistoryID, &record.ActorName, &record.ActorKind); err != nil {
				rows.Close()
				return nil, err
			}
			if at.Valid {
				record.At = &at.Time
			}
			result[id] = record
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// CompletedDuring includes items that entered a completed state in the interval,
// including items that have since been reopened or completed again.
func (r *ItemRepository) CompletedDuring(ctx context.Context, items []models.Item, from, to time.Time) (map[int]bool, error) {
	result := make(map[int]bool)
	for batch := range slices.Chunk(items, 200) {
		params := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, 0, len(batch)+2)
		args = append(args, from.UTC(), to.UTC())
		for _, item := range batch {
			args = append(args, item.ID)
		}
		rows, err := r.db.QueryContext(ctx, "SELECT DISTINCT h.item_id "+completionEventRows+
			" AND h.changed_at >= ? AND h.changed_at < ? AND h.item_id IN ("+params+")", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			result[id] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *ItemRepository) ListSCMLinkSummaries(ctx context.Context, itemID, limit, offset int) ([]SCMLinkSummary, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT link_type, COALESCE(title, ''), COALESCE(external_url, ''),
		COALESCE(state, ''), updated_at FROM item_scm_links WHERE item_id = ?
		ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`, itemID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []SCMLinkSummary{}
	for rows.Next() {
		var link SCMLinkSummary
		if err := rows.Scan(&link.Type, &link.Title, &link.URL, &link.State, &link.UpdatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (r *ItemRepository) ListRecentlyCompletedForUser(ctx context.Context, userID int, workspaces []int, from, to time.Time, limit, offset int) ([]models.Item, error) {
	items := []models.Item{}
	if len(workspaces) == 0 {
		return items, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(workspaces)), ",")
	args := []any{from.UTC(), to.UTC(), userID, userID}
	for _, id := range workspaces {
		args = append(args, id)
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `SELECT i.id, i.workspace_id, i.workspace_item_number, w.key, i.title, completion.changed_at
		FROM items i JOIN workspaces w ON w.id = i.workspace_id
		JOIN item_history completion ON completion.id = (
			SELECT h.id `+completionEventRows+` AND h.item_id = i.id AND h.changed_at >= ? AND h.changed_at < ?
			ORDER BY h.changed_at DESC, h.id DESC LIMIT 1
		)
		WHERE (i.assignee_id = ? OR (w.is_personal = TRUE AND w.owner_id = ?))
		  AND i.workspace_id IN (`+marks+`)
		ORDER BY completion.changed_at DESC, i.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item models.Item
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &item.WorkspaceKey, &item.Title, &item.CompletedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type ItemDiscussionMatch struct {
	ID        int
	ItemID    int
	Content   string
	CreatedAt time.Time
	IsPrivate bool
}

func (r *ItemRepository) SearchDiscussions(ctx context.Context, workspaces []int, query string, limit int) ([]ItemDiscussionMatch, error) {
	matches := []ItemDiscussionMatch{}
	if len(workspaces) == 0 {
		return matches, nil
	}
	pattern := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(strings.TrimSpace(query)))
	args := []any{"%" + pattern + "%"}
	for _, id := range workspaces {
		args = append(args, id)
	}
	args = append(args, limit)
	marks := strings.TrimSuffix(strings.Repeat("?,", len(workspaces)), ",")
	rows, err := r.db.QueryContext(ctx, `SELECT c.id, c.item_id, c.content, c.created_at, c.is_private
		FROM comments c JOIN items i ON i.id = c.item_id
		WHERE LOWER(c.content) LIKE ? ESCAPE '!' AND i.workspace_id IN (`+marks+`)
		ORDER BY c.created_at DESC, c.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var match ItemDiscussionMatch
		if err := rows.Scan(&match.ID, &match.ItemID, &match.Content, &match.CreatedAt, &match.IsPrivate); err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	return matches, rows.Err()
}
