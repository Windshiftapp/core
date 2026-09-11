package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

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

// getUserPermissionSummary gets a complete permission summary for a user
func (h *PermissionHandler) getUserPermissionSummary(userID int) (*models.UserPermissionSummary, error) {
	summary := &models.UserPermissionSummary{
		UserID:               userID,
		GlobalPermissions:    []models.UserGlobalPermission{},    // Initialize as empty slice, not nil
		WorkspacePermissions: []models.UserWorkspacePermission{}, // Initialize as empty slice, not nil
	}

	// Get user info
	user, err := h.repo.GetUserBasic(userID)
	if err != nil {
		return nil, err
	}
	summary.User = user

	// Get global permissions
	globalGrants, err := h.repo.ListUserGlobalGrants(userID)
	if err != nil {
		return nil, err
	}
	for _, ugp := range globalGrants {
		summary.GlobalPermissions = append(summary.GlobalPermissions, ugp)
		if ugp.Permission != nil && ugp.Permission.PermissionKey == models.PermissionSystemAdmin {
			summary.HasSystemAdmin = true
		}
	}

	// Get permissions inherited from groups
	groupGrants, err := h.repo.ListUserGroupGlobalGrants(userID)
	if err != nil {
		return nil, err
	}
	for _, ugp := range groupGrants {
		summary.GlobalPermissions = append(summary.GlobalPermissions, ugp)
		if ugp.Permission != nil && ugp.Permission.PermissionKey == models.PermissionSystemAdmin {
			summary.HasSystemAdmin = true
		}
	}

	// Get workspace permissions from explicit role assignments
	// Track already-added workspace permissions to avoid duplicates
	addedPerms := make(map[int]map[string]bool) // workspace_id -> permission_key -> true

	workspaceGrants, err := h.repo.ListUserWorkspaceRoleGrants(userID)
	if err != nil {
		return nil, err
	}
	for _, uwp := range workspaceGrants {
		summary.WorkspacePermissions = append(summary.WorkspacePermissions, uwp)

		if addedPerms[uwp.WorkspaceID] == nil {
			addedPerms[uwp.WorkspaceID] = make(map[string]bool)
		}
		addedPerms[uwp.WorkspaceID][uwp.Permission.PermissionKey] = true
	}

	// Supplement with group-based and "Everyone" implicit permissions from the
	// permission cache.  The cache already resolves all three sources (explicit,
	// group, everyone) so we only need to add entries not already covered above.
	if h.permissionService != nil {
		effectiveCache, cacheErr := h.permissionService.GetUserEffectivePermissions(userID)
		if cacheErr == nil && !effectiveCache.IsSystemAdmin {
			// Build a permission-key → Permission lookup so we can populate the
			// Permission field on synthetic UserWorkspacePermission entries.
			permLookup, lookupErr := h.repo.PermissionsByKey()
			if lookupErr != nil {
				permLookup = make(map[string]*models.Permission)
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
				workspaces, wsErr := h.repo.ListWorkspacesBasic()
				if wsErr == nil {
					for _, w := range workspaces {
						if needWSIDs[w.ID] {
							cp := w
							wsLookup[w.ID] = &cp
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
