package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"windshift/internal/repository"
	"windshift/internal/services"
)

type ActiveTimerHandler struct {
	repo                  *repository.ActiveTimerRepository
	timePermissionService *services.TimePermissionService
}

func NewActiveTimerHandler(repo *repository.ActiveTimerRepository, timePermissionService *services.TimePermissionService) *ActiveTimerHandler {
	return &ActiveTimerHandler{repo: repo, timePermissionService: timePermissionService}
}

// StartTimer starts a new active timer
func (h *ActiveTimerHandler) StartTimer(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var req struct {
		WorkspaceID int    `json:"workspace_id"`
		ItemID      *int   `json:"item_id,omitempty"`
		ProjectID   int    `json:"project_id"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.WorkspaceID == 0 {
		respondValidationError(w, r, "workspace_id is required")
		return
	}
	if req.ProjectID == 0 {
		respondValidationError(w, r, "project_id is required")
		return
	}
	if req.Description == "" {
		respondValidationError(w, r, "description is required")
		return
	}

	if h.timePermissionService != nil {
		canBook, err := h.timePermissionService.CanBookTimeOnProject(user.ID, req.ProjectID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canBook {
			respondForbidden(w, r)
			return
		}
	}

	projectStatus, err := h.repo.GetProjectStatus(req.ProjectID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "project")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if projectStatus != "Active" {
		respondValidationError(w, r, "cannot start timer on a project that is not active")
		return
	}

	hasActive, err := h.repo.HasActiveTimerForUser(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if hasActive {
		respondConflict(w, r, "An active timer is already running. Stop it before starting a new one.")
		return
	}

	now := time.Now().UTC().Unix()
	id, err := h.repo.CreateTimer(repository.CreateTimerInput{
		WorkspaceID:  req.WorkspaceID,
		ItemID:       req.ItemID,
		ProjectID:    req.ProjectID,
		UserID:       user.ID,
		Description:  req.Description,
		StartTimeUTC: now,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	timer, err := h.repo.GetTimerByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, timer)
}

// GetActiveTimer gets the currently active timer for the authenticated user
func (h *ActiveTimerHandler) GetActiveTimer(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	timer, err := h.repo.GetTimerForUser(user.ID)
	if errors.Is(err, repository.ErrNotFound) {
		respondJSONOK(w, nil)
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, timer)
}

// StopTimer stops the active timer and creates a worklog entry
func (h *ActiveTimerHandler) StopTimer(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	timerID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	timer, err := h.repo.GetTimerByID(timerID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "timer")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if timer.UserID != user.ID {
		respondForbidden(w, r)
		return
	}

	endTimeUTC := time.Now().UTC().Unix()
	durationSeconds := endTimeUTC - timer.StartTimeUTC

	customerID, err := h.repo.GetProjectCustomerID(timer.ProjectID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	startTime := time.Unix(timer.StartTimeUTC, 0).UTC()
	dateInt := int(startTime.Truncate(24 * time.Hour).Unix())
	durationMinutes := int(durationSeconds / 60)
	nowUnix := time.Now().UTC().Unix()

	if err := h.repo.CreateWorklog(repository.CreateWorklogInput{
		ProjectID:       timer.ProjectID,
		CustomerID:      customerID,
		UserID:          user.ID,
		ItemID:          timer.ItemID,
		Description:     timer.Description,
		DateUnix:        dateInt,
		StartTimeUnix:   int(timer.StartTimeUTC),
		EndTimeUnix:     int(endTimeUTC),
		DurationMinutes: durationMinutes,
		NowUnix:         nowUnix,
	}); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.repo.DeleteTimer(timerID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	safeString := func(s *string) string {
		if s != nil {
			return *s
		}
		return ""
	}

	respondJSONOK(w, map[string]interface{}{
		"timer_id":         timerID,
		"duration_seconds": durationSeconds,
		"worklog_created":  true,
		"start_time_utc":   timer.StartTimeUTC,
		"end_time_utc":     endTimeUTC,
		"description":      timer.Description,
		"project_name":     safeString(timer.ProjectName),
		"item_title":       safeString(timer.ItemTitle),
		"workspace_name":   safeString(timer.WorkspaceName),
	})
}
