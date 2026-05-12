package routes

import "net/http"

// RegisterPortalRoutes registers portal-related routes.
func RegisterPortalRoutes(deps *Deps) {
	api := deps.API
	customersPerm := deps.PermissionMiddleware.RequireGlobalPermission("customers.manage")

	// Public portal attachment endpoint (for logos, backgrounds - no auth required)
	api.Handle("GET /portal-assets/{id}", deps.Portal.Portal.DownloadPortalAttachment)

	// Public portal API endpoints (OptionalAuth - work both authenticated and unauthenticated)
	api.Handle("GET /portal/{slug}", deps.Portal.Portal.GetPortal)
	api.Handle("GET /portal/{slug}/request-types", deps.Portal.Portal.GetRequestTypes)
	api.Handle("GET /portal/{slug}/asset-reports", deps.Portal.Portal.GetAssetReports)
	api.HandleH("GET /portal/{slug}/asset-reports/{id}/execute", deps.PortalSearchLimiter.Limit(http.HandlerFunc(deps.Portal.Portal.ExecuteAssetReport)))
	api.HandleH("POST /portal/{slug}/asset-reports/{id}/execute", deps.PortalSearchLimiter.Limit(http.HandlerFunc(deps.Portal.Portal.ExecuteAssetReport)))
	api.HandleH("GET /portal/{slug}/asset-reports/{id}/fields", http.HandlerFunc(deps.Portal.Portal.GetAssetReportFields))
	api.HandleH("POST /portal/{slug}/knowledge-base/search", deps.PortalSearchLimiter.Limit(http.HandlerFunc(deps.Portal.Portal.SearchKnowledgeBase)))

	// Portal customer authentication endpoints (magic link)
	if deps.Portal.PortalAuth != nil {
		api.HandleH("POST /portal/{slug}/auth/request", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalAuth.RequestMagicLink)))
		api.HandleH("GET /portal/{slug}/auth/verify", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalAuth.VerifyMagicLink)))
		api.HandleH("POST /portal/{slug}/auth/logout", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalAuth.Logout)))
		api.HandleH("GET /portal/{slug}/auth/me", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalAuth.GetCurrentCustomer)))
	}

	// Public portal passkey login endpoints (rate-limited, no auth)
	if deps.Portal.PortalWebAuthn != nil {
		api.HandleH("POST /portal/{slug}/auth/webauthn/login/start", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalWebAuthn.StartPortalLogin)))
		api.HandleH("POST /portal/{slug}/auth/webauthn/login/complete", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalWebAuthn.CompletePortalLogin)))
	}

	// Portal-authenticated endpoints (accept both internal and portal sessions)
	if deps.PortalAuthMiddleware != nil {
		portalAuth := deps.PortalAuthMiddleware.RequirePortalAuth

		// Request type fields and custom fields
		api.HandleH("GET /portal/{slug}/request-types/{id}/fields", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetRequestTypeFields)))
		api.HandleH("GET /portal/{slug}/custom-fields", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetCustomFields)))

		// Submission (with rate limiting)
		api.HandleH("POST /portal/{slug}/submit", deps.PortalSubmitLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.SubmitToPortal))))

		// Request tracking endpoints
		api.HandleH("GET /portal/{slug}/my-requests", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetMyRequests)))
		api.HandleH("GET /portal/{slug}/requests/{itemId}", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetRequestDetail)))
		api.HandleH("GET /portal/{slug}/requests/{itemId}/comments", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetRequestComments)))
		api.HandleH("POST /portal/{slug}/requests/{itemId}/comments", deps.PortalSubmitLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.AddRequestComment))))

		// Portal request form drafts — at most one per (caller, request type).
		// Writes share the submit rate limiter; reads are unmetered like the
		// rest of the my-requests surface.
		api.HandleH("POST /portal/{slug}/drafts", deps.PortalSubmitLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.SaveDraft))))
		api.HandleH("GET /portal/{slug}/drafts", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetMyDrafts)))
		api.HandleH("GET /portal/{slug}/drafts/{requestTypeId}", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetDraftByRequestType)))
		api.HandleH("DELETE /portal/{slug}/drafts/{requestTypeId}", portalAuth(http.HandlerFunc(deps.Portal.Portal.DeleteDraft)))

		// Portal-side approval endpoints — mirror /api/approvals/* for the
		// active portal customer (or internal user with a portal_customers.user_id link).
		api.HandleH("GET /portal/{slug}/approvals/mine", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetMyApprovals)))
		api.HandleH("GET /portal/{slug}/approvals/{id}", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetApproval)))
		api.HandleH("POST /portal/{slug}/approvals/{id}/decide", portalAuth(http.HandlerFunc(deps.Portal.Portal.DecideAsPortalCustomer)))

		// Portal customer passkey management (requires portal customer session;
		// internal users hit StatusBadRequest from requirePortalCustomer).
		if deps.Portal.PortalWebAuthn != nil {
			api.HandleH("POST /portal/{slug}/credentials/webauthn/register/start", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.StartPortalRegistration)))
			api.HandleH("POST /portal/{slug}/credentials/webauthn/register/complete", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.CompletePortalRegistration)))
			api.HandleH("GET /portal/{slug}/credentials/webauthn", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.GetPortalCredentials)))
			api.HandleH("DELETE /portal/{slug}/credentials/webauthn/{credentialId}", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.RemovePortalCredential)))
			api.HandleH("POST /portal/{slug}/passkey-prompt/dismiss", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.DismissPasskeyPrompt)))
		}
	}

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

	//nolint:misspell // British spelling used in database
	// Customer Organisations Management
	auth := deps.AuthMiddleware.RequireAuth
	api.HandleH("GET /customer-organisations", auth(http.HandlerFunc(deps.TimeTracking.Customer.GetAll)))
	api.HandleH("POST /customer-organisations", customersPerm(http.HandlerFunc(deps.TimeTracking.Customer.Create)))
	api.HandleH("GET /customer-organisations/{id}", auth(http.HandlerFunc(deps.TimeTracking.Customer.Get)))
	api.HandleH("PUT /customer-organisations/{id}", customersPerm(http.HandlerFunc(deps.TimeTracking.Customer.Update)))
	api.HandleH("DELETE /customer-organisations/{id}", customersPerm(http.HandlerFunc(deps.TimeTracking.Customer.Delete)))
	// Sub-resources use handler-level CanView gating so listed members + managers see them,
	// not just global customers.manage holders.
	api.HandleH("GET /customer-organisations/{id}/contacts", auth(http.HandlerFunc(deps.Portal.PortalCustomer.GetOrganisationContacts)))
	api.HandleH("GET /customer-organisations/{id}/tickets", auth(http.HandlerFunc(deps.Portal.PortalCustomer.GetOrganisationTickets)))
	api.HandleH("GET /customer-organisations/{id}/projects", auth(http.HandlerFunc(deps.TimeTracking.Project.GetByCustomer)))

	// Per-org ACL: customer-organisation member/manager management
	//nolint:misspell // matches the British spelling used in the SQL identifier above
	api.HandleH("GET /customer-organisations/{id}/members", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.GetMembers)))
	api.HandleH("POST /customer-organisations/{id}/members", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.AddMember)))
	api.HandleH("DELETE /customer-organisations/{id}/members/{memberId}", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.RemoveMember)))
	api.HandleH("GET /customer-organisations/{id}/managers", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.GetManagers)))
	api.HandleH("POST /customer-organisations/{id}/managers", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.AddManager)))
	api.HandleH("DELETE /customer-organisations/{id}/managers/{managerId}", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.RemoveManager)))

	// Portal Hub endpoints (for internal users)
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	if deps.Portal.Hub != nil {
		api.HandleH("GET /hub", auth(http.HandlerFunc(deps.Portal.Hub.GetHub)))
		api.HandleH("PUT /hub/config", admin(http.HandlerFunc(deps.Portal.Hub.UpdateHubConfig)))
		api.HandleH("GET /hub/inbox", auth(http.HandlerFunc(deps.Portal.Hub.GetHubInbox)))
		api.HandleH("GET /hub/inbox/{itemId}", auth(http.HandlerFunc(deps.Portal.Hub.GetHubInboxItem)))
	}
}
