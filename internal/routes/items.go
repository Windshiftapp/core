package routes

import "net/http"

// RegisterItemRoutes registers item-related routes (items, recurrence, comments, attachments, diagrams).
func RegisterItemRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	// Session-only item endpoints. Canonical item CRUD and related operations
	// are mounted at /api/v2 by the shared v2 router.
	api.HandleH("GET /items/cache-stats", auth(http.HandlerFunc(deps.Items.Item.GetCacheStats)))
	// Item live-update stream (WI-484). Item-view gated (404 on no view); the
	// path ends in /events so it is exempt from per-user concurrency slots.
	api.HandleH("GET /items/{id}/events", auth(http.HandlerFunc(deps.Items.Item.Events)))

	// Calendar scheduling endpoints
	api.HandleH("POST /items/{id}/schedule", auth(http.HandlerFunc(deps.Items.Item.ScheduleItem)))
	api.HandleH("DELETE /items/{id}/unschedule", auth(http.HandlerFunc(deps.Items.Item.UnscheduleItem)))
	api.HandleH("GET /calendar/scheduled-items", auth(http.HandlerFunc(deps.Items.Item.GetScheduledItems)))

	// Attachment endpoints (only if enabled)
	if deps.Items.Attachment != nil {
		api.HandleH("POST /attachments/upload", auth(deps.UploadLimiter.Limit(http.HandlerFunc(deps.Items.Attachment.Upload))))
		api.HandleH("GET /attachments/{id}/download", auth(http.HandlerFunc(deps.Items.Attachment.Download)))
		api.HandleH("GET /attachments/{id}/thumbnail", auth(http.HandlerFunc(deps.Items.Attachment.Thumbnail)))
		api.HandleH("DELETE /attachments/{id}", auth(http.HandlerFunc(deps.Items.Attachment.Delete)))
	}

	// Attachment settings endpoints
	if deps.Items.AttachmentSettings != nil {
		api.HandleH("GET /attachment-settings", auth(http.HandlerFunc(deps.Items.AttachmentSettings.Get)))
		api.HandleH("PUT /attachment-settings/{id}", admin(http.HandlerFunc(deps.Items.AttachmentSettings.Update)))
		api.HandleH("GET /attachment-settings/status", auth(http.HandlerFunc(deps.Items.AttachmentSettings.GetStatus)))
	}

}
