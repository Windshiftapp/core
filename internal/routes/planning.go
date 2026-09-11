package routes

import (
	"net/http"

	"windshift/internal/models"
)

// RegisterPlanningRoutes retains session-only catalog and personal-label APIs.
// Milestones and iterations use the canonical /api/v2 routes.
func RegisterPlanningRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	globalMilestoneManage := deps.PermissionMiddleware.RequireGlobalPermission(models.PermissionMilestoneCreate)
	globalIterationManage := deps.PermissionMiddleware.RequireGlobalPermission(models.PermissionIterationManage)

	api.HandleH("GET /milestone-categories", auth(http.HandlerFunc(deps.Planning.MilestoneCategory.GetAll)))
	api.HandleH("POST /milestone-categories", auth(globalMilestoneManage(http.HandlerFunc(deps.Planning.MilestoneCategory.Create))))
	api.HandleH("GET /milestone-categories/{id}", auth(http.HandlerFunc(deps.Planning.MilestoneCategory.Get)))
	api.HandleH("PUT /milestone-categories/{id}", auth(globalMilestoneManage(http.HandlerFunc(deps.Planning.MilestoneCategory.Update))))
	api.HandleH("DELETE /milestone-categories/{id}", auth(globalMilestoneManage(http.HandlerFunc(deps.Planning.MilestoneCategory.Delete))))

	api.HandleH("GET /iteration-types", auth(http.HandlerFunc(deps.Planning.IterationType.GetAll)))
	api.HandleH("POST /iteration-types", auth(globalIterationManage(http.HandlerFunc(deps.Planning.IterationType.Create))))
	api.HandleH("GET /iteration-types/{id}", auth(http.HandlerFunc(deps.Planning.IterationType.Get)))
	api.HandleH("PUT /iteration-types/{id}", auth(globalIterationManage(http.HandlerFunc(deps.Planning.IterationType.Update))))
	api.HandleH("DELETE /iteration-types/{id}", auth(globalIterationManage(http.HandlerFunc(deps.Planning.IterationType.Delete))))

	api.HandleH("GET /personal-labels", auth(http.HandlerFunc(deps.Planning.PersonalLabel.GetAll)))
	api.HandleH("POST /personal-labels", auth(http.HandlerFunc(deps.Planning.PersonalLabel.Create)))
	api.HandleH("GET /personal-labels/{id}", auth(http.HandlerFunc(deps.Planning.PersonalLabel.Get)))
	api.HandleH("PUT /personal-labels/{id}", auth(http.HandlerFunc(deps.Planning.PersonalLabel.Update)))
	api.HandleH("DELETE /personal-labels/{id}", auth(http.HandlerFunc(deps.Planning.PersonalLabel.Delete)))
	api.HandleH("GET /items/{id}/personal-labels", auth(http.HandlerFunc(deps.Planning.PersonalLabel.GetItemPersonalLabels)))
	api.HandleH("PUT /items/{id}/personal-labels", auth(http.HandlerFunc(deps.Planning.PersonalLabel.SetItemPersonalLabels)))
	api.HandleH("POST /items/{id}/personal-labels", auth(http.HandlerFunc(deps.Planning.PersonalLabel.AddItemPersonalLabel)))
	api.HandleH("DELETE /items/{id}/personal-labels/{labelId}", auth(http.HandlerFunc(deps.Planning.PersonalLabel.RemoveItemPersonalLabel)))
}
