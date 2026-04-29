package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TeamHandler struct {
	db                database.Database
	teamRepo          *repository.TeamRepository
	leaveRepo         *repository.LeaveRepository
	permissionService *services.PermissionService
}

func NewTeamHandler(db database.Database, teamRepo *repository.TeamRepository, leaveRepo *repository.LeaveRepository, permissionService *services.PermissionService) *TeamHandler {
	return &TeamHandler{
		db:                db,
		teamRepo:          teamRepo,
		leaveRepo:         leaveRepo,
		permissionService: permissionService,
	}
}

// canManageTeam checks if the current user has permission to manage a team.
// Returns true if the user has the global teams.manage permission or is a team admin.
func (h *TeamHandler) canManageTeam(w http.ResponseWriter, r *http.Request, teamID int) bool {
	user, ok := RequireAuth(w, r)
	if !ok {
		return false
	}
	// Check teams.manage global permission
	hasPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionTeamsManage)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if hasPerm {
		return true
	}
	// Check team admin role
	isAdmin, err := h.teamRepo.IsTeamAdmin(teamID, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if isAdmin {
		return true
	}
	respondForbidden(w, r)
	return false
}

// requireTeamManage parses the team ID from the route and checks manage permission.
// Returns the teamID and true on success, or writes an error response and returns false.
func (h *TeamHandler) requireTeamManage(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return 0, false
	}
	if !h.canManageTeam(w, r, id) {
		return 0, false
	}
	return id, true
}

// requireTeamMemberAccess parses the team ID and member user ID from the route,
// and checks team manage permission. Returns (teamID, memberID, ok).
func (h *TeamHandler) requireTeamMemberAccess(w http.ResponseWriter, r *http.Request) (teamID, memberID int, ok bool) {
	teamID, ok = h.requireTeamManage(w, r)
	if !ok {
		return 0, 0, false
	}
	memberID, ok = requireIDParam(w, r, "userId")
	if !ok {
		return 0, 0, false
	}
	return teamID, memberID, true
}

// GetAll returns all teams with member counts
func (h *TeamHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	teams, err := h.teamRepo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if teams == nil {
		teams = []models.Team{}
	}

	respondJSONOK(w, teams)
}

// Get returns a specific team with its direct members and mapped groups
func (h *TeamHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	team, err := h.teamRepo.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "team")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Load direct members
	members, err := h.teamRepo.GetDirectMembers(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	team.DirectMembers = members

	// Load mapped groups
	groups, err := h.teamRepo.GetMappedGroups(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	team.MappedGroups = groups

	respondJSONOK(w, team)
}

// Create creates a new team
func (h *TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	hasPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionTeamsManage)
	if err != nil || !hasPerm {
		respondForbidden(w, r)
		return
	}

	req, ok := decodeJSON[models.TeamCreateRequest](w, r)
	if !ok {
		return
	}

	req.Name = utils.SanitizeName(req.Name)
	req.Description = utils.SanitizeDescription(req.Description)

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	teamID, err := h.teamRepo.Create(req.Name, req.Description, req.Icon, req.Color, req.AvatarURL, user.ID)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Team name already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	team, err := h.teamRepo.GetByID(teamID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Audit log
	auditUser := utils.GetCurrentUser(r)
	if auditUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       auditUser.ID,
			Username:     auditUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionTeamCreate,
			ResourceType: logger.ResourceTeam,
			ResourceID:   &teamID,
			ResourceName: req.Name,
			Details: map[string]interface{}{
				"description": req.Description,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, team)
}

// Update updates an existing team
func (h *TeamHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireTeamManage(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.TeamUpdateRequest](w, r)
	if !ok {
		return
	}

	req.Name = utils.SanitizeName(req.Name)
	req.Description = utils.SanitizeDescription(req.Description)

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	err := h.teamRepo.Update(id, req.Name, req.Description, req.IsActive, req.Icon, req.Color, req.AvatarURL)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Team name already exists")
			return
		}
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "team")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	team, err := h.teamRepo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, team)
}

