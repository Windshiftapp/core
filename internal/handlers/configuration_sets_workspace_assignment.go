package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type assignWorkspaceConfigSetRequest struct {
	ConfigurationSetID *int `json:"configuration_set_id"`
}

// AssignWorkspaceConfigurationSet assigns or clears the configuration set of a
// single workspace. The route is gated on workspace.admin (WI-1359); every
// other configuration-set mutation stays system-admin-only. Only the
// workspace-assignment join rows are touched — never the set's own fields.
func (h *ConfigurationSetHandler) AssignWorkspaceConfigurationSet(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	req, ok := decodeJSON[assignWorkspaceConfigSetRequest](w, r)
	if !ok {
		return
	}

	if exists, err := h.repo.WorkspaceExists(workspaceID); err != nil {
		respondInternalError(w, r, err)
		return
	} else if !exists {
		respondNotFound(w, r, "workspace")
		return
	}

	var target *models.ConfigurationSet
	if req.ConfigurationSetID != nil {
		found, err := h.repo.FindByIDBasic(*req.ConfigurationSetID)
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "configuration_set")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if found.IsDefault {
			respondBadRequest(w, r, "The default configuration set applies automatically and cannot be assigned explicitly")
			return
		}
		target = found
	}

	currentID, err := h.repo.GetWorkspaceConfigSetID(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Switching between two sets requires the workspace's items to be
	// compatible with the target. Mirror the PUT /configuration-sets/{id} 409
	// contract so clients can drive the migration flow; unassigning has no
	// compatibility constraint.
	if currentID != nil && req.ConfigurationSetID != nil && *currentID != *req.ConfigurationSetID {
		if h.respondMigrationConflictIfNeeded(w, r, workspaceID, *currentID, *req.ConfigurationSetID, nil) {
			return
		}
	}

	if err := h.repo.AssignWorkspace(workspaceID, req.ConfigurationSetID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if h.permissionService != nil {
		_ = h.permissionService.InvalidateWorkspaceMemberCaches(workspaceID)
	}
	var warnings []models.APIWarning
	if h.notificationService != nil {
		if err := h.notificationService.ForceRefreshCache(); err != nil {
			warnings = append(warnings, createCacheWarning("notification", err, fmt.Sprintf("workspace_id:%d", workspaceID)))
		}
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		var resourceID *int
		resourceName := ""
		if target != nil {
			resourceID = req.ConfigurationSetID
			resourceName = target.Name
		}
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionConfigSetWorkspaceAssign,
			ResourceType: logger.ResourceConfigurationSet,
			ResourceID:   resourceID,
			ResourceName: resourceName,
			Details: map[string]any{
				"workspace_id":         workspaceID,
				"configuration_set_id": req.ConfigurationSetID,
			},
			Success: true,
		})
	}

	respondJSONOKWithWarnings(w, map[string]any{
		"workspace_id":         workspaceID,
		"configuration_set_id": req.ConfigurationSetID,
	}, warnings)
}
