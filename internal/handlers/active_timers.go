package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"windshift/internal/repository"
	"windshift/internal/services"
)

type ActiveTimerHandler struct {
	repo  *repository.ActiveTimerRepository
	timer *services.TimerService
}

func NewActiveTimerHandler(repo *repository.ActiveTimerRepository, timer *services.TimerService) *ActiveTimerHandler {
	return &ActiveTimerHandler{repo: repo, timer: timer}
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

	timer, err := h.timer.StartTimer(user.ID, req.WorkspaceID, req.ProjectID, req.ItemID, req.Description)
	if err != nil {
		writeTimerError(w, r, err)
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

	res, err := h.timer.StopTimerByID(user.ID, timerID)
	if err != nil {
		writeTimerError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"timer_id":         res.TimerID,
		"duration_seconds": res.DurationSeconds,
		"worklog_created":  res.WorklogCreated,
		"start_time_utc":   res.StartTimeUTC,
		"end_time_utc":     res.EndTimeUTC,
		"description":      res.Description,
		"project_name":     res.ProjectName,
		"item_title":       res.ItemTitle,
		"workspace_name":   res.WorkspaceName,
	})
}

// writeTimerError maps TimerService sentinel errors to HTTP responses.
func writeTimerError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrTimerValidation):
		respondValidationError(w, r, err.Error())
	case errors.Is(err, services.ErrTimerNotFound):
		respondNotFound(w, r, "timer")
	case errors.Is(err, services.ErrTimerForbidden):
		respondForbidden(w, r)
	case errors.Is(err, services.ErrTimerProjectInactive):
		respondValidationError(w, r, "cannot start timer on a project that is not active")
	case errors.Is(err, services.ErrTimerAlreadyRunning):
		respondConflict(w, r, "An active timer is already running. Stop it before starting a new one.")
	default:
		respondInternalError(w, r, err)
	}
}
