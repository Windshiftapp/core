package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type OnCallHandler struct {
	onCallRepo        *repository.OnCallRepository
	teamRepo          *repository.TeamRepository
	itemRepo          *repository.ItemRepository
	onCallService     *services.OnCallService
	incidentService   *services.IncidentService
	permissionService *services.PermissionService
	auditor           *logger.Auditor
}

// sanitizeScheduleRequest sanitizes schedule fields.
func sanitizeScheduleRequest(req *models.OnCallScheduleRequest) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText},
		sanitize.Pair{Target: &req.Timezone, Policy: sanitize.ShortIdentifier},
	)
}

func (h *OnCallHandler) auditMutation(r *http.Request, action, resource string, resourceID *int, resourceName string, details map[string]any) {
	if user := utils.GetCurrentUser(r); user != nil {
		h.auditor.LogWithDetails(r, user, action, resource, resourceID, resourceName, details)
	}
}

// sanitizeLayerRequest sanitizes rotation-layer fields.
func sanitizeLayerRequest(req *models.OnCallScheduleLayerRequest) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.RotationType, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.HandoffTime, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.StartDate, Policy: sanitize.ShortIdentifier},
	)
	if req.EndDate != nil {
		sanitize.Apply(req.EndDate, sanitize.ShortIdentifier)
	}
}

// sanitizeEscalationPolicy sanitizes escalation-policy fields.
func sanitizeEscalationPolicy(req *models.OnCallEscalationPolicyRequest) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText},
	)
}

func NewOnCallHandler(onCallRepo *repository.OnCallRepository, teamRepo *repository.TeamRepository, itemRepo *repository.ItemRepository, onCallService *services.OnCallService, incidentService *services.IncidentService, permissionService *services.PermissionService, auditor *logger.Auditor) *OnCallHandler {
	return &OnCallHandler{
		onCallRepo:        onCallRepo,
		teamRepo:          teamRepo,
		itemRepo:          itemRepo,
		onCallService:     onCallService,
		incidentService:   incidentService,
		permissionService: permissionService,
		auditor:           auditor,
	}
}

// canManageTeamOnCall permits global team managers and team admins.
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

// canViewTeamOnCall permits managers and team members to view roster data.
func (h *OnCallHandler) canViewTeamOnCall(w http.ResponseWriter, r *http.Request, teamID int) bool {
	user, ok := RequireAuth(w, r)
	if !ok {
		return false
	}
	allowed, err := h.hasTeamOnCallViewAccess(user.ID, teamID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if allowed {
		return true
	}
	respondForbidden(w, r)
	return false
}

// hasTeamOnCallViewAccess checks roster access without writing a response.
func (h *OnCallHandler) hasTeamOnCallViewAccess(userID, teamID int) (bool, error) {
	hasGlobal, err := h.permissionService.HasGlobalPermission(userID, models.PermissionTeamsManage)
	if err == nil && hasGlobal {
		return true, nil
	}
	isAdmin, err := h.teamRepo.IsTeamAdmin(teamID, userID)
	if err != nil {
		return false, err
	}
	if isAdmin {
		return true, nil
	}
	isMember, err := h.teamRepo.IsTeamMember(teamID, userID)
	if err != nil {
		return false, err
	}
	return isMember, nil
}

// resolveSchedule loads and authorizes management of a schedule.
func (h *OnCallHandler) resolveSchedule(w http.ResponseWriter, r *http.Request, paramName string) (*models.OnCallSchedule, bool) {
	id, ok := requireIDParam(w, r, paramName)
	if !ok {
		return nil, false
	}

	schedule, err := h.onCallRepo.GetScheduleByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Schedule")
			return nil, false
		}
		respondInternalError(w, r, err)
		return nil, false
	}

	if !h.canManageTeamOnCall(w, r, schedule.TeamID) {
		return nil, false
	}

	return schedule, true
}

// resolveLayerForSchedule verifies a layer belongs to an authorized schedule.
// Foreign layers return 404 to avoid disclosing their existence.
func (h *OnCallHandler) resolveLayerForSchedule(w http.ResponseWriter, r *http.Request, paramName string, schedule *models.OnCallSchedule) (int, bool) {
	layerID, ok := requireIDParam(w, r, paramName)
	if !ok {
		return 0, false
	}

	layer, err := h.onCallRepo.GetLayerByID(layerID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Layer")
			return 0, false
		}
		respondInternalError(w, r, err)
		return 0, false
	}

	if layer.ScheduleID != schedule.ID {
		respondNotFound(w, r, "Layer")
		return 0, false
	}

	return layerID, true
}

