package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type WorkspaceHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	activityTracker   *services.ActivityTracker
}

// CreateWorkspaceRequest represents the request payload for creating a workspace
type CreateWorkspaceRequest struct {
	Name          string `json:"name" validate:"required,max=100"`
	Key           string `json:"key" validate:"required,min=2,max=10,alphanum"`
	Description   string `json:"description" validate:"max=500"`
	Active        *bool  `json:"active,omitempty"` // Defaults to true if not specified
	TimeProjectID *int   `json:"time_project_id,omitempty"`
	IsPersonal    bool   `json:"is_personal"`
	OwnerID       *int   `json:"owner_id,omitempty"`
	Icon          string `json:"icon,omitempty"`
	Color         string `json:"color,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	DefaultView   string `json:"default_view,omitempty"` // Default view when entering workspace (board, backlog, list, tree, map)
}

// UpdateWorkspaceRequest represents the request payload for updating a workspace
type UpdateWorkspaceRequest struct {
	Name                  string `json:"name" validate:"required,max=100"`
	Key                   string `json:"key" validate:"omitempty,min=2,max=10,alphanum"` // Optional - if not provided, keeps existing key
	Description           string `json:"description" validate:"max=500"`
	Active                bool   `json:"active"`
	TimeProjectID         *int   `json:"time_project_id,omitempty"`
	IsPersonal            bool   `json:"is_personal"`
	OwnerID               *int   `json:"owner_id,omitempty"`
	Icon                  string `json:"icon,omitempty"`
	Color                 string `json:"color,omitempty"`
	AvatarURL             string `json:"avatar_url,omitempty"`
	DefaultView           string `json:"default_view,omitempty"` // Default view when entering workspace (board, backlog, list, tree, map)
	TimeProjectCategories []int  `json:"time_project_categories,omitempty"`
}

func NewWorkspaceHandler(db database.Database, permissionService *services.PermissionService, activityTracker *services.ActivityTracker) *WorkspaceHandler {
	return &WorkspaceHandler{
		db:                db,
		permissionService: permissionService,
		activityTracker:   activityTracker,
	}
}

func (h *WorkspaceHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user := r.Context().Value("user")
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	currentUser, ok := user.(*models.User)
	if !ok {
		http.Error(w, "Invalid user context", http.StatusInternalServerError)
		return
	}

	// Check for is_personal query parameter
	isPersonalParam := r.URL.Query().Get("is_personal")

	var query string
	var rows *sql.Rows
	var err error

	if isPersonalParam == "true" {
		// Filter to only current user's personal workspace
		query = `
			SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at,
			       COUNT(p.id) as project_count,
			       tp.name as time_project_name
			FROM workspaces w
			LEFT JOIN projects p ON w.id = p.workspace_id
			LEFT JOIN time_projects tp ON w.time_project_id = tp.id
			WHERE w.is_personal = ? AND w.owner_id = ?
			GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at, tp.name
			ORDER BY w.name`
		rows, err = h.db.Query(query, true, currentUser.ID)
	} else {
		// Get all workspaces excluding other users' personal workspaces
		// Only include: 1) non-personal workspaces, 2) current user's personal workspace
		query = `
			SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at,
			       COUNT(p.id) as project_count,
			       tp.name as time_project_name
			FROM workspaces w
			LEFT JOIN projects p ON w.id = p.workspace_id
			LEFT JOIN time_projects tp ON w.time_project_id = tp.id
			WHERE w.is_personal = 0 OR w.is_personal IS NULL OR w.owner_id = ?
			GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at, tp.name
			ORDER BY w.is_personal ASC, w.name`
		rows, err = h.db.Query(query, currentUser.ID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var workspaces []models.Workspace
	for rows.Next() {
		var workspace models.Workspace
		var timeProjectName, icon, color, defaultView sql.NullString
		err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
			&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &icon, &color, &workspace.AvatarURL, &defaultView, &workspace.CreatedAt, &workspace.UpdatedAt,
			&workspace.ProjectCount, &timeProjectName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		workspace.Icon = icon.String
		workspace.Color = color.String
		workspace.DefaultView = defaultView.String
		workspace.TimeProjectName = timeProjectName.String
		workspaces = append(workspaces, workspace)
	}

	// Filter workspaces by permission
	filteredWorkspaces, err := h.filterWorkspacesByPermissions(currentUser.ID, workspaces)
	if err != nil {
		http.Error(w, "Error filtering workspaces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter out inactive workspaces unless user can access them
	accessibleWorkspaces := []models.Workspace{}
	for _, ws := range filteredWorkspaces {
		// If workspace is active, include it
		if ws.Active {
			accessibleWorkspaces = append(accessibleWorkspaces, ws)
			continue
		}

		// If workspace is inactive, check if user can access it
		canAccess, err := h.canAccessInactiveWorkspace(currentUser, ws.ID)
		if err != nil {
			// Log error but don't fail the entire request
			// Just skip this workspace
			continue
		}

		if canAccess {
			accessibleWorkspaces = append(accessibleWorkspaces, ws)
		}
	}

	respondJSONOK(w, accessibleWorkspaces)
}

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

func (h *WorkspaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user := r.Context().Value("user")
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	currentUser, ok := user.(*models.User)
	if !ok {
		http.Error(w, "Invalid user context", http.StatusInternalServerError)
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var workspace models.Workspace
	var timeProjectName, icon, color, defaultView sql.NullString
	var configSetID sql.NullInt64
	err := h.db.QueryRow(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at,
		       COUNT(p.id) as project_count,
		       tp.name as time_project_name,
		       wcs.configuration_set_id
		FROM workspaces w
		LEFT JOIN projects p ON w.id = p.workspace_id
		LEFT JOIN time_projects tp ON w.time_project_id = tp.id
		LEFT JOIN workspace_configuration_sets wcs ON w.id = wcs.workspace_id
		WHERE w.id = ?
		GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at, tp.name, wcs.configuration_set_id
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &icon, &color, &workspace.AvatarURL, &defaultView, &workspace.CreatedAt, &workspace.UpdatedAt,
		&workspace.ProjectCount, &timeProjectName, &configSetID)

	workspace.Icon = icon.String
	workspace.Color = color.String
	workspace.DefaultView = defaultView.String
	workspace.TimeProjectName = timeProjectName.String
	if configSetID.Valid {
		workspace.ConfigurationSetID = &configSetID.Int64
	}

	if err == sql.ErrNoRows {
		http.Error(w, "Workspace not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permissions based on workspace state
	if !workspace.Active {
		// For inactive workspaces, check if user has admin access
		canAccess, err := h.canAccessInactiveWorkspace(currentUser, workspace.ID)
		if err != nil {
			http.Error(w, "Error checking permissions: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !canAccess {
			http.Error(w, "This workspace is inactive and you don't have permission to access it", http.StatusForbidden)
			return
		}
	} else {
		// For active workspaces, check if user has view permission
		canView, err := h.canViewWorkspace(currentUser.ID, workspace.ID)
		if err != nil {
			http.Error(w, "Error checking permissions: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !canView {
			http.Error(w, "Insufficient permissions to view this workspace", http.StatusForbidden)
			return
		}
	}

	// Track workspace visit
	if h.activityTracker != nil {
		if err := h.activityTracker.TrackWorkspaceVisit(currentUser.ID, workspace.ID); err != nil {
			slog.Error("failed to track workspace visit", slog.String("component", "workspaces"), slog.Int("user_id", currentUser.ID), slog.Int("workspace_id", workspace.ID), slog.Any("error", err))
			// Don't fail the request, just log the error
		}
	}

	// Load time project categories for this workspace
	timeProjectCats, err := h.loadTimeProjectCategories(id)
	if err != nil {
		slog.Error("failed to load time project categories", slog.String("component", "workspaces"), slog.Int("workspace_id", id), slog.Any("error", err))
		// Don't fail the request, just log the error
		workspace.TimeProjectCategories = []int{} // Always include the field
	} else {
		workspace.TimeProjectCategories = timeProjectCats // Set even if empty
	}

	respondJSONOK(w, workspace)
}

func (h *WorkspaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check if user has permission to create workspaces
	canCreate, err := h.canCreateWorkspace(user.ID)
	if err != nil {
		http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !canCreate {
		http.Error(w, "Insufficient permissions to create workspaces", http.StatusForbidden)
		return
	}

	// Parse request
	var req CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input using validator
	if err := utils.Validate(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Sanitize for defense in depth
	req.Name = utils.SanitizeName(req.Name)
	req.Key = utils.SanitizeName(req.Key)
	req.Description = utils.SanitizeDescription(req.Description)

	// Post-sanitization validation: ensure name and key are not empty after sanitization
	if req.Name == "" {
		http.Error(w, "Workspace name is required", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "Workspace key is required", http.StatusBadRequest)
		return
	}

	// Default active to true if not specified
	isActive := true
	if req.Active != nil {
		isActive = *req.Active
	}

	// Default view to 'board' if not specified
	defaultView := req.DefaultView
	if defaultView == "" {
		defaultView = "board"
	}

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO workspaces (name, key, description, active, time_project_id, is_personal, owner_id, icon, color, avatar_url, default_view, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, req.Name, req.Key, req.Description, isActive, req.TimeProjectID, req.IsPersonal, req.OwnerID, req.Icon, req.Color, req.AvatarURL, defaultView, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create item number sequence for this workspace (PostgreSQL only, no-op for SQLite)
	if err := h.db.CreateWorkspaceItemSequence(id); err != nil {
		slog.Warn("failed to create item sequence for workspace", slog.String("component", "workspaces"), slog.Int64("workspace_id", id), slog.Any("error", err))
	}

	// Return the created workspace with joined data
	var workspace models.Workspace
	var timeProjectName, icon, color, defaultViewStr sql.NullString
	err = h.db.QueryRow(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at,
		       COUNT(p.id) as project_count,
		       tp.name as time_project_name
		FROM workspaces w
		LEFT JOIN projects p ON w.id = p.workspace_id
		LEFT JOIN time_projects tp ON w.time_project_id = tp.id
		WHERE w.id = ?
		GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at, tp.name
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &icon, &color, &workspace.AvatarURL, &defaultViewStr, &workspace.CreatedAt, &workspace.UpdatedAt, &workspace.ProjectCount, &timeProjectName)

	workspace.Icon = icon.String
	workspace.Color = color.String
	workspace.DefaultView = defaultViewStr.String
	workspace.TimeProjectName = timeProjectName.String

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionWorkspaceCreate,
			ResourceType: logger.ResourceWorkspace,
			ResourceID:   &workspace.ID,
			ResourceName: workspace.Name,
			Details: map[string]interface{}{
				"key":         workspace.Key,
				"description": workspace.Description,
				"is_personal": workspace.IsPersonal,
				"active":      workspace.Active,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, workspace)
}

func (h *WorkspaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check if user has permission to administer this workspace
	canAdmin, permErr := h.canAdminWorkspace(user.ID, id)
	if permErr != nil {
		http.Error(w, "Permission check failed: "+permErr.Error(), http.StatusInternalServerError)
		return
	}
	if !canAdmin {
		http.Error(w, "Insufficient permissions to update this workspace", http.StatusForbidden)
		return
	}

	// Get the old workspace for audit logging
	var oldWorkspace models.Workspace
	var oldIcon, oldColor sql.NullString
	err := h.db.QueryRow(`
		SELECT id, name, key, description, active, is_personal, icon, color
		FROM workspaces
		WHERE id = ?
	`, id).Scan(&oldWorkspace.ID, &oldWorkspace.Name, &oldWorkspace.Key, &oldWorkspace.Description, &oldWorkspace.Active, &oldWorkspace.IsPersonal, &oldIcon, &oldColor)

	if err == sql.ErrNoRows {
		http.Error(w, "Workspace not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	oldWorkspace.Icon = oldIcon.String
	oldWorkspace.Color = oldColor.String

	// Parse request
	var req UpdateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input using validator
	if err := utils.Validate(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If key is not provided, use the existing key
	keyToUse := req.Key
	if keyToUse == "" {
		keyToUse = oldWorkspace.Key
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE workspaces
		SET name = ?, key = ?, description = ?, active = ?, time_project_id = ?, is_personal = ?, owner_id = ?, icon = ?, color = ?, avatar_url = ?, default_view = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, keyToUse, req.Description, req.Active, req.TimeProjectID, req.IsPersonal, req.OwnerID, req.Icon, req.Color, req.AvatarURL, req.DefaultView, now, id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save time project categories if provided
	if req.TimeProjectCategories != nil {
		if err := h.saveTimeProjectCategories(id, req.TimeProjectCategories); err != nil {
			slog.Error("failed to save time project categories", slog.String("component", "workspaces"), slog.Int("workspace_id", id), slog.Any("error", err))
			// Don't fail the entire update, just log the error
		}
	}

	// Return the updated workspace with joined data
	var workspace models.Workspace
	var timeProjectName, icon, color, defaultView sql.NullString
	err = h.db.QueryRow(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at,
		       COUNT(p.id) as project_count,
		       tp.name as time_project_name
		FROM workspaces w
		LEFT JOIN projects p ON w.id = p.workspace_id
		LEFT JOIN time_projects tp ON w.time_project_id = tp.id
		WHERE w.id = ?
		GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.created_at, w.updated_at, tp.name
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &icon, &color, &workspace.AvatarURL, &defaultView, &workspace.CreatedAt, &workspace.UpdatedAt, &workspace.ProjectCount, &timeProjectName)

	workspace.Icon = icon.String
	workspace.Color = color.String
	workspace.DefaultView = defaultView.String
	workspace.TimeProjectName = timeProjectName.String

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Load time project categories for the response
	timeProjectCats, err := h.loadTimeProjectCategories(id)
	if err != nil {
		slog.Error("failed to load time project categories", slog.String("component", "workspaces"), slog.Int("workspace_id", id), slog.Any("error", err))
		// Don't fail the request, just log the error
		workspace.TimeProjectCategories = []int{} // Always include the field
	} else {
		workspace.TimeProjectCategories = timeProjectCats // Set even if empty
	}

	// Log audit event with change tracking
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldWorkspace.Name != workspace.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldWorkspace.Name,
				"new": workspace.Name,
			}
		}
		if oldWorkspace.Key != workspace.Key {
			details["key_changed"] = map[string]interface{}{
				"old": oldWorkspace.Key,
				"new": workspace.Key,
			}
		}
		if oldWorkspace.Description != workspace.Description {
			details["description_changed"] = map[string]interface{}{
				"old": oldWorkspace.Description,
				"new": workspace.Description,
			}
		}
		if oldWorkspace.Active != workspace.Active {
			details["active_changed"] = map[string]interface{}{
				"old": oldWorkspace.Active,
				"new": workspace.Active,
			}
		}
		if oldWorkspace.IsPersonal != workspace.IsPersonal {
			details["is_personal_changed"] = map[string]interface{}{
				"old": oldWorkspace.IsPersonal,
				"new": workspace.IsPersonal,
			}
		}
		if oldWorkspace.Icon != workspace.Icon {
			details["icon_changed"] = map[string]interface{}{
				"old": oldWorkspace.Icon,
				"new": workspace.Icon,
			}
		}
		if oldWorkspace.Color != workspace.Color {
			details["color_changed"] = map[string]interface{}{
				"old": oldWorkspace.Color,
				"new": workspace.Color,
			}
		}

		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionWorkspaceUpdate,
			ResourceType: logger.ResourceWorkspace,
			ResourceID:   &workspace.ID,
			ResourceName: workspace.Name,
			Details:      details,
			Success:      true,
		})
	}

	respondJSONOK(w, workspace)
}

func (h *WorkspaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check if user has permission to administer this workspace
	canAdmin, permErr := h.canAdminWorkspace(user.ID, id)
	if permErr != nil {
		http.Error(w, "Permission check failed: "+permErr.Error(), http.StatusInternalServerError)
		return
	}
	if !canAdmin {
		http.Error(w, "Insufficient permissions to delete this workspace", http.StatusForbidden)
		return
	}

	// Get the workspace details for audit logging before deletion
	var workspaceName, workspaceKey, description string
	var active, isPersonal bool
	err := h.db.QueryRow(`
		SELECT name, key, description, active, is_personal
		FROM workspaces
		WHERE id = ?
	`, id).Scan(&workspaceName, &workspaceKey, &description, &active, &isPersonal)

	if err == sql.ErrNoRows {
		http.Error(w, "Workspace not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Drop item number sequence for this workspace (PostgreSQL only, no-op for SQLite)
	if err := h.db.DropWorkspaceItemSequence(int64(id)); err != nil {
		slog.Warn("failed to drop item sequence for workspace", slog.String("component", "workspaces"), slog.Int("workspace_id", id), slog.Any("error", err))
	}

	_, err = h.db.ExecWrite("DELETE FROM workspaces WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionWorkspaceDelete,
			ResourceType: logger.ResourceWorkspace,
			ResourceID:   &id,
			ResourceName: workspaceName,
			Details: map[string]interface{}{
				"key":         workspaceKey,
				"description": description,
				"active":      active,
				"is_personal": isPersonal,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetOrCreatePersonalWorkspace gets or creates a personal workspace for a user
func (h *WorkspaceHandler) GetOrCreatePersonalWorkspace(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user from context
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	userID := user.ID
	userName := user.Username
	if userName == "" {
		userName = "User"
	}

	// Check if personal workspace already exists for this user
	var workspace models.Workspace
	var timeProjectName sql.NullString
	err := h.db.QueryRow(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at,
		       COUNT(p.id) as project_count,
		       tp.name as time_project_name
		FROM workspaces w
		LEFT JOIN projects p ON w.id = p.workspace_id
		LEFT JOIN time_projects tp ON w.time_project_id = tp.id
		WHERE w.is_personal = true AND w.owner_id = ?
		GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at, tp.name
	`, userID).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &workspace.CreatedAt, &workspace.UpdatedAt,
		&workspace.ProjectCount, &timeProjectName)

	if err == nil {
		// Personal workspace exists, return it
		workspace.TimeProjectName = timeProjectName.String
		respondJSONOK(w, workspace)
		return
	}

	if err != sql.ErrNoRows {
		// Database error occurred
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Personal workspace doesn't exist, create it
	// Use first name if available, otherwise fall back to username
	displayName := userName
	if user.FirstName != "" {
		displayName = user.FirstName
	}
	workspaceName := displayName + "'s Todo List"

	// Generate slugified workspace key derived from the user's name
	baseKey := h.generatePersonalWorkspaceKey(displayName, userName, userID)

	// Check for uniqueness and add counter if needed
	workspaceKey := baseKey
	counter := 1
	for {
		var exists bool
		checkErr := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE key = ?)", workspaceKey).Scan(&exists)
		if checkErr != nil || !exists {
			break
		}
		workspaceKey = baseKey + "-" + strconv.Itoa(counter)
		counter++
	}

	description := "Personal todo list and task management"

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO workspaces (name, key, description, active, time_project_id, is_personal, owner_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, workspaceName, workspaceKey, description, true, nil, true, userID, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create item number sequence for this workspace (PostgreSQL only, no-op for SQLite)
	if err := h.db.CreateWorkspaceItemSequence(id); err != nil {
		slog.Warn("failed to create item sequence for personal workspace", slog.String("component", "workspaces"), slog.Int64("workspace_id", id), slog.Any("error", err))
	}

	// Return the created personal workspace
	err = h.db.QueryRow(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at,
		       COUNT(p.id) as project_count,
		       tp.name as time_project_name
		FROM workspaces w
		LEFT JOIN projects p ON w.id = p.workspace_id
		LEFT JOIN time_projects tp ON w.time_project_id = tp.id
		WHERE w.id = ?
		GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at, tp.name
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &workspace.CreatedAt, &workspace.UpdatedAt, &workspace.ProjectCount, &timeProjectName)

	workspace.TimeProjectName = timeProjectName.String

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONCreated(w, workspace)
}