// Delete deletes a team
func (h *TeamHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	hasPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionTeamsManage)
	if err != nil || !hasPerm {
		respondForbidden(w, r)
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get team for audit logging before deletion
	team, err := h.teamRepo.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "team")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	err = h.teamRepo.Delete(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "team")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Audit log
	auditUser := utils.GetCurrentUser(r)
	if auditUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       auditUser.ID,
			Username:     auditUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionTeamDelete,
			ResourceType: logger.ResourceTeam,
			ResourceID:   &id,
			ResourceName: team.Name,
			Details: map[string]interface{}{
				"description": team.Description,
				"is_active":   team.IsActive,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetResolvedMembers returns the resolved union of direct members and group members
func (h *TeamHandler) GetResolvedMembers(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	members, err := h.teamRepo.GetResolvedMembers(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if members == nil {
		members = []models.ResolvedTeamMember{}
	}

	respondJSONOK(w, members)
}

// AddMembers adds direct members to a team
func (h *TeamHandler) AddMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireTeamManage(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.TeamMemberRequest](w, r)
	if !ok {
		return
	}

	if len(req.UserIDs) == 0 {
		respondValidationError(w, r, "At least one user ID is required")
		return
	}

	// Default role to "member" if empty
	role := req.Role
	if role == "" {
		role = "member"
	}

	// Validate role
	if role != "member" && role != "admin" {
		respondValidationError(w, r, "Role must be 'member' or 'admin'")
		return
	}

	currentUser := utils.GetCurrentUser(r)
	addedBy := currentUser.ID

	for _, userID := range req.UserIDs {
		// Check user exists
		var userExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", userID).Scan(&userExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !userExists {
			respondValidationError(w, r, "User not found")
			return
		}

		err = h.teamRepo.AddDirectMember(id, userID, role, addedBy)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	respondJSONOK(w, map[string]interface{}{
		"message": "Members added successfully",
	})
}

// RemoveMembers removes direct members from a team
func (h *TeamHandler) RemoveMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireTeamManage(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.TeamMemberRequest](w, r)
	if !ok {
		return
	}

	for _, userID := range req.UserIDs {
		err := h.teamRepo.RemoveDirectMember(id, userID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	respondJSONOK(w, map[string]interface{}{
		"message": "Members removed successfully",
	})
}

// UpdateMemberRole updates the role of a direct team member
func (h *TeamHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	teamID, userID, ok := h.requireTeamMemberAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.TeamMemberRoleRequest](w, r)
	if !ok {
		return
	}

	if req.Role != "member" && req.Role != "admin" {
		respondValidationError(w, r, "Role must be 'member' or 'admin'")
		return
	}

	err := h.teamRepo.UpdateMemberRole(teamID, userID, req.Role)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "team member")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"message": "Member role updated successfully",
	})
}

// AddGroups adds group mappings to a team
func (h *TeamHandler) AddGroups(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireTeamManage(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.TeamGroupRequest](w, r)
	if !ok {
		return
	}

	if len(req.GroupIDs) == 0 {
		respondValidationError(w, r, "At least one group ID is required")
		return
	}

	currentUser := utils.GetCurrentUser(r)
	addedBy := currentUser.ID

	for _, groupID := range req.GroupIDs {
		// Check group exists
		var groupExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE id = ?)", groupID).Scan(&groupExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !groupExists {
			respondValidationError(w, r, "Group not found")
			return
		}

		err = h.teamRepo.AddGroupMapping(id, groupID, addedBy)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	respondJSONOK(w, map[string]interface{}{
		"message": "Groups added successfully",
	})
}

// RemoveGroups removes group mappings from a team
func (h *TeamHandler) RemoveGroups(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireTeamManage(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.TeamGroupRequest](w, r)
	if !ok {
		return
	}

	for _, groupID := range req.GroupIDs {
		err := h.teamRepo.RemoveGroupMapping(id, groupID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	respondJSONOK(w, map[string]interface{}{
		"message": "Groups removed successfully",
	})
}

// GetTeamsForUser returns all teams a user belongs to (directly or via groups)
func (h *TeamHandler) GetTeamsForUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	teams, err := h.teamRepo.GetTeamsForUser(userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if teams == nil {
		teams = []models.Team{}
	}

	respondJSONOK(w, teams)
}
