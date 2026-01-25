package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
)

// UserHandler handles public API requests for users
type UserHandler struct {
	permissionService *services.PermissionService
	userSvc           *services.UserReadService
}

// NewUserHandler creates a new user handler
func NewUserHandler(db database.Database, permissionService *services.PermissionService) *UserHandler {
	return &UserHandler{
		permissionService: permissionService,
		userSvc:           services.NewUserReadService(db),
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
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	// Check user.list permission
	hasPermission, _ := h.permissionService.HasGlobalPermission(user.ID, models.PermissionUserList)
	if !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", "user.list permission required"))
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

	// Map to response DTOs
	response := make([]UserResponse, len(users))
	for i, u := range users {
		response[i] = mapUserToResponse(&u)
	}

	restapi.RespondPaginated(w, response, restapi.NewPaginationMeta(pagination, total))
}

// Get handles GET /rest/api/v1/users/{id}
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid user ID"))
		return
	}

	u, err := h.userSvc.GetByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			restapi.RespondError(w, r, restapi.ErrUserNotFound)
			return
		}
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondOK(w, mapUserToResponse(u))
}

// GetCurrent handles GET /rest/api/v1/users/me
func (h *UserHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	u, err := h.userSvc.GetByID(user.ID)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondOK(w, mapUserToResponse(u))
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