var personalWorkspaceKeySanitizer = regexp.MustCompile(`[^A-Za-z0-9]+`)

// generatePersonalWorkspaceKey builds a slug-like key (max ~10 chars) based on user identity.
func (h *WorkspaceHandler) generatePersonalWorkspaceKey(displayName, userName string, userID int) string {
	candidates := []string{displayName, userName}
	for _, candidate := range candidates {
		if key := sanitizePersonalWorkspaceKeyCandidate(candidate); key != "" {
			return key
		}
	}
	return sanitizePersonalWorkspaceKeyCandidate(fmt.Sprintf("USER-%d", userID))
}

func sanitizePersonalWorkspaceKeyCandidate(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	key := personalWorkspaceKeySanitizer.ReplaceAllString(strings.ToUpper(input), "-")
	key = strings.Trim(key, "-")

	// Keep workspace keys reasonably short to match create/update validation expectations.
	const maxKeyLength = 10
	if len(key) > maxKeyLength {
		key = key[:maxKeyLength]
		key = strings.Trim(key, "-")
	}

	if key == "" {
		return ""
	}

	return key
}

// Helper functions for permission checking

// canViewWorkspace checks if a user can view a workspace (has item.view permission)
func (h *WorkspaceHandler) canViewWorkspace(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		// If permission service is not available, allow access (backward compatibility)
		return true, nil
	}

	// HasWorkspacePermission now handles checking if workspace has restrictions
	// and returns true if workspace has no restrictions (accessible to all)
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
}

