package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"windshift/internal/auth"
	"windshift/internal/database"
)

// AuthMiddleware handles authentication for protected routes
type AuthMiddleware struct {
	sessionManager    *auth.SessionManager
	tokenManager      *auth.TokenManager
	db                database.Database
	setupCompleted    atomic.Bool  // Thread-safe cached value
	mu                sync.RWMutex // Protects one-way state transitions
	useProxy          bool         // Whether proxy mode is enabled
	additionalProxies []net.IP     // Additional proxy IPs beyond private ranges
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(sessionManager *auth.SessionManager, tokenManager *auth.TokenManager, db database.Database, useProxy bool, additionalProxies []string, setupCompleted bool) *AuthMiddleware {
	// Parse additional proxy IPs (beyond auto-trusted private ranges)
	var additionalIPs []net.IP
	for _, proxyStr := range additionalProxies {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalIPs = append(additionalIPs, ip)
		}
	}

	am := &AuthMiddleware{
		sessionManager:    sessionManager,
		tokenManager:      tokenManager,
		db:                db,
		useProxy:          useProxy,
		additionalProxies: additionalIPs,
	}

	// Initialize setup status atomically
	am.setupCompleted.Store(setupCompleted)

	// Log security mode at initialization
	if setupCompleted {
		slog.Info("🔒 Authentication middleware initialized in PRODUCTION mode - authentication required")
	} else {
		slog.Warn("🔓 Authentication middleware initialized in SETUP mode - authentication disabled for initial configuration")
	}

	return am
}

// authResult represents the outcome of an authentication attempt
type authResult struct {
	ctx             context.Context // The context with auth info added (nil if not authenticated)
	authenticated   bool            // Whether authentication succeeded
	errorMessage    string          // Error message (only for bearer token failures)
	shouldClearCookie bool          // Whether to clear the session cookie
}

// tryAuthenticate attempts to authenticate the request using all available methods.
// Returns an authResult indicating the outcome. This method does not write any HTTP response.
func (am *AuthMiddleware) tryAuthenticate(r *http.Request) authResult {
	clientIP := am.getClientIP(r)

	// Try X-Session-Token header (used by TUI/internal services)
	if sessionToken := r.Header.Get("X-Session-Token"); sessionToken != "" {
		session, err := am.sessionManager.ValidateSession(sessionToken, clientIP)
		if err == nil {
			ctx := context.WithValue(r.Context(), ContextKeySession, session)
			ctx = context.WithValue(ctx, ContextKeyUser, session.User)
			ctx = context.WithValue(ctx, ContextKeyAuthMethod, "session-header")
			ctx = context.WithValue(ctx, ContextKeyCSRFExempt, true)
			return authResult{ctx: ctx, authenticated: true}
		}
		// Fall through to try other auth methods
	}

	// Try Bearer token (API tokens for external integrations)
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		user, apiToken, err := am.tokenManager.ValidateToken(token)
		if err != nil {
			// Bearer token was provided but invalid - this is an explicit error
			return authResult{errorMessage: "Invalid API token"}
		}
		ctx := context.WithValue(r.Context(), ContextKeyUser, user)
		ctx = context.WithValue(ctx, ContextKeyAPIToken, apiToken)
		ctx = context.WithValue(ctx, ContextKeyAuthMethod, "bearer")
		ctx = context.WithValue(ctx, ContextKeyCSRFExempt, true)
		return authResult{ctx: ctx, authenticated: true}
	}

	// Try session cookie
	token, err := am.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		// No session found
		return authResult{}
	}

	session, err := am.sessionManager.ValidateSession(token, clientIP)
	if err != nil {
		// Invalid session
		errMsg := "Authentication failed"
		switch err {
		case auth.ErrSessionNotFound:
			errMsg = "Session not found"
		case auth.ErrSessionExpired:
			errMsg = "Session expired"
		case auth.ErrInvalidSession:
			errMsg = "Invalid session"
		}
		return authResult{errorMessage: errMsg, shouldClearCookie: true}
	}

	ctx := context.WithValue(r.Context(), ContextKeySession, session)
	ctx = context.WithValue(ctx, ContextKeyUser, session.User)
	ctx = context.WithValue(ctx, ContextKeyAuthMethod, "session")
	return authResult{ctx: ctx, authenticated: true}
}

