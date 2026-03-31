package routes

import "net/http"

// RegisterTeamRoutes registers team, leave, and on-call routes.
func RegisterTeamRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth

	// Team CRUD
	api.HandleH("GET /teams", auth(http.HandlerFunc(deps.Teams.Team.GetAll)))
	api.HandleH("POST /teams", auth(http.HandlerFunc(deps.Teams.Team.Create)))
	api.HandleH("GET /teams/{id}", auth(http.HandlerFunc(deps.Teams.Team.Get)))
	api.HandleH("PUT /teams/{id}", auth(http.HandlerFunc(deps.Teams.Team.Update)))
	api.HandleH("DELETE /teams/{id}", auth(http.HandlerFunc(deps.Teams.Team.Delete)))

	// Team members
	api.HandleH("GET /teams/{id}/resolved-members", auth(http.HandlerFunc(deps.Teams.Team.GetResolvedMembers)))
	api.HandleH("POST /teams/{id}/members", auth(http.HandlerFunc(deps.Teams.Team.AddMembers)))
	api.HandleH("DELETE /teams/{id}/members", auth(http.HandlerFunc(deps.Teams.Team.RemoveMembers)))
	api.HandleH("PUT /teams/{id}/members/{userId}/role", auth(http.HandlerFunc(deps.Teams.Team.UpdateMemberRole)))

	// Team group mappings
	api.HandleH("POST /teams/{id}/groups", auth(http.HandlerFunc(deps.Teams.Team.AddGroups)))
	api.HandleH("DELETE /teams/{id}/groups", auth(http.HandlerFunc(deps.Teams.Team.RemoveGroups)))

	// User's teams
	api.HandleH("GET /users/{userId}/teams", auth(http.HandlerFunc(deps.Teams.Team.GetTeamsForUser)))

	// Leave periods
	api.HandleH("GET /users/{userId}/leave", auth(http.HandlerFunc(deps.Teams.Leave.GetForUser)))
	api.HandleH("POST /users/{userId}/leave", auth(http.HandlerFunc(deps.Teams.Leave.Create)))
	api.HandleH("PUT /users/{userId}/leave/{leaveId}", auth(http.HandlerFunc(deps.Teams.Leave.Update)))
	api.HandleH("DELETE /users/{userId}/leave/{leaveId}", auth(http.HandlerFunc(deps.Teams.Leave.Delete)))

	// On-call schedules
	api.HandleH("GET /teams/{id}/on-call/schedules", auth(http.HandlerFunc(deps.Teams.OnCall.ListSchedules)))
	api.HandleH("POST /teams/{id}/on-call/schedules", auth(http.HandlerFunc(deps.Teams.OnCall.CreateSchedule)))
	api.HandleH("GET /on-call/schedules/{id}", auth(http.HandlerFunc(deps.Teams.OnCall.GetSchedule)))
	api.HandleH("PUT /on-call/schedules/{id}", auth(http.HandlerFunc(deps.Teams.OnCall.UpdateSchedule)))
	api.HandleH("DELETE /on-call/schedules/{id}", auth(http.HandlerFunc(deps.Teams.OnCall.DeleteSchedule)))

	// On-call schedule layers
	api.HandleH("POST /on-call/schedules/{id}/layers", auth(http.HandlerFunc(deps.Teams.OnCall.AddLayer)))
	api.HandleH("PUT /on-call/schedules/{scheduleId}/layers/{layerId}", auth(http.HandlerFunc(deps.Teams.OnCall.UpdateLayer)))
	api.HandleH("DELETE /on-call/schedules/{scheduleId}/layers/{layerId}", auth(http.HandlerFunc(deps.Teams.OnCall.DeleteLayer)))
	api.HandleH("PUT /on-call/schedules/{scheduleId}/layers/{layerId}/members", auth(http.HandlerFunc(deps.Teams.OnCall.SetLayerMembers)))

	// On-call overrides
	api.HandleH("POST /on-call/schedules/{id}/overrides", auth(http.HandlerFunc(deps.Teams.OnCall.CreateOverride)))
	api.HandleH("DELETE /on-call/schedules/{scheduleId}/overrides/{overrideId}", auth(http.HandlerFunc(deps.Teams.OnCall.DeleteOverride)))

	// On-call current
	api.HandleH("GET /on-call/schedules/{id}/current", auth(http.HandlerFunc(deps.Teams.OnCall.GetCurrentOnCall)))

	// Swap requests
	api.HandleH("POST /on-call/schedules/{id}/swap-requests", auth(http.HandlerFunc(deps.Teams.OnCall.CreateSwapRequest)))
	api.HandleH("PUT /on-call/swap-requests/{id}", auth(http.HandlerFunc(deps.Teams.OnCall.RespondSwapRequest)))

	// Escalation policies
	api.HandleH("GET /teams/{id}/on-call/escalation-policies", auth(http.HandlerFunc(deps.Teams.OnCall.ListPolicies)))
	api.HandleH("POST /teams/{id}/on-call/escalation-policies", auth(http.HandlerFunc(deps.Teams.OnCall.CreatePolicy)))
	api.HandleH("GET /on-call/escalation-policies/{id}", auth(http.HandlerFunc(deps.Teams.OnCall.GetPolicy)))
	api.HandleH("PUT /on-call/escalation-policies/{id}", auth(http.HandlerFunc(deps.Teams.OnCall.UpdatePolicy)))
	api.HandleH("DELETE /on-call/escalation-policies/{id}", auth(http.HandlerFunc(deps.Teams.OnCall.DeletePolicy)))
	api.HandleH("PUT /on-call/escalation-policies/{id}/rules", auth(http.HandlerFunc(deps.Teams.OnCall.SetRules)))

	// Incidents
	api.HandleH("GET /on-call/incidents", auth(http.HandlerFunc(deps.Teams.OnCall.ListIncidents)))
	api.HandleH("POST /on-call/incidents/{id}/acknowledge", auth(http.HandlerFunc(deps.Teams.OnCall.AcknowledgeIncident)))
	api.HandleH("POST /on-call/incidents/{id}/resolve", auth(http.HandlerFunc(deps.Teams.OnCall.ResolveIncident)))
}
