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

	// Workspace endpoints
	api.HandleH("GET /workspaces", auth(http.HandlerFunc(deps.Workspaces.Workspace.GetAll)))
	api.HandleH("POST /workspaces", auth(http.HandlerFunc(deps.Workspaces.Workspace.Create)))
	api.HandleH("GET /workspaces/personal", auth(http.HandlerFunc(deps.Workspaces.Workspace.GetOrCreatePersonalWorkspace)))
	api.HandleH("GET /workspaces/{id}", auth(http.HandlerFunc(deps.Workspaces.Workspace.Get)))
	api.HandleH("PUT /workspaces/{id}", auth(http.HandlerFunc(deps.Workspaces.Workspace.Update)))
	api.HandleH("DELETE /workspaces/{id}", auth(http.HandlerFunc(deps.Workspaces.Workspace.Delete)))
	api.HandleH("GET /workspaces/{id}/stats", auth(http.HandlerFunc(deps.Workspaces.Workspace.GetStats)))
	api.HandleH("GET /workspaces/{id}/statuses", auth(http.HandlerFunc(deps.Workspaces.Workspace.GetStatuses)))
	api.HandleH("GET /workspaces/{id}/homepage/layout", auth(http.HandlerFunc(deps.Workspaces.Workspace.GetHomepageLayout)))
	api.HandleH("PUT /workspaces/{id}/homepage/layout", auth(http.HandlerFunc(deps.Workspaces.Workspace.UpdateHomepageLayout)))

	// Workspace field requirement endpoints
	api.HandleH("GET /workspaces/{id}/field-requirements", auth(http.HandlerFunc(deps.Workspaces.FieldRequirement.GetByWorkspace)))
	api.HandleH("POST /workspaces/{id}/field-requirements", admin(http.HandlerFunc(deps.Workspaces.FieldRequirement.SetRequirement)))
	api.HandleH("GET /workspaces/{id}/available-fields", auth(http.HandlerFunc(deps.Workspaces.FieldRequirement.GetAvailableFields)))
	api.HandleH("DELETE /workspaces/{workspaceId}/field-requirements/{fieldId}", admin(http.HandlerFunc(deps.Workspaces.FieldRequirement.RemoveRequirement)))

	// Workspace-scoped time projects (with category restrictions)
	api.HandleH("GET /workspaces/{id}/projects", auth(http.HandlerFunc(deps.TimeTracking.Project.GetByWorkspace)))

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

	// Item Type endpoints
	api.HandleH("GET /item-types", auth(http.HandlerFunc(deps.Workspaces.ItemType.GetAll)))
	api.HandleH("POST /item-types", admin(http.HandlerFunc(deps.Workspaces.ItemType.Create)))
	api.HandleH("GET /item-types/{id}", auth(http.HandlerFunc(deps.Workspaces.ItemType.Get)))
	api.HandleH("PUT /item-types/{id}", admin(http.HandlerFunc(deps.Workspaces.ItemType.Update)))
	api.HandleH("DELETE /item-types/{id}", admin(http.HandlerFunc(deps.Workspaces.ItemType.Delete)))

	// Priority endpoints
	api.HandleH("GET /priorities", auth(http.HandlerFunc(deps.Workspaces.Priority.GetAll)))
	api.HandleH("POST /priorities", admin(http.HandlerFunc(deps.Workspaces.Priority.Create)))
	api.HandleH("GET /priorities/{id}", auth(http.HandlerFunc(deps.Workspaces.Priority.Get)))
	api.HandleH("PUT /priorities/{id}", admin(http.HandlerFunc(deps.Workspaces.Priority.Update)))
	api.HandleH("DELETE /priorities/{id}", admin(http.HandlerFunc(deps.Workspaces.Priority.Delete)))

	// Hierarchy Level endpoints
	api.HandleH("GET /hierarchy-levels", auth(http.HandlerFunc(deps.Workspaces.HierarchyLevel.GetAll)))
	api.HandleH("POST /hierarchy-levels", admin(http.HandlerFunc(deps.Workspaces.HierarchyLevel.Create)))
	api.HandleH("GET /hierarchy-levels/{id}", auth(http.HandlerFunc(deps.Workspaces.HierarchyLevel.Get)))
	api.HandleH("PUT /hierarchy-levels/{id}", admin(http.HandlerFunc(deps.Workspaces.HierarchyLevel.Update)))
	api.HandleH("DELETE /hierarchy-levels/{id}", admin(http.HandlerFunc(deps.Workspaces.HierarchyLevel.Delete)))

	// Status Category endpoints
	api.HandleH("GET /status-categories", auth(http.HandlerFunc(deps.Workspaces.StatusCategory.GetAll)))
	api.HandleH("POST /status-categories", admin(http.HandlerFunc(deps.Workspaces.StatusCategory.Create)))
	api.HandleH("GET /status-categories/{id}", auth(http.HandlerFunc(deps.Workspaces.StatusCategory.Get)))
	api.HandleH("PUT /status-categories/{id}", admin(http.HandlerFunc(deps.Workspaces.StatusCategory.Update)))
	api.HandleH("DELETE /status-categories/{id}", admin(http.HandlerFunc(deps.Workspaces.StatusCategory.Delete)))

	// Status endpoints
	api.HandleH("GET /statuses", auth(http.HandlerFunc(deps.Workspaces.Status.GetAll)))
	api.HandleH("POST /statuses", admin(http.HandlerFunc(deps.Workspaces.Status.Create)))
	api.HandleH("GET /statuses/non-done-ids", auth(http.HandlerFunc(deps.Workspaces.StatusLegacy.GetNonDoneStatusIDs)))
	api.HandleH("GET /statuses/{id}", auth(http.HandlerFunc(deps.Workspaces.Status.Get)))
	api.HandleH("PUT /statuses/{id}", admin(http.HandlerFunc(deps.Workspaces.Status.Update)))
	api.HandleH("DELETE /statuses/{id}", admin(http.HandlerFunc(deps.Workspaces.Status.Delete)))

	// Workflow endpoints
	api.HandleH("GET /workflows", auth(http.HandlerFunc(deps.Workspaces.Workflow.GetAll)))
	api.HandleH("POST /workflows", admin(http.HandlerFunc(deps.Workspaces.Workflow.Create)))
	api.HandleH("GET /workflows/{id}", auth(http.HandlerFunc(deps.Workspaces.Workflow.Get)))
	api.HandleH("PUT /workflows/{id}", admin(http.HandlerFunc(deps.Workspaces.Workflow.Update)))
	api.HandleH("DELETE /workflows/{id}", admin(http.HandlerFunc(deps.Workspaces.Workflow.Delete)))
	api.HandleH("GET /workflows/{id}/transitions", auth(http.HandlerFunc(deps.Workspaces.Workflow.GetTransitions)))
	api.HandleH("PUT /workflows/{id}/transitions", admin(http.HandlerFunc(deps.Workspaces.Workflow.UpdateTransitions)))
	api.HandleH("GET /workflows/{id}/available-transitions/{statusId}", auth(http.HandlerFunc(deps.Workspaces.Workflow.GetAvailableTransitions)))

	// Actions automation endpoints (workspace-scoped, requires action.manage permission)
	if deps.Workspaces.Actions != nil {
		actionManage := deps.PermissionMiddleware.RequireWorkspacePermission(models.PermissionActionManage)

		api.HandleH("GET /workspaces/{workspaceId}/actions", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.ListActions))))
		api.HandleH("POST /workspaces/{workspaceId}/actions", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.CreateAction))))
		api.HandleH("GET /workspaces/{workspaceId}/actions/{id}", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.GetAction))))
		api.HandleH("PUT /workspaces/{workspaceId}/actions/{id}", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.UpdateAction))))
		api.HandleH("DELETE /workspaces/{workspaceId}/actions/{id}", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.DeleteAction))))
		api.HandleH("POST /workspaces/{workspaceId}/actions/{id}/toggle", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.ToggleAction))))
		api.HandleH("POST /workspaces/{workspaceId}/actions/{id}/execute", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.ExecuteAction))))
		api.HandleH("GET /workspaces/{workspaceId}/actions/{id}/logs", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.GetActionLogs))))
		api.HandleH("GET /workspaces/{workspaceId}/action-logs", auth(actionManage(http.HandlerFunc(deps.Workspaces.Actions.GetWorkspaceLogs))))
	}
}
