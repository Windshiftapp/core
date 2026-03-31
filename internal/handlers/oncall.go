package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// OnCallHandler handles HTTP requests for on-call schedule management
type OnCallHandler struct {
	db                database.Database
	onCallRepo        *repository.OnCallRepository
	teamRepo          *repository.TeamRepository
	onCallService     *services.OnCallService
	permissionService *services.PermissionService
}

// NewOnCallHandler creates a new on-call handler
func NewOnCallHandler(db database.Database, onCallRepo *repository.OnCallRepository, teamRepo *repository.TeamRepository, onCallService *services.OnCallService, permissionService *services.PermissionService) *OnCallHandler {
	return &OnCallHandler{
		db:                db,
		onCallRepo:        onCallRepo,
		teamRepo:          teamRepo,
		onCallService:     onCallService,
		permissionService: permissionService,
	}
}

// canManageTeamOnCall checks whether the current user can manage on-call for the given team.
// Returns true if the user has global teams.manage permission or is a team admin.
func (h *OnCallHandler) canManageTeamOnCall(w http.ResponseWriter, r *http.Request, teamID int) bool {
	user, ok := RequireAuth(w, r)
	if !ok {
		return false
	}
	hasGlobal, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionTeamsManage)
	if err == nil && hasGlobal {
		return true
	}
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

// ---------------------------------------------------------------------------
// Schedule CRUD
// ---------------------------------------------------------------------------

// ListSchedules returns all on-call schedules for a team.
func (h *OnCallHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	teamID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	schedules, err := h.onCallRepo.ListSchedulesForTeam(teamID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if schedules == nil {
		schedules = []models.OnCallSchedule{}
	}
	respondJSONOK(w, schedules)
}

// CreateSchedule creates a new on-call schedule for a team.
func (h *OnCallHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	teamID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !h.canManageTeamOnCall(w, r, teamID) {
		return
	}

	req, ok := decodeJSON[models.OnCallScheduleRequest](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "name is required")
		return
	}
	if strings.TrimSpace(req.Timezone) == "" {
		respondValidationError(w, r, "timezone is required")
		return
	}

	user, _ := RequireAuth(w, r)

	id, err := h.onCallRepo.CreateSchedule(teamID, req.Name, req.Description, req.Timezone, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	schedule, err := h.onCallRepo.GetScheduleByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, schedule)
}

// GetSchedule returns a single on-call schedule by ID.
func (h *OnCallHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	schedule, err := h.onCallRepo.GetScheduleByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Schedule")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, schedule)
}

// UpdateSchedule updates an existing on-call schedule.
func (h *OnCallHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	schedule, err := h.onCallRepo.GetScheduleByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Schedule")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !h.canManageTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	req, ok := decodeJSON[models.OnCallScheduleRequest](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "name is required")
		return
	}
	if strings.TrimSpace(req.Timezone) == "" {
		respondValidationError(w, r, "timezone is required")
		return
	}

	isActive := schedule.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	if err := h.onCallRepo.UpdateSchedule(id, req.Name, req.Description, req.Timezone, isActive); err != nil {
		respondInternalError(w, r, err)
		return
	}

	updated, err := h.onCallRepo.GetScheduleByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, updated)
}

// DeleteSchedule removes an on-call schedule.
func (h *OnCallHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	schedule, err := h.onCallRepo.GetScheduleByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Schedule")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !h.canManageTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	if err := h.onCallRepo.DeleteSchedule(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Layer Management
// ---------------------------------------------------------------------------

// AddLayer adds a rotation layer to a schedule.
func (h *OnCallHandler) AddLayer(w http.ResponseWriter, r *http.Request) {
	scheduleID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	schedule, err := h.onCallRepo.GetScheduleByID(scheduleID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Schedule")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !h.canManageTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	req, ok := decodeJSON[models.OnCallScheduleLayerRequest](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "name is required")
		return
	}
	rotationType := strings.TrimSpace(req.RotationType)
	if rotationType != "daily" && rotationType != "weekly" && rotationType != "custom" {
		respondValidationError(w, r, "rotation_type must be daily, weekly, or custom")
		return
	}
	if strings.TrimSpace(req.StartDate) == "" {
		respondValidationError(w, r, "start_date is required")
		return
	}

	id, err := h.onCallRepo.AddLayer(scheduleID, req.Name, req.Priority, rotationType, req.RotationIntervalDays, req.HandoffTime, req.StartDate, req.EndDate)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, map[string]int{"id": id})
}

