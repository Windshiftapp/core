// Package handlers provides HTTP handlers for the REST API v1 endpoints.
package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/services"
)

// ========================================
// Item Types Handler
// ========================================

type ItemTypeHandler struct {
	BaseHandler
	configSvc *services.ConfigReadService
}

func NewItemTypeHandler(db database.Database, permissionService *services.PermissionService) *ItemTypeHandler {
	return &ItemTypeHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		configSvc:   services.NewConfigReadService(db),
	}
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
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	results, err := h.configSvc.ListItemTypes()
	if err != nil {
		h.RespondInternalError(w, r)
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

	h.RespondOK(w, types)
}

func (h *ItemTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "item type ID")
	if !ok {
		return
	}

	t, err := h.configSvc.GetItemType(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, ItemTypeResponse{
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
	BaseHandler
	configSvc *services.ConfigReadService
}

func NewPriorityHandler(db database.Database, permissionService *services.PermissionService) *PriorityHandler {
	return &PriorityHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		configSvc:   services.NewConfigReadService(db),
	}
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
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	results, err := h.configSvc.ListPriorities()
	if err != nil {
		h.RespondInternalError(w, r)
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

	h.RespondOK(w, priorities)
}

func (h *PriorityHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "priority ID")
	if !ok {
		return
	}

	p, err := h.configSvc.GetPriority(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, PriorityResponse{
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
	BaseHandler
	configSvc *services.ConfigReadService
}

func NewCustomFieldHandler(db database.Database, permissionService *services.PermissionService) *CustomFieldHandler {
	return &CustomFieldHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		configSvc:   services.NewConfigReadService(db),
	}
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
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	results, err := h.configSvc.ListCustomFields()
	if err != nil {
		h.RespondInternalError(w, r)
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

	h.RespondOK(w, fields)
}

func (h *CustomFieldHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "custom field ID")
	if !ok {
		return
	}

	f, err := h.configSvc.GetCustomField(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, CustomFieldResponse{
		ID:           f.ID,
		Name:         f.Name,
		FieldType:    f.FieldType,
		Description:  f.Description,
		Options:      f.Options,
		Required:     f.Required,
		DisplayOrder: f.DisplayOrder,
	})
}
