package middleware

// ContextKey is a typed key for context values to avoid string key collisions.
type ContextKey string

const (
	// ContextKeyUser stores the authenticated user (*models.User)
	ContextKeyUser ContextKey = "user"
	// ContextKeySession stores the session (*auth.Session)
	ContextKeySession ContextKey = "session"
	// ContextKeyAPIToken stores the API token (*models.ApiToken)
	ContextKeyAPIToken ContextKey = "api_token"
	// ContextKeyAuthMethod stores the authentication method (string: "session-header", "bearer", "cookie")
	ContextKeyAuthMethod ContextKey = "auth_method"
	// ContextKeyCSRFExempt indicates if the request is exempt from CSRF checks (bool)
	ContextKeyCSRFExempt ContextKey = "csrf_exempt"
	// ContextKeySCIMToken stores the SCIM token (*models.SCIMToken)
	ContextKeySCIMToken ContextKey = "scim_token"
)
