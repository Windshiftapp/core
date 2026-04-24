package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/models"
)

// validDashboardWidgetTypes lists widget types usable on the personal dashboard.
// Keep in sync with frontend/src/lib/services/dashboardWidgetRegistry.js.
var validDashboardWidgetTypes = map[string]bool{
	"daily-briefing":      true,
	"whats-new":           true,
	"your-activity":       true,
	"quick-access":        true,
	"upcoming-milestones": true,
	"watched-items":       true,
	"recent-workspaces":   true,
	"assigned-to-me":      true,
}

const (
	dashboardMaxSections = 20
	dashboardMaxWidgets  = 100
)

// GetDashboardLayout handles GET /api/user/dashboard-layout
func (h *UserPreferencesHandler) GetDashboardLayout(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	prefs, err := h.loadUserPreferences(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	layout := models.UserDashboardLayout{
		Sections: []models.UserDashboardSection{},
		Widgets:  []models.UserDashboardWidget{},
	}
	if prefs.DashboardLayout != nil {
		layout = *prefs.DashboardLayout
		if layout.Sections == nil {
			layout.Sections = []models.UserDashboardSection{}
		}
		if layout.Widgets == nil {
			layout.Widgets = []models.UserDashboardWidget{}
		}
	}

	respondJSONOK(w, layout)
}

// UpdateDashboardLayout handles PUT /api/user/dashboard-layout
func (h *UserPreferencesHandler) UpdateDashboardLayout(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	layout, ok := decodeJSON[models.UserDashboardLayout](w, r)
	if !ok {
		return
	}

	if len(layout.Sections) > dashboardMaxSections {
		respondValidationError(w, r, fmt.Sprintf("Too many sections: %d (max %d)", len(layout.Sections), dashboardMaxSections))
		return
	}
	if len(layout.Widgets) > dashboardMaxWidgets {
		respondValidationError(w, r, fmt.Sprintf("Too many widgets: %d (max %d)", len(layout.Widgets), dashboardMaxWidgets))
		return
	}

	sectionIDs := make(map[string]bool, len(layout.Sections))
	for _, section := range layout.Sections {
		if section.ID == "" {
			respondValidationError(w, r, "Section id is required")
			return
		}
		if sectionIDs[section.ID] {
			respondValidationError(w, r, fmt.Sprintf("Duplicate section id: %s", section.ID))
			return
		}
		sectionIDs[section.ID] = true
	}

	widgetIDs := make(map[string]bool, len(layout.Widgets))
	for _, widget := range layout.Widgets {
		if !validDashboardWidgetTypes[widget.Type] {
			respondValidationError(w, r, fmt.Sprintf("Invalid widget type: %s", widget.Type))
			return
		}
		if widget.Width < 1 || widget.Width > 3 {
			respondValidationError(w, r, fmt.Sprintf("Invalid widget width: %d (must be 1-3)", widget.Width))
			return
		}
		if widget.ID == "" {
			respondValidationError(w, r, "Widget id is required")
			return
		}
		if widgetIDs[widget.ID] {
			respondValidationError(w, r, fmt.Sprintf("Duplicate widget id: %s", widget.ID))
			return
		}
		widgetIDs[widget.ID] = true
		if widget.SectionID == "" || !sectionIDs[widget.SectionID] {
			respondValidationError(w, r, fmt.Sprintf("Widget %s references unknown section_id: %q", widget.ID, widget.SectionID))
			return
		}
	}

	// Merge layout into existing preferences so we don't clobber color_mode / theme_id.
	prefs, err := h.loadUserPreferences(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	prefs.DashboardLayout = &layout

	if err := h.saveUserPreferences(user.ID, prefs); err != nil {
		slog.Error("failed to save dashboard layout", slog.String("component", "user_preferences"), slog.Int("user_id", user.ID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, layout)
}

// loadUserPreferences reads and parses the user's preferences JSON, returning a
// zero value when no row exists.
func (h *UserPreferencesHandler) loadUserPreferences(userID int) (models.UserPreferencesData, error) {
	var prefs models.UserPreferencesData
	var raw string
	err := h.DB.QueryRow("SELECT preferences FROM user_preferences WHERE user_id = ?", userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return prefs, nil
	}
	if err != nil {
		return prefs, err
	}
	if raw == "" {
		return prefs, nil
	}
	if err := json.Unmarshal([]byte(raw), &prefs); err != nil {
		return prefs, err
	}
	return prefs, nil
}

// saveUserPreferences writes the preferences JSON, inserting when no row exists.
func (h *UserPreferencesHandler) saveUserPreferences(userID int, prefs models.UserPreferencesData) error {
	bytes, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	now := time.Now()

	var existing string
	err = h.DB.QueryRow("SELECT preferences FROM user_preferences WHERE user_id = ?", userID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = h.DB.Exec(
			"INSERT INTO user_preferences (user_id, preferences, created_at, updated_at) VALUES (?, ?, ?, ?)",
			userID, string(bytes), now, now,
		)
		return err
	}
	if err != nil {
		return err
	}
	_, err = h.DB.Exec(
		"UPDATE user_preferences SET preferences = ?, updated_at = ? WHERE user_id = ?",
		string(bytes), now, userID,
	)
	return err
}
