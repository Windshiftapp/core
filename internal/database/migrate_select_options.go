package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"windshift/internal/models"
)

// MigrateSelectFieldOptions migrates legacy string-array options to ID-based format.
// This is safe to re-run (idempotent).
// It takes a Database interface so it works for both SQLite and Postgres.
// last review: ser, 210426, NOTE: will be dropped after 0.7
func MigrateSelectFieldOptions(db Database) error {
	// Step 1: Migrate field definitions and build lookup maps
	fieldLabelToID, err := migrateFieldDefinitions(db)
	if err != nil {
		return fmt.Errorf("failed to migrate field definitions: %w", err)
	}

	if len(fieldLabelToID) == 0 {
		// No fields needed migration
		return nil
	}

	// Step 2: Migrate item custom_field_values
	if err := migrateCustomFieldValues(db, "items", fieldLabelToID); err != nil {
		return fmt.Errorf("failed to migrate items custom field values: %w", err)
	}

	// Step 3: Migrate asset custom_field_values
	if err := migrateCustomFieldValues(db, "assets", fieldLabelToID); err != nil {
		return fmt.Errorf("failed to migrate assets custom field values: %w", err)
	}

	// Step 4: Migrate portal custom_field_values table
	if err := migratePortalCustomFieldValues(db, fieldLabelToID); err != nil {
		return fmt.Errorf("failed to migrate portal custom field values: %w", err)
	}

	return nil
}

// selectFieldInfo holds the info needed to map labels to IDs for a select/multiselect field
type selectFieldInfo struct {
	fieldType string
	labelToID map[string]int
}

// migrateFieldDefinitions converts legacy []string options to the new ID-based format.
// Returns a map of fieldID -> selectFieldInfo for use in migrating item values.
func migrateFieldDefinitions(db Database) (map[int]selectFieldInfo, error) {
	rows, err := db.Query(`SELECT id, field_type, options FROM custom_field_definitions WHERE field_type IN ('select', 'multiselect') AND options IS NOT NULL AND options != ''`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int]selectFieldInfo)
	type fieldRow struct {
		id        int
		fieldType string
		options   string
	}
	var fieldsToMigrate []fieldRow

	for rows.Next() {
		var id int
		var fieldType string
		var optionsStr sql.NullString
		if err := rows.Scan(&id, &fieldType, &optionsStr); err != nil {
			return nil, err
		}
		if !optionsStr.Valid || optionsStr.String == "" {
			continue
		}

		optionsJSON := strings.TrimSpace(optionsStr.String)

		// Already in new format? Build the map but don't update.
		if strings.HasPrefix(optionsJSON, "{") {
			opts, parseErr := models.ParseSelectOptions(optionsJSON)
			if parseErr == nil && opts.NextID > 0 {
				info := selectFieldInfo{fieldType: fieldType, labelToID: make(map[string]int)}
				for _, item := range opts.Items {
					info.labelToID[item.Label] = item.ID
				}
				result[id] = info
				continue
			}
		}

		// Legacy format: needs migration
		if strings.HasPrefix(optionsJSON, "[") {
			fieldsToMigrate = append(fieldsToMigrate, fieldRow{id: id, fieldType: fieldType, options: optionsJSON})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate custom_field_definitions: %w", err)
	}

	// Migrate legacy fields
	for _, f := range fieldsToMigrate {
		opts, parseErr := models.ParseSelectOptions(f.options)
		if parseErr != nil {
			slog.Warn("skipping field with unparseable options", "field_id", f.id, "error", parseErr)
			continue
		}

		normalized, serErr := models.SerializeSelectOptions(opts)
		if serErr != nil {
			slog.Warn("skipping field with serialization error", "field_id", f.id, "error", serErr)
			continue
		}

		if _, err := db.ExecWrite(`UPDATE custom_field_definitions SET options = ? WHERE id = ?`, normalized, f.id); err != nil {
			return nil, fmt.Errorf("failed to update field %d: %w", f.id, err)
		}

		info := selectFieldInfo{fieldType: f.fieldType, labelToID: make(map[string]int)}
		for _, item := range opts.Items {
			info.labelToID[item.Label] = item.ID
		}
		result[f.id] = info

		slog.Info("migrated select field options to ID format", "field_id", f.id, "options_count", len(opts.Items))
	}

	return result, nil
}