// canAdminWorkspace checks if a user can administer a workspace (has workspace.admin permission)
func (h *WorkspaceHandler) canAdminWorkspace(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		// If permission service is not available, allow access (backward compatibility)
		return true, nil
	}

	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
}

// canAccessInactiveWorkspace checks if a user can access an inactive workspace
// Returns true if user is system admin OR has workspace.admin permission for the workspace
func (h *WorkspaceHandler) canAccessInactiveWorkspace(user *models.User, workspaceID int) (bool, error) {
	// System admins can always access inactive workspaces
	if h.permissionService != nil {
		isSystemAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
		if err == nil && isSystemAdmin {
			return true, nil
		}
	}

	// Check if user has workspace admin permission for this specific workspace
	if h.permissionService == nil {
		// If permission service is not available, deny access to inactive workspaces for non-system-admins
		return false, nil
	}

	return h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionWorkspaceAdmin)
}

// getUserFromContext retrieves the authenticated user from the request context
func (h *WorkspaceHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value("user"); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}

// canCreateWorkspace checks if a user can create workspaces (has global workspace.create permission)
func (h *WorkspaceHandler) canCreateWorkspace(userID int) (bool, error) {
	if h.permissionService == nil {
		// If permission service is not available, allow access (backward compatibility)
		return true, nil
	}

	return h.permissionService.HasGlobalPermission(userID, models.PermissionWorkspaceCreate)
}

