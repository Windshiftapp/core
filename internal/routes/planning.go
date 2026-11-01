package routes

import (
	"net/http"

	"windshift/internal/models"
)

// RegisterPlanningRoutes retains session-only catalog APIs. Milestones and
// iterations use the canonical /api/v2 routes.
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
}
