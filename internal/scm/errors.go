package scm

import "errors"

// Common SCM errors
var (
	ErrUnsupportedProvider = errors.New("unsupported SCM provider")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrNotAuthenticated    = errors.New("not authenticated")
	ErrTokenExpired        = errors.New("token expired")
	ErrRateLimited         = errors.New("rate limited")
	ErrNotFound            = errors.New("resource not found")
	ErrForbidden           = errors.New("access forbidden")
	ErrInvalidWebhook      = errors.New("invalid webhook signature")
	ErrProviderError       = errors.New("provider error")
	ErrAlreadyExists       = errors.New("resource already exists")
	ErrUserSCMNotConnected = errors.New("user has not connected their SCM account")

	// ErrRepositoryNotInWorkspace is returned by issue-sync helpers that take a
	// workspaceRepositoryID together with a workspaceID when the repo does not
	// belong to that workspace. Handlers should map this to 404 Not Found so a
	// user with access to workspace A cannot probe IDs from workspace B.
	ErrRepositoryNotInWorkspace = errors.New("repository not in workspace")

	// ErrSyncConfigExists is returned by CreateSyncConfig when the workspace
	// already has an issue sync configuration. The workspace-scoped API expects
	// at most one config per workspace.
	ErrSyncConfigExists = errors.New("issue sync config already exists for workspace")

	// ErrRefreshTokenInvalid signals that the refresh token cannot be used
	// to obtain a new access token — typically because the user revoked the
	// authorization, the OAuth app's secrets rotated, or (Gitea-specific)
	// because a concurrent refresh already consumed the rotation. This is a
	// terminal condition: callers should treat the credentials as dead and
	// require a fresh user-driven OAuth connect.
	ErrRefreshTokenInvalid = errors.New("refresh token invalid")
)
