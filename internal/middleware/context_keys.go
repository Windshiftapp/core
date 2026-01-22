package middleware

import "windshift/internal/contextkeys"

// ContextKey is aliased from contextkeys for backward compatibility
type ContextKey = contextkeys.ContextKey

// Context key constants - aliased from contextkeys for backward compatibility
const (
	ContextKeyUser       = contextkeys.User
	ContextKeySession    = contextkeys.Session
	ContextKeyAPIToken   = contextkeys.APIToken
	ContextKeyAuthMethod = contextkeys.AuthMethod
	ContextKeyCSRFExempt = contextkeys.CSRFExempt
	ContextKeySCIMToken  = contextkeys.SCIMToken
)
