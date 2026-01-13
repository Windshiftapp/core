package handlers

import (
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

)

type PermissionSetHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewPermissionSetHandler(db database.Database) *PermissionSetHandler {
	return &PermissionSetHandler{db: db}
}

func NewPermissionSetHandlerWithPool(db database.Database, permissionService *services.PermissionService) *PermissionSetHandler {
	return &PermissionSetHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// getReadDB returns the database connection for read operations
func (h *PermissionSetHandler) getReadDB() *sql.DB {
	return h.db.GetDB()
}

// getWriteDB returns the database connection for write operations
func (h *PermissionSetHandler) getWriteDB() *sql.DB {
	return h.db.GetDB()
}

// GetAll returns all permission sets
func (h *PermissionSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		ORDER BY is_system DESC, name ASC`

	rows, err := h.getReadDB().Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var permissionSets []models.PermissionSet
	for rows.Next() {
		var ps models.PermissionSet
		err := rows.Scan(&ps.ID, &ps.Name, &ps.Description, &ps.IsSystem,
			&ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		permissionSets = append(permissionSets, ps)
	}

	if permissionSets == nil {
		permissionSets = []models.PermissionSet{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(permissionSets)
}

// Get returns a single permission set with its permissions
func (h *PermissionSetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var ps models.PermissionSet
	err = h.getReadDB().QueryRow(`
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		WHERE id = ?
	`, id).Scan(&ps.ID, &ps.Name, &ps.Description, &ps.IsSystem,
		&ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt)

	if err == sql.ErrNoRows {
		http.Error(w, "Permission set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Load permissions for this set
	permRows, err := h.getReadDB().Query(`
		SELECT p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at
		FROM permissions p
		JOIN permission_set_permissions psp ON p.id = psp.permission_id
		WHERE psp.permission_set_id = ?
		ORDER BY p.scope, p.permission_name
	`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer permRows.Close()

	ps.Permissions = []models.Permission{}
	for permRows.Next() {
		var perm models.Permission
		err := permRows.Scan(&perm.ID, &perm.PermissionKey, &perm.PermissionName,
			&perm.Description, &perm.Scope, &perm.IsSystem, &perm.CreatedAt, &perm.UpdatedAt)
		if err == nil {
			ps.Permissions = append(ps.Permissions, perm)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ps)
}

// Create creates a new permission set
func (h *PermissionSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.PermissionSetCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Get current user ID from context
	userID := h.getSessionUserID(r)
	if userID == 0 {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Create permission set
	result, err := h.getWriteDB().Exec(`
		INSERT INTO permission_sets (name, description, is_system, created_by, created_at, updated_at)
		VALUES (?, ?, 0, ?, ?, ?)
	`, req.Name, req.Description, userID, time.Now(), time.Now())

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	permSetID, _ := result.LastInsertId()

	// Add permissions to the set
	for _, permID := range req.PermissionIDs {
		_, err = h.getWriteDB().Exec(`
			INSERT INTO permission_set_permissions (permission_set_id, permission_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?)
		`, permSetID, permID, userID, time.Now())
		if err != nil {
			// Log error but continue
			fmt.Printf("Failed to add permission %d to set: %v\n", permID, err)
		}
	}

	// Return the created permission set
	var ps models.PermissionSet
	err = h.getReadDB().QueryRow(`
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		WHERE id = ?
	`, permSetID).Scan(&ps.ID, &ps.Name, &ps.Description, &ps.IsSystem,
		&ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		psID := int(permSetID)
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionSetCreate,
			ResourceType: logger.ResourcePermissionSet,
			ResourceID:   &psID,
			ResourceName: req.Name,
			Details: map[string]interface{}{
				"description":     req.Description,
				"permission_count": len(req.PermissionIDs),
			},
			Success: true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ps)
}

// Update updates a permission set
func (h *PermissionSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// Get the old permission set for audit logging
	var oldPS models.PermissionSet
	err = h.getReadDB().QueryRow(`
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		WHERE id = ?
	`, id).Scan(&oldPS.ID, &oldPS.Name, &oldPS.Description, &oldPS.IsSystem,
		&oldPS.CreatedBy, &oldPS.CreatedAt, &oldPS.UpdatedAt)

	if err == sql.ErrNoRows {
		http.Error(w, "Permission set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req models.PermissionSetUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get current user ID
	userID := h.getSessionUserID(r)

	// Update permission set metadata
	_, err = h.getWriteDB().Exec(`
		UPDATE permission_sets
		SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Description, time.Now(), id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Replace permissions - delete old ones and insert new ones
	_, err = h.getWriteDB().Exec("DELETE FROM permission_set_permissions WHERE permission_set_id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, permID := range req.PermissionIDs {
		_, err = h.getWriteDB().Exec(`
			INSERT INTO permission_set_permissions (permission_set_id, permission_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?)
		`, id, permID, userID, time.Now())
		if err != nil {
			fmt.Printf("Failed to add permission %d to set: %v\n", permID, err)
		}
	}

	// Invalidate cache for all configuration sets using this permission set
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if err := h.permissionService.OnPermissionSetChanged(id); err != nil {
			warnings = append(warnings, createCacheWarning("permission_set", err, fmt.Sprintf("permission_set_id:%d", id)))
		}
	}

	// Return updated permission set
	var ps models.PermissionSet
	err = h.getReadDB().QueryRow(`
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		WHERE id = ?
	`, id).Scan(&ps.ID, &ps.Name, &ps.Description, &ps.IsSystem,
		&ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log audit event with change tracking
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldPS.Name != ps.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldPS.Name,
				"new": ps.Name,
			}
		}
		if oldPS.Description != ps.Description {
			details["description_changed"] = map[string]interface{}{
				"old": oldPS.Description,
				"new": ps.Description,
			}
		}
		details["permission_count"] = len(req.PermissionIDs)

		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionSetUpdate,
			ResourceType: logger.ResourcePermissionSet,
			ResourceID:   &id,
			ResourceName: ps.Name,
			Details:      details,
			Success:      true,
		})
	}

	respondJSONOKWithWarnings(w, ps, warnings)
}