// filterWorkspacesByPermissions filters a list of workspaces based on user's view permissions
func (h *WorkspaceHandler) filterWorkspacesByPermissions(userID int, workspaces []models.Workspace) ([]models.Workspace, error) {
	if h.permissionService == nil {
		// No permission service, return all workspaces (backward compatibility)
		return workspaces, nil
	}

	// Filter workspaces based on permissions
	// HasWorkspacePermission handles checking if workspace has restrictions
	filtered := make([]models.Workspace, 0)
	for _, ws := range workspaces {
		// IMPORTANT: For inactive workspaces, pass them through for now
		// They will be filtered later by canAccessInactiveWorkspace
		// We need to include them here so they can be checked properly
		if !ws.Active {
			// Include inactive workspaces in the filtered list
			// They will be filtered out later unless user has admin access
			filtered = append(filtered, ws)
			continue
		}

		// For active workspaces, check normal view permission
		hasPermission, err := h.permissionService.HasWorkspacePermission(userID, ws.ID, models.PermissionItemView)
		if err != nil {
			return nil, fmt.Errorf("error checking permission for workspace %d: %w", ws.ID, err)
		}
		if hasPermission {
			filtered = append(filtered, ws)
		}
	}

	return filtered, nil
}

// WorkspaceStats represents comprehensive statistics for a workspace
type WorkspaceStats struct {
	TotalCollections       int                       `json:"total_collections"`
	ItemsByStatusCategory  map[string]int            `json:"items_by_status_category"`
	TotalItems             int                       `json:"total_items"`
	AssignmentDistribution []AssignmentStats         `json:"assignment_distribution"`
	ProjectStatistics      []ProjectStats            `json:"project_statistics"`
	PriorityBreakdown      map[string]int            `json:"priority_breakdown"`
	MilestoneProgress      []MilestoneStatusProgress `json:"milestone_progress"`
}