// migrateCustomFieldValues migrates custom_field_values in items or assets table
func migrateCustomFieldValues(db Database, tableName string, fieldMap map[int]selectFieldInfo) error {
	rows, err := db.Query(fmt.Sprintf(`SELECT id, custom_field_values FROM %s WHERE custom_field_values IS NOT NULL AND custom_field_values != '' AND custom_field_values != '{}'`, tableName))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type rowToUpdate struct {
		id     int
		newVal string
	}
	var updates []rowToUpdate

	for rows.Next() {
		var id int
		var cfvStr string
		if err := rows.Scan(&id, &cfvStr); err != nil {
			return err
		}

		var cfv map[string]interface{}
		if err := json.Unmarshal([]byte(cfvStr), &cfv); err != nil {
			continue
		}

		changed := false
		for fieldKey, val := range cfv {
			fieldID, err := strconv.Atoi(fieldKey)
			if err != nil {
				continue
			}
			info, ok := fieldMap[fieldID]
			if !ok {
				continue
			}

			newVal := convertFieldValue(val, info)
			if newVal != nil {
				cfv[fieldKey] = newVal
				changed = true
			}
		}

		if changed {
			b, err := json.Marshal(cfv)
			if err != nil {
				continue
			}
			updates = append(updates, rowToUpdate{id: id, newVal: string(b)})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate %s: %w", tableName, err)
	}

	for _, u := range updates {
		if _, err := db.ExecWrite(fmt.Sprintf(`UPDATE %s SET custom_field_values = ? WHERE id = ?`, tableName), u.newVal, u.id); err != nil {
			slog.Warn("failed to update custom field values", "table", tableName, "id", u.id, "error", err)
		}
	}

	if len(updates) > 0 {
		slog.Info("migrated custom field values", "table", tableName, "rows_updated", len(updates))
	}

	return nil
}

// migratePortalCustomFieldValues migrates the portal custom_field_values key-value table
func migratePortalCustomFieldValues(db Database, fieldMap map[int]selectFieldInfo) error {
	// Check if the table exists by attempting a count query
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM custom_field_values`).Scan(&count); err != nil {
		// Table doesn't exist - skip
		return nil
	}
	if count == 0 {
		return nil
	}

	rows, err := db.Query(`SELECT id, custom_field_id, value FROM custom_field_values WHERE value IS NOT NULL AND value != ''`)
	if err != nil {
		return nil // Table might not have these columns
	}
	defer func() { _ = rows.Close() }()

	type rowToUpdate struct {
		id     int
		newVal string
	}
	var updates []rowToUpdate

	for rows.Next() {
		var id, cfID int
		var val string
		if err := rows.Scan(&id, &cfID, &val); err != nil {
			continue
		}

		info, ok := fieldMap[cfID]
		if !ok {
			continue
		}

		// Check if value is already numeric
		if _, err := strconv.Atoi(val); err == nil {
			continue // Already migrated
		}

		// Single select: map label to ID
		switch info.fieldType {
		case "select":
			if optID, ok := info.labelToID[val]; ok {
				updates = append(updates, rowToUpdate{id: id, newVal: strconv.Itoa(optID)})
			}
		case "multiselect":
			// Could be comma-separated or JSON array
			var labels []string
			if err := json.Unmarshal([]byte(val), &labels); err != nil {
				labels = strings.Split(val, ",")
				for i := range labels {
					labels[i] = strings.TrimSpace(labels[i])
				}
			}
			var ids []int
			for _, label := range labels {
				if optID, ok := info.labelToID[label]; ok {
					ids = append(ids, optID)
				}
			}
			if len(ids) > 0 {
				b, _ := json.Marshal(ids)
				updates = append(updates, rowToUpdate{id: id, newVal: string(b)})
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate custom_field_values: %w", err)
	}

	for _, u := range updates {
		if _, err := db.ExecWrite(`UPDATE custom_field_values SET value = ? WHERE id = ?`, u.newVal, u.id); err != nil {
			slog.Warn("failed to update portal custom field value", "id", u.id, "error", err)
		}
	}

	if len(updates) > 0 {
		slog.Info("migrated portal custom field values", "rows_updated", len(updates))
	}

	return nil
}

// convertFieldValue converts a single field value from legacy string to numeric ID.
// Returns nil if no conversion is needed.
func convertFieldValue(val interface{}, info selectFieldInfo) interface{} {
	switch v := val.(type) {
	case float64:
		// Already numeric (JSON numbers decode as float64)
		return nil
	case json.Number:
		return nil
	case string:
		// Check if already numeric
		if _, err := strconv.Atoi(v); err == nil {
			return nil
		}

		switch info.fieldType {
		case "select":
			if optID, ok := info.labelToID[v]; ok {
				return optID
			}
			// Could be comma-separated multiselect stored in a select field
			if strings.Contains(v, ",") {
				parts := strings.Split(v, ",")
				var ids []int
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if optID, ok := info.labelToID[part]; ok {
						ids = append(ids, optID)
					}
				}
				if len(ids) == 1 {
					return ids[0]
				}
				if len(ids) > 0 {
					return ids
				}
			}
		case "multiselect":
			parts := strings.Split(v, ",")
			var ids []int
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if optID, ok := info.labelToID[part]; ok {
					ids = append(ids, optID)
				}
			}
			if len(ids) > 0 {
				return ids
			}
		}
	case []interface{}:
		// Array of strings (legacy multiselect)
		var ids []int
		allNumeric := true
		for _, item := range v {
			switch itemVal := item.(type) {
			case float64:
				ids = append(ids, int(itemVal))
			case string:
				if n, err := strconv.Atoi(itemVal); err == nil {
					ids = append(ids, n)
				} else {
					allNumeric = false
					if optID, ok := info.labelToID[itemVal]; ok {
						ids = append(ids, optID)
					}
				}
			}
		}
		if allNumeric {
			return nil // Already all numeric
		}
		if len(ids) > 0 {
			return ids
		}
	}
	return nil
}
