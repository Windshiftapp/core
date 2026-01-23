package routes

import "net/http"

// RegisterAuthRoutes registers authentication-related routes (auth, SSO, WebAuthn).
func RegisterAuthRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	// CSRF token endpoint (OptionalAuth - works both authenticated and unauthenticated)
	api.HandleH("GET /csrf-token", deps.AuthRateLimiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if deps.DisableCSRF {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"csrf_token": ""}`))
		} else {
			deps.CSRFMiddleware.GetTokenHandler(w, r)
		}
	})))

	// Authentication endpoints with rate limiting
	api.HandleH("POST /auth/login", deps.LoginRateLimiter.Limit(http.HandlerFunc(deps.Auth.Auth.Login)))
	api.HandleH("POST /auth/logout", deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Auth.Auth.Logout)))
	api.HandleH("GET /auth/me", auth(http.HandlerFunc(deps.Auth.Auth.GetCurrentUser)))
	api.HandleH("POST /auth/refresh", deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Auth.Auth.RefreshSession)))
	api.HandleH("POST /auth/logout-all", auth(deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Auth.Auth.LogoutAll))))
	api.HandleH("POST /auth/change-password", auth(deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Auth.Auth.ChangePassword))))

	// Email verification endpoints
	api.HandleH("GET /auth/verify-email", deps.EmailVerifyLimiter.Limit(http.HandlerFunc(deps.Auth.Auth.VerifyEmail)))
	api.HandleH("POST /auth/resend-verification", deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Auth.Auth.ResendVerification)))
	api.Handle("GET /auth/verification-status", deps.Auth.Auth.GetVerificationStatus)

	// WebAuthn (FIDO) authentication endpoints
	api.HandleH("POST /auth/webauthn/login/start", deps.FIDORateLimiter.Limit(http.HandlerFunc(deps.Auth.WebAuthn.StartFIDOLoginNew)))
	api.HandleH("POST /auth/webauthn/login/complete", deps.FIDORateLimiter.Limit(http.HandlerFunc(deps.Auth.WebAuthn.CompleteFIDOLoginNew)))

	// SSO (Single Sign-On) endpoints - Public with rate limiting
	// Rate limiting prevents brute force attacks and DoS on SSO endpoints
	api.Handle("GET /sso/status", deps.Auth.SSO.GetStatus)
	api.HandleH("GET /sso/login/{slug}", deps.SSORateLimiter.Limit(http.HandlerFunc(deps.Auth.SSO.StartLogin)))
	api.HandleH("GET /sso/callback/{slug}", deps.SSORateLimiter.Limit(http.HandlerFunc(deps.Auth.SSO.Callback)))

	// SSO Admin endpoints for provider management
	api.HandleH("GET /sso/providers", admin(http.HandlerFunc(deps.Auth.SSO.ListProviders)))
	api.HandleH("POST /sso/providers", admin(http.HandlerFunc(deps.Auth.SSO.CreateProvider)))
	api.HandleH("GET /sso/providers/{id}", admin(http.HandlerFunc(deps.Auth.SSO.GetProvider)))
	api.HandleH("PUT /sso/providers/{id}", admin(http.HandlerFunc(deps.Auth.SSO.UpdateProvider)))
	api.HandleH("DELETE /sso/providers/{id}", admin(http.HandlerFunc(deps.Auth.SSO.DeleteProvider)))
	api.HandleH("POST /sso/providers/{id}/test", admin(http.HandlerFunc(deps.Auth.SSO.TestProvider)))

	// User external account endpoints
	api.HandleH("GET /sso/external-accounts", auth(http.HandlerFunc(deps.Auth.SSO.GetExternalAccounts)))
	api.HandleH("DELETE /sso/external-accounts/{id}", auth(http.HandlerFunc(deps.Auth.SSO.UnlinkExternalAccount)))
}
