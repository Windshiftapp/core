package handlers

import (
	"net/http"
	"strings"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// AdminUserHandler handles admin user management in REST API v1.
type AdminUserHandler struct {
	db      database.Database
	userSvc *services.UserReadService
}

// NewAdminUserHandler creates a new admin user handler.
func NewAdminUserHandler(db database.Database) *AdminUserHandler {
	return &AdminUserHandler{
		db:      db,
		userSvc: services.NewUserReadService(db),
	}
}

// AdminUserResponse is the admin representation of a user.
type AdminUserResponse struct {
	ID        int      `json:"id"`
	Email     string   `json:"email"`
	Username  string   `json:"username"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	FullName  string   `json:"full_name"`
	IsActive  bool     `json:"is_active"`
	AvatarURL string   `json:"avatar_url,omitempty"`
	Timezone  string   `json:"timezone,omitempty"`
	Language  string   `json:"language,omitempty"`
	GroupIDs  []int    `json:"group_ids"`
	CreatedAt string   `json:"created_at"`
}

// AdminUserUpdateRequest is the request body for updating a user.
type AdminUserUpdateRequest struct {
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	Email     *string `json:"email,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
}

// List handles GET /rest/api/v1/admin/users
func (h *AdminUserHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAuth(w, r)
	if !ok {
		return
	}

	pagination := restapi.ParsePaginationParams(r)

	users, total, err := h.userSvc.List(services.PaginationParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
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

	restapi.RespondPaginated(w, response, restapi.NewPaginationMeta(pagination, total))
}

// Update handles PUT /rest/api/v1/admin/users/{id}
func (h *AdminUserHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAuth(w, r)
	if !ok {
		return
	}

	id, ok := parsePathID(w, r, "id", "user ID")
	if !ok {
		return
	}

	var req AdminUserUpdateRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}

	// Build dynamic update
	var sets []string
	var args []interface{}

	if req.FirstName != nil {
		sets = append(sets, "first_name = ?")
		args = append(args, *req.FirstName)
	}
	if req.LastName != nil {
		sets = append(sets, "last_name = ?")
		args = append(args, *req.LastName)
	}
	if req.Email != nil {
		sets = append(sets, "email = ?")
		args = append(args, *req.Email)
	}
	if req.IsActive != nil {
		sets = append(sets, "is_active = ?")
		args = append(args, *req.IsActive)
	}

	if len(sets) == 0 {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "No fields to update"))
		return
	}

	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := "UPDATE users SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	result, err := h.db.ExecWrite(query, args...)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		restapi.RespondError(w, r, restapi.ErrUserNotFound)
		return
	}

	u, err := h.userSvc.GetByID(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondOK(w, mapUserToResponse(u))
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
	if ids == nil {
		return []int{}
	}
	return ids
}
