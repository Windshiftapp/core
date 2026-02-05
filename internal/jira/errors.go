package jira

import "errors"

// Common Jira API errors
var (
	ErrInvalidCredentials = errors.New("invalid Jira credentials")
	ErrNotAuthenticated   = errors.New("not authenticated to Jira")
	ErrRateLimited        = errors.New("jira API rate limit exceeded")
	ErrNotFound           = errors.New("jira resource not found")
	ErrForbidden          = errors.New("access to Jira resource forbidden")
	ErrAPIError           = errors.New("jira API error")
	ErrAssetsNotAvailable = errors.New("jira assets API not available")
	ErrInvalidURL         = errors.New("invalid Jira instance URL")
	ErrConnectionFailed   = errors.New("failed to connect to Jira")
)
