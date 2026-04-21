package middleware

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// csrfExemptPaths lists API paths that carry their own replay protection and
// are invoked from non-browser contexts, so Sec-Fetch-Site / Origin is
// unavailable. Every entry here must have its own defense — cryptographic
// state, single-use code, or bearer token. Be conservative: browser-called
// endpoints must NOT appear here.
var csrfExemptPaths = map[string]bool{
	// Redeems a one-time code minted by /cli/auth/approve; the code itself
	// is the CSRF defense (state round-trip + single-use guard).
	"/api/cli/auth/exchange": true,
}

// CSRFProtection is a stateless CSRF middleware that uses the browser's
// Sec-Fetch-Site header as the primary check and falls back to Origin/Referer
// validation when the header is missing (e.g. reverse proxies stripping it).
//
// Primary check (Sec-Fetch-Site — forbidden header, cannot be spoofed by JS):
//   - "same-origin", "none": allowed
//   - "cross-site", "same-site", other: blocked (authoritative when present)
//
// Fallback (when Sec-Fetch-Site is absent):
//  1. Check Origin header against allowedOrigins
//  2. If no Origin, extract origin from Referer and check
//  3. If neither validates → block with 403
//
// Non-browser API clients should use bearer tokens, which are exempt via ContextKeyCSRFExempt.
func CSRFProtection(allowedOrigins []string) func(http.Handler) http.Handler {
	// Pre-compute a lowercased set for O(1) lookup.
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[strings.ToLower(o)] = true
	}

	return func(next http.Handler) http.Handler {
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

			// Path-based exemptions: endpoints with their own replay protection
			// that are always called from non-browser contexts.
			if csrfExemptPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			secFetchSite := r.Header.Get("Sec-Fetch-Site")

			switch secFetchSite {
			case "same-origin", "none":
				next.ServeHTTP(w, r)
				return
			case "":
				// Header missing — fall back to Origin/Referer validation.
				if checkOriginReferer(r, originSet) {
					slog.Debug("CSRF: Sec-Fetch-Site missing, allowed via Origin/Referer fallback",
						"method", r.Method,
						"path", r.URL.Path,
					)
					next.ServeHTTP(w, r)
					return
				}

				slog.Warn("CSRF: request blocked — Sec-Fetch-Site missing and Origin/Referer validation failed", //nolint:gosec // logging remote address for debugging is intentional
					"method", r.Method,
					"path", r.URL.Path,
					"origin", r.Header.Get("Origin"),
					"referer", r.Header.Get("Referer"),
					"remote_addr", r.RemoteAddr,
				)
				handleCSRFError(w, r, "Cross-site request blocked")
			default:
				// Sec-Fetch-Site is present and indicates cross-site/same-site — block
				// regardless of Origin header because the browser signal is authoritative.
				handleCSRFError(w, r, "Cross-site request blocked")
			}
		})
	}
}

// checkOriginReferer validates the request against allowed origins using the
// Origin header first, then falling back to the Referer header.
func checkOriginReferer(r *http.Request, originSet map[string]bool) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return isAllowedOrigin(origin, originSet)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return isAllowedReferer(referer, originSet)
	}
	return false
}

// isAllowedOrigin checks whether the given Origin value is in the allowed set.
// "null" origins (sent by sandboxed iframes, data: URIs, etc.) are never allowed.
func isAllowedOrigin(origin string, originSet map[string]bool) bool {
	return originSet[strings.ToLower(origin)]
}

// isAllowedReferer extracts the origin (scheme + host) from a Referer URL and
// checks it against the allowed set. Malformed URLs are rejected.
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
		_, _ = w.Write([]byte(`{"error": "` + message + `", "code": "CSRF_ERROR"}`))
		return
	}

	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(message))
}
