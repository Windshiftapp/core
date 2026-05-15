package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
)

// requestTypeValidationResult contains the result of request type field validation
type requestTypeValidationResult struct {
	itemTypeID         *int
	virtualFieldValues map[string]interface{}
	customFieldValues  map[string]interface{}
	// titleFieldInForm is true when the request type's field config includes
	// the default "title" field — meaning the submitter saw a title input on
	// the form. Callers that need a title (every item create) use this to
	// decide between trusting submission.Title vs. rendering a title template.
	titleFieldInForm bool
}

// validateAndSeparateFields validates request type fields and separates virtual from custom fields.
// The component parameter is used for log attribution (e.g. "forms", "portal").
func validateAndSeparateFields(ctx context.Context, db database.Database, requestTypeID *int, title, description string, customFields map[string]interface{}) (*requestTypeValidationResult, error) {
	result := &requestTypeValidationResult{}

	if requestTypeID == nil {
		if title == "" {
			return nil, fmt.Errorf("title is required")
		}
		return result, nil
	}

	var rtID int
	var rtName string
	var itemTypeID int
	err := db.QueryRowContext(ctx, `SELECT id, name, item_type_id FROM request_types WHERE id = ? AND is_active = true`, *requestTypeID).Scan(
		&rtID, &rtName, &itemTypeID,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid request type (ID: %d): %w", *requestTypeID, err)
	}
	result.itemTypeID = &itemTypeID

	virtualFieldIDs := make(map[string]bool)
	rows, err := db.QueryContext(ctx, `SELECT field_identifier, field_type, is_required FROM request_type_fields WHERE request_type_id = ? ORDER BY display_order`, *requestTypeID)
	if err != nil {
		return nil, fmt.Errorf("failed to load request type fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var fieldID, fieldType string
		var isRequired bool
		if err := rows.Scan(&fieldID, &fieldType, &isRequired); err != nil {
			continue
		}

		if fieldType == "virtual" {
			virtualFieldIDs[fieldID] = true
		}

		// Title is always required when shown on the form, regardless of the
		// admin-set is_required flag — items.title is NOT NULL and the
		// portal's title-template fallback only applies when the field is
		// hidden entirely.
		if fieldType == "default" && fieldID == "title" {
			result.titleFieldInForm = true
			if title == "" {
				return nil, fmt.Errorf("title is required")
			}
		}

		if isRequired {
			switch fieldType {
			case "default":
				if fieldID == "description" && description == "" {
					return nil, fmt.Errorf("description is required")
				}
			case "custom", "virtual":
				if customFields == nil || customFields[fieldID] == nil || customFields[fieldID] == "" {
					return nil, fmt.Errorf("field %s is required", fieldID)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate request type fields: %w", err)
	}

	if len(virtualFieldIDs) > 0 && customFields != nil {
		result.virtualFieldValues = make(map[string]interface{})
		result.customFieldValues = make(map[string]interface{})

		for fieldID, value := range customFields {
			if virtualFieldIDs[fieldID] {
				result.virtualFieldValues[fieldID] = value
			} else {
				result.customFieldValues[fieldID] = value
			}
		}
	} else {
		result.customFieldValues = customFields
	}

	return result, nil
}

// storeCustomFieldValues stores custom field values for an item.
// The component parameter is used for log attribution (e.g. "forms", "portal").
func storeCustomFieldValues(ctx context.Context, db database.Database, component string, itemID int64, customFields map[string]interface{}) {
	if len(customFields) == 0 {
		return
	}

	now := time.Now()
	for fieldIDStr, value := range customFields {
		if value == nil || value == "" {
			continue
		}

		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		case float64:
			valueStr = fmt.Sprintf("%v", v)
		case bool:
			valueStr = fmt.Sprintf("%v", v)
		default:
			valueBytes, err := json.Marshal(v)
			if err == nil {
				valueStr = string(valueBytes)
			}
		}

		if valueStr != "" {
			if _, err := db.ExecWriteContext(ctx, `
				INSERT INTO custom_field_values (item_id, custom_field_id, value, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(item_id, custom_field_id) DO UPDATE SET value = ?, updated_at = ?
			`, itemID, fieldIDStr, valueStr, now, now, valueStr, now); err != nil {
				slog.Warn("failed to save custom field value", slog.String("component", component), slog.Int64("item_id", itemID), slog.String("field_id", fieldIDStr), slog.Any("error", err))
			}
		}
	}

	customFieldsJSON, err := json.Marshal(customFields)
	if err == nil {
		if _, err := db.ExecWriteContext(ctx, `UPDATE items SET custom_field_values = ? WHERE id = ?`, string(customFieldsJSON), itemID); err != nil {
			slog.Warn("failed to update item custom_field_values", slog.String("component", component), slog.Int64("item_id", itemID), slog.Any("error", err))
		}
	}
}

// storeVirtualFieldValues stores virtual field values for an item.
// The component parameter is used for log attribution (e.g. "forms", "portal").
func storeVirtualFieldValues(ctx context.Context, db database.Database, component string, itemID int64, virtualFields map[string]interface{}) {
	if len(virtualFields) == 0 {
		return
	}

	virtualFieldsJSON, err := json.Marshal(virtualFields)
	if err != nil {
		slog.Warn("failed to marshal virtual field values", slog.String("component", component), slog.Int64("item_id", itemID), slog.Any("error", err))
		return
	}

	if _, err := db.ExecWriteContext(ctx, `UPDATE items SET virtual_field_data = ? WHERE id = ?`, string(virtualFieldsJSON), itemID); err != nil {
		slog.Warn("failed to update item virtual_field_data", slog.String("component", component), slog.Int64("item_id", itemID), slog.Any("error", err))
	}
}