// UpdateLayer updates an existing rotation layer.
func (h *OnCallHandler) UpdateLayer(w http.ResponseWriter, r *http.Request) {
	scheduleID, ok := requireIDParam(w, r, "scheduleId")
	if !ok {
		return
	}

	schedule, err := h.onCallRepo.GetScheduleByID(scheduleID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Schedule")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !h.canManageTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	layerID, ok := requireIDParam(w, r, "layerId")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.OnCallScheduleLayerRequest](w, r)
	if !ok {
		return
	}

	if err := h.onCallRepo.UpdateLayer(layerID, req.Name, req.Priority, req.RotationType, req.RotationIntervalDays, req.HandoffTime, req.StartDate, req.EndDate); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{"status": "ok"})
}

// DeleteLayer removes a rotation layer.
func (h *OnCallHandler) DeleteLayer(w http.ResponseWriter, r *http.Request) {
	scheduleID, ok := requireIDParam(w, r, "scheduleId")
	if !ok {
		return
	}

	schedule, err := h.onCallRepo.GetScheduleByID(scheduleID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Schedule")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !h.canManageTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	layerID, ok := requireIDParam(w, r, "layerId")
	if !ok {
		return
	}

	if err := h.onCallRepo.DeleteLayer(layerID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetLayerMembers replaces the member list for a rotation layer.
func (h *OnCallHandler) SetLayerMembers(w http.ResponseWriter, r *http.Request) {
	scheduleID, ok := requireIDParam(w, r, "scheduleId")
	if !ok {
		return
	}

	schedule, err := h.onCallRepo.GetScheduleByID(scheduleID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Schedule")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !h.canManageTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	layerID, ok := requireIDParam(w, r, "layerId")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.SetLayerMembersRequest](w, r)
	if !ok {
		return
	}

	if err := h.onCallRepo.SetLayerMembers(layerID, req.UserIDs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Overrides
// ---------------------------------------------------------------------------

// CreateOverride creates a manual override for a schedule.
func (h *OnCallHandler) CreateOverride(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	scheduleID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.OnCallOverrideRequest](w, r)
	if !ok {
		return
	}

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		respondValidationError(w, r, "start_time must be a valid RFC3339 timestamp")
		return
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		respondValidationError(w, r, "end_time must be a valid RFC3339 timestamp")
		return
	}

	id, err := h.onCallRepo.CreateOverride(scheduleID, req.UserID, req.OverrideUserID, startTime, endTime, req.Reason, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, map[string]int{"id": id})
}

// DeleteOverride removes a schedule override.
func (h *OnCallHandler) DeleteOverride(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	overrideID, ok := requireIDParam(w, r, "overrideId")
	if !ok {
		return
	}

	if err := h.onCallRepo.DeleteOverride(overrideID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Current On-Call
// ---------------------------------------------------------------------------

// GetCurrentOnCall returns who is currently on call for a schedule.
func (h *OnCallHandler) GetCurrentOnCall(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	scheduleID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	result, err := h.onCallService.GetCurrentOnCall(scheduleID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, result)
}

// ---------------------------------------------------------------------------
// Swap Requests
// ---------------------------------------------------------------------------

// CreateSwapRequest creates a shift swap request between two on-call members.
func (h *OnCallHandler) CreateSwapRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	scheduleID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.OnCallSwapRequestCreate](w, r)
	if !ok {
		return
	}

	swapStart, err := time.Parse(time.RFC3339, req.SwapStart)
	if err != nil {
		respondValidationError(w, r, "swap_start must be a valid RFC3339 timestamp")
		return
	}
	swapEnd, err := time.Parse(time.RFC3339, req.SwapEnd)
	if err != nil {
		respondValidationError(w, r, "swap_end must be a valid RFC3339 timestamp")
		return
	}

	id, err := h.onCallRepo.CreateSwapRequest(scheduleID, user.ID, req.TargetUserID, swapStart, swapEnd)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, map[string]int{"id": id})
}

// RespondSwapRequest handles approval or rejection of a swap request.
func (h *OnCallHandler) RespondSwapRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.OnCallSwapRequestResponse](w, r)
	if !ok {
		return
	}

	status := strings.TrimSpace(req.Status)
	if status != "approved" && status != "rejected" {
		respondValidationError(w, r, "status must be approved or rejected")
		return
	}

	swap, err := h.onCallRepo.GetSwapRequestByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Swap request")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if swap.TargetUserID != user.ID {
		respondForbidden(w, r)
		return
	}

	if err := h.onCallRepo.UpdateSwapRequestStatus(id, status); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if status == "approved" {
		if err := h.onCallService.CreateSwapOverride(id); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	respondJSONOK(w, map[string]string{"status": status})
}

// ---------------------------------------------------------------------------
// Escalation Policies
// ---------------------------------------------------------------------------

// ListPolicies returns all escalation policies for a team.
func (h *OnCallHandler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	teamID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	policies, err := h.onCallRepo.ListPoliciesForTeam(teamID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if policies == nil {
		policies = []models.OnCallEscalationPolicy{}
	}
	respondJSONOK(w, policies)
}

// CreatePolicy creates a new escalation policy for a team.
func (h *OnCallHandler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	teamID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !h.canManageTeamOnCall(w, r, teamID) {
		return
	}

	req, ok := decodeJSON[models.OnCallEscalationPolicyRequest](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "name is required")
		return
	}

	user, _ := RequireAuth(w, r)

	id, err := h.onCallRepo.CreatePolicy(teamID, req.Name, req.Description, req.RepeatCount, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, map[string]int{"id": id})
}

// GetPolicy returns a single escalation policy by ID.
func (h *OnCallHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	policy, err := h.onCallRepo.GetPolicyByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Policy")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, policy)
}

// UpdatePolicy updates an existing escalation policy.
func (h *OnCallHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	policy, err := h.onCallRepo.GetPolicyByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Policy")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !h.canManageTeamOnCall(w, r, policy.TeamID) {
		return
	}

	req, ok := decodeJSON[models.OnCallEscalationPolicyRequest](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "name is required")
		return
	}

	isActive := policy.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	if err := h.onCallRepo.UpdatePolicy(id, req.Name, req.Description, req.RepeatCount, isActive); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{"status": "ok"})
}

