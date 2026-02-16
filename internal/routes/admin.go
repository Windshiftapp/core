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

	// Plugin management endpoints
	api.HandleH("GET /plugins", admin(http.HandlerFunc(deps.Admin.Plugin.ListPlugins)))
	api.HandleH("POST /plugins/upload", admin(deps.UploadLimiter.Limit(http.HandlerFunc(deps.Admin.Plugin.UploadPlugin))))
	api.HandleH("GET /plugins/extensions", auth(http.HandlerFunc(deps.Admin.Plugin.GetExtensions)))
	api.HandleH("GET /plugins/{name}/assets/{asset...}", http.HandlerFunc(deps.Admin.Plugin.GetAsset))
	api.HandleH("PUT /plugins/{name}/toggle", admin(http.HandlerFunc(deps.Admin.Plugin.TogglePlugin)))
	api.HandleH("DELETE /plugins/{name}", admin(http.HandlerFunc(deps.Admin.Plugin.DeletePlugin)))
	api.HandleH("POST /plugins/{name}/reload", admin(http.HandlerFunc(deps.Admin.Plugin.ReloadPlugin)))

	// Admin endpoint for token cleanup
	api.HandleH("POST /admin/api-tokens/cleanup", admin(http.HandlerFunc(deps.Users.APIToken.CleanupExpiredTokens)))

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
