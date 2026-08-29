package handlers

import (
	"errors"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// ========================================
// Milestones Handler
// ========================================

type MilestoneHandler struct {
	BaseHandler
	planningService *services.PlanningService
	itemCRUD        *services.ItemCRUDService
}

func (b *BaseHandler) respondPlanningMutationError(w http.ResponseWriter, r *http.Request, err error) bool {
	validationErr, ok := services.AsPlanningValidationError(err)
	if !ok {
		return false
	}
	b.RespondError(w, r, restapi.NewAPIError(
		http.StatusBadRequest,
		restapi.ErrCodeValidationFailed,
		validationErr.Error(),
	).WithDetails(map[string]any{"field": validationErr.Field}))
	return true
}

func NewMilestoneHandler(db database.Database, permissionService *services.PermissionService) *MilestoneHandler {
	return &MilestoneHandler{
		BaseHandler:     NewBaseHandler(db, permissionService),
		planningService: services.NewPlanningService(db),
		itemCRUD:        services.NewItemCRUDService(db),
	}
}

// MilestoneResponse — Warnings carries the user-facing strings the
// frontend surfaces at info severity for any field sanitize had to
// modify at decode time. omitempty when nothing was modified.
type MilestoneResponse struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	TargetDate    string   `json:"target_date,omitempty"`
	Status        string   `json:"status"`
	CategoryID    *int     `json:"category_id,omitempty"`
	CategoryName  string   `json:"category_name,omitempty"`
	CategoryColor string   `json:"category_color,omitempty"`
	IsGlobal      bool     `json:"is_global"`
	WorkspaceID   *int     `json:"workspace_id,omitempty"`
	Position      int      `json:"position"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	Warnings      []string `json:"warnings,omitempty"`
}

type MilestoneCreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty"`
	TargetDate  string `json:"target_date,omitempty"`
	Status      string `json:"status,omitempty"`
	CategoryID  *int   `json:"category_id,omitempty"`
}

// MilestoneUpdateRequest is the body for the milestone PUT endpoints. Every
// field is optional: absent fields keep their persisted value, so a caller can
// change one attribute without re-sending the whole milestone.
type MilestoneUpdateRequest = models.MilestonePatch

// sortOrderFromPagination translates the pagination SortAsc flag into the
// "asc"/"desc" string the service layer expects.
func sortOrderFromPagination(asc bool) string {
	if asc {
		return "asc"
	}
	return "desc"
}

// milestoneSortByFromRequest preserves the milestone service default manual
// position ordering unless the client explicitly supplies ?sort=... .
func milestoneSortByFromRequest(r *http.Request, sortBy string) string {
	if r.URL.Query().Get("sort") == "" {
		return ""
	}
	return sortBy
}

