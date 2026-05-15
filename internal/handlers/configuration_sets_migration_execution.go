package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// errMigrationConflict signals that a compare-and-swap on
// workspace_configuration_sets failed because another caller changed the
// workspace's assignment between analysis and execution. Surface as 409.
var errMigrationConflict = errors.New("migration conflict")

func (h *ConfigurationSetHandler) ExecuteMigration(w http.ResponseWriter, r *http.Request) {
	migrationReq, ok := decodeJSON[models.WorkflowMigrationRequest](w, r)
	if !ok {
		return
	}

	// Validate configuration set exists and load its workflow_id for membership checks.
	var configSetExists bool
	var targetWorkflowID sql.NullInt64
	err := h.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?),
		       (SELECT workflow_id FROM configuration_sets WHERE id = ?)
	`, migrationReq.ConfigurationSetID, migrationReq.ConfigurationSetID).Scan(&configSetExists, &targetWorkflowID)
	if err != nil || !configSetExists {
		respondBadRequest(w, r, "Configuration set not found")
		return
	}

	// Validate workspace IDs provided
	if len(migrationReq.WorkspaceIDs) == 0 {
		respondValidationError(w, r, "At least one workspace ID is required")
		return
	}

	// Validate target status IDs are members of the target workflow.
	if !targetWorkflowID.Valid && len(migrationReq.StatusMappings) > 0 {
		respondBadRequest(w, r, "Target configuration set has no workflow; cannot accept status mappings")
		return
	}
	for _, mapping := range migrationReq.StatusMappings {
		if !h.statusIsInWorkflow(mapping.ToStatusID, int(targetWorkflowID.Int64)) {
			respondBadRequest(w, r, fmt.Sprintf("Target status ID %d is not part of the target workflow", mapping.ToStatusID))
			return
		}
	}

	// Start transaction for atomic migration
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	totalMigrated := 0
	for _, mapping := range migrationReq.StatusMappings {
		rows, err := h.applyStatusMapping(tx, mapping, migrationReq.WorkspaceIDs)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		totalMigrated += rows
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
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
	req, ok := decodeJSON[models.ComprehensiveMigrationRequest](w, r)
	if !ok {
		return
	}

	// Validate configuration sets exist. OldConfigurationSetID == 0 is allowed
	// and means "the workspace had no configuration set assigned".
	var oldConfigSetExists, newConfigSetExists bool
	if req.OldConfigurationSetID != 0 {
		if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", req.OldConfigurationSetID).Scan(&oldConfigSetExists); err != nil {
			slog.Error("migration validation failed", slog.Any("error", err), slog.String("check", "old_configuration_set"))
			respondInternalError(w, r, fmt.Errorf("failed to validate old configuration set: %w", err))
			return
		}
		if !oldConfigSetExists {
			respondBadRequest(w, r, "Old configuration set not found")
			return
		}
	}
	var targetWorkflowID sql.NullInt64
	if err := h.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?),
		       (SELECT workflow_id FROM configuration_sets WHERE id = ?)
	`, req.NewConfigurationSetID, req.NewConfigurationSetID).Scan(&newConfigSetExists, &targetWorkflowID); err != nil {
		slog.Error("migration validation failed", slog.Any("error", err), slog.String("check", "new_configuration_set"))
		respondInternalError(w, r, fmt.Errorf("failed to validate new configuration set: %w", err))
		return
	}
	if !newConfigSetExists {
		respondBadRequest(w, r, "New configuration set not found")
		return
	}

	// Validate workspace IDs provided
	if len(req.WorkspaceIDs) == 0 {
		respondValidationError(w, r, "At least one workspace ID is required")
		return
	}

	// Target item types: must be members of the target config set's allow-list,
	// or any item_type if the target has no explicit list ("accept all").
	allowedItemTypes, err := h.allowedItemTypesForConfigSet(req.NewConfigurationSetID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load target item types: %w", err))
		return
	}
	for _, mapping := range req.ItemTypeMappings {
		if !inIntSet(allowedItemTypes, mapping.ToItemTypeID) {
			respondBadRequest(w, r, fmt.Sprintf("Target item type ID %d is not allowed by the target configuration set", mapping.ToItemTypeID))
			return
		}
	}

	// Target priorities: same pattern.
	allowedPriorities, err := h.allowedPrioritiesForConfigSet(req.NewConfigurationSetID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load target priorities: %w", err))
		return
	}
	for _, mapping := range req.PriorityMappings {
		if !inIntSet(allowedPriorities, mapping.ToPriorityID) {
			respondBadRequest(w, r, fmt.Sprintf("Target priority ID %d is not allowed by the target configuration set", mapping.ToPriorityID))
			return
		}
	}

	// Target statuses: must be members of the target workflow. When the caller
	// supplies ApplyWorkflowToConfigSet, validate against *that* workflow
	// instead of the workflow currently persisted on the config set — this is
	// the intra-set workflow-change path, where the new workflow_id will be
	// written inside this same transaction below.
	effectiveTargetWorkflowID := int(targetWorkflowID.Int64)
	effectiveTargetWorkflowValid := targetWorkflowID.Valid
	if req.ApplyWorkflowToConfigSet != nil {
		// Verify the target workflow actually exists.
		var exists bool
		if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workflows WHERE id = ?)", *req.ApplyWorkflowToConfigSet).Scan(&exists); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to validate apply_workflow_to_config_set: %w", err))
			return
		}
		if !exists {
			respondBadRequest(w, r, fmt.Sprintf("apply_workflow_to_config_set: workflow %d not found", *req.ApplyWorkflowToConfigSet))
			return
		}
		effectiveTargetWorkflowID = *req.ApplyWorkflowToConfigSet
		effectiveTargetWorkflowValid = true
	}
	if !effectiveTargetWorkflowValid && len(req.StatusMappings) > 0 {
		respondBadRequest(w, r, "Target configuration set has no workflow; cannot accept status mappings")
		return
	}
	for _, mapping := range req.StatusMappings {
		if !h.statusIsInWorkflow(mapping.ToStatusID, effectiveTargetWorkflowID) {
			respondBadRequest(w, r, fmt.Sprintf("Target status ID %d is not part of the target workflow", mapping.ToStatusID))
			return
		}
	}

	// Start transaction for atomic migration
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	stats := struct {
		ItemTypesMigrated   int `json:"item_types_migrated"`
		StatusesMigrated    int `json:"statuses_migrated"`
		PrioritiesMigrated  int `json:"priorities_migrated"`
		CustomFieldsUpdated int `json:"custom_fields_updated"`
	}{}

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
		updateArgs = appendInts(updateArgs, req.WorkspaceIDs)

		var result sql.Result
		result, err = tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to migrate item types: %w", err))
			return
		}
		rowsAffected, _ := result.RowsAffected()
		stats.ItemTypesMigrated += int(rowsAffected)
	}

	// 2. Execute Custom Field Migrations (only add_default needs action)
	for _, mapping := range req.CustomFieldMappings {
		if mapping.Action == "add_default" && mapping.DefaultValue != nil {
			var count int
			count, err = h.addDefaultFieldValue(tx, req.WorkspaceIDs, mapping.FieldID, mapping.DefaultValue)
			if err != nil {
				respondInternalError(w, r, fmt.Errorf("failed to add default field values: %w", err))
				return
			}
			stats.CustomFieldsUpdated += count
		}
		// "keep" and "orphan" require no action - data is preserved in JSON
	}

	// 3. Execute Status Migrations (NULL-aware via *int FromStatusID)
	for _, mapping := range req.StatusMappings {
		rows, err := h.applyStatusMappingTx(tx, mapping, req.WorkspaceIDs, wsPlaceholders, now)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to migrate statuses: %w", err))
			return
		}
		stats.StatusesMigrated += rows
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
		updateArgs = appendInts(updateArgs, req.WorkspaceIDs)

		var result sql.Result
		result, err = tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to migrate priorities: %w", err))
			return
		}
		rowsAffected, _ := result.RowsAffected()
		stats.PrioritiesMigrated += int(rowsAffected)
	}

	// 5. Optional atomic attach: swap each workspace's configuration_set_id
	// from req.OldConfigurationSetID to req.NewConfigurationSetID in the same
	// transaction. Compare-and-swap detects concurrent edits without needing
	// FOR UPDATE (which SQLite does not support).
	swappedWorkspaceIDs := []int{}
	if req.AttachAfterMigration {
		for _, wsID := range req.WorkspaceIDs {
			if err = h.swapWorkspaceConfigSet(tx, wsID, req.OldConfigurationSetID, req.NewConfigurationSetID, now); err != nil {
				if errors.Is(err, errMigrationConflict) {
					respondJSON(w, http.StatusConflict, map[string]interface{}{
						"error":   "workspace_configuration_changed",
						"message": fmt.Sprintf("Workspace %d is no longer assigned to configuration set %d; refresh and retry.", wsID, req.OldConfigurationSetID),
					})
					return
				}
				respondInternalError(w, r, fmt.Errorf("failed to swap workspace configuration set: %w", err))
				return
			}
			swappedWorkspaceIDs = append(swappedWorkspaceIDs, wsID)
		}
	}

	// 6. Optional atomic workflow swap on the target configuration set.
	// Used by the intra-set workflow-change flow: the FE pre-validates target
	// statuses against this workflow, the items are migrated to it, and the
	// config set's workflow_id is updated — all in one transaction.
	if req.ApplyWorkflowToConfigSet != nil {
		if _, err = tx.Exec(`
			UPDATE configuration_sets SET workflow_id = ?, updated_at = ?
			WHERE id = ?
		`, *req.ApplyWorkflowToConfigSet, now, req.NewConfigurationSetID); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to update configuration set workflow: %w", err))
			return
		}
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate permission caches for any workspaces whose config set changed.
	// Also invalidate when only the workflow was swapped, since downstream
	// permission checks consume workflow state via the config set lookup.
	if h.permissionService != nil {
		seen := make(map[int]struct{}, len(swappedWorkspaceIDs))
		for _, wsID := range swappedWorkspaceIDs {
			if _, ok := seen[wsID]; ok {
				continue
			}
			seen[wsID] = struct{}{}
			_ = h.permissionService.InvalidateWorkspaceMemberCaches(wsID)
		}
		if req.ApplyWorkflowToConfigSet != nil {
			affected, _ := repository.NewConfigurationSetRepository(h.db).ListWorkspaceIDsForConfigSet(req.NewConfigurationSetID)
			for _, wsID := range affected {
				if _, ok := seen[wsID]; ok {
					continue
				}
				seen[wsID] = struct{}{}
				_ = h.permissionService.InvalidateWorkspaceMemberCaches(wsID)
			}
		}
	}

	totalMigrated := stats.ItemTypesMigrated + stats.StatusesMigrated + stats.PrioritiesMigrated

	response := map[string]interface{}{
		"success":          true,
		"message":          fmt.Sprintf("Successfully migrated %d items", totalMigrated),
		"migrated_items":   totalMigrated,
		"details":          stats,
		"attached":         req.AttachAfterMigration,
		"workflow_applied": req.ApplyWorkflowToConfigSet != nil,
	}

	respondJSONOK(w, response)
}

