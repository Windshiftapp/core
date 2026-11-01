package routes

import (
	"net/http"
)

// RegisterCustomWidgetRoutes registers custom dashboard widget endpoints.
// Validation is registered before the wildcard so /custom-widgets/validate is
// never parsed as an ID.
func RegisterCustomWidgetRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	handler := deps.Widgets.Custom

	api.HandleH("GET /custom-widgets", auth(http.HandlerFunc(handler.GetCustomWidgets)))
	api.HandleH("POST /custom-widgets", auth(http.HandlerFunc(handler.CreateCustomWidget)))
	api.HandleH("POST /custom-widgets/validate", auth(http.HandlerFunc(handler.ValidateCustomWidget)))
	// Generation calls an LLM and produces a persisted artifact, so it uses a
	// tighter per-user bucket than the general AI limiter.
	api.HandleH("POST /custom-widgets/generate",
		auth(deps.WidgetGenerateLimiter.Limit(http.HandlerFunc(deps.Widgets.Generate.GenerateCustomWidget))))
	api.HandleH("GET /custom-widgets/{id}", auth(http.HandlerFunc(handler.GetCustomWidget)))
	api.HandleH("PUT /custom-widgets/{id}", auth(http.HandlerFunc(handler.UpdateCustomWidget)))
	api.HandleH("DELETE /custom-widgets/{id}", auth(http.HandlerFunc(handler.DeleteCustomWidget)))
}