// resolvePolicy loads and authorizes management of an escalation policy.
func (h *OnCallHandler) resolvePolicy(w http.ResponseWriter, r *http.Request, paramName string) (*models.OnCallEscalationPolicy, bool) {
	id, ok := requireIDParam(w, r, paramName)
	if !ok {
		return nil, false
	}

	policy, err := h.onCallRepo.GetPolicyByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Policy")
			return nil, false
		}
		respondInternalError(w, r, err)
		return nil, false
	}

	if !h.canManageTeamOnCall(w, r, policy.TeamID) {
		return nil, false
	}

	return policy, true
}

// validateScheduleRequest validates required schedule fields.
func validateScheduleRequest(w http.ResponseWriter, r *http.Request, req models.OnCallScheduleRequest) bool {
	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "name is required")
		return false
	}
	if strings.TrimSpace(req.Timezone) == "" {
		respondValidationError(w, r, "timezone is required")
		return false
	}
	if _, _, err := services.ResolveTimezone(req.Timezone); err != nil {
		respondValidationError(w, r, err.Error())
		return false
	}
	return true
}

func validateLayerRequest(w http.ResponseWriter, r *http.Request, req models.OnCallScheduleLayerRequest) bool {
	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "name is required")
		return false
	}
	rotationType := strings.TrimSpace(req.RotationType)
	if rotationType != "daily" && rotationType != "weekly" && rotationType != "custom" {
		respondValidationError(w, r, "rotation_type must be daily, weekly, or custom")
		return false
	}
	if rotationType == "custom" && req.RotationIntervalDays <= 0 {
		respondValidationError(w, r, "rotation_interval_days must be positive for custom rotations")
		return false
	}
	if _, err := time.Parse("15:04", req.HandoffTime); err != nil {
		respondValidationError(w, r, "handoff_time must be in HH:MM format")
		return false
	}
	startDate, err := time.Parse(time.DateOnly, req.StartDate)
	if err != nil {
		respondValidationError(w, r, "start_date must be in YYYY-MM-DD format")
		return false
	}
	if req.EndDate != nil {
		endDate, err := time.Parse(time.DateOnly, *req.EndDate)
		if err != nil {
			respondValidationError(w, r, "end_date must be in YYYY-MM-DD format")
			return false
		}
		if endDate.Before(startDate) {
			respondValidationError(w, r, "end_date must be on or after start_date")
			return false
		}
	}
	return true
}

// ListSchedules returns all on-call schedules for a team.
func (h *OnCallHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	teamID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	includeRoster, err := h.hasTeamOnCallViewAccess(user.ID, teamID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	schedules, err := h.onCallRepo.ListSchedulesForTeam(teamID, includeRoster)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if schedules == nil {
		schedules = []models.OnCallSchedule{}
	}
	if includeRoster {
		now := time.Now()
		for i := range schedules {
			schedules[i].CurrentOnCall = h.onCallService.CurrentOnCallForSchedule(&schedules[i], now)
		}
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
	sanitizeScheduleRequest(&req)

	if !validateScheduleRequest(w, r, req) {
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
	h.auditMutation(r, logger.ActionOnCallScheduleCreate, logger.ResourceOnCallSchedule, &id, schedule.Name, map[string]any{"team_id": teamID})

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
	if !h.canViewTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	respondJSONOK(w, schedule)
}

// UpdateSchedule updates an existing on-call schedule.
func (h *OnCallHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	schedule, ok := h.resolveSchedule(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.OnCallScheduleRequest](w, r)
	if !ok {
		return
	}
	sanitizeScheduleRequest(&req)

	if !validateScheduleRequest(w, r, req) {
		return
	}

	isActive := schedule.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	if err := h.onCallRepo.UpdateSchedule(schedule.ID, req.Name, req.Description, req.Timezone, isActive); err != nil {
		respondInternalError(w, r, err)
		return
	}

	updated, err := h.onCallRepo.GetScheduleByID(schedule.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallScheduleUpdate, logger.ResourceOnCallSchedule, &schedule.ID, updated.Name, map[string]any{"team_id": schedule.TeamID})

	respondJSONOK(w, updated)
}

// DeleteSchedule removes an on-call schedule.
func (h *OnCallHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	schedule, ok := h.resolveSchedule(w, r, "id")
	if !ok {
		return
	}

	if err := h.onCallRepo.DeleteSchedule(schedule.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallScheduleDelete, logger.ResourceOnCallSchedule, &schedule.ID, schedule.Name, map[string]any{"team_id": schedule.TeamID})

	w.WriteHeader(http.StatusNoContent)
}

// AddLayer adds a rotation layer to a schedule.
func (h *OnCallHandler) AddLayer(w http.ResponseWriter, r *http.Request) {
	schedule, ok := h.resolveSchedule(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.OnCallScheduleLayerRequest](w, r)
	if !ok {
		return
	}
	sanitizeLayerRequest(&req)

	if !validateLayerRequest(w, r, req) {
		return
	}
	rotationType := strings.TrimSpace(req.RotationType)

	id, err := h.onCallRepo.AddLayer(schedule.ID, req.Name, req.Priority, rotationType, req.RotationIntervalDays, req.HandoffTime, req.StartDate, req.EndDate)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallLayerCreate, logger.ResourceOnCallLayer, &id, req.Name, map[string]any{"schedule_id": schedule.ID})

	respondJSONCreated(w, map[string]int{"id": id})
}

// UpdateLayer updates an existing rotation layer.
func (h *OnCallHandler) UpdateLayer(w http.ResponseWriter, r *http.Request) {
	schedule, ok := h.resolveSchedule(w, r, "scheduleId")
	if !ok {
		return
	}

	layerID, ok := h.resolveLayerForSchedule(w, r, "layerId", schedule)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.OnCallScheduleLayerRequest](w, r)
	if !ok {
		return
	}
	sanitizeLayerRequest(&req)
	if !validateLayerRequest(w, r, req) {
		return
	}

	if err := h.onCallRepo.UpdateLayer(layerID, req.Name, req.Priority, strings.TrimSpace(req.RotationType), req.RotationIntervalDays, req.HandoffTime, req.StartDate, req.EndDate); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallLayerUpdate, logger.ResourceOnCallLayer, &layerID, req.Name, map[string]any{"schedule_id": schedule.ID})

	respondJSONOK(w, map[string]string{"status": "ok"})
}

