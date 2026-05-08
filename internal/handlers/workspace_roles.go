package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type WorkspaceRoleHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
	approvalService   *services.ApprovalService
}

func NewWorkspaceRoleHandlerWithPool(db database.Database, permissionService *services.PermissionService) *WorkspaceRoleHandler {
	return &WorkspaceRoleHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

// SetApprovalService wires ApprovalService for the role-delete impact check
// (refuses delete when in-flight approvals snapshot this role). Optional;
// when nil, the impact check is skipped.
func (h *WorkspaceRoleHandler) SetApprovalService(svc *services.ApprovalService) {
	h.approvalService = svc
}

// GetAll returns all workspace roles
func (h *WorkspaceRoleHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	query := `
		SELECT id, name, description, is_system, permissions_enabled, display_order, created_at, updated_at
		FROM workspace_roles
		ORDER BY display_order ASC, name ASC`

	rows, err := db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var roles []models.WorkspaceRole
	for rows.Next() {
		var role models.WorkspaceRole
		err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem,
			&role.PermissionsEnabled, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		roles = append(roles, role)
	}

	if roles == nil {
		roles = []models.WorkspaceRole{}
	}

	respondJSONOK(w, roles)
}

// Get returns a single workspace role with its permissions
func (h *WorkspaceRoleHandler) Get(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	var role models.WorkspaceRole
	err = db.QueryRow(`
		SELECT id, name, description, is_system, permissions_enabled, display_order, created_at, updated_at
		FROM workspace_roles
		WHERE id = ?
	`, id).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem,
		&role.PermissionsEnabled, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "workspace_role")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load permissions for this role
	permRows, err := db.Query(`
		SELECT p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at
		FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		WHERE rp.role_id = ?
		ORDER BY p.scope, p.permission_name
	`, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = permRows.Close() }()

	role.Permissions = []models.Permission{}
	for permRows.Next() {
		var perm models.Permission
		err := permRows.Scan(&perm.ID, &perm.PermissionKey, &perm.PermissionName,
			&perm.Description, &perm.Scope, &perm.IsSystem, &perm.CreatedAt, &perm.UpdatedAt)
		if err == nil {
			role.Permissions = append(role.Permissions, perm)
		}
	}

	respondJSONOK(w, role)
}

// AssignRoleToUser assigns a role to a user in a workspace
func (h *WorkspaceRoleHandler) AssignRoleToUser(w http.ResponseWriter, r *http.Request) {
	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}
	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.UserRoleAssignmentRequest](w, r)
	if !ok {
		return
	}

	// Get current user ID
	granterID := h.getSessionUserID(r)
	if granterID == 0 {
		respondUnauthorized(w, r)
		return
	}

	// Check if role exists
	var roleExists bool
	err := readDB.QueryRow("SELECT EXISTS(SELECT 1 FROM workspace_roles WHERE id = ?)", req.RoleID).Scan(&roleExists)
	if err != nil || !roleExists {
		respondNotFound(w, r, "role")
		return
	}

	// Count existing assignments for this role+workspace before the operation
	var countBefore int
	_ = readDB.QueryRow(`
		SELECT COUNT(*) FROM user_workspace_roles WHERE workspace_id = ? AND role_id = ?
		UNION ALL
		SELECT COUNT(*) FROM group_workspace_roles WHERE workspace_id = ? AND role_id = ?
	`, req.WorkspaceID, req.RoleID, req.WorkspaceID, req.RoleID).Scan(&countBefore)

	// Insert or update role assignment
	_, err = writeDB.Exec(`
		INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, workspace_id, role_id) DO UPDATE SET granted_by = ?, granted_at = ?
	`, req.UserID, req.WorkspaceID, req.RoleID, granterID, time.Now(), granterID, time.Now())

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate cache: if this is the first assignment for this role+workspace,
	// everyone's implicit access changed → full cache reset.
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if countBefore == 0 {
			h.permissionService.OnEveryoneAccessChanged()
		} else {
			if err := h.permissionService.OnUserPermissionChanged(req.UserID); err != nil {
				warnings = append(warnings, createCacheWarning("permission", err, fmt.Sprintf("user_id:%d", req.UserID)))
			}
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		// Get role, target user, and workspace details for audit log
		var roleName, targetUsername, workspaceName string
		_ = readDB.QueryRow("SELECT name FROM workspace_roles WHERE id = ?", req.RoleID).Scan(&roleName)
		_ = readDB.QueryRow("SELECT username FROM users WHERE id = ?", req.UserID).Scan(&targetUsername)
		_ = readDB.QueryRow("SELECT name FROM workspaces WHERE id = ?", req.WorkspaceID).Scan(&workspaceName)

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionRoleAssign,
			ResourceType: logger.ResourceRole,
			ResourceID:   &req.RoleID,
			ResourceName: roleName,
			Details: map[string]interface{}{
				"target_user_id":  req.UserID,
				"target_username": targetUsername,
				"role_id":         req.RoleID,
				"role_name":       roleName,
				"workspace_id":    req.WorkspaceID,
				"workspace_name":  workspaceName,
			},
			Success: true,
		})
	}

	respondJSONCreatedWithWarnings(w, map[string]string{"message": "Role assigned successfully"}, warnings)
}

