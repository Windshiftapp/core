package repository

import (
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ItemLinkPair represents a directed item-to-item link.
type ItemLinkPair struct {
	SourceID int
	TargetID int
}

type CaptureItemLink struct {
	LinkType string
	TargetID int
}

// ItemLinkRepository provides data access methods for the item_links table.
type ItemLinkRepository struct {
	db database.Database
}

// ListCaptureItemLinks returns outgoing item-to-item links for export verification.
func (r *ItemLinkRepository) ListCaptureItemLinks(itemID int) ([]CaptureItemLink, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(lt.name, ''), il.target_id
		FROM item_links il
		LEFT JOIN link_types lt ON lt.id = il.link_type_id
		WHERE il.source_type = 'item' AND il.source_id = ? AND il.target_type = 'item'
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list capture item links: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []CaptureItemLink{}
	for rows.Next() {
		var link CaptureItemLink
		if err := rows.Scan(&link.LinkType, &link.TargetID); err != nil {
			return nil, fmt.Errorf("scan capture item link: %w", err)
		}
		out = append(out, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate capture item links: %w", err)
	}
	return out, nil
}

// LinkedItemSummary is the small projection used by item briefings.
type LinkedItemSummary struct {
	LinkType string
	Title    string
	ItemKey  string
}

// NewItemLinkRepository creates a new ItemLinkRepository.
func NewItemLinkRepository(db database.Database) *ItemLinkRepository {
	return &ItemLinkRepository{db: db}
}

// FindItemToItemLinksWithin returns every item-to-item link whose source_id
// and target_id both fall within the provided set of item IDs. Returns an
// empty slice when itemIDs is empty.
func (r *ItemLinkRepository) FindItemToItemLinksWithin(itemIDs []int) ([]ItemLinkPair, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(itemIDs))
	for i := range itemIDs {
		placeholders[i] = "?"
	}
	args := make([]any, 0, len(itemIDs)*2)
	for _, id := range itemIDs {
		args = append(args, id)
	}
	for _, id := range itemIDs {
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT source_id, target_id FROM item_links
		WHERE source_type = 'item' AND target_type = 'item'
		  AND source_id IN (%s) AND target_id IN (%s)`,
		strings.Join(placeholders, ","),
		strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query item_links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pairs []ItemLinkPair
	for rows.Next() {
		var p ItemLinkPair
		if err := rows.Scan(&p.SourceID, &p.TargetID); err != nil {
			return nil, fmt.Errorf("scan item_link row: %w", err)
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item_links: %w", err)
	}
	return pairs, nil
}

// ListLinkedItemSummaries returns both incoming and outgoing item links.
func (r *ItemLinkRepository) ListLinkedItemSummaries(itemID int) ([]LinkedItemSummary, error) {
	rows, err := r.db.Query(`
		SELECT lt.name, linked.title, w.key || '-' || linked.workspace_item_number
		FROM item_links il
		JOIN link_types lt ON il.link_type_id = lt.id
		JOIN items linked ON linked.id = CASE
			WHEN il.source_type = 'item' AND il.source_id = ? THEN il.target_id
			ELSE il.source_id
		END
		JOIN workspaces w ON linked.workspace_id = w.id
		WHERE (il.source_type = 'item' AND il.source_id = ? AND il.target_type = 'item')
		   OR (il.target_type = 'item' AND il.target_id = ? AND il.source_type = 'item')
	`, itemID, itemID, itemID)
	if err != nil {
		return nil, fmt.Errorf("list linked item summaries for item %d: %w", itemID, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]LinkedItemSummary, 0)
	for rows.Next() {
		var summary LinkedItemSummary
		if err := rows.Scan(&summary.LinkType, &summary.Title, &summary.ItemKey); err != nil {
			return nil, fmt.Errorf("scan linked item summary for item %d: %w", itemID, err)
		}
		out = append(out, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked item summaries for item %d: %w", itemID, err)
	}
	return out, nil
}

// FindLinkedItems returns the items linked to itemID via item_links rows
// where source_type='item' and target_type='item'. Filters:
//
//   - linkTypeID nil → any link type
//   - direction "outgoing" → itemID is the source; the linked items are the targets
//   - direction "incoming" → itemID is the target; the linked items are the sources
//   - direction "" or "both" → union of both directions
//
// Items are loaded with the same joined-detail shape as ItemRepository.GetChildren
// so iterator-driven node bodies can read status/workspace/item_type fields without
// a second fetch.
func (r *ItemLinkRepository) FindLinkedItems(itemID int, linkTypeID *int, direction string) ([]*models.Item, error) {
	if direction == "" {
		direction = "both"
	}

	var (
		clauses []string
		args    []any
	)
	if direction == "outgoing" || direction == "both" {
		c := "(il.source_type = 'item' AND il.target_type = 'item' AND il.source_id = ?)"
		clauses = append(clauses, c)
		args = append(args, itemID)
	}
	if direction == "incoming" || direction == "both" {
		c := "(il.source_type = 'item' AND il.target_type = 'item' AND il.target_id = ?)"
		clauses = append(clauses, c)
		args = append(args, itemID)
	}
	if len(clauses) == 0 {
		return nil, fmt.Errorf("invalid direction %q (want outgoing|incoming|both)", direction)
	}

	query := `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.is_task, i.iteration_id,
		       i.project_id, i.inherit_project, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.frac_index, i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name,
		       sc.color as status_color,
		       s.builtin_key as status_builtin_key,
		       it.name as item_type_name
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.id IN (
		    SELECT CASE WHEN il.source_id = ? THEN il.target_id ELSE il.source_id END
		    FROM item_links il
		    WHERE (` + strings.Join(clauses, " OR ") + `)`
	args = append([]any{itemID}, args...)

	if linkTypeID != nil {
		query += ` AND il.link_type_id = ?`
		args = append(args, *linkTypeID)
	}

	query += `
		)
		ORDER BY i.frac_index
	`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query linked items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanItemsWithDetails(rows)
}
