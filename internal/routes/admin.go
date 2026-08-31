package routes

import "net/http"

// RegisterAdminRoutes registers admin routes.
func RegisterAdminRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	if deps.Admin.ObjectTranslation != nil {
		translations := deps.Admin.ObjectTranslation
		api.HandleH("GET /admin/object-translations/definitions", admin(http.HandlerFunc(translations.ListDefinitions)))
		api.HandleH("POST /admin/object-translations/resolve", admin(http.HandlerFunc(translations.Resolve)))
		api.HandleH("GET /admin/object-translations/orphans", admin(http.HandlerFunc(translations.FindOrphans)))
		api.HandleH("GET /admin/object-translations/canonical-differences", admin(http.HandlerFunc(translations.FindCanonicalDifferences)))
		api.HandleH("GET /admin/object-translations/{object_type}/{object_id}", admin(http.HandlerFunc(translations.List)))
		api.HandleH("PUT /admin/object-translations/{object_type}/{object_id}/{field}/{locale}", admin(http.HandlerFunc(translations.Upsert)))
		api.HandleH("DELETE /admin/object-translations/{object_type}/{object_id}/{field}/{locale}", admin(http.HandlerFunc(translations.Delete)))
	}

	// Admin security settings
	api.HandleH("GET /admin/security-settings", admin(http.HandlerFunc(deps.Admin.SecuritySettings.GetSecuritySettings)))
	api.HandleH("PUT /admin/security-settings", admin(http.HandlerFunc(deps.Admin.SecuritySettings.UpdateSecuritySettings)))

	// Coding-agent acting-identity gate (WI-87). Master flag + allowlist
	// for which service users a workspace admin may bind a run to.
	if deps.Admin.AgentSecurity != nil {
		api.HandleH("GET /admin/agent-security/settings", admin(http.HandlerFunc(deps.Admin.AgentSecurity.GetSettings)))
		api.HandleH("PUT /admin/agent-security/settings", admin(http.HandlerFunc(deps.Admin.AgentSecurity.UpdateSettings)))
		api.HandleH("GET /admin/agent-security/allowlist", admin(http.HandlerFunc(deps.Admin.AgentSecurity.ListAllowlist)))
		api.HandleH("POST /admin/agent-security/allowlist", admin(http.HandlerFunc(deps.Admin.AgentSecurity.AddAllowlist)))
		api.HandleH("DELETE /admin/agent-security/allowlist/{user_id}", admin(http.HandlerFunc(deps.Admin.AgentSecurity.RemoveAllowlist)))
	}

	// System diagnostics (admin-only)
	api.HandleH("GET /admin/diagnostics/action-logs", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetActionLogs)))
	api.HandleH("GET /admin/diagnostics/webhook-deliveries", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetWebhookDeliveries)))
	api.HandleH("GET /admin/diagnostics/webhook-stats", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetWebhookStats)))
	api.HandleH("GET /admin/diagnostics/webhook-dispatch", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetWebhookDispatch)))
	api.HandleH("GET /admin/diagnostics/transition-matrix", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetTransitionMatrix)))
	api.HandleH("GET /admin/diagnostics/bulk-operations", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetBulkOperations)))
	api.HandleH("POST /admin/diagnostics/webhook-deliveries/purge", admin(http.HandlerFunc(deps.Admin.Diagnostics.PurgeWebhookDeliveries)))
	api.HandleH("GET /admin/diagnostics/scheduler-runs", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetSchedulerRuns)))
	api.HandleH("GET /admin/diagnostics/scheduler-stats", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetSchedulerStats)))
	api.HandleH("POST /admin/diagnostics/scheduler-runs/purge", admin(http.HandlerFunc(deps.Admin.Diagnostics.PurgeSchedulerRuns)))
	api.HandleH("GET /admin/diagnostics/frac-index", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetFracIndexState)))
	api.HandleH("POST /admin/diagnostics/frac-index/migration", admin(http.HandlerFunc(deps.Admin.Diagnostics.ControlGlobalRankMigration)))
	api.HandleH("GET /admin/diagnostics/llm-providers", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetLLMProviderStatus)))
	api.HandleH("GET /admin/diagnostics/briefing-failures", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetBriefingFailures)))
	api.HandleH("GET /admin/diagnostics/runner-pools", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetRunnerPools)))
	api.HandleH("GET /admin/diagnostics/database-pool", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetDatabasePool)))
	api.HandleH("GET /admin/diagnostics/cache-memory", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetCacheMemory)))
	api.HandleH("GET /admin/diagnostics/session-validation-cache", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetSessionValidationCache)))
	api.HandleH("GET /admin/diagnostics/recurrence-volume", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetRecurrenceVolume)))
	api.HandleH("PUT /admin/diagnostics/recurrence-volume", admin(http.HandlerFunc(deps.Admin.Diagnostics.UpdateRecurrenceVolumeSettings)))
	api.HandleH("GET /admin/diagnostics/domain-events", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetDomainEvents)))
	api.HandleH("GET /admin/diagnostics/scm-connections", admin(http.HandlerFunc(deps.Admin.Diagnostics.GetSCMConnections)))
	api.HandleH("POST /admin/diagnostics/domain-events/{eventID}/{consumerKey}/replay", admin(http.HandlerFunc(deps.Admin.Diagnostics.ReplayDomainEvent)))
	api.HandleH("POST /admin/diagnostics/domain-events/{eventID}/{consumerKey}/skip", admin(http.HandlerFunc(deps.Admin.Diagnostics.SkipDomainEvent)))

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
	api.HandleH("GET /admin/audit-logs/since", admin(http.HandlerFunc(deps.Admin.AuditLog.StreamAuditLogsSince)))
	api.HandleH("GET /admin/audit-logs/action-types", admin(http.HandlerFunc(deps.Admin.AuditLog.GetAuditLogActionTypes)))
	api.HandleH("GET /admin/audit-logs/resource-types", admin(http.HandlerFunc(deps.Admin.AuditLog.GetAuditLogResourceTypes)))
	api.HandleH("GET /admin/audit-logs/{id}/agent-transcript", admin(http.HandlerFunc(deps.Admin.AuditLog.GetAgentTranscript)))

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

	// Agent template catalog (WI-922): system-admin overrides for the Agent
	// Studio creation catalog. The handler is always available.
	if deps.Admin.AgentTemplateCatalog != nil {
		api.HandleH("GET /admin/agent-templates", admin(http.HandlerFunc(deps.Admin.AgentTemplateCatalog.ListTemplates)))
		api.HandleH("POST /admin/agent-templates", admin(http.HandlerFunc(deps.Admin.AgentTemplateCatalog.CreateTemplate)))
		api.HandleH("GET /admin/agent-templates/defaults", admin(http.HandlerFunc(deps.Admin.AgentTemplateCatalog.DefaultTemplates)))
		api.HandleH("GET /admin/agent-templates/{id}", admin(http.HandlerFunc(deps.Admin.AgentTemplateCatalog.GetTemplate)))
		api.HandleH("PUT /admin/agent-templates/{id}", admin(http.HandlerFunc(deps.Admin.AgentTemplateCatalog.UpdateTemplate)))
		api.HandleH("DELETE /admin/agent-templates/{id}", admin(http.HandlerFunc(deps.Admin.AgentTemplateCatalog.DeleteTemplate)))
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
	if deps.Admin.ShellBootstrap != nil {
		api.HandleH("GET /shell-bootstrap", auth(http.HandlerFunc(deps.Admin.ShellBootstrap.Get)))
	}

	// Jira import.
	api.HandleH("GET /admin/jira-import/connections", admin(http.HandlerFunc(deps.Admin.JiraImport.GetConnections)))
	api.HandleH("DELETE /admin/jira-import/connections/{connectionId}", admin(http.HandlerFunc(deps.Admin.JiraImport.DeleteConnection)))
	api.HandleH("POST /admin/jira-import/connect", admin(http.HandlerFunc(deps.Admin.JiraImport.Connect)))
	api.HandleH("GET /admin/jira-import/projects", admin(http.HandlerFunc(deps.Admin.JiraImport.GetProjects)))
	api.HandleH("POST /admin/jira-import/projects/counts", admin(http.HandlerFunc(deps.Admin.JiraImport.GetProjectCounts)))
	api.HandleH("POST /admin/jira-import/analyze", admin(http.HandlerFunc(deps.Admin.JiraImport.Analyze)))
	api.HandleH("POST /admin/jira-import/readiness", admin(http.HandlerFunc(deps.Admin.JiraImport.Readiness)))
	api.HandleH("GET /admin/jira-import/assets", admin(http.HandlerFunc(deps.Admin.JiraImport.GetAssetSchemas)))
	api.HandleH("GET /admin/jira-import/assets/{schemaId}/types", admin(http.HandlerFunc(deps.Admin.JiraImport.GetAssetTypes)))
	api.HandleH("GET /admin/jira-import/jobs", admin(http.HandlerFunc(deps.Admin.JiraImport.GetImportJobs)))
	api.HandleH("GET /admin/jira-import/jobs/{jobId}", admin(http.HandlerFunc(deps.Admin.JiraImport.GetJobStatus)))
	api.HandleH("DELETE /admin/jira-import/jobs/{jobId}/data", admin(http.HandlerFunc(deps.Admin.JiraImport.DeleteImportedData)))
	api.HandleH("POST /admin/jira-import/start", admin(deps.SetupLimiter.Limit(http.HandlerFunc(deps.Admin.JiraImport.StartImport))))
	api.HandleH("GET /admin/jira-import/previous-imports", admin(http.HandlerFunc(deps.Admin.JiraImport.GetPreviousImports)))
}
