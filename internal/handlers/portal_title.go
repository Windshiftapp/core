package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"windshift/internal/models"
	tmpl "windshift/internal/services/template"
)

// renderPortalTitle returns the item title for a portal submission whose
// request type does not include the title field on the form. The request
// type's TitleTemplate is rendered using the {{var}} placeholder syntax
// shared with workspace actions.
//
// Supported variables:
//   - {{type.name}}                request type name
//   - {{type.id}}                  request type id
//   - {{requester.name}}           submitter's display name (best-effort)
//   - {{requester.email}}          submitter's email
//   - {{description}}              submitted description, truncated to 120 chars
//   - {{custom.<field_name>}}      submitted custom field, keyed by the
//     custom_field_definitions.name slug
//
// Returns the rendered, trimmed title. Empty result means the template
// rendered to whitespace (or was empty); callers reject that.
func (h *PortalHandler) renderPortalTitle(ctx context.Context, rt *models.RequestType, description string, customFields map[string]interface{}, userID, customerID *int) string {
	if rt == nil || strings.TrimSpace(rt.TitleTemplate) == "" {
		return ""
	}

	vars := map[string]string{
		"type.name":       rt.Name,
		"type.id":         strconv.Itoa(rt.ID),
		"description":     truncateRunes(description, 120),
		"requester.name":  "",
		"requester.email": "",
	}

	switch {
	case userID != nil:
		var firstName, lastName, email string
		if err := h.db.QueryRowContext(ctx,
			`SELECT first_name, last_name, email FROM users WHERE id = ?`,
			*userID,
		).Scan(&firstName, &lastName, &email); err == nil {
			name := strings.TrimSpace(firstName + " " + lastName)
			vars["requester.name"] = name
			vars["requester.email"] = email
		}
	case customerID != nil:
		var name, email string
		if err := h.db.QueryRowContext(ctx,
			`SELECT name, email FROM portal_customers WHERE id = ?`,
			*customerID,
		).Scan(&name, &email); err == nil {
			vars["requester.name"] = name
			vars["requester.email"] = email
		}
	}

	for name, value := range resolveCustomFieldNames(ctx, h, customFields) {
		vars["custom."+name] = value
	}

	return strings.TrimSpace(tmpl.Substitute(rt.TitleTemplate, vars))
}

// resolveCustomFieldNames maps each numeric custom-field-id key in
// customFields to its custom_field_definitions.name, returning a
// {name → string-value} map ready to fold into the template var map.
// Non-numeric keys (virtual fields, malformed input) are skipped.
func resolveCustomFieldNames(ctx context.Context, h *PortalHandler, customFields map[string]interface{}) map[string]string {
	if len(customFields) == 0 {
		return nil
	}

	var ids []interface{}
	keyToValue := map[string]interface{}{}
	for k, v := range customFields {
		if _, err := strconv.Atoi(k); err == nil {
			ids = append(ids, k)
			keyToValue[k] = v
		}
	}
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	rows, err := h.db.QueryContext(ctx,
		"SELECT id, name FROM custom_field_definitions WHERE id IN ("+placeholders+")",
		ids...,
	)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		idStr := strconv.Itoa(id)
		if v, ok := keyToValue[idStr]; ok && name != "" {
			out[name] = formatTemplateValue(v)
		}
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return out
}

func formatTemplateValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers always decode as float64 in encoding/json. Render
		// integer-valued floats without a trailing .000000 to keep titles
		// looking sane.
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
