package routes

import "net/http"

// RegisterPlanningRoutes registers planning-related routes (milestones, iterations, labels).
func RegisterPlanningRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth

	// Milestone Category endpoints
	api.HandleH("GET /milestone-categories", auth(http.HandlerFunc(deps.Planning.MilestoneCategory.GetAll)))
	api.HandleH("POST /milestone-categories", auth(http.HandlerFunc(deps.Planning.MilestoneCategory.Create)))
	api.HandleH("GET /milestone-categories/{id}", auth(http.HandlerFunc(deps.Planning.MilestoneCategory.Get)))
	api.HandleH("PUT /milestone-categories/{id}", auth(http.HandlerFunc(deps.Planning.MilestoneCategory.Update)))
	api.HandleH("DELETE /milestone-categories/{id}", auth(http.HandlerFunc(deps.Planning.MilestoneCategory.Delete)))

	// Milestone endpoints
	api.HandleH("GET /milestones", auth(http.HandlerFunc(deps.Planning.Milestone.GetAll)))
	api.HandleH("POST /milestones", auth(http.HandlerFunc(deps.Planning.Milestone.Create)))
	api.HandleH("GET /milestones/{id}", auth(http.HandlerFunc(deps.Planning.Milestone.Get)))
	api.HandleH("PUT /milestones/{id}", auth(http.HandlerFunc(deps.Planning.Milestone.Update)))
	api.HandleH("DELETE /milestones/{id}", auth(http.HandlerFunc(deps.Planning.Milestone.Delete)))
	api.HandleH("GET /milestones/{id}/test-statistics", auth(http.HandlerFunc(deps.Planning.Milestone.GetTestStatistics)))
	api.HandleH("GET /milestones/{id}/progress", auth(http.HandlerFunc(deps.Planning.Milestone.GetProgress)))

	// Iteration type endpoints
	api.HandleH("GET /iteration-types", auth(http.HandlerFunc(deps.Planning.IterationType.GetAll)))
	api.HandleH("POST /iteration-types", auth(http.HandlerFunc(deps.Planning.IterationType.Create)))
	api.HandleH("GET /iteration-types/{id}", auth(http.HandlerFunc(deps.Planning.IterationType.Get)))
	api.HandleH("PUT /iteration-types/{id}", auth(http.HandlerFunc(deps.Planning.IterationType.Update)))
	api.HandleH("DELETE /iteration-types/{id}", auth(http.HandlerFunc(deps.Planning.IterationType.Delete)))

	// Iteration endpoints
	api.HandleH("GET /iterations", auth(http.HandlerFunc(deps.Planning.Iteration.GetAll)))
	api.HandleH("POST /iterations", auth(http.HandlerFunc(deps.Planning.Iteration.Create)))
	api.HandleH("GET /iterations/{id}", auth(http.HandlerFunc(deps.Planning.Iteration.Get)))
	api.HandleH("PUT /iterations/{id}", auth(http.HandlerFunc(deps.Planning.Iteration.Update)))
	api.HandleH("DELETE /iterations/{id}", auth(http.HandlerFunc(deps.Planning.Iteration.Delete)))
	api.HandleH("GET /iterations/{id}/progress", auth(http.HandlerFunc(deps.Planning.Iteration.GetProgress)))

	// Personal label endpoints
	api.HandleH("GET /personal-labels", auth(http.HandlerFunc(deps.Planning.PersonalLabel.GetAll)))
	api.HandleH("POST /personal-labels", auth(http.HandlerFunc(deps.Planning.PersonalLabel.Create)))
	api.HandleH("GET /personal-labels/{id}", auth(http.HandlerFunc(deps.Planning.PersonalLabel.Get)))
	api.HandleH("PUT /personal-labels/{id}", auth(http.HandlerFunc(deps.Planning.PersonalLabel.Update)))
	api.HandleH("DELETE /personal-labels/{id}", auth(http.HandlerFunc(deps.Planning.PersonalLabel.Delete)))
}
