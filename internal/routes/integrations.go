package routes

import (
	"net/http"

	"windshift/internal/handlers"
)

// IntegrationHandlers groups integration-related handlers.
type IntegrationHandlers struct {
	Provider    *handlers.IntegrationProviderHandler
	OAuth       *handlers.IntegrationOAuthHandler
	ItemLinks   *handlers.IntegrationItemLinksHandler
	TodoistSync *handlers.TodoistSyncHandler
	Zammad      *handlers.ZammadHandler
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
	api.HandleH("POST /admin/integration-providers/{id}/oauth/start", admin(http.HandlerFunc(deps.Integrations.OAuth.StartSystemOAuth)))

	// OAuth flow
	api.HandleH("GET /integrations/oauth/{slug}/start", auth(http.HandlerFunc(deps.Integrations.OAuth.StartOAuth)))
	api.Handle("GET /integrations/oauth/{slug}/callback", http.HandlerFunc(deps.Integrations.OAuth.OAuthCallback))
	api.Handle("GET /integrations/oauth/system/{providerType}/callback", http.HandlerFunc(deps.Integrations.OAuth.SystemOAuthCallback))

	// User connections
	api.HandleH("GET /users/me/integration-connections", auth(http.HandlerFunc(deps.Integrations.OAuth.GetUserConnections)))
	api.HandleH("GET /users/me/integration-connections/available", auth(http.HandlerFunc(deps.Integrations.OAuth.GetAvailableProviders)))
	api.HandleH("DELETE /users/me/integration-connections/{provider_id}", auth(http.HandlerFunc(deps.Integrations.OAuth.DisconnectProvider)))

	// Todoist personal-task sync
	api.HandleH("GET /users/me/todoist-sync", auth(http.HandlerFunc(deps.Integrations.TodoistSync.GetSync)))
	api.HandleH("PUT /users/me/todoist-sync", auth(http.HandlerFunc(deps.Integrations.TodoistSync.UpdateSync)))
	api.HandleH("GET /users/me/todoist-sync/projects", auth(http.HandlerFunc(deps.Integrations.TodoistSync.GetProjects)))
	api.HandleH("POST /users/me/todoist-sync/run", auth(http.HandlerFunc(deps.Integrations.TodoistSync.RunSync)))

	// Zammad system connections and workspace-scoped ticket operations.
	api.HandleH("GET /admin/zammad-connections", admin(http.HandlerFunc(deps.Integrations.Zammad.ListConnections)))
	api.HandleH("POST /admin/zammad-connections", admin(http.HandlerFunc(deps.Integrations.Zammad.CreateConnection)))
	api.HandleH("GET /admin/zammad-connections/{id}", admin(http.HandlerFunc(deps.Integrations.Zammad.GetConnection)))
	api.HandleH("PUT /admin/zammad-connections/{id}", admin(http.HandlerFunc(deps.Integrations.Zammad.UpdateConnection)))
	api.HandleH("DELETE /admin/zammad-connections/{id}", admin(http.HandlerFunc(deps.Integrations.Zammad.DeleteConnection)))
	api.HandleH("POST /admin/zammad-connections/{id}/test", admin(http.HandlerFunc(deps.Integrations.Zammad.TestConnection)))
	api.HandleH("POST /admin/zammad-ticket-links/refresh", admin(http.HandlerFunc(deps.Integrations.Zammad.RefreshAllTickets)))
	api.HandleH("POST /admin/zammad-ticket-links/{linkId}/retry-create", admin(http.HandlerFunc(deps.Integrations.Zammad.RetryUncertainTicketCreation)))
	api.HandleH("POST /admin/zammad-ticket-links/{linkId}/detach-local", admin(http.HandlerFunc(deps.Integrations.Zammad.DetachTicketLinkLocally)))
	api.HandleH("GET /workspaces/{workspaceId}/zammad-connections", auth(http.HandlerFunc(deps.Integrations.Zammad.ListWorkspaceConnections)))
	api.HandleH("GET /workspaces/{workspaceId}/zammad-connections/{id}/metadata", auth(http.HandlerFunc(deps.Integrations.Zammad.GetWorkspaceMetadata)))
	api.HandleH("GET /workspaces/{workspaceId}/zammad-connections/{id}/owners", auth(http.HandlerFunc(deps.Integrations.Zammad.GetWorkspaceOwners)))
	api.HandleH("GET /items/{id}/zammad-links", auth(http.HandlerFunc(deps.Integrations.Zammad.GetItemLinks)))
	api.HandleH("GET /zammad-ticket-links/resolve/{correlationKey}", auth(http.HandlerFunc(deps.Integrations.Zammad.ResolveTicketLink)))
	api.HandleH("POST /items/{id}/zammad-tickets", auth(http.HandlerFunc(deps.Integrations.Zammad.CreateTicket)))
	api.HandleH("POST /items/{id}/zammad-ticket-links", auth(http.HandlerFunc(deps.Integrations.Zammad.LinkExistingTicket)))
	api.HandleH("PUT /zammad-ticket-links/{linkId}", auth(http.HandlerFunc(deps.Integrations.Zammad.UpdateTicketLink)))
	api.HandleH("DELETE /zammad-ticket-links/{linkId}", auth(http.HandlerFunc(deps.Integrations.Zammad.DeleteTicketLink)))
	api.HandleH("POST /zammad-ticket-links/{linkId}/refresh", auth(http.HandlerFunc(deps.Integrations.Zammad.RefreshTicket)))

	// Item links
	api.HandleH("GET /items/{id}/integration-links", auth(http.HandlerFunc(deps.Integrations.ItemLinks.GetItemLinks)))
	api.HandleH("POST /items/{id}/integration-links", auth(http.HandlerFunc(deps.Integrations.ItemLinks.CreateItemLink)))
	api.HandleH("DELETE /item-integration-links/{linkId}", auth(http.HandlerFunc(deps.Integrations.ItemLinks.DeleteItemLink)))
	api.HandleH("POST /item-integration-links/{linkId}/refresh", auth(http.HandlerFunc(deps.Integrations.ItemLinks.RefreshItemLink)))
	api.HandleH("GET /items/{id}/integration-search", auth(http.HandlerFunc(deps.Integrations.ItemLinks.SearchPages)))
}
