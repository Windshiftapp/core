package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type WorkspaceHandler struct {
	db                database.Database
	repo              *repository.WorkspaceRepository
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
		repo:              repository.NewWorkspaceRepository(db),
		permissionService: permissionService,
		activityTracker:   activityTracker,
	}
}

func (h *WorkspaceHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user := r.Context().Value(middleware.ContextKeyUser)
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

func (h *WorkspaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user := r.Context().Value(middleware.ContextKeyUser)
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

// getUserFromContext retrieves the authenticated user from the request context
func (h *WorkspaceHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}
