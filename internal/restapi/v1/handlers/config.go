// Package handlers provides HTTP handlers for the REST API v1 endpoints.
package handlers

import (
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
)

// ========================================
// Item Types Handler
// ========================================

type ItemTypeHandler struct {
	configSvc *services.ConfigReadService
}

func NewItemTypeHandler(db database.Database) *ItemTypeHandler {
	return &ItemTypeHandler{configSvc: services.NewConfigReadService(db)}
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

	results, err := h.configSvc.ListItemTypes()
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var types []ItemTypeResponse
	for _, t := range results {
		types = append(types, ItemTypeResponse{
			ID:             t.ID,
			Name:           t.Name,
			Description:    t.Description,
			Icon:           t.Icon,
			Color:          t.Color,
			HierarchyLevel: t.HierarchyLevel,
			SortOrder:      t.SortOrder,
			IsDefault:      t.IsDefault,
		})
	}

	if types == nil {
		types = []ItemTypeResponse{}
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

	t, err := h.configSvc.GetItemType(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondOK(w, ItemTypeResponse{
		ID:             t.ID,
		Name:           t.Name,
		Description:    t.Description,
		Icon:           t.Icon,
		Color:          t.Color,
		HierarchyLevel: t.HierarchyLevel,
		SortOrder:      t.SortOrder,
		IsDefault:      t.IsDefault,
	})
}

// ========================================
// Priorities Handler
// ========================================

type PriorityHandler struct {
	configSvc *services.ConfigReadService
}

func NewPriorityHandler(db database.Database) *PriorityHandler {
	return &PriorityHandler{configSvc: services.NewConfigReadService(db)}
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

	results, err := h.configSvc.ListPriorities()
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var priorities []PriorityResponse
	for _, p := range results {
		priorities = append(priorities, PriorityResponse{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			Icon:        p.Icon,
			Color:       p.Color,
			SortOrder:   p.SortOrder,
			IsDefault:   p.IsDefault,
		})
	}

	if priorities == nil {
		priorities = []PriorityResponse{}
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

	p, err := h.configSvc.GetPriority(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondOK(w, PriorityResponse{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Icon:        p.Icon,
		Color:       p.Color,
		SortOrder:   p.SortOrder,
		IsDefault:   p.IsDefault,
	})
}

// ========================================
// Custom Fields Handler
// ========================================

type CustomFieldHandler struct {
	configSvc *services.ConfigReadService
}

func NewCustomFieldHandler(db database.Database) *CustomFieldHandler {
	return &CustomFieldHandler{configSvc: services.NewConfigReadService(db)}
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

	results, err := h.configSvc.ListCustomFields()
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var fields []CustomFieldResponse
	for _, f := range results {
		fields = append(fields, CustomFieldResponse{
			ID:           f.ID,
			Name:         f.Name,
			FieldType:    f.FieldType,
			Description:  f.Description,
			Options:      f.Options,
			Required:     f.Required,
			DisplayOrder: f.DisplayOrder,
		})
	}

	if fields == nil {
		fields = []CustomFieldResponse{}
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

	f, err := h.configSvc.GetCustomField(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	restapi.RespondOK(w, CustomFieldResponse{
		ID:           f.ID,
		Name:         f.Name,
		FieldType:    f.FieldType,
		Description:  f.Description,
		Options:      f.Options,
		Required:     f.Required,
		DisplayOrder: f.DisplayOrder,
	})
}
