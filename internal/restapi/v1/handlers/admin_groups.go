package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// AdminGroupHandler handles admin group management in REST API v1.
type AdminGroupHandler struct {
	BaseHandler
	db database.Database
}

// NewAdminGroupHandler creates a new admin group handler.
func NewAdminGroupHandler(db database.Database, permissionService *services.PermissionService) *AdminGroupHandler {
	return &AdminGroupHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		db:          db,
	}
}

// AdminGroupResponse is the admin representation of a group.
type AdminGroupResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MemberCount int    `json:"member_count"`
	CreatedAt   string `json:"created_at"`
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

	var total int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM team_groups").Scan(&total); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	rows, err := h.db.Query(`
		SELECT g.id, g.name, g.description,
		       (SELECT COUNT(*) FROM group_members gm WHERE gm.group_id = g.id) AS member_count,
		       g.created_at
		FROM team_groups g
		ORDER BY g.name ASC
		LIMIT ? OFFSET ?
	`, pagination.Limit, pagination.Offset)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	defer rows.Close()

	var groups []AdminGroupResponse
	for rows.Next() {
		var g AdminGroupResponse
		var desc sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&g.ID, &g.Name, &desc, &g.MemberCount, &createdAt); err != nil {
			continue
		}
		if desc.Valid {
			g.Description = desc.String
		}
		g.CreatedAt = createdAt.Format("2006-01-02T15:04:05Z07:00")
		groups = append(groups, g)
	}

	if groups == nil {
		groups = []AdminGroupResponse{}
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

	if req.Name == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, "Group name is required"))
		return
	}

	var id int
	err := h.db.QueryRow(`
		INSERT INTO team_groups (name, description, created_by, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, req.Name, req.Description, user.ID).Scan(&id)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondCreated(w, AdminGroupResponse{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		MemberCount: 0,
		CreatedAt:   time.Now().Format("2006-01-02T15:04:05Z07:00"),
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
	_, ok := h.RequireAuth(w, r)
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

	b := NewDynamicUpdateBuilder()
	b.AddString("name", req.Name)
	b.AddString("description", req.Description)

	if !h.ValidateNoFields(w, r, b) {
		return
	}
	b.AddTimestamp()

	query, args := b.BuildUpdateByID("team_groups", id)

	result, err := h.db.ExecWrite(query, args...)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		h.RespondNotFound(w, r)
		return
	}

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
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "group ID")
	if !ok {
		return
	}

	// Delete members first, then group
	_, _ = h.db.ExecWrite("DELETE FROM group_members WHERE group_id = ?", id)

	result, err := h.db.ExecWrite("DELETE FROM team_groups WHERE id = ?", id)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		h.RespondNotFound(w, r)
		return
	}

	h.RespondNoContent(w)
}
