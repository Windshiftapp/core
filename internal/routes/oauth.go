package routes

import "net/http"

// RegisterOAuthRoutes registers the public OAuth 2.0 server endpoints.
//
// /info, /approve, and /deny are session-authenticated — only a logged-in
// browser can populate the consent screen and click Allow/Deny. /token is
// public (server-to-server) and rate-limited because clients post
// credentials directly to it without a session.
//
// The user-facing route `/oauth/authorize?...` is an SPA route, not an API
// endpoint — the SvelteKit frontend renders the consent page, which then
// fetches /api/oauth/authorize/info to populate.
func RegisterOAuthRoutes(deps *Deps) {
	if deps.Users.OAuth == nil {
		return
	}
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth

	api.HandleH("GET /oauth/authorize/info", auth(http.HandlerFunc(deps.Users.OAuth.AuthorizeInfo)))
	api.HandleH("POST /oauth/authorize/approve", auth(http.HandlerFunc(deps.Users.OAuth.AuthorizeApprove)))
	api.HandleH("POST /oauth/authorize/deny", auth(http.HandlerFunc(deps.Users.OAuth.AuthorizeDeny)))

	// /userinfo is the OIDC-compatible identity endpoint OAuth clients call
	// after token exchange. Bearer-token-authenticated like any other API.
	api.HandleH("GET /oauth/userinfo", auth(http.HandlerFunc(deps.Users.OAuth.Userinfo)))

	// /token is public (server-to-server). Rate-limited via the same
	// limiter the rest of the auth surface uses to deter brute-force on
	// client_secret + code.
	api.HandleH("POST /oauth/token", deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Users.OAuth.Token)))
}
