package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type WorkspaceRoleHandler struct {
	repo              *repository.WorkspaceRoleRepository
	permissionService *services.PermissionService
	approvalService   *services.ApprovalService
	auditor           *logger.Auditor
}

func NewWorkspaceRoleHandlerWithPool(repo *repository.WorkspaceRoleRepository, permissionService *services.PermissionService, auditor *logger.Auditor) *WorkspaceRoleHandler {
	return &WorkspaceRoleHandler{
		repo:              repo,
		permissionService: permissionService,
		auditor:           auditor,
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
	roles, err := h.repo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, roles)
}

// Get returns a single workspace role with its permissions
func (h *WorkspaceRoleHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	role, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "workspace_role")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load permissions for this role
	permissions, err := h.repo.GetPermissions(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	role.Permissions = permissions

	respondJSONOK(w, role)
}

// AssignRoleToUser assigns a role to a user in a workspace
func (h *WorkspaceRoleHandler) AssignRoleToUser(w http.ResponseWriter, r *http.Request) {
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
	roleExists, err := h.repo.Exists(req.RoleID)
	if err != nil || !roleExists {
		respondNotFound(w, r, "role")
		return
	}

	// Check that the target workspace exists and is active. Without this the
	// INSERT below would surface a FK violation as a generic 500; validate up
	// front so the caller gets a clean not-found / bad-request instead.
	workspaceActive, err := h.repo.WorkspaceActive(req.WorkspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "workspace")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !workspaceActive {
		respondBadRequest(w, r, "cannot assign roles in an inactive workspace")
		return
	}

	// Count existing assignments for this role+workspace before the operation
	countBefore, _ := h.repo.CountAssignmentsPreAssign(req.WorkspaceID, req.RoleID)

	// Insert or update role assignment
	if err := h.repo.AssignToUser(req.UserID, req.WorkspaceID, req.RoleID, granterID); err != nil {
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
		roleName := h.repo.RoleName(req.RoleID)
		targetUsername := h.repo.Username(req.UserID)
		workspaceName := h.repo.WorkspaceName(req.WorkspaceID)

		h.auditor.LogWithDetails(r, currentUser, logger.ActionRoleAssign, logger.ResourceRole, &req.RoleID, roleName,
			map[string]any{
				"target_user_id":  req.UserID,
				"target_username": targetUsername,
				"role_id":         req.RoleID,
				"role_name":       roleName,
				"workspace_id":    req.WorkspaceID,
				"workspace_name":  workspaceName,
			})
	}

	respondJSONCreatedWithWarnings(w, map[string]string{"message": "Role assigned successfully"}, warnings)
}

