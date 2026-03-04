package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// PermissionHandler handles permission-related HTTP requests
type PermissionHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

// NewPermissionHandler creates a new permission handler
func NewPermissionHandler(db database.Database) *PermissionHandler {
	return &PermissionHandler{db: db}
}

// NewPermissionHandlerWithCache creates a new permission handler with cached permission service
func NewPermissionHandlerWithCache(db database.Database, permissionService *services.PermissionService) *PermissionHandler {
	return &PermissionHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// GetAllPermissions returns all available permissions
func (h *PermissionHandler) GetAllPermissions(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, permission_key, permission_name, description, scope, is_system, created_at, updated_at
		FROM permissions
		ORDER BY scope, permission_name
	`

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var permissions []models.Permission
	for rows.Next() {
		var p models.Permission
		err := rows.Scan(
			&p.ID, &p.PermissionKey, &p.PermissionName,
			&p.Description, &p.Scope, &p.IsSystem,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		permissions = append(permissions, p)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(permissions)
}

// GetUserPermissions returns all permissions for a specific user
func (h *PermissionHandler) GetUserPermissions(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
		return
	}

	// Get current user from context
	currentUser := r.Context().Value(middleware.ContextKeyUser)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	user, ok := currentUser.(*models.User)
	if !ok {
		respondInternalError(w, r, fmt.Errorf("invalid user context"))
		return
	}

	// Allow users to access their own permissions OR require system admin for others
	if user.ID != userID {
		var isSystemAdmin bool
		isSystemAdmin, err = h.permissionService.IsSystemAdmin(user.ID)
		if err != nil || !isSystemAdmin {
			respondForbidden(w, r)
			return
		}
	}

	summary, err := h.getUserPermissionSummary(userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summary)
}

// GrantGlobalPermission grants a global permission to a user
func (h *PermissionHandler) GrantGlobalPermission(w http.ResponseWriter, r *http.Request) {
	var req models.PermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.WorkspaceID != nil {
		respondValidationError(w, r, "Workspace ID should not be provided for global permissions")
		return
	}

	// Get the granter from session context
	granterID := h.getSessionUserID(r)
	if granterID == 0 {
		respondUnauthorized(w, r)
		return
	}

	// Verify the permission exists and is global
	var permissionScope string
	err := h.db.QueryRow("SELECT scope FROM permissions WHERE id = ?", req.PermissionID).Scan(&permissionScope)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "permission")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if permissionScope != models.PermissionScopeGlobal {
		respondValidationError(w, r, "Permission is not a global permission")
		return
	}

	// Grant the permission (only if not already granted)
	_, err = h.db.ExecWrite(`
		INSERT INTO user_global_permissions (user_id, permission_id, granted_by, granted_at)
		SELECT ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM user_global_permissions
			WHERE user_id = ? AND permission_id = ?
		)
	`, req.UserID, req.PermissionID, granterID, time.Now(), req.UserID, req.PermissionID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate permission cache for the user
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if err := h.permissionService.OnUserPermissionChanged(req.UserID); err != nil {
			warnings = append(warnings, createCacheWarning("permission", err, fmt.Sprintf("user_id:%d", req.UserID)))
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		// Get permission and target user details for audit log
		var permissionName, targetUsername string
		if err := h.db.QueryRow("SELECT permission_name FROM permissions WHERE id = ?", req.PermissionID).Scan(&permissionName); err != nil {
			slog.Warn("failed to look up permission name", slog.Any("error", err))
		}
		if err := h.db.QueryRow("SELECT username FROM users WHERE id = ?", req.UserID).Scan(&targetUsername); err != nil {
			slog.Warn("failed to look up username", slog.Any("error", err))
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionGrant,
			ResourceType: logger.ResourcePermission,
			ResourceID:   &req.PermissionID,
			ResourceName: permissionName,
			Details: map[string]interface{}{
				"target_user_id":  req.UserID,
				"target_username": targetUsername,
				"permission_id":   req.PermissionID,
				"permission_name": permissionName,
				"scope":           "global",
			},
			Success: true,
		})
	}

	respondJSONCreatedWithWarnings(w, map[string]string{"message": "Permission granted successfully"}, warnings)
}

// RevokeGlobalPermission removes a global permission from a user
func (h *PermissionHandler) RevokeGlobalPermission(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
		return
	}

	permissionID, err := strconv.Atoi(r.PathValue("permissionId"))
	if err != nil {
		respondInvalidID(w, r, "permissionId")
		return
	}

	// Don't allow revoking system admin from the last admin
	var permissionKey string
	err = h.db.QueryRow("SELECT permission_key FROM permissions WHERE id = ?", permissionID).Scan(&permissionKey)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if permissionKey == models.PermissionSystemAdmin {
		var adminCount int
		err = h.db.QueryRow(`
			SELECT COUNT(*) FROM user_global_permissions ugp
			JOIN permissions p ON ugp.permission_id = p.id
			WHERE p.permission_key = 'system.admin'
		`).Scan(&adminCount)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if adminCount <= 1 {
			respondForbidden(w, r)
			return
		}
	}

	// Revoke the permission
	result, err := h.db.ExecWrite(`
		DELETE FROM user_global_permissions
		WHERE user_id = ? AND permission_id = ?
	`, userID, permissionID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "permission")
		return
	}

	// Invalidate permission cache for the user
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if err := h.permissionService.OnUserPermissionChanged(userID); err != nil {
			warnings = append(warnings, createCacheWarning("permission", err, fmt.Sprintf("user_id:%d", userID)))
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		// Get permission and target user details for audit log
		var permissionName, targetUsername string
		if err := h.db.QueryRow("SELECT permission_name FROM permissions WHERE id = ?", permissionID).Scan(&permissionName); err != nil {
			slog.Warn("failed to look up permission name", slog.Any("error", err))
		}
		if err := h.db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&targetUsername); err != nil {
			slog.Warn("failed to look up username", slog.Any("error", err))
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionRevoke,
			ResourceType: logger.ResourcePermission,
			ResourceID:   &permissionID,
			ResourceName: permissionName,
			Details: map[string]interface{}{
				"target_user_id":  userID,
				"target_username": targetUsername,
				"permission_id":   permissionID,
				"permission_name": permissionName,
				"scope":           "global",
			},
			Success: true,
		})
	}

	respondJSONOKWithWarnings(w, map[string]string{"message": "Permission revoked successfully"}, warnings)
}

// GrantGlobalPermissionToGroup grants a global permission to a group
func (h *PermissionHandler) GrantGlobalPermissionToGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupID      int `json:"group_id"`
		PermissionID int `json:"permission_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Get the granter from session context
	granterID := h.getSessionUserID(r)
	if granterID == 0 {
		respondUnauthorized(w, r)
		return
	}

	// Verify the permission exists and is global
	var permissionScope string
	err := h.db.QueryRow("SELECT scope FROM permissions WHERE id = ?", req.PermissionID).Scan(&permissionScope)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "permission")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if permissionScope != models.PermissionScopeGlobal {
		respondValidationError(w, r, "Permission is not a global permission")
		return
	}

	// Verify the group exists
	var groupExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE id = ?)", req.GroupID).Scan(&groupExists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !groupExists {
		respondNotFound(w, r, "group")
		return
	}

	// Grant the permission (only if not already granted)
	_, err = h.db.ExecWrite(`
		INSERT INTO group_global_permissions (group_id, permission_id, granted_by, granted_at)
		SELECT ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM group_global_permissions
			WHERE group_id = ? AND permission_id = ?
		)
	`, req.GroupID, req.PermissionID, granterID, time.Now(), req.GroupID, req.PermissionID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate permission cache for all users in the group
	var warnings []models.APIWarning
	if h.permissionService != nil {
		var userIDs []int
		rows, err := h.db.Query("SELECT user_id FROM group_members WHERE group_id = ?", req.GroupID)
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var userID int
				if err := rows.Scan(&userID); err == nil {
					userIDs = append(userIDs, userID)
				}
			}

			// Invalidate cache for each user in the group
			for _, userID := range userIDs {
				if err := h.permissionService.OnUserPermissionChanged(userID); err != nil {
					warnings = append(warnings, createCacheWarning("permission", err, fmt.Sprintf("user_id:%d,group_id:%d", userID, req.GroupID)))
				}
			}
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		var permissionName, groupName string
		if err := h.db.QueryRow("SELECT permission_name FROM permissions WHERE id = ?", req.PermissionID).Scan(&permissionName); err != nil {
			slog.Warn("failed to look up permission name", slog.Any("error", err))
		}
		if err := h.db.QueryRow("SELECT name FROM groups WHERE id = ?", req.GroupID).Scan(&groupName); err != nil {
			slog.Warn("failed to look up group name", slog.Any("error", err))
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionGrant,
			ResourceType: logger.ResourcePermission,
			ResourceID:   &req.PermissionID,
			ResourceName: permissionName,
			Details: map[string]interface{}{
				"target_group_id":   req.GroupID,
				"target_group_name": groupName,
				"permission_id":     req.PermissionID,
				"permission_name":   permissionName,
				"scope":             "global",
			},
			Success: true,
		})
	}

	respondJSONCreatedWithWarnings(w, map[string]string{"message": "Permission granted to group successfully"}, warnings)
}

