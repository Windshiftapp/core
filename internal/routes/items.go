package routes

import "net/http"

// RegisterItemRoutes registers item-related routes (items, recurrence, comments, attachments, diagrams).
func RegisterItemRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth

	// Item endpoints
	api.HandleH("GET /items", auth(http.HandlerFunc(deps.Items.Item.GetAll)))
	api.HandleH("POST /items", auth(http.HandlerFunc(deps.Items.Item.Create)))
	api.HandleH("GET /items/search", auth(http.HandlerFunc(deps.Items.Item.Search)))
	api.HandleH("GET /items/backlog", auth(http.HandlerFunc(deps.Items.Item.GetBacklogItems)))
	api.HandleH("GET /items/cache-stats", auth(http.HandlerFunc(deps.Items.Item.GetCacheStats)))
	api.HandleH("GET /items/{id}", auth(http.HandlerFunc(deps.Items.Item.Get)))
	api.HandleH("PUT /items/{id}", auth(http.HandlerFunc(deps.Items.Item.Update)))
	api.HandleH("DELETE /items/{id}", auth(http.HandlerFunc(deps.Items.Item.Delete)))
	api.HandleH("POST /items/{id}/copy", auth(http.HandlerFunc(deps.Items.Item.Copy)))
	api.HandleH("GET /items/{id}/available-status-transitions", auth(http.HandlerFunc(deps.Items.Item.GetAvailableStatusTransitions)))
	api.HandleH("GET /items/{id}/history", auth(http.HandlerFunc(deps.Items.Item.GetItemHistory)))

	// Item hierarchy endpoints
	api.HandleH("GET /items/{id}/children", auth(http.HandlerFunc(deps.Items.Item.GetChildren)))
	api.HandleH("GET /items/{id}/ancestors", auth(http.HandlerFunc(deps.Items.Item.GetAncestors)))
	api.HandleH("GET /items/{id}/descendants-new", auth(http.HandlerFunc(deps.Items.Item.GetDescendantsNew)))
	api.HandleH("GET /items/{id}/children-new", auth(http.HandlerFunc(deps.Items.Item.GetChildrenNew)))

	// Item watch endpoints
	api.HandleH("POST /items/{id}/watch", auth(http.HandlerFunc(deps.Items.Item.AddWatch)))
	api.HandleH("DELETE /items/{id}/watch", auth(http.HandlerFunc(deps.Items.Item.RemoveWatch)))
	api.HandleH("GET /items/{id}/watch", auth(http.HandlerFunc(deps.Items.Item.GetWatchStatus)))

	// Fractional indexing endpoint for manual ordering
	api.HandleH("PUT /items/{id}/frac-index", auth(http.HandlerFunc(deps.Items.Item.UpdateFracIndex)))

	// Calendar scheduling endpoints
	api.HandleH("POST /items/{id}/schedule", auth(http.HandlerFunc(deps.Items.Item.ScheduleItem)))
	api.HandleH("DELETE /items/{id}/unschedule", auth(http.HandlerFunc(deps.Items.Item.UnscheduleItem)))
	api.HandleH("GET /calendar/scheduled-items", auth(http.HandlerFunc(deps.Items.Item.GetScheduledItems)))

	// Personal task relationship endpoints
	api.HandleH("GET /items/{id}/personal-tasks", auth(http.HandlerFunc(deps.Items.Item.GetPersonalTasks)))
	api.HandleH("DELETE /items/{id}/related-work-item", auth(http.HandlerFunc(deps.Items.Item.RemoveRelatedWorkItem)))

	// Recurrence endpoints
	api.HandleH("GET /items/{id}/recurrence", auth(http.HandlerFunc(deps.Items.Recurrence.GetRecurrence)))
	api.HandleH("POST /items/{id}/recurrence", auth(http.HandlerFunc(deps.Items.Recurrence.CreateRecurrence)))
	api.HandleH("PUT /items/{id}/recurrence", auth(http.HandlerFunc(deps.Items.Recurrence.UpdateRecurrence)))
	api.HandleH("DELETE /items/{id}/recurrence", auth(http.HandlerFunc(deps.Items.Recurrence.DeleteRecurrence)))
	api.HandleH("GET /items/{id}/recurrence/instances", auth(http.HandlerFunc(deps.Items.Recurrence.ListInstances)))
	api.HandleH("POST /items/{id}/recurrence/generate", auth(http.HandlerFunc(deps.Items.Recurrence.ForceGenerate)))
	api.HandleH("POST /recurrence-rules/preview", auth(http.HandlerFunc(deps.Items.Recurrence.PreviewRRule)))

	// Comment endpoints
	api.HandleH("GET /items/{id}/comments", auth(http.HandlerFunc(deps.Items.Comment.GetComments)))
	api.HandleH("POST /items/{id}/comments", auth(http.HandlerFunc(deps.Items.Comment.CreateComment)))
	api.HandleH("PUT /comments/{id}", auth(http.HandlerFunc(deps.Items.Comment.UpdateComment)))
	api.HandleH("DELETE /comments/{id}", auth(http.HandlerFunc(deps.Items.Comment.DeleteComment)))

	// Attachment endpoints (only if enabled)
	if deps.Items.Attachment != nil {
		api.HandleH("POST /attachments/upload", auth(http.HandlerFunc(deps.Items.Attachment.Upload)))
		api.HandleH("GET /attachments/{id}/download", auth(http.HandlerFunc(deps.Items.Attachment.Download)))
		api.HandleH("GET /attachments/{id}/thumbnail", auth(http.HandlerFunc(deps.Items.Attachment.Thumbnail)))
		api.HandleH("DELETE /attachments/{id}", auth(http.HandlerFunc(deps.Items.Attachment.Delete)))
		api.HandleH("GET /items/{itemId}/attachments", auth(http.HandlerFunc(deps.Items.Attachment.GetByItem)))
	}

	// Attachment settings endpoints
	if deps.Items.AttachmentSettings != nil {
		api.HandleH("GET /attachment-settings", auth(http.HandlerFunc(deps.Items.AttachmentSettings.Get)))
		api.HandleH("PUT /attachment-settings/{id}", auth(http.HandlerFunc(deps.Items.AttachmentSettings.Update)))
		api.HandleH("GET /attachment-settings/status", auth(http.HandlerFunc(deps.Items.AttachmentSettings.GetStatus)))
	}

	// Diagram endpoints
	api.HandleH("POST /items/{itemId}/diagrams", auth(http.HandlerFunc(deps.Items.Diagram.Create)))
	api.HandleH("GET /items/{itemId}/diagrams", auth(http.HandlerFunc(deps.Items.Diagram.GetByItem)))
	api.HandleH("GET /diagrams/{id}", auth(http.HandlerFunc(deps.Items.Diagram.Get)))
	api.HandleH("PUT /diagrams/{id}", auth(http.HandlerFunc(deps.Items.Diagram.Update)))
	api.HandleH("DELETE /diagrams/{id}", auth(http.HandlerFunc(deps.Items.Diagram.Delete)))

	// Item links
	api.HandleH("POST /links", auth(http.HandlerFunc(deps.Items.ItemLink.CreateLink)))
	api.HandleH("DELETE /links/{id}", auth(http.HandlerFunc(deps.Items.ItemLink.DeleteLink)))
	api.HandleH("GET /links/search", auth(http.HandlerFunc(deps.Items.ItemLink.SearchLinkableItems)))
	api.HandleH("GET /items/{id}/links", auth(http.HandlerFunc(deps.Items.ItemLink.GetLinksForItem)))
	api.HandleH("GET /items/{id}/linked-assets", auth(http.HandlerFunc(deps.Items.ItemLink.GetLinkedAssets)))

	// Get worklogs by item
	api.HandleH("GET /items/{id}/worklogs", auth(http.HandlerFunc(deps.TimeTracking.Worklog.GetByItem)))
}
