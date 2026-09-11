package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/restapi"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// AdminUserHandler handles admin user management in REST API v1.
type AdminUserHandler struct {
	BaseHandler
	userSvc *services.UserReadService
}

// NewAdminUserHandler creates a new admin user handler.
func NewAdminUserHandler(db database.Database, permissionService *services.PermissionService) *AdminUserHandler {
	return &AdminUserHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		userSvc:     services.NewUserReadService(db),
	}
}

// AdminUserResponse is the admin representation of a user.
//
// Warnings carries user-facing strings (frontend toasts at info
// severity) for any field the handler had to sanitize at decode time.
// Empty / omitted when nothing was modified.
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
	Warnings  []string `json:"warnings,omitempty"`
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

	users, total, err := h.userSvc.ListAdmin(services.PaginationParams{
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
	actor, ok := h.RequireAuth(w, r)
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
	// Email is intentionally not sanitized — it's format-validated as an
	// email address by the user service before write; running it through
	// PlainTextField would silently turn "<script>@evil.com" into
	// "@evil.com" rather than rejecting it as malformed.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: req.FirstName, Policy: sanitize.PlainTextField, Label: "First name"},
		sanitize.Pair{Target: req.LastName, Policy: sanitize.PlainTextField, Label: "Last name"},
	)

	update := services.AdminUserUpdate{
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Email:     req.Email,
		IsActive:  req.IsActive,
	}
	if update.IsEmpty() {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "No fields to update"))
		return
	}
	result, err := h.userSvc.UpdateAdmin(id, update)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrUserNotFound):
			h.RespondError(w, r, restapi.ErrUserNotFound)
		case errors.Is(err, services.ErrUserManagedExternally):
			h.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, restapi.ErrCodeForbidden, "User is managed externally"))
		case errors.Is(err, services.ErrUserEmailInvalid):
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid email"))
		case errors.Is(err, services.ErrUserEmailExists), errors.Is(err, services.ErrUserUsernameExists):
			h.RespondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, err.Error()))
		default:
			h.RespondInternalError(w, r)
		}
		return
	}
	before := result.Before
	u := result.User

	resp := mapUserToResponse(u)
	resp.Warnings = warnings
	changes := map[string]any{}
	if before.FirstName != u.FirstName {
		changes["first_name"] = map[string]string{"old": before.FirstName, "new": u.FirstName}
	}
	if before.LastName != u.LastName {
		changes["last_name"] = map[string]string{"old": before.LastName, "new": u.LastName}
	}
	if before.Email != u.Email {
		changes["email"] = map[string]string{"old": before.Email, "new": u.Email}
	}
	if before.IsActive != u.IsActive {
		changes["is_active"] = map[string]bool{"old": before.IsActive, "new": u.IsActive}
	}
	h.Auditor.LogWithDetails(r, actor, logger.ActionUserUpdate, logger.ResourceUser, &u.ID, u.Username, changes)
	h.RespondOK(w, resp)
}

func (h *AdminUserHandler) getUserGroupIDs(userID int) []int {
	ids, err := h.userSvc.GetGroupIDs(userID)
	if err != nil {
		return []int{}
	}
	return ids
}