func toMilestoneResponse(m *services.MilestoneResult) MilestoneResponse {
	return MilestoneResponse{
		ID:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		TargetDate:    m.TargetDate,
		Status:        m.Status,
		CategoryID:    m.CategoryID,
		CategoryName:  m.CategoryName,
		CategoryColor: m.CategoryColor,
		IsGlobal:      m.IsGlobal,
		WorkspaceID:   m.WorkspaceID,
		Position:      m.Position,
		CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// List handles GET /rest/api/v1/milestones
//
// @Summary      List milestones
// @Description  Paginated list of milestones across all scopes (global and workspace-scoped). Filtering by workspace is done via /workspaces/{id}/milestones.
// @Tags         milestones
// @Produce      json
// @Security     BearerAuth
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]handlers.MilestoneResponse}
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the milestones:read scope"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /milestones [get]
func (h *MilestoneHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	workspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	pagination := h.ParsePagination(r)

	results, total, err := h.planningService.ListMilestones(services.MilestoneListParams{
		Limit:         pagination.Limit,
		Offset:        pagination.Offset,
		WorkspaceIDs:  workspaceIDs,
		IncludeGlobal: true,
		SortBy:        milestoneSortByFromRequest(r, pagination.SortBy),
		SortOrder:     sortOrderFromPagination(pagination.SortAsc),
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	var milestones []MilestoneResponse
	for _, m := range results {
		milestones = append(milestones, toMilestoneResponse(&m))
	}

	if milestones == nil {
		milestones = []MilestoneResponse{}
	}

	h.RespondPaginated(w, milestones, pagination, total)
}

// Get handles GET /rest/api/v1/milestones/{id}
//
// @Summary      Get a milestone by ID
// @Description  Returns the milestone whether it is global or workspace-scoped. Workspace-scoped milestones invisible to the caller surface as 404.
// @Tags         milestones
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Milestone ID"
// @Success      200  {object}  handlers.MilestoneResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid milestone ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the milestones:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Milestone not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /milestones/{id} [get]
func (h *MilestoneHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, id, _, ok := h.requireMilestoneAccessByID(w, r, false)
	if !ok {
		return
	}

	m, err := h.planningService.GetMilestone(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, toMilestoneResponse(m))
}

// Create handles POST /rest/api/v1/milestones
//
// @Summary      Create a global milestone
// @Description  Creates a global (cross-workspace) milestone. Requires the global `milestone.create` permission in addition to the milestones:write token scope. Workspace-scoped milestones should be created via POST /workspaces/{id}/milestones.
// @Tags         milestones
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handlers.MilestoneCreateRequest  true  "Milestone to create"
// @Success      201   {object}  handlers.MilestoneResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or missing required field"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks milestones:write or caller lacks milestone.create"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /milestones [post]
func (h *MilestoneHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	if !h.RequireGlobalPermission(w, r, user.ID, models.PermissionMilestoneCreate, "milestone.create") {
		return
	}

	var req MilestoneCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}

	var targetDate *string
	if req.TargetDate != "" {
		targetDate = &req.TargetDate
	}

	auditActor := h.AuditActor(r, user)
	m, err := h.planningService.CreateMilestone(services.CreateMilestoneParams{
		Name:        req.Name,
		Description: req.Description,
		TargetDate:  targetDate,
		Status:      req.Status,
		CategoryID:  req.CategoryID,
		IsGlobal:    true,
		AuditActor:  &auditActor,
	})
	if err != nil {
		if h.respondPlanningMutationError(w, r, err) {
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	resp := toMilestoneResponse(m)
	resp.Warnings = warnings
	h.RespondCreated(w, resp)
}

// requireMilestoneAccessByID is the scope-aware permission check for the
// global /milestones/{id} routes. It parses the milestone ID, authenticates
// the user, looks up whether the milestone is global or workspace-scoped,
// and applies the appropriate check:
//   - Global milestone, view: any authenticated user.
//   - Global milestone, edit: HasGlobalPermission(milestone.create).
//   - Workspace-scoped milestone, view: CanViewWorkspace.
//   - Workspace-scoped milestone, edit: CanEditWorkspace.
//
// Returns the (userID, milestoneID, scope) tuple plus ok. The workspace-scoped
// /workspaces/{id}/milestones/... routes don't need this — they carry scope in
// the URL and use requireWorkspaceMilestone* helpers instead.
func (h *MilestoneHandler) requireMilestoneAccessByID(w http.ResponseWriter, r *http.Request, edit bool) (userID, milestoneID int, workspaceID *int, ok bool) {
	user, authed := h.RequireAuth(w, r)
	if !authed {
		return 0, 0, nil, false
	}
	id, parsed := h.ParsePathID(w, r, "id", "milestone ID")
	if !parsed {
		return 0, 0, nil, false
	}
	global, wsID, err := h.planningService.IsMilestoneGlobal(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return 0, 0, nil, false
	}
	if global {
		if edit {
			hasPerm, permErr := h.Perms.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
			if permErr != nil || !hasPerm {
				h.RespondError(w, r, restapi.ErrForbidden)
				return 0, 0, nil, false
			}
		}
		return user.ID, id, nil, true
	}
	if wsID == nil {
		// Treat an invalid workspace-scoped row as not found.
		h.RespondNotFound(w, r)
		return 0, 0, nil, false
	}
	var hasPerm bool
	var permErr error
	if edit {
		hasPerm, permErr = h.Perms.CanEditWorkspace(user.ID, *wsID)
	} else {
		hasPerm, permErr = h.Perms.CanViewWorkspace(user.ID, *wsID)
	}
	if permErr != nil || !hasPerm {
		h.RespondError(w, r, restapi.ErrForbidden)
		return 0, 0, nil, false
	}
	return user.ID, id, wsID, true
}

// storedMilestoneTargetDate normalizes the driver's timestamp for partial
// updates; an empty value represents a stored NULL.
func storedMilestoneTargetDate(stored string) *string {
	if stored == "" {
		return nil
	}
	if parsed, err := time.Parse(time.RFC3339, stored); err == nil {
		date := parsed.Format("2006-01-02")
		return &date
	}
	return &stored
}

// applyMilestoneUpdate merges a partial request with the stored row before
// writing every column; workspaceID keeps the update in its original scope.
func (h *MilestoneHandler) applyMilestoneUpdate(w http.ResponseWriter, r *http.Request, current *services.MilestoneResult, workspaceID *int) {
	var req MilestoneUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	merged := req.Apply(models.Milestone{
		Name:        current.Name,
		Description: current.Description,
		TargetDate:  storedMilestoneTargetDate(current.TargetDate),
		Status:      current.Status,
		CategoryID:  current.CategoryID,
	})
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &merged.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &merged.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	user := middleware.GetUser(r.Context())
	var auditActor *services.AuditActor
	if user != nil {
		actor := h.AuditActor(r, user)
		auditActor = &actor
	}
	updated, err := h.planningService.UpdateMilestone(services.UpdateMilestoneParams{
		ID:          current.ID,
		Name:        merged.Name,
		Description: merged.Description,
		TargetDate:  merged.TargetDate,
		Status:      merged.Status,
		CategoryID:  merged.CategoryID,
		WorkspaceID: workspaceID,
		AuditActor:  auditActor,
	})
	if err != nil {
		if h.respondPlanningMutationError(w, r, err) {
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	resp := toMilestoneResponse(updated)
	resp.Warnings = warnings
	h.RespondOK(w, resp)
}

// Update handles PUT /rest/api/v1/milestones/{id}
//
// @Summary      Update a milestone
// @Description  Updates a milestone in place. Omitted fields keep their current value; explicit null clears target_date and category_id. Scope (global vs workspace-scoped) is taken from the persisted row, not the request body — milestones cannot be retargeted between scopes.
// @Tags         milestones
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "Milestone ID"
// @Param        body  body      handlers.MilestoneUpdateRequest  true  "Fields to update"
// @Success      200   {object}  handlers.MilestoneResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid milestone ID or request body"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the milestones:write scope or caller cannot edit this milestone"
// @Failure      404   {object}  handlers.ErrorResponse  "Milestone not found or not visible to caller"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /milestones/{id} [put]
func (h *MilestoneHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, id, workspaceID, ok := h.requireMilestoneAccessByID(w, r, true)
	if !ok {
		return
	}

	current, err := h.planningService.GetMilestone(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.applyMilestoneUpdate(w, r, current, workspaceID)
}

// Delete handles DELETE /rest/api/v1/milestones/{id}
//
// @Summary      Delete a milestone
// @Tags         milestones
// @Security     BearerAuth
// @Param        id   path  int  true  "Milestone ID"
// @Success      204  "Milestone deleted"
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid milestone ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the milestones:delete scope or caller cannot delete this milestone"
// @Failure      404  {object}  handlers.ErrorResponse  "Milestone not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /milestones/{id} [delete]
func (h *MilestoneHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, id, _, ok := h.requireMilestoneAccessByID(w, r, true)
	if !ok {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil || user.ID != userID {
		h.RespondUnauthorized(w, r)
		return
	}
	err := h.planningService.DeleteMilestone(id, h.AuditActor(r, user))
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}

// milestoneReorderRequest is the body for the v1 reorder endpoints.
type milestoneReorderRequest struct {
	OrderedIDs []int `json:"ordered_ids"`
	CategoryID *int  `json:"category_id,omitempty"`
}

// applyMilestoneReorder decodes a reorder request and applies it to the
// supplied scope. Shared by the global and workspace-scoped v1 reorder
// handlers — only the scope differs.
func (h *MilestoneHandler) applyMilestoneReorder(w http.ResponseWriter, r *http.Request, scope services.MilestoneScope) {
	var req milestoneReorderRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	if len(req.OrderedIDs) == 0 {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "ordered_ids is required"))
		return
	}
	scope.CategoryID = req.CategoryID

	user := middleware.GetUser(r.Context())
	if user == nil {
		h.RespondUnauthorized(w, r)
		return
	}
	if err := h.planningService.ReorderMilestones(scope, req.OrderedIDs, h.AuditActor(r, user)); err != nil {
		if errors.Is(err, services.ErrInvalidMilestoneReorder) {
			restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, err.Error()))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, map[string]bool{"ok": true})
}

// ReorderGlobal handles POST /rest/api/v1/milestones/reorder
//
// @Summary      Reorder global milestones
// @Description  Reassigns manual sort positions for global milestones. The full, in-scope ordering is supplied as ordered_ids; category_id optionally narrows the scope to a single category. Requires the global milestone.create permission.
// @Tags         milestones
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  handlers.milestoneReorderRequest  true  "Ordered milestone IDs"
// @Success      200   {object}  map[string]bool
// @Failure      400   {object}  handlers.ErrorResponse
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks milestones:write or caller lacks milestone.create"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /milestones/reorder [post]
func (h *MilestoneHandler) ReorderGlobal(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	if !h.RequireGlobalPermission(w, r, user.ID, models.PermissionMilestoneCreate, "milestone.create") {
		return
	}
	h.applyMilestoneReorder(w, r, services.MilestoneScope{IsGlobal: true})
}

// ReorderInWorkspace handles POST /rest/api/v1/workspaces/{id}/milestones/reorder
//
// @Summary      Reorder milestones in a workspace
// @Description  Reassigns manual sort positions for milestones owned by the workspace in the URL. Global milestones are not affected. category_id optionally narrows the scope to a single category.
// @Tags         workspaces, milestones
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int     true  "Workspace ID"
// @Param        body  body  handlers.milestoneReorderRequest  true  "Ordered milestone IDs"
// @Success      200   {object}  map[string]bool
// @Failure      400   {object}  handlers.ErrorResponse
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks items:write"
// @Failure      404   {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/milestones/reorder [post]
func (h *MilestoneHandler) ReorderInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}
	h.applyMilestoneReorder(w, r, services.MilestoneScope{IsGlobal: false, WorkspaceID: &wsID})
}

// MilestoneProgressResponse is the v1 representation of services.MilestoneProgressReport.
type MilestoneProgressResponse struct {
	MilestoneID     int                                        `json:"milestone_id"`
	MilestoneName   string                                     `json:"milestone_name"`
	Description     string                                     `json:"description,omitempty"`
	TargetDate      *string                                    `json:"target_date,omitempty"`
	Status          string                                     `json:"status"`
	CategoryColor   string                                     `json:"category_color,omitempty"`
	TotalItems      int                                        `json:"total_items"`
	CompletedItems  int                                        `json:"completed_items"`
	PercentComplete float64                                    `json:"percent_complete"`
	StatusBreakdown []MilestoneStatusBreakdownResponse         `json:"status_breakdown"`
	ItemsByCategory map[string][]MilestoneProgressItemResponse `json:"items_by_category"`
}

type MilestoneStatusBreakdownResponse struct {
	CategoryName       string `json:"category_name"`
	CategoryBuiltinKey string `json:"category_builtin_key,omitempty"`
	CategoryColor      string `json:"category_color,omitempty"`
	ItemCount          int    `json:"item_count"`
	IsCompleted        bool   `json:"is_completed"`
}

type MilestoneProgressItemResponse struct {
	ID                 int    `json:"id"`
	Title              string `json:"title"`
	WorkspaceID        int    `json:"workspace_id"`
	WorkspaceKey       string `json:"workspace_key"`
	ItemNumber         int    `json:"item_number"`
	StatusName         string `json:"status_name,omitempty"`
	StatusBuiltinKey   string `json:"status_builtin_key,omitempty"`
	StatusColor        string `json:"status_color,omitempty"`
	PriorityName       string `json:"priority_name,omitempty"`
	PriorityBuiltinKey string `json:"priority_builtin_key,omitempty"`
	PriorityColor      string `json:"priority_color,omitempty"`
	AssigneeName       string `json:"assignee_name,omitempty"`
	AssigneeAvatar     string `json:"assignee_avatar,omitempty"`
}

func toMilestoneProgressResponse(r *services.MilestoneProgressReport) MilestoneProgressResponse {
	resp := MilestoneProgressResponse{
		MilestoneID:     r.MilestoneID,
		MilestoneName:   r.MilestoneName,
		Description:     r.Description,
		TargetDate:      r.TargetDate,
		Status:          r.Status,
		CategoryColor:   r.CategoryColor,
		TotalItems:      r.TotalItems,
		CompletedItems:  r.CompletedItems,
		PercentComplete: r.PercentComplete,
	}
	resp.StatusBreakdown = make([]MilestoneStatusBreakdownResponse, 0, len(r.StatusBreakdown))
	for _, sb := range r.StatusBreakdown {
		resp.StatusBreakdown = append(resp.StatusBreakdown, MilestoneStatusBreakdownResponse{
			CategoryName:       sb.CategoryName,
			CategoryBuiltinKey: sb.CategoryBuiltinKey,
			CategoryColor:      sb.CategoryColor,
			ItemCount:          sb.ItemCount,
			IsCompleted:        sb.IsCompleted,
		})
	}
	if len(r.ItemsByCategory) > 0 {
		resp.ItemsByCategory = make(map[string][]MilestoneProgressItemResponse, len(r.ItemsByCategory))
		for category, items := range r.ItemsByCategory {
			converted := make([]MilestoneProgressItemResponse, 0, len(items))
			for _, it := range items {
				converted = append(converted, MilestoneProgressItemResponse{
					ID:                 it.ID,
					Title:              it.Title,
					WorkspaceID:        it.WorkspaceID,
					WorkspaceKey:       it.WorkspaceKey,
					ItemNumber:         it.ItemNumber,
					StatusName:         it.StatusName,
					StatusBuiltinKey:   it.StatusBuiltinKey,
					StatusColor:        it.StatusColor,
					PriorityName:       it.PriorityName,
					PriorityBuiltinKey: it.PriorityBuiltinKey,
					PriorityColor:      it.PriorityColor,
					AssigneeName:       it.AssigneeName,
					AssigneeAvatar:     it.AssigneeAvatar,
				})
			}
			resp.ItemsByCategory[category] = converted
		}
	}
	return resp
}

// GetProgress handles GET /rest/api/v1/milestones/{id}/progress
//
// @Summary      Get progress report for a milestone
// @Description  Returns aggregated progress metrics for the milestone (totals, status breakdown, items grouped by category).
// @Tags         milestones
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Milestone ID"
// @Success      200  {object}  handlers.MilestoneProgressResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid milestone ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the milestones:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Milestone not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /milestones/{id}/progress [get]
func (h *MilestoneHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	userID, id, _, ok := h.requireMilestoneAccessByID(w, r, false)
	if !ok {
		return
	}
	workspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(userID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	report, err := h.planningService.GetMilestoneProgress(id, workspaceIDs)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, toMilestoneProgressResponse(report))
}

// GetItems handles GET /rest/api/v1/milestones/{id}/items
//
// @Summary      List items belonging to a milestone
// @Description  Paginated list of items associated with the milestone, filtered by the caller's accessible workspaces. Global milestones may aggregate items from many workspaces.
// @Tags         milestones, items
// @Produce      json
// @Security     BearerAuth
// @Param        id     path      int     true   "Milestone ID"
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]dto.ItemResponse}
// @Failure      400    {object}  handlers.ErrorResponse  "Invalid milestone ID"
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the milestones:read scope"
// @Failure      404    {object}  handlers.ErrorResponse  "Milestone not found or not visible to caller"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /milestones/{id}/items [get]
func (h *MilestoneHandler) GetItems(w http.ResponseWriter, r *http.Request) {
	userID, milestoneID, _, ok := h.requireMilestoneAccessByID(w, r, false)
	if !ok {
		return
	}

	// Filter aggregated items to workspaces visible to the caller.
	accessibleWorkspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(userID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	pagination := h.ParsePagination(r)
	baseURL := getBaseURL(r)

	if len(accessibleWorkspaceIDs) == 0 {
		h.RespondPaginated(w, []dto.ItemResponse{}, pagination, 0)
		return
	}

	items, total, err := h.itemCRUD.List(services.ItemListParams{
		WorkspaceIDs: accessibleWorkspaceIDs,
		Filters: services.ItemFilters{
			MilestoneID: &milestoneID,
		},
		Pagination: services.PaginationParams{
			Limit:  pagination.Limit,
			Offset: pagination.Offset,
		},
		SortBy:  "created_at",
		SortAsc: false,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.maskProjectNames(userID, items)

	response := dto.MapItemsToResponse(items, baseURL)
	h.RespondPaginated(w, response, pagination, total)
}

// ----------------------------------------
// Workspace-scoped milestone routes
// ----------------------------------------
// Routes under /workspaces/{id}/milestones[...] mirror the global surface
// but constrain every read and mutation to the workspace named in the URL.
// A token issued for one workspace cannot reach another workspace's
// milestones via these routes — IsGlobal milestones and milestones owned
// by a different workspace surface as 404 to avoid leaking existence.

// resolveWorkspaceMilestone parses the milestoneId path param, fetches the
// milestone, and verifies it is workspace-scoped to wsID. Global milestones
// or milestones owned by a different workspace return 404.
func (h *MilestoneHandler) resolveWorkspaceMilestone(w http.ResponseWriter, r *http.Request, wsID int) (*services.MilestoneResult, bool) {
	milestoneID, ok := h.ParsePathID(w, r, "milestoneId", "milestone ID")
	if !ok {
		return nil, false
	}
	m, err := h.planningService.GetMilestone(milestoneID)
	if err != nil {
		h.RespondNotFound(w, r)
		return nil, false
	}
	if m.IsGlobal || m.WorkspaceID == nil || *m.WorkspaceID != wsID {
		h.RespondNotFound(w, r)
		return nil, false
	}
	return m, true
}

// ListForWorkspace handles GET /rest/api/v1/workspaces/{id}/milestones
//
// @Summary      List milestones in a workspace
// @Description  Lists milestones owned by the given workspace. Global milestones are not included — use GET /milestones for those.
// @Tags         workspaces, milestones
// @Produce      json
// @Security     BearerAuth
// @Param        id     path      int     true   "Workspace ID"
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]handlers.MilestoneResponse}
// @Failure      400    {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404    {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/milestones [get]
func (h *MilestoneHandler) ListForWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	results, total, err := h.planningService.ListMilestones(services.MilestoneListParams{
		Limit:         pagination.Limit,
		Offset:        pagination.Offset,
		WorkspaceID:   &wsID,
		IncludeGlobal: false,
		SortBy:        milestoneSortByFromRequest(r, pagination.SortBy),
		SortOrder:     sortOrderFromPagination(pagination.SortAsc),
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	milestones := make([]MilestoneResponse, 0, len(results))
	for _, m := range results {
		milestones = append(milestones, toMilestoneResponse(&m))
	}

	h.RespondPaginated(w, milestones, pagination, total)
}

// CreateInWorkspace handles POST /rest/api/v1/workspaces/{id}/milestones
//
// @Summary      Create a milestone in a workspace
// @Description  Creates a workspace-scoped milestone. The new milestone is owned by the workspace named in the URL.
// @Tags         workspaces, milestones
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "Workspace ID"
// @Param        body  body      handlers.MilestoneCreateRequest  true  "Milestone to create"
// @Success      201   {object}  handlers.MilestoneResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid workspace ID, request body, or missing required field"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/milestones [post]
func (h *MilestoneHandler) CreateInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	var req MilestoneCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}

	var targetDate *string
	if req.TargetDate != "" {
		targetDate = &req.TargetDate
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		h.RespondUnauthorized(w, r)
		return
	}
	auditActor := h.AuditActor(r, user)
	m, err := h.planningService.CreateMilestone(services.CreateMilestoneParams{
		Name:        req.Name,
		Description: req.Description,
		TargetDate:  targetDate,
		Status:      req.Status,
		CategoryID:  req.CategoryID,
		IsGlobal:    false,
		WorkspaceID: &wsID,
		AuditActor:  &auditActor,
	})
	if err != nil {
		if h.respondPlanningMutationError(w, r, err) {
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	resp := toMilestoneResponse(m)
	resp.Warnings = warnings
	h.RespondCreated(w, resp)
}

// GetInWorkspace handles GET /rest/api/v1/workspaces/{id}/milestones/{milestoneId}
//
// @Summary      Get a workspace milestone by ID
// @Description  Returns the milestone only if it is owned by the workspace in the URL. Global milestones and milestones owned by another workspace surface as 404.
// @Tags         workspaces, milestones
// @Produce      json
// @Security     BearerAuth
// @Param        id           path      int  true  "Workspace ID"
// @Param        milestoneId  path      int  true  "Milestone ID"
// @Success      200          {object}  handlers.MilestoneResponse
// @Failure      400          {object}  handlers.ErrorResponse  "Invalid workspace or milestone ID"
// @Failure      401          {object}  handlers.ErrorResponse
// @Failure      403          {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404          {object}  handlers.ErrorResponse  "Workspace not found, milestone not found, or not visible to caller"
// @Failure      500          {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/milestones/{milestoneId} [get]
func (h *MilestoneHandler) GetInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	m, ok := h.resolveWorkspaceMilestone(w, r, wsID)
	if !ok {
		return
	}

	h.RespondOK(w, toMilestoneResponse(m))
}

// UpdateInWorkspace handles PUT /rest/api/v1/workspaces/{id}/milestones/{milestoneId}
//
// @Summary      Update a workspace milestone
// @Description  Updates a workspace-scoped milestone. Omitted fields keep their current value; explicit null clears target_date and category_id. The milestone cannot be retargeted to another workspace via the body — workspace ownership is taken from the URL.
// @Tags         workspaces, milestones
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id           path      int                              true  "Workspace ID"
// @Param        milestoneId  path      int                              true  "Milestone ID"
// @Param        body         body      handlers.MilestoneUpdateRequest  true  "Fields to update"
// @Success      200          {object}  handlers.MilestoneResponse
// @Failure      400          {object}  handlers.ErrorResponse  "Invalid workspace ID, milestone ID, or request body"
// @Failure      401          {object}  handlers.ErrorResponse
// @Failure      403          {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404          {object}  handlers.ErrorResponse  "Workspace not found, milestone not found, or not visible to caller"
// @Failure      500          {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/milestones/{milestoneId} [put]
func (h *MilestoneHandler) UpdateInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	m, ok := h.resolveWorkspaceMilestone(w, r, wsID)
	if !ok {
		return
	}

	// Scope the update to the resolved workspace as defense in depth.
	h.applyMilestoneUpdate(w, r, m, &wsID)
}

