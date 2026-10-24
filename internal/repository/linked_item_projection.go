package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// MaxProjectedLinksPerAsset bounds how many linked items one asset carries.
	MaxProjectedLinksPerAsset = 20
	// MaxProjectedLinksTotal bounds the whole projection for one slot.
	MaxProjectedLinksTotal = 5000
	// linkedProjectionChunk keeps IN lists well under SQLite's variable limit.
	linkedProjectionChunk = 500
)

// ProjectLinkedItems returns, per asset ID, the items linked to that asset,
// projected to the requested fields.
//
// Only items in accessibleWorkspaceIDs are returned: an asset the viewer can
// read can link to items in workspaces they cannot, and an empty workspace set
// means no links are visible. The second return value reports truncation.
func (r *ItemLinkRepository) ProjectLinkedItems(
	ctx context.Context,
	assetIDs []int,
	accessibleWorkspaceIDs []int,
	fields []string,
) (projected map[int][]map[string]any, truncated bool, err error) {
	if len(assetIDs) == 0 || len(accessibleWorkspaceIDs) == 0 || len(fields) == 0 {
		return map[int][]map[string]any{}, false, nil
	}

	customFieldIDs, err := r.customFieldIDsForFields(ctx, fields)
	if err != nil {
		return nil, false, err
	}

	projected = make(map[int][]map[string]any)

	for start := 0; start < len(assetIDs); start += linkedProjectionChunk {
		end := min(start+linkedProjectionChunk, len(assetIDs))
		chunk := assetIDs[start:end]

		for _, direction := range []string{"source", "target"} {
			if truncated {
				break
			}
			query, args := linkedProjectionQuery(direction, chunk, accessibleWorkspaceIDs)
			rows, err := r.db.QueryContext(ctx, query, args...)
			if err != nil {
				return nil, false, fmt.Errorf("project linked items: %w", err)
			}
			err = r.collectLinkedItems(rows, fields, customFieldIDs, projected, &truncated)
			_ = rows.Close()
			if err != nil {
				return nil, false, err
			}
		}
	}
	return projected, truncated, nil
}

func (r *ItemLinkRepository) collectLinkedItems(
	rows *sql.Rows,
	fields []string,
	customFieldIDs map[string]string,
	projected map[int][]map[string]any,
	truncated *bool,
) error {
	total := 0
	for _, links := range projected {
		total += len(links)
	}

	for rows.Next() {
		var (
			assetID       int
			itemID        int
			title         string
			workspaceID   int
			workspaceKey  string
			workspaceName string
			itemNumber    int
			statusName    string
			creatorName   string
			customFields  string
		)
		if err := rows.Scan(&assetID, &itemID, &title, &workspaceID, &workspaceKey, &workspaceName, &itemNumber, &statusName, &creatorName, &customFields); err != nil {
			return fmt.Errorf("scan linked item: %w", err)
		}

		if len(projected[assetID]) >= MaxProjectedLinksPerAsset || total >= MaxProjectedLinksTotal {
			*truncated = true
			continue
		}

		row := map[string]any{"id": itemID}
		values := linkedItemValues{
			id:           itemID,
			key:          fmt.Sprintf("%s-%d", workspaceKey, itemNumber),
			title:        title,
			status:       statusName,
			workspace:    workspaceName,
			creator:      creatorName,
			customFields: parseCustomFieldValues(customFields),
		}
		for _, field := range fields {
			if name, ok := strings.CutPrefix(field, "cf_"); ok {
				row[field] = values.customField(customFieldIDs[name])
				continue
			}
			if value, ok := values.scalar(field); ok {
				row[field] = value
			}
		}

		projected[assetID] = append(projected[assetID], row)
		total++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate linked items: %w", err)
	}
	return nil
}

