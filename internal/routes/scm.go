package routes

import (
	"net/http"

	"windshift/internal/models"
)

// RegisterSCMRoutes registers source code management routes (providers, workspace connections, item links).
func RegisterSCMRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	// SCM Provider endpoints
	api.HandleH("GET /admin/scm-providers", admin(http.HandlerFunc(deps.SCM.Provider.GetProviders)))
	api.HandleH("POST /admin/scm-providers", admin(http.HandlerFunc(deps.SCM.Provider.CreateProvider)))
	api.HandleH("GET /admin/scm-providers/{id}", admin(http.HandlerFunc(deps.SCM.Provider.GetProvider)))
	api.HandleH("PUT /admin/scm-providers/{id}", admin(http.HandlerFunc(deps.SCM.Provider.UpdateProvider)))
	api.HandleH("DELETE /admin/scm-providers/{id}", admin(http.HandlerFunc(deps.SCM.Provider.DeleteProvider)))
	api.HandleH("POST /admin/scm-providers/{id}/test", admin(http.HandlerFunc(deps.SCM.Provider.TestProvider)))

	// SCM Provider workspace allowlist endpoints
	api.HandleH("GET /admin/scm-providers/{id}/allowed-workspaces", admin(http.HandlerFunc(deps.SCM.Provider.GetProviderAllowedWorkspaces)))
	api.HandleH("PUT /admin/scm-providers/{id}/allowed-workspaces", admin(http.HandlerFunc(deps.SCM.Provider.UpdateProviderAllowedWorkspaces)))
	api.HandleH("POST /admin/scm-providers/{id}/allowed-workspaces", admin(http.HandlerFunc(deps.SCM.Provider.AddWorkspaceToProviderAllowlist)))
	api.HandleH("DELETE /admin/scm-providers/{id}/allowed-workspaces/{workspace_id}", admin(http.HandlerFunc(deps.SCM.Provider.RemoveWorkspaceFromProviderAllowlist)))

	// SCM GitHub App discovery endpoints
	api.HandleH("POST /admin/scm-providers/github-app/discover-installations", admin(http.HandlerFunc(deps.SCM.Provider.DiscoverGitHubAppInstallations)))
	api.HandleH("POST /admin/scm-providers/{id}/github-app/refresh-installation", admin(http.HandlerFunc(deps.SCM.Provider.RefreshGitHubAppInstallation)))

	// SCM OAuth endpoints
	api.HandleH("GET /scm/oauth/{slug}/start", auth(http.HandlerFunc(deps.SCM.Provider.StartOAuth)))
	api.Handle("GET /scm/oauth/{slug}/callback", deps.SCM.Provider.OAuthCallback)

	// Email Provider endpoints
	api.HandleH("GET /admin/email-providers", admin(http.HandlerFunc(deps.SCM.EmailProvider.GetEmailProviders)))
	api.HandleH("POST /admin/email-providers", admin(http.HandlerFunc(deps.SCM.EmailProvider.CreateEmailProvider)))
	api.HandleH("GET /admin/email-providers/{id}", admin(http.HandlerFunc(deps.SCM.EmailProvider.GetEmailProvider)))
	api.HandleH("PUT /admin/email-providers/{id}", admin(http.HandlerFunc(deps.SCM.EmailProvider.UpdateEmailProvider)))
	api.HandleH("DELETE /admin/email-providers/{id}", admin(http.HandlerFunc(deps.SCM.EmailProvider.DeleteEmailProvider)))
	api.HandleH("POST /channels/{channel_id}/email-oauth/start", auth(http.HandlerFunc(deps.SCM.EmailProvider.StartEmailOAuth)))
	api.Handle("GET /email/oauth/{provider_slug}/callback", http.HandlerFunc(deps.SCM.EmailProvider.EmailOAuthCallback))
	api.HandleH("POST /channels/{id}/test-email", auth(http.HandlerFunc(deps.SCM.EmailProvider.TestEmailChannel)))

	// Workspace SCM connection endpoints
	wsView := deps.PermissionMiddleware.RequireWorkspacePermission(models.PermissionItemView)
	wsAdmin := deps.PermissionMiddleware.RequireWorkspacePermission(models.PermissionWorkspaceAdmin)
	api.HandleH("GET /workspaces/{id}/scm-providers", auth(wsView(http.HandlerFunc(deps.SCM.Workspace.GetAvailableSCMProviders))))
	api.HandleH("GET /workspaces/{id}/scm-connections", auth(wsView(http.HandlerFunc(deps.SCM.Workspace.GetWorkspaceSCMConnections))))
	api.HandleH("POST /workspaces/{id}/scm-connections", auth(wsAdmin(http.HandlerFunc(deps.SCM.Workspace.CreateWorkspaceSCMConnection))))
	api.HandleH("GET /workspaces/{id}/scm-connections/{connId}", auth(wsView(http.HandlerFunc(deps.SCM.Workspace.GetWorkspaceSCMConnection))))
	api.HandleH("PUT /workspaces/{id}/scm-connections/{connId}", auth(wsAdmin(http.HandlerFunc(deps.SCM.Workspace.UpdateWorkspaceSCMConnection))))
	api.HandleH("DELETE /workspaces/{id}/scm-connections/{connId}", auth(wsAdmin(http.HandlerFunc(deps.SCM.Workspace.DeleteWorkspaceSCMConnection))))
	api.HandleH("GET /workspaces/{id}/scm-connections/{connId}/repositories/available", auth(wsView(http.HandlerFunc(deps.SCM.Workspace.ListAvailableRepositories))))
	api.HandleH("GET /workspaces/{id}/scm-connections/{connId}/repositories", auth(wsView(http.HandlerFunc(deps.SCM.Workspace.GetLinkedRepositories))))
	api.HandleH("POST /workspaces/{id}/scm-connections/{connId}/repositories", auth(wsAdmin(http.HandlerFunc(deps.SCM.Workspace.LinkRepository))))
	api.HandleH("DELETE /workspace-repositories/{repoId}", auth(http.HandlerFunc(deps.SCM.Workspace.UnlinkRepository)))

	// Workspace SCM connection auth endpoints
	api.HandleH("POST /workspaces/{id}/scm-connections/{connId}/auth/start", auth(wsAdmin(http.HandlerFunc(deps.SCM.Workspace.StartWorkspaceOAuth))))
	api.HandleH("POST /workspaces/{id}/scm-connections/{connId}/auth/pat", auth(wsAdmin(http.HandlerFunc(deps.SCM.Workspace.SetWorkspacePAT))))
	api.HandleH("DELETE /workspaces/{id}/scm-connections/{connId}/auth", auth(wsAdmin(http.HandlerFunc(deps.SCM.Workspace.ClearWorkspaceCredentials))))
	api.HandleH("GET /workspaces/{id}/scm-connections/{connId}/auth/status", auth(wsView(http.HandlerFunc(deps.SCM.Workspace.GetWorkspaceConnectionAuthStatus))))
	api.HandleH("POST /workspace-repositories/{repoId}/sync", auth(http.HandlerFunc(deps.SCM.ItemLinks.SyncWorkspaceRepository)))

	// Item SCM Links endpoints
	api.HandleH("GET /items/{id}/scm-links", auth(http.HandlerFunc(deps.SCM.ItemLinks.GetItemSCMLinks)))
	api.HandleH("POST /items/{id}/scm-links", auth(http.HandlerFunc(deps.SCM.ItemLinks.CreateItemSCMLink)))
	api.HandleH("POST /items/{id}/scm-links/create-branch", auth(http.HandlerFunc(deps.SCM.ItemLinks.CreateBranchForItem)))
	api.HandleH("GET /items/{id}/scm-repositories", auth(http.HandlerFunc(deps.SCM.ItemLinks.GetWorkspaceRepositoriesForItem)))
	api.HandleH("GET /items/{id}/scm-connection-status", auth(http.HandlerFunc(deps.SCM.ItemLinks.GetSCMConnectionStatus)))
	api.HandleH("DELETE /item-scm-links/{linkId}", auth(http.HandlerFunc(deps.SCM.ItemLinks.DeleteItemSCMLink)))
	api.HandleH("POST /item-scm-links/{linkId}/refresh", auth(http.HandlerFunc(deps.SCM.ItemLinks.RefreshItemSCMLink)))
	api.HandleH("POST /item-scm-links/{linkId}/create-pr", auth(http.HandlerFunc(deps.SCM.ItemLinks.CreatePRFromBranch)))

	// User SCM connections (personal OAuth tokens)
	api.HandleH("GET /users/me/scm-connections", auth(http.HandlerFunc(deps.SCM.UserToken.GetUserConnections)))
	api.HandleH("GET /users/me/scm-connections/available", auth(http.HandlerFunc(deps.SCM.UserToken.GetAvailableProviders)))
	api.HandleH("GET /users/me/scm-connections/{provider_id}", auth(http.HandlerFunc(deps.SCM.UserToken.GetConnectionStatus)))
	api.HandleH("DELETE /users/me/scm-connections/{provider_id}", auth(http.HandlerFunc(deps.SCM.UserToken.DisconnectProvider)))
}
