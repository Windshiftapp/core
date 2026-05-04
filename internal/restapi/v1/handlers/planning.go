package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
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

func NewMilestoneHandler(db database.Database, permissionService *services.PermissionService) *MilestoneHandler {
	return &MilestoneHandler{
		BaseHandler:     NewBaseHandler(db, permissionService),
		planningService: services.NewPlanningService(db),
		itemCRUD:        services.NewItemCRUDService(db),
	}
}

type MilestoneResponse struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	TargetDate    string `json:"target_date,omitempty"`
	Status        string `json:"status"`
	CategoryID    *int   `json:"category_id,omitempty"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type MilestoneCreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty"`
	TargetDate  string `json:"target_date,omitempty"`
	Status      string `json:"status,omitempty"`
	CategoryID  *int   `json:"category_id,omitempty"`
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
		CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *MilestoneHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	results, total, err := h.planningService.ListMilestones(services.MilestoneListParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
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

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}

	var targetDate *string
	if req.TargetDate != "" {
		targetDate = &req.TargetDate
	}

	m, err := h.planningService.CreateMilestone(services.CreateMilestoneParams{
		Name:        req.Name,
		Description: req.Description,
		TargetDate:  targetDate,
		Status:      req.Status,
		CategoryID:  req.CategoryID,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondCreated(w, toMilestoneResponse(m))
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
		// workspace-scoped row missing workspace_id — treat as not found rather
		// than 500; this should be impossible per the schema constraint.
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

func (h *MilestoneHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, id, workspaceID, ok := h.requireMilestoneAccessByID(w, r, true)
	if !ok {
		return
	}

	var req MilestoneCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	var updateTargetDate *string
	if req.TargetDate != "" {
		updateTargetDate = &req.TargetDate
	}

	// WorkspaceID scopes the SQL UPDATE to the resolved milestone's owning
	// workspace (nil for global). UpdateMilestoneParams refuses cross-scope
	// edits via its WHERE clause, so this also defends against a milestone
	// being retargeted between scopes via concurrent modification.
	m, err := h.planningService.UpdateMilestone(services.UpdateMilestoneParams{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		TargetDate:  updateTargetDate,
		Status:      req.Status,
		CategoryID:  req.CategoryID,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, toMilestoneResponse(m))
}

func (h *MilestoneHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, id, _, ok := h.requireMilestoneAccessByID(w, r, true)
	if !ok {
		return
	}

	err := h.planningService.DeleteMilestone(id)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
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
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color,omitempty"`
	ItemCount     int    `json:"item_count"`
	IsCompleted   bool   `json:"is_completed"`
}

type MilestoneProgressItemResponse struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	WorkspaceID    int    `json:"workspace_id"`
	WorkspaceKey   string `json:"workspace_key"`
	ItemNumber     int    `json:"item_number"`
	StatusName     string `json:"status_name,omitempty"`
	StatusColor    string `json:"status_color,omitempty"`
	PriorityName   string `json:"priority_name,omitempty"`
	PriorityColor  string `json:"priority_color,omitempty"`
	AssigneeName   string `json:"assignee_name,omitempty"`
	AssigneeAvatar string `json:"assignee_avatar,omitempty"`
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
			CategoryName:  sb.CategoryName,
			CategoryColor: sb.CategoryColor,
			ItemCount:     sb.ItemCount,
			IsCompleted:   sb.IsCompleted,
		})
	}
	if len(r.ItemsByCategory) > 0 {
		resp.ItemsByCategory = make(map[string][]MilestoneProgressItemResponse, len(r.ItemsByCategory))
		for category, items := range r.ItemsByCategory {
			converted := make([]MilestoneProgressItemResponse, 0, len(items))
			for _, it := range items {
				converted = append(converted, MilestoneProgressItemResponse{
					ID:             it.ID,
					Title:          it.Title,
					WorkspaceID:    it.WorkspaceID,
					WorkspaceKey:   it.WorkspaceKey,
					ItemNumber:     it.ItemNumber,
					StatusName:     it.StatusName,
					StatusColor:    it.StatusColor,
					PriorityName:   it.PriorityName,
					PriorityColor:  it.PriorityColor,
					AssigneeName:   it.AssigneeName,
					AssigneeAvatar: it.AssigneeAvatar,
				})
			}
			resp.ItemsByCategory[category] = converted
		}
	}
	return resp
}

