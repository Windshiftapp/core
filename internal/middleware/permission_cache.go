package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/services"
)

// CachedPermissionMiddleware handles permission checking using the cached permission service
type CachedPermissionMiddleware struct {
	permissionService *services.PermissionService
}

// NewCachedPermissionMiddleware creates a new cached permission middleware
func NewCachedPermissionMiddleware(permissionService *services.PermissionService) *CachedPermissionMiddleware {
	return &CachedPermissionMiddleware{
		permissionService: permissionService,
	}
}

// RequireGlobalPermission creates middleware that requires a specific global permission
func (pm *CachedPermissionMiddleware) RequireGlobalPermission(permissionKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := pm.getUserFromContext(r)
			if user == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Check permission using cached service
			hasPermission, err := pm.permissionService.HasGlobalPermission(user.ID, permissionKey)
			if err != nil {
				slog.Error("failed to check global permission", slog.Any("error", err), slog.Int("user_id", user.ID), slog.String("permission", permissionKey)) //nolint:gosec // logging internal values for debugging
				http.Error(w, "Permission check failed", http.StatusInternalServerError)
				return
			}

			if !hasPermission {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireWorkspacePermission creates middleware that requires a specific workspace permission
func (pm *CachedPermissionMiddleware) RequireWorkspacePermission(permissionKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := pm.getUserFromContext(r)
			if user == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Extract workspace ID from URL
			workspaceID, err := pm.getWorkspaceIDFromPath(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Check permission using cached service
			hasPermission, err := pm.permissionService.HasWorkspacePermission(user.ID, workspaceID, permissionKey)
			if err != nil {
				slog.Error("failed to check workspace permission", slog.Any("error", err), slog.Int("user_id", user.ID), slog.Int("workspace_id", workspaceID)) //nolint:gosec // logging internal values for debugging
				http.Error(w, "Permission check failed", http.StatusInternalServerError)
				return
			}

			if !hasPermission {
				http.Error(w, "Insufficient permissions for this workspace", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireWorkspacePermissions creates middleware that requires ANY of the specified permissions
func (pm *CachedPermissionMiddleware) RequireWorkspacePermissions(permissions []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := pm.getUserFromContext(r)
			if user == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Extract workspace ID from URL
			workspaceID, err := pm.getWorkspaceIDFromPath(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Check multiple permissions in single cache lookup
			hasPerms, err := pm.permissionService.HasWorkspacePermissions(user.ID, workspaceID, permissions)
			if err != nil {
				slog.Error("failed to check workspace permissions", slog.Any("error", err), slog.Int("user_id", user.ID), slog.Int("workspace_id", workspaceID)) //nolint:gosec // logging internal values for debugging
				http.Error(w, "Permission check failed", http.StatusInternalServerError)
				return
			}

			// Check if user has ANY of the required permissions
			for _, hasPermission := range hasPerms {
				if hasPermission {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, "Insufficient permissions for this workspace", http.StatusForbidden)
		})
	}
}

// RequireAllWorkspacePermissions creates middleware that requires ALL specified permissions
func (pm *CachedPermissionMiddleware) RequireAllWorkspacePermissions(permissions []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := pm.getUserFromContext(r)
			if user == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Extract workspace ID from URL
			workspaceID, err := pm.getWorkspaceIDFromPath(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Check multiple permissions in single cache lookup
			hasPerms, err := pm.permissionService.HasWorkspacePermissions(user.ID, workspaceID, permissions)
			if err != nil {
				slog.Error("failed to check workspace permissions", slog.Any("error", err), slog.Int("user_id", user.ID), slog.Int("workspace_id", workspaceID)) //nolint:gosec // logging internal values for debugging
				http.Error(w, "Permission check failed", http.StatusInternalServerError)
				return
			}

			// Check if user has ALL required permissions
			for _, permission := range permissions {
				if !hasPerms[permission] {
					http.Error(w, fmt.Sprintf("Missing permission: %s", permission), http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireSystemAdmin creates middleware that requires system admin privileges
func (pm *CachedPermissionMiddleware) RequireSystemAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := pm.getUserFromContext(r)
			if user == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Check if user is system admin using cached service
			isAdmin, err := pm.permissionService.IsSystemAdmin(user.ID)
			if err != nil {
				slog.Error("failed to check system admin status", slog.Any("error", err), slog.Int("user_id", user.ID)) //nolint:gosec // logging internal values for debugging
				http.Error(w, "Permission check failed", http.StatusInternalServerError)
				return
			}

			if !isAdmin {
				http.Error(w, "System administrator privileges required", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyWorkspaceAccess allows access if user has ANY permission in the workspace
func (pm *CachedPermissionMiddleware) RequireAnyWorkspaceAccess() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := pm.getUserFromContext(r)
			if user == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Extract workspace ID from URL
			workspaceID, err := pm.getWorkspaceIDFromPath(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Check if user has any workspace access (admin override handled in service)
			hasAccess, err := pm.hasAnyWorkspaceAccess(user.ID, workspaceID)
			if err != nil {
				slog.Error("failed to check workspace access", slog.Any("error", err), slog.Int("user_id", user.ID), slog.Int("workspace_id", workspaceID)) //nolint:gosec // logging internal values for debugging
				http.Error(w, "Permission check failed", http.StatusInternalServerError)
				return
			}

			if !hasAccess {
				http.Error(w, "No access to this workspace", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Helper functions

// getUserFromContext extracts user from request context
func (pm *CachedPermissionMiddleware) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value(ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}

// getWorkspaceIDFromPath extracts workspace ID from URL path
func (pm *CachedPermissionMiddleware) getWorkspaceIDFromPath(r *http.Request) (int, error) {
	// Try common workspace ID parameter names (PathValue returns "" if not found)
	workspaceIDStr := r.PathValue("workspaceId")
	if workspaceIDStr == "" {
		workspaceIDStr = r.PathValue("id")
	}
	if workspaceIDStr == "" {
		workspaceIDStr = r.PathValue("workspace_id")
	}
	if workspaceIDStr == "" {
		return 0, fmt.Errorf("workspace ID not found in URL path")
	}

	workspaceID, err := strconv.Atoi(workspaceIDStr)
	if err != nil {
		return 0, fmt.Errorf("invalid workspace ID: %w", err)
	}

	return workspaceID, nil
}

// hasAnyWorkspaceAccess checks if user has any access to workspace
func (pm *CachedPermissionMiddleware) hasAnyWorkspaceAccess(userID, workspaceID int) (bool, error) {
	// Check if user is system admin first (fastest check)
	isAdmin, err := pm.permissionService.IsSystemAdmin(userID)
	if err != nil {
		return false, err
	}
	if isAdmin {
		return true, nil
	}

	// Check common workspace permissions
	commonPermissions := []string{
		models.PermissionWorkspaceAdmin,
		models.PermissionItemView,
		models.PermissionItemEdit,
		models.PermissionItemDelete,
	}

	hasPerms, err := pm.permissionService.HasWorkspacePermissions(userID, workspaceID, commonPermissions)
	if err != nil {
		return false, err
	}

	// Return true if user has any permission
	for _, hasPerm := range hasPerms {
		if hasPerm {
			return true, nil
		}
	}

	return false, nil
}

// Utility functions for handlers

// CheckGlobalPermission utility function for handlers
func (pm *CachedPermissionMiddleware) CheckGlobalPermission(userID int, permissionKey string) (bool, error) {
	return pm.permissionService.HasGlobalPermission(userID, permissionKey)
}

// CheckWorkspacePermission utility function for handlers
func (pm *CachedPermissionMiddleware) CheckWorkspacePermission(userID, workspaceID int, permissionKey string) (bool, error) {
	return pm.permissionService.HasWorkspacePermission(userID, workspaceID, permissionKey)
}

// CheckMultipleWorkspacePermissions utility function for handlers
func (pm *CachedPermissionMiddleware) CheckMultipleWorkspacePermissions(userID, workspaceID int, permissions []string) (map[string]bool, error) {
	return pm.permissionService.HasWorkspacePermissions(userID, workspaceID, permissions)
}

// GetUserPermissionLevel returns a descriptive permission level for a user in a workspace
func (pm *CachedPermissionMiddleware) GetUserPermissionLevel(userID, workspaceID int) (string, error) {
	// Check if user is system admin
	isAdmin, err := pm.permissionService.IsSystemAdmin(userID)
	if err != nil {
		return "Error", fmt.Errorf("error checking system admin status: %w", err)
	}
	if isAdmin {
		return "System Admin", nil
	}

	// Check workspace permissions
	permissions := []string{
		models.PermissionWorkspaceAdmin,
	}

	hasPerms, err := pm.permissionService.HasWorkspacePermissions(userID, workspaceID, permissions)
	if err != nil {
		return "Error", fmt.Errorf("error checking workspace permissions: %w", err)
	}

	if hasPerms[models.PermissionWorkspaceAdmin] {
		return "Workspace Administrator", nil
	}

	// Check if user has any access
	hasAccess, err := pm.hasAnyWorkspaceAccess(userID, workspaceID)
	if err != nil {
		return "Error", fmt.Errorf("error checking workspace access: %w", err)
	}

	if hasAccess {
		return "Member", nil
	}

	return "No Access", nil
}

// GetCacheStats returns cache performance statistics
func (pm *CachedPermissionMiddleware) GetCacheStats() models.CacheStats {
	return pm.permissionService.GetCacheStats()
}
