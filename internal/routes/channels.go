package routes

import "net/http"

// RegisterChannelRoutes registers channel-related routes (channels, notifications, webhooks).
func RegisterChannelRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()
	channelMgmt := deps.PermissionMiddleware.RequireChannelManagement()

	// Channel Category endpoints
	api.HandleH("GET /channel-categories", auth(http.HandlerFunc(deps.Channels.ChannelCategory.GetAll)))
	api.HandleH("POST /channel-categories", admin(http.HandlerFunc(deps.Channels.ChannelCategory.Create)))
	api.HandleH("GET /channel-categories/{id}", auth(http.HandlerFunc(deps.Channels.ChannelCategory.Get)))
	api.HandleH("PUT /channel-categories/{id}", admin(http.HandlerFunc(deps.Channels.ChannelCategory.Update)))
	api.HandleH("DELETE /channel-categories/{id}", admin(http.HandlerFunc(deps.Channels.ChannelCategory.Delete)))

	// Channel endpoints - Read operations
	api.HandleH("GET /channels", auth(http.HandlerFunc(deps.Channels.Channel.GetChannels)))
	api.HandleH("GET /channels/{id}", auth(http.HandlerFunc(deps.Channels.Channel.GetChannel)))
	api.HandleH("GET /channels/{id}/managers", auth(http.HandlerFunc(deps.Channels.Channel.GetChannelManagers)))

	// Channel endpoints - Write operations
	api.HandleH("POST /channels", admin(http.HandlerFunc(deps.Channels.Channel.CreateChannel)))
	api.HandleH("PUT /channels/{id}", channelMgmt(http.HandlerFunc(deps.Channels.Channel.UpdateChannel)))
	api.HandleH("DELETE /channels/{id}", channelMgmt(http.HandlerFunc(deps.Channels.Channel.DeleteChannel)))
	api.HandleH("POST /channels/{id}/test", channelMgmt(http.HandlerFunc(deps.Channels.Channel.TestChannel)))
	api.HandleH("PUT /channels/{id}/config", channelMgmt(http.HandlerFunc(deps.Channels.Channel.UpdateChannelConfig)))
	api.HandleH("POST /channels/{id}/managers", channelMgmt(http.HandlerFunc(deps.Channels.Channel.AddChannelManager)))
	api.HandleH("DELETE /channels/{id}/managers/{managerId}", channelMgmt(http.HandlerFunc(deps.Channels.Channel.RemoveChannelManager)))
	api.HandleH("POST /channels/{id}/test-config", channelMgmt(http.HandlerFunc(deps.Channels.Channel.TestChannelConfig)))
	api.HandleH("POST /channels/{id}/process-emails", auth(deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Channels.Channel.ProcessEmailsNow))))
	api.HandleH("GET /channels/{id}/email-log", channelMgmt(http.HandlerFunc(deps.Channels.Channel.GetEmailLog)))

	// Channel email OAuth endpoints
	api.HandleH("POST /channels/{id}/inline-oauth/start", channelMgmt(http.HandlerFunc(deps.Channels.Channel.StartChannelEmailOAuth)))
	api.Handle("GET /channels/inline-oauth/callback", deps.Channels.Channel.ChannelEmailOAuthCallback) // No auth - OAuth redirect

	// Request Type endpoints (channel-scoped)
	api.HandleH("GET /channels/{channel_id}/request-types", auth(http.HandlerFunc(deps.Workspaces.RequestType.GetAllForChannel)))
	api.HandleH("POST /channels/{channel_id}/request-types", auth(http.HandlerFunc(deps.Workspaces.RequestType.Create)))
	api.HandleH("GET /request-types/{id}", auth(http.HandlerFunc(deps.Workspaces.RequestType.Get)))
	api.HandleH("PUT /request-types/{id}", auth(http.HandlerFunc(deps.Workspaces.RequestType.Update)))
	api.HandleH("DELETE /request-types/{id}", auth(http.HandlerFunc(deps.Workspaces.RequestType.Delete)))
	api.HandleH("GET /request-types/{id}/fields", auth(http.HandlerFunc(deps.Workspaces.RequestType.GetFields)))
	api.HandleH("PUT /request-types/{id}/fields", auth(http.HandlerFunc(deps.Workspaces.RequestType.UpdateFields)))
	api.HandleH("GET /request-types/{id}/available-fields", auth(http.HandlerFunc(deps.Workspaces.RequestType.GetAvailableFields)))
	api.HandleH("PUT /request-types/{id}/visibility", channelMgmt(http.HandlerFunc(deps.Workspaces.RequestType.UpdateVisibility)))

	// Asset Report endpoints (channel-scoped)
	api.HandleH("GET /channels/{channel_id}/asset-reports", auth(http.HandlerFunc(deps.Channels.AssetReport.GetAllForChannel)))
	api.HandleH("POST /channels/{channel_id}/asset-reports", auth(http.HandlerFunc(deps.Channels.AssetReport.Create)))
	api.HandleH("GET /asset-reports/{id}", auth(http.HandlerFunc(deps.Channels.AssetReport.Get)))
	api.HandleH("PUT /asset-reports/{id}", auth(http.HandlerFunc(deps.Channels.AssetReport.Update)))
	api.HandleH("DELETE /asset-reports/{id}", auth(http.HandlerFunc(deps.Channels.AssetReport.Delete)))
	api.HandleH("PUT /asset-reports/{id}/visibility", channelMgmt(http.HandlerFunc(deps.Channels.AssetReport.UpdateVisibility)))

	// Notification endpoints
	api.HandleH("GET /notifications", auth(http.HandlerFunc(deps.Channels.Notification.GetNotifications)))
	api.HandleH("POST /notifications", auth(http.HandlerFunc(deps.Channels.Notification.CreateNotification)))
	api.HandleH("PATCH /notifications/{id}/read", auth(http.HandlerFunc(deps.Channels.Notification.MarkNotificationAsRead)))
	api.HandleH("POST /notifications/refresh-cache", admin(http.HandlerFunc(deps.Channels.Notification.RefreshCache)))

	// Notification template endpoints
	api.HandleH("GET /notification-templates", admin(http.HandlerFunc(deps.Channels.NotificationTemplate.GetAllTemplates)))
	api.HandleH("POST /notification-templates", admin(http.HandlerFunc(deps.Channels.NotificationTemplate.CreateTemplate)))
	api.HandleH("GET /notification-templates/{id}", admin(http.HandlerFunc(deps.Channels.NotificationTemplate.GetTemplate)))
	api.HandleH("PUT /notification-templates/{id}", admin(http.HandlerFunc(deps.Channels.NotificationTemplate.UpdateTemplate)))
	api.HandleH("DELETE /notification-templates/{id}", admin(http.HandlerFunc(deps.Channels.NotificationTemplate.DeleteTemplate)))

	// Webhook manual trigger endpoints
	api.HandleH("POST /webhooks/{webhookId}/trigger", auth(deps.WebhookLimiter.Limit(http.HandlerFunc(deps.Channels.Webhook.TriggerWebhook))))
	api.HandleH("GET /items/{id}/webhooks", auth(http.HandlerFunc(deps.Channels.Webhook.GetWebhooksForItem)))
}
