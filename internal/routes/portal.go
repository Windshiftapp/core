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
}
