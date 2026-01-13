package jira

import "errors"

// Common Jira API errors
var (
	ErrInvalidCredentials  = errors.New("invalid Jira credentials")
	ErrNotAuthenticated    = errors.New("not authenticated to Jira")
	ErrRateLimited         = errors.New("Jira API rate limit exceeded")
	ErrNotFound            = errors.New("Jira resource not found")
	ErrForbidden           = errors.New("access to Jira resource forbidden")
	ErrAPIError            = errors.New("Jira API error")
	ErrAssetsNotAvailable  = errors.New("Jira Assets API not available")
	ErrInvalidURL          = errors.New("invalid Jira instance URL")
	ErrConnectionFailed    = errors.New("failed to connect to Jira")
)
