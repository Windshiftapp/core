package server

import "context"

type contextKey int

const contextKeyCSPNonce contextKey = iota

// CSPNonceFromContext retrieves the CSP nonce stored in the request context.
func CSPNonceFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(contextKeyCSPNonce).(string); ok {
		return v
	}
	return ""
}