// DeleteLayer removes a rotation layer.
func (h *OnCallHandler) DeleteLayer(w http.ResponseWriter, r *http.Request) {
	schedule, ok := h.resolveSchedule(w, r, "scheduleId")
	if !ok {
		return
	}

	layerID, ok := h.resolveLayerForSchedule(w, r, "layerId", schedule)
	if !ok {
		return
	}

	if err := h.onCallRepo.DeleteLayer(layerID); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallLayerDelete, logger.ResourceOnCallLayer, &layerID, "", map[string]any{"schedule_id": schedule.ID})

	w.WriteHeader(http.StatusNoContent)
}

// SetLayerMembers replaces the member list for a rotation layer.
func (h *OnCallHandler) SetLayerMembers(w http.ResponseWriter, r *http.Request) {
	schedule, ok := h.resolveSchedule(w, r, "scheduleId")
	if !ok {
		return
	}

	layerID, ok := h.resolveLayerForSchedule(w, r, "layerId", schedule)
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

	h.auditMutation(r, logger.ActionOnCallSetLayerMembers, logger.ResourceOnCallLayer, &layerID, "", map[string]any{"schedule_id": schedule.ID, "user_ids": req.UserIDs})

	respondJSONOK(w, map[string]string{"status": "ok"})
}

// CreateOverride creates a manual override for a schedule.
func (h *OnCallHandler) CreateOverride(w http.ResponseWriter, r *http.Request) {
	schedule, ok := h.resolveSchedule(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.OnCallOverrideRequest](w, r)
	if !ok {
		return
	}
	// Timestamp fields are sanitized before RFC3339 validation.
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Reason, Policy: sanitize.RichText},
		sanitize.Pair{Target: &req.StartTime, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.EndTime, Policy: sanitize.ShortIdentifier},
	)

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

	id, err := h.onCallRepo.CreateOverride(schedule.ID, req.UserID, req.OverrideUserID, startTime, endTime, req.Reason, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallOverrideCreate, logger.ResourceOnCallOverride, &id, "", map[string]any{
		"schedule_id": schedule.ID, "user_id": req.UserID, "override_user_id": req.OverrideUserID,
	})

	respondJSONCreated(w, map[string]int{"id": id})
}

// DeleteOverride authorizes against the override's parent schedule.
func (h *OnCallHandler) DeleteOverride(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	overrideID, ok := requireIDParam(w, r, "overrideId")
	if !ok {
		return
	}

	override, err := h.onCallRepo.GetOverrideByID(overrideID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "override")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	schedule, err := h.onCallRepo.GetScheduleByID(override.ScheduleID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !h.canManageTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	if err := h.onCallRepo.DeleteOverride(overrideID); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallOverrideDelete, logger.ResourceOnCallOverride, &overrideID, "", map[string]any{"schedule_id": override.ScheduleID})

	w.WriteHeader(http.StatusNoContent)
}

