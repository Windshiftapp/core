package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

func (h *ConfigurationSetHandler) ExecuteMigration(w http.ResponseWriter, r *http.Request) {
	var migrationReq models.WorkflowMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&migrationReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate configuration set exists
	var configSetExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", migrationReq.ConfigurationSetID).Scan(&configSetExists)
	if err != nil || !configSetExists {
		http.Error(w, "Configuration set not found", http.StatusBadRequest)
		return
	}

	// Validate workspace IDs provided
	if len(migrationReq.WorkspaceIDs) == 0 {
		http.Error(w, "At least one workspace ID is required", http.StatusBadRequest)
		return
	}

	// Validate all target status IDs exist
	for _, mapping := range migrationReq.StatusMappings {
		var statusExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", mapping.ToStatusID).Scan(&statusExists)
		if err != nil || !statusExists {
			http.Error(w, fmt.Sprintf("Target status ID %d not found", mapping.ToStatusID), http.StatusBadRequest)
			return
		}
	}

	// Start transaction for atomic migration
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	totalMigrated := 0

	// Execute status migrations
	for _, mapping := range migrationReq.StatusMappings {
		var updateQuery string
		var updateArgs []interface{}

		// Build query with optional item_type_id filter
		if mapping.ItemTypeID != nil {
			// Update only items of specific type
			updateQuery = `
				UPDATE items
				SET status_id = ?, updated_at = ?
				WHERE status_id = ?
				AND item_type_id = ?
				AND workspace_id IN (?` + strings.Repeat(",?", len(migrationReq.WorkspaceIDs)-1) + `)`

			updateArgs = []interface{}{mapping.ToStatusID, time.Now(), mapping.FromStatusID, *mapping.ItemTypeID}
		} else {
			// Update all items regardless of type (original behavior)
			updateQuery = `
				UPDATE items
				SET status_id = ?, updated_at = ?
				WHERE status_id = ?
				AND workspace_id IN (?` + strings.Repeat(",?", len(migrationReq.WorkspaceIDs)-1) + `)`

			updateArgs = []interface{}{mapping.ToStatusID, time.Now(), mapping.FromStatusID}
		}

		for _, wsID := range migrationReq.WorkspaceIDs {
			updateArgs = append(updateArgs, wsID)
		}

		result, err := tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		rowsAffected, _ := result.RowsAffected()
		totalMigrated += int(rowsAffected)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":        true,
		"message":        fmt.Sprintf("Successfully migrated %d items", totalMigrated),
		"migrated_items": totalMigrated,
	}

	respondJSONOK(w, response)
}

