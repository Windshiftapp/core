package restapi

// Context keys for request context values
type contextKey string

const (
	ContextKeyRequestID contextKey = "request_id"
	ContextKeyUser      contextKey = "user"
	ContextKeyAPIToken  contextKey = "api_token"
	ContextKeyAuthMethod contextKey = "auth_method"
)
