package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type AttachmentSettingsHandler struct {
	db              database.Database
	settingsService *services.AttachmentSettingsService
}

func NewAttachmentSettingsHandler(db database.Database, settingsService *services.AttachmentSettingsService) *AttachmentSettingsHandler {
	return &AttachmentSettingsHandler{
		db:              db,
		settingsService: settingsService,
	}
}

// Get retrieves current attachment settings
func (h *AttachmentSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := h.settingsService.Get()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, settings)
}

// Update modifies attachment settings
func (h *AttachmentSettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	settingsID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	req, ok := decodeJSON[models.AttachmentSettingsRequest](w, r)
	if !ok {
		return
	}

	settings, err := h.settingsService.Update(settingsID, &req)
	if err != nil {
		// Check if it's a validation error
		if err.Error() == "max file size must be greater than 0" {
			respondValidationError(w, r, err.Error())
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionAttachmentSettingsUpdate,
			ResourceType: logger.ResourceAttachmentSettings,
			ResourceID:   &settingsID,
			ResourceName: "attachment_settings",
			Success:      true,
		})
	}

	respondJSONOK(w, settings)
}

// GetStatus returns attachment system status (enabled/disabled, path info)
func (h *AttachmentSettingsHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.settingsService.GetStatus()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, status)
}
