package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type ThemeHandler struct {
	service *services.ThemeService
	auditor *logger.Auditor
}

func NewThemeHandler(service *services.ThemeService, auditor *logger.Auditor) *ThemeHandler {
	return &ThemeHandler{service: service, auditor: auditor}
}

// GetThemes returns all themes
func (h *ThemeHandler) GetThemes(w http.ResponseWriter, r *http.Request) {
	themes, err := h.service.List()
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to query themes: %w", err))
		return
	}
	respondJSONOK(w, themes)
}

// GetActiveTheme returns the currently active theme
func (h *ThemeHandler) GetActiveTheme(w http.ResponseWriter, r *http.Request) {
	theme, err := h.service.GetActive()
	if errors.Is(err, repository.ErrNotFound) {
		// No active theme found - return null
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get active theme: %w", err))
		return
	}
	respondJSONOK(w, theme)
}

// validateThemeFields checks the required color and name fields shared by create and update requests.
func validateThemeFields(name, navBgLight, navTextLight, navBgDark, navTextDark string) string {
	if name == "" {
		return "Name is required"
	}
	if navBgLight == "" {
		return "Navigation background color (light) is required"
	}
	if navTextLight == "" {
		return "Navigation text color (light) is required"
	}
	if navBgDark == "" {
		return "Navigation background color (dark) is required"
	}
	if navTextDark == "" {
		return "Navigation text color (dark) is required"
	}
	return ""
}

// CreateTheme creates a new theme
func (h *ThemeHandler) CreateTheme(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.ThemeCreateRequest](w, r)
	if !ok {
		return
	}
	if msg := validateThemeFields(req.Name, req.NavBackgroundColorLight, req.NavTextColorLight, req.NavBackgroundColorDark, req.NavTextColorDark); msg != "" {
		respondValidationError(w, r, msg)
		return
	}

	theme, err := h.service.Create(req)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to create theme: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionThemeCreate, logger.ResourceTheme, &theme.ID, theme.Name)
	}
	respondJSONCreated(w, theme)
}

// UpdateTheme updates an existing theme
func (h *ThemeHandler) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	themeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	req, ok := decodeJSON[models.ThemeUpdateRequest](w, r)
	if !ok {
		return
	}
	if msg := validateThemeFields(req.Name, req.NavBackgroundColorLight, req.NavTextColorLight, req.NavBackgroundColorDark, req.NavTextColorDark); msg != "" {
		respondValidationError(w, r, msg)
		return
	}

	theme, err := h.service.Update(themeID, req)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "theme")
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to update theme: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionThemeUpdate, logger.ResourceTheme, &themeID, theme.Name)
	}
	respondJSONOK(w, theme)
}

// DeleteTheme deletes a theme
func (h *ThemeHandler) DeleteTheme(w http.ResponseWriter, r *http.Request) {
	themeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.service.Delete(themeID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "theme")
		return
	}
	if errors.Is(err, services.ErrDefaultThemeDelete) {
		respondValidationError(w, r, "Cannot delete default theme")
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to delete theme: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionThemeDelete, logger.ResourceTheme, &themeID, "")
	}
	w.WriteHeader(http.StatusNoContent)
}

// ActivateTheme sets a theme as active
func (h *ThemeHandler) ActivateTheme(w http.ResponseWriter, r *http.Request) {
	themeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.service.Activate(themeID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "theme")
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to activate theme: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionThemeActivate, logger.ResourceTheme, &themeID, "")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success": true}`))
}