// ExecuteComprehensiveMigration executes all migration dimensions in a single transaction
func (h *ConfigurationSetHandler) ExecuteComprehensiveMigration(w http.ResponseWriter, r *http.Request) {
	var req models.ComprehensiveMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate configuration sets exist
	var oldConfigSetExists, newConfigSetExists bool
	h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", req.OldConfigurationSetID).Scan(&oldConfigSetExists)
	h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", req.NewConfigurationSetID).Scan(&newConfigSetExists)

	if !oldConfigSetExists {
		http.Error(w, "Old configuration set not found", http.StatusBadRequest)
		return
	}
	if !newConfigSetExists {
		http.Error(w, "New configuration set not found", http.StatusBadRequest)
		return
	}

	// Validate workspace IDs provided
	if len(req.WorkspaceIDs) == 0 {
		http.Error(w, "At least one workspace ID is required", http.StatusBadRequest)
		return
	}

	// Validate all target IDs exist
	for _, mapping := range req.ItemTypeMappings {
		var exists bool
		h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", mapping.ToItemTypeID).Scan(&exists)
		if !exists {
			http.Error(w, fmt.Sprintf("Target item type ID %d not found", mapping.ToItemTypeID), http.StatusBadRequest)
			return
		}
	}

	for _, mapping := range req.StatusMappings {
		var exists bool
		h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", mapping.ToStatusID).Scan(&exists)
		if !exists {
			http.Error(w, fmt.Sprintf("Target status ID %d not found", mapping.ToStatusID), http.StatusBadRequest)
			return
		}
	}

	for _, mapping := range req.PriorityMappings {
		var exists bool
		h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM priorities WHERE id = ?)", mapping.ToPriorityID).Scan(&exists)
		if !exists {
			http.Error(w, fmt.Sprintf("Target priority ID %d not found", mapping.ToPriorityID), http.StatusBadRequest)
			return
		}
	}

	// Start transaction for atomic migration
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	now := time.Now()
	stats := struct {
		ItemTypesMigrated   int `json:"item_types_migrated"`
		StatusesMigrated    int `json:"statuses_migrated"`
		PrioritiesMigrated  int `json:"priorities_migrated"`
		CustomFieldsUpdated int `json:"custom_fields_updated"`
	}{}

	// Build workspace placeholders
	wsPlaceholders := "?" + strings.Repeat(",?", len(req.WorkspaceIDs)-1)

	// 1. Execute Item Type Migrations
	for _, mapping := range req.ItemTypeMappings {
		var updateQuery string
		var updateArgs []interface{}

		if mapping.FromItemTypeID == nil {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET item_type_id = ?, updated_at = ?
				WHERE item_type_id IS NULL
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToItemTypeID, now}
		} else {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET item_type_id = ?, updated_at = ?
				WHERE item_type_id = ?
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToItemTypeID, now, *mapping.FromItemTypeID}
		}

		for _, wsID := range req.WorkspaceIDs {
			updateArgs = append(updateArgs, wsID)
		}

		result, err := tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			http.Error(w, "Failed to migrate item types: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, _ := result.RowsAffected()
		stats.ItemTypesMigrated += int(rowsAffected)
	}

	// 2. Execute Custom Field Migrations (only add_default needs action)
	for _, mapping := range req.CustomFieldMappings {
		if mapping.Action == "add_default" && mapping.DefaultValue != nil {
			count, err := h.addDefaultFieldValue(tx, req.WorkspaceIDs, mapping.FieldID, mapping.DefaultValue)
			if err != nil {
				http.Error(w, "Failed to add default field values: "+err.Error(), http.StatusInternalServerError)
				return
			}
			stats.CustomFieldsUpdated += count
		}
		// "keep" and "orphan" require no action - data is preserved in JSON
	}

	// 3. Execute Status Migrations
	for _, mapping := range req.StatusMappings {
		var updateQuery string
		var updateArgs []interface{}

		if mapping.ItemTypeID != nil {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET status_id = ?, updated_at = ?
				WHERE status_id = ?
				AND item_type_id = ?
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToStatusID, now, mapping.FromStatusID, *mapping.ItemTypeID}
		} else {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET status_id = ?, updated_at = ?
				WHERE status_id = ?
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToStatusID, now, mapping.FromStatusID}
		}

		for _, wsID := range req.WorkspaceIDs {
			updateArgs = append(updateArgs, wsID)
		}

		result, err := tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			http.Error(w, "Failed to migrate statuses: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, _ := result.RowsAffected()
		stats.StatusesMigrated += int(rowsAffected)
	}

	// 4. Execute Priority Migrations
	for _, mapping := range req.PriorityMappings {
		var updateQuery string
		var updateArgs []interface{}

		if mapping.FromPriorityID == nil {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET priority_id = ?, updated_at = ?
				WHERE priority_id IS NULL
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToPriorityID, now}
		} else {
			updateQuery = fmt.Sprintf(`
				UPDATE items SET priority_id = ?, updated_at = ?
				WHERE priority_id = ?
				AND workspace_id IN (%s)`, wsPlaceholders)
			updateArgs = []interface{}{mapping.ToPriorityID, now, *mapping.FromPriorityID}
		}

		for _, wsID := range req.WorkspaceIDs {
			updateArgs = append(updateArgs, wsID)
		}

		result, err := tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			http.Error(w, "Failed to migrate priorities: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, _ := result.RowsAffected()
		stats.PrioritiesMigrated += int(rowsAffected)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalMigrated := stats.ItemTypesMigrated + stats.StatusesMigrated + stats.PrioritiesMigrated

	response := map[string]interface{}{
		"success":        true,
		"message":        fmt.Sprintf("Successfully migrated %d items", totalMigrated),
		"migrated_items": totalMigrated,
		"details":        stats,
	}

	respondJSONOK(w, response)
}

// addDefaultFieldValue adds a default value for a custom field to items that don't have it
func (h *ConfigurationSetHandler) addDefaultFieldValue(tx database.Tx, workspaceIDs []int, fieldID int, defaultValue interface{}) (int, error) {
	fieldKey := strconv.Itoa(fieldID)
	count := 0

	// Build workspace placeholders
	wsPlaceholders := "?" + strings.Repeat(",?", len(workspaceIDs)-1)
	wsArgs := make([]interface{}, len(workspaceIDs))
	for i, wsID := range workspaceIDs {
		wsArgs[i] = wsID
	}

	// Get all items in the workspaces
	query := fmt.Sprintf(`
		SELECT id, COALESCE(custom_field_values, '{}') as cfv
		FROM items
		WHERE workspace_id IN (%s)
	`, wsPlaceholders)

	rows, err := tx.Query(query, wsArgs...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type itemUpdate struct {
		id     int
		newCFV string
	}
	var updates []itemUpdate

	for rows.Next() {
		var id int
		var cfvJSON string
		if err := rows.Scan(&id, &cfvJSON); err != nil {
			return 0, err
		}

		var cfv map[string]interface{}
		if err := json.Unmarshal([]byte(cfvJSON), &cfv); err != nil {
			cfv = make(map[string]interface{})
		}

		// Only add if field not already set
		if _, exists := cfv[fieldKey]; !exists {
			cfv[fieldKey] = defaultValue
			newJSON, err := json.Marshal(cfv)
			if err != nil {
				return 0, err
			}
			updates = append(updates, itemUpdate{id: id, newCFV: string(newJSON)})
		}
	}

	// Apply updates
	now := time.Now()
	for _, update := range updates {
		_, err := tx.Exec(`UPDATE items SET custom_field_values = ?, updated_at = ? WHERE id = ?`,
			update.newCFV, now, update.id)
		if err != nil {
			return 0, err
		}
		count++
	}

	return count, nil
}