// RevokeGlobalPermissionFromGroup removes a global permission from a group
func (h *PermissionHandler) RevokeGlobalPermissionFromGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupId"))
	if err != nil {
		respondInvalidID(w, r, "groupId")
		return
	}

	permissionID, err := strconv.Atoi(r.PathValue("permissionId"))
	if err != nil {
		respondInvalidID(w, r, "permissionId")
		return
	}

	// Revoke the permission
	result, err := h.db.ExecWrite(`
		DELETE FROM group_global_permissions
		WHERE group_id = ? AND permission_id = ?
	`, groupID, permissionID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "permission")
		return
	}

	// Invalidate permission cache for all users in the group
	var warnings []models.APIWarning
	if h.permissionService != nil {
		var userIDs []int
		rows, err := h.db.Query("SELECT user_id FROM group_members WHERE group_id = ?", groupID)
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var userID int
				if err := rows.Scan(&userID); err == nil {
					userIDs = append(userIDs, userID)
				}
			}

			// Invalidate cache for each user in the group
			for _, userID := range userIDs {
				if err := h.permissionService.OnUserPermissionChanged(userID); err != nil {
					warnings = append(warnings, createCacheWarning("permission", err, fmt.Sprintf("user_id:%d,group_id:%d", userID, groupID)))
				}
			}
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		var permissionName, groupName string
		if err := h.db.QueryRow("SELECT permission_name FROM permissions WHERE id = ?", permissionID).Scan(&permissionName); err != nil {
			slog.Warn("failed to look up permission name", slog.Any("error", err))
		}
		if err := h.db.QueryRow("SELECT name FROM groups WHERE id = ?", groupID).Scan(&groupName); err != nil {
			slog.Warn("failed to look up group name", slog.Any("error", err))
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionRevoke,
			ResourceType: logger.ResourcePermission,
			ResourceID:   &permissionID,
			ResourceName: permissionName,
			Details: map[string]interface{}{
				"target_group_id":   groupID,
				"target_group_name": groupName,
				"permission_id":     permissionID,
				"permission_name":   permissionName,
				"scope":             "global",
			},
			Success: true,
		})
	}

	respondJSONOKWithWarnings(w, map[string]string{"message": "Permission revoked from group successfully"}, warnings)
}