// Delete deletes a permission set
func (h *PermissionSetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// Get the permission set details for audit logging before deletion
	var psName, description string
	var isSystem bool
	err = h.getReadDB().QueryRow(`
		SELECT name, description, is_system
		FROM permission_sets
		WHERE id = ?
	`, id).Scan(&psName, &description, &isSystem)

	if err == sql.ErrNoRows {
		http.Error(w, "Permission set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if permission set is in use by configuration sets
	var usageCount int
	err = h.getReadDB().QueryRow("SELECT COUNT(*) FROM configuration_sets WHERE permission_set_id = ?", id).Scan(&usageCount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if usageCount > 0 {
		http.Error(w, "Cannot delete permission set that is in use by configuration sets", http.StatusConflict)
		return
	}

	// Delete permission set (permissions will be cascade deleted)
	_, err = h.getWriteDB().Exec("DELETE FROM permission_sets WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionSetDelete,
			ResourceType: logger.ResourcePermissionSet,
			ResourceID:   &id,
			ResourceName: psName,
			Details: map[string]interface{}{
				"description": description,
				"is_system":   isSystem,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAssignments returns all assignments (roles/groups/users) for a permission set
func (h *PermissionSetHandler) GetAssignments(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// Check if permission set exists
	var exists bool
	err = h.getReadDB().QueryRow("SELECT EXISTS(SELECT 1 FROM permission_sets WHERE id = ?)", setID).Scan(&exists)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Permission set not found", http.StatusNotFound)
		return
	}

	type AssignmentResponse struct {
		RoleAssignments  []models.PermissionSetRoleAssignment  `json:"role_assignments"`
		GroupAssignments []models.PermissionSetGroupAssignment `json:"group_assignments"`
		UserAssignments  []models.PermissionSetUserAssignment  `json:"user_assignments"`
	}

	response := AssignmentResponse{
		RoleAssignments:  []models.PermissionSetRoleAssignment{},
		GroupAssignments: []models.PermissionSetGroupAssignment{},
		UserAssignments:  []models.PermissionSetUserAssignment{},
	}

	// Get role assignments
	roleRows, err := h.getReadDB().Query(`
		SELECT ra.id, ra.permission_set_id, ra.permission_id, ra.role_id, ra.created_by, ra.created_at,
		       p.permission_key, p.permission_name, p.description,
		       r.name as role_name
		FROM permission_set_role_assignments ra
		JOIN permissions p ON ra.permission_id = p.id
		JOIN workspace_roles r ON ra.role_id = r.id
		WHERE ra.permission_set_id = ?
		ORDER BY p.permission_name, r.name
	`, setID)
	if err == nil {
		defer roleRows.Close()
		for roleRows.Next() {
			var ra models.PermissionSetRoleAssignment
			var perm models.Permission
			var role models.WorkspaceRole
			err := roleRows.Scan(&ra.ID, &ra.PermissionSetID, &ra.PermissionID, &ra.RoleID, &ra.CreatedBy, &ra.CreatedAt,
				&perm.PermissionKey, &perm.PermissionName, &perm.Description,
				&role.Name)
			if err == nil {
				perm.ID = ra.PermissionID
				role.ID = ra.RoleID
				ra.Permission = &perm
				ra.Role = &role
				response.RoleAssignments = append(response.RoleAssignments, ra)
			}
		}
	}

	// Get group assignments
	groupRows, err := h.getReadDB().Query(`
		SELECT ga.id, ga.permission_set_id, ga.permission_id, ga.group_id, ga.created_by, ga.created_at,
		       p.permission_key, p.permission_name, p.description,
		       g.name as group_name
		FROM permission_set_group_assignments ga
		JOIN permissions p ON ga.permission_id = p.id
		JOIN groups g ON ga.group_id = g.id
		WHERE ga.permission_set_id = ?
		ORDER BY p.permission_name, g.name
	`, setID)
	if err == nil {
		defer groupRows.Close()
		for groupRows.Next() {
			var ga models.PermissionSetGroupAssignment
			var perm models.Permission
			var group models.Group
			err := groupRows.Scan(&ga.ID, &ga.PermissionSetID, &ga.PermissionID, &ga.GroupID, &ga.CreatedBy, &ga.CreatedAt,
				&perm.PermissionKey, &perm.PermissionName, &perm.Description,
				&group.GroupName)
			if err == nil {
				perm.ID = ga.PermissionID
				group.ID = ga.GroupID
				ga.Permission = &perm
				ga.Group = &group
				response.GroupAssignments = append(response.GroupAssignments, ga)
			}
		}
	}

	// Get user assignments
	userRows, err := h.getReadDB().Query(`
		SELECT ua.id, ua.permission_set_id, ua.permission_id, ua.user_id, ua.created_by, ua.created_at,
		       p.permission_key, p.permission_name, p.description,
		       u.username, u.first_name, u.last_name
		FROM permission_set_user_assignments ua
		JOIN permissions p ON ua.permission_id = p.id
		JOIN users u ON ua.user_id = u.id
		WHERE ua.permission_set_id = ?
		ORDER BY p.permission_name, u.username
	`, setID)
	if err == nil {
		defer userRows.Close()
		for userRows.Next() {
			var ua models.PermissionSetUserAssignment
			var perm models.Permission
			var user models.User
			err := userRows.Scan(&ua.ID, &ua.PermissionSetID, &ua.PermissionID, &ua.UserID, &ua.CreatedBy, &ua.CreatedAt,
				&perm.PermissionKey, &perm.PermissionName, &perm.Description,
				&user.Username, &user.FirstName, &user.LastName)
			if err == nil {
				perm.ID = ua.PermissionID
				user.ID = ua.UserID
				ua.Permission = &perm
				ua.User = &user
				response.UserAssignments = append(response.UserAssignments, ua)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CreateAssignment adds a role/group/user assignment to a permission in the set
func (h *PermissionSetHandler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var req models.PermissionSetAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate that exactly one of role/group/user is specified
	count := 0
	if req.RoleID != nil {
		count++
	}
	if req.GroupID != nil {
		count++
	}
	if req.UserID != nil {
		count++
	}
	if count != 1 {
		http.Error(w, "Must specify exactly one of role_id, group_id, or user_id", http.StatusBadRequest)
		return
	}

	// Get current user ID
	userID := h.getSessionUserID(r)
	if userID == 0 {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Create the appropriate assignment
	if req.RoleID != nil {
		_, err = h.getWriteDB().Exec(`
			INSERT INTO permission_set_role_assignments (permission_set_id, permission_id, role_id, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, setID, req.PermissionID, *req.RoleID, userID, time.Now())
	} else if req.GroupID != nil {
		_, err = h.getWriteDB().Exec(`
			INSERT INTO permission_set_group_assignments (permission_set_id, permission_id, group_id, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, setID, req.PermissionID, *req.GroupID, userID, time.Now())
	} else if req.UserID != nil {
		_, err = h.getWriteDB().Exec(`
			INSERT INTO permission_set_user_assignments (permission_set_id, permission_id, user_id, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, setID, req.PermissionID, *req.UserID, userID, time.Now())
	}

	if err != nil {
		// Check for unique constraint violation
		errMsg := err.Error()
		if strings.Contains(errMsg, "UNIQUE constraint") || strings.Contains(errMsg, "duplicate key") {
			http.Error(w, "This assignment already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Invalidate cache
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if err := h.permissionService.OnPermissionSetChanged(setID); err != nil {
			warnings = append(warnings, createCacheWarning("permission_set", err, fmt.Sprintf("permission_set_id:%d", setID)))
		}
	}

	respondJSONCreatedWithWarnings(w, map[string]bool{"success": true}, warnings)
}

// DeleteAssignment removes an assignment
func (h *PermissionSetHandler) DeleteAssignment(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid permission set ID", http.StatusBadRequest)
		return
	}

	assignmentID, err := strconv.Atoi(r.PathValue("assignmentId"))
	if err != nil {
		http.Error(w, "Invalid assignment ID", http.StatusBadRequest)
		return
	}

	assignmentType := r.URL.Query().Get("type") // "role", "group", or "user"
	if assignmentType == "" {
		http.Error(w, "Assignment type parameter required", http.StatusBadRequest)
		return
	}

	// Use map whitelist to prevent SQL injection if new types are added
	tableMap := map[string]string{
		"role":  "permission_set_role_assignments",
		"group": "permission_set_group_assignments",
		"user":  "permission_set_user_assignments",
	}

	table, ok := tableMap[assignmentType]
	if !ok {
		http.Error(w, "Invalid assignment type", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = ? AND permission_set_id = ?", table)
	result, err := h.getWriteDB().Exec(query, assignmentID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Assignment not found", http.StatusNotFound)
		return
	}

	// Invalidate cache
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if err := h.permissionService.OnPermissionSetChanged(setID); err != nil {
			warnings = append(warnings, createCacheWarning("permission_set", err, fmt.Sprintf("permission_set_id:%d", setID)))
		}
	}

	// If there are warnings, return 200 with body, otherwise 204 No Content
	if len(warnings) > 0 {
		respondJSONOKWithWarnings(w, map[string]string{"message": "Assignment deleted successfully"}, warnings)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// getSessionUserID extracts user ID from session context
func (h *PermissionSetHandler) getSessionUserID(r *http.Request) int {
	if user := r.Context().Value("user"); user != nil {
		if u, ok := user.(*models.User); ok {
			return u.ID
		}
	}
	return 0
}
