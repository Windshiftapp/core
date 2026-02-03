package handlers

import (
	"encoding/json"
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

	var cs models.ConfigurationSet
	if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	// Validate required fields
	if strings.TrimSpace(cs.Name) == "" {
		respondValidationError(w, r, "Configuration set name is required")
		return
	}

	// Verify workspaces exist
	for _, workspaceID := range cs.WorkspaceIDs {
		exists, err := h.repo.WorkspaceExists(workspaceID)
		if err != nil || !exists {
			respondBadRequest(w, r, "One or more workspaces not found")
			return
		}
	}

	// Check if any workspace is moving from a different config set (requires migration)
	// Skip this check if skip_migration_check query param is set (used after migration is complete)
	skipMigrationCheck := r.URL.Query().Get("skip_migration_check") == "true"
	if !skipMigrationCheck {
		for _, workspaceID := range cs.WorkspaceIDs {
			currentConfigSetID, err := h.repo.GetWorkspaceConfigSetID(workspaceID)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}

			// If workspace is currently assigned to a different config set
			if currentConfigSetID != nil && *currentConfigSetID != id {
				// Analyze migration requirements
				sourceID := *currentConfigSetID

				itemTypeMigrations, _, requiresItemTypeMigration := h.analyzeItemTypeMigration(workspaceID, sourceID, id)
				customFieldMigrations, requiresFieldMigration := h.analyzeCustomFieldMigration(workspaceID, sourceID, id)
				priorityMigrations, _, requiresPriorityMigration := h.analyzePriorityMigration(workspaceID, sourceID, id)
				statusMigrations, requiresStatusMigration := h.analyzeStatusMigration(workspaceID, id)

				requiresMigration := requiresItemTypeMigration || requiresFieldMigration ||
					requiresPriorityMigration || requiresStatusMigration

				if requiresMigration {
					// Get config set names for the response
					var sourceConfigSetName, targetConfigSetName string
					h.db.QueryRow(`SELECT name FROM configuration_sets WHERE id = ?`, sourceID).Scan(&sourceConfigSetName)
					h.db.QueryRow(`SELECT name FROM configuration_sets WHERE id = ?`, id).Scan(&targetConfigSetName)

					var totalItems int
					h.db.QueryRow(`SELECT COUNT(*) FROM items WHERE workspace_id = ?`, workspaceID).Scan(&totalItems)

					analysis := models.ComprehensiveMigrationAnalysis{
						OldConfigSetID:            sourceID,
						OldConfigSetName:          sourceConfigSetName,
						NewConfigSetID:            id,
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

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error":    "migration_required",
						"message":  "Migration is required before this workspace can be assigned to the new configuration set",
						"analysis": analysis,
					})
					return
				}
			}
		}
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer tx.Rollback()

	// Update the configuration set
	if err := h.repo.Update(tx, id, &cs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save notification setting
	var notificationSettingID *int
	if cs.NotificationSettingID != nil {
		nsID := int(*cs.NotificationSettingID)
		notificationSettingID = &nsID
	}
	if err := h.repo.SaveNotificationSetting(tx, id, notificationSettingID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save workspace assignments
	if err := h.repo.SaveWorkspaceAssignments(tx, id, cs.WorkspaceIDs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save screen assignments
	if err := h.repo.SaveScreenAssignments(tx, id, cs.CreateScreenID, cs.EditScreenID, cs.ViewScreenID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save item type configurations
	if err := h.repo.SaveItemTypeConfigs(tx, id, cs.ItemTypeConfigs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save priority assignments
	if err := h.repo.SavePriorityAssignments(tx, id, cs.PriorityIDs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Refresh notification cache if service is available
	var warnings []models.APIWarning
	if h.notificationService != nil {
		if err := h.notificationService.ForceRefreshCache(); err != nil {
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
			oldWorkflowID = int(*oldCS.WorkflowID)
		}
		newWorkflowID := 0
		if updatedCS.WorkflowID != nil {
			newWorkflowID = int(*updatedCS.WorkflowID)
		}
		if oldWorkflowID != newWorkflowID {
			details["workflow_changed"] = map[string]interface{}{
				"old": oldWorkflowID,
				"new": newWorkflowID,
			}
		}
		details["workspace_count"] = len(updatedCS.WorkspaceIDs)

		logger.LogAudit(h.db, logger.AuditEvent{
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
