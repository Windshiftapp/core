package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// AdminGroupHandler handles admin group management in REST API v1.
type AdminGroupHandler struct {
	BaseHandler
	application *services.GroupApplicationService
}

// NewAdminGroupHandler creates a new admin group handler.
func NewAdminGroupHandler(db database.Database, permissionService *services.PermissionService, invalidators ...*services.AuthorizationCacheInvalidator) *AdminGroupHandler {
	cacheInvalidator := services.NewAuthorizationCacheInvalidator(permissionService, nil)
	if len(invalidators) > 0 && invalidators[0] != nil {
		cacheInvalidator = invalidators[0]
	}
	return &AdminGroupHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		application: services.NewGroupApplicationService(repository.NewGroupRepository(db), cacheInvalidator),
	}
}

// AdminGroupResponse is the admin representation of a group.
//
// Warnings carries user-facing strings the frontend toast machinery
// surfaces at info severity — e.g. "Group name had HTML formatting
// removed." Stamped by the handler from sanitize.ApplyAllWithWarnings
// when input was modified at decode time. omitempty so the field
// disappears entirely when there's nothing to report.
type AdminGroupResponse struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	MemberCount int      `json:"member_count"`
	CreatedAt   string   `json:"created_at"`
	Warnings    []string `json:"warnings,omitempty"`
}

// AdminGroupCreateRequest is the request body for creating a group.
type AdminGroupCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AdminGroupUpdateRequest is the request body for updating a group.
type AdminGroupUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// List handles GET /rest/api/v1/admin/groups
//
// @Summary      List groups (admin)
// @Description  System-admin only.
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        page   query     int  false  "Page number (1-based)"
// @Param        limit  query     int  false  "Items per page (max 100)"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]handlers.AdminGroupResponse}
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:groups:read scope"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /admin/groups [get]
func (h *AdminGroupHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	records, total, err := h.application.ListPage(pagination.Limit, pagination.Offset)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	groups := make([]AdminGroupResponse, 0, len(records))
	for _, record := range records {
		groups = append(groups, AdminGroupResponse{
			ID:          record.ID,
			Name:        record.Name,
			Description: record.Description,
			MemberCount: record.MemberCount,
			CreatedAt:   record.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	h.RespondPaginated(w, groups, pagination, total)
}

// Create handles POST /rest/api/v1/admin/groups
//
// @Summary      Create a group (admin)
// @Description  System-admin only.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handlers.AdminGroupCreateRequest  true  "Group to create"
// @Success      201   {object}  handlers.AdminGroupResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or missing name"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:groups:write scope"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /admin/groups [post]
func (h *AdminGroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	var req AdminGroupCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	result, err := h.application.Create(req.Name, req.Description, user.ID)
	if errors.Is(err, services.ErrGroupNameRequired) {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "Group name is required"))
		return
	}
	if errors.Is(err, services.ErrGroupDuplicate) {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, "Group name already exists"))
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	id := result.Group.ID
	h.Auditor.Log(r, user, logger.ActionGroupCreate, logger.ResourceGroup, &id, result.Group.Name)
	h.RespondCreated(w, AdminGroupResponse{
		ID:          id,
		Name:        result.Group.Name,
		Description: result.Group.Description,
		MemberCount: 0,
		CreatedAt:   result.Group.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Warnings:    result.Warnings,
	})
}

// Update handles PUT /rest/api/v1/admin/groups/{id}
//
// @Summary      Update a group (admin)
// @Description  System-admin only.
// @Tags         admin
// @Accept       json
// @Security     BearerAuth
// @Param        id    path  int                               true  "Group ID"
// @Param        body  body  handlers.AdminGroupUpdateRequest  true  "Fields to update"
// @Success      204   "Group updated"
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid group ID, request body, or no fields to update"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:groups:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Group not found"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /admin/groups/{id} [put]
func (h *AdminGroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "group ID")
	if !ok {
		return
	}

	var req AdminGroupUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	if req.Name != nil && *req.Name == "" {
		req.Name = nil
	}
	if req.Description != nil && *req.Description == "" {
		req.Description = nil
	}
	if req.Name == nil && req.Description == nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "No fields to update"))
		return
	}

	result, err := h.application.Update(id, services.GroupUpdateInput{Name: req.Name, Description: req.Description})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.RespondNotFound(w, r)
			return
		}
		if errors.Is(err, services.ErrGroupManaged) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, restapi.ErrCodeForbidden, "Managed groups cannot be changed"))
			return
		}
		if errors.Is(err, services.ErrGroupDuplicate) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, "Group name already exists"))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	h.Auditor.Log(r, user, logger.ActionGroupUpdate, logger.ResourceGroup, &id, result.Group.Name)
	h.RespondNoContent(w)
}

// Delete handles DELETE /rest/api/v1/admin/groups/{id}
//
// @Summary      Delete a group (admin)
// @Description  System-admin only. Cascades through group_members.
// @Tags         admin
// @Security     BearerAuth
// @Param        id   path  int  true  "Group ID"
// @Success      204  "Group deleted"
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid group ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:groups:write scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Group not found"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /admin/groups/{id} [delete]
func (h *AdminGroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "group ID")
	if !ok {
		return
	}
	snapshot, err := h.application.Delete(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.RespondNotFound(w, r)
			return
		}
		if errors.Is(err, services.ErrGroupSystem) || errors.Is(err, services.ErrGroupManaged) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, restapi.ErrCodeForbidden, "Protected groups cannot be deleted"))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	h.Auditor.Log(r, user, logger.ActionGroupDelete, logger.ResourceGroup, &id, snapshot.Name)
	h.RespondNoContent(w)
}
