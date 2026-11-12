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

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

func (h *ConfigurationSetHandler) requireMigrationAnalysisAccess(w http.ResponseWriter, r *http.Request, workspaceIDs []int) bool {
	if h.permissionService == nil {
		return true
	}
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return false
	}
	for _, workspaceID := range workspaceIDs {
		if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
			return false
		}
	}
	return true
}

// normalizeStatusName normalizes status names for comparison by replacing underscores with spaces and converting to lowercase
func normalizeStatusName(name string) string {
	normalized := strings.ReplaceAll(name, "_", " ")
	return strings.ToLower(normalized)
}

// Migration Assistant endpoints

func (h *ConfigurationSetHandler) AnalyzeMigration(w http.ResponseWriter, r *http.Request) {
	configSetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Resolve the configuration set and its affected workspaces.
	var configSet models.ConfigurationSet
	var workflowName sql.NullString
	err := h.db.QueryRow(`
		SELECT cs.id, cs.name, cs.workflow_id, cs.differentiate_by_item_type, wf.name as workflow_name
		FROM configuration_sets cs
		LEFT JOIN workflows wf ON cs.workflow_id = wf.id
		WHERE cs.id = ?
	`, configSetID).Scan(&configSet.ID, &configSet.Name, &configSet.WorkflowID, &configSet.DifferentiateByItemType, &workflowName)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "Configuration set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	configSet.WorkflowName = workflowName.String

	workspaceFilter := r.URL.Query().Get("workspace_id")
	var affectedWorkspaces []int

	if workspaceFilter != "" {
		workspaceID, err := strconv.Atoi(workspaceFilter)
		if err != nil {
			respondBadRequest(w, r, "Invalid workspace_id parameter")
			return
		}
		exists, err := h.repo.WorkspaceExists(workspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondNotFound(w, r, "Workspace")
			return
		}
		affectedWorkspaces = []int{workspaceID}
	} else {
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
		if err := workspaceRows.Err(); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if !h.requireMigrationAnalysisAccess(w, r, affectedWorkspaces) {
		return
	}

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

	workflowService := services.NewWorkflowService(h.db)

	var statusMigrations []models.StatusMigrationInfo
	totalAffectedItems := 0
	requiresMigration := false

	// With item-type differentiation, resolve each type's workflow separately.
	if configSet.DifferentiateByItemType {
		rowsByType, err := repository.NewItemRepository(h.db).ListItemTypeStatusCountsForWorkspaces(affectedWorkspaces)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		workflowStatusesCache := make(map[int]map[string]models.Status)

		for _, row := range rowsByType {
			itemTypeName := row.ItemTypeName
			currentStatusID := row.StatusID
			currentStatusName := row.StatusName
			itemCount := row.ItemCount

			totalAffectedItems += itemCount

			itemTypeIDPtr := row.ItemTypeID

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

			if workflowID == nil {
				statusMigrations = append(statusMigrations, migration)
				continue
			}

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
				if err := workflowStatusRows.Err(); err != nil {
					_ = workflowStatusRows.Close()
					respondInternalError(w, r, err)
					return
				}
				_ = workflowStatusRows.Close()

				workflowStatusesCache[*workflowID] = workflowStatuses
			}

			normalizedCurrentStatus := normalizeStatusName(currentStatusName)
			if workflowStatus, exists := workflowStatuses[normalizedCurrentStatus]; exists {
				migration.SuggestedStatusID = &workflowStatus.ID
				migration.SuggestedStatusName = workflowStatus.Name
				if migration.CurrentStatusID == nil || *migration.CurrentStatusID != workflowStatus.ID {
					migration.RequiresMigration = true
					requiresMigration = true
				}
			} else {
				migration.RequiresMigration = true
				requiresMigration = true
				h.suggestStatusMapping(&migration, normalizedCurrentStatus, workflowStatuses)
			}

			statusMigrations = append(statusMigrations, migration)
		}
	} else {
		// Without differentiation, reconcile against the config-set workflow.
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

		statusCounts, err := repository.NewItemRepository(h.db).ListStatusCountsForWorkspaces(affectedWorkspaces)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

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
		if err := workflowStatusRows.Err(); err != nil {
			respondInternalError(w, r, err)
			return
		}

		for _, row := range statusCounts {
			currentStatusID := row.StatusID
			currentStatusName := row.StatusName
			itemCount := row.ItemCount

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
				if migration.CurrentStatusID == nil || *migration.CurrentStatusID != workflowStatus.ID {
					migration.RequiresMigration = true
					requiresMigration = true
				}
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
	workspaceExists, err := h.repo.WorkspaceExists(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !workspaceExists {
		respondNotFound(w, r, "Workspace")
		return
	}

	if !h.requireMigrationAnalysisAccess(w, r, []int{workspaceID}) {
		return
	}

	// Get workspace's current explicit configuration set.
	var sourceConfigSetID sql.NullInt64
	var sourceConfigSetName string
	err = h.db.QueryRow(`
		SELECT wcs.configuration_set_id, COALESCE(cs.name, '') as config_set_name
		FROM workspace_configuration_sets wcs
		LEFT JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
		WHERE wcs.workspace_id = ?
	`, workspaceID).Scan(&sourceConfigSetID, &sourceConfigSetName)

	if errors.Is(err, sql.ErrNoRows) {
		// No explicit assignment means the default configuration set is applied
		// automatically. Keep 0 as the assignment CAS sentinel, but analyze from
		// the effective default set below.
		sourceConfigSetID = sql.NullInt64{Int64: 0, Valid: false}
		sourceConfigSetName = "(No Configuration Set)"
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
	effectiveSourceID, effectiveSourceName, err := h.resolveMigrationSourceConfigSet(sourceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if sourceID == 0 {
		sourceConfigSetName = effectiveSourceName
	}

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
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "Target configuration set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Count total items in workspace
	totalItems, err := repository.NewItemRepository(h.db).CountByField("workspace_id", workspaceID)
	if err != nil {
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
	itemTypeMigrations, availableItemTypes, requiresItemTypeMigration, err := h.analyzeItemTypeMigration(workspaceID, targetConfigSetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	analysis.ItemTypeMigrations = itemTypeMigrations
	analysis.AvailableItemTypes = availableItemTypes
	analysis.RequiresItemTypeMigration = requiresItemTypeMigration

	// Analyze custom fields
	customFieldMigrations, requiresFieldMigration, err := h.analyzeCustomFieldMigration(workspaceID, effectiveSourceID, targetConfigSetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	analysis.CustomFieldMigrations = customFieldMigrations
	analysis.RequiresFieldMigration = requiresFieldMigration

	// Analyze priorities
	priorityMigrations, availablePriorities, requiresPriorityMigration, err := h.analyzePriorityMigration(workspaceID, targetConfigSetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	analysis.PriorityMigrations = priorityMigrations
	analysis.AvailablePriorities = availablePriorities
	analysis.RequiresPriorityMigration = requiresPriorityMigration

	// Analyze statuses against the target configuration's effective workflows.
	statusMigrations, requiresStatusMigration, err := h.analyzeStatusMigration(workspaceID, targetConfigSetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	analysis.StatusMigrations = statusMigrations
	analysis.RequiresStatusMigration = requiresStatusMigration

	// Overall migration flag
	analysis.RequiresMigration = requiresItemTypeMigration || requiresFieldMigration ||
		requiresPriorityMigration || requiresStatusMigration

	respondJSONOK(w, analysis)
}

// analyzeItemTypeMigration compares item types between source and target config sets
func (h *ConfigurationSetHandler) analyzeItemTypeMigration(workspaceID, targetConfigSetID int) ([]models.ItemTypeMigrationInfo, []models.ItemTypeTarget, bool, error) {
	// Get item types in target config set
	targetItemTypes := make(map[int]models.ItemTypeTarget)
	var availableTargets []models.ItemTypeTarget
	rows, err := h.db.Query(`
		SELECT it.id, it.name, it.icon, it.color, it.hierarchy_level
		FROM configuration_set_item_types csit
		JOIN item_types it ON csit.item_type_id = it.id
		WHERE csit.configuration_set_id = ?
		ORDER BY CASE WHEN it.hierarchy_level = -1 THEN 1 ELSE 0 END, it.hierarchy_level, it.sort_order
	`, targetConfigSetID)
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var target models.ItemTypeTarget
		if err := rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.HierarchyLevel); err != nil {
			return nil, nil, false, err
		}
		targetItemTypes[target.ID] = target
		availableTargets = append(availableTargets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, err
	}

	// If target config set has no explicit item types, it accepts ALL item types
	if len(availableTargets) == 0 {
		rows, err = h.db.Query(`
			SELECT id, name, icon, color, hierarchy_level
			FROM item_types
			ORDER BY CASE WHEN hierarchy_level = -1 THEN 1 ELSE 0 END, hierarchy_level, sort_order
		`)
		if err != nil {
			return nil, nil, false, err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var target models.ItemTypeTarget
			if err := rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.HierarchyLevel); err != nil {
				return nil, nil, false, err
			}
			targetItemTypes[target.ID] = target
			availableTargets = append(availableTargets, target)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, false, err
		}
	}

	// Build map by normalized name for suggestion matching
	targetByNameAndLevel := make(map[string]models.ItemTypeTarget)
	targetsByLevel := make(map[int][]models.ItemTypeTarget)
	for _, t := range targetItemTypes {
		targetByNameAndLevel[fmt.Sprintf("%d:%s", t.HierarchyLevel, strings.ToLower(t.Name))] = t
		targetsByLevel[t.HierarchyLevel] = append(targetsByLevel[t.HierarchyLevel], t)
	}

	// Count items by type in workspace
	var migrations []models.ItemTypeMigrationInfo
	requiresMigration := false

	typeCounts, err := repository.NewItemRepository(h.db).ListItemTypeCountsForWorkspace(workspaceID)
	if err != nil {
		return nil, nil, false, err
	}

	for _, tc := range typeCounts {
		typeID := tc.TypeID
		typeName := tc.TypeName
		itemCount := tc.ItemCount

		mappingTargets := availableTargets
		sourceLevel := 0
		if typeID != 0 {
			if err := h.db.QueryRow(`SELECT hierarchy_level FROM item_types WHERE id = ?`, typeID).Scan(&sourceLevel); err != nil {
				return nil, nil, false, err
			}
			mappingTargets = append([]models.ItemTypeTarget{}, targetsByLevel[sourceLevel]...)
		}

		migration := models.ItemTypeMigrationInfo{
			CurrentItemTypeName: typeName,
			ItemCount:           itemCount,
			AvailableTargets:    mappingTargets,
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
		} else if target, exists := targetByNameAndLevel[fmt.Sprintf("%d:%s", sourceLevel, strings.ToLower(typeName))]; exists {
			// A same-name target is a useful suggestion, but the persisted item
			// still references the disallowed source ID and must be remapped.
			migration.SuggestedItemTypeID = &target.ID
			migration.SuggestedItemTypeName = target.Name
			migration.RequiresMigration = true
			requiresMigration = true
		} else if typeID == 0 && len(mappingTargets) > 0 {
			// No type set - suggest first available
			migration.SuggestedItemTypeID = &mappingTargets[0].ID
			migration.SuggestedItemTypeName = mappingTargets[0].Name
			migration.RequiresMigration = true
			requiresMigration = true
		} else if typeID != 0 {
			// Type not in target config set
			migration.RequiresMigration = true
			requiresMigration = true
		}

		migrations = append(migrations, migration)
	}

	return migrations, availableTargets, requiresMigration, nil
}

// analyzeCustomFieldMigration compares custom fields between source and target screens
func (h *ConfigurationSetHandler) analyzeCustomFieldMigration(workspaceID, sourceConfigSetID, targetConfigSetID int) ([]models.CustomFieldMigrationInfo, bool, error) {
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
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var name, fieldType string
		if err := rows.Scan(&id, &name, &fieldType); err != nil {
			return nil, false, err
		}
		sourceFields[id] = struct {
			name      string
			fieldType string
		}{name, fieldType}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
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
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var name, fieldType string
		var required bool
		if err := rows.Scan(&id, &name, &fieldType, &required); err != nil {
			return nil, false, err
		}
		targetFields[id] = struct {
			name      string
			fieldType string
			required  bool
		}{name, fieldType, required}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	var migrations []models.CustomFieldMigrationInfo
	requiresMigration := false

	// Count items with values for each source field
	fieldValueCounts := make(map[int]int)
	cfvJSONs, err := repository.NewItemRepository(h.db).ListNonEmptyCustomFieldJSONForWorkspace(workspaceID)
	if err != nil {
		return nil, false, err
	}
	for _, cfvJSON := range cfvJSONs {
		var cfv map[string]any
		if err := json.Unmarshal([]byte(cfvJSON), &cfv); err != nil {
			return nil, false, err
		}
		for key := range cfv {
			if fieldID, err := strconv.Atoi(key); err == nil {
				fieldValueCounts[fieldID]++
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

	return migrations, requiresMigration, nil
}

// analyzePriorityMigration compares priorities between source and target config sets
func (h *ConfigurationSetHandler) analyzePriorityMigration(workspaceID, targetConfigSetID int) ([]models.PriorityMigrationInfo, []models.PriorityTarget, bool, error) {
	// Get priorities in target config set
	targetPriorities := make(map[int]models.PriorityTarget)
	var availableTargets []models.PriorityTarget
	rows, err := h.db.Query(`
		SELECT p.id, p.name, p.icon, p.color, p.sort_order
		FROM configuration_set_priorities csp
		JOIN priorities p ON csp.priority_id = p.id
		WHERE csp.configuration_set_id = ?
		ORDER BY p.sort_order
	`, targetConfigSetID)
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var target models.PriorityTarget
		if err := rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.SortOrder); err != nil {
			return nil, nil, false, err
		}
		targetPriorities[target.ID] = target
		availableTargets = append(availableTargets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, err
	}

	// If target config set has no explicit priorities, it accepts ALL priorities
	if len(availableTargets) == 0 {
		rows, err = h.db.Query(`
			SELECT id, name, icon, color, sort_order
			FROM priorities
			ORDER BY sort_order
		`)
		if err != nil {
			return nil, nil, false, err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var target models.PriorityTarget
			if err := rows.Scan(&target.ID, &target.Name, &target.Icon, &target.Color, &target.SortOrder); err != nil {
				return nil, nil, false, err
			}
			targetPriorities[target.ID] = target
			availableTargets = append(availableTargets, target)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, false, err
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

	priorityCounts, err := repository.NewItemRepository(h.db).ListPriorityCountsForWorkspace(workspaceID)
	if err != nil {
		return nil, nil, false, err
	}

	for _, pc := range priorityCounts {
		priorityID := pc.PriorityID
		priorityName := pc.PriorityName
		itemCount := pc.ItemCount

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
			// Matching names do not make different priority IDs interchangeable.
			migration.SuggestedPriorityID = &target.ID
			migration.SuggestedPriorityName = target.Name
			migration.RequiresMigration = true
			requiresMigration = true
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

	return migrations, availableTargets, requiresMigration, nil
}

// analyzeStatusMigration analyzes status migration using the target configuration
// set's effective workflow for each item type, including item-type overrides
// and the global default workflow fallback.
func (h *ConfigurationSetHandler) analyzeStatusMigration(workspaceID, targetConfigSetID int) ([]models.StatusMigrationInfo, bool, error) {
	return h.analyzeStatusMigrationForConfigSet([]int{workspaceID}, targetConfigSetID, nil)
}

// analyzeStatusMigrationForConfigSet compares item statuses with the workflows
// they will use after the configuration change. overrideDefaultWorkflowID is
// used by an intra-set default-workflow change that has not been persisted yet;
// item-type workflow overrides still take precedence.
func (h *ConfigurationSetHandler) analyzeStatusMigrationForConfigSet(
	workspaceIDs []int,
	targetConfigSetID int,
	overrideDefaultWorkflowID *int,
) ([]models.StatusMigrationInfo, bool, error) {
	statusCounts, err := repository.NewItemRepository(h.db).ListItemTypeStatusCountsForWorkspaces(workspaceIDs)
	if err != nil {
		return nil, false, err
	}

	workflowStatusesCache := make(map[int]map[string]models.Status)
	migrations := make([]models.StatusMigrationInfo, 0, len(statusCounts))
	requiresMigration := false

	for _, row := range statusCounts {
		workflowID, err := h.resolveConfigSetWorkflowID(targetConfigSetID, row.ItemTypeID, overrideDefaultWorkflowID)
		if err != nil {
			return nil, false, err
		}

		statusID := row.StatusID
		migration := models.StatusMigrationInfo{
			CurrentStatus:     row.StatusName,
			ItemTypeID:        row.ItemTypeID,
			ItemTypeName:      row.ItemTypeName,
			RequiresMigration: false,
			ItemCount:         row.ItemCount,
		}
		if statusID != 0 {
			migration.CurrentStatusID = &statusID
		}

		// No workflow at any level means statuses are unrestricted.
		if workflowID == nil {
			migrations = append(migrations, migration)
			continue
		}

		workflowStatuses, ok := workflowStatusesCache[*workflowID]
		if !ok {
			workflowStatuses, err = h.loadWorkflowStatuses(*workflowID)
			if err != nil {
				return nil, false, err
			}
			workflowStatusesCache[*workflowID] = workflowStatuses
		}

		normalizedStatus := normalizeStatusName(row.StatusName)
		if status, exists := workflowStatuses[normalizedStatus]; exists {
			migration.SuggestedStatusID = &status.ID
			migration.SuggestedStatusName = status.Name
			if migration.CurrentStatusID == nil || *migration.CurrentStatusID != status.ID {
				migration.RequiresMigration = true
				requiresMigration = true
			}
		} else {
			migration.RequiresMigration = true
			requiresMigration = true
			h.suggestStatusMapping(&migration, normalizedStatus, workflowStatuses)
		}
		migrations = append(migrations, migration)
	}

	return migrations, requiresMigration, nil
}

func (h *ConfigurationSetHandler) resolveConfigSetWorkflowID(
	configSetID int,
	itemTypeID *int,
	overrideDefaultWorkflowID *int,
) (*int, error) {
	var configSetWorkflowID, itemTypeWorkflowID sql.NullInt64
	if itemTypeID == nil {
		if err := h.db.QueryRow(`
			SELECT workflow_id FROM configuration_sets WHERE id = ?
		`, configSetID).Scan(&configSetWorkflowID); err != nil {
			return nil, err
		}
	} else {
		if err := h.db.QueryRow(`
			SELECT cs.workflow_id, csit.workflow_id
			FROM configuration_sets cs
			LEFT JOIN configuration_set_item_types csit
			  ON csit.configuration_set_id = cs.id AND csit.item_type_id = ?
			WHERE cs.id = ?
		`, *itemTypeID, configSetID).Scan(&configSetWorkflowID, &itemTypeWorkflowID); err != nil {
			return nil, err
		}
	}

	if itemTypeWorkflowID.Valid {
		id := int(itemTypeWorkflowID.Int64)
		return &id, nil
	}
	if overrideDefaultWorkflowID != nil {
		id := *overrideDefaultWorkflowID
		return &id, nil
	}
	if configSetWorkflowID.Valid {
		id := int(configSetWorkflowID.Int64)
		return &id, nil
	}

	var globalDefaultID int
	if err := h.db.QueryRow(`SELECT id FROM workflows WHERE is_default = true ORDER BY id LIMIT 1`).Scan(&globalDefaultID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &globalDefaultID, nil
}

func (h *ConfigurationSetHandler) loadWorkflowStatuses(workflowID int) (map[string]models.Status, error) {
	rows, err := h.db.Query(`
		SELECT DISTINCT s.id, s.name
		FROM workflow_transitions wt
		JOIN statuses s ON (wt.from_status_id = s.id OR wt.to_status_id = s.id)
		WHERE wt.workflow_id = ?
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	statuses := make(map[string]models.Status)
	for rows.Next() {
		var status models.Status
		if err := rows.Scan(&status.ID, &status.Name); err != nil {
			return nil, err
		}
		statuses[normalizeStatusName(status.Name)] = status
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return statuses, nil
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
