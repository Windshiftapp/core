package handlers

import (
	"database/sql"
	"net/http"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// BaseHandler provides common database access patterns for all handlers
type BaseHandler struct {
	db database.Database
}

// NewBaseHandler creates a new base handler with database connection
func NewBaseHandler(db database.Database) *BaseHandler {
	return &BaseHandler{db: db}
}

// getReadDB returns the database connection for read operations
func (h *BaseHandler) getReadDB() *sql.DB {
	if h.db != nil {
		return h.db.GetDB()
	}
	panic("BaseHandler: database is nil")
}

// getWriteDB returns the database connection for write operations
func (h *BaseHandler) getWriteDB() *sql.DB {
	if h.db != nil {
		return h.db.GetDB()
	}
	panic("BaseHandler: database is nil")
}

// executeInTransaction executes a function within a transaction
func (h *BaseHandler) executeInTransaction(fn func(*sql.Tx) error) error {
	if h.db == nil {
		panic("BaseHandler: database is nil")
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

// AuthorizeUserRequest checks if the current user is authorized to access resources for the target user.
// It returns the current user if authorized, nil otherwise (with appropriate HTTP error written to response).
// Access is granted if:
// - The current user is accessing their own resources (currentUser.ID == targetUserID), OR
// - The current user has system.admin permission
func AuthorizeUserRequest(w http.ResponseWriter, r *http.Request, targetUserID int, permissionService *services.PermissionService) *models.User {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return nil
	}

	// Check if user is system admin or accessing their own resources
	if currentUser.ID != targetUserID {
		// Check for system.admin permission
		isSystemAdmin, err := permissionService.IsSystemAdmin(currentUser.ID)
		if err != nil || !isSystemAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return nil
		}
	}

	return currentUser
}
