package handlers

import (
	"database/sql"
	"net/http"
	"strconv"


	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
)

// StatusHandler handles public API requests for statuses
type StatusHandler struct {
	db database.Database
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(db database.Database) *StatusHandler {
	return &StatusHandler{db: db}
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

	rows, err := h.db.Query(`
		SELECT s.id, s.name, s.description, s.category_id, s.is_default,
		       sc.name as category_name, sc.color as category_color, sc.is_completed
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		ORDER BY sc.id, s.name
	`)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var statuses []StatusResponse
	for rows.Next() {
		var s StatusResponse
		var description sql.NullString
		rows.Scan(&s.ID, &s.Name, &description, &s.CategoryID, &s.IsDefault,
			&s.CategoryName, &s.CategoryColor, &s.IsCompleted)
		if description.Valid {
			s.Description = description.String
		}
		statuses = append(statuses, s)
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

	var s StatusResponse
	var description sql.NullString
	err = h.db.QueryRow(`
		SELECT s.id, s.name, s.description, s.category_id, s.is_default,
		       sc.name as category_name, sc.color as category_color, sc.is_completed
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE s.id = ?
	`, id).Scan(&s.ID, &s.Name, &description, &s.CategoryID, &s.IsDefault,
		&s.CategoryName, &s.CategoryColor, &s.IsCompleted)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	if description.Valid {
		s.Description = description.String
	}

	restapi.RespondOK(w, s)
}

// ListCategories handles GET /rest/api/v1/status-categories
func (h *StatusHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, color, description, is_default, is_completed
		FROM status_categories
		ORDER BY id
	`)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var categories []StatusCategoryResponse
	for rows.Next() {
		var c StatusCategoryResponse
		var description sql.NullString
		rows.Scan(&c.ID, &c.Name, &c.Color, &description, &c.IsDefault, &c.IsCompleted)
		if description.Valid {
			c.Description = description.String
		}
		categories = append(categories, c)
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

	var c StatusCategoryResponse
	var description sql.NullString
	err = h.db.QueryRow(`
		SELECT id, name, color, description, is_default, is_completed
		FROM status_categories WHERE id = ?
	`, id).Scan(&c.ID, &c.Name, &c.Color, &description, &c.IsDefault, &c.IsCompleted)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	if description.Valid {
		c.Description = description.String
	}

	restapi.RespondOK(w, c)
}
