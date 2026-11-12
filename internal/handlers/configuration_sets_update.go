package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

func (h *ConfigurationSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the old configuration set for audit logging
	oldCS, err := h.repo.FindByID(id)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "configuration_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	cs, fields, ok := decodeJSONWithFields[models.ConfigurationSet](w, r)
	if !ok {
		return
	}
	preserveOmittedConfigurationSetFields(&cs, oldCS, fields)
	sanitize.ApplyAll(
		sanitize.Pair{Target: &cs.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &cs.Description, Policy: sanitize.RichText},
	)

	// Validate required fields
	if strings.TrimSpace(cs.Name) == "" {
		respondValidationError(w, r, "Configuration set name is required")
		return
	}

	// Verify workspaces exist
	for _, workspaceID := range cs.WorkspaceIDs {
		var exists bool
		exists, err = h.repo.WorkspaceExists(workspaceID)
		if err != nil || !exists {
			respondBadRequest(w, r, "One or more workspaces not found")
			return
		}
	}

	// A condition set is bound to a workflow; attaching one from a different
	// workflow would leave its transition bindings unmatched and silently
	// disable the gate.
	if err := h.provisioning.ValidateGovernanceRefs(&cs); err != nil {
		handleServiceError(w, r, err)
		return
	}

	// Snapshot the workspaces currently attached to this config set BEFORE
	// SaveWorkspaceAssignments rewrites the join table. We need this so we can
	// invalidate permission caches for workspaces that are being detached;
	// post-swap lookups won't see them.
	oldWorkspaceIDs, err := h.repo.ListWorkspaceIDsForConfigSet(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Detect whether the default workflow_id is changing in this PUT. When it
	// is, both the cross-set and intra-set migration checks must validate
	// against the *new* workflow — the DB still holds the old value at this
	// point. resolveEffectiveWorkflowID also handles the case where the new
	// value is nil (resolves to the global default workflow).
	oldWorkflowID, newWorkflowID := nullableInt(oldCS.WorkflowID), nullableInt(cs.WorkflowID)
	workflowChanging := oldWorkflowID != newWorkflowID

	var effectiveNewWorkflowID *int
	if workflowChanging {
		effectiveNewWorkflowID, err = h.resolveEffectiveWorkflowID(cs.WorkflowID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		// If the admin is removing the workflow and there is no global default
		// to fall back to, we cannot validate items at all. Refuse rather than
		// silently orphan them.
		if cs.WorkflowID == nil && effectiveNewWorkflowID == nil {
			respondBadRequest(w, r, "Cannot remove workflow_id: no default workflow is configured to fall back to")
			return
		}
	}

	// Check if any workspace is moving from a different config set (requires
	// migration). The migration assistant flow runs the analyzer fresh after
	// migrations are applied, so once items are compatible the analyzer reports
	// requires_migration=false and the swap proceeds. When the workflow_id is
	// also changing in this PUT, validate the moving workspace's items against
	// the *new* workflow, not the stale DB value.
	for _, workspaceID := range cs.WorkspaceIDs {
		var currentConfigSetID *int
		currentConfigSetID, err = h.repo.GetWorkspaceConfigSetID(workspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if currentConfigSetID == nil || *currentConfigSetID != id {
			sourceConfigSetID := 0
			if currentConfigSetID != nil {
				sourceConfigSetID = *currentConfigSetID
			}
			var override *int
			if workflowChanging {
				override = effectiveNewWorkflowID
			}
			if h.respondMigrationConflictIfNeeded(w, r, workspaceID, sourceConfigSetID, id, override) {
				return
			}
		}
	}

	// Narrowing this configuration set's explicit item-type allow-list can
	// strand items in every workspace currently attached to it. Require an
	// intra-set item-type migration first; the returned targets are constrained
	// to the removed type's hierarchy level.
	if h.respondIntraSetItemTypeConflictIfNeeded(w, r, id, oldWorkspaceIDs, cs.ItemTypeConfigs) {
		return
	}

	// Detect intra-config-set workflow change. Workspaces that stay attached
	// to this config set need a status migration when the default workflow
	// itself changes — otherwise items can be left on status_ids that are
	// not part of the new workflow. We aggregate across all retained
	// workspaces in a single 409 so the migration assistant can migrate them
	// in one atomic call (otherwise the workflow_id swap mid-flight orphans
	// the not-yet-migrated workspaces).
	if workflowChanging {
		retained := intersectInts(oldWorkspaceIDs, cs.WorkspaceIDs)
		if h.respondIntraSetWorkflowConflictIfNeeded(w, r, id, retained, effectiveNewWorkflowID) {
			return
		}
	}

	// Update the configuration set and dependent assignments
	if err = h.repo.UpdateFull(id, &cs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate permission cache for the union of old + new workspace IDs.
	// OnConfigurationSetChanged only sees the post-swap state, so workspaces
	// that were just detached would not be invalidated by it.
	if h.permissionService != nil {
		seen := make(map[int]struct{}, len(oldWorkspaceIDs)+len(cs.WorkspaceIDs))
		for _, wsID := range oldWorkspaceIDs {
			seen[wsID] = struct{}{}
		}
		for _, wsID := range cs.WorkspaceIDs {
			seen[wsID] = struct{}{}
		}
		for wsID := range seen {
			_ = h.permissionService.InvalidateWorkspaceMemberCaches(wsID)
		}
	}

	// Refresh notification cache if service is available
	var warnings []models.APIWarning
	if h.notificationService != nil {
		if err = h.notificationService.ForceRefreshCache(); err != nil {
			warnings = append(warnings, createCacheWarning("notification", err, fmt.Sprintf("configuration_set_id:%d", id)))
		}
	}

	// Load and return the updated configuration set with all relations
	updatedCS, err := h.repo.FindByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event with change tracking
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]any)

		// Track what changed
		if oldCS.Name != updatedCS.Name {
			details["name_changed"] = map[string]any{
				"old": oldCS.Name,
				"new": updatedCS.Name,
			}
		}
		if oldCS.Description != updatedCS.Description {
			details["description_changed"] = map[string]any{
				"old": oldCS.Description,
				"new": updatedCS.Description,
			}
		}
		// Track workflow change
		oldWorkflowID := 0
		if oldCS.WorkflowID != nil {
			oldWorkflowID = *oldCS.WorkflowID
		}
		newWorkflowID := 0
		if updatedCS.WorkflowID != nil {
			newWorkflowID = *updatedCS.WorkflowID
		}
		if oldWorkflowID != newWorkflowID {
			details["workflow_changed"] = map[string]any{
				"old": oldWorkflowID,
				"new": newWorkflowID,
			}
		}
		details["workspace_count"] = len(updatedCS.WorkspaceIDs)

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionConfigSetUpdate,
			ResourceType: logger.ResourceConfigurationSet,
			ResourceID:   &id,
			ResourceName: updatedCS.Name,
			Details:      details,
			Success:      true,
		})
	}

	respondJSONOKWithWarnings(w, updatedCS, warnings)
}

