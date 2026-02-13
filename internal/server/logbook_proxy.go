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
}

// NewLogbookProxy creates a reverse proxy that authenticates requests via the
// main server's auth middleware, then forwards to the logbook sidecar with
// trusted X-Logbook-* headers injected.
func NewLogbookProxy(cfg LogbookProxyConfig) http.Handler {
	target, err := url.Parse(cfg.Endpoint)
	if err != nil {
		slog.Error("invalid logbook endpoint", "endpoint", cfg.Endpoint, "error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Logbook service misconfigured", http.StatusInternalServerError)
		})
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

	// Wrap the reverse proxy with auth + header injection
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

	// Wrap with RequireAuth so the user is authenticated before we reach our handler
	return cfg.AuthMiddleware.RequireAuth(handler)
}
