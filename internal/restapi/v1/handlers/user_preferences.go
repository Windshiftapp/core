package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// UserPreferencesHandler serves the bearer-token owner's own preferences
// sub-documents. There is deliberately no user id in the URL — a token can
// only read/write its own user's preferences.
type UserPreferencesHandler struct {
	BaseHandler
	prefsSvc *services.UserPreferencesService
}

// NewUserPreferencesHandler creates a new user-preferences handler.
func NewUserPreferencesHandler(db database.Database, permissionService *services.PermissionService) *UserPreferencesHandler {
	return &UserPreferencesHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		prefsSvc: services.NewUserPreferencesService(
			repository.NewUserPreferencesRepository(db),
			repository.NewThemeRepository(db),
			permissionService,
		),
	}
}

// GetTUI handles GET /rest/api/v1/users/me/tui-preferences
//
// @Summary      Get the authenticated user's TUI preferences
// @Description  Returns the SSH TUI preferences sub-document for the bearer token's owning user. Empty fields mean "unset" — the TUI applies its defaults.
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  models.UserTUIPreferences
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the user-preferences:read scope"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /users/me/tui-preferences [get]
func (h *UserPreferencesHandler) GetTUI(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	tui, err := h.prefsSvc.GetTUI(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, tui)
}

// UpdateTUI handles PUT /rest/api/v1/users/me/tui-preferences
//
// @Summary      Replace the authenticated user's TUI preferences
// @Description  Full replace of the SSH TUI preferences sub-document (theme, split ratio, last workspace). Values are normalized server-side (theme truncated to 64 chars, split ratio clamped to [0.1, 0.9]); other preference sub-documents are untouched.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        preferences  body      models.UserTUIPreferences  true  "TUI preferences"
// @Success      200  {object}  models.UserTUIPreferences
// @Failure      400  {object}  handlers.ErrorResponse  "Malformed body"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the user-preferences:write scope"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /users/me/tui-preferences [put]
func (h *UserPreferencesHandler) UpdateTUI(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	var req models.UserTUIPreferences
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}

	if err := h.prefsSvc.UpdateTUI(user.ID, req); err != nil {
		h.RespondInternalError(w, r)
		return
	}

	tui, err := h.prefsSvc.GetTUI(user.ID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, tui)
}
