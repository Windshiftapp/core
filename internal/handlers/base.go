package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
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
func (h *BaseHandler) getReadDB() (*sql.DB, error) {
	if h.db != nil {
		return h.db.GetDB(), nil
	}
	return nil, ErrDatabaseNil
}

// getWriteDB returns the database connection for write operations.
// Returns an error if the database connection is not initialized.
func (h *BaseHandler) getWriteDB() (*sql.DB, error) {
	if h.db != nil {
		return h.db.GetDB(), nil
	}
	return nil, ErrDatabaseNil
}

// requireReadDB returns the database connection and writes an HTTP error if unavailable.
// Returns nil and false if the database is unavailable (error already written to response).
// Returns db and true if the database is available.
func (h *BaseHandler) requireReadDB(w http.ResponseWriter, r *http.Request) (*sql.DB, bool) {
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
func (h *BaseHandler) requireWriteDB(w http.ResponseWriter, r *http.Request) (*sql.DB, bool) {
	db, err := h.getWriteDB()
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return db, true
}

// executeInTransaction executes a function within a transaction
func (h *BaseHandler) executeInTransaction(fn func(*sql.Tx) error) error {
	if h.db == nil {
		return ErrDatabaseNil
	}

	tx, err := h.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx.(*sql.Tx)); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
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
func CheckItemPermission(w http.ResponseWriter, r *http.Request, db database.Database,
	permService *services.PermissionService, itemID int, permission string) bool {
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return false
	}
	var workspaceID int
	err := db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
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

// GetAccessibleWorkspaceIDs returns IDs of active workspaces the user can view.
func GetAccessibleWorkspaceIDs(user *models.User, db database.Database,
	permService *services.PermissionService) ([]int, error) {
	if user == nil || permService == nil {
		return []int{}, nil
	}
	rows, err := db.Query("SELECT id FROM workspaces WHERE active = 1")
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()
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
	rows, err := db.Query("SELECT id, key FROM workspaces WHERE active = 1")
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()
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
func BuildWorkspaceIDPlaceholders(ids []int) (string, []interface{}) {
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return strings.Join(placeholders, ", "), args
}