// GetCurrentOnCall returns a schedule's roster to authorized team members.
func (h *OnCallHandler) GetCurrentOnCall(w http.ResponseWriter, r *http.Request) {
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

	if !h.canViewTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	result, err := h.onCallService.GetCurrentOnCall(scheduleID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, result)
}

// CreateSwapRequest creates a shift swap between members of one team.
func (h *OnCallHandler) CreateSwapRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

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

	if !h.canViewTeamOnCall(w, r, schedule.TeamID) {
		return
	}

	req, ok := decodeJSON[models.OnCallSwapRequestCreate](w, r)
	if !ok {
		return
	}
	// Sanitize timestamps before validation errors can echo them.
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.SwapStart, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.SwapEnd, Policy: sanitize.ShortIdentifier},
	)

	targetIsMember, err := h.teamRepo.IsTeamMember(schedule.TeamID, req.TargetUserID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !targetIsMember {
		// Team admins may not have an explicit membership row.
		targetIsAdmin, adminErr := h.teamRepo.IsTeamAdmin(schedule.TeamID, req.TargetUserID)
		if adminErr != nil {
			respondInternalError(w, r, adminErr)
			return
		}
		if !targetIsAdmin {
			respondValidationError(w, r, "target user is not a member of this team")
			return
		}
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
	sanitizeEscalationPolicy(&req)

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
	h.auditMutation(r, logger.ActionOnCallPolicyCreate, logger.ResourceOnCallPolicy, &id, req.Name, map[string]any{"team_id": teamID})

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
	policy, ok := h.resolvePolicy(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.OnCallEscalationPolicyRequest](w, r)
	if !ok {
		return
	}
	sanitizeEscalationPolicy(&req)

	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, r, "name is required")
		return
	}

	isActive := policy.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	if err := h.onCallRepo.UpdatePolicy(policy.ID, req.Name, req.Description, req.RepeatCount, isActive); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallPolicyUpdate, logger.ResourceOnCallPolicy, &policy.ID, req.Name, map[string]any{"team_id": policy.TeamID})

	respondJSONOK(w, map[string]string{"status": "ok"})
}

// DeletePolicy removes an escalation policy.
func (h *OnCallHandler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	policy, ok := h.resolvePolicy(w, r, "id")
	if !ok {
		return
	}

	if err := h.onCallRepo.DeletePolicy(policy.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallPolicyDelete, logger.ResourceOnCallPolicy, &policy.ID, policy.Name, map[string]any{"team_id": policy.TeamID})

	w.WriteHeader(http.StatusNoContent)
}

// SetRules replaces the escalation rules for a policy.
func (h *OnCallHandler) SetRules(w http.ResponseWriter, r *http.Request) {
	policy, ok := h.resolvePolicy(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.SetEscalationRulesRequest](w, r)
	if !ok {
		return
	}

	if err := h.onCallRepo.SetEscalationRules(policy.ID, req.Rules); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditMutation(r, logger.ActionOnCallPolicySetRules, logger.ResourceOnCallPolicy, &policy.ID, policy.Name, map[string]any{"team_id": policy.TeamID, "rule_count": len(req.Rules)})

	respondJSONOK(w, map[string]string{"status": "ok"})
}

