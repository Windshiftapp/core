package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// sanitizeHomepageLayout scrubs the user-supplied strings on a homepage
// layout payload. Section Title + Subtitle render as homepage headings;
// section/widget IDs are client-generated UUIDs but get echoed back in
// validation errors, so they're bounded as identifiers. Widget Type is
// enum-validated against the registry and Config is a JSON blob consumed
// per-widget, so both stay untouched.
func sanitizeHomepageLayout(layout *models.WorkspaceHomepageLayout) {
	for i := range layout.Sections {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &layout.Sections[i].ID, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &layout.Sections[i].Title, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &layout.Sections[i].Subtitle, Policy: sanitize.PlainTextField},
		)
		// WidgetIDs carries the same client-generated UUIDs as widget.ID,
		// stored verbatim and echoed back on GET, so each element gets the
		// same identifier bound.
		for j := range layout.Sections[i].WidgetIDs {
			sanitize.Apply(&layout.Sections[i].WidgetIDs[j], sanitize.ShortIdentifier)
		}
	}
	for i := range layout.Widgets {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &layout.Widgets[i].ID, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &layout.Widgets[i].SectionID, Policy: sanitize.ShortIdentifier},
		)
	}
	sanitize.Apply(&layout.BackgroundImageURL, sanitize.PlainTextField)
}

func (h *WorkspaceHandler) loadWorkspaceStatuses(workspaceID int, itemTypeID *int) ([]models.Status, error) {
	if itemTypeID == nil {
		return h.workspaceService.GetStatuses(workspaceID)
	}

	workflowService := services.NewWorkflowService(h.db)
	workflowID, err := workflowService.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}
	if workflowID == nil {
		workflowID, err = workflowService.GetDefaultWorkflowID()
		if err != nil || workflowID == nil {
			return []models.Status{}, err
		}
	}

	results, err := services.NewStatusService(h.db).ListWorkflowStatuses(*workflowID)
	if err != nil {
		return nil, err
	}
	statuses := make([]models.Status, 0, len(results))
	for _, result := range results {
		statuses = append(statuses, models.Status{
			ID: result.ID, Name: result.Name, Description: result.Description,
			CategoryID: result.CategoryID, IsDefault: result.IsDefault,
			CreatedAt: result.CreatedAt, UpdatedAt: result.UpdatedAt,
			CategoryName: result.CategoryName, CategoryColor: result.CategoryColor,
			IsCompleted: result.IsCompleted,
		})
	}
	return statuses, nil
}

func (h *WorkspaceHandler) GetStatuses(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "id")
	if !ok || !h.requireWorkspacePermission(w, r, workspaceID, models.PermissionItemView) {
		return
	}
	var itemTypeID *int
	if raw := r.URL.Query().Get("item_type_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil {
			respondBadRequest(w, r, "invalid item_type_id")
			return
		}
		itemTypeID = &id
	}
	statuses, err := h.loadWorkspaceStatuses(workspaceID, itemTypeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, statuses)
}

// requireWorkspacePermission checks authentication and workspace-level permission in one step.
// It writes the appropriate error response and returns nil, false when the check fails.
func (h *WorkspaceHandler) requireWorkspacePermission(w http.ResponseWriter, r *http.Request, workspaceID int, perm string) bool {
	user, ok := RequireAuth(w, r)
	if !ok {
		return false
	}
	hasAccess, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, perm)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !hasAccess {
		respondForbidden(w, r)
		return false
	}
	_ = user // authenticated but callers only need the permission check
	return true
}

// GetHomepageLayout handles GET /api/workspaces/:id/homepage/layout
func (h *WorkspaceHandler) GetHomepageLayout(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "id")
	if !ok {
		return
	}

	// Check authentication and workspace view permission
	ok = h.requireWorkspacePermission(w, r, workspaceID, models.PermissionItemView)
	if !ok {
		return
	}

	layout, err := h.loadHomepageLayout(workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "workspace")
		return
	}
	if err != nil {
		slog.Error("failed to get homepage layout", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, layout)
}

func (h *WorkspaceHandler) loadHomepageLayout(workspaceID int) (models.WorkspaceHomepageLayout, error) {
	homepageLayout, err := h.repo.GetHomepageLayoutJSON(workspaceID)
	if err != nil {
		return models.WorkspaceHomepageLayout{}, err
	}

	layout := models.WorkspaceHomepageLayout{
		Sections: []models.WorkspaceHomepageSection{},
		Widgets:  []models.WorkspaceWidget{},
	}
	if homepageLayout == "" {
		return layout, nil
	}
	if err := json.Unmarshal([]byte(homepageLayout), &layout); err != nil {
		return models.WorkspaceHomepageLayout{}, err
	}
	return layout, nil
}

// UpdateHomepageLayout handles PUT /api/workspaces/:id/homepage/layout
func (h *WorkspaceHandler) UpdateHomepageLayout(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "id")
	if !ok {
		return
	}

	// Check authentication and workspace admin permission
	ok = h.requireWorkspacePermission(w, r, workspaceID, models.PermissionWorkspaceAdmin)
	if !ok {
		return
	}

	// Parse request body
	layout, ok := decodeJSON[models.WorkspaceHomepageLayout](w, r)
	if !ok {
		return
	}
	sanitizeHomepageLayout(&layout)

	// Validate widgets. Keep this list in sync with
	// frontend/src/lib/services/widgetRegistry.js — types missing a frontend
	// component would render as empty cards.
	validTypes := map[string]bool{
		"stats":              true,
		"completion-chart":   true,
		"created-chart":      true,
		"milestone-progress": true,
		"recent-items":       true,
		"my-tasks":           true,
		"overdue-items":      true,
		"upcoming-deadlines": true,
		"iteration-timeline": true,
		"test-coverage":      true,
		"saved-search":       true,
	}

	const (
		maxSections = 20
		maxWidgets  = 100
	)
	if len(layout.Sections) > maxSections {
		respondValidationError(w, r, fmt.Sprintf("Too many sections: %d (max %d)", len(layout.Sections), maxSections))
		return
	}
	if len(layout.Widgets) > maxWidgets {
		respondValidationError(w, r, fmt.Sprintf("Too many widgets: %d (max %d)", len(layout.Widgets), maxWidgets))
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
		if !validTypes[widget.Type] {
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

	// Convert to JSON
	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		slog.Error("failed to marshal homepage layout", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Update database
	if err := h.repo.UpdateHomepageLayoutJSON(workspaceID, string(layoutJSON), time.Now()); err != nil {
		slog.Error("failed to update homepage layout", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, layout)
}
