package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// TimeProjectHandler serves the remaining customer-portal project query.
type TimeProjectHandler struct {
	projects              *repository.TimeProjectRepository
	timePermissionService *services.TimePermissionService
	customerOrgPermission *services.CustomerOrganisationPermissionService
}

func NewTimeProjectHandler(db database.Database, timePermissionService *services.TimePermissionService, customerOrgPermission *services.CustomerOrganisationPermissionService) *TimeProjectHandler {
	return &TimeProjectHandler{
		projects:              repository.NewTimeProjectRepository(db),
		timePermissionService: timePermissionService,
		customerOrgPermission: customerOrgPermission,
	}
}

func (h *TimeProjectHandler) GetByCustomer(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	customerID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	canView, err := h.customerOrgPermission.CanView(user.ID, customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}
	accessibleIDs, err := h.timePermissionService.GetAccessibleProjects(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	details, err := h.projects.ListDetailsFiltered(repository.TimeProjectListFilter{
		AccessibleIDs: accessibleIDs,
		CustomerID:    &customerID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	projects := make([]models.TimeProject, len(details))
	for i, detail := range details {
		projects[i] = models.TimeProject{
			ID: detail.ID, CustomerID: detail.CustomerID, CategoryID: detail.CategoryID,
			Name: detail.Name, Description: detail.Description, Status: detail.Status,
			Color: detail.Color, HourlyRate: detail.HourlyRate, Settings: detail.Settings,
			CreatedAt: detail.CreatedAt, UpdatedAt: detail.UpdatedAt, CustomerName: detail.CustomerName,
			CategoryName: detail.CategoryName, CategoryColor: detail.CategoryColor, TotalHours: detail.TotalHours,
		}
	}
	respondJSONOK(w, projects)
}
