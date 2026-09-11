package handlers

import (
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type GroupHandler struct {
	repo              *repository.GroupRepository
	permissionService *services.PermissionService
	application       *services.GroupApplicationService
}

func NewGroupHandler(repo *repository.GroupRepository, permissionService *services.PermissionService, _ *logger.Auditor, invalidators ...*services.AuthorizationCacheInvalidator) *GroupHandler {
	invalidator := services.NewAuthorizationCacheInvalidator(permissionService, nil)
	if len(invalidators) > 0 && invalidators[0] != nil {
		invalidator = invalidators[0]
	}
	return &GroupHandler{
		repo: repo, permissionService: permissionService,
		application: services.NewGroupApplicationService(repo, invalidator),
	}
}

func (h *GroupHandler) Application() *services.GroupApplicationService { return h.application }

func (h *GroupHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	groups, err := h.application.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, groups)
}

func (h *GroupHandler) GetUserMemberships(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	// Check if user exists
	userExists, err := h.repo.UserExists(userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !userExists {
		respondNotFound(w, r, "user")
		return
	}

	groups, err := h.repo.ListUserMemberships(userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if groups == nil {
		groups = []models.TeamGroup{}
	}

	response := models.TeamGroupMembershipResponse{
		UserID: userID,
		Groups: groups,
	}

	respondJSONOK(w, response)
}
