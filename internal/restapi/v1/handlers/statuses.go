package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/services"
)

// StatusHandler handles public API requests for statuses
type StatusHandler struct {
	BaseHandler
	statusService *services.StatusService
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(db database.Database, permissionService *services.PermissionService) *StatusHandler {
	return &StatusHandler{
		BaseHandler:   NewBaseHandler(db, permissionService),
		statusService: services.NewStatusService(db),
	}
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
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	results, err := h.statusService.ListStatuses()
	if err != nil {
		h.RespondInternalError(w, r)
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

	h.RespondOK(w, statuses)
}

// Get handles GET /rest/api/v1/statuses/{id}
func (h *StatusHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "status ID")
	if !ok {
		return
	}

	s, err := h.statusService.GetStatus(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, StatusResponse{
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
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	results, err := h.statusService.ListCategories()
	if err != nil {
		h.RespondInternalError(w, r)
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

	h.RespondOK(w, categories)
}

// GetCategory handles GET /rest/api/v1/status-categories/{id}
func (h *StatusHandler) GetCategory(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "category ID")
	if !ok {
		return
	}

	c, err := h.statusService.GetCategory(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, StatusCategoryResponse{
		ID:          c.ID,
		Name:        c.Name,
		Color:       c.Color,
		Description: c.Description,
		IsDefault:   c.IsDefault,
		IsCompleted: c.IsCompleted,
	})
}