// AssignmentStats represents the distribution of items per assignee
type AssignmentStats struct {
	UserID       *int   `json:"user_id"`
	UserName     string `json:"user_name"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	ItemCount    int    `json:"item_count"`
	IsUnassigned bool   `json:"is_unassigned"`
}

// ProjectStats represents statistics for a specific project
type ProjectStats struct {
	ProjectID         *int    `json:"project_id"`
	ProjectName       string  `json:"project_name"`
	ProjectColor      string  `json:"project_color,omitempty"`
	ItemCount         int     `json:"item_count"`
	CompletedCount    int     `json:"completed_count"`
	CompletionPercent float64 `json:"completion_percent"`
}

// MilestoneStatusBreakdown represents the distribution of items per status category within a milestone
type MilestoneStatusBreakdown struct {
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color,omitempty"`
	ItemCount     int    `json:"item_count"`
	IsCompleted   bool   `json:"is_completed"`
}

// MilestoneStatusProgress aggregates milestone progress for a workspace
type MilestoneStatusProgress struct {
	MilestoneID     int                        `json:"milestone_id"`
	MilestoneName   string                     `json:"milestone_name"`
	TargetDate      *string                    `json:"target_date,omitempty"`
	Status          string                     `json:"status,omitempty"`
	CategoryColor   string                     `json:"category_color,omitempty"`
	TotalItems      int                        `json:"total_items"`
	CompletedItems  int                        `json:"completed_items"`
	PercentComplete float64                    `json:"percent_complete"`
	StatusBreakdown []MilestoneStatusBreakdown `json:"status_breakdown"`
}

