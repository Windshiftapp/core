package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ReleaseNoteComment is one public comment attached to a work item, reduced to
// the author, body, and creation date the release-notes prompt can summarize.
// Private comments are never selected.
type ReleaseNoteComment struct {
	Author    string
	Content   string
	CreatedAt time.Time
}

// ReleaseNoteItem is the release-notes enrichment for one work item: the fields
// the model needs beyond the key, title and status already present in the
// milestone progress report. Comments hold the newest public comments first,
// capped per item by the caller.
type ReleaseNoteItem struct {
	ItemID             int
	Description        string
	ItemTypeName       string
	ItemTypeBuiltinKey string
	ParentKey          string
	ParentItemNumber   int
	ParentTitle        string
	Comments           []ReleaseNoteComment
}

// ReleaseNoteItemsContext returns enrichment for the supplied work-item IDs,
// keyed by item ID. IDs with no matching row are omitted. commentsPerItem <= 0
// skips the comment query entirely. Callers must pass only IDs they have
// already authorized: this read performs no permission filtering of its own.
func (r *ItemRepository) ReleaseNoteItemsContext(ctx context.Context, itemIDs []int, commentsPerItem int) (map[int]ReleaseNoteItem, error) {
	result := make(map[int]ReleaseNoteItem, len(itemIDs))
	if len(itemIDs) == 0 {
		return result, nil
	}

	in, args := inPlaceholders(itemIDs)
	// The parent join is scoped to the item's own hierarchy; a parent may live in
	// the same workspace, so the workspace key is read from the parent row.
	query := `SELECT i.id, COALESCE(i.description, ''),
			COALESCE(it.name, ''), COALESCE(it.builtin_key, ''),
			COALESCE(pw.key, ''), p.workspace_item_number, COALESCE(p.title, '')
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN items p ON i.parent_id = p.id
		LEFT JOIN workspaces pw ON p.workspace_id = pw.id
		WHERE i.id IN (` + in + `)`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query release note items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var item ReleaseNoteItem
		var parentNumber sql.NullInt64
		if err := rows.Scan(&item.ItemID, &item.Description, &item.ItemTypeName, &item.ItemTypeBuiltinKey,
			&item.ParentKey, &parentNumber, &item.ParentTitle); err != nil {
			return nil, fmt.Errorf("scan release note item: %w", err)
		}
		if parentNumber.Valid {
			item.ParentItemNumber = int(parentNumber.Int64)
		}
		result[item.ItemID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate release note items: %w", err)
	}

	if commentsPerItem > 0 {
		comments, err := r.releaseNoteComments(ctx, itemIDs, commentsPerItem)
		if err != nil {
			return nil, err
		}
		for itemID, itemComments := range comments {
			item, ok := result[itemID]
			if !ok {
				continue
			}
			item.Comments = itemComments
			result[itemID] = item
		}
	}

	return result, nil
}

// releaseNoteComments returns at most perItem public comments for each supplied
// item, newest first, keyed by item ID. A window function caps the rows in SQL
// so a long comment thread on one item cannot push other items out of the
// result. Private comments are excluded.
func (r *ItemRepository) releaseNoteComments(ctx context.Context, itemIDs []int, perItem int) (map[int][]ReleaseNoteComment, error) {
	in, args := inPlaceholders(itemIDs)
	args = append(args, perItem)
	query := `SELECT item_id, author, content, created_at FROM (
			SELECT c.item_id,
				COALESCE(NULLIF(TRIM(u.first_name || ' ' || u.last_name), ''), u.username, pc.name, 'Unknown') AS author,
				c.content,
				c.created_at,
				ROW_NUMBER() OVER (PARTITION BY c.item_id ORDER BY c.created_at DESC, c.id DESC) AS rn
			FROM comments c
			LEFT JOIN users u ON c.author_id = u.id
			LEFT JOIN portal_customers pc ON c.portal_customer_id = pc.id
			WHERE c.item_id IN (` + in + `) AND c.is_private = false
		) ranked
		WHERE rn <= ?
		ORDER BY item_id ASC, rn ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query release note comments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int][]ReleaseNoteComment)
	for rows.Next() {
		var itemID int
		var comment ReleaseNoteComment
		if err := rows.Scan(&itemID, &comment.Author, &comment.Content, &comment.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan release note comment: %w", err)
		}
		result[itemID] = append(result[itemID], comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate release note comments: %w", err)
	}
	return result, nil
}