// getUserPermissionSummary gets a complete permission summary for a user
func (h *PermissionHandler) getUserPermissionSummary(userID int) (*models.UserPermissionSummary, error) {
	summary := &models.UserPermissionSummary{
		UserID:               userID,
		GlobalPermissions:    []models.UserGlobalPermission{},    // Initialize as empty slice, not nil
		WorkspacePermissions: []models.UserWorkspacePermission{}, // Initialize as empty slice, not nil
	}

	// Get user info
	var user models.User
	err := h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active
		FROM users WHERE id = ?
	`, userID).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName, &user.IsActive)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	summary.User = &user

	// Get global permissions
	globalQuery := `
		SELECT ugp.id, ugp.user_id, ugp.permission_id, ugp.granted_by, ugp.granted_at,
		       p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at
		FROM user_global_permissions ugp
		JOIN permissions p ON ugp.permission_id = p.id
		WHERE ugp.user_id = ?
		ORDER BY p.permission_name
	`

	rows, err := h.db.Query(globalQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get global permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var ugp models.UserGlobalPermission
		var p models.Permission

		err = rows.Scan(
			&ugp.ID, &ugp.UserID, &ugp.PermissionID, &ugp.GrantedBy, &ugp.GrantedAt,
			&p.ID, &p.PermissionKey, &p.PermissionName, &p.Description, &p.Scope, &p.IsSystem, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			continue
		}

		ugp.Permission = &p
		summary.GlobalPermissions = append(summary.GlobalPermissions, ugp)

		if p.PermissionKey == models.PermissionSystemAdmin {
			summary.HasSystemAdmin = true
		}
	}

	// Get permissions inherited from groups
	groupPermissionsQuery := `
		SELECT DISTINCT ggp.id, ggp.permission_id, ggp.granted_by, ggp.granted_at,
		       p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at
		FROM group_members gm
		JOIN group_global_permissions ggp ON gm.group_id = ggp.group_id
		JOIN permissions p ON ggp.permission_id = p.id
		JOIN groups g ON gm.group_id = g.id
		WHERE gm.user_id = ? AND g.is_active = true
		ORDER BY p.permission_name
	`

	groupRows, err := h.db.Query(groupPermissionsQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group permissions: %w", err)
	}
	defer func() { _ = groupRows.Close() }()

	for groupRows.Next() {
		var ugp models.UserGlobalPermission
		var p models.Permission

		err = groupRows.Scan(
			&ugp.ID, &ugp.PermissionID, &ugp.GrantedBy, &ugp.GrantedAt,
			&p.ID, &p.PermissionKey, &p.PermissionName, &p.Description, &p.Scope, &p.IsSystem, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			continue
		}

		// Set UserID to the queried user (not the group)
		ugp.UserID = userID
		ugp.Permission = &p
		summary.GlobalPermissions = append(summary.GlobalPermissions, ugp)

		if p.PermissionKey == models.PermissionSystemAdmin {
			summary.HasSystemAdmin = true
		}
	}

	// Get workspace permissions from role assignments
	// Query user's role assignments and derive permissions from roles
	workspaceQuery := `
		SELECT uwr.workspace_id, uwr.role_id, uwr.granted_by, uwr.granted_at,
		       p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at,
		       w.id, w.name, w.description, w.key
		FROM user_workspace_roles uwr
		JOIN role_permissions rp ON uwr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		JOIN workspaces w ON uwr.workspace_id = w.id
		WHERE uwr.user_id = ?
		ORDER BY w.name, p.permission_name
	`

	rows, err = h.db.Query(workspaceQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var uwp models.UserWorkspacePermission
		var p models.Permission
		var w models.Workspace
		var roleID int

		err := rows.Scan(
			&uwp.WorkspaceID, &roleID, &uwp.GrantedBy, &uwp.GrantedAt,
			&p.ID, &p.PermissionKey, &p.PermissionName, &p.Description, &p.Scope, &p.IsSystem, &p.CreatedAt, &p.UpdatedAt,
			&w.ID, &w.Name, &w.Description, &w.Key,
		)
		if err != nil {
			continue
		}

		uwp.UserID = userID
		uwp.PermissionID = p.ID
		uwp.Permission = &p
		uwp.Workspace = &w
		summary.WorkspacePermissions = append(summary.WorkspacePermissions, uwp)
	}

	return summary, nil
}

// getSessionUserID extracts user ID from session context
func (h *PermissionHandler) getSessionUserID(r *http.Request) int {
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u.ID
		}
	}
	return 0
}

// GetAllGroupPermissions returns all group permission assignments
func (h *PermissionHandler) GetAllGroupPermissions(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT ggp.group_id, ggp.permission_id, ggp.granted_by, ggp.granted_at
		FROM group_global_permissions ggp
		ORDER BY ggp.group_id, ggp.permission_id
	`

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type GroupPermission struct {
		GroupID      int    `json:"group_id"`
		PermissionID int    `json:"permission_id"`
		GrantedBy    *int   `json:"granted_by"`
		GrantedAt    string `json:"granted_at"`
	}

	// Initialize as empty slice to ensure JSON encoding returns [] instead of null
	groupPermissions := make([]GroupPermission, 0)
	for rows.Next() {
		var gp GroupPermission
		err := rows.Scan(&gp.GroupID, &gp.PermissionID, &gp.GrantedBy, &gp.GrantedAt)
		if err != nil {
			continue
		}
		groupPermissions = append(groupPermissions, gp)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(groupPermissions)
}
