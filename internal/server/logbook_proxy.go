package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"windshift/internal/logbookauth"
	"windshift/internal/middleware"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// LogbookProxyConfig holds the configuration for the authenticating logbook proxy.
type LogbookProxyConfig struct {
	Endpoint          string
	AuthMiddleware    *middleware.AuthMiddleware
	PermissionService *services.PermissionService
	UploadLimiter     *middleware.RateLimiter
	// SharedSecret is the HMAC key used to sign X-Logbook-* headers so the
	// sidecar can verify requests originated from this proxy rather than
	// trusting network reachability. Sourced from SSO_SECRET.
	SharedSecret string
}

// newLogbookProxyHandler creates the inner proxy handler that strips spoofed
// headers, extracts the authenticated user, injects trusted X-Logbook-* headers,
// and forwards the request to the logbook sidecar.
func newLogbookProxyHandler(cfg LogbookProxyConfig) (http.Handler, error) {
	target, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid logbook endpoint %q: %w", cfg.Endpoint, err)
	}
	if cfg.SharedSecret == "" {
		return nil, fmt.Errorf("logbook proxy requires SharedSecret (SSO_SECRET) for request signing")
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			// Path is forwarded as-is (/api/logbook/*)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("logbook proxy error", "path", r.URL.Path, "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"Logbook service unavailable","code":"SERVICE_UNAVAILABLE"}`))
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip all incoming X-Logbook-* headers to prevent spoofing
		for key := range r.Header {
			if strings.HasPrefix(strings.ToLower(key), "x-logbook-") {
				r.Header.Del(key)
			}
		}

		// Get authenticated user from context (set by auth middleware)
		user := utils.GetCurrentUser(r)
		if user == nil {
			http.Error(w, `{"error":"Unauthorized","code":"UNAUTHORIZED"}`, http.StatusUnauthorized)
			return
		}

		// Get group memberships
		groupIDs, err := cfg.PermissionService.GetGroupMemberships(user.ID)
		if err != nil {
			slog.Error("failed to get group memberships for logbook proxy",
				"user_id", user.ID, "error", err)
			groupIDs = []int{} // Continue with empty groups rather than failing
		}

		// Build comma-separated group ID list
		groupIDStrs := make([]string, len(groupIDs))
		for i, gid := range groupIDs {
			groupIDStrs[i] = fmt.Sprintf("%d", gid)
		}

		isAdmin, err := cfg.PermissionService.IsSystemAdmin(user.ID)
		if err != nil {
			slog.Error("failed to check system admin for logbook proxy",
				"user_id", user.ID, "error", err)
			isAdmin = false // Fail closed
		}

		nonce, err := newNonce()
		if err != nil {
			slog.Error("failed to generate logbook signature nonce", "error", err)
			http.Error(w, `{"error":"Internal Server Error","code":"INTERNAL_ERROR"}`, http.StatusInternalServerError)
			return
		}

		claims := logbookauth.Claims{
			Timestamp: time.Now().Unix(),
			Method:    r.Method,
			Path:      r.URL.Path,
			Nonce:     nonce,
			UserID:    fmt.Sprintf("%d", user.ID),
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			IsAdmin:   fmt.Sprintf("%t", isAdmin),
			GroupIDs:  strings.Join(groupIDStrs, ","),
		}

		// Inject trusted headers
		r.Header.Set("X-Logbook-User-ID", claims.UserID)
		r.Header.Set("X-Logbook-User-Email", claims.Email)
		r.Header.Set("X-Logbook-User-First-Name", claims.FirstName)
		r.Header.Set("X-Logbook-User-Last-Name", claims.LastName)
		r.Header.Set("X-Logbook-Is-Admin", claims.IsAdmin)
		r.Header.Set("X-Logbook-Group-IDs", claims.GroupIDs)
		r.Header.Set(logbookauth.HeaderTimestamp, fmt.Sprintf("%d", claims.Timestamp))
		r.Header.Set(logbookauth.HeaderNonce, claims.Nonce)
		r.Header.Set(logbookauth.HeaderSignature, logbookauth.Sign(cfg.SharedSecret, claims))

		proxy.ServeHTTP(w, r)
	})

	return handler, nil
}

// NewLogbookProxy creates a reverse proxy that authenticates requests via the
// main server's auth middleware, then forwards to the logbook sidecar with
// trusted X-Logbook-* headers injected.
func NewLogbookProxy(cfg LogbookProxyConfig) http.Handler {
	handler, err := newLogbookProxyHandler(cfg)
	if err != nil {
		slog.Error("failed to create logbook proxy", "error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Logbook service misconfigured", http.StatusInternalServerError)
		})
	}
	return cfg.AuthMiddleware.RequireAuth(handler)
}

// newNonce returns a 128-bit random hex-encoded nonce for signature binding.
func newNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// NewLogbookUploadProxy creates a rate-limited reverse proxy for logbook upload
// endpoints. The middleware chain is: RequireAuth → UploadLimiter → proxy handler.
func NewLogbookUploadProxy(cfg LogbookProxyConfig) http.Handler {
	handler, err := newLogbookProxyHandler(cfg)
	if err != nil {
		slog.Error("failed to create logbook upload proxy", "error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Logbook service misconfigured", http.StatusInternalServerError)
		})
	}
	return cfg.AuthMiddleware.RequireAuth(cfg.UploadLimiter.Limit(handler))
}