// RequireAuth middleware that requires authentication for all routes except setup
func (am *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for setup endpoints when setup is not completed
		if am.shouldSkipAuth(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Check if OptionalAuth already authenticated the user (avoid duplicate validation)
		if user := r.Context().Value(ContextKeyUser); user != nil {
			next.ServeHTTP(w, r)
			return
		}

		result := am.tryAuthenticate(r)

		if result.shouldClearCookie {
			am.sessionManager.ClearSessionCookie(w, r)
		}

		if result.authenticated {
			next.ServeHTTP(w, r.WithContext(result.ctx))
			return
		}

		// Authentication failed - return error
		errMsg := result.errorMessage
		if errMsg == "" {
			errMsg = "No session token found"
		}
		am.handleAuthError(w, r, errMsg)
	})
}

// OptionalAuth middleware that adds user context if authenticated but doesn't require it
func (am *AuthMiddleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := am.tryAuthenticate(r)

		if result.shouldClearCookie {
			am.sessionManager.ClearSessionCookie(w, r)
		}

		if result.authenticated {
			next.ServeHTTP(w, r.WithContext(result.ctx))
			return
		}

		// Not authenticated - continue without user context (optional auth)
		next.ServeHTTP(w, r)
	})
}

// shouldSkipAuth determines if authentication should be skipped for a request
func (am *AuthMiddleware) shouldSkipAuth(r *http.Request) bool {
	path := r.URL.Path

	// Always skip auth for setup endpoints
	if strings.HasPrefix(path, "/api/setup/") {
		return true
	}

	// Explicit allowlist of public authentication endpoints (login only)
	publicAuthEndpoints := map[string]bool{
		"/api/auth/login":                   true,
		"/api/auth/webauthn/login/start":    true,
		"/api/auth/webauthn/login/complete": true,
	}

	// Skip auth for static files
	if strings.HasPrefix(path, "/assets/") ||
		strings.HasPrefix(path, "/favicon.ico") ||
		strings.HasPrefix(path, "/manifest.json") ||
		strings.HasPrefix(path, "/cmicon") {
		return true
	}

	// Check if setup is completed using cached atomic value (no database query)
	// This value is determined at startup and can only transition from false→true
	setupCompleted := am.setupCompleted.Load()

	if !setupCompleted {
		return true // Setup mode - skip authentication
	}

	// For completed setup, only skip auth for specific public endpoints
	publicEndpoints := []string{
		"/api/setup/status", // Always allow checking setup status
	}

	for _, endpoint := range publicEndpoints {
		if path == endpoint {
			return true
		}
	}

	// Allow listed auth endpoints (login flows) even after setup
	if publicAuthEndpoints[path] {
		slog.Debug("skipping auth for public endpoint", slog.String("path", path))
		return true
	}

	return false
}

// MarkSetupCompleted transitions the authentication middleware from setup mode to production mode.
// This is a ONE-WAY transition (false → true) that immediately enables authentication.
// This method is called after successful setup completion (admin user creation).
func (am *AuthMiddleware) MarkSetupCompleted() {
	am.mu.Lock()
	defer am.mu.Unlock()

	// ONE-WAY transition only: false → true
	// Never allow downgrading from production to setup mode
	if !am.setupCompleted.Load() {
		am.setupCompleted.Store(true)
		slog.Warn("🔒 Setup completed - transitioning to PRODUCTION mode - authentication now required for all protected endpoints")
	}
}

// authErrorResponse represents a JSON error response for authentication failures
type authErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// handleAuthError handles authentication errors
func (am *AuthMiddleware) handleAuthError(w http.ResponseWriter, r *http.Request, message string) {
	// For API requests, return JSON error
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		response := authErrorResponse{
			Error: message,
			Code:  "AUTHENTICATION_REQUIRED",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.Error("failed to encode auth error response", slog.Any("error", err))
		}
		return
	}

	// For web requests, return 401 (frontend will handle by showing login dialog)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("Authentication required"))
}

