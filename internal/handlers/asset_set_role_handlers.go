package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/models"
	"windshift/internal/utils"
)

// GetSetRoles returns all role assignments for a set (users, groups, and everyone default)
func (h *AssetHandler) GetSetRoles(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "set ID")
		return
	}

	// Check admin permission
	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	// Get user role assignments
	userRoleRows, err := h.db.Query(`
		SELECT uasr.id, uasr.user_id, uasr.set_id, uasr.role_id, uasr.granted_by, uasr.granted_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as user_name,
		       u.email as user_email,
		       ar.name as role_name,
		       COALESCE(g.first_name || ' ' || g.last_name, g.username, '') as granted_by_name
		FROM user_asset_set_roles uasr
		LEFT JOIN users u ON uasr.user_id = u.id
		LEFT JOIN asset_roles ar ON uasr.role_id = ar.id
		LEFT JOIN users g ON uasr.granted_by = g.id
		WHERE uasr.set_id = ?
		ORDER BY uasr.granted_at DESC
	`, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = userRoleRows.Close() }()

	var userRoles []models.UserAssetSetRole
	for userRoleRows.Next() {
		var role models.UserAssetSetRole
		var userName, userEmail, roleName, grantedByName sql.NullString

		err = userRoleRows.Scan(
			&role.ID, &role.UserID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&userName, &userEmail, &roleName, &grantedByName,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		role.UserName = userName.String
		role.UserEmail = userEmail.String
		role.RoleName = roleName.String
		role.GrantedByName = grantedByName.String

		userRoles = append(userRoles, role)
	}

	// Get group role assignments
	groupRoleRows, err := h.db.Query(`
		SELECT gasr.id, gasr.group_id, gasr.set_id, gasr.role_id, gasr.granted_by, gasr.granted_at,
		       g.name as group_name,
		       ar.name as role_name,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as granted_by_name
		FROM group_asset_set_roles gasr
		LEFT JOIN groups g ON gasr.group_id = g.id
		LEFT JOIN asset_roles ar ON gasr.role_id = ar.id
		LEFT JOIN users u ON gasr.granted_by = u.id
		WHERE gasr.set_id = ?
		ORDER BY gasr.granted_at DESC
	`, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = groupRoleRows.Close() }()

	var groupRoles []models.GroupAssetSetRole
	for groupRoleRows.Next() {
		var role models.GroupAssetSetRole
		var groupName, roleName, grantedByName sql.NullString

		err = groupRoleRows.Scan(
			&role.ID, &role.GroupID, &role.SetID, &role.RoleID, &role.GrantedBy, &role.GrantedAt,
			&groupName, &roleName, &grantedByName,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		role.GroupName = groupName.String
		role.RoleName = roleName.String
		role.GrantedByName = grantedByName.String

		groupRoles = append(groupRoles, role)
	}

	// Get everyone default role
	var everyoneRole *models.AssetSetEveryoneRole
	var roleID sql.NullInt64
	var grantedBy sql.NullInt64
	var grantedAt time.Time
	var roleName, grantedByName sql.NullString

	err = h.db.QueryRow(`
		SELECT aser.set_id, aser.role_id, aser.granted_by, aser.granted_at,
		       ar.name as role_name,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as granted_by_name
		FROM asset_set_everyone_roles aser
		LEFT JOIN asset_roles ar ON aser.role_id = ar.id
		LEFT JOIN users u ON aser.granted_by = u.id
		WHERE aser.set_id = ?
	`, setID).Scan(&setID, &roleID, &grantedBy, &grantedAt, &roleName, &grantedByName)

	if err == nil {
		everyoneRole = &models.AssetSetEveryoneRole{
			SetID:         setID,
			GrantedAt:     grantedAt,
			RoleID:        utils.NullInt64ToPtr(roleID),
			GrantedBy:     utils.NullInt64ToPtr(grantedBy),
			RoleName:      roleName.String,
			GrantedByName: grantedByName.String,
		}
	} else if err != sql.ErrNoRows {
		respondInternalError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"user_roles":    userRoles,
		"group_roles":   groupRoles,
		"everyone_role": everyoneRole,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// AssignRoleRequest represents the request body for assigning a role
type AssignRoleRequest struct {
	UserID  *int `json:"user_id,omitempty"`
	GroupID *int `json:"group_id,omitempty"`
	RoleID  int  `json:"role_id"`
}

// AssignSetRole assigns a role to a user or group for a set
func (h *AssetHandler) AssignSetRole(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "set ID")
		return
	}

	// Check admin permission
	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	var req AssignRoleRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate role exists
	var roleExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM asset_roles WHERE id = ?)", req.RoleID).Scan(&roleExists)
	if err != nil || !roleExists {
		respondInvalidID(w, r, "role ID")
		return
	}

	// Must specify either user_id or group_id
	if req.UserID == nil && req.GroupID == nil {
		respondValidationError(w, r, "Must specify user_id or group_id")
		return
	}

	now := time.Now()

	if req.UserID != nil {
		// Assign role to user (upsert)
		_, err = h.db.ExecWrite(`
			INSERT INTO user_asset_set_roles (set_id, user_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(user_id, set_id) DO UPDATE SET role_id = excluded.role_id, granted_by = excluded.granted_by, granted_at = excluded.granted_at
		`, setID, *req.UserID, req.RoleID, currentUser.ID, now)
	} else {
		// Assign role to group (upsert)
		_, err = h.db.ExecWrite(`
			INSERT INTO group_asset_set_roles (set_id, group_id, role_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(group_id, set_id) DO UPDATE SET role_id = excluded.role_id, granted_by = excluded.granted_by, granted_at = excluded.granted_at
		`, setID, *req.GroupID, req.RoleID, currentUser.ID, now)
	}

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// RevokeSetRole revokes a role assignment from a user or group
func (h *AssetHandler) RevokeSetRole(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "set ID")
		return
	}

	roleAssignmentID, err := strconv.Atoi(r.PathValue("assignmentId"))
	if err != nil {
		respondInvalidID(w, r, "assignment ID")
		return
	}

	// Check admin permission
	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	// Check assignment type from query param
	assignmentType := r.URL.Query().Get("type")

	var result sql.Result
	if assignmentType == "group" {
		result, err = h.db.ExecWrite("DELETE FROM group_asset_set_roles WHERE id = ? AND set_id = ?", roleAssignmentID, setID)
	} else {
		result, err = h.db.ExecWrite("DELETE FROM user_asset_set_roles WHERE id = ? AND set_id = ?", roleAssignmentID, setID)
	}

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "Role assignment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