func preserveOmittedConfigurationSetFields(cs, old *models.ConfigurationSet, fields map[string]json.RawMessage) {
	if _, ok := fields["name"]; !ok {
		cs.Name = old.Name
	}
	if _, ok := fields["description"]; !ok {
		cs.Description = old.Description
	}
	if _, ok := fields["is_default"]; !ok {
		cs.IsDefault = old.IsDefault
	}
	if _, ok := fields["differentiate_by_item_type"]; !ok {
		cs.DifferentiateByItemType = old.DifferentiateByItemType
	}
	if _, ok := fields["workflow_id"]; !ok {
		cs.WorkflowID = old.WorkflowID
	}
	if _, ok := fields["notification_setting_id"]; !ok {
		cs.NotificationSettingID = old.NotificationSettingID
	}
	if _, ok := fields["condition_set_id"]; !ok {
		cs.ConditionSetID = old.ConditionSetID
	}
	if _, ok := fields["approval_set_id"]; !ok {
		cs.ApprovalSetID = old.ApprovalSetID
	}
	if _, ok := fields["workspace_ids"]; !ok {
		cs.WorkspaceIDs = old.WorkspaceIDs
	}
	if _, ok := fields["item_type_configs"]; !ok {
		cs.ItemTypeConfigs = old.ItemTypeConfigs
	}
	if _, ok := fields["priority_ids"]; !ok {
		cs.PriorityIDs = old.PriorityIDs
	}
	if _, ok := fields["create_screen_id"]; !ok {
		cs.CreateScreenID = old.CreateScreenID
	}
	if _, ok := fields["edit_screen_id"]; !ok {
		cs.EditScreenID = old.EditScreenID
	}
	if _, ok := fields["view_screen_id"]; !ok {
		cs.ViewScreenID = old.ViewScreenID
	}
	if _, ok := fields["default_item_type_id"]; !ok {
		cs.DefaultItemTypeID = old.DefaultItemTypeID
	}
}
