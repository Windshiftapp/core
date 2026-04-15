package handlers

import (
	"net/http"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// UserHandler handles public API requests for users
type UserHandler struct {
	BaseHandler
	userSvc *services.UserReadService
}

// NewUserHandler creates a new user handler
func NewUserHandler(db database.Database, permissionService *services.PermissionService) *UserHandler {
	return &UserHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		userSvc:     services.NewUserReadService(db),
	}
}

// UserResponse is the public API representation of a User
type UserResponse struct {
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
	CreatedAt string `json:"created_at"`
}

// List handles GET /rest/api/v1/users
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	// Check user.list permission
	hasPermission, _ := h.PermissionService.HasGlobalPermission(user.ID, models.PermissionUserList)
	if !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "user.list permission required"))
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

	isAdmin, _ := h.PermissionService.IsSystemAdmin(user.ID)

	// Map to response DTOs
	response := make([]UserResponse, len(users))
	for i, u := range users {
		if isAdmin {
			response[i] = mapUserToResponse(&u)
		} else {
			response[i] = mapUserToLimitedResponse(&u)
		}
	}

	h.RespondPaginated(w, response, pagination, total)
}

// Get handles GET /rest/api/v1/users/{id}
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "user ID")
	if !ok {
		return
	}

	u, err := h.userSvc.GetByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.RespondError(w, r, restapi.ErrUserNotFound)
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	isAdmin, _ := h.PermissionService.IsSystemAdmin(user.ID)
	if user.ID == id || isAdmin {
		h.RespondOK(w, mapUserToResponse(u))
	} else {
		h.RespondOK(w, mapUserToLimitedResponse(u))
	}
}

// GetCurrent handles GET /rest/api/v1/users/me
func (h *UserHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	u, err := h.userSvc.GetByID(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, mapUserToResponse(u))
}

// mapUserToLimitedResponse converts a models.User to UserResponse with sensitive fields stripped
func mapUserToLimitedResponse(u *models.User) UserResponse {
	return UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		FullName:  u.FullName,
		IsActive:  u.IsActive,
		AvatarURL: u.AvatarURL,
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// mapUserToResponse converts a models.User to UserResponse
func mapUserToResponse(u *models.User) UserResponse {
	return UserResponse{
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
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
