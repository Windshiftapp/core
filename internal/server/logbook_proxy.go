package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

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
}

// newLogbookProxyHandler creates the inner proxy handler that strips spoofed
// headers, extracts the authenticated user, injects trusted X-Logbook-* headers,
// and forwards the request to the logbook sidecar.
func newLogbookProxyHandler(cfg LogbookProxyConfig) (http.Handler, error) {
	target, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid logbook endpoint %q: %w", cfg.Endpoint, err)
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

		// Inject trusted headers
		r.Header.Set("X-Logbook-User-ID", fmt.Sprintf("%d", user.ID))
		r.Header.Set("X-Logbook-User-Email", user.Email)
		r.Header.Set("X-Logbook-User-First-Name", user.FirstName)
		r.Header.Set("X-Logbook-User-Last-Name", user.LastName)
		isAdmin, err := cfg.PermissionService.IsSystemAdmin(user.ID)
		if err != nil {
			slog.Error("failed to check system admin for logbook proxy",
				"user_id", user.ID, "error", err)
			isAdmin = false // Fail closed
		}
		r.Header.Set("X-Logbook-Is-Admin", fmt.Sprintf("%t", isAdmin))
		r.Header.Set("X-Logbook-Group-IDs", strings.Join(groupIDStrs, ","))

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
