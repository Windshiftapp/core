package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type UserPreferencesHandler struct {
	service *services.UserPreferencesService
}

func NewUserPreferencesHandler(service *services.UserPreferencesService) *UserPreferencesHandler {
	return &UserPreferencesHandler{service: service}
}

// GetUserPreferences returns the current user's preferences
func (h *UserPreferencesHandler) GetUserPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	response, err := h.service.Get(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, response)
}

// UpdateUserPreferences updates the current user's preferences
func (h *UserPreferencesHandler) UpdateUserPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.UserPreferencesRequest](w, r)
	if !ok {
		return
	}

	response, err := h.service.Update(user.ID, req)
	if errors.Is(err, services.ErrInvalidColorMode) {
		respondValidationError(w, r, "Invalid color_mode: must be 'light', 'dark', or 'system'")
		return
	}
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "theme")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, response)
}
