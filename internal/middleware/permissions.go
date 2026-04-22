package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// PermissionMiddleware handles permission checking for protected routes
type PermissionMiddleware struct {
	db                database.Database
	permissionService *services.PermissionService
}

// NewPermissionMiddleware creates a new permission middleware
func NewPermissionMiddleware(db database.Database, permissionService *services.PermissionService) *PermissionMiddleware {
	return &PermissionMiddleware{db: db, permissionService: permissionService}
}

// permissionCheck is the per-request decision produced by a Require* middleware.
// Exactly one of APIErr (a specific non-500 response), Err (logged as 500), or
// a plain allowed flag should drive the outcome.
type permissionCheck struct {
	Allowed bool
	Err     error
	APIErr  *restapi.APIError
}

// requireWithCheck builds a middleware that applies the standard auth + system-admin
// short-circuit scaffold, then delegates the real permission decision to check.
func (pm *PermissionMiddleware) requireWithCheck(
	errLabel string,
	check func(user *models.User, r *http.Request) permissionCheck,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := pm.getUserFromContext(r)
			if user == nil {
				restapi.RespondError(w, r, restapi.ErrUnauthorized)
				return
			}

			// System admins have all permissions.
			if pm.isSystemAdmin(user.ID) {
				next.ServeHTTP(w, r)
				return
			}

			result := check(user, r)
			if result.APIErr != nil {
				restapi.RespondError(w, r, result.APIErr)
				return
			}
			if result.Err != nil {
				slog.Error(errLabel, slog.Any("error", result.Err))
				restapi.RespondError(w, r, restapi.ErrInternalError)
				return
			}
			if !result.Allowed {
				restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireGlobalPermission creates middleware that requires a specific global permission
func (pm *PermissionMiddleware) RequireGlobalPermission(permissionKey string) func(http.Handler) http.Handler {
	return pm.requireWithCheck("error checking global permission",
		func(user *models.User, _ *http.Request) permissionCheck {
			ok, err := pm.hasGlobalPermission(user.ID, permissionKey)
			return permissionCheck{Allowed: ok, Err: err}
		})
}

// RequireWorkspacePermission creates middleware that requires a specific workspace permission
// The workspace ID should be in the URL path as {workspaceId}
func (pm *PermissionMiddleware) RequireWorkspacePermission(permissionKey string) func(http.Handler) http.Handler {
	return pm.requireWithCheck("error checking workspace permission",
		func(user *models.User, r *http.Request) permissionCheck {
			workspaceID, err := extractWorkspaceID(r)
			if err != nil {
				return permissionCheck{APIErr: restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, err.Error())}
			}
			ok, checkErr := pm.permissionService.HasWorkspacePermission(user.ID, workspaceID, permissionKey)
			return permissionCheck{Allowed: ok, Err: checkErr}
		})
}

// RequireSystemAdmin creates middleware that requires system admin privileges
func (pm *PermissionMiddleware) RequireSystemAdmin() func(http.Handler) http.Handler {
	return pm.RequireGlobalPermission(models.PermissionSystemAdmin)
}

// RequireAnyWorkspacePermission allows access if user has ANY workspace permission for the workspace
// Useful for general workspace access
func (pm *PermissionMiddleware) RequireAnyWorkspacePermission() func(http.Handler) http.Handler {
	return pm.requireWithCheck("error checking workspace permissions",
		func(user *models.User, r *http.Request) permissionCheck {
			workspaceID, err := extractWorkspaceID(r)
			if err != nil {
				return permissionCheck{APIErr: restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, err.Error())}
			}
			ok, checkErr := pm.hasAnyWorkspacePermission(user.ID, workspaceID)
			return permissionCheck{Allowed: ok, Err: checkErr}
		})
}

