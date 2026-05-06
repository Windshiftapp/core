// Package handlers provides HTTP request handlers for the Windshift API.
package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// ErrDatabaseNil is returned when database connection is not initialized
var ErrDatabaseNil = errors.New("database connection is nil")

// BaseHandler provides common database access patterns for all handlers
type BaseHandler struct {
	db database.Database
}

// NewBaseHandler creates a new base handler with database connection
func NewBaseHandler(db database.Database) *BaseHandler {
	return &BaseHandler{db: db}
}

// getReadDB returns the database connection for read operations.
// Returns an error if the database connection is not initialized.
func (h *BaseHandler) getReadDB() (database.Database, error) {
	if h.db != nil {
		return h.db, nil
	}
	return nil, ErrDatabaseNil
}

// getWriteDB returns the database connection for write operations.
// Returns an error if the database connection is not initialized.
func (h *BaseHandler) getWriteDB() (database.Database, error) {
	if h.db != nil {
		return h.db, nil
	}
	return nil, ErrDatabaseNil
}

// requireReadDB returns the database connection and writes an HTTP error if unavailable.
// Returns nil and false if the database is unavailable (error already written to response).
// Returns db and true if the database is available.
func (h *BaseHandler) requireReadDB(w http.ResponseWriter, r *http.Request) (database.Database, bool) {
	db, err := h.getReadDB()
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return db, true
}

// requireWriteDB returns the database connection and writes an HTTP error if unavailable.
// Returns nil and false if the database is unavailable (error already written to response).
// Returns db and true if the database is available.
func (h *BaseHandler) requireWriteDB(w http.ResponseWriter, r *http.Request) (database.Database, bool) {
	db, err := h.getWriteDB()
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return db, true
}

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

// requireWorkspaceIDAndID parses {workspaceId} and {id} path params and pulls
// the current user. Used by workspace-scoped resource handlers that don't need
// a DB handle (services/repositories manage their own connections).
func requireWorkspaceIDAndID(w http.ResponseWriter, r *http.Request) (workspaceID, id int, user *models.User, ok bool) {
	workspaceID, ok = requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	id, ok = requireIDParam(w, r, "id")
	if !ok {
		return
	}
	user = utils.GetCurrentUser(r)
	return
}

// requireWorkspaceIDAndIDForWrite parses {workspaceId} and {id} path params,
// pulls the current user, and resolves the write DB. Used by any mutating
// handler that follows the standard "workspace-scoped resource with id" shape.
func (h *BaseHandler) requireWorkspaceIDAndIDForWrite(
	w http.ResponseWriter, r *http.Request,
) (workspaceID, id int, user *models.User, db database.Database, ok bool) {
	workspaceID, id, user, ok = requireWorkspaceIDAndID(w, r)
	if !ok {
		return
	}
	db, ok = h.requireWriteDB(w, r)
	return
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
// If the user doesn't have permission, it writes a 403 Forbidden response.
// Returns true if permitted, false otherwise (error already written to response).
// Usage:
//
//	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
//	    return
//	}
func RequireWorkspacePermission(w http.ResponseWriter, r *http.Request, userID, workspaceID int, permission string, permService *services.PermissionService) bool {
	hasPermission, err := permService.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
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
// Active-pool membership is a snapshot: once the step closes (is_active=0) or
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
		inPool, perr := approvalService.UserHasActivePoolMembershipOnItem(user.ID, itemID)
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
func userCanViewItemAsActor(userID, itemID, workspaceID int,
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
	return approvalService.UserHasActivePoolMembershipOnItem(userID, itemID)
}

// GetAccessibleWorkspaceIDs returns IDs of active workspaces the user can view.
func GetAccessibleWorkspaceIDs(user *models.User, db database.Database,
	permService *services.PermissionService) ([]int, error) {
	if user == nil || permService == nil {
		return []int{}, nil
	}
	rows, err := db.Query("SELECT id FROM workspaces WHERE active = true")
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		hasView, err := permService.HasWorkspacePermission(user.ID, id, models.PermissionItemView)
		if err != nil {
			slog.Error("error checking view permission", slog.Int("workspace_id", id), slog.Any("error", err))
			continue
		}
		if hasView {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// GetAccessibleWorkspaceKeys returns a set of workspace keys the user can view.
func GetAccessibleWorkspaceKeys(user *models.User, db database.Database,
	permService *services.PermissionService) (map[string]bool, error) {
	if user == nil || permService == nil {
		return map[string]bool{}, nil
	}
	rows, err := db.Query("SELECT id, key FROM workspaces WHERE active = true")
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	keys := make(map[string]bool)
	for rows.Next() {
		var id int
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			continue
		}
		hasView, err := permService.HasWorkspacePermission(user.ID, id, models.PermissionItemView)
		if err != nil {
			continue
		}
		if hasView {
			keys[key] = true
		}
	}
	return keys, rows.Err()
}

// BuildWorkspaceIDPlaceholders builds a parameterized IN clause for workspace IDs.
func BuildWorkspaceIDPlaceholders(ids []int) (placeholders string, args []interface{}) {
	ph := make([]string, len(ids))
	args = make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	placeholders = strings.Join(ph, ", ")
	return placeholders, args
}
