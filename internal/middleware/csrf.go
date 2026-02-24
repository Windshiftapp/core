package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// SecFetchSiteProtection is a stateless CSRF middleware that uses the browser's
// Sec-Fetch-Site header to verify request origin. This header is a "forbidden header"
// that cannot be modified by JavaScript, making it a reliable CSRF indicator.
//
// Allowed values:
//   - "same-origin": normal frontend requests from the same origin
//   - "none": direct navigation / user-initiated (e.g. bookmark, address bar)
//
// Rejected values: "same-site", "cross-site", empty (non-browser clients), unknown.
// Non-browser API clients should use bearer tokens, which are exempt via ContextKeyCSRFExempt.
func SecFetchSiteProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip CSRF check for safe methods
		if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		// Skip CSRF check if request is marked as exempt (bearer token / SCIM auth)
		if exempt, ok := r.Context().Value(ContextKeyCSRFExempt).(bool); ok && exempt {
			next.ServeHTTP(w, r)
			return
		}

		secFetchSite := r.Header.Get("Sec-Fetch-Site")

		switch secFetchSite {
		case "same-origin", "none":
			next.ServeHTTP(w, r)
		case "":
			// Empty header means non-browser client or proxy stripping the header.
			// Log a warning to help diagnose misconfigured reverse proxies.
			slog.Warn("CSRF: Sec-Fetch-Site header missing on state-changing request",
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
			)
			handleCSRFError(w, r, "Cross-site request blocked")
		default:
			handleCSRFError(w, r, "Cross-site request blocked")
		}
	})
}

// handleCSRFError handles CSRF validation errors with appropriate content type.
func handleCSRFError(w http.ResponseWriter, r *http.Request, message string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error": "` + message + `", "code": "CSRF_ERROR"}`))
		return
	}

	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(message))
}