// RequireChannelManagement creates middleware that requires channel management permission
// The channel ID should be in the URL path as {id}
func (pm *PermissionMiddleware) RequireChannelManagement() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := pm.getUserFromContext(r)
			if user == nil {
				restapi.RespondError(w, r, restapi.ErrUnauthorized)
				return
			}

			// System admins can manage all channels
			if pm.isSystemAdmin(user.ID) {
				next.ServeHTTP(w, r)
				return
			}

			// Extract channel ID from URL (PathValue returns "" if not found)
			channelIDStr := r.PathValue("id")
			if channelIDStr == "" {
				restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Channel ID not found in URL"))
				return
			}

			channelID, err := strconv.Atoi(channelIDStr)
			if err != nil {
				restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid channel ID"))
				return
			}

			// Check if channel is a default channel
			var isDefault bool
			err = pm.db.QueryRow(`SELECT is_default FROM channels WHERE id = ?`, channelID).Scan(&isDefault)
			if err != nil {
				restapi.RespondError(w, r, restapi.ErrChannelNotFound)
				return
			}

			// Default channels can only be managed by system admins
			if isDefault {
				restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, restapi.ErrCodeForbidden, "Default channels can only be managed by system administrators"))
				return
			}

			// For non-default channels, check if user is a channel manager
			hasPermission, err := pm.isChannelManager(user.ID, channelID)
			if err != nil {
				slog.Error("error checking channel management permission", slog.Any("error", err))
				restapi.RespondError(w, r, restapi.ErrInternalError)
				return
			}

			if !hasPermission {
				restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, restapi.ErrCodeInsufficientPermission, "You must be a channel manager or administrator to modify this channel"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Helper functions

// extractWorkspaceID extracts the workspace ID from the URL path parameters.
// It checks for "workspaceId" first, then falls back to "id".
func extractWorkspaceID(r *http.Request) (int, error) {
	workspaceIDStr := r.PathValue("workspaceId")
	if workspaceIDStr == "" {
		workspaceIDStr = r.PathValue("id")
	}
	if workspaceIDStr == "" {
		return 0, fmt.Errorf("workspace ID not found in URL")
	}

	workspaceID, err := strconv.Atoi(workspaceIDStr)
	if err != nil {
		return 0, fmt.Errorf("invalid workspaceId")
	}
	return workspaceID, nil
}

func (pm *PermissionMiddleware) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value(ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}

func (pm *PermissionMiddleware) isSystemAdmin(userID int) bool {
	// Check if user has system.admin permission
	var hasPermission bool
	err := pm.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM user_global_permissions ugp
			JOIN permissions p ON ugp.permission_id = p.id
			WHERE ugp.user_id = ? AND p.permission_key = 'system.admin'
		)
	`, userID).Scan(&hasPermission)
	if err != nil {
		slog.Error("error checking system admin permission", slog.Any("error", err))
		return false
	}
	return hasPermission
}

func (pm *PermissionMiddleware) hasGlobalPermission(userID int, permissionKey string) (bool, error) {
	var count int
	err := pm.db.QueryRow(`
		SELECT COUNT(*) FROM user_global_permissions ugp
		JOIN permissions p ON ugp.permission_id = p.id
		WHERE ugp.user_id = ? AND p.permission_key = ?
	`, userID, permissionKey).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (pm *PermissionMiddleware) hasAnyWorkspacePermission(userID, workspaceID int) (bool, error) {
	var count int
	// Check if user has any role in the workspace
	err := pm.db.QueryRow(`
		SELECT COUNT(*) FROM user_workspace_roles
		WHERE user_id = ? AND workspace_id = ?
	`, userID, workspaceID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (pm *PermissionMiddleware) isChannelManager(userID, channelID int) (bool, error) {
	var isManager bool
	err := pm.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM channel_managers
			WHERE channel_id = ?
			AND ((manager_type = 'user' AND manager_id = ?)
				 OR (manager_type = 'group' AND manager_id IN (
					 SELECT group_id FROM group_members WHERE user_id = ?
				 )))
		)
	`, channelID, userID, userID).Scan(&isManager)
	if err != nil {
		return false, err
	}
	return isManager, nil
}

// Permission helper functions that can be used by handlers

// CheckGlobalPermission is a utility function for handlers to check global permissions
func (pm *PermissionMiddleware) CheckGlobalPermission(userID int, permissionKey string) (bool, error) {
	// System admins have all permissions
	if pm.isSystemAdmin(userID) {
		return true, nil
	}

	return pm.hasGlobalPermission(userID, permissionKey)
}

// CheckWorkspacePermission is a utility function for handlers to check workspace permissions
func (pm *PermissionMiddleware) CheckWorkspacePermission(userID, workspaceID int, permissionKey string) (bool, error) {
	// System admins have all permissions
	if pm.isSystemAdmin(userID) {
		return true, nil
	}

	return pm.permissionService.HasWorkspacePermission(userID, workspaceID, permissionKey)
}

// GetUserPermissionLevel returns a descriptive permission level for a user in a workspace
func (pm *PermissionMiddleware) GetUserPermissionLevel(userID, workspaceID int) string {
	// Check if user is system admin
	if pm.isSystemAdmin(userID) {
		return "System Admin"
	}

	// Check for workspace administrator
	isWorkspaceAdmin, err := pm.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
	if err == nil && isWorkspaceAdmin {
		return "Workspace Administrator"
	}

	// Check if user has any permissions
	hasAny, err := pm.hasAnyWorkspacePermission(userID, workspaceID)
	if err == nil && hasAny {
		return "Member"
	}

	return "No Access"
}

// RequireSetupNotComplete blocks access if initial setup has already been completed
// This prevents unauthorized access to setup endpoints after the system is configured
func (pm *PermissionMiddleware) RequireSetupNotComplete() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if setup is already completed
			var setupCompleted string
			err := pm.db.QueryRow(`SELECT value FROM system_settings WHERE key = 'setup_completed'`).Scan(&setupCompleted)
			if err != nil {
				slog.Error("error checking setup status", slog.Any("error", err))
				restapi.RespondError(w, r, restapi.ErrInternalError)
				return
			}

			// Block access if setup is already completed
			if setupCompleted == "true" {
				restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, restapi.ErrCodeForbidden, "Setup has already been completed"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
