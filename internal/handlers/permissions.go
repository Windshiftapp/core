package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// PermissionHandler handles permission-related HTTP requests
type PermissionHandler struct {
	repo              *repository.PermissionRepository
	permissionService *services.PermissionService
	auditor           *logger.Auditor
}

// NewPermissionHandlerWithCache creates a new permission handler with cached permission service
func NewPermissionHandlerWithCache(repo *repository.PermissionRepository, permissionService *services.PermissionService, auditor *logger.Auditor) *PermissionHandler {
	return &PermissionHandler{
		repo:              repo,
		permissionService: permissionService,
		auditor:           auditor,
	}
}

// GetAllPermissions returns all available permissions
func (h *PermissionHandler) GetAllPermissions(w http.ResponseWriter, r *http.Request) {
	permissions, err := h.repo.ListAll()
	if err != nil {
		respondInternalError(w, r, err)
		return
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

	permissionScope, err := h.repo.GetScope(permissionID)
	if errors.Is(err, repository.ErrNotFound) {
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
	if err := h.repo.GrantGlobalToUser(req.UserID, req.PermissionID, granterID); err != nil {
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
		permissionName, err := h.repo.GetName(req.PermissionID)
		if err != nil {
			slog.Warn("failed to look up permission name", slog.Any("error", err))
		}
		targetUsername, err := h.repo.GetUsername(req.UserID)
		if err != nil {
			slog.Warn("failed to look up username", slog.Any("error", err))
		}

		h.auditor.LogEvent(logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionGrant,
			ResourceType: logger.ResourcePermission,
			ResourceID:   &req.PermissionID,
			ResourceName: permissionName,
			Details: map[string]any{
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

	// Don't allow revoking system admin from the last admin
	permissionKey, err := h.repo.GetKey(permissionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if permissionKey == models.PermissionSystemAdmin {
		adminCount, err := h.repo.CountSystemAdminGrants()
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
	rowsAffected, err := h.repo.RevokeGlobalFromUser(userID, permissionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

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
		permissionName, err := h.repo.GetName(permissionID)
		if err != nil {
			slog.Warn("failed to look up permission name", slog.Any("error", err))
		}
		targetUsername, err := h.repo.GetUsername(userID)
		if err != nil {
			slog.Warn("failed to look up username", slog.Any("error", err))
		}

		h.auditor.LogEvent(logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionRevoke,
			ResourceType: logger.ResourcePermission,
			ResourceID:   &permissionID,
			ResourceName: permissionName,
			Details: map[string]any{
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

// invalidateGroupMemberCaches invalidates the permission cache for every
// member of the given group. Incomplete enumeration falls back to a full
// reset so a committed revocation cannot retain an unenumerated snapshot.
func (h *PermissionHandler) invalidateGroupMemberCaches(groupID int) []models.APIWarning {
	var warnings []models.APIWarning
	if h.permissionService == nil {
		return warnings
	}

	userIDs, iterErr, queryErr := h.repo.GroupMemberUserIDs(groupID)
	plan := services.AuthorizationInvalidation{UserIDs: userIDs}
	if queryErr != nil || iterErr != nil {
		plan.ResetPermissions = true
	}
	if err := services.NewAuthorizationCacheInvalidator(h.permissionService, nil).Apply(plan); err != nil {
		warnings = append(warnings, createCacheWarning("permission", err, fmt.Sprintf("group_id:%d", groupID)))
	}
	return warnings
}

// GrantGlobalPermissionToGroup grants a global permission to a group
func (h *PermissionHandler) GrantGlobalPermissionToGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupID      int `json:"group_id"`
		PermissionID int `json:"permission_id"`
	}
	if err := newJSONDecoder(w, r).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	granterID, ok := h.requireGlobalPermissionScope(w, r, req.PermissionID)
	if !ok {
		return
	}

	// Verify the group exists
	groupExists, err := h.repo.GroupExists(req.GroupID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !groupExists {
		respondNotFound(w, r, "group")
		return
	}

	// Grant the permission (only if not already granted)
	if err := h.repo.GrantGlobalToGroup(req.GroupID, req.PermissionID, granterID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate permission cache for all users in the group
	warnings := h.invalidateGroupMemberCaches(req.GroupID)

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		permissionName, err := h.repo.GetName(req.PermissionID)
		if err != nil {
			slog.Warn("failed to look up permission name", slog.Any("error", err))
		}
		groupName, err := h.repo.GetGroupName(req.GroupID)
		if err != nil {
			slog.Warn("failed to look up group name", slog.Any("error", err))
		}

		h.auditor.LogEvent(logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionGrant,
			ResourceType: logger.ResourcePermission,
			ResourceID:   &req.PermissionID,
			ResourceName: permissionName,
			Details: map[string]any{
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
	rowsAffected, err := h.repo.RevokeGlobalFromGroup(groupID, permissionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "permission")
		return
	}

	// Invalidate permission cache for all users in the group
	warnings := h.invalidateGroupMemberCaches(groupID)

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		permissionName, err := h.repo.GetName(permissionID)
		if err != nil {
			slog.Warn("failed to look up permission name", slog.Any("error", err))
		}
		groupName, err := h.repo.GetGroupName(groupID)
		if err != nil {
			slog.Warn("failed to look up group name", slog.Any("error", err))
		}

		h.auditor.LogEvent(logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionRevoke,
			ResourceType: logger.ResourcePermission,
			ResourceID:   &permissionID,
			ResourceName: permissionName,
			Details: map[string]any{
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

// getUserPermissionSummary builds the compact permission profile: permission
// keys only, workspace permissions grouped by workspace ID. Explicit grants,
// group grants, and the "Everyone" fallback all collapse into the same maps,
// so the payload stays flat regardless of how many workspaces the user can
// reach.
func (h *PermissionHandler) getUserPermissionSummary(userID int) (*models.UserPermissionSummary, error) {
	summary := &models.UserPermissionSummary{
		UserID:               userID,
		GlobalPermissions:    []string{},
		WorkspacePermissions: map[int][]string{},
	}

	// Global permissions: direct grants plus active-group grants.
	globalKeys := make(map[string]bool)
	globalSources := [][]models.UserGlobalPermission{}
	directGlobal, err := h.repo.ListUserGlobalGrants(userID)
	if err != nil {
		return nil, err
	}
	globalSources = append(globalSources, directGlobal)
	groupGlobal, err := h.repo.ListUserGroupGlobalGrants(userID)
	if err != nil {
		return nil, err
	}
	globalSources = append(globalSources, groupGlobal)

	for _, source := range globalSources {
		for _, grant := range source {
			if grant.Permission == nil {
				continue
			}
			globalKeys[grant.Permission.PermissionKey] = true
			if grant.Permission.PermissionKey == models.PermissionSystemAdmin {
				summary.HasSystemAdmin = true
			}
		}
	}
	for key := range globalKeys {
		summary.GlobalPermissions = append(summary.GlobalPermissions, key)
	}
	sort.Strings(summary.GlobalPermissions)

	// Workspace permissions: explicit role assignments first, then the
	// effective cache supplies group-based and "Everyone" keys. The cache
	// already resolves all three sources, so it only fills gaps.
	wsKeys := make(map[int]map[string]bool)

	explicit, err := h.repo.ListUserWorkspacePermissionKeys(userID)
	if err != nil {
		return nil, err
	}
	for workspaceID, keys := range explicit {
		wsKeys[workspaceID] = keys
	}

	if h.permissionService != nil {
		effectiveCache, cacheErr := h.permissionService.GetUserEffectivePermissions(userID)
		// A failed cache build must not widen access: skip the supplement
		// (fail closed) rather than erroring the whole profile.
		if cacheErr == nil && !effectiveCache.IsSystemAdmin {
			mergeKeys := func(perms map[int]map[string]bool) {
				for workspaceID, keys := range perms {
					if wsKeys[workspaceID] == nil {
						wsKeys[workspaceID] = make(map[string]bool)
					}
					for key := range keys {
						wsKeys[workspaceID][key] = true
					}
				}
			}
			mergeKeys(effectiveCache.WorkspacePermissions)
			mergeKeys(effectiveCache.WorkspaceEveryone)
		}
	}

	for workspaceID, keys := range wsKeys {
		keyList := make([]string, 0, len(keys))
		for key := range keys {
			keyList = append(keyList, key)
		}
		sort.Strings(keyList)
		summary.WorkspacePermissions[workspaceID] = keyList
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
	groupPermissions, err := h.repo.ListGroupGlobalGrants()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, groupPermissions)
}

// GetAllUserGlobalPermissions returns compact effective global permission
// assignments for every user. Route middleware restricts this bulk view to
// system administrators.
func (h *PermissionHandler) GetAllUserGlobalPermissions(w http.ResponseWriter, r *http.Request) {
	userPermissions, err := h.repo.ListEffectiveUserGlobalGrants()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, userPermissions)
}
