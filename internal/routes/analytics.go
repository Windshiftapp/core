package routes

import "net/http"

// RegisterAnalyticsRoutes registers workspace analytics routes.
func RegisterAnalyticsRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth

	api.HandleH("GET /workspaces/{id}/analytics", auth(http.HandlerFunc(deps.Workspaces.Analytics.GetAnalytics)))
}
