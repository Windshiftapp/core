package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// TimeWorklogHandler provides the v1 bearer-token REST surface for time
// tracking worklogs. Worklogs are user-scoped: listing returns the
// authenticated user's entries; create/update/delete target their own.
type TimeWorklogHandler struct {
	BaseHandler
	timePerm    *services.TimePermissionService
	application *services.TimeWorklogService
}

// NewTimeWorklogHandler wires the handler with the shared permission pipeline.
func NewTimeWorklogHandler(base BaseHandler, timePerm *services.TimePermissionService) *TimeWorklogHandler {
	return &TimeWorklogHandler{
		BaseHandler: base,
		timePerm:    timePerm,
		application: services.NewTimeWorklogService(base.DB),
	}
}

type worklogResponse struct {
	ID                  int      `json:"id"`
	ProjectID           int      `json:"project_id"`
	CustomerID          int      `json:"customer_id"`
	ItemID              *int     `json:"item_id,omitempty"`
	Description         string   `json:"description"`
	Date                int64    `json:"date"`
	StartTime           int64    `json:"start_time"`
	EndTime             int64    `json:"end_time"`
	DurationMinutes     int      `json:"duration_minutes"`
	CreatedAt           int64    `json:"created_at"`
	UpdatedAt           int64    `json:"updated_at"`
	CustomerName        string   `json:"customer_name,omitempty"`
	ProjectName         string   `json:"project_name,omitempty"`
	ItemTitle           string   `json:"item_title,omitempty"`
	WorkspaceID         *int     `json:"workspace_id,omitempty"`
	WorkspaceKey        string   `json:"workspace_key,omitempty"`
	WorkspaceItemNumber int      `json:"workspace_item_number,omitempty"`
	ProjectMaxHours     *float64 `json:"project_max_hours,omitempty"`
	ProjectTotalHours   *float64 `json:"project_total_hours,omitempty"`
}

func mapWorklogToResponse(wl models.Worklog) worklogResponse {
	return worklogResponse{
		ID:                  wl.ID,
		ProjectID:           wl.ProjectID,
		CustomerID:          wl.CustomerID,
		ItemID:              wl.ItemID,
		Description:         wl.Description,
		Date:                wl.Date,
		StartTime:           wl.StartTime,
		EndTime:             wl.EndTime,
		DurationMinutes:     wl.DurationMins,
		CreatedAt:           wl.CreatedAt,
		UpdatedAt:           wl.UpdatedAt,
		CustomerName:        wl.CustomerName,
		ProjectName:         wl.ProjectName,
		ItemTitle:           wl.ItemTitle,
		WorkspaceID:         wl.WorkspaceID,
		WorkspaceKey:        wl.WorkspaceKey,
		WorkspaceItemNumber: wl.WorkspaceItemNumber,
		ProjectMaxHours:     wl.ProjectMaxHours,
		ProjectTotalHours:   wl.ProjectTotalHours,
	}
}

