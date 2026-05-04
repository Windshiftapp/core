package handlers

import (
	"database/sql"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// respondIntraSetWorkflowConflictIfNeeded checks whether the items in the
// retained workspaces are compatible with the new workflow, and if not,
// responds 409 with an aggregate analysis across *all* retained workspaces.
// Used for the intra-config-set workflow swap path: a single migration call
// must cover every workspace still attached, since updating workflow_id
// halfway through would orphan the not-yet-migrated workspaces.
func (h *ConfigurationSetHandler) respondIntraSetWorkflowConflictIfNeeded(
	w http.ResponseWriter, r *http.Request, //nolint:unparam // r kept for symmetry with respondMigrationConflictIfNeeded
	configSetID int, retainedWorkspaceIDs []int, newWorkflowID *int,
) bool {
	if newWorkflowID == nil || len(retainedWorkspaceIDs) == 0 {
		return false
	}

	// Reuse the per-workspace analyzer over each retained workspace and merge
	// by (status_id, item_type_id). Counts sum across workspaces; the
	// suggested target is whichever the analyzer proposes — they all see the
	// same target workflow, so suggestions agree.
	type key struct {
		statusID   int  // 0 means NULL
		itemTypeID *int // nil means "no per-item-type filter"
	}
	merged := make(map[key]models.StatusMigrationInfo)
	requires := false
	for _, wsID := range retainedWorkspaceIDs {
		mig, req := h.analyzeStatusMigrationAgainstWorkflow(wsID, *newWorkflowID)
		if req {
			requires = true
		}
		for _, m := range mig {
			sid := 0
			if m.CurrentStatusID != nil {
				sid = *m.CurrentStatusID
			}
			k := key{statusID: sid, itemTypeID: m.ItemTypeID}
			if existing, ok := merged[k]; ok {
				existing.ItemCount += m.ItemCount
				merged[k] = existing
			} else {
				merged[k] = m
			}
		}
	}
	if !requires {
		return false
	}

	statusMigrations := make([]models.StatusMigrationInfo, 0, len(merged))
	for _, v := range merged {
		statusMigrations = append(statusMigrations, v)
	}

	var configSetName string
	_ = h.db.QueryRow(`SELECT name FROM configuration_sets WHERE id = ?`, configSetID).Scan(&configSetName)

	itemRepo := repository.NewItemRepository(h.db)
	totalItems := 0
	for _, wsID := range retainedWorkspaceIDs {
		n, _ := itemRepo.CountByField("workspace_id", wsID)
		totalItems += n
	}

	analysis := models.ComprehensiveMigrationAnalysis{
		OldConfigSetID:          configSetID,
		OldConfigSetName:        configSetName,
		NewConfigSetID:          configSetID,
		NewConfigSetName:        configSetName,
		AffectedWorkspaces:      append([]int{}, retainedWorkspaceIDs...),
		TotalAffectedItems:      totalItems,
		StatusMigrations:        statusMigrations,
		RequiresMigration:       true,
		RequiresStatusMigration: true,
		NewWorkflowID:           newWorkflowID,
	}

	respondJSON(w, http.StatusConflict, map[string]interface{}{
		"error":    "migration_required",
		"message":  "Migration is required before the workflow change can be applied",
		"analysis": analysis,
	})
	return true
}

// resolveEffectiveWorkflowID returns the workflow_id that will actually govern
// items after a configuration_set update — either the explicit value supplied
// by the caller, or the global default workflow when nil. Returns (nil, nil)
// when neither is available; the caller decides whether that's an error.
func (h *ConfigurationSetHandler) resolveEffectiveWorkflowID(explicit *int) (*int, error) {
	if explicit != nil {
		return explicit, nil
	}
	var fallback sql.NullInt64
	err := h.db.QueryRow(`SELECT id FROM workflows WHERE is_default = true LIMIT 1`).Scan(&fallback)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v := int(fallback.Int64)
	return &v, nil
}

// respondMigrationConflictIfNeeded runs the four migration analyzers for a
// single workspace moving from sourceConfigSetID to targetConfigSetID. If any
// dimension requires migration, it writes a 409 Conflict with the analysis
// payload and returns true; the caller should then return immediately.
//
// When overrideTargetWorkflowID is non-nil, status migration is analyzed
// against that workflow instead of the workflow currently persisted on the
// target config set. This is used by Update() to detect intra-config-set
// workflow swaps where the new workflow_id is in the request body but has not
// yet been written to the database.
func (h *ConfigurationSetHandler) respondMigrationConflictIfNeeded(
	w http.ResponseWriter, r *http.Request, //nolint:unparam // r kept for symmetry with HTTP-handler-style helpers
	workspaceID, sourceConfigSetID, targetConfigSetID int,
	overrideTargetWorkflowID *int,
) bool {
	itemTypeMigrations, _, requiresItemTypeMigration := h.analyzeItemTypeMigration(workspaceID, sourceConfigSetID, targetConfigSetID)
	customFieldMigrations, requiresFieldMigration := h.analyzeCustomFieldMigration(workspaceID, sourceConfigSetID, targetConfigSetID)
	priorityMigrations, _, requiresPriorityMigration := h.analyzePriorityMigration(workspaceID, sourceConfigSetID, targetConfigSetID)

	var statusMigrations []models.StatusMigrationInfo
	var requiresStatusMigration bool
	if overrideTargetWorkflowID != nil {
		statusMigrations, requiresStatusMigration = h.analyzeStatusMigrationAgainstWorkflow(workspaceID, *overrideTargetWorkflowID)
	} else {
		statusMigrations, requiresStatusMigration = h.analyzeStatusMigration(workspaceID, targetConfigSetID)
	}

	requiresMigration := requiresItemTypeMigration || requiresFieldMigration ||
		requiresPriorityMigration || requiresStatusMigration
	if !requiresMigration {
		return false
	}

	var sourceConfigSetName, targetConfigSetName string
	_ = h.db.QueryRow(`SELECT name FROM configuration_sets WHERE id = ?`, sourceConfigSetID).Scan(&sourceConfigSetName)
	_ = h.db.QueryRow(`SELECT name FROM configuration_sets WHERE id = ?`, targetConfigSetID).Scan(&targetConfigSetName)

	totalItems, _ := repository.NewItemRepository(h.db).CountByField("workspace_id", workspaceID)

	analysis := models.ComprehensiveMigrationAnalysis{
		OldConfigSetID:            sourceConfigSetID,
		OldConfigSetName:          sourceConfigSetName,
		NewConfigSetID:            targetConfigSetID,
		NewConfigSetName:          targetConfigSetName,
		AffectedWorkspaces:        []int{workspaceID},
		TotalAffectedItems:        totalItems,
		ItemTypeMigrations:        itemTypeMigrations,
		CustomFieldMigrations:     customFieldMigrations,
		PriorityMigrations:        priorityMigrations,
		StatusMigrations:          statusMigrations,
		RequiresMigration:         true,
		RequiresItemTypeMigration: requiresItemTypeMigration,
		RequiresFieldMigration:    requiresFieldMigration,
		RequiresPriorityMigration: requiresPriorityMigration,
		RequiresStatusMigration:   requiresStatusMigration,
	}
	if overrideTargetWorkflowID != nil {
		analysis.NewWorkflowID = overrideTargetWorkflowID
	}

	respondJSON(w, http.StatusConflict, map[string]interface{}{
		"error":    "migration_required",
		"message":  "Migration is required before this configuration set update can be applied",
		"analysis": analysis,
	})
	return true
}

func nullableInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func intersectInts(a, b []int) []int {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	set := make(map[int]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	out := make([]int, 0)
	seen := make(map[int]struct{}, len(b))
	for _, v := range b {
		if _, ok := set[v]; !ok {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
