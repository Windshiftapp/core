package routes

import (
	"net/http"

	"windshift/internal/handlers"
)

// IntegrationHandlers groups integration-related handlers.
type IntegrationHandlers struct {
	Provider  *handlers.IntegrationProviderHandler
	OAuth     *handlers.IntegrationOAuthHandler
	ItemLinks *handlers.IntegrationItemLinksHandler
}

// RegisterIntegrationRoutes registers integration provider, OAuth, and item link routes.
func RegisterIntegrationRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	// Admin provider management
	api.HandleH("GET /admin/integration-providers", admin(http.HandlerFunc(deps.Integrations.Provider.GetProviders)))
	api.HandleH("POST /admin/integration-providers", admin(http.HandlerFunc(deps.Integrations.Provider.CreateProvider)))
	api.HandleH("GET /admin/integration-providers/{id}", admin(http.HandlerFunc(deps.Integrations.Provider.GetProvider)))
	api.HandleH("PUT /admin/integration-providers/{id}", admin(http.HandlerFunc(deps.Integrations.Provider.UpdateProvider)))
	api.HandleH("DELETE /admin/integration-providers/{id}", admin(http.HandlerFunc(deps.Integrations.Provider.DeleteProvider)))

	// OAuth flow
	api.HandleH("GET /integrations/oauth/{slug}/start", auth(http.HandlerFunc(deps.Integrations.OAuth.StartOAuth)))
	api.Handle("GET /integrations/oauth/{slug}/callback", http.HandlerFunc(deps.Integrations.OAuth.OAuthCallback))

	// User connections
	api.HandleH("GET /users/me/integration-connections", auth(http.HandlerFunc(deps.Integrations.OAuth.GetUserConnections)))
	api.HandleH("GET /users/me/integration-connections/available", auth(http.HandlerFunc(deps.Integrations.OAuth.GetAvailableProviders)))
	api.HandleH("DELETE /users/me/integration-connections/{provider_id}", auth(http.HandlerFunc(deps.Integrations.OAuth.DisconnectProvider)))

	// Item links
	api.HandleH("GET /items/{id}/integration-links", auth(http.HandlerFunc(deps.Integrations.ItemLinks.GetItemLinks)))
	api.HandleH("POST /items/{id}/integration-links", auth(http.HandlerFunc(deps.Integrations.ItemLinks.CreateItemLink)))
	api.HandleH("DELETE /item-integration-links/{linkId}", auth(http.HandlerFunc(deps.Integrations.ItemLinks.DeleteItemLink)))
	api.HandleH("POST /item-integration-links/{linkId}/refresh", auth(http.HandlerFunc(deps.Integrations.ItemLinks.RefreshItemLink)))
	api.HandleH("GET /items/{id}/integration-search", auth(http.HandlerFunc(deps.Integrations.ItemLinks.SearchPages)))
}