// RevokeRoleFromUser revokes a role from a user in a workspace
func (h *WorkspaceRoleHandler) RevokeRoleFromUser(w http.ResponseWriter, r *http.Request) {
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
	countBefore, _ := h.repo.CountAssignments(workspaceID, roleID)

	rowsAffected, err := h.repo.RevokeFromUser(userID, workspaceID, roleID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

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
		roleName := h.repo.RoleName(roleID)
		targetUsername := h.repo.Username(userID)
		workspaceName := h.repo.WorkspaceName(workspaceID)

		h.auditor.LogWithDetails(r, currentUser, logger.ActionRoleRevoke, logger.ResourceRole, &roleID, roleName,
			map[string]any{
				"target_user_id":  userID,
				"target_username": targetUsername,
				"role_id":         roleID,
				"role_name":       roleName,
				"workspace_id":    workspaceID,
				"workspace_name":  workspaceName,
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
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	roles, err := h.repo.ListUserRoles(userID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, roles)
}

// GetWorkspaceRoleAssignments returns all users with their role assignments for a workspace
func (h *WorkspaceRoleHandler) GetWorkspaceRoleAssignments(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	// Get all role assignments for this workspace with user details
	assignments, err := h.repo.ListUserAssignments(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	type Role struct {
		RoleID          int    `json:"role_id"`
		RoleBuiltinKey  string `json:"role_builtin_key,omitempty"`
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

	for _, a := range assignments {
		// Get or create user entry
		user, exists := userMap[a.UserID]
		if !exists {
			user = &UserWithRoles{
				UserID:    a.UserID,
				Username:  a.Username,
				Email:     a.Email,
				FirstName: a.FirstName,
				LastName:  a.LastName,
				AvatarURL: a.AvatarURL,
				Roles:     []Role{},
			}
			userMap[a.UserID] = user
		}

		// Add role to user
		user.Roles = append(user.Roles, Role{
			RoleID:          a.RoleID,
			RoleBuiltinKey:  a.RoleBuiltinKey,
			RoleName:        a.RoleName,
			RoleDescription: a.RoleDescription,
			AssignmentID:    a.AssignmentID,
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

// AssignRoleToGroup assigns a workspace role to a group. Mirrors AssignRoleToUser
// but writes group_workspace_roles; the cache builder already joins this table on
// every permission path, so the only missing piece was a write surface.
func (h *WorkspaceRoleHandler) AssignRoleToGroup(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.GroupRoleAssignmentRequest](w, r)
	if !ok {
		return
	}

	granterID := h.getSessionUserID(r)
	if granterID == 0 {
		respondUnauthorized(w, r)
		return
	}

	// Check that the role exists
	roleExists, err := h.repo.Exists(req.RoleID)
	if err != nil || !roleExists {
		respondNotFound(w, r, "role")
		return
	}

	// Check that the group exists
	groupExists, err := h.repo.GroupExists(req.GroupID)
	if err != nil || !groupExists {
		respondNotFound(w, r, "group")
		return
	}

	// Check that the target workspace exists and is active (see AssignRoleToUser).
	workspaceActive, err := h.repo.WorkspaceActive(req.WorkspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "workspace")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !workspaceActive {
		respondBadRequest(w, r, "cannot assign roles in an inactive workspace")
		return
	}

	// Count existing assignments for this role+workspace before the operation
	countBefore, _ := h.repo.CountAssignmentsPreAssign(req.WorkspaceID, req.RoleID)

	if err := h.repo.AssignToGroup(req.GroupID, req.WorkspaceID, req.RoleID, granterID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate cache: if this is the first assignment for this role+workspace,
	// everyone's implicit access changed → full cache reset. Otherwise only the
	// group's members are affected.
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if countBefore == 0 {
			h.permissionService.OnEveryoneAccessChanged()
		} else if err := h.permissionService.OnGroupPermissionChanged(req.GroupID); err != nil {
			warnings = append(warnings, createCacheWarning("permission", err, fmt.Sprintf("group_id:%d", req.GroupID)))
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		roleName := h.repo.RoleName(req.RoleID)
		groupName := h.repo.GroupName(req.GroupID)
		workspaceName := h.repo.WorkspaceName(req.WorkspaceID)

		h.auditor.LogWithDetails(r, currentUser, logger.ActionRoleAssign, logger.ResourceRole, &req.RoleID, roleName,
			map[string]any{
				"target_group_id":   req.GroupID,
				"target_group_name": groupName,
				"role_id":           req.RoleID,
				"role_name":         roleName,
				"workspace_id":      req.WorkspaceID,
				"workspace_name":    workspaceName,
			})
	}

	respondJSONCreatedWithWarnings(w, map[string]string{"message": "Role assigned to group successfully"}, warnings)
}

// RevokeRoleFromGroup revokes a workspace role from a group. Mirrors RevokeRoleFromUser.
func (h *WorkspaceRoleHandler) RevokeRoleFromGroup(w http.ResponseWriter, r *http.Request) {
	groupID, ok := requireIDParam(w, r, "groupId")
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
	countBefore, _ := h.repo.CountAssignments(workspaceID, roleID)

	rowsAffected, err := h.repo.RevokeFromGroup(groupID, workspaceID, roleID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "role_assignment")
		return
	}

	// Invalidate cache: if this was the last assignment for this role+workspace,
	// everyone's implicit access changed → full cache reset. Otherwise only the
	// group's members are affected.
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if countBefore == 1 {
			h.permissionService.OnEveryoneAccessChanged()
		} else if err := h.permissionService.OnGroupPermissionChanged(groupID); err != nil {
			warnings = append(warnings, createCacheWarning("permission", err, fmt.Sprintf("group_id:%d", groupID)))
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		roleName := h.repo.RoleName(roleID)
		groupName := h.repo.GroupName(groupID)
		workspaceName := h.repo.WorkspaceName(workspaceID)

		h.auditor.LogWithDetails(r, currentUser, logger.ActionRoleRevoke, logger.ResourceRole, &roleID, roleName,
			map[string]any{
				"target_group_id":   groupID,
				"target_group_name": groupName,
				"role_id":           roleID,
				"role_name":         roleName,
				"workspace_id":      workspaceID,
				"workspace_name":    workspaceName,
			})
	}

	if len(warnings) > 0 {
		respondJSONOKWithWarnings(w, map[string]string{"message": "Role revoked from group successfully"}, warnings)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetWorkspaceGroupRoleAssignments returns all groups with their role assignments
// for a workspace. Group analog of GetWorkspaceRoleAssignments.
func (h *WorkspaceRoleHandler) GetWorkspaceGroupRoleAssignments(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	assignments, err := h.repo.ListGroupAssignments(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	type Role struct {
		RoleID          int    `json:"role_id"`
		RoleBuiltinKey  string `json:"role_builtin_key,omitempty"`
		RoleName        string `json:"role_name"`
		RoleDescription string `json:"role_description"`
		AssignmentID    int    `json:"assignment_id"`
	}

	type GroupWithRoles struct {
		GroupID          int    `json:"group_id"`
		GroupName        string `json:"group_name"`
		GroupDescription string `json:"group_description"`
		Roles            []Role `json:"roles"`
	}

	groupMap := make(map[int]*GroupWithRoles)

	for _, a := range assignments {
		group, exists := groupMap[a.GroupID]
		if !exists {
			desc := ""
			if a.GroupDescription != nil {
				desc = *a.GroupDescription
			}
			group = &GroupWithRoles{
				GroupID:          a.GroupID,
				GroupName:        a.GroupName,
				GroupDescription: desc,
				Roles:            []Role{},
			}
			groupMap[a.GroupID] = group
		}

		group.Roles = append(group.Roles, Role{
			RoleID:          a.RoleID,
			RoleBuiltinKey:  a.RoleBuiltinKey,
			RoleName:        a.RoleName,
			RoleDescription: a.RoleDescription,
			AssignmentID:    a.AssignmentID,
		})
	}

	groups := make([]GroupWithRoles, 0, len(groupMap))
	for _, group := range groupMap {
		groups = append(groups, *group)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].GroupName < groups[j].GroupName
	})

	respondJSONOK(w, groups)
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
	// Role Name renders in every member list, role picker, and assignment
	// dialog — a short user-facing label. Description shows in the role
	// directory and is multi-line free-form text.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &body.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &body.Description, Policy: sanitize.RichText, Label: "Description"},
	)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		respondValidationError(w, r, "name is required")
		return
	}

	// Workspace_roles.name is UNIQUE — short-circuit with a friendly conflict
	// before letting the DB raise a generic constraint error.
	if nameTaken, err := h.repo.NameExists(name); err == nil && nameTaken {
		respondConflict(w, r, fmt.Sprintf("A role named %q already exists", name))
		return
	}

	now := time.Now()
	id, err := h.repo.CreateCustomRole(name, body.Description, now)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.auditor.Log(r, user, logger.ActionWorkspaceRoleCreate, logger.ResourceRole, &id, name)

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
	respondJSONCreated(w, struct {
		models.WorkspaceRole
		Warnings []string `json:"warnings,omitempty"`
	}{out, warnings})
}

// Delete removes a custom workspace role. System roles (is_system=true) cannot
// be deleted. The DELETE cascades to user_workspace_roles + group_workspace_roles
// + role_permissions via existing FKs; action allowlists deliberately block the
// delete because removing their last row would broaden manual-action access.
// We still flush the permission cache for affected users so any cached
// label-only role assignment goes away.
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

	role, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "workspace_role")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if role.IsSystem {
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
	if actionCount, err := h.repo.CountManualActionRestrictions(id); err != nil {
		respondInternalError(w, r, err)
		return
	} else if actionCount > 0 {
		respondConflict(w, r, fmt.Sprintf("Cannot delete: %d manual action(s) still restrict access to this role", actionCount))
		return
	}

	// Snapshot affected users for cache invalidation before the DELETE cascades.
	affected := h.repo.AffectedUserIDs(id)

	if err := h.repo.Delete(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.auditor.Log(r, user, logger.ActionWorkspaceRoleDelete, logger.ResourceRole, &id, role.Name)

	if h.permissionService != nil && len(affected) > 0 {
		ids := make([]int, 0, len(affected))
		for uid := range affected {
			ids = append(ids, uid)
		}
		_ = h.permissionService.InvalidateMultipleUserCaches(ids)
	}

	w.WriteHeader(http.StatusNoContent)
}
