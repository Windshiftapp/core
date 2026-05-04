package routes

import "net/http"

// RegisterAdminRoutes registers admin-related routes (audit, security, themes, plugins, jira import).
func RegisterAdminRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	// Admin security settings
	api.HandleH("GET /admin/security-settings", admin(http.HandlerFunc(deps.Admin.SecuritySettings.GetSecuritySettings)))
	api.HandleH("PUT /admin/security-settings", admin(http.HandlerFunc(deps.Admin.SecuritySettings.UpdateSecuritySettings)))

	// System diagnostics (admin-only)
	api.HandleH("GET /admin/diagnostics/action-logs", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetActionLogs)))
	api.HandleH("GET /admin/diagnostics/webhook-deliveries", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetWebhookDeliveries)))
	api.HandleH("GET /admin/diagnostics/webhook-stats", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetWebhookStats)))
	api.HandleH("POST /admin/diagnostics/webhook-deliveries/purge", admin(http.HandlerFunc(deps.Admin.Diagnostics.PurgeWebhookDeliveries)))
	api.HandleH("GET /admin/diagnostics/scheduler-runs", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetSchedulerRuns)))
	api.HandleH("GET /admin/diagnostics/scheduler-stats", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetSchedulerStats)))
	api.HandleH("POST /admin/diagnostics/scheduler-runs/purge", admin(http.HandlerFunc(deps.Admin.Diagnostics.PurgeSchedulerRuns)))

	// Authentication policy endpoints (admin only)
	api.HandleH("GET /admin/auth-policy", admin(http.HandlerFunc(deps.Admin.AuthPolicy.GetAuthPolicy)))
	api.HandleH("PUT /admin/auth-policy", admin(http.HandlerFunc(deps.Admin.AuthPolicy.UpdateAuthPolicy)))
	api.HandleH("GET /admin/auth-policy/stats", admin(http.HandlerFunc(deps.Admin.AuthPolicy.GetAuthPolicyStats)))
	api.HandleH("GET /admin/auth-policy/affected", admin(http.HandlerFunc(deps.Admin.AuthPolicy.GetAffectedUsers)))

	// Public auth policy status endpoint (no auth required - for login page)
	api.HandleH("GET /auth/policy-status", deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Admin.AuthPolicy.GetPublicPolicyStatus)))

	// Theme management endpoints
	api.HandleH("GET /themes", auth(http.HandlerFunc(deps.Admin.Theme.GetThemes)))
	api.HandleH("GET /themes/active", auth(http.HandlerFunc(deps.Admin.Theme.GetActiveTheme)))
	api.HandleH("POST /themes", admin(http.HandlerFunc(deps.Admin.Theme.CreateTheme)))
	api.HandleH("PUT /themes/{id}", admin(http.HandlerFunc(deps.Admin.Theme.UpdateTheme)))
	api.HandleH("DELETE /themes/{id}", admin(http.HandlerFunc(deps.Admin.Theme.DeleteTheme)))
	api.HandleH("POST /themes/{id}/activate", admin(http.HandlerFunc(deps.Admin.Theme.ActivateTheme)))

	// User preferences routes
	api.HandleH("GET /user/preferences", auth(http.HandlerFunc(deps.Admin.UserPreferences.GetUserPreferences)))
	api.HandleH("PUT /user/preferences", auth(http.HandlerFunc(deps.Admin.UserPreferences.UpdateUserPreferences)))

	// Personal dashboard layout (per-user)
	api.HandleH("GET /user/dashboard-layout", auth(http.HandlerFunc(deps.Admin.UserPreferences.GetDashboardLayout)))
	api.HandleH("PUT /user/dashboard-layout", auth(http.HandlerFunc(deps.Admin.UserPreferences.UpdateDashboardLayout)))

	// Plugin management endpoints
	api.HandleH("GET /plugins", admin(http.HandlerFunc(deps.Admin.Plugin.ListPlugins)))
	api.HandleH("POST /plugins/upload", admin(deps.UploadLimiter.Limit(http.HandlerFunc(deps.Admin.Plugin.UploadPlugin))))
	api.HandleH("GET /plugins/extensions", auth(http.HandlerFunc(deps.Admin.Plugin.GetExtensions)))
	api.HandleH("GET /plugins/{name}/assets/{asset...}", http.HandlerFunc(deps.Admin.Plugin.GetAsset))
	api.HandleH("PUT /plugins/{name}/toggle", admin(http.HandlerFunc(deps.Admin.Plugin.TogglePlugin)))
	api.HandleH("DELETE /plugins/{name}", admin(http.HandlerFunc(deps.Admin.Plugin.DeletePlugin)))
	api.HandleH("POST /plugins/{name}/reload", admin(http.HandlerFunc(deps.Admin.Plugin.ReloadPlugin)))

	// Admin API token management
	api.HandleH("GET /admin/api-tokens", admin(http.HandlerFunc(deps.Users.APIToken.ListAllTokens)))
	api.HandleH("DELETE /admin/api-tokens/{id}", admin(http.HandlerFunc(deps.Users.APIToken.AdminRevokeToken)))
	api.HandleH("POST /admin/api-tokens/cleanup", admin(http.HandlerFunc(deps.Users.APIToken.CleanupExpiredTokens)))

	// Audit log endpoints (admin-only)
	api.HandleH("GET /admin/audit-logs", admin(http.HandlerFunc(deps.Admin.AuditLog.ListAuditLogs)))
	api.HandleH("GET /admin/audit-logs/action-types", admin(http.HandlerFunc(deps.Admin.AuditLog.GetAuditLogActionTypes)))
	api.HandleH("GET /admin/audit-logs/resource-types", admin(http.HandlerFunc(deps.Admin.AuditLog.GetAuditLogResourceTypes)))

	// OAuth client management (admin-only). Backs the generic OAuth 2.0
	// authorization-code-with-PKCE server: admins register third-party apps
	// here, and any registered app can drive `/api/oauth/authorize` +
	// `/api/oauth/token` to mint per-user `crw_…` tokens. See
	// internal/handlers/admin_oauth_clients.go.
	if deps.Admin.OAuthClients != nil {
		api.HandleH("GET /admin/oauth-clients", admin(http.HandlerFunc(deps.Admin.OAuthClients.GetClients)))
		api.HandleH("POST /admin/oauth-clients", admin(http.HandlerFunc(deps.Admin.OAuthClients.CreateClient)))
		api.HandleH("GET /admin/oauth-clients/{id}", admin(http.HandlerFunc(deps.Admin.OAuthClients.GetClient)))
		api.HandleH("PUT /admin/oauth-clients/{id}", admin(http.HandlerFunc(deps.Admin.OAuthClients.UpdateClient)))
		api.HandleH("POST /admin/oauth-clients/{id}/rotate-secret", admin(http.HandlerFunc(deps.Admin.OAuthClients.RotateSecret)))
		api.HandleH("DELETE /admin/oauth-clients/{id}", admin(http.HandlerFunc(deps.Admin.OAuthClients.DeleteClient)))
	}

	// LDAP directory management endpoints (admin-only)
	if deps.Admin.LDAP != nil {
		api.HandleH("GET /admin/ldap/configs", admin(http.HandlerFunc(deps.Admin.LDAP.ListConfigs)))
		api.HandleH("POST /admin/ldap/configs", admin(http.HandlerFunc(deps.Admin.LDAP.CreateConfig)))
		api.HandleH("GET /admin/ldap/configs/{id}", admin(http.HandlerFunc(deps.Admin.LDAP.GetConfig)))
		api.HandleH("PUT /admin/ldap/configs/{id}", admin(http.HandlerFunc(deps.Admin.LDAP.UpdateConfig)))
		api.HandleH("DELETE /admin/ldap/configs/{id}", admin(http.HandlerFunc(deps.Admin.LDAP.DeleteConfig)))
		api.HandleH("POST /admin/ldap/configs/{id}/test", admin(http.HandlerFunc(deps.Admin.LDAP.TestConnection)))
		api.HandleH("POST /admin/ldap/configs/{id}/sync", admin(http.HandlerFunc(deps.Admin.LDAP.TriggerSync)))
		api.HandleH("GET /admin/ldap/configs/{id}/sync-status", admin(http.HandlerFunc(deps.Admin.LDAP.GetSyncStatus)))
	}

	// Feature discovery endpoint (public, no auth required)
	if deps.Admin.Features != nil {
		api.HandleH("GET /features", http.HandlerFunc(deps.Admin.Features.GetFeatures))
	}

	// Jira Import endpoints
	api.HandleH("GET /admin/jira-import/connections", admin(http.HandlerFunc(deps.Admin.JiraImport.GetConnections)))
	api.HandleH("DELETE /admin/jira-import/connections/{connectionId}", admin(http.HandlerFunc(deps.Admin.JiraImport.DeleteConnection)))
	api.HandleH("POST /admin/jira-import/connect", admin(http.HandlerFunc(deps.Admin.JiraImport.Connect)))
	api.HandleH("GET /admin/jira-import/projects", admin(http.HandlerFunc(deps.Admin.JiraImport.GetProjects)))
	api.HandleH("POST /admin/jira-import/analyze", admin(http.HandlerFunc(deps.Admin.JiraImport.Analyze)))
	api.HandleH("GET /admin/jira-import/assets", admin(http.HandlerFunc(deps.Admin.JiraImport.GetAssetSchemas)))
	api.HandleH("GET /admin/jira-import/assets/{schemaId}/types", admin(http.HandlerFunc(deps.Admin.JiraImport.GetAssetTypes)))
	api.HandleH("GET /admin/jira-import/jobs", admin(http.HandlerFunc(deps.Admin.JiraImport.GetImportJobs)))
	api.HandleH("GET /admin/jira-import/jobs/{jobId}", admin(http.HandlerFunc(deps.Admin.JiraImport.GetJobStatus)))
	api.HandleH("DELETE /admin/jira-import/jobs/{jobId}/data", admin(http.HandlerFunc(deps.Admin.JiraImport.DeleteImportedData)))
	api.HandleH("POST /admin/jira-import/start", admin(deps.SetupLimiter.Limit(http.HandlerFunc(deps.Admin.JiraImport.StartImport))))
	api.HandleH("GET /admin/jira-import/previous-imports", admin(http.HandlerFunc(deps.Admin.JiraImport.GetPreviousImports)))
}
