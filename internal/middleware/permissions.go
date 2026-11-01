package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// PermissionMiddleware handles permission checking for protected routes
type PermissionMiddleware struct {
	db                database.Database
	permissionService *services.PermissionService
	auditor           *logger.Auditor
}

// NewPermissionMiddleware creates a new permission middleware
func NewPermissionMiddleware(db database.Database, permissionService *services.PermissionService) *PermissionMiddleware {
	return &PermissionMiddleware{
		db:                db,
		permissionService: permissionService,
		auditor:           logger.NewAuditor(db),
	}
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
// requiredPermission is recorded on the audit row when a mutating request is denied.
func (pm *PermissionMiddleware) requireWithCheck(
	errLabel, requiredPermission string,
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
				if result.APIErr.StatusCode == http.StatusForbidden {
					auditDenied(pm.auditor, r, user, requiredPermission)
				}
				restapi.RespondError(w, r, result.APIErr)
				return
			}
			if result.Err != nil {
				slog.Error(errLabel, slog.Any("error", result.Err))
				restapi.RespondError(w, r, restapi.ErrInternalError)
				return
			}
			if !result.Allowed {
				auditDenied(pm.auditor, r, user, requiredPermission)
				restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// auditDenied records a route-level authorization denial for mutating requests.
// Reads are skipped so high-volume GET paths do not flood the audit log.
func auditDenied(auditor *logger.Auditor, r *http.Request, user *models.User, requiredPermission string) {
	if auditor == nil || !utils.IsMutationMethod(r.Method) {
		return
	}
	auditor.LogDenied(r, user, logger.ActionAccessDenied, logger.ResourceRoute, nil, r.URL.Path, requiredPermission, nil)
}

// RequireGlobalPermission creates middleware that requires a specific global permission
func (pm *PermissionMiddleware) RequireGlobalPermission(permissionKey string) func(http.Handler) http.Handler {
	return pm.requireWithCheck("error checking global permission", permissionKey,
		func(user *models.User, _ *http.Request) permissionCheck {
			ok, err := pm.hasGlobalPermission(user.ID, permissionKey)
			return permissionCheck{Allowed: ok, Err: err}
		})
}

// RequireWorkspacePermission creates middleware that requires a specific workspace permission
// The workspace ID should be in the URL path as {workspaceId}
func (pm *PermissionMiddleware) RequireWorkspacePermission(permissionKey string) func(http.Handler) http.Handler {
	return pm.requireWithCheck("error checking workspace permission", permissionKey,
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

			// Extract channel ID from URL. Prefer {channel_id} for routes where the
			// channel is the parent of another resource (e.g. /channels/{channel_id}/request-types/{id}),
			// fall back to {id} for routes where the channel itself is the resource (e.g. /channels/{id}).
			channelIDStr := r.PathValue("channel_id")
			if channelIDStr == "" {
				channelIDStr = r.PathValue("id")
			}
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
			err = pm.db.QueryRowContext(r.Context(), `SELECT COALESCE(is_default, false) FROM channels WHERE id = ?`, channelID).Scan(&isDefault)
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
			hasPermission, err := pm.isChannelManager(r.Context(), user.ID, channelID)
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

// isSystemAdmin / hasGlobalPermission delegate to PermissionService so the
// cookie-auth middleware shares the cached check used by every other surface
// (handlers, v1 API, internal/authz). The previous in-middleware SQL queries
// drifted from the service version (no cache, no consistent slog labels).
func (pm *PermissionMiddleware) isSystemAdmin(userID int) bool {
	isAdmin, err := pm.permissionService.IsSystemAdmin(userID)
	if err != nil {
		slog.Error("error checking system admin permission", slog.Any("error", err))
		return false
	}
	return isAdmin
}

func (pm *PermissionMiddleware) hasGlobalPermission(userID int, permissionKey string) (bool, error) {
	return pm.permissionService.HasGlobalPermission(userID, permissionKey)
}

func (pm *PermissionMiddleware) isChannelManager(ctx context.Context, userID, channelID int) (bool, error) {
	var isManager bool
	err := pm.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM channel_managers
			WHERE channel_id = ?
			AND ((manager_type = 'user' AND manager_id = ?)
				 OR (manager_type = 'group' AND manager_id IN (
					 SELECT gm.group_id
					 FROM group_members gm
					 JOIN groups g ON g.id = gm.group_id
					 WHERE gm.user_id = ? AND g.is_active = true
				 )))
		)
	`, channelID, userID, userID).Scan(&isManager)
	if err != nil {
		return false, err
	}
	return isManager, nil
}

// RequireSetupNotComplete blocks access if initial setup has already been completed
// This prevents unauthorized access to setup endpoints after the system is configured
func (pm *PermissionMiddleware) RequireSetupNotComplete() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if setup is already completed
			var setupCompleted string
			err := pm.db.QueryRowContext(r.Context(), `SELECT value FROM system_settings WHERE key = 'setup_completed'`).Scan(&setupCompleted)
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
