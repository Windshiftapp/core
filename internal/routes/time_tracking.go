package routes

import "net/http"

// RegisterTimeTrackingRoutes registers time tracking routes.
func RegisterTimeTrackingRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth


	// Time project categories
	api.HandleH("GET /time/project-categories", auth(http.HandlerFunc(deps.TimeTracking.ProjectCategory.GetCategories)))
	api.HandleH("POST /time/project-categories", auth(http.HandlerFunc(deps.TimeTracking.ProjectCategory.CreateCategory)))
	api.HandleH("GET /time/project-categories/{id}", auth(http.HandlerFunc(deps.TimeTracking.ProjectCategory.GetCategory)))
	api.HandleH("PUT /time/project-categories/{id}", auth(http.HandlerFunc(deps.TimeTracking.ProjectCategory.UpdateCategory)))
	api.HandleH("DELETE /time/project-categories/{id}", auth(http.HandlerFunc(deps.TimeTracking.ProjectCategory.DeleteCategory)))
	api.HandleH("POST /time/project-categories/reorder", auth(http.HandlerFunc(deps.TimeTracking.ProjectCategory.ReorderCategories)))

	// Time projects
	api.HandleH("GET /time/projects", auth(http.HandlerFunc(deps.TimeTracking.Project.GetAll)))
	api.HandleH("POST /time/projects", auth(http.HandlerFunc(deps.TimeTracking.Project.Create)))
	api.HandleH("GET /time/projects/{id}", auth(http.HandlerFunc(deps.TimeTracking.Project.Get)))
	api.HandleH("PUT /time/projects/{id}", auth(http.HandlerFunc(deps.TimeTracking.Project.Update)))
	api.HandleH("DELETE /time/projects/{id}", auth(http.HandlerFunc(deps.TimeTracking.Project.Delete)))
	api.HandleH("GET /time/projects/{id}/worklogs", auth(http.HandlerFunc(deps.TimeTracking.Worklog.GetByProject)))

	// Time project permissions (managers and members)
	api.HandleH("GET /time/projects/{id}/managers", auth(http.HandlerFunc(deps.TimeTracking.ProjectPermission.GetManagers)))
	api.HandleH("POST /time/projects/{id}/managers", auth(http.HandlerFunc(deps.TimeTracking.ProjectPermission.AddManager)))
	api.HandleH("DELETE /time/projects/{id}/managers/{managerId}", auth(http.HandlerFunc(deps.TimeTracking.ProjectPermission.RemoveManager)))
	api.HandleH("GET /time/projects/{id}/members", auth(http.HandlerFunc(deps.TimeTracking.ProjectPermission.GetMembers)))
	api.HandleH("POST /time/projects/{id}/members", auth(http.HandlerFunc(deps.TimeTracking.ProjectPermission.AddMember)))
	api.HandleH("DELETE /time/projects/{id}/members/{memberId}", auth(http.HandlerFunc(deps.TimeTracking.ProjectPermission.RemoveMember)))

	// Time worklogs
	api.HandleH("GET /time/worklogs", auth(http.HandlerFunc(deps.TimeTracking.Worklog.GetAll)))
	api.HandleH("POST /time/worklogs", auth(http.HandlerFunc(deps.TimeTracking.Worklog.Create)))
	api.HandleH("GET /time/worklogs/{id}", auth(http.HandlerFunc(deps.TimeTracking.Worklog.Get)))
	api.HandleH("PUT /time/worklogs/{id}", auth(http.HandlerFunc(deps.TimeTracking.Worklog.Update)))
	api.HandleH("DELETE /time/worklogs/{id}", auth(http.HandlerFunc(deps.TimeTracking.Worklog.Delete)))

	// Active timer endpoints
	api.HandleH("POST /timer/start", auth(http.HandlerFunc(deps.TimeTracking.ActiveTimer.StartTimer)))
	api.HandleH("GET /timer/active", auth(http.HandlerFunc(deps.TimeTracking.ActiveTimer.GetActiveTimer)))
	api.HandleH("DELETE /timer/{id}/stop", auth(http.HandlerFunc(deps.TimeTracking.ActiveTimer.StopTimer)))
}
