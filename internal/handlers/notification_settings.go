package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type NotificationSettingsHandler struct {
	repo    *repository.NotificationSettingsRepository
	auditor *logger.Auditor
	service NotificationService
}

func NewNotificationSettingsHandler(repo *repository.NotificationSettingsRepository, auditor *logger.Auditor, service NotificationService) *NotificationSettingsHandler {
	return &NotificationSettingsHandler{repo: repo, auditor: auditor, service: service}
}

// refreshRuleCache forces an immediate reload of the notification rule cache
// so the just-applied settings change takes effect without waiting for the
// 5-minute background refresh. A failure here is logged but doesn't fail the
// request — the change is already persisted; the cache will catch up.
func (h *NotificationSettingsHandler) refreshRuleCache(action string) {
	if h.service == nil {
		return
	}
	if err := h.service.ForceRefreshCache(); err != nil {
		slog.Warn("notification rule cache refresh failed after settings change",
			slog.String("component", "notifications"),
			slog.String("action", action),
			slog.Any("error", err))
	}
}

// GetNotificationSettings returns all notification settings with their event rules
func (h *NotificationSettingsHandler) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.ListAll()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, settings)
}

// GetNotificationSetting returns a specific notification setting by ID
func (h *NotificationSettingsHandler) GetNotificationSetting(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	setting, err := h.repo.FindByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "notification_setting")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, setting)
}

// CreateNotificationSetting creates a new notification setting
func (h *NotificationSettingsHandler) CreateNotificationSetting(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.NotificationSetting](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if req.CreatedBy == 0 {
		respondValidationError(w, r, "CreatedBy is required")
		return
	}

	id, err := h.repo.CreateWithRules(&req)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	req.ID = id

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionNotificationSettingCreate, logger.ResourceNotificationSetting, &id, req.Name)
	}

	h.refreshRuleCache("create")
	respondJSONCreated(w, req)
}

// UpdateNotificationSetting updates an existing notification setting
func (h *NotificationSettingsHandler) UpdateNotificationSetting(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.NotificationSetting](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	if err := h.repo.UpdateWithRules(id, &req); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "notification_setting")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionNotificationSettingUpdate, logger.ResourceNotificationSetting, &id, req.Name)
	}

	h.refreshRuleCache("update")
	req.ID = id
	respondJSONOK(w, req)
}

// DeleteNotificationSetting deletes a notification setting
func (h *NotificationSettingsHandler) DeleteNotificationSetting(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	count, err := h.repo.CountConfigurationSetAssignments(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if count > 0 {
		respondConflict(w, r, "Cannot delete notification setting: it is assigned to one or more configuration sets")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "notification_setting")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionNotificationSettingDelete, logger.ResourceNotificationSetting, &id, "")
	}

	h.refreshRuleCache("delete")
	w.WriteHeader(http.StatusNoContent)
}

// GetAvailableEvents returns all available notification event types
func (h *NotificationSettingsHandler) GetAvailableEvents(w http.ResponseWriter, _ *http.Request) {
	events := models.GetAvailableNotificationEvents()
	respondJSONOK(w, events)
}
