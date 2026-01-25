package handlers

import (
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
)

// StatusHandler handles public API requests for statuses
type StatusHandler struct {
	db            database.Database
	statusService *services.StatusService
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(db database.Database) *StatusHandler {
	return &StatusHandler{
		db:            db,
		statusService: services.NewStatusService(db),
	}
}

// SetStatusService allows injecting a configured status service
func (h *StatusHandler) SetStatusService(ss *services.StatusService) {
	h.statusService = ss
}

// StatusResponse is the public API representation of a Status
type StatusResponse struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	CategoryID    int    `json:"category_id"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	IsDefault     bool   `json:"is_default"`
	IsCompleted   bool   `json:"is_completed"`
}

// StatusCategoryResponse is the public API representation of a StatusCategory
type StatusCategoryResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default"`
	IsCompleted bool   `json:"is_completed"`
}

// List handles GET /rest/api/v1/statuses
func (h *StatusHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	results, err := h.statusService.ListStatuses()
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var statuses []StatusResponse
	for _, s := range results {
		statuses = append(statuses, StatusResponse{
			ID:            s.ID,
			Name:          s.Name,
			Description:   s.Description,
			CategoryID:    s.CategoryID,
			CategoryName:  s.CategoryName,
			CategoryColor: s.CategoryColor,
			IsDefault:     s.IsDefault,
			IsCompleted:   s.IsCompleted,
		})
	}

	if statuses == nil {
		statuses = []StatusResponse{}
	}

	restapi.RespondOK(w, statuses)
}

// Get handles GET /rest/api/v1/statuses/{id}
func (h *StatusHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid status ID"))
		return
	}

	s, err := h.statusService.GetStatus(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondOK(w, StatusResponse{
		ID:            s.ID,
		Name:          s.Name,
		Description:   s.Description,
		CategoryID:    s.CategoryID,
		CategoryName:  s.CategoryName,
		CategoryColor: s.CategoryColor,
		IsDefault:     s.IsDefault,
		IsCompleted:   s.IsCompleted,
	})
}

// ListCategories handles GET /rest/api/v1/status-categories
func (h *StatusHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	results, err := h.statusService.ListCategories()
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var categories []StatusCategoryResponse
	for _, c := range results {
		categories = append(categories, StatusCategoryResponse{
			ID:          c.ID,
			Name:        c.Name,
			Color:       c.Color,
			Description: c.Description,
			IsDefault:   c.IsDefault,
			IsCompleted: c.IsCompleted,
		})
	}

	if categories == nil {
		categories = []StatusCategoryResponse{}
	}

	restapi.RespondOK(w, categories)
}

// GetCategory handles GET /rest/api/v1/status-categories/{id}
func (h *StatusHandler) GetCategory(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid category ID"))
		return
	}

	c, err := h.statusService.GetCategory(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondOK(w, StatusCategoryResponse{
		ID:          c.ID,
		Name:        c.Name,
		Color:       c.Color,
		Description: c.Description,
		IsDefault:   c.IsDefault,
		IsCompleted: c.IsCompleted,
	})
}
