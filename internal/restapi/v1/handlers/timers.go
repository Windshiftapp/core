package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// ActiveTimerHandler provides the v1 bearer-token REST surface for
// active timers (start, get, stop). It uses TimerService which owns
// the full start/stop lifecycle including workspace/item access
// validation and worklog creation.
type ActiveTimerHandler struct {
	BaseHandler
	repo  *repository.ActiveTimerRepository
	timer *services.TimerService
}

// NewActiveTimerHandler wires the handler with the shared timer pipeline.
func NewActiveTimerHandler(base BaseHandler, repo *repository.ActiveTimerRepository, timer *services.TimerService) *ActiveTimerHandler {
	return &ActiveTimerHandler{
		BaseHandler: base,
		repo:        repo,
		timer:       timer,
	}
}

type startTimerRequest struct {
	WorkspaceID int    `json:"workspace_id"`
	ProjectID   int    `json:"project_id"`
	ItemID      *int   `json:"item_id,omitempty"`
	Description string `json:"description"`
}

// StartTimer handles POST /rest/api/v1/timer/start
//
// @Summary      Start a timer
// @Description  Starts the authenticated user's active timer. A user has at most one running timer; starting while one runs is rejected.
// @Tags         time-tracking
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handlers.startTimerRequest  true  "Timer to start"
// @Success      201   {object}  map[string]any
// @Failure      400   {object}  restapi.ErrorResponse
// @Failure      401   {object}  restapi.ErrorResponse
// @Failure      403   {object}  restapi.ErrorResponse
// @Failure      409   {object}  restapi.ErrorResponse  "A timer is already running"
// @Router       /timer/start [post]
func (h *ActiveTimerHandler) StartTimer(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	var req startTimerRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	sanitize.Apply(&req.Description, sanitize.RichText)

	timer, err := h.timer.StartTimerLegacy(user.ID, req.WorkspaceID, req.ProjectID, req.ItemID, req.Description)
	if err != nil {
		writeTimerV1Error(w, r, err)
		return
	}

	h.RespondCreated(w, map[string]any{
		"id":             timer.ID,
		"description":    timer.Description,
		"start_time_utc": timer.StartTimeUTC,
		"started":        true,
	})
}

// GetActiveTimer handles GET /rest/api/v1/timer/active
//
// @Summary      Get the active timer
// @Description  Returns the authenticated user's currently running timer, or a null body when no timer is running.
// @Tags         time-tracking
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  models.ActiveTimer
// @Failure      401  {object}  restapi.ErrorResponse
// @Router       /timer/active [get]
func (h *ActiveTimerHandler) GetActiveTimer(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	timer, err := h.timer.GetActiveForUser(user.ID)
	if errors.Is(err, repository.ErrNotFound) {
		h.RespondOK(w, nil)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, timer)
}

// StopTimer handles DELETE /rest/api/v1/timer/stop
//
// Stops the active timer by user (takes no timer ID — stops whichever timer
// the user currently has running). This matches the aitools stop_timer
// behavior where agents don't provide a timer ID.
//
// @Summary      Stop the active timer
// @Description  Stops the authenticated user's running timer and creates the corresponding worklog. Takes no timer ID — a user has at most one active timer.
// @Tags         time-tracking
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      404  {object}  restapi.ErrorResponse  "No timer is running"
// @Router       /timer/stop [delete]
func (h *ActiveTimerHandler) StopTimer(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	res, err := h.timer.StopActiveForUser(user.ID)
	if err != nil {
		writeTimerV1Error(w, r, err)
		return
	}

	h.RespondOK(w, map[string]any{
		"stopped":          true,
		"timer_id":         res.TimerID,
		"description":      res.Description,
		"duration_seconds": res.DurationSeconds,
		"duration_minutes": res.DurationMinutes,
		"worklog_created":  res.Booked,
	})
}

// writeTimerV1Error maps TimerService sentinel errors to HTTP responses.
func writeTimerV1Error(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrTimerValidation):
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, err.Error()))
	case errors.Is(err, services.ErrTimerNotFound):
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, "NOT_FOUND", err.Error()))
	case errors.Is(err, services.ErrTimerForbidden):
		restapi.RespondError(w, r, restapi.ErrForbidden)
	case errors.Is(err, services.ErrTimerProjectInactive):
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "cannot start timer on a project that is not active"))
	case errors.Is(err, services.ErrTimerAlreadyRunning):
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, "CONFLICT", "An active timer is already running. Stop it before starting a new one."))
	default:
		restapi.RespondError(w, r, restapi.ErrInternalError)
	}
}
