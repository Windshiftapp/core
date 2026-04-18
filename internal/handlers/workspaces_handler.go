package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
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
	keyCache          *WorkspaceKeyCache
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
	Name                    string `json:"name" validate:"required,max=100"`
	Key                     string `json:"key" validate:"omitempty,min=2,max=10,alphanum"` // Optional - if not provided, keeps existing key
	Description             string `json:"description" validate:"max=500"`
	Active                  bool   `json:"active"`
	TimeProjectID           *int   `json:"time_project_id,omitempty"`
	IsPersonal              bool   `json:"is_personal"`
	OwnerID                 *int   `json:"owner_id,omitempty"`
	Icon                    string `json:"icon,omitempty"`
	Color                   string `json:"color,omitempty"`
	AvatarURL               string `json:"avatar_url,omitempty"`
	DefaultView             string `json:"default_view,omitempty"` // Default view when entering workspace (board, backlog, list, tree, map)
	InternalCommentsEnabled bool   `json:"internal_comments_enabled"`
	TimeProjectCategories   []int  `json:"time_project_categories,omitempty"`
}

func NewWorkspaceHandler(db database.Database, permissionService *services.PermissionService, activityTracker *services.ActivityTracker, keyCache *WorkspaceKeyCache) *WorkspaceHandler {
	return &WorkspaceHandler{
		db:                db,
		repo:              repository.NewWorkspaceRepository(db),
		permissionService: permissionService,
		activityTracker:   activityTracker,
		keyCache:          keyCache,
	}
}