// linkedProjectionQuery builds one direction of the asset-to-item join. The
// item side is always the "other" endpoint of the link, and the workspace
// filter is what keeps the projection inside the viewer's permissions.
func linkedProjectionQuery(direction string, assetIDs, workspaceIDs []int) (query string, args []any) {
	assetColumn, itemJoin := "il.source_id", "il.target_id"
	assetPredicate := "il.source_type = 'asset' AND il.target_type = 'item'"
	if direction == "target" {
		assetColumn, itemJoin = "il.target_id", "il.source_id"
		assetPredicate = "il.target_type = 'asset' AND il.source_type = 'item'"
	}

	query = `
		SELECT ` + assetColumn + `, i.id, i.title, i.workspace_id,
		       COALESCE(w.key, ''), COALESCE(w.name, ''), i.workspace_item_number,
		       COALESCE(s.name, ''),
		       COALESCE(NULLIF(TRIM(COALESCE(cu.first_name, '') || ' ' || COALESCE(cu.last_name, '')), ''), ''),
		       COALESCE(i.custom_field_values, '')
		FROM item_links il
		JOIN items i ON i.id = ` + itemJoin + `
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN users cu ON i.creator_id = cu.id
		WHERE ` + assetPredicate + `
		  AND ` + assetColumn + ` IN (` + placeholders(len(assetIDs)) + `)
		  AND i.workspace_id IN (` + placeholders(len(workspaceIDs)) + `)
		ORDER BY il.id`

	args = make([]any, 0, len(assetIDs)+len(workspaceIDs))
	for _, id := range assetIDs {
		args = append(args, id)
	}
	for _, id := range workspaceIDs {
		args = append(args, id)
	}
	return query, args
}

// customFieldIDsForFields maps the cf_<name> fields in a projection to their
// custom field IDs, because custom_field_values is keyed by ID.
func (r *ItemLinkRepository) customFieldIDsForFields(ctx context.Context, fields []string) (map[string]string, error) {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if name, ok := strings.CutPrefix(field, "cf_"); ok {
			names = append(names, strings.ToLower(name))
		}
	}
	if len(names) == 0 {
		return map[string]string{}, nil
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id, LOWER(name) FROM custom_field_definitions`)
	if err != nil {
		return nil, fmt.Errorf("load custom field definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	ids := make(map[string]string, len(names))
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan custom field definition: %w", err)
		}
		if wanted[name] {
			ids[name] = fmt.Sprintf("%d", id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom field definitions: %w", err)
	}
	return ids, nil
}

type linkedItemValues struct {
	id           int
	key          string
	title        string
	status       string
	workspace    string
	creator      string
	customFields map[string]json.RawMessage
}

func (v linkedItemValues) scalar(field string) (any, bool) {
	switch field {
	case "id":
		return v.id, true
	case "key":
		return v.key, true
	case "title":
		return v.title, true
	case "status":
		return v.status, true
	case "workspace":
		return v.workspace, true
	case "creator":
		return v.creator, true
	default:
		return nil, false
	}
}

// customField renders a stored custom field value as a display string.
// Reference-kind fields store {id, title} objects or arrays of them, so the
// label is preferred over the raw ID.
func (v linkedItemValues) customField(fieldID string) any {
	if fieldID == "" {
		return nil
	}
	raw, ok := v.customFields[fieldID]
	if !ok {
		return nil
	}
	return renderCustomFieldValue(raw)
}

func parseCustomFieldValues(encoded string) map[string]json.RawMessage {
	if strings.TrimSpace(encoded) == "" {
		return nil
	}
	values := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(encoded), &values); err != nil {
		return nil
	}
	return values
}

func renderCustomFieldValue(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil
		}
		labels := make([]string, 0, len(entries))
		for _, entry := range entries {
			if label := referenceLabel(entry); label != "" {
				labels = append(labels, label)
			}
		}
		return strings.Join(labels, ", ")
	}
	if strings.HasPrefix(trimmed, "{") {
		return referenceLabel(raw)
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func referenceLabel(raw json.RawMessage) string {
	var object struct {
		Title string `json:"title"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(raw, &object); err == nil {
		if object.Title != "" {
			return object.Title
		}
		if object.Name != "" {
			return object.Name
		}
	}
	var scalar any
	if err := json.Unmarshal(raw, &scalar); err != nil {
		return ""
	}
	if text, ok := scalar.(string); ok {
		return text
	}
	return ""
}
