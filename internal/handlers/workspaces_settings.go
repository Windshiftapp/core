package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/models"
	"windshift/internal/services"
)

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

	// Get workspace homepage_layout from database
	var homepageLayout sql.NullString
	err := h.db.QueryRow(`
		SELECT homepage_layout
		FROM workspaces
		WHERE id = ?
	`, workspaceID).Scan(&homepageLayout)

	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "workspace")
			return
		}
		slog.Error("failed to get homepage layout", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// If no layout exists, return empty structure
	var layout models.WorkspaceHomepageLayout
	if homepageLayout.Valid && homepageLayout.String != "" {
		if err := json.Unmarshal([]byte(homepageLayout.String), &layout); err != nil {
			slog.Error("failed to parse homepage layout JSON", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
			respondInternalError(w, r, err)
			return
		}
	} else {
		// Return empty structure with empty arrays
		layout = models.WorkspaceHomepageLayout{
			Sections: []models.WorkspaceHomepageSection{},
			Widgets:  []models.WorkspaceWidget{},
		}
	}

	respondJSONOK(w, layout)
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
	_, err = h.db.Exec(`
		UPDATE workspaces
		SET homepage_layout = ?, updated_at = ?
		WHERE id = ?
	`, string(layoutJSON), time.Now(), workspaceID)

	if err != nil {
		slog.Error("failed to update homepage layout", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, layout)
}

// GetStatuses returns statuses available for a workspace based on its configuration set workflow,
// or the default workflow if none is assigned
func (h *WorkspaceHandler) GetStatuses(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "id")
	if !ok {
		return
	}

	// Check authentication and workspace view permission
	ok = h.requireWorkspacePermission(w, r, workspaceID, models.PermissionItemView)
	if !ok {
		return
	}

	// Parse optional item_type_id query parameter
	var itemTypeIDPtr *int
	if itStr := r.URL.Query().Get("item_type_id"); itStr != "" {
		itID, err := strconv.Atoi(itStr)
		if err != nil {
			respondBadRequest(w, r, "invalid item_type_id")
			return
		}
		itemTypeIDPtr = &itID
	}

	// Use WorkflowService for proper fallback chain (item type override → config set → global default)
	workflowService := services.NewWorkflowService(h.db)
	workflowID, err := workflowService.GetWorkflowIDForItem(workspaceID, itemTypeIDPtr)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Fall back to default workflow if service returned nil (e.g. personal workspace)
	if workflowID == nil {
		var defaultID int
		err = h.db.QueryRow(`SELECT id FROM workflows WHERE is_default = true LIMIT 1`).Scan(&defaultID)
		if err != nil {
			respondJSONOK(w, []models.Status{})
			return
		}
		workflowID = &defaultID
	}

	// Get statuses from workflow transitions
	rows, err := h.db.Query(`
		SELECT DISTINCT s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
		       sc.name as category_name, sc.color as category_color, sc.is_completed
		FROM workflow_transitions wt
		JOIN statuses s ON s.id = wt.to_status_id OR (wt.from_status_id IS NOT NULL AND s.id = wt.from_status_id)
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE wt.workflow_id = ?
		ORDER BY s.id
	`, *workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var statuses []models.Status
	for rows.Next() {
		var status models.Status
		var categoryName, categoryColor sql.NullString
		var isCompleted sql.NullBool
		err := rows.Scan(
			&status.ID, &status.Name, &status.Description, &status.CategoryID,
			&status.IsDefault, &status.CreatedAt, &status.UpdatedAt,
			&categoryName, &categoryColor, &isCompleted,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		status.CategoryName = categoryName.String
		status.CategoryColor = categoryColor.String
		status.IsCompleted = isCompleted.Bool

		statuses = append(statuses, status)
	}

	if statuses == nil {
		statuses = []models.Status{}
	}

	respondJSONOK(w, statuses)
}
