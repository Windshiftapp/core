package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

type ConfigurationSetNotificationHandler struct {
	repo *repository.ConfigurationSetRepository
}

func NewConfigurationSetNotificationHandler(repo *repository.ConfigurationSetRepository) *ConfigurationSetNotificationHandler {
	return &ConfigurationSetNotificationHandler{repo: repo}
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

	id, err := h.repo.AssignNotification(configSetID, req.NotificationSettingID)
	if errors.Is(err, repository.ErrDuplicateEntry) {
		respondConflict(w, r, "Notification setting is already assigned to this configuration set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

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

	err = h.repo.UnassignNotification(configSetID, assignmentID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "Assignment")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
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
