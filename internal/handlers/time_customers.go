package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TimeCustomerHandler struct {
	repo                  *repository.CustomerOrganisationRepository
	auditor               *logger.Auditor
	timePermissionService *services.TimePermissionService
}

func NewTimeCustomerHandler(
	repo *repository.CustomerOrganisationRepository,
	auditor *logger.Auditor,
	timePermissionService *services.TimePermissionService,
) *TimeCustomerHandler {
	return &TimeCustomerHandler{
		repo:                  repo,
		auditor:               auditor,
		timePermissionService: timePermissionService,
	}
}

// checkCustomerPermission is a helper that checks if the user has customers.manage or project.manage permission
func (h *TimeCustomerHandler) checkCustomerPermission(w http.ResponseWriter, r *http.Request) (*models.User, bool) { //nolint:unparam // User return kept for future use
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, false
	}

	if h.timePermissionService != nil {
		hasPermission, err := h.timePermissionService.HasCustomersManagePermission(user.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return nil, false
		}
		if !hasPermission {
			respondForbidden(w, r)
			return nil, false
		}
	}

	return user, true
}

func (h *TimeCustomerHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	customers, err := h.repo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, customers)
}

func (h *TimeCustomerHandler) Get(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	c, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "customer")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, c)
}

func (h *TimeCustomerHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.checkCustomerPermission(w, r)
	if !ok {
		return
	}

	c, ok := decodeJSON[models.CustomerOrganisation](w, r)
	if !ok {
		return
	}

	c.Name = utils.SanitizeTitle(c.Name)
	c.Description = utils.SanitizeCommentContent(c.Description)

	id, now, err := h.repo.Create(&c)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	c.ID = id
	c.CreatedAt = now
	c.UpdatedAt = now

	if user != nil {
		h.auditor.Log(r, user, logger.ActionTimeCustomerCreate, logger.ResourceTimeCustomer, &id, c.Name)
	}

	respondJSONCreated(w, c)
}

func (h *TimeCustomerHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := h.checkCustomerPermission(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	c, ok := decodeJSON[models.CustomerOrganisation](w, r)
	if !ok {
		return
	}

	c.Name = utils.SanitizeTitle(c.Name)
	c.Description = utils.SanitizeCommentContent(c.Description)

	now, err := h.repo.Update(id, &c)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	c.ID = id
	c.UpdatedAt = now

	if user != nil {
		h.auditor.Log(r, user, logger.ActionTimeCustomerUpdate, logger.ResourceTimeCustomer, &id, c.Name)
	}

	respondJSONOK(w, c)
}

func (h *TimeCustomerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.checkCustomerPermission(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	projectCount, err := h.repo.CountTimeProjects(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if projectCount > 0 {
		respondValidationError(w, r, "Cannot delete customer with associated projects")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if user != nil {
		h.auditor.Log(r, user, logger.ActionTimeCustomerDelete, logger.ResourceTimeCustomer, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}
