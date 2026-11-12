package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

// sanitizeStatusMappings bounds the user-facing status label carried on each
// mapping. FromStatus is the display name echoed back from the analyzer; the
// IDs are validated numerically and stay untouched.
func sanitizeStatusMappings(mappings []models.StatusMigrationMapping) {
	for i := range mappings {
		sanitize.Apply(&mappings[i].FromStatus, sanitize.PlainTextField)
	}
}

// defaultValueNodeBudget bounds migration default-value JSON structure.
const defaultValueNodeBudget = 256

// sanitizeJSONValue recursively sanitizes strings and keys under a shared node
// budget, preventing nested values from bypassing input bounds.
func sanitizeJSONValue(v any, budget *int) any {
	if *budget <= 0 {
		return nil
	}
	*budget--
	switch x := v.(type) {
	case string:
		return sanitize.PlainTextField.Sanitize(x)
	case []any:
		out := make([]any, 0, len(x))
		for _, e := range x {
			if *budget <= 0 {
				break
			}
			out = append(out, sanitizeJSONValue(e, budget))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			if *budget <= 0 {
				break
			}
			out[sanitize.PlainTextField.Sanitize(k)] = sanitizeJSONValue(e, budget)
		}
		return out
	default:
		return x
	}
}

// sanitizeCustomFieldMappings scrubs the custom-field migration batch. Action
// is identifier-shaped ("keep"/"orphan"/"add_default"); DefaultValue is
// persisted verbatim into item custom-field JSON and rendered on item views,
// so every string anywhere in its decoded shape gets the plain-text scrub —
// not just a top-level string, which would leave array/object wrappers as a
// trivial bypass. The node budget bounds the structure itself.
func sanitizeCustomFieldMappings(mappings []models.CustomFieldMigrationMapping) {
	for i := range mappings {
		sanitize.Apply(&mappings[i].Action, sanitize.ShortIdentifier)
		budget := defaultValueNodeBudget
		mappings[i].DefaultValue = sanitizeJSONValue(mappings[i].DefaultValue, &budget)
	}
}

// errMigrationConflict signals that a compare-and-swap on
// workspace_configuration_sets failed because another caller changed the
// workspace's assignment between analysis and execution. Surface as 409.
var errMigrationConflict = errors.New("migration conflict")