// applyStatusMapping executes a single status mapping inside the open
// transaction. Returns the number of rows updated. Branches on whether the
// caller addressed items via NULL status_id (FromStatusID == nil) or a
// concrete status_id, since SQL `=` does not match NULL.
func (h *ConfigurationSetHandler) applyStatusMapping(tx database.Tx, mapping models.StatusMigrationMapping, workspaceIDs []int) (int, error) {
	wsPlaceholders := "?" + strings.Repeat(",?", len(workspaceIDs)-1)
	return h.applyStatusMappingTx(tx, mapping, workspaceIDs, wsPlaceholders, time.Now())
}

func (h *ConfigurationSetHandler) applyStatusMappingTx(tx database.Tx, mapping models.StatusMigrationMapping, workspaceIDs []int, wsPlaceholders string, now time.Time) (int, error) {
	// FromStatusID == nil OR == 0 both mean "items with status_id IS NULL".
	// Older clients may send 0; treat it the same as nil for back-compat.
	fromIsNull := mapping.FromStatusID == nil || *mapping.FromStatusID == 0

	var query string
	var args []interface{}

	switch {
	case fromIsNull && mapping.ItemTypeID != nil:
		query = fmt.Sprintf(`
			UPDATE items SET status_id = ?, updated_at = ?
			WHERE status_id IS NULL
			AND item_type_id = ?
			AND workspace_id IN (%s)`, wsPlaceholders)
		args = []interface{}{mapping.ToStatusID, now, *mapping.ItemTypeID}
	case fromIsNull:
		query = fmt.Sprintf(`
			UPDATE items SET status_id = ?, updated_at = ?
			WHERE status_id IS NULL
			AND workspace_id IN (%s)`, wsPlaceholders)
		args = []interface{}{mapping.ToStatusID, now}
	case mapping.ItemTypeID != nil:
		query = fmt.Sprintf(`
			UPDATE items SET status_id = ?, updated_at = ?
			WHERE status_id = ?
			AND item_type_id = ?
			AND workspace_id IN (%s)`, wsPlaceholders)
		args = []interface{}{mapping.ToStatusID, now, *mapping.FromStatusID, *mapping.ItemTypeID}
	default:
		query = fmt.Sprintf(`
			UPDATE items SET status_id = ?, updated_at = ?
			WHERE status_id = ?
			AND workspace_id IN (%s)`, wsPlaceholders)
		args = []interface{}{mapping.ToStatusID, now, *mapping.FromStatusID}
	}
	args = appendInts(args, workspaceIDs)

	res, err := tx.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	return int(rows), nil
}

