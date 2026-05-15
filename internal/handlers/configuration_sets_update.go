package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

func (h *ConfigurationSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the old configuration set for audit logging
	oldCS, err := h.repo.FindByIDBasic(id)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "configuration_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	cs, ok := decodeJSON[models.ConfigurationSet](w, r)
	if !ok {
		return
	}

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

		if currentConfigSetID != nil && *currentConfigSetID != id {
			var override *int
			if workflowChanging {
				override = effectiveNewWorkflowID
			}
			if h.respondMigrationConflictIfNeeded(w, r, workspaceID, *currentConfigSetID, id, override) {
				return
			}
		}
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
		details := make(map[string]interface{})

		// Track what changed
		if oldCS.Name != updatedCS.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldCS.Name,
				"new": updatedCS.Name,
			}
		}
		if oldCS.Description != updatedCS.Description {
			details["description_changed"] = map[string]interface{}{
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
			details["workflow_changed"] = map[string]interface{}{
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
