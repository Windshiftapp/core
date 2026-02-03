package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
	"windshift/internal/middleware"
	"windshift/internal/models"
)

// loadTimeProjectCategories loads time project categories for a workspace
func (h *WorkspaceHandler) loadTimeProjectCategories(workspaceID int) ([]int, error) {
	query := `
		SELECT time_project_category_id
		FROM workspace_time_project_categories
		WHERE workspace_id = ?
	`
	rows, err := h.db.Query(query, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := []int{} // Initialize as empty slice instead of nil
	for rows.Next() {
		var categoryID int
		if err := rows.Scan(&categoryID); err != nil {
			return nil, err
		}
		categories = append(categories, categoryID)
	}
	return categories, rows.Err()
}

// saveTimeProjectCategories saves time project categories for a workspace
func (h *WorkspaceHandler) saveTimeProjectCategories(workspaceID int, categories []int) error {
	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing associations
	_, err = tx.Exec("DELETE FROM workspace_time_project_categories WHERE workspace_id = ?", workspaceID)
	if err != nil {
		return err
	}

	// Insert new associations
	for _, categoryID := range categories {
		_, err = tx.Exec(
			"INSERT INTO workspace_time_project_categories (workspace_id, time_project_category_id) VALUES (?, ?)",
			workspaceID, categoryID,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetHomepageLayout handles GET /api/workspaces/:id/homepage/layout
func (h *WorkspaceHandler) GetHomepageLayout(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user := r.Context().Value(middleware.ContextKeyUser)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}
	currentUser, ok := user.(*models.User)
	if !ok {
		respondInternalError(w, r, fmt.Errorf("invalid user context"))
		return
	}

	// Check if user has access to this workspace
	hasAccess, permErr := h.permissionService.HasWorkspacePermission(currentUser.ID, workspaceID, models.PermissionItemView)
	if permErr != nil || !hasAccess {
		respondForbidden(w, r)
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
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user := r.Context().Value(middleware.ContextKeyUser)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}
	currentUser, ok := user.(*models.User)
	if !ok {
		respondInternalError(w, r, fmt.Errorf("invalid user context"))
		return
	}

	// Check if user has admin access to this workspace
	hasAccess, permErr := h.permissionService.HasWorkspacePermission(currentUser.ID, workspaceID, models.PermissionWorkspaceAdmin)
	if permErr != nil || !hasAccess {
		respondAdminRequired(w, r)
		return
	}

	// Parse request body
	var layout models.WorkspaceHomepageLayout
	if err := json.NewDecoder(r.Body).Decode(&layout); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate widgets
	validTypes := map[string]bool{
		"stats":              true,
		"completion-chart":   true,
		"created-chart":      true,
		"milestone-progress": true,
		"recent-items":       true,
		"my-tasks":           true,
		"overdue-items":      true,
		"item-filter":        true,
		"saved-search":       true,
		"upcoming-deadlines": true,
		"sprint-timeline":    true,
		"test-coverage":      true,
	}

	for _, widget := range layout.Widgets {
		if !validTypes[widget.Type] {
			respondValidationError(w, r, fmt.Sprintf("Invalid widget type: %s", widget.Type))
			return
		}
		if widget.Width < 1 || widget.Width > 3 {
			respondValidationError(w, r, fmt.Sprintf("Invalid widget width: %d (must be 1-3)", widget.Width))
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
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context for permission check
	user := r.Context().Value(middleware.ContextKeyUser)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}
	currentUser, ok := user.(*models.User)
	if !ok {
		respondInternalError(w, r, fmt.Errorf("invalid user context"))
		return
	}

	// Check if user has permission to view this workspace
	canView, permErr := h.canViewWorkspace(currentUser.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	// Try to get workflow from configuration set
	var workflowID *int
	err := h.db.QueryRow(`
		SELECT workflow_id
		FROM configuration_sets cs
		JOIN workspace_configuration_sets wcs ON cs.id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ?
		LIMIT 1
	`, workspaceID).Scan(&workflowID)

	if err != nil && err != sql.ErrNoRows {
		respondInternalError(w, r, err)
		return
	}

	// Fall back to default workflow if no configuration set
	if workflowID == nil {
		var defaultID int
		err = h.db.QueryRow(`SELECT id FROM workflows WHERE is_default = true LIMIT 1`).Scan(&defaultID)
		if err != nil {
			// No default workflow found - return empty array
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
	defer rows.Close()

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
