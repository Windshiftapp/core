package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/models"
)

// HubInboxFilter captures the user-scoped filters the hub inbox endpoint
// supports: an optional portal_id, an optional status name, and pagination.
type HubInboxFilter struct {
	UserID       int
	PortalID     *int   // nil = no portal filter
	StatusFilter string // empty = no status filter
	PerPage      int
	Offset       int
}

// ListHubInboxItems returns the items submitted via portal by the given user,
// the total row count (ignoring pagination but honoring portal + status
// filters), and the distinct status-name/color facets across the user's
// submissions (computed without the status filter so the UI dropdown keeps
// every option visible).
func (r *ItemRepository) ListHubInboxItems(ctx context.Context, f HubInboxFilter) ([]models.HubInboxItem, int, []models.HubInboxStatusFacet, error) {
	baseFrom := `
		FROM items i
		JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN channels c ON i.channel_id = c.id
		LEFT JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
		WHERE c.type = 'portal' AND i.creator_id = ?
	`
	facetArgs := []interface{}{f.UserID}
	if f.PortalID != nil {
		baseFrom += " AND c.id = ?"
		facetArgs = append(facetArgs, *f.PortalID)
	}

	baseQuery := baseFrom
	args := append([]interface{}{}, facetArgs...)
	if f.StatusFilter != "" {
		baseQuery += " AND s.name = ?"
		args = append(args, f.StatusFilter)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT i.id) "+baseQuery, args...).Scan(&total); err != nil {
		return nil, 0, nil, fmt.Errorf("hub inbox count: %w", err)
	}

	itemArgs := append([]interface{}{}, args...)
	itemArgs = append(itemArgs, f.PerPage, f.Offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			i.id, i.title, COALESCE(i.description, ''), i.created_at,
			s.name, COALESCE(sc.color, '#6b7280'),
			w.key, i.workspace_item_number,
			COALESCE(c.name, ''), COALESCE(JSON_EXTRACT(c.config, '$.portal_slug'), ''),
			pc.name, pc.email
	`+baseQuery+`
		ORDER BY i.created_at DESC
		LIMIT ? OFFSET ?
	`, itemArgs...)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("hub inbox list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []models.HubInboxItem{}
	for rows.Next() {
		var item models.HubInboxItem
		var submitterName, submitterEmail sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Title, &item.Description, &item.CreatedAt,
			&item.StatusName, &item.StatusColor,
			&item.WorkspaceKey, &item.WorkspaceItemNumber,
			&item.PortalName, &item.PortalSlug,
			&submitterName, &submitterEmail,
		); err != nil {
			return nil, 0, nil, fmt.Errorf("scan hub inbox row: %w", err)
		}
		if submitterName.Valid {
			item.SubmitterName = &submitterName.String
		}
		if submitterEmail.Valid {
			item.SubmitterEmail = &submitterEmail.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, err
	}

	facets := []models.HubInboxStatusFacet{}
	facetRows, err := r.db.QueryContext(ctx, "SELECT DISTINCT s.name, COALESCE(sc.color, '#6b7280') "+baseFrom+" ORDER BY s.name ASC", facetArgs...)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("hub inbox facets: %w", err)
	}
	defer func() { _ = facetRows.Close() }()
	for facetRows.Next() {
		var f models.HubInboxStatusFacet
		if err := facetRows.Scan(&f.Name, &f.Color); err != nil {
			return nil, 0, nil, err
		}
		facets = append(facets, f)
	}
	return items, total, facets, facetRows.Err()
}

// CountHubOpenRequests returns the number of portal-submitted items created by
// userID whose status category is not marked as completed. Mirrors the join
// shape of ListHubInboxItems so the count stays semantically aligned.
func (r *ItemRepository) CountHubOpenRequests(ctx context.Context, userID int) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT i.id)
		FROM items i
		JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		LEFT JOIN channels c ON i.channel_id = c.id
		WHERE c.type = 'portal'
		  AND i.creator_id = ?
		  AND COALESCE(sc.is_completed, FALSE) = FALSE
	`, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("hub open request count: %w", err)
	}
	return n, nil
}

// FindHubInboxItem returns a single hub-inbox item (portal submission) owned
// by the given user. ErrNotFound when the row doesn't exist or belongs to
// someone else.
func (r *ItemRepository) FindHubInboxItem(ctx context.Context, userID, itemID int) (*models.HubInboxItem, error) {
	var item models.HubInboxItem
	var submitterName, submitterEmail sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT
			i.id, i.title, COALESCE(i.description, ''), i.created_at,
			s.name, COALESCE(sc.color, '#6b7280'),
			w.key, i.workspace_item_number,
			COALESCE(c.name, ''), COALESCE(JSON_EXTRACT(c.config, '$.portal_slug'), ''),
			pc.name, pc.email
		FROM items i
		JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN channels c ON i.channel_id = c.id
		LEFT JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
		WHERE i.id = ? AND c.type = 'portal' AND i.creator_id = ?
	`, itemID, userID).Scan(
		&item.ID, &item.Title, &item.Description, &item.CreatedAt,
		&item.StatusName, &item.StatusColor,
		&item.WorkspaceKey, &item.WorkspaceItemNumber,
		&item.PortalName, &item.PortalSlug,
		&submitterName, &submitterEmail,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find hub inbox item: %w", err)
	}
	if submitterName.Valid {
		item.SubmitterName = &submitterName.String
	}
	if submitterEmail.Valid {
		item.SubmitterEmail = &submitterEmail.String
	}
	return &item, nil
}
