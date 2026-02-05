package llm

import "errors"

// LLM client errors
var (
	ErrConnectionFailed = errors.New("failed to connect to LLM service")
	ErrServiceNotReady  = errors.New("LLM service is not ready")
	ErrAPIError         = errors.New("LLM API error")
	ErrNotConfigured    = errors.New("LLM service is not configured")
)
