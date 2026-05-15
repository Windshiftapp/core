package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// AdminUserHandler handles admin user management in REST API v1.
type AdminUserHandler struct {
	BaseHandler
	db      database.Database
	userSvc *services.UserReadService
}

// NewAdminUserHandler creates a new admin user handler.
func NewAdminUserHandler(db database.Database, permissionService *services.PermissionService) *AdminUserHandler {
	return &AdminUserHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		db:          db,
		userSvc:     services.NewUserReadService(db),
	}
}

// AdminUserResponse is the admin representation of a user.
type AdminUserResponse struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	FullName  string `json:"full_name"`
	IsActive  bool   `json:"is_active"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	Language  string `json:"language,omitempty"`
	GroupIDs  []int  `json:"group_ids"`
	CreatedAt string `json:"created_at"`
}

// AdminUserUpdateRequest is the request body for updating a user.
type AdminUserUpdateRequest struct {
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	Email     *string `json:"email,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
}

// List handles GET /rest/api/v1/admin/users
//
// @Summary      List users (admin)
// @Description  System-admin only. Returns the full user record including email, timezone, language, and group memberships.
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        page   query     int  false  "Page number (1-based)"
// @Param        limit  query     int  false  "Items per page (max 100)"
// @Success      200    {object}  handlers.PaginatedResponse{data=[]handlers.AdminUserResponse}
// @Failure      401    {object}  handlers.ErrorResponse
// @Failure      403    {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:users:read scope"
// @Failure      500    {object}  handlers.ErrorResponse
// @Router       /admin/users [get]
func (h *AdminUserHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	users, total, err := h.userSvc.List(services.PaginationParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	response := make([]AdminUserResponse, len(users))
	for i, u := range users {
		groupIDs := h.getUserGroupIDs(u.ID)
		response[i] = AdminUserResponse{
			ID:        u.ID,
			Email:     u.Email,
			Username:  u.Username,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			FullName:  u.FullName,
			IsActive:  u.IsActive,
			AvatarURL: u.AvatarURL,
			Timezone:  u.Timezone,
			Language:  u.Language,
			GroupIDs:  groupIDs,
			CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	h.RespondPaginated(w, response, pagination, total)
}

// Update handles PUT /rest/api/v1/admin/users/{id}
//
// @Summary      Update a user (admin)
// @Description  System-admin only.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "User ID"
// @Param        body  body      handlers.AdminUserUpdateRequest  true  "Fields to update"
// @Success      200   {object}  handlers.UserResponse
// @Failure      400   {object}  handlers.ErrorResponse  "Invalid user ID, request body, or no fields to update"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Caller is not a system admin or token lacks the admin:users:write scope"
// @Failure      404   {object}  handlers.ErrorResponse  "User not found"
// @Failure      500   {object}  handlers.ErrorResponse
// @Router       /admin/users/{id} [put]
func (h *AdminUserHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "user ID")
	if !ok {
		return
	}

	var req AdminUserUpdateRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	b := NewDynamicUpdateBuilder()
	b.AddString("first_name", req.FirstName)
	b.AddString("last_name", req.LastName)
	b.AddString("email", req.Email)
	b.AddBool("is_active", req.IsActive)

	if !h.ValidateNoFields(w, r, b) {
		return
	}
	b.AddTimestamp()

	query, args := b.BuildUpdateByID("users", id)

	result, err := h.db.ExecWrite(query, args...)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		h.RespondError(w, r, restapi.ErrUserNotFound)
		return
	}

	u, err := h.userSvc.GetByID(id)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, mapUserToResponse(u))
}

func (h *AdminUserHandler) getUserGroupIDs(userID int) []int {
	rows, err := h.db.Query("SELECT group_id FROM group_members WHERE user_id = ?", userID)
	if err != nil {
		return []int{}
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return []int{}
	}
	if ids == nil {
		return []int{}
	}
	return ids
}
