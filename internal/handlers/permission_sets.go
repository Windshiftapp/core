package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type PermissionSetHandler struct {
	BaseHandler
	permissionService *services.PermissionService
}

func NewPermissionSetHandler(db database.Database) *PermissionSetHandler {
	return &PermissionSetHandler{BaseHandler: BaseHandler{db: db}}
}

func NewPermissionSetHandlerWithPool(db database.Database, permissionService *services.PermissionService) *PermissionSetHandler {
	return &PermissionSetHandler{
		BaseHandler:       BaseHandler{db: db},
		permissionService: permissionService,
	}
}

// GetAll returns all permission sets
func (h *PermissionSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	query := `
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		ORDER BY is_system DESC, name ASC`

	rows, err := db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var permissionSets []models.PermissionSet
	for rows.Next() {
		var ps models.PermissionSet
		err := rows.Scan(&ps.ID, &ps.Name, &ps.Description, &ps.IsSystem,
			&ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		permissionSets = append(permissionSets, ps)
	}

	if permissionSets == nil {
		permissionSets = []models.PermissionSet{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(permissionSets)
}

// Get returns a single permission set with its permissions
func (h *PermissionSetHandler) Get(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var ps models.PermissionSet
	err = db.QueryRow(`
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		WHERE id = ?
	`, id).Scan(&ps.ID, &ps.Name, &ps.Description, &ps.IsSystem,
		&ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "permission_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load permissions for this set
	permRows, err := db.Query(`
		SELECT p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at
		FROM permissions p
		JOIN permission_set_permissions psp ON p.id = psp.permission_id
		WHERE psp.permission_set_id = ?
		ORDER BY p.scope, p.permission_name
	`, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = permRows.Close() }()

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
	_ = json.NewEncoder(w).Encode(ps)
}

// Create creates a new permission set
func (h *PermissionSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}
	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	var req models.PermissionSetCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Get current user ID from context
	userID := h.getSessionUserID(r)
	if userID == 0 {
		respondUnauthorized(w, r)
		return
	}

	// Create permission set
	var permSetID int64
	err := writeDB.QueryRow(`
		INSERT INTO permission_sets (name, description, is_system, created_by, created_at, updated_at)
		VALUES (?, ?, 0, ?, ?, ?) RETURNING id
	`, req.Name, req.Description, userID, time.Now(), time.Now()).Scan(&permSetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Add permissions to the set
	for _, permID := range req.PermissionIDs {
		_, err = writeDB.Exec(`
			INSERT INTO permission_set_permissions (permission_set_id, permission_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?)
		`, permSetID, permID, userID, time.Now())
		if err != nil {
			// Log error but continue
			slog.Warn("failed to add permission to set", slog.Int("permission_id", permID), slog.Any("error", err))
		}
	}

	// Return the created permission set
	var ps models.PermissionSet
	err = readDB.QueryRow(`
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		WHERE id = ?
	`, permSetID).Scan(&ps.ID, &ps.Name, &ps.Description, &ps.IsSystem,
		&ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		psID := int(permSetID)
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionPermissionSetCreate,
			ResourceType: logger.ResourcePermissionSet,
			ResourceID:   &psID,
			ResourceName: req.Name,
			Details: map[string]interface{}{
				"description":      req.Description,
				"permission_count": len(req.PermissionIDs),
			},
			Success: true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(ps)
}

// Update updates a permission set
func (h *PermissionSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}
	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the old permission set for audit logging
	var oldPS models.PermissionSet
	err = readDB.QueryRow(`
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		WHERE id = ?
	`, id).Scan(&oldPS.ID, &oldPS.Name, &oldPS.Description, &oldPS.IsSystem,
		&oldPS.CreatedBy, &oldPS.CreatedAt, &oldPS.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "permission_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var req models.PermissionSetUpdateRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Get current user ID
	userID := h.getSessionUserID(r)

	// Update permission set metadata
	_, err = writeDB.Exec(`
		UPDATE permission_sets
		SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Description, time.Now(), id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Replace permissions - delete old ones and insert new ones
	_, err = writeDB.Exec("DELETE FROM permission_set_permissions WHERE permission_set_id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	for _, permID := range req.PermissionIDs {
		_, err = writeDB.Exec(`
			INSERT INTO permission_set_permissions (permission_set_id, permission_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?)
		`, id, permID, userID, time.Now())
		if err != nil {
			slog.Warn("failed to add permission to set", slog.Int("permission_id", permID), slog.Any("error", err))
		}
	}

	// Invalidate cache for all configuration sets using this permission set
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if err = h.permissionService.OnPermissionSetChanged(id); err != nil {
			warnings = append(warnings, createCacheWarning("permission_set", err, fmt.Sprintf("permission_set_id:%d", id)))
		}
	}

	// Return updated permission set
	var ps models.PermissionSet
	err = readDB.QueryRow(`
		SELECT id, name, description, is_system, created_by, created_at, updated_at
		FROM permission_sets
		WHERE id = ?
	`, id).Scan(&ps.ID, &ps.Name, &ps.Description, &ps.IsSystem,
		&ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
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

		_ = logger.LogAudit(h.db, logger.AuditEvent{
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
	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}
	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get the permission set details for audit logging before deletion
	var psName, description string
	var isSystem bool
	err = readDB.QueryRow(`
		SELECT name, description, is_system
		FROM permission_sets
		WHERE id = ?
	`, id).Scan(&psName, &description, &isSystem)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "permission_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check if permission set is in use by configuration sets
	var usageCount int
	err = readDB.QueryRow("SELECT COUNT(*) FROM configuration_sets WHERE permission_set_id = ?", id).Scan(&usageCount)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if usageCount > 0 {
		respondConflict(w, r, "Cannot delete permission set that is in use by configuration sets")
		return
	}

	// Delete permission set (permissions will be cascade deleted)
	_, err = writeDB.Exec("DELETE FROM permission_sets WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
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
	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Check if permission set exists
	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM permission_sets WHERE id = ?)", setID).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "permission_set")
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
	roleRows, err := db.Query(`
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
		defer func() { _ = roleRows.Close() }()
		for roleRows.Next() {
			var ra models.PermissionSetRoleAssignment
			var perm models.Permission
			var role models.WorkspaceRole
			err = roleRows.Scan(&ra.ID, &ra.PermissionSetID, &ra.PermissionID, &ra.RoleID, &ra.CreatedBy, &ra.CreatedAt,
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
	groupRows, err := db.Query(`
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
		defer func() { _ = groupRows.Close() }()
		for groupRows.Next() {
			var ga models.PermissionSetGroupAssignment
			var perm models.Permission
			var group models.Group
			err = groupRows.Scan(&ga.ID, &ga.PermissionSetID, &ga.PermissionID, &ga.GroupID, &ga.CreatedBy, &ga.CreatedAt,
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
	userRows, err := db.Query(`
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
		defer func() { _ = userRows.Close() }()
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
	_ = json.NewEncoder(w).Encode(response)
}

// CreateAssignment adds a role/group/user assignment to a permission in the set
func (h *PermissionSetHandler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var req models.PermissionSetAssignmentRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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
		respondValidationError(w, r, "Must specify exactly one of role_id, group_id, or user_id")
		return
	}

	// Get current user ID
	userID := h.getSessionUserID(r)
	if userID == 0 {
		respondUnauthorized(w, r)
		return
	}

	// Create the appropriate assignment
	switch {
	case req.RoleID != nil:
		_, err = db.Exec(`
			INSERT INTO permission_set_role_assignments (permission_set_id, permission_id, role_id, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, setID, req.PermissionID, *req.RoleID, userID, time.Now())
	case req.GroupID != nil:
		_, err = db.Exec(`
			INSERT INTO permission_set_group_assignments (permission_set_id, permission_id, group_id, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, setID, req.PermissionID, *req.GroupID, userID, time.Now())
	case req.UserID != nil:
		_, err = db.Exec(`
			INSERT INTO permission_set_user_assignments (permission_set_id, permission_id, user_id, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, setID, req.PermissionID, *req.UserID, userID, time.Now())
	}

	if err != nil {
		// Check for unique constraint violation
		errMsg := err.Error()
		if strings.Contains(errMsg, "UNIQUE constraint") || strings.Contains(errMsg, "duplicate key") {
			respondConflict(w, r, "This assignment already exists")
			return
		}
		respondInternalError(w, r, err)
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
	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	assignmentID, err := strconv.Atoi(r.PathValue("assignmentId"))
	if err != nil {
		respondInvalidID(w, r, "assignmentId")
		return
	}

	assignmentType := r.URL.Query().Get("type") // "role", "group", or "user"
	if assignmentType == "" {
		respondValidationError(w, r, "Assignment type parameter required")
		return
	}

	// Use map whitelist to prevent SQL injection if new types are added
	tableMap := map[string]string{
		"role":  "permission_set_role_assignments",
		"group": "permission_set_group_assignments",
		"user":  "permission_set_user_assignments",
	}

	table, tableOk := tableMap[assignmentType]
	if !tableOk {
		respondValidationError(w, r, "Invalid assignment type")
		return
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = ? AND permission_set_id = ?", table) //nolint:gosec // G201: table name from whitelist, parameters are bound
	result, err := db.Exec(query, assignmentID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "assignment")
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
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u.ID
		}
	}
	return 0
}