// DeleteInWorkspace handles DELETE /rest/api/v1/workspaces/{id}/milestones/{milestoneId}
//
// @Summary      Delete a workspace milestone
// @Tags         workspaces, milestones
// @Security     BearerAuth
// @Param        id           path  int  true  "Workspace ID"
// @Param        milestoneId  path  int  true  "Milestone ID"
// @Success      204          "Milestone deleted"
// @Failure      400          {object}  handlers.ErrorResponse  "Invalid workspace or milestone ID"
// @Failure      401          {object}  handlers.ErrorResponse
// @Failure      403          {object}  handlers.ErrorResponse  "Token lacks the items:delete scope"
// @Failure      404          {object}  handlers.ErrorResponse  "Workspace not found, milestone not found, or not visible to caller"
// @Failure      500          {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/milestones/{milestoneId} [delete]
func (h *MilestoneHandler) DeleteInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	m, ok := h.resolveWorkspaceMilestone(w, r, wsID)
	if !ok {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		h.RespondUnauthorized(w, r)
		return
	}
	if err := h.planningService.DeleteMilestone(m.ID, h.AuditActor(r, user)); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}

// GetItemsInWorkspace handles GET /rest/api/v1/workspaces/{id}/milestones/{milestoneId}/items
//
// @Summary      List items belonging to a workspace milestone
// @Description  Paginated list of items in this workspace assigned to the given milestone.
// @Tags         workspaces, milestones, items
// @Produce      json
// @Security     BearerAuth
// @Param        id           path      int     true   "Workspace ID"
// @Param        milestoneId  path      int     true   "Milestone ID"
// @Param        page         query     int     false  "Page number (1-based)"
// @Param        limit        query     int     false  "Items per page (max 100)"
// @Param        sort         query     string  false  "Sort field"
// @Param        order        query     string  false  "Sort order: asc or desc"
// @Success      200          {object}  handlers.PaginatedResponse{data=[]dto.ItemResponse}
// @Failure      400          {object}  handlers.ErrorResponse  "Invalid workspace or milestone ID"
// @Failure      401          {object}  handlers.ErrorResponse
// @Failure      403          {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404          {object}  handlers.ErrorResponse  "Workspace not found, milestone not found, or not visible to caller"
// @Failure      500          {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/milestones/{milestoneId}/items [get]
func (h *MilestoneHandler) GetItemsInWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	m, ok := h.resolveWorkspaceMilestone(w, r, wsID)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)
	baseURL := getBaseURL(r)

	items, total, err := h.itemCRUD.List(services.ItemListParams{
		WorkspaceIDs: []int{wsID},
		Filters: services.ItemFilters{
			MilestoneID: &m.ID,
		},
		Pagination: services.PaginationParams{
			Limit:  pagination.Limit,
			Offset: pagination.Offset,
		},
		SortBy:  "created_at",
		SortAsc: false,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.maskProjectNames(user.ID, items)

	response := dto.MapItemsToResponse(items, baseURL)
	h.RespondPaginated(w, response, pagination, total)
}

// GetProgressInWorkspace handles GET /rest/api/v1/workspaces/{id}/milestones/{milestoneId}/progress
//
// @Summary      Get progress report for a workspace milestone
// @Description  Returns aggregated progress metrics for the given workspace milestone.
// @Tags         workspaces, milestones
// @Produce      json
// @Security     BearerAuth
// @Param        id           path      int  true  "Workspace ID"
// @Param        milestoneId  path      int  true  "Milestone ID"
// @Success      200          {object}  handlers.MilestoneProgressResponse
// @Failure      400          {object}  handlers.ErrorResponse  "Invalid workspace or milestone ID"
// @Failure      401          {object}  handlers.ErrorResponse
// @Failure      403          {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404          {object}  handlers.ErrorResponse  "Workspace not found, milestone not found, or not visible to caller"
// @Failure      500          {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/milestones/{milestoneId}/progress [get]
func (h *MilestoneHandler) GetProgressInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	m, ok := h.resolveWorkspaceMilestone(w, r, wsID)
	if !ok {
		return
	}

	report, err := h.planningService.GetMilestoneProgress(m.ID, []int{wsID})
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, toMilestoneProgressResponse(report))
}

