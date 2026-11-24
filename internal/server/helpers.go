package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/utils"

	"github.com/jub0bs/cors"
)

// checkSetupStatusWithRetry checks the setup_completed status with exponential backoff retry logic.
func checkSetupStatusWithRetry(db database.Database, maxRetries int, initialDelay time.Duration) (bool, error) { //nolint:unparam // error return kept for API consistency
	delay := initialDelay

	for attempt := 1; attempt <= maxRetries; attempt++ {
		slog.Info("checking setup status", "attempt", attempt, "max_retries", maxRetries)

		query := `SELECT value FROM system_settings WHERE key = 'setup_completed'`
		var value string
		err := db.QueryRow(query).Scan(&value)

		if err == nil {
			setupCompleted := strings.EqualFold(value, "true")
			if setupCompleted {
				slog.Info("setup status: COMPLETED - server will run in production mode")
			} else {
				slog.Warn("setup status: NOT COMPLETED - server will run in setup mode")
			}
			return setupCompleted, nil
		}

		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("setup status: system_settings row missing - assuming NOT COMPLETED")
			return false, nil
		}

		slog.Warn("failed to check setup status, will retry",
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", err,
			"retry_delay", delay)

		if attempt < maxRetries {
			time.Sleep(delay)
			delay *= 2
		}
	}

	return false, nil
}

// corsErrorResponse writes a structured JSON error for CORS failures,
// matching the restapi.ErrorResponse shape the frontend already parses.
func corsErrorResponse(w http.ResponseWriter, status int, message, code string, details map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := struct {
		Error   string            `json:"error"`
		Code    string            `json:"code"`
		Details map[string]string `json:"details,omitempty"`
	}{
		Error:   message,
		Code:    code,
		Details: details,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// buildAllowedOrigins constructs a list of allowed origins from the configured
// hosts, port, and scheme. This is shared between CORS and CSRF middleware to
// ensure identical origin validation.
//
// When useProxy is true, each host without an explicit scheme prefix is also
// emitted with the opposite scheme (without port). The proxy terminates TLS,
// so the browser may hit us on either http or https and the server cannot
// know which — accepting both keeps the host as the real security boundary.
func buildAllowedOrigins(allowedHosts, serverPort, scheme string, useProxy bool) []string {
	if allowedHosts == "" {
		return nil
	}

	var origins []string
	hosts := strings.Split(allowedHosts, ",")
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}

		if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
			origins = append(origins, host)
			continue
		}

		s := scheme
		if s == "" {
			s = "https"
		}
		origin := s + "://" + host
		if serverPort != "" && !isDefaultPort(s, serverPort) {
			origin += ":" + serverPort
		}
		origins = append(origins, origin)

		if useProxy {
			other := "http"
			if s == "http" {
				other = "https"
			}
			origins = append(origins, other+"://"+host)
		}
	}
	return origins
}

