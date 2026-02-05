package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/services"
)

type AttachmentSettingsHandler struct {
	settingsService *services.AttachmentSettingsService
}

func NewAttachmentSettingsHandler(settingsService *services.AttachmentSettingsService) *AttachmentSettingsHandler {
	return &AttachmentSettingsHandler{
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(settings)
}

// Update modifies attachment settings
func (h *AttachmentSettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	settingsID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "settings ID")
		return
	}

	var req models.AttachmentSettingsRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(settings)
}

// GetStatus returns attachment system status (enabled/disabled, path info)
func (h *AttachmentSettingsHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.settingsService.GetStatus()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