// ========================================
// Iterations Handler
// ========================================

type IterationHandler struct {
	BaseHandler
	planningService *services.PlanningService
}

func NewIterationHandler(db database.Database, permissionService *services.PermissionService) *IterationHandler {
	return &IterationHandler{
		BaseHandler:     NewBaseHandler(db, permissionService),
		planningService: services.NewPlanningService(db),
	}
}

// IterationResponse — Warnings: same shape as MilestoneResponse.
type IterationResponse struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	StartDate   string   `json:"start_date"`
	EndDate     string   `json:"end_date"`
	Status      string   `json:"status"`
	TypeID      *int     `json:"type_id,omitempty"`
	TypeName    string   `json:"type_name,omitempty"`
	TypeColor   string   `json:"type_color,omitempty"`
	IsGlobal    bool     `json:"is_global"`
	WorkspaceID *int     `json:"workspace_id,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Warnings    []string `json:"warnings,omitempty"`
}

type IterationCreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"start_date" validate:"required"`
	EndDate     string `json:"end_date" validate:"required"`
	Status      string `json:"status,omitempty"`
	TypeID      *int   `json:"type_id,omitempty"`
	IsGlobal    bool   `json:"is_global,omitempty"`
	WorkspaceID *int   `json:"workspace_id,omitempty"`
}

