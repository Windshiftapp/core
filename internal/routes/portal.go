package routes

import "net/http"

// RegisterPortalRoutes registers portal-related routes.
func RegisterPortalRoutes(deps *Deps) {
	api := deps.API
	customersPerm := deps.PermissionMiddleware.RequireGlobalPermission("customers.manage")

	api.Handle("GET /portal-assets/{id}", deps.Portal.Portal.DownloadPortalAttachment)

	if deps.PortalAuthMiddleware != nil {
		api.HandleH("GET /portal/{slug}/bootstrap", deps.PortalAuthMiddleware.OptionalPortalAuth(http.HandlerFunc(deps.Portal.Portal.GetBootstrap)))
	} else {
		api.Handle("GET /portal/{slug}/bootstrap", deps.Portal.Portal.GetBootstrap)
	}

	if deps.Portal.PortalAuth != nil {
		api.HandleH("POST /portal/{slug}/auth/request", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalAuth.RequestMagicLink)))
		api.HandleH("GET /portal/{slug}/auth/verify", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalAuth.VerifyMagicLink)))
		api.HandleH("POST /portal/{slug}/auth/logout", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalAuth.Logout)))
		api.HandleH("GET /portal/{slug}/auth/me", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalAuth.GetCurrentCustomer)))
	}

	if deps.Portal.PortalWebAuthn != nil {
		api.HandleH("POST /portal/{slug}/auth/webauthn/login/start", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalWebAuthn.StartPortalLogin)))
		api.HandleH("POST /portal/{slug}/auth/webauthn/login/complete", deps.PortalAuthLimiter.Limit(http.HandlerFunc(deps.Portal.PortalWebAuthn.CompletePortalLogin)))
	}

	if deps.PortalAuthMiddleware != nil {
		portalAuth := deps.PortalAuthMiddleware.RequirePortalAuth
		api.HandleH("GET /portal/{slug}/user-bootstrap", deps.PortalAuthMiddleware.OptionalPortalAuth(http.HandlerFunc(deps.Portal.Portal.GetUserBootstrap)))

		api.HandleH("GET /portal/{slug}", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetPortal)))
		api.HandleH("GET /portal/{slug}/request-types", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetRequestTypes)))
		api.HandleH("GET /portal/{slug}/asset-reports", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetAssetReports)))
		api.HandleH("GET /portal/{slug}/asset-reports/{id}/execute", deps.PortalSearchLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.ExecuteAssetReport))))
		api.HandleH("POST /portal/{slug}/asset-reports/{id}/execute", deps.PortalSearchLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.ExecuteAssetReport))))
		api.HandleH("GET /portal/{slug}/asset-reports/{id}/fields", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetAssetReportFields)))
		api.HandleH("GET /portal/{slug}/assets/search", deps.PortalSearchLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.SearchPortalAssets))))
		api.HandleH("POST /portal/{slug}/assets/summaries", deps.PortalSearchLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.PortalAssetSummaries))))
		api.HandleH("GET /portal/{slug}/assets/{id}", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetPortalAsset)))
		api.HandleH("POST /portal/{slug}/knowledge-base/search", deps.PortalSearchLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.SearchKnowledgeBase))))
		api.HandleH("GET /portal/{slug}/knowledge-base/pages", deps.PortalSearchLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.ListKnowledgeBasePages))))
		api.HandleH("GET /portal/{slug}/knowledge-base/pages/{pageId}", deps.PortalSearchLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.GetKnowledgeBasePage))))

		api.HandleH("GET /portal/{slug}/request-types/{id}/fields", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetRequestTypeFields)))
		api.HandleH("GET /portal/{slug}/custom-fields", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetCustomFields)))

		api.HandleH("POST /portal/{slug}/submit", deps.PortalSubmitLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.SubmitToPortal))))

		api.HandleH("GET /portal/{slug}/my-requests", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetMyRequests)))
		api.HandleH("GET /portal/{slug}/requests/{itemId}", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetRequestDetail)))
		api.HandleH("GET /portal/{slug}/requests/{itemId}/comments", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetRequestComments)))
		api.HandleH("POST /portal/{slug}/requests/{itemId}/comments", deps.PortalSubmitLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.AddRequestComment))))

		// Draft writes share the submission rate limiter.
		api.HandleH("POST /portal/{slug}/drafts", deps.PortalSubmitLimiter.Limit(portalAuth(http.HandlerFunc(deps.Portal.Portal.SaveDraft))))
		api.HandleH("GET /portal/{slug}/drafts", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetMyDrafts)))
		api.HandleH("GET /portal/{slug}/drafts/{requestTypeId}", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetDraftByRequestType)))
		api.HandleH("DELETE /portal/{slug}/drafts/{requestTypeId}", portalAuth(http.HandlerFunc(deps.Portal.Portal.DeleteDraft)))

		// Approval routes accept the active portal customer or linked user.
		api.HandleH("GET /portal/{slug}/approvals/mine", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetMyApprovals)))
		api.HandleH("GET /portal/{slug}/approvals/{id}", portalAuth(http.HandlerFunc(deps.Portal.Portal.GetApproval)))
		api.HandleH("POST /portal/{slug}/approvals/{id}/decide", portalAuth(http.HandlerFunc(deps.Portal.Portal.DecideAsPortalCustomer)))

		// Passkey management requires a portal-customer session.
		if deps.Portal.PortalWebAuthn != nil {
			api.HandleH("POST /portal/{slug}/credentials/webauthn/register/start", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.StartPortalRegistration)))
			api.HandleH("POST /portal/{slug}/credentials/webauthn/register/complete", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.CompletePortalRegistration)))
			api.HandleH("GET /portal/{slug}/credentials/webauthn", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.GetPortalCredentials)))
			api.HandleH("DELETE /portal/{slug}/credentials/webauthn/{credentialId}", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.RemovePortalCredential)))
			api.HandleH("POST /portal/{slug}/passkey-prompt/dismiss", portalAuth(http.HandlerFunc(deps.Portal.PortalWebAuthn.DismissPasskeyPrompt)))
		}
	}

	api.HandleH("GET /portal-customers", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.GetPortalCustomers)))
	api.HandleH("POST /portal-customers", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.CreatePortalCustomer)))
	api.HandleH("GET /portal-customers/{id}", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.GetPortalCustomer)))
	api.HandleH("PUT /portal-customers/{id}", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.UpdatePortalCustomer)))
	api.HandleH("GET /portal-customers/{id}/channels", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.GetCustomerChannels)))
	api.HandleH("GET /portal-customers/{id}/submissions", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.GetCustomerSubmissions)))
	api.HandleH("PUT /portal-customers/{id}/organisation", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.UpdatePortalCustomerOrganisation)))
	api.HandleH("DELETE /portal-customers/{id}", customersPerm(http.HandlerFunc(deps.Portal.PortalCustomer.DeletePortalCustomer)))

	api.HandleH("GET /contact-roles", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.GetAll)))
	api.HandleH("POST /contact-roles", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.Create)))
	api.HandleH("GET /contact-roles/{id}", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.Get)))
	api.HandleH("PUT /contact-roles/{id}", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.Update)))
	api.HandleH("DELETE /contact-roles/{id}", customersPerm(http.HandlerFunc(deps.Portal.ContactRole.Delete)))

	//nolint:misspell // British spelling used in database
	auth := deps.AuthMiddleware.RequireAuth
	api.HandleH("GET /customer-organisations", auth(http.HandlerFunc(deps.TimeTracking.Customer.GetAll)))
	api.HandleH("POST /customer-organisations", customersPerm(http.HandlerFunc(deps.TimeTracking.Customer.Create)))
	api.HandleH("GET /customer-organisations/{id}", auth(http.HandlerFunc(deps.TimeTracking.Customer.Get)))
	api.HandleH("PUT /customer-organisations/{id}", customersPerm(http.HandlerFunc(deps.TimeTracking.Customer.Update)))
	api.HandleH("DELETE /customer-organisations/{id}", customersPerm(http.HandlerFunc(deps.TimeTracking.Customer.Delete)))
	// Handler-level access permits organization members and managers.
	api.HandleH("GET /customer-organisations/{id}/contacts", auth(http.HandlerFunc(deps.Portal.PortalCustomer.GetOrganisationContacts)))
	api.HandleH("GET /customer-organisations/{id}/tickets", auth(http.HandlerFunc(deps.Portal.PortalCustomer.GetOrganisationTickets)))
	api.HandleH("GET /customer-organisations/{id}/projects", auth(http.HandlerFunc(deps.TimeTracking.Project.GetByCustomer)))

	//nolint:misspell // matches the British spelling used in the SQL identifier above
	api.HandleH("GET /customer-organisations/{id}/members", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.GetMembers)))
	api.HandleH("POST /customer-organisations/{id}/members", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.AddMember)))
	api.HandleH("DELETE /customer-organisations/{id}/members/{memberId}", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.RemoveMember)))
	api.HandleH("GET /customer-organisations/{id}/managers", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.GetManagers)))
	api.HandleH("POST /customer-organisations/{id}/managers", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.AddManager)))
	api.HandleH("DELETE /customer-organisations/{id}/managers/{managerId}", auth(http.HandlerFunc(deps.TimeTracking.CustomerPermission.RemoveManager)))

	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	if deps.Portal.Hub != nil {
		api.HandleH("GET /hub", auth(http.HandlerFunc(deps.Portal.Hub.GetHub)))
		api.HandleH("PUT /hub/config", admin(http.HandlerFunc(deps.Portal.Hub.UpdateHubConfig)))
		api.HandleH("GET /hub/inbox", auth(http.HandlerFunc(deps.Portal.Hub.GetHubInbox)))
		api.HandleH("GET /hub/inbox/{itemId}", auth(http.HandlerFunc(deps.Portal.Hub.GetHubInboxItem)))
	}
}
