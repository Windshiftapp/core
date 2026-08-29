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
	BuiltinKey     string `json:"builtin_key,omitempty"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Icon           string `json:"icon,omitempty"`
	Color          string `json:"color,omitempty"`
	HierarchyLevel int    `json:"hierarchy_level"`
	SortOrder      int    `json:"sort_order"`
	IsDefault      bool   `json:"is_default"`
}

// List handles GET /rest/api/v1/item-types
//
// @Summary      List item types
// @Tags         item-types
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handlers.ItemTypeResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the item-types:read scope"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /item-types [get]
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
			BuiltinKey:     t.BuiltinKey,
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

// Get handles GET /rest/api/v1/item-types/{id}
//
// @Summary      Get an item type by ID
// @Tags         item-types
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Item type ID"
// @Success      200  {object}  handlers.ItemTypeResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid item type ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the item-types:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Item type not found"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /item-types/{id} [get]
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
		BuiltinKey:     t.BuiltinKey,
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
	BuiltinKey  string `json:"builtin_key,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
	SortOrder   int    `json:"sort_order"`
	IsDefault   bool   `json:"is_default"`
}

// List handles GET /rest/api/v1/priorities
//
// @Summary      List priorities
// @Tags         priorities
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handlers.PriorityResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the priorities:read scope"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /priorities [get]
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
			BuiltinKey:  p.BuiltinKey,
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

// Get handles GET /rest/api/v1/priorities/{id}
//
// @Summary      Get a priority by ID
// @Tags         priorities
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Priority ID"
// @Success      200  {object}  handlers.PriorityResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid priority ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the priorities:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Priority not found"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /priorities/{id} [get]
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
		BuiltinKey:  p.BuiltinKey,
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

// List handles GET /rest/api/v1/custom-fields
//
// @Summary      List custom fields
// @Tags         custom-fields
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handlers.CustomFieldResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the custom-fields:read scope"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /custom-fields [get]
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

// Get handles GET /rest/api/v1/custom-fields/{id}
//
// @Summary      Get a custom field by ID
// @Tags         custom-fields
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Custom field ID"
// @Success      200  {object}  handlers.CustomFieldResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid custom field ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the custom-fields:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Custom field not found"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /custom-fields/{id} [get]
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
