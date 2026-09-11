package routes

import "net/http"

// RegisterAssetRoutes registers asset management routes.
func RegisterAssetRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth

	// Asset Actions (Automations)
	api.HandleH("GET /asset-sets/{setId}/actions", auth(http.HandlerFunc(deps.Assets.Action.ListActions)))
	api.HandleH("POST /asset-sets/{setId}/actions", auth(http.HandlerFunc(deps.Assets.Action.CreateAction)))
	api.HandleH("GET /asset-sets/{setId}/actions/{id}", auth(http.HandlerFunc(deps.Assets.Action.GetAction)))
	api.HandleH("PUT /asset-sets/{setId}/actions/{id}", auth(http.HandlerFunc(deps.Assets.Action.UpdateAction)))
	api.HandleH("DELETE /asset-sets/{setId}/actions/{id}", auth(http.HandlerFunc(deps.Assets.Action.DeleteAction)))
	api.HandleH("POST /asset-sets/{setId}/actions/{id}/toggle", auth(http.HandlerFunc(deps.Assets.Action.ToggleAction)))
	api.HandleH("POST /asset-sets/{setId}/actions/{id}/execute", auth(http.HandlerFunc(deps.Assets.Action.ExecuteAction)))
	api.HandleH("GET /asset-sets/{setId}/actions/{id}/logs", auth(http.HandlerFunc(deps.Assets.Action.GetActionLogs)))
	api.HandleH("GET /asset-sets/{setId}/action-logs", auth(http.HandlerFunc(deps.Assets.Action.GetSetLogs)))
}