// GetStats handles GET /api/workspaces/{id}/stats - returns comprehensive workspace statistics
func (h *WorkspaceHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	// Get workspace ID from URL
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context for permission check
	user := r.Context().Value("user")
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	queryParams := r.URL.Query()
	vqlQuery := queryParams.Get("vql")
	if vqlQuery == "" {
		vqlQuery = queryParams.Get("cql")
	}

	// Support filtering via collection_id by reusing its CQL query
	if vqlQuery == "" {
		if collectionParam := queryParams.Get("collection_id"); collectionParam != "" {
			collectionID, err := strconv.Atoi(collectionParam)
			if err != nil {
				http.Error(w, "Invalid collection_id", http.StatusBadRequest)
				return
			}

			var collectionWorkspaceID sql.NullInt64
			var collectionQuery sql.NullString
			err = h.db.QueryRow(`SELECT workspace_id, ql_query FROM collections WHERE id = ?`, collectionID).
				Scan(&collectionWorkspaceID, &collectionQuery)
			if err == sql.ErrNoRows {
				http.Error(w, "Collection not found", http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to load collection: %v", err), http.StatusInternalServerError)
				return
			}

			if collectionWorkspaceID.Valid && collectionWorkspaceID.Int64 != int64(workspaceID) {
				http.Error(w, "Collection does not belong to this workspace", http.StatusBadRequest)
				return
			}

			if collectionQuery.Valid && strings.TrimSpace(collectionQuery.String) != "" {
				vqlQuery = collectionQuery.String
			}
		}
	}

	var filterSQL string
	var filterArgs []interface{}
	if strings.TrimSpace(vqlQuery) != "" {
		workspaceMap, err := h.buildWorkspaceMap()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to load workspace mapping: %v", err), http.StatusInternalServerError)
			return
		}
		evaluator := cql.NewEvaluator(workspaceMap)
		filterSQL, filterArgs, err = evaluator.EvaluateToSQL(vqlQuery)
		if err != nil {
			http.Error(w, "VQL query error: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Calculate time window (last 30 days)
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30).Format("2006-01-02 15:04:05")

	stats := WorkspaceStats{
		ItemsByStatusCategory:  make(map[string]int),
		AssignmentDistribution: []AssignmentStats{},
		ProjectStatistics:      []ProjectStats{},
		PriorityBreakdown:      make(map[string]int),
		MilestoneProgress:      []MilestoneStatusProgress{},
	}

	// 1. Get total collections count
	var collectionCount int
	var err error
	err = h.db.QueryRow(`
		SELECT COUNT(*)
		FROM collections
		WHERE workspace_id = ?
	`, workspaceID).Scan(&collectionCount)
	if err != nil {
		slog.Error("failed to count collections", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
	}
	stats.TotalCollections = collectionCount + 1 // +1 for default collection

	// 2. Get total items count
	totalItemsQuery := `
		SELECT COUNT(*)
		FROM items i
		WHERE i.workspace_id = ?`
	totalItemsArgs := []interface{}{workspaceID}
	if filterSQL != "" {
		totalItemsQuery += " AND (" + filterSQL + ")"
		totalItemsArgs = append(totalItemsArgs, filterArgs...)
	}

	err = h.db.QueryRow(totalItemsQuery, totalItemsArgs...).Scan(&stats.TotalItems)
	if err != nil {
		slog.Error("failed to count items", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
	}

	// 3. Get items by status category
	statusQuery := `
		SELECT sc.name, COUNT(i.id) as item_count
		FROM items i
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?`
	if filterSQL != "" {
		statusQuery += " AND (" + filterSQL + ")"
	}
	statusQuery += `
		GROUP BY sc.name`
	statusArgs := []interface{}{workspaceID}
	if filterSQL != "" {
		statusArgs = append(statusArgs, filterArgs...)
	}

	rows, err := h.db.Query(statusQuery, statusArgs...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var categoryName sql.NullString
			var count int
			if err := rows.Scan(&categoryName, &count); err == nil {
				if categoryName.Valid {
					stats.ItemsByStatusCategory[categoryName.String] = count
				}
			}
		}
	}

	// 4. Get assignment distribution (last 30 days)
	assignmentQuery := `
		SELECT
			i.assignee_id,
			COALESCE(u.username, 'Unassigned') as user_name,
			COALESCE(u.first_name, '') as first_name,
			COALESCE(u.last_name, '') as last_name,
			COUNT(i.id) as item_count
		FROM items i
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`
	if filterSQL != "" {
		assignmentQuery += " AND (" + filterSQL + ")"
	}
	assignmentQuery += `
		GROUP BY i.assignee_id, u.username, u.first_name, u.last_name
		ORDER BY item_count DESC
		LIMIT 10`
	assignmentArgs := []interface{}{workspaceID, thirtyDaysAgo}
	if filterSQL != "" {
		assignmentArgs = append(assignmentArgs, filterArgs...)
	}

	assignmentRows, err := h.db.Query(assignmentQuery, assignmentArgs...)
	if err == nil {
		defer assignmentRows.Close()
		for assignmentRows.Next() {
			var assignment AssignmentStats
			var assigneeID sql.NullInt64
			if err := assignmentRows.Scan(&assigneeID, &assignment.UserName, &assignment.FirstName, &assignment.LastName, &assignment.ItemCount); err == nil {
				if assigneeID.Valid {
					id := int(assigneeID.Int64)
					assignment.UserID = &id
					assignment.IsUnassigned = false
				} else {
					assignment.IsUnassigned = true
				}
				stats.AssignmentDistribution = append(stats.AssignmentDistribution, assignment)
			}
		}
	}

	// 5. Get project statistics (last 30 days)
	projectQuery := `
		SELECT
			tp.id,
			tp.name,
			tp.color,
			COUNT(i.id) as item_count,
			SUM(CASE WHEN LOWER(sc.name) = 'done' THEN 1 ELSE 0 END) as completed_count
		FROM items i
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?
		  AND i.time_project_id IS NOT NULL`
	if filterSQL != "" {
		projectQuery += " AND (" + filterSQL + ")"
	}
	projectQuery += `
		GROUP BY tp.id, tp.name, tp.color
		ORDER BY item_count DESC
		LIMIT 10`
	projectArgs := []interface{}{workspaceID, thirtyDaysAgo}
	if filterSQL != "" {
		projectArgs = append(projectArgs, filterArgs...)
	}

	projectRows, err := h.db.Query(projectQuery, projectArgs...)
	if err == nil {
		defer projectRows.Close()
		for projectRows.Next() {
			var project ProjectStats
			var projectID sql.NullInt64
			var projectColor sql.NullString
			if err := projectRows.Scan(&projectID, &project.ProjectName, &projectColor, &project.ItemCount, &project.CompletedCount); err == nil {
				project.ProjectID = utils.NullInt64ToPtr(projectID)
				project.ProjectColor = projectColor.String
				if project.ItemCount > 0 {
					project.CompletionPercent = float64(project.CompletedCount) / float64(project.ItemCount) * 100
				}
				stats.ProjectStatistics = append(stats.ProjectStatistics, project)
			}
		}
	}

	// 6. Get priority breakdown (last 30 days)
	priorityQuery := `
		SELECT
			COALESCE(pri.name, 'None') as priority,
			COUNT(i.id) as item_count
		FROM items i
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`
	if filterSQL != "" {
		priorityQuery += " AND (" + filterSQL + ")"
	}
	priorityQuery += `
		GROUP BY pri.name`
	priorityArgs := []interface{}{workspaceID, thirtyDaysAgo}
	if filterSQL != "" {
		priorityArgs = append(priorityArgs, filterArgs...)
	}

	priorityRows, err := h.db.Query(priorityQuery, priorityArgs...)
	if err == nil {
		defer priorityRows.Close()
		for priorityRows.Next() {
			var priority string
			var count int
			if err := priorityRows.Scan(&priority, &count); err == nil {
				stats.PriorityBreakdown[priority] = count
			}
		}
	}

	// 7. Load milestone progress for active milestones referenced in this workspace
	if milestoneProgress, mpErr := h.loadWorkspaceMilestoneProgress(workspaceID, filterSQL, filterArgs); mpErr == nil {
		stats.MilestoneProgress = milestoneProgress
	} else {
		slog.Error("failed to load milestone progress", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", mpErr))
	}

	respondJSONOK(w, stats)
}

// loadWorkspaceMilestoneProgress aggregates milestone status counts for a workspace
func (h *WorkspaceHandler) loadWorkspaceMilestoneProgress(workspaceID int, filterSQL string, filterArgs []interface{}) ([]MilestoneStatusProgress, error) {
	query := `
		SELECT
			m.id,
			m.name,
			m.target_date,
			m.status,
			mc.color,
			sc.name,
			sc.color,
			sc.is_completed,
			COUNT(i.id) as item_count
		FROM items i
		JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?
		  AND i.milestone_id IS NOT NULL
		  AND (m.status IS NULL OR LOWER(m.status) <> 'completed')`

	args := []interface{}{workspaceID}
	if filterSQL != "" {
		query += " AND (" + filterSQL + ")"
		args = append(args, filterArgs...)
	}

	query += `
		GROUP BY m.id, m.name, m.target_date, m.status, mc.color, sc.name, sc.color, sc.is_completed
		ORDER BY m.target_date IS NULL, m.target_date, m.name`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	progressMap := make(map[int]*MilestoneStatusProgress)

	for rows.Next() {
		var milestoneID int
		var milestoneName string
		var targetDate sql.NullString
		var milestoneStatus sql.NullString
		var milestoneColor sql.NullString
		var categoryName sql.NullString
		var categoryColor sql.NullString
		var categoryCompleted sql.NullBool
		var itemCount int

		if err := rows.Scan(
			&milestoneID,
			&milestoneName,
			&targetDate,
			&milestoneStatus,
			&milestoneColor,
			&categoryName,
			&categoryColor,
			&categoryCompleted,
			&itemCount,
		); err != nil {
			return nil, err
		}

		if itemCount == 0 {
			continue
		}

		progress, exists := progressMap[milestoneID]
		if !exists {
			progress = &MilestoneStatusProgress{
				MilestoneID:     milestoneID,
				MilestoneName:   milestoneName,
				StatusBreakdown: []MilestoneStatusBreakdown{},
			}
			progress.TargetDate = utils.NullStringToPtr(targetDate)
			progress.Status = milestoneStatus.String
			progress.CategoryColor = milestoneColor.String
			progressMap[milestoneID] = progress
		}

		label := strings.TrimSpace(categoryName.String)
		if label == "" {
			label = "No Status"
		}

		breakdown := MilestoneStatusBreakdown{
			CategoryName:  label,
			ItemCount:     itemCount,
			IsCompleted:   categoryCompleted.Valid && categoryCompleted.Bool,
			CategoryColor: categoryColor.String,
		}

		progress.StatusBreakdown = append(progress.StatusBreakdown, breakdown)
		progress.TotalItems += itemCount
		if breakdown.IsCompleted {
			progress.CompletedItems += itemCount
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(progressMap) == 0 {
		return []MilestoneStatusProgress{}, nil
	}

	// Build a deterministic order: upcoming target date first, then name
	keys := make([]int, 0, len(progressMap))
	for id := range progressMap {
		keys = append(keys, id)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := progressMap[keys[i]]
		right := progressMap[keys[j]]

		if left.TargetDate == nil && right.TargetDate != nil {
			return false
		}
		if left.TargetDate != nil && right.TargetDate == nil {
			return true
		}
		if left.TargetDate != nil && right.TargetDate != nil && *left.TargetDate != *right.TargetDate {
			return *left.TargetDate < *right.TargetDate
		}
		return strings.ToLower(left.MilestoneName) < strings.ToLower(right.MilestoneName)
	})

	results := make([]MilestoneStatusProgress, 0, len(progressMap))
	for _, id := range keys {
		entry := progressMap[id]
		if entry.TotalItems > 0 {
			entry.PercentComplete = float64(entry.CompletedItems) / float64(entry.TotalItems) * 100.0
		}

		// Order breakdown by count desc to highlight most significant segments first
		sort.SliceStable(entry.StatusBreakdown, func(i, j int) bool {
			if entry.StatusBreakdown[i].ItemCount == entry.StatusBreakdown[j].ItemCount {
				return strings.ToLower(entry.StatusBreakdown[i].CategoryName) < strings.ToLower(entry.StatusBreakdown[j].CategoryName)
			}
			return entry.StatusBreakdown[i].ItemCount > entry.StatusBreakdown[j].ItemCount
		})

		results = append(results, *entry)
	}

	return results, nil
}

// GetHomepageLayout handles GET /api/workspaces/:id/homepage/layout
func (h *WorkspaceHandler) GetHomepageLayout(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user := r.Context().Value("user")
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	currentUser, ok := user.(*models.User)
	if !ok {
		http.Error(w, "Invalid user context", http.StatusInternalServerError)
		return
	}

	// Check if user has access to this workspace
	hasAccess, permErr := h.permissionService.HasWorkspacePermission(currentUser.ID, workspaceID, models.PermissionItemView)
	if permErr != nil || !hasAccess {
		http.Error(w, "Unauthorized access to workspace", http.StatusForbidden)
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
			http.Error(w, "Workspace not found", http.StatusNotFound)
			return
		}
		slog.Error("failed to get homepage layout", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
		http.Error(w, "Failed to get homepage layout", http.StatusInternalServerError)
		return
	}

	// If no layout exists, return empty structure
	var layout models.WorkspaceHomepageLayout
	if homepageLayout.Valid && homepageLayout.String != "" {
		if err := json.Unmarshal([]byte(homepageLayout.String), &layout); err != nil {
			slog.Error("failed to parse homepage layout JSON", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
			http.Error(w, "Failed to parse homepage layout", http.StatusInternalServerError)
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
	user := r.Context().Value("user")
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	currentUser, ok := user.(*models.User)
	if !ok {
		http.Error(w, "Invalid user context", http.StatusInternalServerError)
		return
	}

	// Check if user has admin access to this workspace
	hasAccess, permErr := h.permissionService.HasWorkspacePermission(currentUser.ID, workspaceID, models.PermissionWorkspaceAdmin)
	if permErr != nil || !hasAccess {
		http.Error(w, "Unauthorized: admin access required", http.StatusForbidden)
		return
	}

	// Parse request body
	var layout models.WorkspaceHomepageLayout
	if err := json.NewDecoder(r.Body).Decode(&layout); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
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
			http.Error(w, fmt.Sprintf("Invalid widget type: %s", widget.Type), http.StatusBadRequest)
			return
		}
		if widget.Width < 1 || widget.Width > 3 {
			http.Error(w, fmt.Sprintf("Invalid widget width: %d (must be 1-3)", widget.Width), http.StatusBadRequest)
			return
		}
	}

	// Convert to JSON
	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		slog.Error("failed to marshal homepage layout", slog.String("component", "workspaces"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
		http.Error(w, "Failed to encode homepage layout", http.StatusInternalServerError)
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
		http.Error(w, "Failed to update homepage layout", http.StatusInternalServerError)
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
	user := r.Context().Value("user")
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	currentUser, ok := user.(*models.User)
	if !ok {
		http.Error(w, "Invalid user context", http.StatusInternalServerError)
		return
	}

	// Check if user has permission to view this workspace
	canView, permErr := h.canViewWorkspace(currentUser.ID, workspaceID)
	if permErr != nil {
		http.Error(w, "Error checking permissions: "+permErr.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Insufficient permissions to view this workspace", http.StatusForbidden)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

// buildWorkspaceMap creates a mapping of workspace identifiers for VQL evaluation
func (h *WorkspaceHandler) buildWorkspaceMap() (map[string]int, error) {
	workspaceMap := make(map[string]int)

	rows, err := h.db.Query("SELECT id, name, key FROM workspaces")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, key string
		if err := rows.Scan(&id, &name, &key); err != nil {
			return nil, err
		}

		workspaceMap[strconv.Itoa(id)] = id
		workspaceMap[strings.ToLower(name)] = id
		workspaceMap[strings.ToLower(key)] = id
	}

	return workspaceMap, nil
}
