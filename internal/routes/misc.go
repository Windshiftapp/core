package routes

import "net/http"

// RegisterMiscRoutes registers miscellaneous routes (homepage, reviews, calendar, custom fields, etc.).
func RegisterMiscRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	// Homepage endpoint
	api.HandleH("GET /homepage", auth(http.HandlerFunc(deps.Misc.Homepage.GetHomepage)))

	// Review endpoints
	api.HandleH("GET /reviews", auth(http.HandlerFunc(deps.Misc.Review.GetReviews)))
	api.HandleH("POST /reviews", auth(http.HandlerFunc(deps.Misc.Review.CreateReview)))
	api.HandleH("GET /reviews/completed-items", auth(http.HandlerFunc(deps.Misc.Review.GetCompletedItems)))
	api.HandleH("GET /reviews/{id}", auth(http.HandlerFunc(deps.Misc.Review.GetReview)))
	api.HandleH("PUT /reviews/{id}", auth(http.HandlerFunc(deps.Misc.Review.UpdateReview)))
	api.HandleH("DELETE /reviews/{id}", auth(http.HandlerFunc(deps.Misc.Review.DeleteReview)))

	// Calendar feed endpoints
	api.HandleH("GET /calendar/feed/token", auth(http.HandlerFunc(deps.Misc.CalendarFeed.GetFeedToken)))
	api.HandleH("POST /calendar/feed/token", auth(http.HandlerFunc(deps.Misc.CalendarFeed.CreateFeedToken)))
	api.HandleH("DELETE /calendar/feed/token", auth(http.HandlerFunc(deps.Misc.CalendarFeed.RevokeFeedToken)))
	// Public endpoint - uses token auth, no session required
	api.Handle("GET /calendar/feed/{token}", deps.Misc.CalendarFeed.ServeICSFeed)

	// Custom field endpoints
	api.HandleH("GET /custom-fields", auth(http.HandlerFunc(deps.Misc.CustomField.GetAll)))
	api.HandleH("POST /custom-fields", admin(http.HandlerFunc(deps.Misc.CustomField.Create)))
	api.HandleH("GET /custom-fields/{id}", auth(http.HandlerFunc(deps.Misc.CustomField.Get)))
	api.HandleH("PUT /custom-fields/{id}", admin(http.HandlerFunc(deps.Misc.CustomField.Update)))
	api.HandleH("DELETE /custom-fields/{id}", admin(http.HandlerFunc(deps.Misc.CustomField.Delete)))

	// Link type endpoints
	api.HandleH("GET /link-types", auth(http.HandlerFunc(deps.Items.LinkType.GetAll)))
	api.HandleH("POST /link-types", admin(http.HandlerFunc(deps.Items.LinkType.Create)))
	api.HandleH("GET /link-types/{id}", auth(http.HandlerFunc(deps.Items.LinkType.Get)))
	api.HandleH("PUT /link-types/{id}", admin(http.HandlerFunc(deps.Items.LinkType.Update)))
	api.HandleH("DELETE /link-types/{id}", admin(http.HandlerFunc(deps.Items.LinkType.Delete)))

	// Setup endpoints
	api.Handle("GET /setup/status", deps.Admin.Setup.GetSetupStatus)
	api.HandleH("POST /setup/complete", deps.SetupLimiter.Limit(deps.PermissionMiddleware.RequireSetupNotComplete()(http.HandlerFunc(deps.Admin.Setup.CompleteInitialSetup))))
	api.HandleH("GET /setup/modules", auth(http.HandlerFunc(deps.Admin.Setup.GetModuleSettings)))
	api.HandleH("PUT /setup/modules", admin(http.HandlerFunc(deps.Admin.Setup.UpdateModuleSettings)))

	// System endpoints
	api.HandleH("POST /shutdown", auth(http.HandlerFunc(deps.Admin.System.Shutdown)))
}