func (h *WorkspaceHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check for is_personal query parameter
	isPersonalOnly := r.URL.Query().Get("is_personal") == "true"

	workspaces, err := h.repo.FindAll(currentUser.ID, isPersonalOnly)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Filter workspaces by permission
	filteredWorkspaces, err := h.filterWorkspacesByPermissions(currentUser.ID, workspaces)
	if err != nil {
		respondInternalError(w, r, err)
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
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "id")
	if !ok {
		return
	}

	workspace, err := h.repo.FindByID(workspaceID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "workspace")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check permissions based on workspace state
	if !workspace.Active {
		// For inactive workspaces, check if user has admin access
		canAccess, err := h.canAccessInactiveWorkspace(currentUser, workspace.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canAccess {
			respondNotFound(w, r, "workspace")
			return
		}
	} else {
		// For active workspaces, check if user has view permission
		canView, err := h.canViewWorkspace(currentUser.ID, workspace.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canView {
			respondNotFound(w, r, "workspace")
			return
		}
	}

	// Track workspace visit
	if h.activityTracker != nil {
		if err = h.activityTracker.TrackWorkspaceVisit(currentUser.ID, workspace.ID); err != nil {
			slog.Error("failed to track workspace visit", slog.String("component", "workspaces"), slog.Int("user_id", currentUser.ID), slog.Int("workspace_id", workspace.ID), slog.Any("error", err))
			// Don't fail the request, just log the error
		}
	}

	// Load time project categories for this workspace
	timeProjectCats, err := h.repo.GetTimeProjectCategories(workspace.ID)
	if err != nil {
		slog.Error("failed to load time project categories", slog.String("component", "workspaces"), slog.Int("workspace_id", workspace.ID), slog.Any("error", err))
		// Don't fail the request, just log the error
		workspace.TimeProjectCategories = []int{} // Always include the field
	} else {
		workspace.TimeProjectCategories = timeProjectCats // Set even if empty
	}

	respondJSONOK(w, workspace)
}

func (h *WorkspaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Require authentication
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check if user has permission to create workspaces
	canCreate, err := h.canCreateWorkspace(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canCreate {
		respondForbidden(w, r)
		return
	}

	// Parse request
	req, ok := decodeJSON[CreateWorkspaceRequest](w, r)
	if !ok {
		return
	}

	// Validate input using validator
	if err = utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Sanitize for defense in depth
	req.Name = utils.SanitizeName(req.Name)
	req.Key = utils.SanitizeName(req.Key)
	req.Description = utils.SanitizeDescription(req.Description)

	// Post-sanitization validation: ensure name and key are not empty after sanitization
	if req.Name == "" {
		respondValidationError(w, r, "Workspace name is required")
		return
	}
	if req.Key == "" {
		respondValidationError(w, r, "Workspace key is required")
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

	avatarURL := req.AvatarURL
	ws := &models.Workspace{
		Name:          req.Name,
		Key:           req.Key,
		Description:   req.Description,
		Active:        isActive,
		TimeProjectID: req.TimeProjectID,
		IsPersonal:    req.IsPersonal,
		OwnerID:       req.OwnerID,
		Icon:          req.Icon,
		Color:         req.Color,
		AvatarURL:     &avatarURL,
		DefaultView:   defaultView,
	}
	id, err := database.WithTxResult(h.db, func(tx database.Tx) (int64, error) {
		newID, err := h.repo.CreateTx(tx, ws)
		if err != nil {
			return 0, err
		}

		result, err := tx.Exec(`
			INSERT INTO user_workspace_roles (workspace_id, user_id, role_id, granted_by, granted_at)
			SELECT ?, ?, id, ?, CURRENT_TIMESTAMP FROM workspace_roles WHERE name = 'Administrator'
		`, newID, user.ID, user.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to grant admin role to workspace creator: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return 0, fmt.Errorf("administrator role not found; workspace creation aborted")
		}
		return newID, nil
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "A workspace with this key already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Create item number sequence for this workspace (PostgreSQL only, no-op for SQLite)
	if err = h.db.CreateWorkspaceItemSequence(id); err != nil {
		slog.Warn("failed to create item sequence for workspace", slog.String("component", "workspaces"), slog.Int64("workspace_id", id), slog.Any("error", err))
	}

	// Return the created workspace with joined data
	workspace, err := h.repo.FindByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate workspace key cache
	h.keyCache.Invalidate()

	// Log audit event
	h.logWorkspaceAudit(r, logger.ActionWorkspaceCreate, &workspace.ID, workspace.Name, workspace.Key, workspace.Description, workspace.Active, workspace.IsPersonal)

	respondJSONCreated(w, workspace)
}

func (h *WorkspaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireWorkspaceAdminAccess(w, r)
	if !ok {
		return
	}

	// Get the old workspace for audit logging
	oldWorkspace, err := h.repo.FindByIDBasic(id)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "workspace")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Parse request
	req, ok := decodeJSON[UpdateWorkspaceRequest](w, r)
	if !ok {
		return
	}

	// Validate input using validator
	if err = utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Sanitize user input for defense in depth
	req.Name = utils.SanitizeName(req.Name)
	req.Description = utils.SanitizeDescription(req.Description)

	// Sanitize key to match Create behavior
	req.Key = utils.SanitizeName(req.Key)

	// If key is not provided, use the existing key
	keyToUse := req.Key
	if keyToUse == "" {
		keyToUse = oldWorkspace.Key
	}

	avatarURL := req.AvatarURL
	updatedWs := &models.Workspace{
		ID:                      id,
		Name:                    req.Name,
		Key:                     keyToUse,
		Description:             req.Description,
		Active:                  req.Active,
		TimeProjectID:           req.TimeProjectID,
		IsPersonal:              req.IsPersonal,
		OwnerID:                 req.OwnerID,
		Icon:                    req.Icon,
		Color:                   req.Color,
		AvatarURL:               &avatarURL,
		DefaultView:             req.DefaultView,
		InternalCommentsEnabled: req.InternalCommentsEnabled,
	}
	err = h.repo.Update(updatedWs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save time project categories if provided
	if req.TimeProjectCategories != nil {
		if err = h.repo.SaveTimeProjectCategories(id, req.TimeProjectCategories); err != nil {
			slog.Error("failed to save time project categories", slog.String("component", "workspaces"), slog.Int("workspace_id", id), slog.Any("error", err))
			// Don't fail the entire update, just log the error
		}
	}

	// Return the updated workspace with joined data
	workspace, err := h.repo.FindByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load time project categories for the response
	timeProjectCats, err := h.repo.GetTimeProjectCategories(id)
	if err != nil {
		slog.Error("failed to load time project categories", slog.String("component", "workspaces"), slog.Int("workspace_id", id), slog.Any("error", err))
		// Don't fail the request, just log the error
		workspace.TimeProjectCategories = []int{} // Always include the field
	} else {
		workspace.TimeProjectCategories = timeProjectCats // Set even if empty
	}

	// Invalidate workspace key cache (key may have changed)
	h.keyCache.Invalidate()

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
		if oldWorkspace.InternalCommentsEnabled != workspace.InternalCommentsEnabled {
			details["internal_comments_enabled_changed"] = map[string]interface{}{
				"old": oldWorkspace.InternalCommentsEnabled,
				"new": workspace.InternalCommentsEnabled,
			}
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
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
	id, ok := h.requireWorkspaceAdminAccess(w, r)
	if !ok {
		return
	}

	// Get the workspace details for audit logging before deletion
	auditWorkspace, err := h.repo.FindByIDBasic(id)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "workspace")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Drop item number sequence for this workspace (PostgreSQL only, no-op for SQLite)
	if err = h.db.DropWorkspaceItemSequence(int64(id)); err != nil {
		slog.Warn("failed to drop item sequence for workspace", slog.String("component", "workspaces"), slog.Int("workspace_id", id), slog.Any("error", err))
	}

	err = h.repo.Delete(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate workspace key cache
	h.keyCache.Invalidate()

	// Log audit event
	h.logWorkspaceAudit(r, logger.ActionWorkspaceDelete, &id, auditWorkspace.Name, auditWorkspace.Key, auditWorkspace.Description, auditWorkspace.Active, auditWorkspace.IsPersonal)

	w.WriteHeader(http.StatusNoContent)
}

// logWorkspaceAudit logs an audit event for workspace create/delete operations
// that share the same details structure (key, description, active, is_personal).
func (h *WorkspaceHandler) logWorkspaceAudit(r *http.Request, actionType string, resourceID *int, resourceName, key, description string, active, isPersonal bool) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		return
	}
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   actionType,
		ResourceType: logger.ResourceWorkspace,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details: map[string]interface{}{
			"key":         key,
			"description": description,
			"active":      active,
			"is_personal": isPersonal,
		},
		Success: true,
	})
}

// requireWorkspaceAdminAccess extracts the workspace ID from the request path,
// authenticates the user, and verifies admin permission on the workspace.
// Returns the workspace ID, authenticated user, and true on success.
// Writes an appropriate HTTP error and returns false on failure.
func (h *WorkspaceHandler) requireWorkspaceAdminAccess(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, ok := requireWorkspaceIDParam(w, r, h.keyCache, "id")
	if !ok {
		return 0, false
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return 0, false
	}

	canAdmin, permErr := h.canAdminWorkspace(user.ID, id)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return 0, false
	}
	if !canAdmin {
		respondForbidden(w, r)
		return 0, false
	}

	return id, true
}