func (h *ConfigurationSetHandler) ExecuteMigration(w http.ResponseWriter, r *http.Request) {
	migrationReq, ok := decodeJSON[models.WorkflowMigrationRequest](w, r)
	if !ok {
		return
	}
	sanitizeStatusMappings(migrationReq.StatusMappings)

	// Validate the configuration set exists.
	var configSetExists bool
	err := h.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)
	`, migrationReq.ConfigurationSetID).Scan(&configSetExists)
	if err != nil || !configSetExists {
		respondBadRequest(w, r, "Configuration set not found")
		return
	}

	// Validate workspace IDs provided
	if len(migrationReq.WorkspaceIDs) == 0 {
		respondValidationError(w, r, "At least one workspace ID is required")
		return
	}

	// Validate target status IDs against each mapping's effective target
	// workflow. Item-type workflow overrides take precedence over the set's
	// default workflow.
	for _, mapping := range migrationReq.StatusMappings {
		workflowID, resolveErr := h.resolveConfigSetWorkflowID(migrationReq.ConfigurationSetID, mapping.ItemTypeID, nil)
		if resolveErr != nil {
			respondInternalError(w, r, resolveErr)
			return
		}
		if workflowID == nil {
			respondBadRequest(w, r, "Target configuration set has no workflow; cannot accept status mappings")
			return
		}
		if !h.statusIsInWorkflow(mapping.ToStatusID, *workflowID) {
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

	// Remapped items land in item history attributed to the acting admin.
	actorUserID := 0
	if currentUser := utils.GetCurrentUser(r); currentUser != nil {
		actorUserID = currentUser.ID
	}

	totalMigrated := 0
	for _, mapping := range migrationReq.StatusMappings {
		rows, err := h.applyStatusMapping(tx, mapping, migrationReq.WorkspaceIDs, actorUserID)
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

	if currentUser := utils.GetCurrentUser(r); currentUser != nil {
		logAuditWithDetails(h.db, r, currentUser, logger.ActionConfigSetMigrationExecute, logger.ResourceConfigurationSet, &migrationReq.ConfigurationSetID, "", map[string]any{
			"configuration_set_id": migrationReq.ConfigurationSetID,
			"workspace_ids":        migrationReq.WorkspaceIDs,
			"status_mapping_count": len(migrationReq.StatusMappings),
			"migrated_items":       totalMigrated,
		})
	}

	response := map[string]any{
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
	sanitizeStatusMappings(req.StatusMappings)
	sanitizeCustomFieldMappings(req.CustomFieldMappings)

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
	if err := h.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)
	`, req.NewConfigurationSetID).Scan(&newConfigSetExists); err != nil {
		slog.Error("migration validation failed", slog.Any("error", err), slog.String("check", "new_configuration_set"))
		respondInternalError(w, r, fmt.Errorf("failed to validate new configuration set: %w", err))
		return
	}
	if !newConfigSetExists {
		respondBadRequest(w, r, "New configuration set not found")
		return
	}
	if req.ApplyWorkflowToConfigSet != nil {
		var exists bool
		if err := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflows WHERE id = ?)`, *req.ApplyWorkflowToConfigSet).Scan(&exists); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to validate apply_workflow_to_config_set: %w", err))
			return
		}
		if !exists {
			respondBadRequest(w, r, fmt.Sprintf("apply_workflow_to_config_set: workflow %d not found", *req.ApplyWorkflowToConfigSet))
			return
		}
	}

	// Validate workspace IDs provided
	if len(req.WorkspaceIDs) == 0 {
		respondValidationError(w, r, "At least one workspace ID is required")
		return
	}

	// Target item types: must be members of the target config set's allow-list,
	// or any item_type if the target has no explicit list ("accept all"). The
	// intra-set removal flow validates against the proposed list that will be
	// applied atomically below rather than the list currently persisted.
	var allowedItemTypes map[int]struct{}
	var err error
	if req.ApplyItemTypeConfigsToConfigSet != nil {
		allowedItemTypes, err = h.allowedItemTypesForConfigs(*req.ApplyItemTypeConfigsToConfigSet)
	} else {
		allowedItemTypes, err = h.allowedItemTypesForConfigSet(req.NewConfigurationSetID)
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load target item types: %w", err))
		return
	}
	for _, mapping := range req.ItemTypeMappings {
		if !inIntSet(allowedItemTypes, mapping.ToItemTypeID) {
			respondBadRequest(w, r, fmt.Sprintf("Target item type ID %d is not allowed by the target configuration set", mapping.ToItemTypeID))
			return
		}
		if mapping.FromItemTypeID != nil {
			sameLevel, err := h.itemTypesShareHierarchyLevel(*mapping.FromItemTypeID, mapping.ToItemTypeID)
			if err != nil {
				respondBadRequest(w, r, err.Error())
				return
			}
			if !sameLevel {
				respondBadRequest(w, r, fmt.Sprintf(
					"Target item type ID %d must use the same hierarchy level as source item type ID %d",
					mapping.ToItemTypeID, *mapping.FromItemTypeID,
				))
				return
			}
		}
	}

	// When this call also narrows the configuration set's allow-list, require
	// a mapping for every currently-used type that would become disallowed.
	// This prevents a forged or stale wizard request from applying the config
	// change while leaving items behind on removed types.
	if req.ApplyItemTypeConfigsToConfigSet != nil {
		required, _, _, err := h.analyzeItemTypesAgainstConfigs(req.WorkspaceIDs, *req.ApplyItemTypeConfigsToConfigSet)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to validate proposed item type configuration: %w", err))
			return
		}
		provided := make(map[int]struct{}, len(req.ItemTypeMappings))
		providedNil := false
		for _, mapping := range req.ItemTypeMappings {
			if mapping.FromItemTypeID == nil {
				providedNil = true
				continue
			}
			provided[*mapping.FromItemTypeID] = struct{}{}
		}
		for _, migration := range required {
			if migration.CurrentItemTypeID == nil {
				if providedNil {
					continue
				}
			} else if _, ok := provided[*migration.CurrentItemTypeID]; ok {
				continue
			}
			respondJSON(w, http.StatusConflict, map[string]any{
				"error":   "item_type_migration_incomplete",
				"message": fmt.Sprintf("A migration mapping is required for item type %q", migration.CurrentItemTypeName),
			})
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

	// Target statuses are validated against the workflow effective for each
	// mapping's item type. A pending default-workflow change overrides only the
	// set-level workflow; item-type overrides still win.
	for _, mapping := range req.StatusMappings {
		workflowID, resolveErr := h.resolveConfigSetWorkflowID(req.NewConfigurationSetID, mapping.ItemTypeID, req.ApplyWorkflowToConfigSet)
		if resolveErr != nil {
			respondInternalError(w, r, resolveErr)
			return
		}
		if workflowID == nil {
			respondBadRequest(w, r, "Target configuration set has no workflow; cannot accept status mappings")
			return
		}
		if !h.statusIsInWorkflow(mapping.ToStatusID, *workflowID) {
			respondBadRequest(w, r, fmt.Sprintf("Target status ID %d is not part of the target workflow", mapping.ToStatusID))
			return
		}
	}

	// Reject forged, stale, or malformed mappings before any write.
	if invalid, err := h.validateMigrationMappingPayload(&req); err != nil {
		respondInternalError(w, r, err)
		return
	} else if invalid != "" {
		respondBadRequest(w, r, invalid)
		return
	}

	if req.AttachAfterMigration {
		incomplete, validateErr := h.validateAttachMigrationMappings(&req)
		if validateErr != nil {
			respondInternalError(w, r, validateErr)
			return
		}
		if incomplete != "" {
			respondJSON(w, http.StatusConflict, map[string]any{
				"error":   "migration_incomplete",
				"message": incomplete,
			})
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

	itemRepo := repository.NewItemRepository(h.db)

	// Remapped items land in item history attributed to the acting admin.
	actorUserID := 0
	if currentUser := utils.GetCurrentUser(r); currentUser != nil {
		actorUserID = currentUser.ID
	}

	// 1. Execute Item Type Migrations
	for _, mapping := range req.ItemTypeMappings {
		var rowsAffected int
		rowsAffected, err = itemRepo.RemapFieldForWorkspacesTxAsUser(tx, "item_type_id", mapping.FromItemTypeID, mapping.ToItemTypeID, nil, req.WorkspaceIDs, now, actorUserID)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to migrate item types: %w", err))
			return
		}
		stats.ItemTypesMigrated += rowsAffected
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
		rows, err := h.applyStatusMappingTx(tx, mapping, req.WorkspaceIDs, now, actorUserID)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to migrate statuses: %w", err))
			return
		}
		stats.StatusesMigrated += rows
	}

	// 4. Execute Priority Migrations
	for _, mapping := range req.PriorityMappings {
		var rowsAffected int
		rowsAffected, err = itemRepo.RemapFieldForWorkspacesTxAsUser(tx, "priority_id", mapping.FromPriorityID, mapping.ToPriorityID, nil, req.WorkspaceIDs, now, actorUserID)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to migrate priorities: %w", err))
			return
		}
		stats.PrioritiesMigrated += rowsAffected
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
					respondJSON(w, http.StatusConflict, map[string]any{
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

	// 7. Optional atomic item-type allow-list replacement. Item remaps above
	// and this configuration change commit together, so there is no successful
	// state in which migrated items still depend on a removed type.
	if req.ApplyItemTypeConfigsToConfigSet != nil {
		configRepo := repository.NewConfigurationSetRepository(h.db)
		if err = configRepo.SaveItemTypeConfigs(tx, req.NewConfigurationSetID, *req.ApplyItemTypeConfigsToConfigSet); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to apply configuration set item types: %w", err))
			return
		}
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate permission caches for any workspaces whose config set changed.
	// Also invalidate when the workflow or item-type list was changed, since
	// downstream permission and validation checks consume configuration-set
	// state.
	if h.permissionService != nil {
		seen := make(map[int]struct{}, len(swappedWorkspaceIDs))
		for _, wsID := range swappedWorkspaceIDs {
			if _, ok := seen[wsID]; ok {
				continue
			}
			seen[wsID] = struct{}{}
			_ = h.permissionService.InvalidateWorkspaceMemberCaches(wsID)
		}
		if req.ApplyWorkflowToConfigSet != nil || req.ApplyItemTypeConfigsToConfigSet != nil {
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

	if currentUser := utils.GetCurrentUser(r); currentUser != nil {
		logAuditWithDetails(h.db, r, currentUser, logger.ActionConfigSetComprehensiveMigrationExecute, logger.ResourceConfigurationSet, &req.NewConfigurationSetID, "", map[string]any{
			"old_configuration_set_id":   req.OldConfigurationSetID,
			"new_configuration_set_id":   req.NewConfigurationSetID,
			"workspace_ids":              req.WorkspaceIDs,
			"item_type_mapping_count":    len(req.ItemTypeMappings),
			"status_mapping_count":       len(req.StatusMappings),
			"priority_mapping_count":     len(req.PriorityMappings),
			"custom_field_mapping_count": len(req.CustomFieldMappings),
			"migrated_items":             totalMigrated,
			"details":                    stats,
			"attached":                   req.AttachAfterMigration,
			"workflow_applied":           req.ApplyWorkflowToConfigSet != nil,
			"item_types_applied":         req.ApplyItemTypeConfigsToConfigSet != nil,
		})
	}

	response := map[string]any{
		"success":            true,
		"message":            fmt.Sprintf("Successfully migrated %d items", totalMigrated),
		"migrated_items":     totalMigrated,
		"details":            stats,
		"attached":           req.AttachAfterMigration,
		"workflow_applied":   req.ApplyWorkflowToConfigSet != nil,
		"item_types_applied": req.ApplyItemTypeConfigsToConfigSet != nil,
	}

	respondJSONOK(w, response)
}

// applyStatusMapping executes a single status mapping inside the open
// transaction. Returns the number of rows updated. Branches on whether the
// caller addressed items via NULL status_id (FromStatusID == nil) or a
// concrete status_id, since SQL `=` does not match NULL.
func (h *ConfigurationSetHandler) applyStatusMapping(tx database.Tx, mapping models.StatusMigrationMapping, workspaceIDs []int, actorUserID int) (int, error) {
	return h.applyStatusMappingTx(tx, mapping, workspaceIDs, time.Now(), actorUserID)
}

func (h *ConfigurationSetHandler) applyStatusMappingTx(tx database.Tx, mapping models.StatusMigrationMapping, workspaceIDs []int, now time.Time, actorUserID int) (int, error) {
	// FromStatusID == nil OR == 0 both mean "items with status_id IS NULL".
	// Older clients may send 0; treat it the same as nil for back-compat.
	fromStatusID := mapping.FromStatusID
	if fromStatusID != nil && *fromStatusID == 0 {
		fromStatusID = nil
	}
	return repository.NewItemRepository(h.db).RemapFieldForWorkspacesTxAsUser(tx, "status_id", fromStatusID, mapping.ToStatusID, mapping.ItemTypeID, workspaceIDs, now, actorUserID)
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

// allowedItemTypesForConfigs resolves a proposed item-type configuration list.
// As with persisted configuration sets, an empty list means "accept all".
func (h *ConfigurationSetHandler) allowedItemTypesForConfigs(configs []models.ItemTypeConfig) (map[int]struct{}, error) {
	if len(configs) == 0 {
		rows, err := h.db.Query(`SELECT id FROM item_types`)
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
		return allowed, rows.Err()
	}

	allowed := make(map[int]struct{}, len(configs))
	for _, config := range configs {
		var exists bool
		if err := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)`, config.ItemTypeID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("proposed item type ID %d does not exist", config.ItemTypeID)
		}
		allowed[config.ItemTypeID] = struct{}{}
	}
	return allowed, nil
}

