// Package restapi provides REST API routing and context utilities.
package restapi

import "windshift/internal/contextkeys"

// ContextKey is aliased from contextkeys for backward compatibility.
type ContextKey = contextkeys.ContextKey

const (
	ContextKeyRequestID  = contextkeys.RequestID
	ContextKeyUser       = contextkeys.User
	ContextKeyAPIToken   = contextkeys.APIToken
	ContextKeyAuthMethod = contextkeys.AuthMethod
)