// ListMine handles GET /rest/api/v1/time/worklogs
//
// @Summary      List my worklogs
// @Description  Returns the authenticated user's worklogs, newest first, with optional date-range and project filters.
// @Tags         time-tracking
// @Produce      json
// @Security     BearerAuth
// @Param        date_from   query     string  false  "Inclusive start date (YYYY-MM-DD)"
// @Param        date_to     query     string  false  "Inclusive end date (YYYY-MM-DD)"
// @Param        timezone    query     string  false  "IANA timezone used to interpret date_from/date_to (defaults to the profile timezone)"
// @Param        project_id  query     int     false  "Filter by time project ID"
// @Param        page        query     int     false  "Page (1-indexed)"
// @Param        limit       query     int     false  "Page size"
// @Success      200  {object}  handlers.PaginatedResponse{data=[]handlers.worklogResponse}
// @Failure      400  {object}  restapi.ErrorResponse
// @Failure      401  {object}  restapi.ErrorResponse
// @Router       /time/worklogs [get]
func (h *TimeWorklogHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)
	filter := repository.WorklogListFilter{
		UserID: user.ID,
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	}

	reportLocation, locationErr := h.resolveReportTimezone(r, user.Timezone)
	if locationErr != nil {
		h.RespondError(w, r, locationErr)
		return
	}

	if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
		start, _, err := services.CivilDateRangeUTC(dateFrom, dateFrom, reportLocation)
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "invalid date_from format, use YYYY-MM-DD"))
			return
		}
		from := start.Unix()
		filter.DateFromUnix = &from
	}
	if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
		_, endExclusive, err := services.CivilDateRangeUTC(dateTo, dateTo, reportLocation)
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "invalid date_to format, use YYYY-MM-DD"))
			return
		}
		to := endExclusive.Unix()
		filter.DateToExclusiveUnix = &to
	}
	if projectIDStr := r.URL.Query().Get("project_id"); projectIDStr != "" {
		pid, err := strconv.Atoi(projectIDStr)
		if err != nil || pid <= 0 {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "invalid project_id"))
			return
		}
		filter.ProjectID = &pid
	}

	worklogs, total, err := repository.NewTimeWorklogRepository(h.DB).ListForUser(filter)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	worklogs = services.RedactInaccessibleWorklogItems(worklogs, func(workspaceID int) (bool, error) {
		return h.PermissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemView)
	})

	out := make([]worklogResponse, 0, len(worklogs))
	for _, wl := range worklogs {
		out = append(out, mapWorklogToResponse(wl))
	}
	h.RespondPaginated(w, out, pagination, total)
}

type createWorklogRequest struct {
	ProjectID       int    `json:"project_id"`
	Description     string `json:"description"`
	Date            string `json:"date"`
	Duration        string `json:"duration,omitempty"`
	DurationMinutes int    `json:"duration_minutes,omitempty"`
	StartTime       string `json:"start_time,omitempty"`
	EndTime         string `json:"end_time,omitempty"`
	ItemID          *int   `json:"item_id,omitempty"`
	ItemKey         string `json:"item_key,omitempty"`
	Timezone        string `json:"timezone,omitempty"`
}

// Create handles POST /rest/api/v1/time/worklogs
//
// @Summary      Log time
// @Description  Creates a worklog for the authenticated user. Provide either `duration`/`duration_minutes` or a `start_time`+`end_time` pair; `item_id`/`item_key` optionally link the entry to a work item.
// @Tags         time-tracking
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handlers.createWorklogRequest  true  "Worklog to create"
// @Success      201   {object}  map[string]any
// @Failure      400   {object}  restapi.ErrorResponse
// @Failure      401   {object}  restapi.ErrorResponse
// @Failure      403   {object}  restapi.ErrorResponse  "No permission to book time on this project"
// @Router       /time/worklogs [post]
func (h *TimeWorklogHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	var req createWorklogRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	if req.ProjectID == 0 || req.Description == "" || req.Date == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "project_id, description, and date are required"))
		return
	}

	canBook, err := h.timePerm.CanBookTimeOnProject(user.ID, req.ProjectID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	if !canBook {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "no permission to book time on this project"))
		return
	}

	// Resolve optional item link
	var itemID *int
	if req.ItemKey != "" || (req.ItemID != nil && *req.ItemID > 0) {
		rawID := 0
		if req.ItemID != nil {
			rawID = *req.ItemID
		}
		id, err := services.ResolveAccessibleWorklogItem(h.DB, rawID, req.ItemKey, func(workspaceID int) (bool, error) {
			return h.PermissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemView)
		})
		if err != nil {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, "NOT_FOUND", "item not found"))
			return
		}
		itemID = &id
	}

	result, err := h.application.Create(user.ID, services.WorklogMutationInput{
		ProjectID: req.ProjectID, ItemID: itemID, Description: req.Description, Date: req.Date,
		Duration: req.Duration, DurationMinutes: req.DurationMinutes,
		StartTime: req.StartTime, EndTime: req.EndTime, Timezone: req.Timezone,
	})
	if err != nil {
		h.respondMutationError(w, r, err)
		return
	}

	h.RespondCreated(w, map[string]any{
		"id":               result.Worklog.ID,
		"project_id":       req.ProjectID,
		"project_name":     result.Worklog.ProjectName,
		"date":             req.Date,
		"duration_minutes": result.DurationMinutes,
		"description":      result.Worklog.Description,
		"timezone":         result.Timezone,
		"start_time_local": time.Unix(result.StartTimeUnix, 0).In(result.Location).Format("15:04"),
		"end_time_local":   time.Unix(result.EndTimeUnix, 0).In(result.Location).Format("15:04"),
		"start_at":         time.Unix(result.StartTimeUnix, 0).UTC().Format(time.RFC3339),
		"end_at":           time.Unix(result.EndTimeUnix, 0).UTC().Format(time.RFC3339),
	})
}

