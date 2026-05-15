package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type GroupHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewGroupHandler(db database.Database, permissionService *services.PermissionService) *GroupHandler {
	return &GroupHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// groupNullableFields holds the nullable scan targets shared by every group-row query.
type groupNullableFields struct {
	ldapDN        sql.NullString
	ldapCN        sql.NullString
	ldapLastSync  sql.NullTime
	createdBy     sql.NullInt64
	createdByName sql.NullString
}

// applyTo populates the corresponding fields on a TeamGroup from nullable scan values.
func (n *groupNullableFields) applyTo(g *models.TeamGroup) {
	g.LDAPDistinguishedName = n.ldapDN.String
	g.LDAPCommonName = n.ldapCN.String
	g.LDAPLastSyncAt = utils.NullTimeToPtr(n.ldapLastSync)
	g.CreatedBy = utils.NullInt64ToPtr(n.createdBy)
	g.CreatedByName = n.createdByName.String
}

// scanGroupMember scans a single group member row from *sql.Rows and returns the populated model.
func scanGroupMember(rows *sql.Rows) (models.TeamGroupMember, error) {
	var member models.TeamGroupMember
	var ldapLastSyncMember sql.NullTime
	var addedBy sql.NullInt64
	var addedByName sql.NullString

	err := rows.Scan(
		&member.ID, &member.GroupID, &member.UserID, &member.LDAPSyncEnabled, &ldapLastSyncMember,
		&addedBy, &member.AddedAt, &member.CreatedAt, &member.UpdatedAt,
		&member.UserEmail, &member.UserName, &member.UserUsername, &addedByName,
	)
	if err != nil {
		return member, err
	}

	member.LDAPLastSyncAt = utils.NullTimeToPtr(ldapLastSyncMember)
	member.AddedBy = utils.NullInt64ToPtr(addedBy)
	member.AddedByName = addedByName.String

	return member, nil
}

// GetAll returns all groups with member counts
func (h *GroupHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT 
			g.id, g.name, g.description, g.ldap_distinguished_name, g.ldap_common_name,
			g.ldap_sync_enabled, g.ldap_last_sync_at, g.is_system_group, g.is_active,
			g.created_by, g.created_at, g.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name,
			(SELECT COUNT(*) FROM group_members gm WHERE gm.group_id = g.id) as member_count
		FROM groups g
		LEFT JOIN users u ON g.created_by = u.id
		ORDER BY g.name
	`

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var groups []models.TeamGroup
	for rows.Next() {
		var group models.TeamGroup
		var nf groupNullableFields

		err := rows.Scan(
			&group.ID, &group.Name, &group.Description, &nf.ldapDN, &nf.ldapCN,
			&group.LDAPSyncEnabled, &nf.ldapLastSync, &group.IsSystemGroup, &group.IsActive,
			&nf.createdBy, &group.CreatedAt, &group.UpdatedAt,
			&nf.createdByName, &group.MemberCount,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		nf.applyTo(&group)
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if groups == nil {
		groups = []models.TeamGroup{}
	}

	respondJSONOK(w, groups)
}

// Get returns a specific group with its members
func (h *GroupHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get group details
	var group models.TeamGroup
	var nf groupNullableFields

	err := h.db.QueryRow(`
		SELECT
			g.id, g.name, g.description, g.ldap_distinguished_name, g.ldap_common_name,
			g.ldap_sync_enabled, g.ldap_last_sync_at, g.is_system_group, g.is_active,
			g.created_by, g.created_at, g.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name
		FROM groups g
		LEFT JOIN users u ON g.created_by = u.id
		WHERE g.id = ?
	`, id).Scan(
		&group.ID, &group.Name, &group.Description, &nf.ldapDN, &nf.ldapCN,
		&group.LDAPSyncEnabled, &nf.ldapLastSync, &group.IsSystemGroup, &group.IsActive,
		&nf.createdBy, &group.CreatedAt, &group.UpdatedAt, &nf.createdByName,
	)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "group")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	nf.applyTo(&group)

	// Get group members
	membersQuery := `
		SELECT 
			gm.id, gm.group_id, gm.user_id, gm.ldap_sync_enabled, gm.ldap_last_sync_at,
			gm.added_by, gm.added_at, gm.created_at, gm.updated_at,
			u.email, u.first_name || ' ' || u.last_name as user_name, u.username,
			adder.first_name || ' ' || adder.last_name as added_by_name
		FROM group_members gm
		JOIN users u ON gm.user_id = u.id
		LEFT JOIN users adder ON gm.added_by = adder.id
		WHERE gm.group_id = ?
		ORDER BY u.last_name, u.first_name
	`

	memberRows, err := h.db.Query(membersQuery, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = memberRows.Close() }()

	var members []models.TeamGroupMember
	for memberRows.Next() {
		member, err := scanGroupMember(memberRows)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		members = append(members, member)
	}

	group.Members = members
	group.MemberCount = len(members)

	respondJSONOK(w, group)
}

// Create creates a new group
func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.TeamGroupCreateRequest](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Get current user ID from session/token
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	createdBy := &currentUser.ID

	// Check uniqueness before insert
	var nameExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE name = ?)", req.Name).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Group name already exists")
		return
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO groups (name, description, is_active, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, req.Name, req.Description, true, createdBy, now, now).Scan(&id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Group name already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Return the created group
	createdGroup := models.TeamGroup{
		ID:          int(id),
		Name:        req.Name,
		Description: req.Description,
		IsActive:    true,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
		MemberCount: 0,
		Members:     []models.TeamGroupMember{},
	}

	// Log audit event
	auditUser := utils.GetCurrentUser(r)
	if auditUser != nil {
		groupID := int(id)
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       auditUser.ID,
			Username:     auditUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionGroupCreate,
			ResourceType: logger.ResourceGroup,
			ResourceID:   &groupID,
			ResourceName: req.Name,
			Details: map[string]interface{}{
				"description": req.Description,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, createdGroup)
}

// Update updates an existing group
func (h *GroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the old group for audit logging
	var oldGroup models.TeamGroup
	err := h.db.QueryRow(`
		SELECT id, name, description, is_active, COALESCE(scim_managed, false)
		FROM groups
		WHERE id = ?
	`, id).Scan(&oldGroup.ID, &oldGroup.Name, &oldGroup.Description, &oldGroup.IsActive, &oldGroup.SCIMManaged)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "group")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check if group is SCIM-managed
	if oldGroup.SCIMManaged {
		respondForbidden(w, r)
		return
	}

	req, ok := decodeJSON[models.TeamGroupUpdateRequest](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Check uniqueness before update
	var nameExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE name = ? AND id != ?)", req.Name, id).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Group name already exists")
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE groups
		SET name = ?, description = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Description, req.IsActive, now, id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Group name already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Log audit event with change tracking
	auditUser := utils.GetCurrentUser(r)
	if auditUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldGroup.Name != req.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldGroup.Name,
				"new": req.Name,
			}
		}
		if oldGroup.Description != req.Description {
			details["description_changed"] = map[string]interface{}{
				"old": oldGroup.Description,
				"new": req.Description,
			}
		}
		if oldGroup.IsActive != req.IsActive {
			details["is_active_changed"] = map[string]interface{}{
				"old": oldGroup.IsActive,
				"new": req.IsActive,
			}
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       auditUser.ID,
			Username:     auditUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionGroupUpdate,
			ResourceType: logger.ResourceGroup,
			ResourceID:   &id,
			ResourceName: req.Name,
			Details:      details,
			Success:      true,
		})
	}

	// Return the updated group (call Get to get full details)
	h.Get(w, r)
}

// Delete deletes a group and all its memberships
func (h *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Ensure authenticated user context exists (required for auditing)
	auditUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get the group details for audit logging before deletion
	var groupName, description string
	var isSystemGroup, isActive, scimManaged bool
	err := h.db.QueryRow(`
		SELECT name, description, is_system_group, is_active, COALESCE(scim_managed, false)
		FROM groups
		WHERE id = ?
	`, id).Scan(&groupName, &description, &isSystemGroup, &isActive, &scimManaged)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "group")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if isSystemGroup {
		respondForbidden(w, r)
		return
	}

	// Check if group is SCIM-managed
	if scimManaged {
		respondForbidden(w, r)
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM groups WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       auditUser.ID,
		Username:     auditUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionGroupDelete,
		ResourceType: logger.ResourceGroup,
		ResourceID:   &id,
		ResourceName: groupName,
		Details: map[string]interface{}{
			"description": description,
			"is_active":   isActive,
		},
		Success: true,
	})

	w.WriteHeader(http.StatusNoContent)
}

// requireGroupMemberAccess parses the group ID from the URL and decodes +
// validates the member request body. It writes an HTTP error and returns
// ok=false when any step fails, so callers can simply return.
func requireGroupMemberAccess(w http.ResponseWriter, r *http.Request) (groupID int, req models.TeamGroupMemberRequest, ok bool) {
	groupID, ok = requireIDParam(w, r, "id")
	if !ok {
		return 0, req, false
	}

	req, ok = decodeJSON[models.TeamGroupMemberRequest](w, r)
	if !ok {
		return 0, req, false
	}

	if len(req.UserIDs) == 0 {
		respondValidationError(w, r, "At least one user ID is required")
		return 0, req, false
	}

	return groupID, req, true
}

// AddMembers adds users to a group
func (h *GroupHandler) AddMembers(w http.ResponseWriter, r *http.Request) {
	groupID, req, ok := requireGroupMemberAccess(w, r)
	if !ok {
		return
	}

	// Check if group exists
	var exists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE id = ?)", groupID).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "group")
		return
	}

	// Get group name for audit logging
	var groupName string
	err = h.db.QueryRow("SELECT name FROM groups WHERE id = ?", groupID).Scan(&groupName)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get current user ID from session/token
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	addedBy := &currentUser.ID

	now := time.Now()
	addedMembers := []models.TeamGroupMember{}
	addedUsernames := []string{}

	for _, userID := range req.UserIDs {
		// Check if user exists
		var userExists bool
		err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", userID).Scan(&userExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !userExists {
			respondValidationError(w, r, "User ID "+strconv.Itoa(userID)+" not found")
			return
		}

		// Check if membership already exists
		var membershipExists bool
		err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id = ? AND user_id = ?)", groupID, userID).Scan(&membershipExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if membershipExists {
			continue // Skip if already a member
		}

		// Add membership
		var membershipID int64
		err = h.db.QueryRow(`
			INSERT INTO group_members (group_id, user_id, added_by, added_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?) RETURNING id
		`, groupID, userID, addedBy, now, now, now).Scan(&membershipID)

		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Invalidate permission cache for the user added to this group
		_ = h.permissionService.OnUserGroupMembershipChanged(userID, groupID)

		// Get user details for the response
		var userEmail, userName, userUsername string
		err = h.db.QueryRow("SELECT email, first_name || ' ' || last_name, username FROM users WHERE id = ?", userID).Scan(&userEmail, &userName, &userUsername)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		addedMembers = append(addedMembers, models.TeamGroupMember{
			ID:           int(membershipID),
			GroupID:      groupID,
			UserID:       userID,
			AddedBy:      addedBy,
			AddedAt:      now,
			CreatedAt:    now,
			UpdatedAt:    now,
			UserEmail:    userEmail,
			UserName:     userName,
			UserUsername: userUsername,
		})
		addedUsernames = append(addedUsernames, userUsername)
	}

	// Log audit event
	auditUser := utils.GetCurrentUser(r)
	if auditUser != nil && len(addedMembers) > 0 {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       auditUser.ID,
			Username:     auditUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionGroupAddMember,
			ResourceType: logger.ResourceGroup,
			ResourceID:   &groupID,
			ResourceName: groupName,
			Details: map[string]interface{}{
				"members_added": addedUsernames,
				"count":         len(addedMembers),
			},
			Success: true,
		})
	}

	respondJSONOK(w, map[string]interface{}{
		"message":       "Members added successfully",
		"added_members": addedMembers,
		"members_added": len(addedMembers),
	})
}

// RemoveMembers removes users from a group
func (h *GroupHandler) RemoveMembers(w http.ResponseWriter, r *http.Request) {
	groupID, req, ok := requireGroupMemberAccess(w, r)
	if !ok {
		return
	}

	// Get group name for audit logging
	var groupName string
	err := h.db.QueryRow("SELECT name FROM groups WHERE id = ?", groupID).Scan(&groupName)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "group")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	removedCount := 0
	removedUsernames := []string{}
	for _, userID := range req.UserIDs {
		// Get username before removing for audit logging
		var username string
		err = h.db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
		if err == nil {
			removedUsernames = append(removedUsernames, username)
		}

		// Remove membership
		result, err := h.db.ExecWrite("DELETE FROM group_members WHERE group_id = ? AND user_id = ?", groupID, userID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			_ = h.permissionService.OnUserGroupMembershipChanged(userID, groupID)
		}
		removedCount += int(rowsAffected)
	}

	// Log audit event
	auditUser := utils.GetCurrentUser(r)
	if auditUser != nil && removedCount > 0 {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       auditUser.ID,
			Username:     auditUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionGroupRemoveMember,
			ResourceType: logger.ResourceGroup,
			ResourceID:   &groupID,
			ResourceName: groupName,
			Details: map[string]interface{}{
				"members_removed": removedUsernames,
				"count":           removedCount,
			},
			Success: true,
		})
	}

	respondJSONOK(w, map[string]interface{}{
		"message":         "Members removed successfully",
		"members_removed": removedCount,
	})
}

// GetUserMemberships returns all groups a user belongs to
func (h *GroupHandler) GetUserMemberships(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	// Check if user exists
	var userExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", userID).Scan(&userExists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !userExists {
		respondNotFound(w, r, "user")
		return
	}

	query := `
		SELECT 
			g.id, g.name, g.description, g.ldap_distinguished_name, g.ldap_common_name,
			g.ldap_sync_enabled, g.ldap_last_sync_at, g.is_system_group, g.is_active,
			g.created_by, g.created_at, g.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name,
			gm.added_at, gm.ldap_sync_enabled as member_ldap_sync
		FROM groups g
		JOIN group_members gm ON g.id = gm.group_id
		LEFT JOIN users u ON g.created_by = u.id
		WHERE gm.user_id = ? AND g.is_active = true
		ORDER BY g.name
	`

	rows, err := h.db.Query(query, userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var groups []models.TeamGroup
	for rows.Next() {
		var group models.TeamGroup
		var nf groupNullableFields
		var memberLdapSync bool

		err := rows.Scan(
			&group.ID, &group.Name, &group.Description, &nf.ldapDN, &nf.ldapCN,
			&group.LDAPSyncEnabled, &nf.ldapLastSync, &group.IsSystemGroup, &group.IsActive,
			&nf.createdBy, &group.CreatedAt, &group.UpdatedAt, &nf.createdByName,
			&group.CreatedAt, &memberLdapSync, // Reusing CreatedAt field for member added_at
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		nf.applyTo(&group)
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if groups == nil {
		groups = []models.TeamGroup{}
	}

	response := models.TeamGroupMembershipResponse{
		UserID: userID,
		Groups: groups,
	}

	respondJSONOK(w, response)
}
