package handlers

import (
	"database/sql"
	"net/http"
	"strconv"


	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
)

// ========================================
// Item Types Handler
// ========================================

type ItemTypeHandler struct {
	db database.Database
}

func NewItemTypeHandler(db database.Database) *ItemTypeHandler {
	return &ItemTypeHandler{db: db}
}

type ItemTypeResponse struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Icon           string `json:"icon,omitempty"`
	Color          string `json:"color,omitempty"`
	HierarchyLevel int    `json:"hierarchy_level"`
	SortOrder      int    `json:"sort_order"`
	IsDefault      bool   `json:"is_default"`
}

func (h *ItemTypeHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, description, icon, color, hierarchy_level, sort_order, is_default
		FROM item_types
		ORDER BY hierarchy_level, sort_order, name
	`)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var types []ItemTypeResponse
	for rows.Next() {
		var t ItemTypeResponse
		var description, icon, color sql.NullString
		rows.Scan(&t.ID, &t.Name, &description, &icon, &color, &t.HierarchyLevel, &t.SortOrder, &t.IsDefault)
		t.Description = nullStringValue(description)
		t.Icon = nullStringValue(icon)
		t.Color = nullStringValue(color)
		types = append(types, t)
	}

	restapi.RespondOK(w, types)
}

func (h *ItemTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid item type ID"))
		return
	}

	var t ItemTypeResponse
	var description, icon, color sql.NullString
	err = h.db.QueryRow(`
		SELECT id, name, description, icon, color, hierarchy_level, sort_order, is_default
		FROM item_types WHERE id = ?
	`, id).Scan(&t.ID, &t.Name, &description, &icon, &color, &t.HierarchyLevel, &t.SortOrder, &t.IsDefault)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	t.Description = nullStringValue(description)
	t.Icon = nullStringValue(icon)
	t.Color = nullStringValue(color)

	restapi.RespondOK(w, t)
}

// ========================================
// Priorities Handler
// ========================================

type PriorityHandler struct {
	db database.Database
}

func NewPriorityHandler(db database.Database) *PriorityHandler {
	return &PriorityHandler{db: db}
}

type PriorityResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
	SortOrder   int    `json:"sort_order"`
	IsDefault   bool   `json:"is_default"`
}

func (h *PriorityHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, description, icon, color, sort_order, is_default
		FROM priorities
		ORDER BY sort_order, name
	`)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var priorities []PriorityResponse
	for rows.Next() {
		var p PriorityResponse
		var description, icon, color sql.NullString
		rows.Scan(&p.ID, &p.Name, &description, &icon, &color, &p.SortOrder, &p.IsDefault)
		p.Description = nullStringValue(description)
		p.Icon = nullStringValue(icon)
		p.Color = nullStringValue(color)
		priorities = append(priorities, p)
	}

	restapi.RespondOK(w, priorities)
}

func (h *PriorityHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid priority ID"))
		return
	}

	var p PriorityResponse
	var description, icon, color sql.NullString
	err = h.db.QueryRow(`
		SELECT id, name, description, icon, color, sort_order, is_default
		FROM priorities WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &description, &icon, &color, &p.SortOrder, &p.IsDefault)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	p.Description = nullStringValue(description)
	p.Icon = nullStringValue(icon)
	p.Color = nullStringValue(color)

	restapi.RespondOK(w, p)
}

// ========================================
// Custom Fields Handler
// ========================================

type CustomFieldHandler struct {
	db database.Database
}

func NewCustomFieldHandler(db database.Database) *CustomFieldHandler {
	return &CustomFieldHandler{db: db}
}

type CustomFieldResponse struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	FieldType    string `json:"field_type"`
	Description  string `json:"description,omitempty"`
	Options      string `json:"options,omitempty"` // JSON string
	Required     bool   `json:"required"`
	DisplayOrder int    `json:"display_order"`
}

func (h *CustomFieldHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, field_type, description, options, required, display_order
		FROM custom_field_definitions
		ORDER BY display_order, name
	`)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var fields []CustomFieldResponse
	for rows.Next() {
		var f CustomFieldResponse
		var description, options sql.NullString
		rows.Scan(&f.ID, &f.Name, &f.FieldType, &description, &options, &f.Required, &f.DisplayOrder)
		f.Description = nullStringValue(description)
		f.Options = nullStringValue(options)
		fields = append(fields, f)
	}

	restapi.RespondOK(w, fields)
}

func (h *CustomFieldHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid custom field ID"))
		return
	}

	var f CustomFieldResponse
	var description, options sql.NullString
	err = h.db.QueryRow(`
		SELECT id, name, field_type, description, options, required, display_order
		FROM custom_field_definitions WHERE id = ?
	`, id).Scan(&f.ID, &f.Name, &f.FieldType, &description, &options, &f.Required, &f.DisplayOrder)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	f.Description = nullStringValue(description)
	f.Options = nullStringValue(options)

	restapi.RespondOK(w, f)
}
