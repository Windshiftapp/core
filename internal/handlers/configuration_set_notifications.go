package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type ConfigurationSetNotificationHandler struct {
	repo    *repository.ConfigurationSetRepository
	service NotificationService
	auditor *logger.Auditor
}

func NewConfigurationSetNotificationHandler(repo *repository.ConfigurationSetRepository, service NotificationService, auditor *logger.Auditor) *ConfigurationSetNotificationHandler {
	return &ConfigurationSetNotificationHandler{repo: repo, service: service, auditor: auditor}
}

func (h *ConfigurationSetNotificationHandler) refreshRuleCache(action string) {
	if h.service == nil {
		return
	}
	if err := h.service.ForceRefreshCache(); err != nil {
		slog.Warn("notification rule cache refresh failed after configuration set change",
			slog.String("component", "notifications"),
			slog.String("action", action),
			slog.Any("error", err))
	}
}

// GetConfigurationSetNotifications returns all notification settings for a configuration set
func (h *ConfigurationSetNotificationHandler) GetConfigurationSetNotifications(w http.ResponseWriter, r *http.Request) {
	configSetIDStr := r.PathValue("config_set_id")
	configSetID, err := strconv.Atoi(configSetIDStr)
	if err != nil {
		respondInvalidID(w, r, "config_set_id")
		return
	}

	assignments, err := h.repo.ListNotificationAssignments(configSetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, assignments)
}

// AssignNotificationToConfigurationSet assigns a notification setting to a configuration set
func (h *ConfigurationSetNotificationHandler) AssignNotificationToConfigurationSet(w http.ResponseWriter, r *http.Request) {
	configSetIDStr := r.PathValue("config_set_id")
	configSetID, err := strconv.Atoi(configSetIDStr)
	if err != nil {
		respondInvalidID(w, r, "config_set_id")
		return
	}

	var req struct {
		NotificationSettingID int `json:"notification_setting_id"`
	}
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	if req.NotificationSettingID == 0 {
		respondValidationError(w, r, "notification_setting_id is required")
		return
	}

	csName, err := h.repo.LookupConfigurationSetName(configSetID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "Configuration set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	ns, err := h.repo.LookupNotificationSetting(req.NotificationSettingID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "Notification setting")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !ns.IsActive {
		respondBadRequest(w, r, "Cannot assign inactive notification setting")
		return
	}

	// Upserts: a second Assign for the same config set replaces the prior
	// notification setting (bughunt #6 — one-to-one mapping).
	id, err := h.repo.AssignNotification(configSetID, req.NotificationSettingID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if h.auditor != nil {
		if currentUser := utils.GetCurrentUser(r); currentUser != nil {
			h.auditor.LogWithDetails(r, currentUser, logger.ActionConfigSetNotificationAssign, logger.ResourceConfigurationSet, &configSetID, csName, map[string]interface{}{
				"notification_setting_id":   req.NotificationSettingID,
				"notification_setting_name": ns.Name,
				"assignment_id":             id,
			})
		}
	}

	h.refreshRuleCache("assign")
	respondJSONCreated(w, models.ConfigurationSetNotificationSetting{
		ID:                      id,
		ConfigurationSetID:      configSetID,
		NotificationSettingID:   req.NotificationSettingID,
		CreatedAt:               time.Now(),
		ConfigurationSetName:    csName,
		NotificationSettingName: ns.Name,
	})
}

// UnassignNotificationFromConfigurationSet removes a notification setting from a configuration set
func (h *ConfigurationSetNotificationHandler) UnassignNotificationFromConfigurationSet(w http.ResponseWriter, r *http.Request) {
	configSetIDStr := r.PathValue("config_set_id")
	assignmentIDStr := r.PathValue("assignment_id")

	configSetID, err := strconv.Atoi(configSetIDStr)
	if err != nil {
		respondInvalidID(w, r, "config_set_id")
		return
	}

	assignmentID, err := strconv.Atoi(assignmentIDStr)
	if err != nil {
		respondInvalidID(w, r, "assignment_id")
		return
	}

	csName, nameErr := h.repo.LookupConfigurationSetName(configSetID)
	if nameErr != nil && !errors.Is(nameErr, repository.ErrNotFound) {
		respondInternalError(w, r, nameErr)
		return
	}

	err = h.repo.UnassignNotification(configSetID, assignmentID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "Assignment")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if h.auditor != nil {
		if currentUser := utils.GetCurrentUser(r); currentUser != nil {
			h.auditor.LogWithDetails(r, currentUser, logger.ActionConfigSetNotificationUnassign, logger.ResourceConfigurationSet, &configSetID, csName, map[string]interface{}{
				"assignment_id": assignmentID,
			})
		}
	}
	h.refreshRuleCache("unassign")
	w.WriteHeader(http.StatusNoContent)
}

// GetAvailableNotificationSettings returns notification settings not yet assigned to a configuration set
func (h *ConfigurationSetNotificationHandler) GetAvailableNotificationSettings(w http.ResponseWriter, r *http.Request) {
	configSetIDStr := r.PathValue("config_set_id")
	configSetID, err := strconv.Atoi(configSetIDStr)
	if err != nil {
		respondInvalidID(w, r, "config_set_id")
		return
	}

	settings, err := h.repo.ListAvailableNotificationSettings(configSetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, settings)
}