// statusIsInWorkflow reports whether statusID participates in workflowID via
// any transition. Mirrors how the analyzer enumerates workflow statuses.
func (h *ConfigurationSetHandler) statusIsInWorkflow(statusID, workflowID int) bool {
	var exists bool
	err := h.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM workflow_transitions
			WHERE workflow_id = ?
			  AND (from_status_id = ? OR to_status_id = ?)
		)
	`, workflowID, statusID, statusID).Scan(&exists)
	return err == nil && exists
}

// allowedItemTypesForConfigSet returns the set of item type IDs allowed in
// the configuration set. If the join table has no rows, the config set
// "accepts all" — return all known item type IDs.
func (h *ConfigurationSetHandler) allowedItemTypesForConfigSet(configSetID int) (map[int]struct{}, error) {
	rows, err := h.db.Query(`
		SELECT item_type_id FROM configuration_set_item_types
		WHERE configuration_set_id = ?
	`, configSetID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	allowed := make(map[int]struct{})
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		allowed[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(allowed) > 0 {
		return allowed, nil
	}
	// Fallback: accept any existing item type.
	rows2, err := h.db.Query(`SELECT id FROM item_types`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows2.Close() }()
	for rows2.Next() {
		var id int
		if err := rows2.Scan(&id); err != nil {
			return nil, err
		}
		allowed[id] = struct{}{}
	}
	if err := rows2.Err(); err != nil {
		return nil, err
	}
	return allowed, nil
}

// allowedPrioritiesForConfigSet mirrors allowedItemTypesForConfigSet for
// configuration_set_priorities.
func (h *ConfigurationSetHandler) allowedPrioritiesForConfigSet(configSetID int) (map[int]struct{}, error) {
	rows, err := h.db.Query(`
		SELECT priority_id FROM configuration_set_priorities
		WHERE configuration_set_id = ?
	`, configSetID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	allowed := make(map[int]struct{})
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		allowed[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(allowed) > 0 {
		return allowed, nil
	}
	rows2, err := h.db.Query(`SELECT id FROM priorities`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows2.Close() }()
	for rows2.Next() {
		var id int
		if err := rows2.Scan(&id); err != nil {
			return nil, err
		}
		allowed[id] = struct{}{}
	}
	if err := rows2.Err(); err != nil {
		return nil, err
	}
	return allowed, nil
}

// swapWorkspaceConfigSet performs a compare-and-swap on
// workspace_configuration_sets. It moves a workspace from oldID → newID,
// requiring that the current row matches oldID (or that no row exists when
// oldID == 0). If neither holds, returns errMigrationConflict so the caller
// can surface a 409 to the client.
func (h *ConfigurationSetHandler) swapWorkspaceConfigSet(tx database.Tx, workspaceID, oldID, newID int, now time.Time) error {
	// Read current state inside the tx for diagnostics.
	var currentID sql.NullInt64
	err := tx.QueryRow(`
		SELECT configuration_set_id FROM workspace_configuration_sets
		WHERE workspace_id = ?
	`, workspaceID).Scan(&currentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	currentIsNull := errors.Is(err, sql.ErrNoRows)
	switch {
	case currentIsNull && oldID == 0:
		// Workspace had no assignment; insert.
		if _, err := tx.Exec(`
			INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
			VALUES (?, ?, ?)
		`, workspaceID, newID, now); err != nil {
			return err
		}
		return nil
	case currentIsNull && oldID != 0:
		return errMigrationConflict
	case !currentIsNull && int(currentID.Int64) != oldID:
		return errMigrationConflict
	default:
		// Compare-and-swap: only update if the row still references oldID.
		res, err := tx.Exec(`
			UPDATE workspace_configuration_sets
			SET configuration_set_id = ?, created_at = ?
			WHERE workspace_id = ? AND configuration_set_id = ?
		`, newID, now, workspaceID, oldID)
		if err != nil {
			return err
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			return errMigrationConflict
		}
		return nil
	}
}

// addDefaultFieldValue adds a default value for a custom field to items that don't have it
func (h *ConfigurationSetHandler) addDefaultFieldValue(tx database.Tx, workspaceIDs []int, fieldID int, defaultValue interface{}) (int, error) {
	fieldKey := strconv.Itoa(fieldID)
	count := 0

	// Get all items in the workspaces (through the repository so NULL handling
	// is uniform with the rest of the codebase).
	itemRows, err := repository.NewItemRepository(h.db).ListItemCustomFieldsTx(tx, workspaceIDs)
	if err != nil {
		return 0, err
	}

	type itemUpdate struct {
		id     int
		newCFV string
	}
	var updates []itemUpdate

	for _, row := range itemRows {
		id := row.ID
		cfvJSON := row.CFVJSON

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

func appendInts(dst []interface{}, src []int) []interface{} {
	for _, v := range src {
		dst = append(dst, v)
	}
	return dst
}

func inIntSet(s map[int]struct{}, v int) bool {
	_, ok := s[v]
	return ok
}