func (h *ConfigurationSetHandler) itemTypesShareHierarchyLevel(sourceID, targetID int) (bool, error) {
	var sourceLevel, targetLevel int
	if err := h.db.QueryRow(`SELECT hierarchy_level FROM item_types WHERE id = ?`, sourceID).Scan(&sourceLevel); err != nil {
		return false, fmt.Errorf("source item type ID %d does not exist", sourceID)
	}
	if err := h.db.QueryRow(`SELECT hierarchy_level FROM item_types WHERE id = ?`, targetID).Scan(&targetLevel); err != nil {
		return false, fmt.Errorf("target item type ID %d does not exist", targetID)
	}
	return sourceLevel == targetLevel, nil
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
func (h *ConfigurationSetHandler) addDefaultFieldValue(tx database.Tx, workspaceIDs []int, fieldID int, defaultValue any) (int, error) {
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

		var cfv map[string]any
		if err := json.Unmarshal([]byte(cfvJSON), &cfv); err != nil {
			cfv = make(map[string]any)
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
	itemRepo := repository.NewItemRepository(h.db)
	for _, update := range updates {
		if err := itemRepo.SetCustomFieldValuesRawTx(tx, update.id, update.newCFV, now); err != nil {
			return 0, err
		}
		count++
	}

	return count, nil
}

// migrationStatusSourceKey identifies a status-migration source. statusID 0
// means items with NULL status_id; itemTypeID 0 means "no per-type filter".
type migrationStatusSourceKey struct {
	statusID   int
	itemTypeID int
}

// validateMigrationMappingPayload rejects forged, stale, or malformed mappings
// before any write. It returns a user-facing message for a 400 response, or an
// error when the analysis itself fails.
func (h *ConfigurationSetHandler) validateMigrationMappingPayload(req *models.ComprehensiveMigrationRequest) (string, error) {
	seenWorkspaces := make(map[int]struct{}, len(req.WorkspaceIDs))
	for _, workspaceID := range req.WorkspaceIDs {
		if _, duplicate := seenWorkspaces[workspaceID]; duplicate {
			return fmt.Sprintf("Duplicate workspace ID %d", workspaceID), nil
		}
		seenWorkspaces[workspaceID] = struct{}{}
		exists, err := h.repo.WorkspaceExists(workspaceID)
		if err != nil {
			return "", err
		}
		if !exists {
			return fmt.Sprintf("Workspace %d not found", workspaceID), nil
		}
	}

	effectiveSourceID, _, err := h.resolveMigrationSourceConfigSet(req.OldConfigurationSetID)
	if err != nil {
		return "", err
	}

	// Item-type sources are path-dependent: an intra-set allow-list change is
	// validated against the proposed list, not the persisted one.
	validItemTypeSources := make(map[int]struct{})
	if req.ApplyItemTypeConfigsToConfigSet != nil {
		required, _, _, err := h.analyzeItemTypesAgainstConfigs(req.WorkspaceIDs, *req.ApplyItemTypeConfigsToConfigSet)
		if err != nil {
			return "", err
		}
		for _, migration := range required {
			validItemTypeSources[nullableInt(migration.CurrentItemTypeID)] = struct{}{}
		}
	} else {
		for _, workspaceID := range req.WorkspaceIDs {
			migrations, _, _, err := h.analyzeItemTypeMigration(workspaceID, req.NewConfigurationSetID)
			if err != nil {
				return "", err
			}
			for _, migration := range migrations {
				if migration.RequiresMigration {
					validItemTypeSources[nullableInt(migration.CurrentItemTypeID)] = struct{}{}
				}
			}
		}
	}

	validPrioritySources := make(map[int]struct{})
	for _, workspaceID := range req.WorkspaceIDs {
		migrations, _, _, err := h.analyzePriorityMigration(workspaceID, req.NewConfigurationSetID)
		if err != nil {
			return "", err
		}
		for _, migration := range migrations {
			if migration.RequiresMigration {
				validPrioritySources[nullableInt(migration.CurrentPriorityID)] = struct{}{}
			}
		}
	}

	statusMigrations, _, err := h.analyzeStatusMigrationForConfigSet(req.WorkspaceIDs, req.NewConfigurationSetID, req.ApplyWorkflowToConfigSet)
	if err != nil {
		return "", err
	}
	validStatusSources := make(map[migrationStatusSourceKey]struct{}, len(statusMigrations))
	for _, migration := range statusMigrations {
		validStatusSources[migrationStatusSourceKey{
			statusID:   nullableInt(migration.CurrentStatusID),
			itemTypeID: nullableInt(migration.ItemTypeID),
		}] = struct{}{}
	}

	validFieldActions := make(map[int]string)
	for _, workspaceID := range req.WorkspaceIDs {
		migrations, _, err := h.analyzeCustomFieldMigration(workspaceID, effectiveSourceID, req.NewConfigurationSetID)
		if err != nil {
			return "", err
		}
		for _, migration := range migrations {
			validFieldActions[migration.FieldID] = migration.Action
		}
	}

	seenItemTypes := make(map[int]struct{}, len(req.ItemTypeMappings))
	for _, mapping := range req.ItemTypeMappings {
		fromID := nullableInt(mapping.FromItemTypeID)
		if _, duplicate := seenItemTypes[fromID]; duplicate {
			return "Duplicate item type mapping", nil
		}
		seenItemTypes[fromID] = struct{}{}
		if _, ok := validItemTypeSources[fromID]; !ok {
			return fmt.Sprintf("Item type mapping for source item type ID %d is not part of the migration", fromID), nil
		}
	}

	seenPriorities := make(map[int]struct{}, len(req.PriorityMappings))
	for _, mapping := range req.PriorityMappings {
		fromID := nullableInt(mapping.FromPriorityID)
		if _, duplicate := seenPriorities[fromID]; duplicate {
			return "Duplicate priority mapping", nil
		}
		seenPriorities[fromID] = struct{}{}
		if _, ok := validPrioritySources[fromID]; !ok {
			return fmt.Sprintf("Priority mapping for source priority ID %d is not part of the migration", fromID), nil
		}
	}

	seenStatuses := make(map[migrationStatusSourceKey]struct{}, len(req.StatusMappings))
	for _, mapping := range req.StatusMappings {
		key := migrationStatusSourceKey{
			statusID:   nullableInt(mapping.FromStatusID),
			itemTypeID: nullableInt(mapping.ItemTypeID),
		}
		if _, duplicate := seenStatuses[key]; duplicate {
			return "Duplicate status mapping", nil
		}
		seenStatuses[key] = struct{}{}
		if _, ok := validStatusSources[key]; !ok {
			return "Status mapping source is not part of the migration", nil
		}
	}

	seenFields := make(map[int]struct{}, len(req.CustomFieldMappings))
	for _, mapping := range req.CustomFieldMappings {
		if _, duplicate := seenFields[mapping.FieldID]; duplicate {
			return "Duplicate custom field mapping", nil
		}
		seenFields[mapping.FieldID] = struct{}{}
		switch mapping.Action {
		case "keep", "orphan", "add_default":
		default:
			return fmt.Sprintf("Unsupported custom field action %q", mapping.Action), nil
		}
		action, ok := validFieldActions[mapping.FieldID]
		if !ok {
			return fmt.Sprintf("Custom field mapping references unknown field ID %d", mapping.FieldID), nil
		}
		if mapping.Action != action {
			return fmt.Sprintf("Custom field %d action %q does not match the analyzed action %q", mapping.FieldID, mapping.Action, action), nil
		}
		if mapping.Action == "add_default" && mapping.DefaultValue == nil {
			return fmt.Sprintf("Custom field %d requires a default value", mapping.FieldID), nil
		}
	}

	return "", nil
}

func (h *ConfigurationSetHandler) validateAttachMigrationMappings(req *models.ComprehensiveMigrationRequest) (string, error) {
	providedItemTypes := make(map[int]struct{}, len(req.ItemTypeMappings))
	for _, mapping := range req.ItemTypeMappings {
		fromID := 0
		if mapping.FromItemTypeID != nil {
			fromID = *mapping.FromItemTypeID
		}
		providedItemTypes[fromID] = struct{}{}
	}
	providedPriorities := make(map[int]struct{}, len(req.PriorityMappings))
	for _, mapping := range req.PriorityMappings {
		fromID := 0
		if mapping.FromPriorityID != nil {
			fromID = *mapping.FromPriorityID
		}
		providedPriorities[fromID] = struct{}{}
	}
	type statusKey struct {
		statusID   int
		itemTypeID int
	}
	providedStatuses := make(map[statusKey]struct{}, len(req.StatusMappings))
	for _, mapping := range req.StatusMappings {
		key := statusKey{}
		if mapping.FromStatusID != nil {
			key.statusID = *mapping.FromStatusID
		}
		if mapping.ItemTypeID != nil {
			key.itemTypeID = *mapping.ItemTypeID
		}
		providedStatuses[key] = struct{}{}
	}
	providedDefaults := make(map[int]struct{}, len(req.CustomFieldMappings))
	for _, mapping := range req.CustomFieldMappings {
		if mapping.Action == "add_default" && mapping.DefaultValue != nil {
			providedDefaults[mapping.FieldID] = struct{}{}
		}
	}

	effectiveSourceID, _, err := h.resolveMigrationSourceConfigSet(req.OldConfigurationSetID)
	if err != nil {
		return "", err
	}
	for _, workspaceID := range req.WorkspaceIDs {
		itemTypes, _, _, err := h.analyzeItemTypeMigration(workspaceID, req.NewConfigurationSetID)
		if err != nil {
			return "", err
		}
		for _, migration := range itemTypes {
			if !migration.RequiresMigration {
				continue
			}
			fromID := 0
			if migration.CurrentItemTypeID != nil {
				fromID = *migration.CurrentItemTypeID
			}
			if _, ok := providedItemTypes[fromID]; !ok {
				return fmt.Sprintf("A migration mapping is required for item type %q", migration.CurrentItemTypeName), nil
			}
		}

		priorities, _, _, err := h.analyzePriorityMigration(workspaceID, req.NewConfigurationSetID)
		if err != nil {
			return "", err
		}
		for _, migration := range priorities {
			if !migration.RequiresMigration {
				continue
			}
			fromID := 0
			if migration.CurrentPriorityID != nil {
				fromID = *migration.CurrentPriorityID
			}
			if _, ok := providedPriorities[fromID]; !ok {
				return fmt.Sprintf("A migration mapping is required for priority %q", migration.CurrentPriorityName), nil
			}
		}

		fields, _, err := h.analyzeCustomFieldMigration(workspaceID, effectiveSourceID, req.NewConfigurationSetID)
		if err != nil {
			return "", err
		}
		for _, migration := range fields {
			if !migration.RequiresDefault {
				continue
			}
			if _, ok := providedDefaults[migration.FieldID]; !ok {
				return fmt.Sprintf("A default value is required for custom field %q", migration.FieldName), nil
			}
		}
	}

	statuses, _, err := h.analyzeStatusMigrationForConfigSet(req.WorkspaceIDs, req.NewConfigurationSetID, req.ApplyWorkflowToConfigSet)
	if err != nil {
		return "", err
	}
	for _, migration := range statuses {
		if !migration.RequiresMigration {
			continue
		}
		key := statusKey{}
		if migration.CurrentStatusID != nil {
			key.statusID = *migration.CurrentStatusID
		}
		if migration.ItemTypeID != nil {
			key.itemTypeID = *migration.ItemTypeID
		}
		if _, ok := providedStatuses[key]; !ok {
			return fmt.Sprintf("A migration mapping is required for status %q", migration.CurrentStatus), nil
		}
	}
	return "", nil
}

func inIntSet(s map[int]struct{}, v int) bool {
	_, ok := s[v]
	return ok
}