type IterationUpdateRequest = models.IterationPatch

func toIterationResponse(iter *services.IterationResult) IterationResponse {
	return IterationResponse{
		ID:          iter.ID,
		Name:        iter.Name,
		Description: iter.Description,
		StartDate:   iter.StartDate,
		EndDate:     iter.EndDate,
		Status:      iter.Status,
		TypeID:      iter.TypeID,
		TypeName:    iter.TypeName,
		TypeColor:   iter.TypeColor,
		IsGlobal:    iter.IsGlobal,
		WorkspaceID: iter.WorkspaceID,
		CreatedAt:   iter.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   iter.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// List handles GET /rest/api/v1/iterations
//
// @Summary      List iterations
// @Description  Paginated list of iterations across all scopes (global and workspace-scoped). Filtering by workspace is done via /workspaces/{id}/iterations.
// @Tags         iterations
// @Produce      json
// @Security     BearerAuth
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]handlers.IterationResponse}
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the iterations:read scope"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /iterations [get]
func (h *IterationHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	workspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	pagination := h.ParsePagination(r)

	results, total, err := h.planningService.ListIterations(services.IterationListParams{
		Limit:         pagination.Limit,
		Offset:        pagination.Offset,
		WorkspaceIDs:  workspaceIDs,
		IncludeGlobal: true,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	var iterations []IterationResponse
	for _, iter := range results {
		iterations = append(iterations, toIterationResponse(&iter))
	}

	if iterations == nil {
		iterations = []IterationResponse{}
	}

	h.RespondPaginated(w, iterations, pagination, total)
}

// requireIterationAccessByID is the iteration analog of
// requireMilestoneAccessByID — same scope-aware permission resolution but
// against PermissionIterationManage / IsIterationGlobal.
func (h *IterationHandler) requireIterationAccessByID(w http.ResponseWriter, r *http.Request, edit bool) (iterationID int, workspaceID *int, ok bool) {
	user, authed := h.RequireAuth(w, r)
	if !authed {
		return 0, nil, false
	}
	id, parsed := h.ParsePathID(w, r, "id", "iteration ID")
	if !parsed {
		return 0, nil, false
	}
	global, wsID, err := h.planningService.IsIterationGlobal(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return 0, nil, false
	}
	if global {
		if edit {
			hasPerm, permErr := h.Perms.HasGlobalPermission(user.ID, models.PermissionIterationManage)
			if permErr != nil || !hasPerm {
				h.RespondError(w, r, restapi.ErrForbidden)
				return 0, nil, false
			}
		}
		return id, nil, true
	}
	if wsID == nil {
		h.RespondNotFound(w, r)
		return 0, nil, false
	}
	var hasPerm bool
	var permErr error
	if edit {
		hasPerm, permErr = h.Perms.CanEditWorkspace(user.ID, *wsID)
	} else {
		hasPerm, permErr = h.Perms.CanViewWorkspace(user.ID, *wsID)
	}
	if permErr != nil || !hasPerm {
		h.RespondError(w, r, restapi.ErrForbidden)
		return 0, nil, false
	}
	return id, wsID, true
}

// Get handles GET /rest/api/v1/iterations/{id}
//
// @Summary      Get an iteration by ID
// @Description  Returns the iteration whether it is global or workspace-scoped. Workspace-scoped iterations invisible to the caller surface as 404.
// @Tags         iterations
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Iteration ID"
// @Success      200  {object}  handlers.IterationResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid iteration ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the iterations:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Iteration not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /iterations/{id} [get]
func (h *IterationHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.requireIterationAccessByID(w, r, false)
	if !ok {
		return
	}

	iter, err := h.planningService.GetIteration(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, toIterationResponse(iter))
}

// Create handles POST /rest/api/v1/iterations
//
// @Summary      Create an iteration
// @Description  Creates a global iteration by default. If the body sets `workspace_id` (and `is_global` is false), the iteration is created in that workspace and the caller must have edit access on it. Global creation requires the `iteration.manage` permission.
// @Tags         iterations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handlers.IterationCreateRequest  true  "Iteration to create"
// @Success      201   {object}  handlers.IterationResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid request body or missing required field"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks iterations:write or caller lacks the required scope permission"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /iterations [post]
func (h *IterationHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	var req IterationCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}

	if req.WorkspaceID != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "workspace_id is only accepted on the workspace iteration endpoint"))
		return
	}
	if !h.RequireGlobalPermission(w, r, user.ID, models.PermissionIterationManage, "iteration.manage") {
		return
	}

	auditActor := h.AuditActor(r, user)
	iter, err := h.planningService.CreateIteration(services.CreateIterationParams{
		Name:        req.Name,
		Description: req.Description,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Status:      req.Status,
		TypeID:      req.TypeID,
		IsGlobal:    true,
		WorkspaceID: nil,
		AuditActor:  &auditActor,
	})
	if err != nil {
		if h.respondPlanningMutationError(w, r, err) {
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	resp := toIterationResponse(iter)
	resp.Warnings = warnings
	h.RespondCreated(w, resp)
}

// Update handles PUT /rest/api/v1/iterations/{id}
//
// @Summary      Update an iteration
// @Description  Updates an iteration in place. Scope (global vs workspace-scoped) is taken from the persisted row, not the request body — iterations cannot be retargeted between scopes.
// @Tags         iterations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "Iteration ID"
// @Param        body  body      handlers.IterationUpdateRequest  true  "Fields to update"
// @Success      200   {object}  handlers.IterationResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid iteration ID or request body"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the iterations:write scope or caller cannot edit this iteration"
// @Failure      404   {object}  handlers.ErrorResponse  "Iteration not found or not visible to caller"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /iterations/{id} [put]
func (h *IterationHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Use the persisted scope; request fields cannot retarget an iteration.
	id, workspaceID, ok := h.requireIterationAccessByID(w, r, true)
	if !ok {
		return
	}

	var req IterationUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	existing, err := h.planningService.GetIteration(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}
	merged := req.Apply(models.Iteration{
		Name:        existing.Name,
		Description: existing.Description,
		StartDate:   existing.StartDate,
		EndDate:     existing.EndDate,
		Status:      existing.Status,
		TypeID:      existing.TypeID,
	})
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &merged.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &merged.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	user := middleware.GetUser(r.Context())
	if user == nil {
		h.RespondUnauthorized(w, r)
		return
	}
	auditActor := h.AuditActor(r, user)
	iter, err := h.planningService.UpdateIteration(services.UpdateIterationParams{
		ID:          id,
		Name:        merged.Name,
		Description: merged.Description,
		StartDate:   merged.StartDate,
		EndDate:     merged.EndDate,
		Status:      merged.Status,
		TypeID:      merged.TypeID,
		WorkspaceID: workspaceID,
		AuditActor:  &auditActor,
	})
	if err != nil {
		if h.respondPlanningMutationError(w, r, err) {
			return
		}
		if errors.Is(err, services.ErrIterationCompletionRequired) || errors.Is(err, services.ErrIterationLifecycleConflict) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, err.Error()))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	resp := toIterationResponse(iter)
	resp.Warnings = warnings
	h.RespondOK(w, resp)
}

