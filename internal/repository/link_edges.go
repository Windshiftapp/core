package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// EdgeQueryChunk bounds one IN list; both endpoints are chunked, so a
	// 1000 x 1000 seed set costs a handful of queries rather than thousands.
	EdgeQueryChunk = 500
)

// LinkEdge is one resolved item-to-asset link.
type LinkEdge struct {
	ItemID  int
	AssetID int
	Label   string
	// Kind is "link" for an item_links row and "field_reference" for an
	// asset-type custom field on the item.
	Kind string
}

// ListAssetItemLinks returns links whose item and asset endpoints are both in
// the given sets. Callers pass already-authorized ID sets, so no per-edge
// permission check is needed here.
func (r *ItemLinkRepository) ListAssetItemLinks(
	ctx context.Context,
	itemIDs []int,
	assetIDs []int,
	maxEdges int,
) (edges []LinkEdge, truncated bool, err error) {
	if len(itemIDs) == 0 || len(assetIDs) == 0 {
		return nil, false, nil
	}

	for itemStart := 0; itemStart < len(itemIDs); itemStart += EdgeQueryChunk {
		itemEnd := min(itemStart+EdgeQueryChunk, len(itemIDs))
		itemChunk := itemIDs[itemStart:itemEnd]

		for assetStart := 0; assetStart < len(assetIDs); assetStart += EdgeQueryChunk {
			assetEnd := min(assetStart+EdgeQueryChunk, len(assetIDs))
			assetChunk := assetIDs[assetStart:assetEnd]

			for _, direction := range []string{"source", "target"} {
				query, args := assetItemLinkQuery(direction, itemChunk, assetChunk)
				rows, err := r.db.QueryContext(ctx, query, args...)
				if err != nil {
					return nil, false, fmt.Errorf("list asset item links: %w", err)
				}
				for rows.Next() {
					var edge LinkEdge
					if err := rows.Scan(&edge.ItemID, &edge.AssetID, &edge.Label); err != nil {
						_ = rows.Close()
						return nil, false, fmt.Errorf("scan asset item link: %w", err)
					}
					edge.Kind = "link"
					if len(edges) >= maxEdges {
						truncated = true
						continue
					}
					edges = append(edges, edge)
				}
				if err := rows.Err(); err != nil {
					_ = rows.Close()
					return nil, false, fmt.Errorf("iterate asset item links: %w", err)
				}
				_ = rows.Close()
			}
		}
	}
	return edges, truncated, nil
}

// assetItemLinkQuery builds one direction of the item-to-asset link join. The
// label comes from the link type so the rendered edge reads like the UI does.
func assetItemLinkQuery(direction string, itemIDs, assetIDs []int) (query string, args []any) {
	itemColumn, assetColumn := "il.source_id", "il.target_id"
	itemPredicate := "il.source_type = 'item' AND il.target_type = 'asset'"
	if direction == "target" {
		itemColumn, assetColumn = "il.target_id", "il.source_id"
		itemPredicate = "il.target_type = 'item' AND il.source_type = 'asset'"
	}

	query = `
		SELECT ` + itemColumn + `, ` + assetColumn + `, COALESCE(lt.forward_label, lt.name, '')
		FROM item_links il
		LEFT JOIN link_types lt ON il.link_type_id = lt.id
		WHERE ` + itemPredicate + `
		  AND ` + itemColumn + ` IN (` + placeholders(len(itemIDs)) + `)
		  AND ` + assetColumn + ` IN (` + placeholders(len(assetIDs)) + `)
		ORDER BY il.id`

	args = make([]any, 0, len(itemIDs)+len(assetIDs))
	for _, id := range itemIDs {
		args = append(args, id)
	}
	for _, id := range assetIDs {
		args = append(args, id)
	}
	return query, args
}

// ListItemAssetReferences returns, per item, the asset IDs referenced by
// asset-type custom fields. Items with no references are absent from the map.
func (r *ItemRepository) ListItemAssetReferences(ctx context.Context, itemIDs []int) (map[int][]int, error) {
	references := make(map[int][]int)
	if len(itemIDs) == 0 {
		return references, nil
	}

	fieldIDs, err := r.assetReferenceFieldIDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(fieldIDs) == 0 {
		return references, nil
	}

	for start := 0; start < len(itemIDs); start += EdgeQueryChunk {
		end := min(start+EdgeQueryChunk, len(itemIDs))
		chunk := itemIDs[start:end]

		rows, err := r.db.QueryContext(ctx,
			`SELECT id, COALESCE(custom_field_values, '') FROM items WHERE id IN (`+placeholders(len(chunk))+`)`,
			intArgs(chunk)...,
		)
		if err != nil {
			return nil, fmt.Errorf("load item custom field values: %w", err)
		}
		for rows.Next() {
			var itemID int
			var encoded string
			if err := rows.Scan(&itemID, &encoded); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan item custom field values: %w", err)
			}
			ids := assetIDsFromCustomFields(encoded, fieldIDs)
			if len(ids) > 0 {
				references[itemID] = ids
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate item custom field values: %w", err)
		}
		_ = rows.Close()
	}
	return references, nil
}

func (r *ItemRepository) assetReferenceFieldIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM custom_field_definitions WHERE field_type = 'asset'`)
	if err != nil {
		return nil, fmt.Errorf("load asset custom fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := make([]string, 0, 4)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan asset custom field: %w", err)
		}
		ids = append(ids, fmt.Sprintf("%d", id))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset custom fields: %w", err)
	}
	return ids, nil
}

// assetIDsFromCustomFields extracts referenced asset IDs from a stored custom
// field blob, which may hold a bare ID, an {id, ...} object, or an array of
// either.
func assetIDsFromCustomFields(encoded string, fieldIDs []string) []int {
	if strings.TrimSpace(encoded) == "" {
		return nil
	}
	values := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(encoded), &values); err != nil {
		return nil
	}

	var ids []int
	for _, fieldID := range fieldIDs {
		raw, ok := values[fieldID]
		if !ok {
			continue
		}
		for _, id := range assetIDsFromValue(raw) {
			if id > 0 {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func assetIDsFromValue(raw json.RawMessage) []int {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil
		}
		var ids []int
		for _, entry := range entries {
			ids = append(ids, assetIDsFromValue(entry)...)
		}
		return ids
	}

	var scalar int
	if err := json.Unmarshal(raw, &scalar); err == nil && scalar > 0 {
		return []int{scalar}
	}
	var object struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw, &object); err == nil && object.ID > 0 {
		return []int{object.ID}
	}
	return nil
}

func intArgs(values []int) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}