// getClientIP extracts the client IP address from request with proxy validation
func (am *AuthMiddleware) getClientIP(r *http.Request) string {
	// Get the immediate client IP (could be proxy)
	remoteAddr := r.RemoteAddr
	if colonIndex := strings.LastIndex(remoteAddr, ":"); colonIndex != -1 {
		remoteAddr = remoteAddr[:colonIndex]
	}

	clientIP := net.ParseIP(remoteAddr)
	if clientIP == nil {
		return remoteAddr // Return as-is if parsing fails
	}

	// Only trust proxy headers if the request comes from a trusted proxy
	if am.isTrustedProxy(clientIP) {
		// Check X-Forwarded-For header (for proxies)
		forwarded := r.Header.Get("X-Forwarded-For")
		if forwarded != "" {
			// Validate and extract the first (original client) IP
			ips := strings.Split(forwarded, ",")
			for _, ipStr := range ips {
				ipStr = strings.TrimSpace(ipStr)
				if ip := net.ParseIP(ipStr); ip != nil && am.isValidClientIP(ip) {
					return ipStr
				}
			}
		}

		// Check X-Real-IP header
		realIP := r.Header.Get("X-Real-IP")
		if realIP != "" {
			if ip := net.ParseIP(realIP); ip != nil && am.isValidClientIP(ip) {
				return realIP
			}
		}
	}

	// Fall back to direct connection IP
	return remoteAddr
}

// isPrivateIP checks if an IP is a private/internal address
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// isTrustedProxy checks if an IP is a trusted proxy (private IP or in additional list)
func (am *AuthMiddleware) isTrustedProxy(ip net.IP) bool {
	if !am.useProxy {
		return false // Proxy mode disabled - trust nothing
	}
	if isPrivateIP(ip) {
		return true
	}
	for _, trustedIP := range am.additionalProxies {
		if ip.Equal(trustedIP) {
			return true
		}
	}
	return false
}

// isValidClientIP validates that an IP address is a valid client IP
func (am *AuthMiddleware) isValidClientIP(ip net.IP) bool {
	// Reject private/reserved ranges that shouldn't be forwarded
	if ip.IsLoopback() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}

	// Allow both public and private IPs (private IPs are valid in internal networks)
	return true
}

// CleanupMiddleware periodically cleans up expired sessions
func (am *AuthMiddleware) CleanupMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Run cleanup occasionally (you might want to do this in a separate goroutine or cron job)
		// For now, we'll skip automatic cleanup to avoid performance impact
		next.ServeHTTP(w, r)
	})
}

// SessionCleanupService should be called periodically to clean up expired sessions
func (am *AuthMiddleware) SessionCleanupService() {
	am.sessionManager.CleanupExpiredSessions()
}

// RequireVerifiedEmail middleware that blocks unverified users from accessing protected routes
// Users can still access verification-related endpoints even if not verified
func (am *AuthMiddleware) RequireVerifiedEmail(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow verification-related endpoints for unverified users
		path := r.URL.Path
		verificationEndpoints := []string{
			"/api/auth/verify-email",
			"/api/auth/resend-verification",
			"/api/auth/verification-status",
			"/api/auth/logout",
			"/api/auth/me", // Allow checking current user status
		}
		for _, endpoint := range verificationEndpoints {
			if path == endpoint {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Get session from context
		session, ok := r.Context().Value(ContextKeySession).(*auth.Session)
		if !ok || session == nil {
			// No session - let RequireAuth handle it
			next.ServeHTTP(w, r)
			return
		}

		// Check if user's email is verified
		// Query the database for email_verified status
		var emailVerified bool
		err := am.db.QueryRow("SELECT email_verified FROM users WHERE id = ?", session.UserID).Scan(&emailVerified)
		if err != nil {
			// Fail closed: deny access on database error for security
			slog.Error("failed to check email verification status", slog.Int("user_id", session.UserID), slog.Any("error", err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "Failed to verify email status", "code": "VERIFICATION_CHECK_FAILED"}`))
			return
		}

		if !emailVerified {
			// User's email is not verified
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error": "Email verification required", "code": "EMAIL_VERIFICATION_REQUIRED"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