type updateWorklogRequest struct {
	Description string `json:"description"`
}

// Update handles PUT /rest/api/v1/time/worklogs/{id}
//
// @Summary      Update a worklog description
// @Description  Changes the description of an existing worklog. Only the owning user may update; other users get 403.
// @Tags         time-tracking
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                            true  "Worklog ID"
// @Param        body  body      handlers.updateWorklogRequest  true  "New description"
// @Success      200   {object}  map[string]any
// @Failure      400   {object}  restapi.ErrorResponse
// @Failure      401   {object}  restapi.ErrorResponse
// @Failure      403   {object}  restapi.ErrorResponse  "Caller does not own the worklog"
// @Failure      404   {object}  restapi.ErrorResponse
// @Router       /time/worklogs/{id} [put]
func (h *TimeWorklogHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	worklogID, ok := h.ParsePathID(w, r, "id", "worklog ID")
	if !ok {
		return
	}

	var req updateWorklogRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	worklogRepo := repository.NewTimeWorklogRepository(h.DB)
	ownerID, err := worklogRepo.GetOwnerID(worklogID)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}
	if ownerID != user.ID {
		h.RespondError(w, r, restapi.ErrForbidden)
		return
	}

	if _, err := h.application.UpdateDescription(worklogID, req.Description); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, map[string]any{"id": worklogID, "updated": true})
}

// Delete handles DELETE /rest/api/v1/time/worklogs/{id}
//
// @Summary      Delete a worklog
// @Description  Removes a worklog. Only the owning user may delete; other users get 403.
// @Tags         time-tracking
// @Security     BearerAuth
// @Param        id   path  int  true  "Worklog ID"
// @Success      204  "Deleted"
// @Failure      400  {object}  restapi.ErrorResponse
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      403  {object}  restapi.ErrorResponse  "Caller does not own the worklog"
// @Failure      404  {object}  restapi.ErrorResponse
// @Router       /time/worklogs/{id} [delete]
func (h *TimeWorklogHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	worklogID, ok := h.ParsePathID(w, r, "id", "worklog ID")
	if !ok {
		return
	}

	worklogRepo := repository.NewTimeWorklogRepository(h.DB)
	ownerID, err := worklogRepo.GetOwnerID(worklogID)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}
	if ownerID != user.ID {
		h.RespondError(w, r, restapi.ErrForbidden)
		return
	}

	if err := h.application.Delete(worklogID); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}

func (h *TimeWorklogHandler) resolveReportTimezone(r *http.Request, profileTimezone string) (*time.Location, *restapi.APIError) {
	raw := r.URL.Query().Get("timezone")
	if raw == "" {
		raw = profileTimezone
	}
	_, location, err := services.ResolveTimezone(raw)
	if err != nil {
		return nil, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, err.Error())
	}
	return location, nil
}

func (h *TimeWorklogHandler) respondMutationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrWorklogProjectNotFound):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "project not found"))
	case errors.Is(err, services.ErrWorklogProjectInactive), errors.Is(err, services.ErrWorklogCustomerMissing), errors.Is(err, services.ErrWorklogInvalidInput):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, err.Error()))
	default:
		h.RespondInternalError(w, r)
	}
}
