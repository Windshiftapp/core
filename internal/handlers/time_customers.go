package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

type TimeCustomerHandler struct {
	repo                  *repository.CustomerOrganisationRepository
	auditor               *logger.Auditor
	timePermissionService *services.TimePermissionService
	customerOrgPermission *services.CustomerOrganisationPermissionService
}

func NewTimeCustomerHandler(
	repo *repository.CustomerOrganisationRepository,
	auditor *logger.Auditor,
	timePermissionService *services.TimePermissionService,
	customerOrgPermission *services.CustomerOrganisationPermissionService,
) *TimeCustomerHandler {
	return &TimeCustomerHandler{
		repo:                  repo,
		auditor:               auditor,
		timePermissionService: timePermissionService,
		customerOrgPermission: customerOrgPermission,
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
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	customers, err := h.repo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if h.customerOrgPermission != nil {
		accessibleIDs, err := h.customerOrgPermission.GetAccessible(user.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if accessibleIDs != nil {
			allowed := make(map[int]struct{}, len(accessibleIDs))
			for _, id := range accessibleIDs {
				allowed[id] = struct{}{}
			}
			filtered := customers[:0]
			for _, c := range customers {
				if _, ok := allowed[c.ID]; ok {
					filtered = append(filtered, c)
				}
			}
			customers = filtered
		}
	}

	respondJSONOK(w, customers)
}

func (h *TimeCustomerHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.customerOrgPermission != nil {
		canView, err := h.customerOrgPermission.CanView(user.ID, id)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canView {
			respondForbidden(w, r)
			return
		}
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
	// builtin_key is server-managed; never let a client create a system row.
	c.BuiltinKey = ""

	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &c.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &c.Description, Policy: sanitize.Comment, Label: "Description"},
	)

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

	respondJSONCreated(w, struct {
		models.CustomerOrganisation
		Warnings []string `json:"warnings,omitempty"`
	}{c, warnings})
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

	existing, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "customer")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if existing.BuiltinKey != "" {
		respondValidationError(w, r, "Built-in customers cannot be edited")
		return
	}

	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &c.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &c.Description, Policy: sanitize.Comment, Label: "Description"},
	)

	now, err := h.repo.Update(id, &c)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "customer")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	c.ID = id
	c.UpdatedAt = now

	if user != nil {
		h.auditor.Log(r, user, logger.ActionTimeCustomerUpdate, logger.ResourceTimeCustomer, &id, c.Name)
	}

	respondJSONOK(w, struct {
		models.CustomerOrganisation
		Warnings []string `json:"warnings,omitempty"`
	}{c, warnings})
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

	existing, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "customer")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if existing.BuiltinKey != "" {
		respondValidationError(w, r, "Built-in customers cannot be deleted")
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
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "customer")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if user != nil {
		h.auditor.Log(r, user, logger.ActionTimeCustomerDelete, logger.ResourceTimeCustomer, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// orgRequestSharingRequest is the body of PUT /customer-organisations/{id}/request-sharing.
type orgRequestSharingRequest struct {
	RequestSharing         string `json:"request_sharing"`
	RequestSharingAudience string `json:"request_sharing_audience"`
	VisibleRoleIDs         []int  `json:"visible_role_ids"`
}

// UpdateRequestSharing updates the organisation's portal request-sharing
// settings (WI-1139). Requires customers.manage, matching the rest of the
// customer-organisation admin surface.
func (h *TimeCustomerHandler) UpdateRequestSharing(w http.ResponseWriter, r *http.Request) {
	user, ok := h.checkCustomerPermission(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[orgRequestSharingRequest](w, r)
	if !ok {
		return
	}

	sharing := models.OrgRequestSharingSettings{
		RequestSharing:         req.RequestSharing,
		RequestSharingAudience: req.RequestSharingAudience,
		RequestVisibleRoleIDs:  req.VisibleRoleIDs,
	}
	if sharing.RequestSharing == "" {
		sharing.RequestSharing = models.OrgRequestSharingDisabled
	}
	switch sharing.RequestSharing {
	case models.OrgRequestSharingDisabled, models.OrgRequestSharingRequesterChoice, models.OrgRequestSharingAutomatic:
	default:
		respondValidationError(w, r, "invalid request_sharing")
		return
	}
	if sharing.RequestSharingAudience == "" {
		sharing.RequestSharingAudience = models.OrgRequestSharingAudienceOrganisation
	}
	switch sharing.RequestSharingAudience {
	case models.OrgRequestSharingAudienceOrganisation, models.OrgRequestSharingAudienceRoles:
	default:
		respondValidationError(w, r, "invalid request_sharing_audience")
		return
	}

	if sharing.RequestSharing == models.OrgRequestSharingDisabled {
		sharing.RequestSharingAudience = models.OrgRequestSharingAudienceOrganisation
		sharing.RequestVisibleRoleIDs = nil
	}
	if sharing.RequestSharingAudience == models.OrgRequestSharingAudienceRoles {
		requested := make(map[int]struct{}, len(sharing.RequestVisibleRoleIDs))
		for _, roleID := range sharing.RequestVisibleRoleIDs {
			requested[roleID] = struct{}{}
		}
		if len(requested) == 0 {
			respondValidationError(w, r, "request_visible_role_ids is required for the roles audience")
			return
		}
		existing, err := h.repo.ExistingContactRoleIDs(sharing.RequestVisibleRoleIDs)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if len(existing) != len(requested) {
			respondValidationError(w, r, "one or more request_visible_role_ids do not exist")
			return
		}
	} else {
		sharing.RequestVisibleRoleIDs = nil
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
	if c.BuiltinKey != "" {
		respondValidationError(w, r, "Built-in customers cannot be edited")
		return
	}

	merged := models.MergeOrgRequestSharingSettings(c.Settings, sharing)
	if _, err := h.repo.UpdateSettings(id, merged); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "customer")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if user != nil {
		h.auditor.Log(r, user, logger.ActionTimeCustomerUpdate, logger.ResourceTimeCustomer, &id, c.Name)
	}

	respondJSONOK(w, map[string]any{
		"request_sharing":          sharing.RequestSharing,
		"request_sharing_audience": sharing.RequestSharingAudience,
		"visible_role_ids":         sharing.RequestVisibleRoleIDs,
	})
}
