package routes

import (
	"net/http"

	"windshift/internal/models"
)

// RegisterWorkspaceRoutes registers workspace-related routes (workspaces, screens, config sets, statuses, workflows).
func RegisterWorkspaceRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()
	workspaceView := deps.PermissionMiddleware.RequireWorkspacePermission(models.PermissionItemView)

	// Workspace endpoints
	api.HandleH("GET /workspaces/personal", auth(http.HandlerFunc(deps.Workspaces.Workspace.GetOrCreatePersonalWorkspace)))
	api.HandleH("GET /workspaces/{id}/bootstrap", auth(http.HandlerFunc(deps.Workspaces.Bootstrap.Get)))
	api.HandleH("GET /workspaces/{id}/stats", auth(workspaceView(http.HandlerFunc(deps.Workspaces.Workspace.GetStats))))
	api.HandleH("GET /workspaces/{id}/homepage/layout", auth(http.HandlerFunc(deps.Workspaces.Workspace.GetHomepageLayout)))
	api.HandleH("PUT /workspaces/{id}/homepage/layout", auth(http.HandlerFunc(deps.Workspaces.Workspace.UpdateHomepageLayout)))

	// Screen endpoints
	api.HandleH("GET /screens", auth(http.HandlerFunc(deps.Workspaces.Screen.GetAll)))
	api.HandleH("POST /screens", admin(http.HandlerFunc(deps.Workspaces.Screen.Create)))
	api.HandleH("GET /screens/{id}", auth(http.HandlerFunc(deps.Workspaces.Screen.Get)))
	api.HandleH("PUT /screens/{id}", admin(http.HandlerFunc(deps.Workspaces.Screen.Update)))
	api.HandleH("DELETE /screens/{id}", admin(http.HandlerFunc(deps.Workspaces.Screen.Delete)))
	api.HandleH("GET /screens/{id}/fields", auth(http.HandlerFunc(deps.Workspaces.Screen.GetFields)))
	api.HandleH("PUT /screens/{id}/fields", admin(http.HandlerFunc(deps.Workspaces.Screen.UpdateFields)))
	api.HandleH("PUT /screens/{id}/system-fields", admin(http.HandlerFunc(deps.Workspaces.Screen.UpdateSystemFields)))

	// Configuration Set endpoints
	api.HandleH("GET /configuration-sets", auth(http.HandlerFunc(deps.Workspaces.ConfigSet.GetAll)))
	api.HandleH("POST /configuration-sets", admin(http.HandlerFunc(deps.Workspaces.ConfigSet.Create)))
	api.HandleH("GET /configuration-sets/{id}", auth(http.HandlerFunc(deps.Workspaces.ConfigSet.Get)))
	api.HandleH("PUT /configuration-sets/{id}", admin(http.HandlerFunc(deps.Workspaces.ConfigSet.Update)))
	api.HandleH("DELETE /configuration-sets/{id}", admin(http.HandlerFunc(deps.Workspaces.ConfigSet.Delete)))
	api.HandleH("GET /configuration-sets/{id}/analyze-migration", auth(http.HandlerFunc(deps.Workspaces.ConfigSet.AnalyzeMigration)))
	api.HandleH("POST /configuration-sets/execute-migration", admin(deps.SetupLimiter.Limit(http.HandlerFunc(deps.Workspaces.ConfigSet.ExecuteMigration))))
	api.HandleH("GET /configuration-sets/{id}/analyze-comprehensive-migration", auth(http.HandlerFunc(deps.Workspaces.ConfigSet.AnalyzeComprehensiveMigration)))
	api.HandleH("POST /configuration-sets/execute-comprehensive-migration", admin(deps.SetupLimiter.Limit(http.HandlerFunc(deps.Workspaces.ConfigSet.ExecuteComprehensiveMigration))))
	api.HandleH("GET /configuration-sets/{id}/export", auth(http.HandlerFunc(deps.Workspaces.ConfigSet.Export)))
	api.HandleH("POST /configuration-sets/import", admin(deps.SetupLimiter.Limit(http.HandlerFunc(deps.Workspaces.ConfigSet.Import))))

	// Notification Settings endpoints
	api.HandleH("GET /notification-settings", auth(http.HandlerFunc(deps.Workspaces.NotificationSettings.GetNotificationSettings)))
	api.HandleH("POST /notification-settings", admin(http.HandlerFunc(deps.Workspaces.NotificationSettings.CreateNotificationSetting)))
	api.HandleH("GET /notification-settings/available-events", auth(http.HandlerFunc(deps.Workspaces.NotificationSettings.GetAvailableEvents)))
	api.HandleH("GET /notification-settings/{id}", auth(http.HandlerFunc(deps.Workspaces.NotificationSettings.GetNotificationSetting)))
	api.HandleH("PUT /notification-settings/{id}", admin(http.HandlerFunc(deps.Workspaces.NotificationSettings.UpdateNotificationSetting)))
	api.HandleH("DELETE /notification-settings/{id}", admin(http.HandlerFunc(deps.Workspaces.NotificationSettings.DeleteNotificationSetting)))

	// Configuration Set Notification assignments
	api.HandleH("GET /configuration-sets/{config_set_id}/notification-settings", auth(http.HandlerFunc(deps.Workspaces.ConfigSetNotification.GetConfigurationSetNotifications)))
	api.HandleH("POST /configuration-sets/{config_set_id}/notification-settings", admin(http.HandlerFunc(deps.Workspaces.ConfigSetNotification.AssignNotificationToConfigurationSet)))
	api.HandleH("DELETE /configuration-sets/{config_set_id}/notification-settings/{assignment_id}", admin(http.HandlerFunc(deps.Workspaces.ConfigSetNotification.UnassignNotificationFromConfigurationSet)))
	api.HandleH("GET /configuration-sets/{config_set_id}/available-notification-settings", auth(http.HandlerFunc(deps.Workspaces.ConfigSetNotification.GetAvailableNotificationSettings)))

	// Hierarchy Level endpoints
	api.HandleH("GET /hierarchy-levels", auth(http.HandlerFunc(deps.Workspaces.HierarchyLevel.GetAll)))
	api.HandleH("POST /hierarchy-levels", admin(http.HandlerFunc(deps.Workspaces.HierarchyLevel.Create)))
	api.HandleH("GET /hierarchy-levels/{id}", auth(http.HandlerFunc(deps.Workspaces.HierarchyLevel.Get)))
	api.HandleH("PUT /hierarchy-levels/{id}", admin(http.HandlerFunc(deps.Workspaces.HierarchyLevel.Update)))
	api.HandleH("DELETE /hierarchy-levels/{id}", admin(http.HandlerFunc(deps.Workspaces.HierarchyLevel.Delete)))

	// Binding handlers apply workspace-admin authorization consistently.
	if deps.Workspaces.AgentBinding != nil {
		api.HandleH("GET /workspaces/{workspaceId}/agent-profiles", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.Catalog)))
		api.HandleH("GET /workspaces/{workspaceId}/agent-bindings", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.List)))
		api.HandleH("GET /workspaces/{workspaceId}/agent-templates", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.Templates)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-profiles", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.CreateProfile)))
		api.HandleH("PATCH /workspaces/{workspaceId}/agent-profiles/{id}", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.UpdateProfile)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-profiles/{id}/migrate-runner", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.MigrateLegacyProfile)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-profiles/{id}/connect-runner", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.ConnectCodingRunner)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-profiles/{id}/test", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.TestProfile)))
		api.HandleH("GET /workspaces/{workspaceId}/agent-profiles/{id}/validation", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.ValidateProfile)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-profiles/{id}/ready", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.ActivateProfile)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-runner-pools/{poolId}/tokens", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.MintRunnerSetupToken)))
		api.HandleH("GET /workspaces/{workspaceId}/agent-runner-pools/{poolId}/tokens", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.ListRunnerSetupTokens)))
		api.HandleH("DELETE /workspaces/{workspaceId}/agent-runner-pools/{poolId}/tokens/{tokenId}", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.RevokeRunnerSetupToken)))
		api.HandleH("GET /workspaces/{workspaceId}/agent-runner-pools/{poolId}/instances", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.ListRunnerSetupInstances)))
		api.HandleH("GET /workspaces/{workspaceId}/agent-tool-capabilities", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.ToolCapabilities)))
		api.HandleH("GET /workspaces/{workspaceId}/agent-bindings/standard-prompt", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.InitialPrompt)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-bindings", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.Create)))
		api.HandleH("PUT /workspaces/{workspaceId}/agent-bindings/{id}", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.Update)))
		api.HandleH("DELETE /workspaces/{workspaceId}/agent-bindings/{id}", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.Delete)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-bindings/{id}/restore", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.Restore)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-bindings/{id}/test-llm", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.TestLLM)))
		api.HandleH("POST /workspaces/{workspaceId}/agent-bindings/{id}/test-run", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.TestRun)))
		api.HandleH("PUT /workspaces/{workspaceId}/agent-bindings/{id}/agent-config", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.UpdateAgentConfig)))
		api.HandleH("GET /workspaces/{workspaceId}/agent-binding-candidates", auth(http.HandlerFunc(deps.Workspaces.AgentBinding.Candidates)))
	}

	// Runner control uses inline runner credentials rather than user sessions.
	if deps.Workspaces.RunnerControl != nil {
		register := http.Handler(http.HandlerFunc(deps.Workspaces.RunnerControl.Register))
		if deps.RunnerRegisterLimiter != nil {
			register = deps.RunnerRegisterLimiter.Limit(register)
		}
		api.HandleH("POST /runner/register", register)
		api.HandleH("POST /runner/claim", http.HandlerFunc(deps.Workspaces.RunnerControl.Claim))
		api.HandleH("POST /runner/runs/{id}/events", http.HandlerFunc(deps.Workspaces.RunnerControl.Events))
		api.HandleH("POST /runner/runs/{id}/result", http.HandlerFunc(deps.Workspaces.RunnerControl.Result))
		api.HandleH("POST /runner/heartbeat", http.HandlerFunc(deps.Workspaces.RunnerControl.Heartbeat))
	}

	// Broker routes authenticate with a per-run token.
	if deps.Workspaces.RunnerBroker != nil {
		api.HandleH("GET /secrets/{run}/{credentialId}", http.HandlerFunc(deps.Workspaces.RunnerBroker.GetSecret))
		api.HandleH("POST /llm-proxy/{run}/complete", http.HandlerFunc(deps.Workspaces.RunnerBroker.ProxyLLM))
		api.HandleH("GET /git-proxy/{ws}/{owner}/{repo}/{gitpath...}", http.HandlerFunc(deps.Workspaces.RunnerBroker.ProxyGit))
		api.HandleH("POST /git-proxy/{ws}/{owner}/{repo}/{gitpath...}", http.HandlerFunc(deps.Workspaces.RunnerBroker.ProxyGit))
		api.HandleH("GET /http-proxy/{run}", http.HandlerFunc(deps.Workspaces.RunnerBroker.ProxyHTTP))
		api.HandleH("POST /http-proxy/{run}", http.HandlerFunc(deps.Workspaces.RunnerBroker.ProxyHTTP))
	}

	if deps.Workspaces.Actions != nil {
		actionManage := deps.PermissionMiddleware.RequireWorkspacePermission(models.PermissionActionManage)

		// Execute performs its own per-action authorization: manual actions use
		// their optional role allowlist (or item.edit by default), while testing
		// event-driven actions remains action.manage-only.
		api.HandleH("GET /workspaces/{workspaceId}/action-logs", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.GetWorkspaceLogs))))

		// Capability listing shares action-authoring authorization.
		api.HandleH("GET /workspaces/{workspaceId}/action-capabilities", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.ListWorkspaceCapabilities))))

		// Template reads are authenticated; applying requires action.manage.

		api.HandleH("GET /admin/action-capabilities", admin(http.HandlerFunc(deps.Workspaces.Actions.ListCapabilities)))
		api.HandleH("POST /admin/action-capabilities", admin(http.HandlerFunc(deps.Workspaces.Actions.CreateCapability)))
		api.HandleH("GET /admin/action-capabilities/{capabilityId}", admin(http.HandlerFunc(deps.Workspaces.Actions.GetCapability)))
		api.HandleH("PUT /admin/action-capabilities/{capabilityId}", admin(http.HandlerFunc(deps.Workspaces.Actions.UpdateCapability)))
		api.HandleH("DELETE /admin/action-capabilities/{capabilityId}", admin(http.HandlerFunc(deps.Workspaces.Actions.DeleteCapability)))

		// Runner-pool child resources.
		if deps.Workspaces.RunnerControl != nil {
			api.HandleH("POST /admin/action-capabilities/{capabilityId}/runner-tokens", admin(http.HandlerFunc(deps.Workspaces.RunnerControl.MintRunnerToken)))
			api.HandleH("GET /admin/action-capabilities/{capabilityId}/runner-tokens", admin(http.HandlerFunc(deps.Workspaces.RunnerControl.ListRunnerTokens)))
			api.HandleH("DELETE /admin/action-capabilities/{capabilityId}/runner-tokens/{tokenId}", admin(http.HandlerFunc(deps.Workspaces.RunnerControl.RevokeRunnerToken)))
			api.HandleH("GET /admin/action-capabilities/{capabilityId}/runner-instances", admin(http.HandlerFunc(deps.Workspaces.RunnerControl.ListRunnerInstances)))
			api.HandleH("DELETE /admin/action-capabilities/{capabilityId}/runner-instances/{instanceId}", admin(http.HandlerFunc(deps.Workspaces.RunnerControl.RevokeRunnerInstance)))
		}

		if deps.Workspaces.ActionCredentials != nil {
			api.HandleH("GET /admin/action-credentials", admin(http.HandlerFunc(deps.Workspaces.ActionCredentials.ListGlobal)))
			api.HandleH("POST /admin/action-credentials", admin(http.HandlerFunc(deps.Workspaces.ActionCredentials.CreateGlobal)))
			api.HandleH("PUT /admin/action-credentials/{credentialId}", admin(http.HandlerFunc(deps.Workspaces.ActionCredentials.UpdateGlobal)))
			api.HandleH("POST /admin/action-credentials/{credentialId}/rotate", admin(http.HandlerFunc(deps.Workspaces.ActionCredentials.RotateGlobal)))
			api.HandleH("DELETE /admin/action-credentials/{credentialId}", admin(http.HandlerFunc(deps.Workspaces.ActionCredentials.DeleteGlobal)))

			// Handler authorization preserves 404-on-missing-workspace access.
			api.HandleH("GET /workspaces/{workspaceId}/action-credentials", auth(http.HandlerFunc(deps.Workspaces.ActionCredentials.ListForWorkspace)))
			api.HandleH("POST /workspaces/{workspaceId}/action-credentials", auth(http.HandlerFunc(deps.Workspaces.ActionCredentials.CreateForWorkspace)))
			api.HandleH("PUT /workspaces/{workspaceId}/action-credentials/{credentialId}", auth(http.HandlerFunc(deps.Workspaces.ActionCredentials.UpdateForWorkspace)))
			api.HandleH("POST /workspaces/{workspaceId}/action-credentials/{credentialId}/rotate", auth(http.HandlerFunc(deps.Workspaces.ActionCredentials.RotateForWorkspace)))
			api.HandleH("DELETE /workspaces/{workspaceId}/action-credentials/{credentialId}", auth(http.HandlerFunc(deps.Workspaces.ActionCredentials.DeleteForWorkspace)))
		}
	}
}
