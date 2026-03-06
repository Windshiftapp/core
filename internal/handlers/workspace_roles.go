package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type WorkspaceRoleHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
}

func NewWorkspaceRoleHandler(db database.Database) *WorkspaceRoleHandler {
	return &WorkspaceRoleHandler{BaseHandler: NewBaseHandler(db)}
}

func NewWorkspaceRoleHandlerWithPool(db database.Database, permissionService *services.PermissionService) *WorkspaceRoleHandler {
	return &WorkspaceRoleHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

// GetAll returns all workspace roles
func (h *WorkspaceRoleHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	query := `
		SELECT id, name, description, is_system, display_order, created_at, updated_at
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
			&role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		roles = append(roles, role)
	}

	if roles == nil {
		roles = []models.WorkspaceRole{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(roles)
}

// Get returns a single workspace role with its permissions
func (h *WorkspaceRoleHandler) Get(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var role models.WorkspaceRole
	err = db.QueryRow(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM workspace_roles
		WHERE id = ?
	`, id).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem,
		&role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)

	if err == sql.ErrNoRows {
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(role)
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

	var req models.UserRoleAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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

	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
		return
	}

	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	roleID, err := strconv.Atoi(r.PathValue("roleId"))
	if err != nil {
		respondInvalidID(w, r, "roleId")
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

	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
		return
	}

	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(roles)
}

// GetWorkspaceRoleAssignments returns all users with their role assignments for a workspace
func (h *WorkspaceRoleHandler) GetWorkspaceRoleAssignments(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(users)
}

func (h *WorkspaceRoleHandler) getWorkspaceRoleByID(roleID int) (*models.WorkspaceRole, error) {
	db, err := h.getReadDB()
	if err != nil {
		return nil, err
	}
	var role models.WorkspaceRole
	err = db.QueryRow(`
		SELECT id, name, description, is_system, display_order, created_at, updated_at
		FROM workspace_roles
		WHERE id = ?
	`, roleID).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.DisplayOrder, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// getSessionUserID extracts user ID from session context
func (h *WorkspaceRoleHandler) getSessionUserID(r *http.Request) int {
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u.ID
		}
	}
	return 0
}
