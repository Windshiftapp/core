package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"windshift/internal/database"
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
func (h *BaseHandler) requireReadDB(w http.ResponseWriter) (*sql.DB, bool) {
	db, err := h.getReadDB()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, false
	}
	return db, true
}

// requireWriteDB returns the database connection and writes an HTTP error if unavailable.
// Returns nil and false if the database is unavailable (error already written to response).
// Returns db and true if the database is available.
func (h *BaseHandler) requireWriteDB(w http.ResponseWriter) (*sql.DB, bool) {
	db, err := h.getWriteDB()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
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
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return nil, false
	}
	return user, true
}

// RequireWorkspacePermission checks if the user has a specific workspace permission.
// If the user doesn't have permission, it writes a 403 Forbidden response.
// Returns true if permitted, false otherwise (error already written to response).
// Usage:
//
//	if !RequireWorkspacePermission(w, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
//	    return
//	}
func RequireWorkspacePermission(w http.ResponseWriter, userID, workspaceID int, permission string, permService *services.PermissionService) bool {
	hasPermission, err := permService.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil || !hasPermission {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// RequireSystemAdmin checks if the user is a system administrator.
// If the user isn't a system admin, it writes a 403 Forbidden response.
// Returns true if admin, false otherwise (error already written to response).
// Usage:
//
//	if !RequireSystemAdmin(w, user.ID, h.permissionService) {
//	    return
//	}
func RequireSystemAdmin(w http.ResponseWriter, userID int, permService *services.PermissionService) bool {
	isAdmin, err := permService.IsSystemAdmin(userID)
	if err != nil || !isAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
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
		if !RequireSystemAdmin(w, currentUser.ID, permissionService) {
			return nil
		}
	}

	return currentUser
}