func createCORSMiddleware(allowedHosts, serverPort, scheme string, disableCSRF, useProxy, allowInsecureHTTP bool) func(http.Handler) http.Handler {
	var origins []string

	if disableCSRF {
		origins = []string{"*"}
	} else {
		origins = buildAllowedOrigins(allowedHosts, serverPort, scheme, useProxy)
	}

	if len(origins) == 0 {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if origin := r.Header.Get("Origin"); origin != "" {
					slog.Warn("CORS request rejected: no origins configured",
						"origin", origin,
						"hint", "Set BASE_URL to your server's public URL")
					corsErrorResponse(w, http.StatusForbidden,
						"Origin not allowed", "CORS_ORIGIN_NOT_ALLOWED",
						map[string]string{
							"origin": origin,
							"hint":   "Set BASE_URL to your server's public URL (e.g. BASE_URL=https://myapp.example.com)",
						})
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	// jub0bs/cors rejects non-localhost http origins under credentialed mode
	// by default. Two deployments legitimately need them: behind a trusted
	// proxy we emit both http:// and https:// variants of each host (see
	// buildAllowedOrigins) because the proxy terminates TLS and the host is
	// the security boundary, and --allow-insecure-http opts a trusted-LAN /
	// testing deployment into plain http explicitly.
	if allowInsecureHTTP {
		slog.Warn("ALLOW_INSECURE_HTTP enabled: credentialed requests from non-localhost http origins are allowed; sessions are interceptable on the network path")
	}
	cfg := cors.Config{
		Origins:                            origins,
		Methods:                            []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		RequestHeaders:                     []string{"Content-Type", "Authorization"},
		Credentialed:                       !disableCSRF,
		MaxAgeInSeconds:                    86400,
		DangerouslyTolerateInsecureOrigins: useProxy || allowInsecureHTTP,
	}

	slog.Info("CORS middleware configured", "allowed_origins", origins)

	corsMw, err := cors.NewMiddleware(cfg)
	if err != nil {
		slog.Error("Failed to create CORS middleware", "error", err)
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if origin := r.Header.Get("Origin"); origin != "" {
					corsErrorResponse(w, http.StatusInternalServerError,
						"CORS configuration error", "CORS_CONFIG_ERROR",
						map[string]string{
							"hint": "Check server logs for details. Common cause: malformed hostname in BASE_URL or ALLOWED_HOSTS.",
						})
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	return corsMw.Wrap
}

func createFormEmbedCORSMiddleware(formEmbedOrigins string, firstPartyOrigins []string, fallback func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	allowedEmbedOrigins := parseOriginList(formEmbedOrigins)
	firstPartySet := make(map[string]bool, len(firstPartyOrigins))
	for _, origin := range firstPartyOrigins {
		firstPartySet[strings.ToLower(origin)] = true
	}

	return func(next http.Handler) http.Handler {
		fallbackHandler := fallback(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/forms/") {
				fallbackHandler.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			if origin == "" {
				fallbackHandler.ServeHTTP(w, r)
				return
			}

			if !isEmbedOriginAllowed(origin, allowedEmbedOrigins) {
				corsErrorResponse(w, http.StatusForbidden,
					"Origin not allowed", "FORM_EMBED_ORIGIN_NOT_ALLOWED",
					map[string]string{"origin": origin})
				return
			}

			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Cross-site form embeds are anonymous: remove browser cookies before
			// OptionalAuth runs, then exempt the submit from CSRF because there is no
			// credential for a forged request to abuse. Same-origin form/portal flows
			// keep cookies and normal CSRF protection.
			if !firstPartySet[strings.ToLower(origin)] && !originMatchesRequestHost(origin, r) {
				clone := r.Clone(r.Context())
				clone.Header = r.Header.Clone()
				clone.Header.Del("Cookie")
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/submit") {
					ctx := context.WithValue(clone.Context(), middleware.ContextKeyCSRFExempt, true)
					clone = clone.WithContext(ctx)
				}
				r = clone
			}

			next.ServeHTTP(w, r)
		})
	}
}

func originMatchesRequestHost(origin string, r *http.Request) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func parseOriginList(origins string) []string {
	var parsed []string
	for _, origin := range strings.Split(origins, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			parsed = append(parsed, strings.ToLower(origin))
		}
	}
	return parsed
}

func isEmbedOriginAllowed(origin string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	origin = strings.ToLower(origin)
	for _, allowedOrigin := range allowed {
		if allowedOrigin == "*" || allowedOrigin == origin {
			return true
		}
	}
	return false
}

func isDefaultPort(scheme, port string) bool {
	return (scheme == "https" && port == "443") || (scheme == "http" && port == "80")
}

const excalidrawFontCDNOrigin = "https://esm.sh"

func createSecurityHeadersWithTrust(enableHTTPS bool, trust utils.TrustPolicy, jiraOrigins func() []string, externalImagesAllowed func() bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Public form pages and the embed widget must be loadable cross-origin
			// so customers can iframe them into their own websites. All other routes
			// keep strict frame protection via CSP frame-ancestors.
			frameAncestors := "'self'"
			if strings.HasPrefix(r.URL.Path, "/forms/") {
				frameAncestors = "*"
			}

			// Generate a per-request cryptographic nonce for CSP script-src
			nonceBytes := make([]byte, 16)
			if _, err := rand.Read(nonceBytes); err != nil {
				slog.Error("failed to generate CSP nonce", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			nonce := base64.StdEncoding.EncodeToString(nonceBytes)

			// Store nonce in request context for downstream handlers
			ctx := context.WithValue(r.Context(), contextKeyCSPNonce, nonce)
			r = r.WithContext(ctx)

			// Keep remote images restricted unless an administrator explicitly
			// accepts the tracking and browser-network request risk.
			imgSrc := "'self' data: blob: https://images.unsplash.com https://api.atlassian.com"
			if externalImagesAllowed != nil && externalImagesAllowed() {
				imgSrc = "'self' data: blob: https: http:"
			}
			if jiraOrigins != nil {
				for _, origin := range jiraOrigins() {
					imgSrc += " " + origin
				}
			}

			csp := "default-src 'self'; " +
				"script-src 'self' 'nonce-" + nonce + "'; " +
				"style-src 'self' 'unsafe-inline'; " +
				"img-src " + imgSrc + "; " +
				"font-src 'self' " + excalidrawFontCDNOrigin + "; " +
				"connect-src 'self' " + excalidrawFontCDNOrigin + "; " +
				"media-src 'self'; " +
				"object-src 'none'; " +
				"frame-ancestors " + frameAncestors + "; " +
				"frame-src 'self'; " +
				"base-uri 'self'; " +
				"form-action 'self'"
			w.Header().Set("Content-Security-Policy", csp)

			permissionsPolicy := "geolocation=(), microphone=(), camera=(), payment=(), usb=()"
			w.Header().Set("Permissions-Policy", permissionsPolicy)

			isSecure := r.TLS != nil || enableHTTPS
			if !isSecure && trust.UseProxy {
				// PeerIP handles IPv6 brackets; a LastIndex(":") split would leave
				// "[::1]" unparseable and make HSTS disagree with cookie Secure and
				// client attribution.
				if clientIP := utils.PeerIP(r); clientIP != nil && trust.IsTrusted(clientIP) {
					if r.Header.Get("X-Forwarded-Proto") == "https" {
						isSecure = true
					}
				}
			}

			if isSecure {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
			}

			next.ServeHTTP(w, r)
		})
	}
}
