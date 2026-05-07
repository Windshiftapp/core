package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// PermissionHandler handles permission-related HTTP requests
type PermissionHandler struct {
	db                database.Database
	permissionService *services.PermissionService
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

	respondJSONOK(w, permissions)
}

// GetUserPermissions returns all permissions for a specific user
func (h *PermissionHandler) GetUserPermissions(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	var err error

	user, ok := RequireAuth(w, r)
	if !ok {
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

	respondJSONOK(w, summary)
}

// requireGlobalPermissionScope validates that the caller is authenticated and
// that the given permissionID refers to an existing global-scoped permission.
// It writes an HTTP error and returns (0, false) on failure.
func (h *PermissionHandler) requireGlobalPermissionScope(w http.ResponseWriter, r *http.Request, permissionID int) (int, bool) {
	granterID := h.getSessionUserID(r)
	if granterID == 0 {
		respondUnauthorized(w, r)
		return 0, false
	}

	var permissionScope string
	err := h.db.QueryRow("SELECT scope FROM permissions WHERE id = ?", permissionID).Scan(&permissionScope)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "permission")
		return 0, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return 0, false
	}

	if permissionScope != models.PermissionScopeGlobal {
		respondValidationError(w, r, "Permission is not a global permission")
		return 0, false
	}

	return granterID, true
}

// GrantGlobalPermission grants a global permission to a user
func (h *PermissionHandler) GrantGlobalPermission(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.PermissionRequest](w, r)
	if !ok {
		return
	}

	if req.WorkspaceID != nil {
		respondValidationError(w, r, "Workspace ID should not be provided for global permissions")
		return
	}

	granterID, ok := h.requireGlobalPermissionScope(w, r, req.PermissionID)
	if !ok {
		return
	}

	// Grant the permission (only if not already granted)
	_, err := h.db.ExecWrite(`
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
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	permissionID, ok := requireIDParam(w, r, "permissionId")
	if !ok {
		return
	}

	var err error

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

	granterID, ok := h.requireGlobalPermissionScope(w, r, req.PermissionID)
	if !ok {
		return
	}

	// Verify the group exists
	var groupExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE id = ?)", req.GroupID).Scan(&groupExists)
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
	groupID, ok := requireIDParam(w, r, "groupId")
	if !ok {
		return
	}

	permissionID, ok := requireIDParam(w, r, "permissionId")
	if !ok {
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

	// Get workspace permissions from explicit role assignments
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

	// Track already-added workspace permissions to avoid duplicates
	addedPerms := make(map[int]map[string]bool) // workspace_id -> permission_key -> true

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

		if addedPerms[uwp.WorkspaceID] == nil {
			addedPerms[uwp.WorkspaceID] = make(map[string]bool)
		}
		addedPerms[uwp.WorkspaceID][p.PermissionKey] = true
	}

	// Supplement with group-based and "Everyone" implicit permissions from the
	// permission cache.  The cache already resolves all three sources (explicit,
	// group, everyone) so we only need to add entries not already covered above.
	if h.permissionService != nil {
		effectiveCache, cacheErr := h.permissionService.GetUserEffectivePermissions(userID)
		if cacheErr == nil && !effectiveCache.IsSystemAdmin {
			// Build a permission-key → Permission lookup so we can populate the
			// Permission field on synthetic UserWorkspacePermission entries.
			permLookup := make(map[string]*models.Permission)
			permRows, permErr := h.db.Query(`
				SELECT id, permission_key, permission_name, description, scope, is_system, created_at, updated_at
				FROM permissions
			`)
			if permErr == nil {
				defer func() { _ = permRows.Close() }()
				for permRows.Next() {
					var p models.Permission
					if scanErr := permRows.Scan(&p.ID, &p.PermissionKey, &p.PermissionName, &p.Description, &p.Scope, &p.IsSystem, &p.CreatedAt, &p.UpdatedAt); scanErr == nil {
						cp := p // copy to avoid pointer reuse
						permLookup[p.PermissionKey] = &cp
					}
				}
			}

			// Build a workspace ID → Workspace lookup for workspaces we haven't seen yet.
			wsLookup := make(map[int]*models.Workspace)

			// Collect workspace IDs we may need from cache sources.
			needWSIDs := make(map[int]bool)
			for wsID, perms := range effectiveCache.WorkspacePermissions {
				for key := range perms {
					if addedPerms[wsID] == nil || !addedPerms[wsID][key] {
						needWSIDs[wsID] = true
					}
				}
			}
			for wsID, perms := range effectiveCache.WorkspaceEveryone {
				for key := range perms {
					if addedPerms[wsID] == nil || !addedPerms[wsID][key] {
						needWSIDs[wsID] = true
					}
				}
			}

			if len(needWSIDs) > 0 {
				wsRows, wsErr := h.db.Query(`SELECT id, name, description, key FROM workspaces`)
				if wsErr == nil {
					defer func() { _ = wsRows.Close() }()
					for wsRows.Next() {
						var w models.Workspace
						if scanErr := wsRows.Scan(&w.ID, &w.Name, &w.Description, &w.Key); scanErr == nil {
							if needWSIDs[w.ID] {
								cp := w
								wsLookup[w.ID] = &cp
							}
						}
					}
				}
			}

			// Helper to add a permission entry if not already present.
			addIfMissing := func(wsID int, permKey string) {
				if addedPerms[wsID] != nil && addedPerms[wsID][permKey] {
					return
				}
				p := permLookup[permKey]
				if p == nil {
					return
				}
				w := wsLookup[wsID]
				if w == nil {
					return
				}

				uwp := models.UserWorkspacePermission{
					UserID:       userID,
					WorkspaceID:  wsID,
					PermissionID: p.ID,
					Permission:   p,
					Workspace:    w,
					GrantedAt:    effectiveCache.CachedAt,
				}
				summary.WorkspacePermissions = append(summary.WorkspacePermissions, uwp)

				if addedPerms[wsID] == nil {
					addedPerms[wsID] = make(map[string]bool)
				}
				addedPerms[wsID][permKey] = true
			}

			// Add group-based workspace permissions
			for wsID, perms := range effectiveCache.WorkspacePermissions {
				for key := range perms {
					addIfMissing(wsID, key)
				}
			}

			// Add "Everyone" implicit workspace permissions
			for wsID, perms := range effectiveCache.WorkspaceEveryone {
				for key := range perms {
					addIfMissing(wsID, key)
				}
			}
		}
	}

	return summary, nil
}

// getSessionUserID extracts user ID from session context
func (h *PermissionHandler) getSessionUserID(r *http.Request) int {
	if user := utils.GetCurrentUser(r); user != nil {
		return user.ID
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

	respondJSONOK(w, groupPermissions)
}
