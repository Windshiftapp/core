package handlers

import (
	"net/http"
	"strings"

	"windshift/internal/repository"
	"windshift/internal/services"
)

// TimeProjectHandler provides the v1 bearer-token REST surface for time
// tracking projects. It is deployed alongside the cookie-auth time-projects
// handler (internal/handlers/time_projects.go) and uses the same
// TimePermissionService for per-project membership enforcement.
type TimeProjectHandler struct {
	BaseHandler
	timePerm *services.TimePermissionService
}

// NewTimeProjectHandler wires the handler with the shared permission pipeline.
func NewTimeProjectHandler(base BaseHandler, timePerm *services.TimePermissionService) *TimeProjectHandler {
	return &TimeProjectHandler{
		BaseHandler: base,
		timePerm:    timePerm,
	}
}

type timeProjectResponse struct {
	ID                 int            `json:"id"`
	CustomerID         *int           `json:"customer_id,omitempty"`
	CategoryID         *int           `json:"category_id,omitempty"`
	Name               string         `json:"name"`
	Description        string         `json:"description"`
	Status             string         `json:"status"`
	Color              string         `json:"color,omitempty"`
	HourlyRate         float64        `json:"hourly_rate"`
	Settings           map[string]any `json:"settings,omitempty"`
	CustomerName       string         `json:"customer_name,omitempty"`
	CustomerBuiltinKey string         `json:"customer_builtin_key,omitempty"`
	CategoryName       string         `json:"category_name,omitempty"`
	CategoryColor      string         `json:"category_color,omitempty"`
	TotalHours         *float64       `json:"total_hours,omitempty"`
}

func mapTimeProjectToResponse(p repository.TimeProjectDetail) timeProjectResponse {
	return timeProjectResponse{
		ID:                 p.ID,
		CustomerID:         p.CustomerID,
		CategoryID:         p.CategoryID,
		Name:               p.Name,
		Description:        p.Description,
		Status:             p.Status,
		Color:              p.Color,
		HourlyRate:         p.HourlyRate,
		Settings:           p.Settings,
		CustomerName:       p.CustomerName,
		CustomerBuiltinKey: p.CustomerBuiltinKey,
		CategoryName:       p.CategoryName,
		CategoryColor:      p.CategoryColor,
		TotalHours:         p.TotalHours,
	}
}

// List handles GET /rest/api/v1/time/projects
//
// @Summary      List time projects
// @Description  Returns the time projects the authenticated user may book time on, optionally filtered by status.
// @Tags         time-tracking
// @Produce      json
// @Security     BearerAuth
// @Param        status  query     string  false  "Filter by project status (e.g. Active)"
// @Success      200     {array}   handlers.timeProjectResponse
// @Failure      401     {object}  handlers.ErrorResponse
// @Failure      500     {object}  handlers.ErrorResponse
// @Router       /time/projects [get]
func (h *TimeProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	accessibleIDs, err := h.timePerm.GetAccessibleProjects(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	if accessibleIDs != nil && len(accessibleIDs) == 0 {
		h.RespondOK(w, []timeProjectResponse{})
		return
	}

	projects, err := repository.NewTimeProjectRepository(h.DB).ListDetails(accessibleIDs, statusFilter)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	out := make([]timeProjectResponse, 0, len(projects))
	for _, p := range projects {
		out = append(out, mapTimeProjectToResponse(p))
	}
	h.RespondOK(w, out)
}

// Get handles GET /rest/api/v1/time/projects/{id}
//
// @Summary      Get a time project
// @Description  Returns a single time project. Responds 404 (not 403) when the project exists but the caller cannot view it.
// @Tags         time-tracking
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Time project ID"
// @Success      200  {object}  handlers.timeProjectResponse
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse
// @Router       /time/projects/{id} [get]
func (h *TimeProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	projectID, ok := h.ParsePathID(w, r, "id", "time project ID")
	if !ok {
		return
	}

	canView, err := h.timePerm.CanViewProject(user.ID, projectID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	if !canView {
		h.RespondNotFound(w, r)
		return
	}

	project, err := repository.NewTimeProjectRepository(h.DB).GetDetail(projectID)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, mapTimeProjectToResponse(*project))
}