func (h *MilestoneHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	_, id, _, ok := h.requireMilestoneAccessByID(w, r, false)
	if !ok {
		return
	}

	report, err := h.planningService.GetMilestoneProgress(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, toMilestoneProgressResponse(report))
}

func (h *MilestoneHandler) GetItems(w http.ResponseWriter, r *http.Request) {
	userID, milestoneID, _, ok := h.requireMilestoneAccessByID(w, r, false)
	if !ok {
		return
	}

	// Even with view access on the milestone, the items list is filtered to
	// workspaces the user can access — a global milestone may aggregate items
	// across many workspaces, and we don't surface items the caller can't see.
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

func (h *MilestoneHandler) CreateInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	var req MilestoneCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}

	var targetDate *string
	if req.TargetDate != "" {
		targetDate = &req.TargetDate
	}

	m, err := h.planningService.CreateMilestone(services.CreateMilestoneParams{
		Name:        req.Name,
		Description: req.Description,
		TargetDate:  targetDate,
		Status:      req.Status,
		CategoryID:  req.CategoryID,
		IsGlobal:    false,
		WorkspaceID: &wsID,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondCreated(w, toMilestoneResponse(m))
}

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

func (h *MilestoneHandler) UpdateInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	m, ok := h.resolveWorkspaceMilestone(w, r, wsID)
	if !ok {
		return
	}

	var req MilestoneCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	var updateTargetDate *string
	if req.TargetDate != "" {
		updateTargetDate = &req.TargetDate
	}

	// WorkspaceID in the UpdateMilestoneParams scopes the SQL UPDATE to this
	// workspace as a defense-in-depth check beyond the URL match above.
	updated, err := h.planningService.UpdateMilestone(services.UpdateMilestoneParams{
		ID:          m.ID,
		Name:        req.Name,
		Description: req.Description,
		TargetDate:  updateTargetDate,
		Status:      req.Status,
		CategoryID:  req.CategoryID,
		WorkspaceID: &wsID,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, toMilestoneResponse(updated))
}

func (h *MilestoneHandler) DeleteInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	m, ok := h.resolveWorkspaceMilestone(w, r, wsID)
	if !ok {
		return
	}

	if err := h.planningService.DeleteMilestone(m.ID); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}

func (h *MilestoneHandler) GetItemsInWorkspace(w http.ResponseWriter, r *http.Request) {
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

	response := dto.MapItemsToResponse(items, baseURL)
	h.RespondPaginated(w, response, pagination, total)
}

func (h *MilestoneHandler) GetProgressInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceViewAccess(w, r)
	if !ok {
		return
	}

	m, ok := h.resolveWorkspaceMilestone(w, r, wsID)
	if !ok {
		return
	}

	report, err := h.planningService.GetMilestoneProgress(m.ID)
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

type IterationResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Status      string `json:"status"`
	TypeID      *int   `json:"type_id,omitempty"`
	TypeName    string `json:"type_name,omitempty"`
	TypeColor   string `json:"type_color,omitempty"`
	IsGlobal    bool   `json:"is_global"`
	WorkspaceID *int   `json:"workspace_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
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

func (h *IterationHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	results, total, err := h.planningService.ListIterations(services.IterationListParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
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

func (h *IterationHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	var req IterationCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}

	if req.IsGlobal || req.WorkspaceID == nil {
		if !h.RequireGlobalPermission(w, r, user.ID, models.PermissionIterationManage, "iteration.manage") {
			return
		}
	}

	iter, err := h.planningService.CreateIteration(services.CreateIterationParams{
		Name:        req.Name,
		Description: req.Description,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Status:      req.Status,
		TypeID:      req.TypeID,
		IsGlobal:    req.IsGlobal,
		WorkspaceID: req.WorkspaceID,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondCreated(w, toIterationResponse(iter))
}

func (h *IterationHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Scope is taken from the persisted iteration, not the request body — body
	// fields for workspace_id / is_global cannot be used to retarget.
	id, workspaceID, ok := h.requireIterationAccessByID(w, r, true)
	if !ok {
		return
	}

	var req IterationCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	iter, err := h.planningService.UpdateIteration(services.UpdateIterationParams{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Status:      req.Status,
		TypeID:      req.TypeID,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, toIterationResponse(iter))
}

func (h *IterationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.requireIterationAccessByID(w, r, true)
	if !ok {
		return
	}

	if err := h.planningService.DeleteIteration(id); err != nil {
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

func (h *IterationHandler) CreateInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	var req IterationCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}

	iter, err := h.planningService.CreateIteration(services.CreateIterationParams{
		Name:        req.Name,
		Description: req.Description,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Status:      req.Status,
		TypeID:      req.TypeID,
		IsGlobal:    false,
		WorkspaceID: &wsID,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondCreated(w, toIterationResponse(iter))
}

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

func (h *IterationHandler) UpdateInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	iter, ok := h.resolveWorkspaceIteration(w, r, wsID)
	if !ok {
		return
	}

	var req IterationCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	// WorkspaceID scopes the SQL UPDATE to this workspace as defense-in-depth
	// beyond the URL match above.
	updated, err := h.planningService.UpdateIteration(services.UpdateIterationParams{
		ID:          iter.ID,
		Name:        req.Name,
		Description: req.Description,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Status:      req.Status,
		TypeID:      req.TypeID,
		WorkspaceID: &wsID,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, toIterationResponse(updated))
}

func (h *IterationHandler) DeleteInWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, ok := h.RequireWorkspaceEditAccess(w, r)
	if !ok {
		return
	}

	iter, ok := h.resolveWorkspaceIteration(w, r, wsID)
	if !ok {
		return
	}

	if err := h.planningService.DeleteIteration(iter.ID); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}

// ========================================
// Projects Handler
// ========================================

type ProjectHandler struct {
	BaseHandler
	planningService *services.PlanningService
}

func NewProjectHandler(db database.Database, permissionService *services.PermissionService) *ProjectHandler {
	return &ProjectHandler{
		BaseHandler:     NewBaseHandler(db, permissionService),
		planningService: services.NewPlanningService(db),
	}
}

type ProjectResponse struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Active        bool   `json:"active"`
	WorkspaceID   *int   `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type ProjectCreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty"`
	WorkspaceID *int   `json:"workspace_id,omitempty"`
	Active      *bool  `json:"active,omitempty"`
}

func toProjectResponse(p *services.ProjectResult) ProjectResponse {
	return ProjectResponse{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		Active:        p.Active,
		WorkspaceID:   p.WorkspaceID,
		WorkspaceName: p.WorkspaceName,
		CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	results, total, err := h.planningService.ListProjects(services.ProjectListParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	var projects []ProjectResponse
	for _, p := range results {
		projects = append(projects, toProjectResponse(&p))
	}

	if projects == nil {
		projects = []ProjectResponse{}
	}

	h.RespondPaginated(w, projects, pagination, total)
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "project ID")
	if !ok {
		return
	}

	p, err := h.planningService.GetProject(id)
	if err != nil {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondOK(w, toProjectResponse(p))
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	var req ProjectCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	if !h.ValidateRequiredString(w, r, req.Name, "name") {
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	p, err := h.planningService.CreateProject(services.CreateProjectParams{
		Name:        req.Name,
		Description: req.Description,
		WorkspaceID: req.WorkspaceID,
		Active:      active,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondCreated(w, toProjectResponse(p))
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "project ID")
	if !ok {
		return
	}

	var req ProjectCreateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	p, err := h.planningService.UpdateProject(services.UpdateProjectParams{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		WorkspaceID: req.WorkspaceID,
		Active:      active,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, toProjectResponse(p))
}

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "project ID")
	if !ok {
		return
	}

	err := h.planningService.DeleteProject(id)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondNoContent(w)
}
