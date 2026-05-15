package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

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

	layout, err := h.service.GetDashboardLayout(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
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

	if err := h.service.UpdateDashboardLayout(user.ID, layout); err != nil {
		slog.Error("failed to save dashboard layout", slog.String("component", "user_preferences"), slog.Int("user_id", user.ID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, layout)
}
