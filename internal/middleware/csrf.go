package middleware

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// csrfExemptPaths contains non-browser endpoints with independent replay
// protection. Browser endpoints must not be added here.
var csrfExemptPaths = map[string]bool{
	// A single-use CLI code provides replay protection.
	"/api/cli/auth/exchange": true,
	// A single-use native SSO code provides replay protection.
	"/api/auth/native/exchange": true,
	// Server-to-server OAuth endpoints use no session cookie.
	"/api/oauth/register": true,
	"/api/oauth/token":    true,
}

// csrfExemptPrefixes contains non-browser control planes with independent
// non-cookie credentials.
var csrfExemptPrefixes = []string{
	"/api/runner/",
	"/api/git-proxy/",
	"/api/llm-proxy/",
	"/api/http-proxy/",
	"/api/secrets/",
}

// CSRFValidator applies the shared browser request-origin policy without
// taking ownership of an HTTP response.
type CSRFValidator struct {
	origins  map[string]bool
	disabled bool
}

// NewCSRFValidator builds a reusable validator for the configured origins.
func NewCSRFValidator(allowedOrigins []string) *CSRFValidator {
	origins := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origins[strings.ToLower(origin)] = true
	}
	return &CSRFValidator{origins: origins}
}

// NewDisabledCSRFValidator builds a validator that permits every request.
func NewDisabledCSRFValidator() *CSRFValidator {
	return &CSRFValidator{disabled: true}
}

// Allows reports whether an unsafe browser request passes CSRF validation.
func (v *CSRFValidator) Allows(r *http.Request) bool {
	if v.disabled {
		return true
	}
	if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
		return true
	}
	if exempt, ok := r.Context().Value(ContextKeyCSRFExempt).(bool); ok && exempt {
		return true
	}
	if csrfExemptPaths[r.URL.Path] {
		return true
	}
	for _, prefix := range csrfExemptPrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return true
		}
	}

	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
		return checkOriginReferer(r, v.origins)
	default:
		return false
	}
}

// CSRFProtection blocks unsafe browser requests using Sec-Fetch-Site, then
// Origin or Referer when that header is absent. Non-browser clients use explicit
// exemptions with independent authentication.
func CSRFProtection(allowedOrigins []string) func(http.Handler) http.Handler {
	validator := NewCSRFValidator(allowedOrigins)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if validator.Allows(r) {
				next.ServeHTTP(w, r)
				return
			}
			slog.Warn("CSRF: request blocked", //nolint:gosec // logging remote address for debugging is intentional
				"method", r.Method,
				"path", r.URL.Path,
				"origin", r.Header.Get("Origin"),
				"referer", r.Header.Get("Referer"),
				"remote_addr", r.RemoteAddr,
			)
			handleCSRFError(w, r, "Cross-site request blocked")
		})
	}
}

// checkOriginReferer validates Origin, then Referer.
func checkOriginReferer(r *http.Request, originSet map[string]bool) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return isAllowedOrigin(origin, originSet)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return isAllowedReferer(referer, originSet)
	}
	return false
}

// isAllowedOrigin accepts only configured origins; "null" is rejected.
func isAllowedOrigin(origin string, originSet map[string]bool) bool {
	return originSet[strings.ToLower(origin)]
}

// isAllowedReferer validates a Referer's scheme and host.
func isAllowedReferer(referer string, originSet map[string]bool) bool {
	parsed, err := url.Parse(referer)
	if err != nil || parsed.Host == "" {
		return false
	}
	origin := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	return originSet[origin]
}

// handleCSRFError handles CSRF validation errors with appropriate content type.
func handleCSRFError(w http.ResponseWriter, r *http.Request, message string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":` + strconv.Quote(message) + `,"code":"CSRF_ERROR"}`))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(message))
}
