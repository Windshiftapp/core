package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// normalizeStatusName normalizes status names for comparison by replacing underscores with spaces and converting to lowercase
func normalizeStatusName(name string) string {
	// Replace underscores with spaces and convert to lowercase
	normalized := strings.ReplaceAll(name, "_", " ")
	return strings.ToLower(normalized)
}

// Migration Assistant endpoints

func (h *ConfigurationSetHandler) AnalyzeMigration(w http.ResponseWriter, r *http.Request) {
	configSetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get configuration set details including differentiate_by_item_type
	var configSet models.ConfigurationSet
	var workflowName sql.NullString
	err := h.db.QueryRow(`
		SELECT cs.id, cs.name, cs.workflow_id, cs.differentiate_by_item_type, wf.name as workflow_name
		FROM configuration_sets cs
		LEFT JOIN workflows wf ON cs.workflow_id = wf.id
		WHERE cs.id = ?
	`, configSetID).Scan(&configSet.ID, &configSet.Name, &configSet.WorkflowID, &configSet.DifferentiateByItemType, &workflowName)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "Configuration set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	configSet.WorkflowName = workflowName.String

	// Get affected workspaces - check for workspace filter in query params
	workspaceFilter := r.URL.Query().Get("workspace_id")
	var affectedWorkspaces []int

	if workspaceFilter != "" {
		// If workspace filter is provided, only analyze that specific workspace
		workspaceID, err := strconv.Atoi(workspaceFilter)
		if err != nil {
			respondBadRequest(w, r, "Invalid workspace_id parameter")
			return
		}
		affectedWorkspaces = []int{workspaceID}
	} else {
		// Otherwise, get all workspaces assigned to this configuration set
		workspaceQuery := `
			SELECT w.id
			FROM workspace_configuration_sets wcs
			JOIN workspaces w ON wcs.workspace_id = w.id
			WHERE wcs.configuration_set_id = ?`

		workspaceRows, err := h.db.Query(workspaceQuery, configSetID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		defer func() { _ = workspaceRows.Close() }()

		for workspaceRows.Next() {
			var workspaceID int
			if err := workspaceRows.Scan(&workspaceID); err != nil {
				respondInternalError(w, r, err)
				return
			}
			affectedWorkspaces = append(affectedWorkspaces, workspaceID)
		}
	}

	// Handle empty workspace list
	if len(affectedWorkspaces) == 0 {
		analysis := models.WorkflowMigrationAnalysis{
			NewWorkflowID:      configSet.WorkflowID,
			NewWorkflowName:    configSet.WorkflowName,
			AffectedWorkspaces: affectedWorkspaces,
			StatusMigrations:   []models.StatusMigrationInfo{},
			RequiresMigration:  false,
			TotalAffectedItems: 0,
		}
		respondJSONOK(w, analysis)
		return
	}

	// Use WorkflowService for proper item type workflow resolution
	workflowService := services.NewWorkflowService(h.db)

	var statusMigrations []models.StatusMigrationInfo
	totalAffectedItems := 0
	requiresMigration := false

	// When differentiate_by_item_type is enabled, analyze per item type
	if configSet.DifferentiateByItemType {
		// Get items grouped by item type and status
		statusQuery := `
			SELECT i.item_type_id, COALESCE(it.name, '') as item_type_name,
			       COALESCE(s.id, 0) as status_id, COALESCE(s.name, '') as status_name,
			       COUNT(*) as item_count
			FROM items i
			LEFT JOIN item_types it ON i.item_type_id = it.id
			LEFT JOIN statuses s ON i.status_id = s.id
			WHERE i.workspace_id IN (?` + strings.Repeat(",?", len(affectedWorkspaces)-1) + `)
			GROUP BY i.item_type_id, it.name, s.id, s.name
			ORDER BY it.name, s.name`

		statusArgs := make([]interface{}, len(affectedWorkspaces))
		for i, wsID := range affectedWorkspaces {
			statusArgs[i] = wsID
		}

		statusRows, err := h.db.Query(statusQuery, statusArgs...)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		defer func() { _ = statusRows.Close() }()

		// Cache workflow statuses by workflow ID to avoid repeated queries
		workflowStatusesCache := make(map[int]map[string]models.Status)

		for statusRows.Next() {
			var itemTypeID sql.NullInt64
			var itemTypeName string
			var currentStatusID int
			var currentStatusName string
			var itemCount int

			if err := statusRows.Scan(&itemTypeID, &itemTypeName, &currentStatusID, &currentStatusName, &itemCount); err != nil {
				respondInternalError(w, r, err)
				return
			}

			totalAffectedItems += itemCount

			// Get the workflow for this item type using WorkflowService
			itemTypeIDPtr := utils.NullInt64ToPtr(itemTypeID)

			// Use first workspace for workflow lookup (they all share the same config set)
			workflowID, err := workflowService.GetWorkflowIDForItem(affectedWorkspaces[0], itemTypeIDPtr)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}

			migration := models.StatusMigrationInfo{
				CurrentStatus:     currentStatusName,
				CurrentStatusID:   &currentStatusID,
				ItemTypeID:        itemTypeIDPtr,
				ItemTypeName:      itemTypeName,
				RequiresMigration: false,
				ItemCount:         itemCount,
			}

			// No workflow configured - no migration needed for this item type
			if workflowID == nil {
				statusMigrations = append(statusMigrations, migration)
				continue
			}

			// Get or cache workflow statuses
			workflowStatuses, exists := workflowStatusesCache[*workflowID]
			if !exists {
				workflowStatuses = make(map[string]models.Status)
				workflowStatusQuery := `
					SELECT DISTINCT s.id, s.name
					FROM workflow_transitions wt
					JOIN statuses s ON (wt.from_status_id = s.id OR wt.to_status_id = s.id)
					WHERE wt.workflow_id = ?
					ORDER BY s.name`

				workflowStatusRows, err := h.db.Query(workflowStatusQuery, *workflowID)
				if err != nil {
					respondInternalError(w, r, err)
					return
				}

				for workflowStatusRows.Next() {
					var status models.Status
					if err := workflowStatusRows.Scan(&status.ID, &status.Name); err != nil {
						_ = workflowStatusRows.Close()
						respondInternalError(w, r, err)
						return
					}
					normalizedName := normalizeStatusName(status.Name)
					workflowStatuses[normalizedName] = status
				}
				_ = workflowStatusRows.Close()

				workflowStatusesCache[*workflowID] = workflowStatuses
			}

			// Check if current status exists in workflow
			normalizedCurrentStatus := normalizeStatusName(currentStatusName)
			if workflowStatus, exists := workflowStatuses[normalizedCurrentStatus]; exists {
				migration.SuggestedStatusID = &workflowStatus.ID
				migration.SuggestedStatusName = workflowStatus.Name
			} else {
				migration.RequiresMigration = true
				requiresMigration = true
				h.suggestStatusMapping(&migration, normalizedCurrentStatus, workflowStatuses)
			}

			statusMigrations = append(statusMigrations, migration)
		}
	} else {
		// Original behavior: no item type differentiation, use config set default workflow
		if configSet.WorkflowID == nil {
			analysis := models.WorkflowMigrationAnalysis{
				NewWorkflowID:      configSet.WorkflowID,
				NewWorkflowName:    configSet.WorkflowName,
				AffectedWorkspaces: affectedWorkspaces,
				StatusMigrations:   []models.StatusMigrationInfo{},
				RequiresMigration:  false,
				TotalAffectedItems: 0,
			}
			respondJSONOK(w, analysis)
			return
		}

		// Get current statuses used in affected workspaces and their counts
		statusQuery := `
			SELECT COALESCE(s.id, 0) as status_id, COALESCE(s.name, '') as status_name, COUNT(*) as item_count
			FROM items i
			LEFT JOIN statuses s ON i.status_id = s.id
			WHERE i.workspace_id IN (?` + strings.Repeat(",?", len(affectedWorkspaces)-1) + `)
			GROUP BY s.id, s.name
			ORDER BY s.name`

		statusArgs := make([]interface{}, len(affectedWorkspaces))
		for i, wsID := range affectedWorkspaces {
			statusArgs[i] = wsID
		}

		statusRows, err := h.db.Query(statusQuery, statusArgs...)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		defer func() { _ = statusRows.Close() }()

		// Get available statuses in the workflow
		workflowStatusQuery := `
			SELECT DISTINCT s.id, s.name
			FROM workflow_transitions wt
			JOIN statuses s ON (wt.from_status_id = s.id OR wt.to_status_id = s.id)
			WHERE wt.workflow_id = ?
			ORDER BY s.name`

		workflowStatusRows, err := h.db.Query(workflowStatusQuery, *configSet.WorkflowID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		defer func() { _ = workflowStatusRows.Close() }()

		// Build map of available workflow statuses
		workflowStatuses := make(map[string]models.Status)
		for workflowStatusRows.Next() {
			var status models.Status
			if err := workflowStatusRows.Scan(&status.ID, &status.Name); err != nil {
				respondInternalError(w, r, err)
				return
			}
			normalizedName := normalizeStatusName(status.Name)
			workflowStatuses[normalizedName] = status
		}

		// Analyze each current status
		for statusRows.Next() {
			var currentStatusID int
			var currentStatusName string
			var itemCount int

			if err := statusRows.Scan(&currentStatusID, &currentStatusName, &itemCount); err != nil {
				respondInternalError(w, r, err)
				return
			}

			totalAffectedItems += itemCount

			migration := models.StatusMigrationInfo{
				CurrentStatus:     currentStatusName,
				CurrentStatusID:   &currentStatusID,
				RequiresMigration: false,
				ItemCount:         itemCount,
			}

			normalizedCurrentStatus := normalizeStatusName(currentStatusName)
			if workflowStatus, exists := workflowStatuses[normalizedCurrentStatus]; exists {
				migration.SuggestedStatusID = &workflowStatus.ID
				migration.SuggestedStatusName = workflowStatus.Name
			} else {
				migration.RequiresMigration = true
				requiresMigration = true
				h.suggestStatusMapping(&migration, normalizedCurrentStatus, workflowStatuses)
			}

			statusMigrations = append(statusMigrations, migration)
		}
	}

	analysis := models.WorkflowMigrationAnalysis{
		NewWorkflowID:      configSet.WorkflowID,
		NewWorkflowName:    configSet.WorkflowName,
		AffectedWorkspaces: affectedWorkspaces,
		StatusMigrations:   statusMigrations,
		RequiresMigration:  requiresMigration,
		TotalAffectedItems: totalAffectedItems,
	}

	respondJSONOK(w, analysis)
}

// AnalyzeComprehensiveMigration analyzes all migration dimensions when moving a workspace to a new config set
// It compares item types, custom fields, priorities, and statuses between old and new configuration sets
func (h *ConfigurationSetHandler) AnalyzeComprehensiveMigration(w http.ResponseWriter, r *http.Request) {
	targetConfigSetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// workspace_id is required for comprehensive migration
	workspaceIDStr := r.URL.Query().Get("workspace_id")
	if workspaceIDStr == "" {
		respondBadRequest(w, r, "workspace_id query parameter is required")
		return
	}
	workspaceID, err := strconv.Atoi(workspaceIDStr)
	if err != nil {
		respondBadRequest(w, r, "Invalid workspace_id parameter")
		return
	}

	// Get workspace's current configuration set
	var sourceConfigSetID sql.NullInt64
	var sourceConfigSetName string
	err = h.db.QueryRow(`
		SELECT wcs.configuration_set_id, COALESCE(cs.name, '') as config_set_name
		FROM workspace_configuration_sets wcs
		LEFT JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
		WHERE wcs.workspace_id = ?
	`, workspaceID).Scan(&sourceConfigSetID, &sourceConfigSetName)

	if err == sql.ErrNoRows {
		// Workspace has no config set assigned - treat source as having no restrictions
		// But still need to check if items are compatible with target restrictions
		sourceConfigSetID = sql.NullInt64{Int64: 0, Valid: false}
		sourceConfigSetName = "(No Configuration Set)"
		// Don't return early - continue with migration analysis
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// If workspace is already assigned to the target config set, no migration needed
	if sourceConfigSetID.Valid && int(sourceConfigSetID.Int64) == targetConfigSetID {
		analysis := models.ComprehensiveMigrationAnalysis{
			OldConfigSetID:     targetConfigSetID,
			NewConfigSetID:     targetConfigSetID,
			AffectedWorkspaces: []int{workspaceID},
			RequiresMigration:  false,
		}
		respondJSONOK(w, analysis)
		return
	}

	sourceID := int(sourceConfigSetID.Int64)

	// Get target configuration set details
	var targetConfigSetName string
	var targetWorkflowID sql.NullInt64
	var targetWorkflowName sql.NullString
	err = h.db.QueryRow(`
		SELECT cs.name, cs.workflow_id, wf.name
		FROM configuration_sets cs
		LEFT JOIN workflows wf ON cs.workflow_id = wf.id
		WHERE cs.id = ?
	`, targetConfigSetID).Scan(&targetConfigSetName, &targetWorkflowID, &targetWorkflowName)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "Target configuration set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Count total items in workspace
	var totalItems int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM items WHERE workspace_id = ?`, workspaceID).Scan(&totalItems); err != nil {
		slog.Warn("failed to get total items count for migration analysis", slog.Any("error", err))
	}

	// Initialize analysis
	analysis := models.ComprehensiveMigrationAnalysis{
		OldConfigSetID:     sourceID,
		OldConfigSetName:   sourceConfigSetName,
		NewConfigSetID:     targetConfigSetID,
		NewConfigSetName:   targetConfigSetName,
		AffectedWorkspaces: []int{workspaceID},
		TotalAffectedItems: totalItems,
		NewWorkflowID:      utils.NullInt64ToPtr(targetWorkflowID),
		NewWorkflowName:    targetWorkflowName.String,
	}

	// Analyze item types
	itemTypeMigrations, availableItemTypes, requiresItemTypeMigration := h.analyzeItemTypeMigration(workspaceID, sourceID, targetConfigSetID)
	analysis.ItemTypeMigrations = itemTypeMigrations
	analysis.AvailableItemTypes = availableItemTypes
	analysis.RequiresItemTypeMigration = requiresItemTypeMigration

	// Analyze custom fields
	customFieldMigrations, requiresFieldMigration := h.analyzeCustomFieldMigration(workspaceID, sourceID, targetConfigSetID)
	analysis.CustomFieldMigrations = customFieldMigrations
	analysis.RequiresFieldMigration = requiresFieldMigration

	// Analyze priorities
	priorityMigrations, availablePriorities, requiresPriorityMigration := h.analyzePriorityMigration(workspaceID, sourceID, targetConfigSetID)
	analysis.PriorityMigrations = priorityMigrations
	analysis.AvailablePriorities = availablePriorities
	analysis.RequiresPriorityMigration = requiresPriorityMigration

	// Analyze statuses (reuse existing logic)
	statusMigrations, requiresStatusMigration := h.analyzeStatusMigration(workspaceID, targetConfigSetID)
	analysis.StatusMigrations = statusMigrations
	analysis.RequiresStatusMigration = requiresStatusMigration

	// Overall migration flag
	analysis.RequiresMigration = requiresItemTypeMigration || requiresFieldMigration ||
		requiresPriorityMigration || requiresStatusMigration

	respondJSONOK(w, analysis)
}

// analyzeItemTypeMigration compares item types between source and target config sets
func (h *ConfigurationSetHandler) analyzeItemTypeMigration(workspaceID, sourceConfigSetID, targetConfigSetID int) ([]models.ItemTypeMigrationInfo, []models.ItemTypeTarget, bool) {
	// Get item types in source config set
	sourceItemTypes := make(map[int]string)
	rows, err := h.db.Query(`
		SELECT it.id, it.name
		FROM configuration_set_item_types csit
		JOIN item_types it ON csit.item_type_id = it.id
		WHERE csit.configuration_set_id = ?
	`, sourceConfigSetID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id int
			var name string
			_ = rows.Scan(&id, &name)
			sourceItemTypes[id] = name
		}
	}

	// Get item types in target config set
	targetItemTypes := make(map[int]models.ItemTypeTarget)
	var availableTargets []models.ItemTypeTarget
	rows, err = h.db.Query(`
		SELECT it.id, it.name, it.icon, it.color, it.hierarchy_level
		FROM configuration_set_item_types csit
		JOIN item_types it ON csit.item_type_id = it.id
		WHERE csit.configuration_set_id = ?
		ORDER BY it.hierarchy_level, it.sort_order
	`, targetConfigSetID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var target models.ItemTypeTarget
			_ = rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.HierarchyLevel)
			targetItemTypes[target.ID] = target
			availableTargets = append(availableTargets, target)
		}
	}

	// If target config set has no explicit item types, it accepts ALL item types
	if len(availableTargets) == 0 {
		rows, err = h.db.Query(`
			SELECT id, name, icon, color, hierarchy_level
			FROM item_types
			ORDER BY hierarchy_level, sort_order
		`)
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var target models.ItemTypeTarget
				_ = rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.HierarchyLevel)
				targetItemTypes[target.ID] = target
				availableTargets = append(availableTargets, target)
			}
		}
	}

	// Build map by normalized name for suggestion matching
	targetByName := make(map[string]models.ItemTypeTarget)
	for _, t := range targetItemTypes {
		targetByName[strings.ToLower(t.Name)] = t
	}

	// Count items by type in workspace
	var migrations []models.ItemTypeMigrationInfo
	requiresMigration := false

	rows, err = h.db.Query(`
		SELECT COALESCE(i.item_type_id, 0) as type_id,
		       COALESCE(it.name, '(No Type)') as type_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.workspace_id = ?
		GROUP BY i.item_type_id, it.name
		ORDER BY it.name
	`, workspaceID)
	if err != nil {
		return migrations, availableTargets, false
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var typeID int
		var typeName string
		var itemCount int
		_ = rows.Scan(&typeID, &typeName, &itemCount)

		migration := models.ItemTypeMigrationInfo{
			CurrentItemTypeName: typeName,
			ItemCount:           itemCount,
			AvailableTargets:    availableTargets,
		}

		if typeID == 0 {
			migration.CurrentItemTypeID = nil
		} else {
			migration.CurrentItemTypeID = &typeID
		}

		// Check if type exists in target by ID
		if target, exists := targetItemTypes[typeID]; exists {
			migration.SuggestedItemTypeID = &target.ID
			migration.SuggestedItemTypeName = target.Name
			migration.RequiresMigration = false
		} else if target, exists := targetByName[strings.ToLower(typeName)]; exists {
			// Match by name
			migration.SuggestedItemTypeID = &target.ID
			migration.SuggestedItemTypeName = target.Name
			migration.RequiresMigration = false
		} else if typeID == 0 && len(availableTargets) > 0 {
			// No type set - suggest first available
			migration.SuggestedItemTypeID = &availableTargets[0].ID
			migration.SuggestedItemTypeName = availableTargets[0].Name
			migration.RequiresMigration = true
			requiresMigration = true
		} else if typeID != 0 {
			// Type not in target config set
			migration.RequiresMigration = true
			requiresMigration = true
		}

		migrations = append(migrations, migration)
	}

	return migrations, availableTargets, requiresMigration
}

// analyzeCustomFieldMigration compares custom fields between source and target screens
func (h *ConfigurationSetHandler) analyzeCustomFieldMigration(workspaceID, sourceConfigSetID, targetConfigSetID int) ([]models.CustomFieldMigrationInfo, bool) {
	// Get custom fields from source config set screens
	sourceFields := make(map[int]struct {
		name      string
		fieldType string
	})
	rows, err := h.db.Query(`
		SELECT DISTINCT cfd.id, cfd.name, cfd.field_type
		FROM configuration_set_screens css
		JOIN screen_fields sf ON css.screen_id = sf.screen_id
		JOIN custom_field_definitions cfd ON sf.field_type = 'custom'
			AND (CASE WHEN sf.field_type = 'custom' THEN CAST(sf.field_identifier AS INTEGER) END) = cfd.id
		WHERE css.configuration_set_id = ?
	`, sourceConfigSetID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id int
			var name, fieldType string
			_ = rows.Scan(&id, &name, &fieldType)
			sourceFields[id] = struct {
				name      string
				fieldType string
			}{name, fieldType}
		}
	}

	// Get custom fields from target config set screens
	targetFields := make(map[int]struct {
		name      string
		fieldType string
		required  bool
	})
	rows, err = h.db.Query(`
		SELECT DISTINCT cfd.id, cfd.name, cfd.field_type, sf.is_required
		FROM configuration_set_screens css
		JOIN screen_fields sf ON css.screen_id = sf.screen_id
		JOIN custom_field_definitions cfd ON sf.field_type = 'custom'
			AND (CASE WHEN sf.field_type = 'custom' THEN CAST(sf.field_identifier AS INTEGER) END) = cfd.id
		WHERE css.configuration_set_id = ?
	`, targetConfigSetID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id int
			var name, fieldType string
			var required bool
			_ = rows.Scan(&id, &name, &fieldType, &required)
			targetFields[id] = struct {
				name      string
				fieldType string
				required  bool
			}{name, fieldType, required}
		}
	}

	var migrations []models.CustomFieldMigrationInfo
	requiresMigration := false

	// Count items with values for each source field
	fieldValueCounts := make(map[int]int)
	rows, err = h.db.Query(`
		SELECT custom_field_values FROM items
		WHERE workspace_id = ?
		AND custom_field_values IS NOT NULL
		AND custom_field_values != ''
		AND custom_field_values != '{}'
	`, workspaceID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var cfvJSON string
			_ = rows.Scan(&cfvJSON)
			var cfv map[string]interface{}
			if json.Unmarshal([]byte(cfvJSON), &cfv) == nil {
				for key := range cfv {
					if fieldID, err := strconv.Atoi(key); err == nil {
						fieldValueCounts[fieldID]++
					}
				}
			}
		}
	}

	// Analyze each source field
	for fieldID, sourceField := range sourceFields {
		if _, existsInTarget := targetFields[fieldID]; existsInTarget {
			// Field exists in both - keep
			migrations = append(migrations, models.CustomFieldMigrationInfo{
				FieldID:   fieldID,
				FieldName: sourceField.name,
				FieldType: sourceField.fieldType,
				ItemCount: fieldValueCounts[fieldID],
				Action:    "keep",
			})
		} else {
			// Field in source but not target - orphan (data preserved but hidden)
			migrations = append(migrations, models.CustomFieldMigrationInfo{
				FieldID:   fieldID,
				FieldName: sourceField.name,
				FieldType: sourceField.fieldType,
				ItemCount: fieldValueCounts[fieldID],
				Action:    "orphan",
			})
		}
	}

	// Check for new required fields in target that aren't in source
	for fieldID, targetField := range targetFields {
		if _, existsInSource := sourceFields[fieldID]; !existsInSource && targetField.required {
			migrations = append(migrations, models.CustomFieldMigrationInfo{
				FieldID:         fieldID,
				FieldName:       targetField.name,
				FieldType:       targetField.fieldType,
				ItemCount:       0,
				Action:          "add_default",
				RequiresDefault: true,
			})
			requiresMigration = true
		}
	}

	return migrations, requiresMigration
}

// analyzePriorityMigration compares priorities between source and target config sets
func (h *ConfigurationSetHandler) analyzePriorityMigration(workspaceID, sourceConfigSetID, targetConfigSetID int) ([]models.PriorityMigrationInfo, []models.PriorityTarget, bool) {
	// Get priorities in source config set
	sourcePriorities := make(map[int]string)
	rows, err := h.db.Query(`
		SELECT p.id, p.name
		FROM configuration_set_priorities csp
		JOIN priorities p ON csp.priority_id = p.id
		WHERE csp.configuration_set_id = ?
	`, sourceConfigSetID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id int
			var name string
			_ = rows.Scan(&id, &name)
			sourcePriorities[id] = name
		}
	}

	// Get priorities in target config set
	targetPriorities := make(map[int]models.PriorityTarget)
	var availableTargets []models.PriorityTarget
	rows, err = h.db.Query(`
		SELECT p.id, p.name, p.icon, p.color, p.sort_order
		FROM configuration_set_priorities csp
		JOIN priorities p ON csp.priority_id = p.id
		WHERE csp.configuration_set_id = ?
		ORDER BY p.sort_order
	`, targetConfigSetID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var target models.PriorityTarget
			_ = rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.SortOrder)
			targetPriorities[target.ID] = target
			availableTargets = append(availableTargets, target)
		}
	}

	// If target config set has no explicit priorities, it accepts ALL priorities
	if len(availableTargets) == 0 {
		rows, err = h.db.Query(`
			SELECT id, name, icon, color, sort_order
			FROM priorities
			ORDER BY sort_order
		`)
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var target models.PriorityTarget
				_ = rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.SortOrder)
				targetPriorities[target.ID] = target
				availableTargets = append(availableTargets, target)
			}
		}
	}

	// Build map by normalized name for suggestion matching
	targetByName := make(map[string]models.PriorityTarget)
	for _, t := range targetPriorities {
		targetByName[strings.ToLower(t.Name)] = t
	}

	// Count items by priority in workspace
	var migrations []models.PriorityMigrationInfo
	requiresMigration := false

	rows, err = h.db.Query(`
		SELECT COALESCE(i.priority_id, 0) as priority_id,
		       COALESCE(p.name, '(No Priority)') as priority_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN priorities p ON i.priority_id = p.id
		WHERE i.workspace_id = ?
		GROUP BY i.priority_id, p.name
		ORDER BY p.name
	`, workspaceID)
	if err != nil {
		return migrations, availableTargets, false
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var priorityID int
		var priorityName string
		var itemCount int
		_ = rows.Scan(&priorityID, &priorityName, &itemCount)

		migration := models.PriorityMigrationInfo{
			CurrentPriorityName: priorityName,
			ItemCount:           itemCount,
		}

		if priorityID == 0 {
			migration.CurrentPriorityID = nil
		} else {
			migration.CurrentPriorityID = &priorityID
		}

		// Check if priority exists in target by ID
		if target, exists := targetPriorities[priorityID]; exists {
			migration.SuggestedPriorityID = &target.ID
			migration.SuggestedPriorityName = target.Name
			migration.RequiresMigration = false
		} else if target, exists := targetByName[strings.ToLower(priorityName)]; exists {
			// Match by name
			migration.SuggestedPriorityID = &target.ID
			migration.SuggestedPriorityName = target.Name
			migration.RequiresMigration = false
		} else if priorityID == 0 {
			// No priority set - this is ok, don't require migration
			migration.RequiresMigration = false
		} else {
			// Priority not in target config set
			migration.RequiresMigration = true
			requiresMigration = true
		}

		migrations = append(migrations, migration)
	}

	return migrations, availableTargets, requiresMigration
}

// analyzeStatusMigration analyzes status migration for a workspace moving to a new config set
func (h *ConfigurationSetHandler) analyzeStatusMigration(workspaceID, targetConfigSetID int) ([]models.StatusMigrationInfo, bool) {
	// Get target workflow
	var targetWorkflowID sql.NullInt64
	_ = h.db.QueryRow(`
		SELECT workflow_id FROM configuration_sets WHERE id = ?
	`, targetConfigSetID).Scan(&targetWorkflowID)

	if !targetWorkflowID.Valid {
		// No workflow configured - no status migration needed
		return nil, false
	}

	// Get available statuses in target workflow
	workflowStatuses := make(map[string]models.Status)
	rows, err := h.db.Query(`
		SELECT DISTINCT s.id, s.name
		FROM workflow_transitions wt
		JOIN statuses s ON (wt.from_status_id = s.id OR wt.to_status_id = s.id)
		WHERE wt.workflow_id = ?
	`, targetWorkflowID.Int64)
	if err != nil {
		return nil, false
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var status models.Status
		_ = rows.Scan(&status.ID, &status.Name)
		normalizedName := normalizeStatusName(status.Name)
		workflowStatuses[normalizedName] = status
	}

	// Get current statuses used in workspace
	var migrations []models.StatusMigrationInfo
	requiresMigration := false

	rows, err = h.db.Query(`
		SELECT COALESCE(s.id, 0) as status_id,
		       COALESCE(s.name, '') as status_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE i.workspace_id = ?
		GROUP BY s.id, s.name
		ORDER BY s.name
	`, workspaceID)
	if err != nil {
		return nil, false
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var statusID int
		var statusName string
		var itemCount int
		_ = rows.Scan(&statusID, &statusName, &itemCount)

		migration := models.StatusMigrationInfo{
			CurrentStatus:   statusName,
			CurrentStatusID: &statusID,
			ItemCount:       itemCount,
		}

		normalizedStatus := normalizeStatusName(statusName)
		if status, exists := workflowStatuses[normalizedStatus]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
			migration.RequiresMigration = false
		} else {
			migration.RequiresMigration = true
			requiresMigration = true
			h.suggestStatusMapping(&migration, normalizedStatus, workflowStatuses)
		}

		migrations = append(migrations, migration)
	}

	return migrations, requiresMigration
}

// suggestStatusMapping suggests a reasonable default status based on common status mappings
func (h *ConfigurationSetHandler) suggestStatusMapping(migration *models.StatusMigrationInfo, normalizedCurrentStatus string, workflowStatuses map[string]models.Status) {
	switch normalizedCurrentStatus {
	case "open", "new", "to do", "todo":
		if status, exists := workflowStatuses["to do"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		} else if status, exists := workflowStatuses["open"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		}
	case "in progress", "doing":
		if status, exists := workflowStatuses["in progress"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		}
	case "completed", "done", "closed":
		if status, exists := workflowStatuses["done"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		} else if status, exists := workflowStatuses["completed"]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		}
	case "cancelled", "canceled": //nolint:misspell // British spelling used in database
		if status, exists := workflowStatuses["cancelled"]; exists { //nolint:misspell // British spelling used in database
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
		}
	}
}