// RevokeRoleFromUser revokes a role from a user in a workspace
func (h *WorkspaceRoleHandler) RevokeRoleFromUser(w http.ResponseWriter, r *http.Request) {
	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}
	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	roleID, ok := requireIDParam(w, r, "roleId")
	if !ok {
		return
	}

	// Count existing assignments for this role+workspace before the operation
	var countBefore int
	_ = readDB.QueryRow(`
		SELECT (SELECT COUNT(*) FROM user_workspace_roles WHERE workspace_id = ? AND role_id = ?)
		     + (SELECT COUNT(*) FROM group_workspace_roles WHERE workspace_id = ? AND role_id = ?)
	`, workspaceID, roleID, workspaceID, roleID).Scan(&countBefore)

	result, err := writeDB.Exec(`
		DELETE FROM user_workspace_roles
		WHERE user_id = ? AND workspace_id = ? AND role_id = ?
	`, userID, workspaceID, roleID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "role_assignment")
		return
	}

	// Invalidate cache: if this was the last assignment for this role+workspace,
	// everyone's implicit access changed → full cache reset.
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if countBefore == 1 {
			// Was the only assignment, now removed → role becomes open to everyone
			h.permissionService.OnEveryoneAccessChanged()
		} else {
			if err := h.permissionService.OnUserPermissionChanged(userID); err != nil {
				warnings = append(warnings, createCacheWarning("permission", err, fmt.Sprintf("user_id:%d", userID)))
			}
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		// Get role, target user, and workspace details for audit log
		var roleName, targetUsername, workspaceName string
		_ = readDB.QueryRow("SELECT name FROM workspace_roles WHERE id = ?", roleID).Scan(&roleName)
		_ = readDB.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&targetUsername)
		_ = readDB.QueryRow("SELECT name FROM workspaces WHERE id = ?", workspaceID).Scan(&workspaceName)

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionRoleRevoke,
			ResourceType: logger.ResourceRole,
			ResourceID:   &roleID,
			ResourceName: roleName,
			Details: map[string]interface{}{
				"target_user_id":  userID,
				"target_username": targetUsername,
				"role_id":         roleID,
				"role_name":       roleName,
				"workspace_id":    workspaceID,
				"workspace_name":  workspaceName,
			},
			Success: true,
		})
	}

	// Note: RevokeRoleFromUser returns 204 No Content on success
	// If there are warnings, we return 200 with the warnings in body instead
	if len(warnings) > 0 {
		respondJSONOKWithWarnings(w, map[string]string{"message": "Role revoked successfully"}, warnings)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetUserRolesInWorkspace returns all roles assigned to a user in a workspace
func (h *WorkspaceRoleHandler) GetUserRolesInWorkspace(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	rows, err := db.Query(`
		SELECT wr.id, wr.name, wr.description, wr.is_system, wr.display_order, wr.created_at, wr.updated_at
		FROM workspace_roles wr
		JOIN user_workspace_roles uwr ON wr.id = uwr.role_id
		WHERE uwr.user_id = ? AND uwr.workspace_id = ?
		ORDER BY wr.display_order ASC
	`, userID, workspaceID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var roles []models.WorkspaceRole
	for rows.Next() {
		var role models.WorkspaceRole
		err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem,
			&role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
		if err == nil {
			roles = append(roles, role)
		}
	}

	if roles == nil {
		roles = []models.WorkspaceRole{}
	}

	respondJSONOK(w, roles)
}

// GetWorkspaceRoleAssignments returns all users with their role assignments for a workspace
func (h *WorkspaceRoleHandler) GetWorkspaceRoleAssignments(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	// Get all role assignments for this workspace with user details
	rows, err := db.Query(`
		SELECT
			u.id, u.username, u.email, u.first_name, u.last_name, u.avatar_url,
			wr.id, wr.name, wr.description,
			uwr.id, uwr.granted_at
		FROM user_workspace_roles uwr
		JOIN users u ON uwr.user_id = u.id
		JOIN workspace_roles wr ON uwr.role_id = wr.id
		WHERE uwr.workspace_id = ?
		ORDER BY u.username, wr.display_order
	`, workspaceID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type Role struct {
		RoleID          int    `json:"role_id"`
		RoleName        string `json:"role_name"`
		RoleDescription string `json:"role_description"`
		AssignmentID    int    `json:"assignment_id"`
	}

	type UserWithRoles struct {
		UserID    int     `json:"user_id"`
		Username  string  `json:"username"`
		Email     string  `json:"email"`
		FirstName *string `json:"first_name"`
		LastName  *string `json:"last_name"`
		AvatarURL *string `json:"avatar_url"`
		Roles     []Role  `json:"roles"`
	}

	// Map to group roles by user
	userMap := make(map[int]*UserWithRoles)

	for rows.Next() {
		var userID, roleID, assignmentID int
		var username, email, roleName, roleDescription string
		var firstName, lastName, avatarURL *string
		var grantedAt time.Time

		err := rows.Scan(
			&userID, &username, &email, &firstName, &lastName, &avatarURL,
			&roleID, &roleName, &roleDescription,
			&assignmentID, &grantedAt,
		)
		if err != nil {
			continue
		}

		// Get or create user entry
		user, exists := userMap[userID]
		if !exists {
			user = &UserWithRoles{
				UserID:    userID,
				Username:  username,
				Email:     email,
				FirstName: firstName,
				LastName:  lastName,
				AvatarURL: avatarURL,
				Roles:     []Role{},
			}
			userMap[userID] = user
		}

		// Add role to user
		user.Roles = append(user.Roles, Role{
			RoleID:          roleID,
			RoleName:        roleName,
			RoleDescription: roleDescription,
			AssignmentID:    assignmentID,
		})
	}

	// Convert map to slice
	users := make([]UserWithRoles, 0, len(userMap))
	for _, user := range userMap {
		users = append(users, *user)
	}

	// Sort by username for consistent ordering
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})

	respondJSONOK(w, users)
}

// getSessionUserID extracts user ID from session context
func (h *WorkspaceRoleHandler) getSessionUserID(r *http.Request) int {
	if user := utils.GetCurrentUser(r); user != nil {
		return user.ID
	}
	return 0
}

// createCustomRoleRequest is the JSON payload for POST /api/workspace-roles.
// Only name + description are accepted; the handler forces is_system=false and
// permissions_enabled=false. Toggling permissions on custom roles is a future
// feature (see /Users/stefanernst/.claude/plans/lets-plan-an-approval-kind-lantern.md).
type createCustomRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Create adds a new custom (label-only) workspace role. Custom roles can be
// used for approval routing but never grant permissions, regardless of any
// permission rows attached to them — the permission cache filters on
// workspace_roles.permissions_enabled.
//
// POST /api/workspace-roles
func (h *WorkspaceRoleHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	body, ok := decodeJSON[createCustomRoleRequest](w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		respondValidationError(w, r, "name is required")
		return
	}

	// Workspace_roles.name is UNIQUE — short-circuit with a friendly conflict
	// before letting the DB raise a generic constraint error.
	var nameTaken bool
	if err := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM workspace_roles WHERE name = ?)`, name).Scan(&nameTaken); err == nil && nameTaken {
		respondConflict(w, r, fmt.Sprintf("A role named %q already exists", name))
		return
	}

	now := time.Now()
	var id64 int64
	if err := h.db.QueryRow(`
		INSERT INTO workspace_roles (name, description, is_system, permissions_enabled, display_order, created_at, updated_at)
		VALUES (?, ?, false, false, COALESCE((SELECT MAX(display_order) + 1 FROM workspace_roles), 1), ?, ?)
		RETURNING id
	`, name, body.Description, now, now).Scan(&id64); err != nil {
		respondInternalError(w, r, err)
		return
	}
	id := int(id64)

	logAudit(h.db, r, user, logger.ActionWorkspaceRoleCreate, logger.ResourceRole, &id, name)

	out := models.WorkspaceRole{
		ID:                 id,
		Name:               name,
		Description:        body.Description,
		IsSystem:           false,
		PermissionsEnabled: false,
		CreatedAt:          now,
		UpdatedAt:          now,
		Permissions:        []models.Permission{},
	}
	respondJSONCreated(w, out)
}

// Delete removes a custom workspace role. System roles (is_system=true) cannot
// be deleted. The DELETE cascades to user_workspace_roles + group_workspace_roles
// + role_permissions via existing FKs; we still flush the permission cache for
// affected users so any cached label-only role assignment goes away.
//
// DELETE /api/workspace-roles/{id}
func (h *WorkspaceRoleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var name string
	var isSystem bool
	err := h.db.QueryRow(`SELECT name, is_system FROM workspace_roles WHERE id = ?`, id).Scan(&name, &isSystem)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "workspace_role")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if isSystem {
		respondValidationError(w, r, "System roles cannot be deleted")
		return
	}

	// Refuse delete if the role is referenced by any pending approval — the
	// snapshot's source_role_id stays intact for audit, but we don't want to
	// orphan an in-flight pool. Cancel the approval first, then delete.
	if h.approvalService != nil {
		if pendingCount, err := h.approvalService.CountPendingApproversForRole(r.Context(), id); err == nil && pendingCount > 0 {
			respondConflict(w, r, fmt.Sprintf("Cannot delete: %d pending approval(s) still reference this role", pendingCount))
			return
		}
	}

	// Snapshot affected users for cache invalidation before the DELETE cascades.
	affected := make(map[int]bool)
	if rows, err := h.db.Query(`SELECT user_id FROM user_workspace_roles WHERE role_id = ?`, id); err == nil {
		for rows.Next() {
			var uid int
			if scanErr := rows.Scan(&uid); scanErr == nil {
				affected[uid] = true
			}
		}
		_ = rows.Close()
	}
	if rows, err := h.db.Query(`
		SELECT DISTINCT gm.user_id
		FROM group_workspace_roles gwr
		JOIN group_members gm ON gm.group_id = gwr.group_id
		WHERE gwr.role_id = ?
	`, id); err == nil {
		for rows.Next() {
			var uid int
			if scanErr := rows.Scan(&uid); scanErr == nil {
				affected[uid] = true
			}
		}
		_ = rows.Close()
	}

	if _, err := h.db.Exec(`DELETE FROM workspace_roles WHERE id = ?`, id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, user, logger.ActionWorkspaceRoleDelete, logger.ResourceRole, &id, name)

	if h.permissionService != nil && len(affected) > 0 {
		ids := make([]int, 0, len(affected))
		for uid := range affected {
			ids = append(ids, uid)
		}
		_ = h.permissionService.InvalidateMultipleUserCaches(ids)
	}

	w.WriteHeader(http.StatusNoContent)
}
