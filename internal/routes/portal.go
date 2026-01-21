package routes

import "net/http"

// RegisterPortalRoutes registers portal-related routes.
func RegisterPortalRoutes(deps *Deps) {
	api := deps.API
	customersPerm := deps.PermissionMiddleware.RequireGlobalPermission("customers.manage")

	// Public portal API endpoints (OptionalAuth - work both authenticated and unauthenticated)
	api.Handle("GET /portal/{slug}", deps.Portal.Portal.GetPortal)
	api.HandleH("POST /portal/{slug}/submit", deps.PortalSubmitLimiter.Limit(http.HandlerFunc(deps.Portal.Portal.SubmitToPortal)))
	api.HandleH("POST /portal/{slug}/knowledge-base/search", deps.PortalSearchLimiter.Limit(http.HandlerFunc(deps.Portal.Portal.SearchKnowledgeBase)))

	// Portal customer authentication endpoints (magic link)
	if deps.Portal.PortalAuth != nil {
		api.HandleH("POST /portal/{slug}/auth/request", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalAuth.RequestMagicLink)))
		api.Handle("GET /portal/{slug}/auth/verify", deps.Portal.PortalAuth.VerifyMagicLink)
		api.Handle("POST /portal/{slug}/auth/logout", deps.Portal.PortalAuth.Logout)
		api.Handle("GET /portal/{slug}/auth/me", deps.Portal.PortalAuth.GetCurrentCustomer)
	}

	// Portal request tracking endpoints (OptionalAuth)
	api.Handle("GET /portal/{slug}/my-requests", deps.Portal.Portal.GetMyRequests)
	api.Handle("GET /portal/{slug}/requests/{itemId}", deps.Portal.Portal.GetRequestDetail)
	api.Handle("GET /portal/{slug}/requests/{itemId}/comments", deps.Portal.Portal.GetRequestComments)
	api.Handle("POST /portal/{slug}/requests/{itemId}/comments", deps.Portal.Portal.AddRequestComment)

	// Portal Customer Management
	api.HandleH("GET /portal-customers", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.GetPortalCustomers)))
	api.HandleH("POST /portal-customers", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.CreatePortalCustomer)))
	api.HandleH("GET /portal-customers/{id}", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.GetPortalCustomer)))
	api.HandleH("PUT /portal-customers/{id}", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.UpdatePortalCustomer)))
	api.HandleH("GET /portal-customers/{id}/channels", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.GetCustomerChannels)))
	api.HandleH("GET /portal-customers/{id}/submissions", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.GetCustomerSubmissions)))
	api.HandleH("PUT /portal-customers/{id}/organisation", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.UpdatePortalCustomerOrganisation)))
	api.HandleH("DELETE /portal-customers/{id}", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.DeletePortalCustomer)))

	// Contact Roles Management
	api.HandleH("GET /contact-roles", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.GetAll)))
	api.HandleH("POST /contact-roles", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.Create)))
	api.HandleH("GET /contact-roles/{id}", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.Get)))
	api.HandleH("PUT /contact-roles/{id}", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.Update)))
	api.HandleH("DELETE /contact-roles/{id}", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.Delete)))

	// Customer Organisation Contacts
	api.HandleH("GET /customer-organisations/{id}/contacts", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.GetOrganisationContacts)))

	// Portal Hub endpoints (for internal users)
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	if deps.Portal.Hub != nil {
		api.HandleH("GET /hub", auth(http.HandlerFunc(deps.Portal.Hub.GetHub)))
		api.HandleH("PUT /hub/config", admin(http.HandlerFunc(deps.Portal.Hub.UpdateHubConfig)))
		api.HandleH("GET /hub/inbox", auth(http.HandlerFunc(deps.Portal.Hub.GetHubInbox)))
		api.HandleH("GET /hub/inbox/{itemId}", auth(http.HandlerFunc(deps.Portal.Hub.GetHubInboxItem)))
	}
}