// Delete handles DELETE /rest/api/v1/iterations/{id}
//
// @Summary      Delete an iteration
// @Tags         iterations
// @Security     BearerAuth
// @Param        id   path  int  true  "Iteration ID"
// @Success      204  "Iteration deleted"
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid iteration ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the iterations:delete scope or caller cannot delete this iteration"
// @Failure      404  {object}  handlers.ErrorResponse  "Iteration not found or not visible to caller"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /iterations/{id} [delete]
func (h *IterationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.requireIterationAccessByID(w, r, true)
	if !ok {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		h.RespondUnauthorized(w, r)
		return
	}
	if err := h.planningService.DeleteIteration(id, h.AuditActor(r, user)); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}

// ----------------------------------------
// Workspace-scoped iteration routes
// ----------------------------------------
// Routes under /workspaces/{id}/iterations[...] mirror the global surface
// but constrain every read and mutation to the workspace named in the URL.
// Global iterations and iterations owned by a different workspace surface
// as 404 to avoid leaking existence.

// resolveWorkspaceIteration parses the iterationId path param, fetches the
// iteration, and verifies it is workspace-scoped to wsID. Global iterations
// or iterations owned by a different workspace return 404.
func (h *IterationHandler) resolveWorkspaceIteration(w http.ResponseWriter, r *http.Request, wsID int) (*services.IterationResult, bool) {
	iterationID, ok := h.ParsePathID(w, r, "iterationId", "iteration ID")
	if !ok {
		return nil, false
	}
	iter, err := h.planningService.GetIteration(iterationID)
	if err != nil {
		h.RespondNotFound(w, r)
		return nil, false
	}
	if iter.IsGlobal || iter.WorkspaceID == nil || *iter.WorkspaceID != wsID {
		h.RespondNotFound(w, r)
		return nil, false
	}
	return iter, true
}

