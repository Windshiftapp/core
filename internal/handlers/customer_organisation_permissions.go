package handlers

import (
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
)

// CustomerOrganisationPermissionHandler handles per-org member/manager CRUD.
// Mirrors TimeProjectPermissionHandler.
type CustomerOrganisationPermissionHandler struct {
	auditor               *logger.Auditor
	customerOrgPermission *services.CustomerOrganisationPermissionService
}

func NewCustomerOrganisationPermissionHandler(auditor *logger.Auditor, customerOrgPermission *services.CustomerOrganisationPermissionService) *CustomerOrganisationPermissionHandler {
	return &CustomerOrganisationPermissionHandler{
		auditor:               auditor,
		customerOrgPermission: customerOrgPermission,
	}
}

func (h *CustomerOrganisationPermissionHandler) requireViewAccess(w http.ResponseWriter, r *http.Request) (int, bool) {
	orgID, ok := requireIDParam(w, r, "id")
	if !ok {
		return 0, false
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return 0, false
	}

	canView, err := h.customerOrgPermission.CanView(user.ID, orgID)
	if err != nil {
		respondInternalError(w, r, err)
		return 0, false
	}
	if !canView {
		respondForbidden(w, r)
		return 0, false
	}
	return orgID, true
}

func (h *CustomerOrganisationPermissionHandler) requireManagerAccess(w http.ResponseWriter, r *http.Request) (int, *models.User, bool) {
	orgID, ok := requireIDParam(w, r, "id")
	if !ok {
		return 0, nil, false
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return 0, nil, false
	}

	isManager, err := h.customerOrgPermission.IsManager(user.ID, orgID)
	if err != nil {
		respondInternalError(w, r, err)
		return 0, nil, false
	}
	if !isManager {
		respondForbidden(w, r)
		return 0, nil, false
	}

	return orgID, user, true
}

// GetMembers returns all members for an org. Requires CanView.
func (h *CustomerOrganisationPermissionHandler) GetMembers(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.requireViewAccess(w, r)
	if !ok {
		return
	}

	members, err := h.customerOrgPermission.GetMembers(orgID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if members == nil {
		members = []models.CustomerOrganisationMember{}
	}
	respondJSONOK(w, members)
}

// AddMember inserts a member. Requires manager rights.
func (h *CustomerOrganisationPermissionHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	orgID, user, ok := h.requireManagerAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.CustomerOrganisationMemberRequest](w, r)
	if !ok {
		return
	}
	if req.MemberType != "user" && req.MemberType != "group" {
		respondValidationError(w, r, "member_type must be 'user' or 'group'")
		return
	}

	member, err := h.customerOrgPermission.AddMember(orgID, req.MemberType, req.MemberID, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.auditor.LogWithDetails(r, user, logger.ActionCustomerOrgAddMember, logger.ResourceTimeCustomer, &orgID, "", map[string]interface{}{"member_id": req.MemberID})
	respondJSONCreated(w, member)
}

// RemoveMember deletes a member by id. Requires manager rights.
func (h *CustomerOrganisationPermissionHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	orgID, user, ok := h.requireManagerAccess(w, r)
	if !ok {
		return
	}

	memberID, ok := requireIDParam(w, r, "memberId")
	if !ok {
		return
	}

	members, err := h.customerOrgPermission.GetMembers(orgID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	found := false
	for _, m := range members {
		if m.ID == memberID {
			found = true
			break
		}
	}
	if !found {
		respondNotFound(w, r, "member")
		return
	}

	if err := h.customerOrgPermission.RemoveMember(memberID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.auditor.LogWithDetails(r, user, logger.ActionCustomerOrgRemoveMember, logger.ResourceTimeCustomer, &orgID, "", map[string]interface{}{"member_id": memberID})
	w.WriteHeader(http.StatusNoContent)
}

// GetManagers returns all managers for an org. Requires CanView.
func (h *CustomerOrganisationPermissionHandler) GetManagers(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.requireViewAccess(w, r)
	if !ok {
		return
	}

	managers, err := h.customerOrgPermission.GetManagers(orgID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if managers == nil {
		managers = []models.CustomerOrganisationManager{}
	}
	respondJSONOK(w, managers)
}

// AddManager inserts a manager. Requires global customers.manage OR being a manager already.
// Mirrors the TimeProject pattern: bootstrapping the first manager needs the global permission.
func (h *CustomerOrganisationPermissionHandler) AddManager(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	hasGlobalManage, err := h.customerOrgPermission.HasCustomersManagePermission(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !hasGlobalManage {
		isManager, err := h.customerOrgPermission.IsManager(user.ID, orgID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !isManager {
			respondForbidden(w, r)
			return
		}
	}

	req, ok := decodeJSON[models.CustomerOrganisationManagerRequest](w, r)
	if !ok {
		return
	}
	if req.ManagerType != "user" && req.ManagerType != "group" {
		respondValidationError(w, r, "manager_type must be 'user' or 'group'")
		return
	}

	manager, err := h.customerOrgPermission.AddManager(orgID, req.ManagerType, req.ManagerID, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.auditor.LogWithDetails(r, user, logger.ActionCustomerOrgAddManager, logger.ResourceTimeCustomer, &orgID, "", map[string]interface{}{"manager_id": req.ManagerID})
	respondJSONCreated(w, manager)
}

// RemoveManager deletes a manager by id. Only global customers.manage can remove managers
// (avoids managers locking each other out — matches the time-project rule).
func (h *CustomerOrganisationPermissionHandler) RemoveManager(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	managerID, ok := requireIDParam(w, r, "managerId")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	hasGlobalManage, err := h.customerOrgPermission.HasCustomersManagePermission(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !hasGlobalManage {
		respondForbidden(w, r)
		return
	}

	managers, err := h.customerOrgPermission.GetManagers(orgID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	found := false
	for _, m := range managers {
		if m.ID == managerID {
			found = true
			break
		}
	}
	if !found {
		respondNotFound(w, r, "manager")
		return
	}

	if err := h.customerOrgPermission.RemoveManager(managerID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.auditor.LogWithDetails(r, user, logger.ActionCustomerOrgRemoveManager, logger.ResourceTimeCustomer, &orgID, "", map[string]interface{}{"manager_id": managerID})
	w.WriteHeader(http.StatusNoContent)
}