// DeletePolicy removes an escalation policy.
func (h *OnCallHandler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	policy, err := h.onCallRepo.GetPolicyByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Policy")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !h.canManageTeamOnCall(w, r, policy.TeamID) {
		return
	}

	if err := h.onCallRepo.DeletePolicy(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetRules replaces the escalation rules for a policy.
func (h *OnCallHandler) SetRules(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	policy, err := h.onCallRepo.GetPolicyByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Policy")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if !h.canManageTeamOnCall(w, r, policy.TeamID) {
		return
	}

	req, ok := decodeJSON[models.SetEscalationRulesRequest](w, r)
	if !ok {
		return
	}

	if err := h.onCallRepo.SetEscalationRules(id, req.Rules); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Incidents
// ---------------------------------------------------------------------------

// ListIncidents returns active incidents, optionally filtered by policy.
func (h *OnCallHandler) ListIncidents(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var policyID *int
	if pidStr := r.URL.Query().Get("policy_id"); pidStr != "" {
		parsed, err := strconv.Atoi(pidStr)
		if err != nil {
			respondValidationError(w, r, "Invalid policy_id")
			return
		}
		policyID = &parsed
	}

	incidents, err := h.onCallRepo.GetActiveIncidents(policyID, "")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if incidents == nil {
		incidents = []models.OnCallIncident{}
	}
	respondJSONOK(w, incidents)
}

// AcknowledgeIncident marks an incident as acknowledged.
func (h *OnCallHandler) AcknowledgeIncident(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.onCallService.AcknowledgeIncident(id, user.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{"status": "acknowledged"})
}

// ResolveIncident marks an incident as resolved.
func (h *OnCallHandler) ResolveIncident(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.onCallService.ResolveIncident(id, user.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{"status": "resolved"})
}
