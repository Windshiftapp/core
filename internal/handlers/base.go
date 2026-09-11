// Package handlers provides HTTP request handlers for the Windshift API.
package handlers

import (
	"context"
	"fmt"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// AvailableField is the public shape for GetAvailableFields responses across
// request-type and asset-report handlers. Identifier is the field key, Name
// is the display label, Type is "default" or "custom", and FieldType (when
// set) is the underlying custom-field data type.
type AvailableField struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	FieldType  string `json:"field_type,omitempty"`
}

// RequireAuth checks if a user is authenticated and returns the user.
// If not authenticated, it writes a 401 Unauthorized response.
// Returns the user and true if authenticated, nil and false otherwise.
// Usage:
//
//	user, ok := RequireAuth(w, r)
//	if !ok {
//	    return
//	}
func RequireAuth(w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return nil, false
	}
	return user, true
}

// RequireWorkspacePermission checks if the user has a specific workspace permission.
// If the user doesn't have permission, it writes a 404 response so private
// workspace existence is not disclosed.
// Returns true if permitted, false otherwise (error already written to response).
// Usage:
//
//	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
//	    return
//	}
func RequireWorkspacePermission(w http.ResponseWriter, r *http.Request, userID, workspaceID int, permission string, permService *services.PermissionService) bool {
	hasPermission, err := permService.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil || !hasPermission {
		respondNotFound(w, r, "workspace")
		return false
	}
	return true
}

// RequireSystemAdmin checks if the user is a system administrator.
// If the user isn't a system admin, it writes a 403 Forbidden response.
// Returns true if admin, false otherwise (error already written to response).
// Usage:
//
//	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
//	    return
//	}
func RequireSystemAdmin(w http.ResponseWriter, r *http.Request, userID int, permService *services.PermissionService) bool {
	isAdmin, err := permService.IsSystemAdmin(userID)
	if err != nil || !isAdmin {
		respondAdminRequired(w, r)
		return false
	}
	return true
}

// AuthorizeUserRequest checks if the current user is authorized to access resources for the target user.
// It returns the current user if authorized, nil otherwise (with appropriate HTTP error written to response).
// Access is granted if:
// - The current user is accessing their own resources (currentUser.ID == targetUserID), OR
// - The current user has system.admin permission
func AuthorizeUserRequest(w http.ResponseWriter, r *http.Request, targetUserID int, permissionService *services.PermissionService) *models.User {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return nil
	}

	// Check if user is system admin or accessing their own resources
	if currentUser.ID != targetUserID {
		// Check for system.admin permission
		if !RequireSystemAdmin(w, r, currentUser.ID, permissionService) {
			return nil
		}
	}

	return currentUser
}

// CheckItemPermission verifies the user has the given permission on the item's workspace.
// Returns 404 on both not-found and no-permission to prevent item existence leakage.
func CheckItemPermission(w http.ResponseWriter, r *http.Request, itemRepo *repository.ItemRepository,
	permService *services.PermissionService, itemID int, permission string) bool {
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return false
	}
	workspaceID, err := itemRepo.GetWorkspaceID(itemID)
	if err != nil {
		respondNotFound(w, r, "Item")
		return false
	}
	hasPermission, err := permService.HasWorkspacePermission(user.ID, workspaceID, permission)
	if err != nil || !hasPermission {
		respondNotFound(w, r, "Item") // 404, not 403 — prevents existence leakage
		return false
	}
	return true
}

// CheckItemPermissionAsActor is CheckItemPermission with one exception: when
// permission == item.view, it falls back to active-approval-pool membership
// before returning 404. This is the documented exception in approvals.go's
// Decide handler — an approver explicitly added to a pending step must be able
// to read the item context (title, description, attachments, comments,
// timeline) to make an informed decision, even without workspace item.view.
//
// For any permission other than item.view, behavior is identical to
// CheckItemPermission — approver-derived access is read-only and never extends
// to edit/delete.
//
// Active-pool membership is a snapshot: once the step closes (is_active=FALSE) or
// the request is no longer pending, the fallback fails and access disappears.
//
// approvalService may be nil (e.g. in tests); behavior degrades to
// CheckItemPermission.
func CheckItemPermissionAsActor(w http.ResponseWriter, r *http.Request, itemRepo *repository.ItemRepository,
	permService *services.PermissionService, approvalService *services.ApprovalService,
	itemID int, permission string) bool {
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return false
	}
	workspaceID, err := itemRepo.GetWorkspaceID(itemID)
	if err != nil {
		respondNotFound(w, r, "Item")
		return false
	}
	hasPermission, err := permService.HasWorkspacePermission(user.ID, workspaceID, permission)
	if err == nil && hasPermission {
		return true
	}
	if permission == models.PermissionItemView && approvalService != nil {
		inPool, perr := approvalService.UserHasActivePoolMembershipOnItem(r.Context(), user.ID, itemID, nil)
		if perr == nil && inPool {
			return true
		}
	}
	respondNotFound(w, r, "Item") // 404, not 403 — prevents existence leakage
	return false
}

// userCanViewItemAsActor is the boolean-returning sibling of
// CheckItemPermissionAsActor for callers that need to make their own response
// decision. Returns true if the user has workspace item.view OR is an active
// approver on the item. See CheckItemPermissionAsActor for the security model.
//
// approvalService may be nil; in that case only the workspace-permission
// branch is consulted.
func userCanViewItemAsActor(ctx context.Context, userID, itemID, workspaceID int,
	permService *services.PermissionService, approvalService *services.ApprovalService) (bool, error) {
	if permService == nil {
		return false, nil
	}
	hasView, err := permService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	if err != nil {
		return false, err
	}
	if hasView {
		return true, nil
	}
	if approvalService == nil {
		return false, nil
	}
	return approvalService.UserHasActivePoolMembershipOnItem(ctx, userID, itemID, nil)
}

// GetAccessibleWorkspaceIDs returns IDs of active workspaces the user can view.
func GetAccessibleWorkspaceIDs(user *models.User, db database.Database,
	permService *services.PermissionService) ([]int, error) {
	if user == nil || permService == nil {
		return []int{}, nil
	}
	return permService.AccessibleWorkspaceIDs(user.ID)
}

// GetAccessibleWorkspaceKeys returns a set of workspace keys the user can view.
func GetAccessibleWorkspaceKeys(user *models.User, db database.Database,
	permService *services.PermissionService) (map[string]bool, error) {
	if user == nil || permService == nil {
		return map[string]bool{}, nil
	}
	pairs, err := permService.AccessibleWorkspaceIDKeys(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	keys := make(map[string]bool)
	for _, pair := range pairs {
		keys[pair.Key] = true
	}
	return keys, nil
}
