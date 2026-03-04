package handlers

import (
	"encoding/json"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// TimeProjectPermissionHandler handles project manager/member CRUD
type TimeProjectPermissionHandler struct {
	db                    database.Database
	timePermissionService *services.TimePermissionService
}

// NewTimeProjectPermissionHandler creates a new handler
func NewTimeProjectPermissionHandler(db database.Database, timePermissionService *services.TimePermissionService) *TimeProjectPermissionHandler {
	return &TimeProjectPermissionHandler{
		db:                    db,
		timePermissionService: timePermissionService,
	}
}

// GetManagers returns all managers for a project
func (h *TimeProjectPermissionHandler) GetManagers(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check if user can view this project
	canView, err := h.timePermissionService.CanViewProject(user.ID, projectID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	managers, err := h.timePermissionService.GetProjectManagers(projectID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if managers == nil {
		managers = []models.TimeProjectManager{}
	}

	respondJSONOK(w, managers)
}

// AddManager adds a manager to a project
func (h *TimeProjectPermissionHandler) AddManager(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check if user has project.manage OR is a manager of this project
	hasGlobalManage, err := h.timePermissionService.HasProjectManagePermission(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if !hasGlobalManage {
		var isManager bool
		isManager, err = h.timePermissionService.IsTimeProjectManager(user.ID, projectID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !isManager {
			respondForbidden(w, r)
			return
		}
	}

	var req models.TimeProjectManagerRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	if req.ManagerType != "user" && req.ManagerType != "group" {
		respondValidationError(w, r, "manager_type must be 'user' or 'group'")
		return
	}

	manager, err := h.timePermissionService.AddProjectManager(projectID, req.ManagerType, req.ManagerID, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if user != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       user.ID,
			Username:     user.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionTimeProjectAddManager,
			ResourceType: logger.ResourceTimeProject,
			ResourceID:   &projectID,
			Details:      map[string]interface{}{"manager_id": req.ManagerID},
			Success:      true,
		})
	}

	respondJSONCreated(w, manager)
}

// RemoveManager removes a manager from a project
func (h *TimeProjectPermissionHandler) RemoveManager(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	managerID, ok := requireIDParam(w, r, "managerId")
	if !ok {
		return
	}

	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Only project.manage can remove managers (not project-level managers)
	hasGlobalManage, err := h.timePermissionService.HasProjectManagePermission(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if !hasGlobalManage {
		respondForbidden(w, r)
		return
	}

	// Verify the manager belongs to this project (for safety)
	managers, err := h.timePermissionService.GetProjectManagers(projectID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	found := false
	for _, m := range managers {
		if m.ID == managerID {
			found = true
			break
		}
	}
	if !found {
		respondNotFound(w, r, "manager")
		return
	}

	if err := h.timePermissionService.RemoveProjectManager(managerID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if user != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       user.ID,
			Username:     user.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionTimeProjectRemoveManager,
			ResourceType: logger.ResourceTimeProject,
			ResourceID:   &projectID,
			Details:      map[string]interface{}{"manager_id": managerID},
			Success:      true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMembers returns all members for a project
func (h *TimeProjectPermissionHandler) GetMembers(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check if user can view this project
	canView, err := h.timePermissionService.CanViewProject(user.ID, projectID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	members, err := h.timePermissionService.GetProjectMembers(projectID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if members == nil {
		members = []models.TimeProjectMember{}
	}

	respondJSONOK(w, members)
}

// AddMember adds a member to a project
func (h *TimeProjectPermissionHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Check if user is a manager of this project
	isManager, err := h.timePermissionService.IsTimeProjectManager(user.ID, projectID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !isManager {
		respondForbidden(w, r)
		return
	}

	var req models.TimeProjectMemberRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	if req.MemberType != "user" && req.MemberType != "group" {
		respondValidationError(w, r, "member_type must be 'user' or 'group'")
		return
	}

	member, err := h.timePermissionService.AddProjectMember(projectID, req.MemberType, req.MemberID, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionTimeProjectAddMember,
		ResourceType: logger.ResourceTimeProject,
		ResourceID:   &projectID,
		Details:      map[string]interface{}{"member_id": req.MemberID},
		Success:      true,
	})

	respondJSONCreated(w, member)
}

// RemoveMember removes a member from a project
func (h *TimeProjectPermissionHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	memberID, ok := requireIDParam(w, r, "memberId")
	if !ok {
		return
	}

	// Get user from context
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Check if user is a manager of this project
	isManager, err := h.timePermissionService.IsTimeProjectManager(user.ID, projectID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !isManager {
		respondForbidden(w, r)
		return
	}

	// Verify the member belongs to this project (for safety)
	members, err := h.timePermissionService.GetProjectMembers(projectID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	found := false
	for _, m := range members {
		if m.ID == memberID {
			found = true
			break
		}
	}
	if !found {
		respondNotFound(w, r, "member")
		return
	}

	if err := h.timePermissionService.RemoveProjectMember(memberID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionTimeProjectRemoveMember,
		ResourceType: logger.ResourceTimeProject,
		ResourceID:   &projectID,
		Details:      map[string]interface{}{"member_id": memberID},
		Success:      true,
	})

	w.WriteHeader(http.StatusNoContent)
}