// ListForWorkspace handles GET /rest/api/v1/workspaces/{id}/iterations
//
// @Summary      List iterations in a workspace
// @Description  Lists iterations owned by the given workspace. Global iterations are not included — use GET /iterations for those.
// @Tags         workspaces, iterations
// @Produce      json
// @Security     BearerAuth
// @Param        id     path      int     true   "Workspace ID"
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]handlers.IterationResponse}
// @Failure      400    {object}  handlers.ErrorResponse  "Invalid workspace ID"
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404    {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/iterations [get]
func (h *IterationHandler) ListForWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	results, total, err := h.planningService.ListIterations(services.IterationListParams{
		Limit:         pagination.Limit,
		Offset:        pagination.Offset,
		WorkspaceID:   &wsID,
		IncludeGlobal: false,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	iterations := make([]IterationResponse, 0, len(results))
	for _, iter := range results {
		iterations = append(iterations, toIterationResponse(&iter))
	}

	h.RespondPaginated(w, iterations, pagination, total)
}

// CreateInWorkspace handles POST /rest/api/v1/workspaces/{id}/iterations
//
// @Summary      Create an iteration in a workspace
// @Description  Creates a workspace-scoped iteration. The new iteration is owned by the workspace named in the URL.
// @Tags         workspaces, iterations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "Workspace ID"
// @Param        body  body      handlers.IterationCreateRequest  true  "Iteration to create"
// @Success      201   {object}  handlers.IterationResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid workspace ID, request body, or missing required field"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "Workspace not found or not visible to caller"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/iterations [post]
func (h *IterationHandler) CreateInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	var req IterationCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		h.RespondUnauthorized(w, r)
		return
	}
	auditActor := h.AuditActor(r, user)
	iter, err := h.planningService.CreateIteration(services.CreateIterationParams{
		Name:        req.Name,
		Description: req.Description,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Status:      req.Status,
		TypeID:      req.TypeID,
		IsGlobal:    false,
		WorkspaceID: &wsID,
		AuditActor:  &auditActor,
	})
	if err != nil {
		if h.respondPlanningMutationError(w, r, err) {
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	resp := toIterationResponse(iter)
	resp.Warnings = warnings
	h.RespondCreated(w, resp)
}

// GetInWorkspace handles GET /rest/api/v1/workspaces/{id}/iterations/{iterationId}
//
// @Summary      Get a workspace iteration by ID
// @Description  Returns the iteration only if it is owned by the workspace in the URL. Global iterations and iterations owned by another workspace surface as 404.
// @Tags         workspaces, iterations
// @Produce      json
// @Security     BearerAuth
// @Param        id           path      int  true  "Workspace ID"
// @Param        iterationId  path      int  true  "Iteration ID"
// @Success      200          {object}  handlers.IterationResponse
// @Failure      400          {object}  handlers.ErrorResponse  "Invalid workspace or iteration ID"
// @Failure      401          {object}  handlers.ErrorResponse
// @Failure      403          {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404          {object}  handlers.ErrorResponse  "Workspace not found, iteration not found, or not visible to caller"
// @Failure      500          {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/iterations/{iterationId} [get]
func (h *IterationHandler) GetInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	iter, ok := h.resolveWorkspaceIteration(w, r, wsID)
	if !ok {
		return
	}

	h.RespondOK(w, toIterationResponse(iter))
}

// UpdateInWorkspace handles PUT /rest/api/v1/workspaces/{id}/iterations/{iterationId}
//
// @Summary      Update a workspace iteration
// @Description  Updates a workspace-scoped iteration. Workspace ownership is taken from the URL — iterations cannot be retargeted via the body.
// @Tags         workspaces, iterations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id           path      int                              true  "Workspace ID"
// @Param        iterationId  path      int                              true  "Iteration ID"
// @Param        body         body      handlers.IterationUpdateRequest  true  "Fields to update"
// @Success      200          {object}  handlers.IterationResponse
// @Failure      400          {object}  handlers.ErrorResponse  "Invalid workspace ID, iteration ID, or request body"
// @Failure      401          {object}  handlers.ErrorResponse
// @Failure      403          {object}  handlers.ErrorResponse  "Token lacks the items:write scope"
// @Failure      404          {object}  handlers.ErrorResponse  "Workspace not found, iteration not found, or not visible to caller"
// @Failure      500          {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/iterations/{iterationId} [put]
func (h *IterationHandler) UpdateInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	iter, ok := h.resolveWorkspaceIteration(w, r, wsID)
	if !ok {
		return
	}

	var req IterationUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	merged := req.Apply(models.Iteration{
		Name:        iter.Name,
		Description: iter.Description,
		StartDate:   iter.StartDate,
		EndDate:     iter.EndDate,
		Status:      iter.Status,
		TypeID:      iter.TypeID,
	})
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &merged.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &merged.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	// Scope the update to the resolved workspace as defense in depth.
	user := middleware.GetUser(r.Context())
	if user == nil {
		h.RespondUnauthorized(w, r)
		return
	}
	auditActor := h.AuditActor(r, user)
	updated, err := h.planningService.UpdateIteration(services.UpdateIterationParams{
		ID:          iter.ID,
		Name:        merged.Name,
		Description: merged.Description,
		StartDate:   merged.StartDate,
		EndDate:     merged.EndDate,
		Status:      merged.Status,
		TypeID:      merged.TypeID,
		WorkspaceID: &wsID,
		AuditActor:  &auditActor,
	})
	if err != nil {
		if h.respondPlanningMutationError(w, r, err) {
			return
		}
		if errors.Is(err, services.ErrIterationCompletionRequired) || errors.Is(err, services.ErrIterationLifecycleConflict) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, err.Error()))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	resp := toIterationResponse(updated)
	resp.Warnings = warnings
	h.RespondOK(w, resp)
}

// DeleteInWorkspace handles DELETE /rest/api/v1/workspaces/{id}/iterations/{iterationId}
//
// @Summary      Delete a workspace iteration
// @Tags         workspaces, iterations
// @Security     BearerAuth
// @Param        id           path  int  true  "Workspace ID"
// @Param        iterationId  path  int  true  "Iteration ID"
// @Success      204          "Iteration deleted"
// @Failure      400          {object}  handlers.ErrorResponse  "Invalid workspace or iteration ID"
// @Failure      401          {object}  handlers.ErrorResponse
// @Failure      403          {object}  handlers.ErrorResponse  "Token lacks the items:delete scope"
// @Failure      404          {object}  handlers.ErrorResponse  "Workspace not found, iteration not found, or not visible to caller"
// @Failure      500          {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/iterations/{iterationId} [delete]
func (h *IterationHandler) DeleteInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	iter, ok := h.resolveWorkspaceIteration(w, r, wsID)
	if !ok {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		h.RespondUnauthorized(w, r)
		return
	}
	if err := h.planningService.DeleteIteration(iter.ID, h.AuditActor(r, user)); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}
