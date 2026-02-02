package contextkeys

// ContextKey is a typed key for context values to avoid string key collisions.
type ContextKey string

const (
	// User stores the authenticated user (*models.User)
	User ContextKey = "user"
	// Session stores the session (*auth.Session)
	Session ContextKey = "session"
	// APIToken stores the API token (*models.ApiToken)
	APIToken ContextKey = "api_token"
	// AuthMethod stores the authentication method (string: "session-header", "bearer", "cookie")
	AuthMethod ContextKey = "auth_method"
	// CSRFExempt indicates if the request is exempt from CSRF checks (bool)
	CSRFExempt ContextKey = "csrf_exempt"
	// SCIMToken stores the SCIM token (*models.SCIMToken)
	SCIMToken ContextKey = "scim_token"
	// PortalSession stores the portal customer session (*auth.PortalSession)
	PortalSession ContextKey = "portal_session"
	// PortalCustomerID stores the portal customer ID (int)
	PortalCustomerID ContextKey = "portal_customer_id"
)