// ListIncidents returns active incidents, optionally filtered by policy.
func (h *OnCallHandler) ListIncidents(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
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

	allTeams, err := h.permissionService.HasGlobalPermissionContext(r.Context(), user.ID, models.PermissionTeamsManage)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var teamIDs []int
	if !allTeams {
		teams, teamErr := h.teamRepo.GetTeamsForUser(user.ID)
		if teamErr != nil {
			respondInternalError(w, r, teamErr)
			return
		}
		teamIDs = make([]int, len(teams))
		for i := range teams {
			teamIDs[i] = teams[i].ID
		}
	}

	workspaceIDs, err := h.permissionService.AccessibleWorkspaceIDs(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	incidents, err := h.onCallRepo.ListIncidents(repository.IncidentFilter{
		PolicyID:     policyID,
		TeamIDs:      teamIDs,
		WorkspaceIDs: workspaceIDs,
		AllTeams:     allTeams,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, incidents)
}

// GetItemIncident returns the incident currently attached to an item. Missing
// incidents return 404 so item-less workspaces do not leak pager state.
func (h *OnCallHandler) GetItemIncident(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	incident, ok := h.itemIncident(w, r)
	if !ok {
		return
	}

	allowed, err := h.canViewIncidentItem(user.ID, incident)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !allowed {
		respondForbidden(w, r)
		return
	}
	respondJSONOK(w, incident)
}

// TriggerIncident declares an incident on an item. The item must have an
// assigned team, and the item's team must have an active escalation policy
// unless one is named explicitly. Gated on item edit permission.
func (h *OnCallHandler) TriggerIncident(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	item, err := h.itemRepo.FindByIDWithDetails(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	allowed, err := h.permissionService.HasWorkspacePermission(user.ID, item.WorkspaceID, models.PermissionItemEdit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !allowed {
		respondForbidden(w, r)
		return
	}

	var req models.IncidentTriggerRequest
	if r.Body != nil && r.ContentLength != 0 {
		decoded, ok := decodeJSON[models.IncidentTriggerRequest](w, r)
		if !ok {
			return
		}
		req = decoded
	}

	incident, err := h.incidentService.Trigger(itemID, req.PolicyID, req.Urgency, "manual")
	if err != nil {
		switch {
		case errors.Is(err, services.ErrIncidentItemTeamRequired), errors.Is(err, services.ErrIncidentPolicyUnknown):
			respondValidationError(w, r, err.Error())
		case errors.Is(err, services.ErrIncidentAlreadyOpen):
			respondConflict(w, r, err.Error())
		default:
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONCreated(w, incident)
}

// AcknowledgeIncident records the ack handshake. Gated on responder-ship, not
// item permission: the 3am responder may have no rights in the item workspace.
func (h *OnCallHandler) AcknowledgeIncident(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	incident, ok := h.itemIncident(w, r)
	if !ok {
		return
	}
	if !h.requireResponderShip(w, r, user, incident) {
		return
	}

	if err := h.incidentService.Acknowledge(incident.ID, user.ID); err != nil {
		if errors.Is(err, services.ErrIncidentResolved) {
			respondConflict(w, r, err.Error())
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, h.reloadItemIncident(incident))
}

// UnacknowledgeIncident returns an acknowledged incident to the triggered state.
func (h *OnCallHandler) UnacknowledgeIncident(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	incident, ok := h.itemIncident(w, r)
	if !ok {
		return
	}
	if !h.requireResponderShip(w, r, user, incident) {
		return
	}

	if err := h.incidentService.Unacknowledge(incident.ID); err != nil {
		if errors.Is(err, services.ErrIncidentResolved) {
			respondConflict(w, r, err.Error())
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, h.reloadItemIncident(incident))
}

// ResolveIncident ends the incident. It does not complete the item.
func (h *OnCallHandler) ResolveIncident(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	incident, ok := h.itemIncident(w, r)
	if !ok {
		return
	}
	if !h.requireResponderShip(w, r, user, incident) {
		return
	}

	if err := h.incidentService.Resolve(incident.ID, user.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, h.reloadItemIncident(incident))
}

// itemIncident loads the incident attached to the {id} item path parameter.
func (h *OnCallHandler) itemIncident(w http.ResponseWriter, r *http.Request) (*models.Incident, bool) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return nil, false
	}
	incident, err := h.onCallRepo.GetIncidentForItem(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Incident")
			return nil, false
		}
		respondInternalError(w, r, err)
		return nil, false
	}
	return incident, true
}

// reloadItemIncident re-reads an incident after a mutation so the response
// carries the updated ack/resolve actors. Write failures only drop the
// enrichment; the status change itself already committed.
func (h *OnCallHandler) reloadItemIncident(incident *models.Incident) *models.Incident {
	updated, err := h.onCallRepo.GetIncidentByID(incident.ID)
	if err != nil {
		return incident
	}
	return updated
}

// canViewIncidentItem permits workspace viewers or responders to read an item's
// incident.
func (h *OnCallHandler) canViewIncidentItem(userID int, incident *models.Incident) (bool, error) {
	allowed, err := h.permissionService.HasWorkspacePermission(userID, incident.WorkspaceID, models.PermissionItemView)
	if err != nil || allowed {
		return allowed, err
	}
	if incident.TeamID == nil {
		return false, nil
	}
	return h.hasTeamOnCallViewAccess(userID, *incident.TeamID)
}

// requireResponderShip enforces that the caller can respond to the incident:
// a member or admin of the incident's team, or a global team manager. It is
// deliberately independent of item workspace permission.
func (h *OnCallHandler) requireResponderShip(w http.ResponseWriter, r *http.Request, user *models.User, incident *models.Incident) bool {
	if incident.TeamID == nil {
		respondForbidden(w, r)
		return false
	}
	allowed, err := h.hasTeamOnCallViewAccess(user.ID, *incident.TeamID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !allowed {
		respondForbidden(w, r)
		return false
	}
	return true
}
