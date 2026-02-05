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
)
