package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
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

		if err == sql.ErrNoRows {
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

func createCORSMiddleware(allowedHosts, serverPort, scheme string, disableCSRF, useProxy bool) func(http.Handler) http.Handler {
	var origins []string

	if disableCSRF {
		origins = []string{"*"}
	} else if allowedHosts != "" {
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
		}
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

	cfg := cors.Config{
		Origins:         origins,
		Methods:         []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		RequestHeaders:  []string{"Content-Type", "Authorization"},
		Credentialed:    !disableCSRF,
		MaxAgeInSeconds: 86400,
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

func isDefaultPort(scheme, port string) bool {
	return (scheme == "https" && port == "443") || (scheme == "http" && port == "80")
}

func createSecurityHeaders(enableHTTPS, useProxy bool, additionalProxies []net.IP) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

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

			csp := "default-src 'self'; " +
				"script-src 'self' 'nonce-" + nonce + "'; " +
				"style-src 'self' 'unsafe-inline'; " +
				"img-src 'self' data: blob: https://images.unsplash.com; " +
				"font-src 'self'; " +
				"connect-src 'self'; " +
				"media-src 'self'; " +
				"object-src 'none'; " +
				"frame-ancestors 'self'; " +
				"frame-src 'self'; " +
				"base-uri 'self'; " +
				"form-action 'self'"
			w.Header().Set("Content-Security-Policy", csp)

			permissionsPolicy := "geolocation=(), microphone=(), camera=(), payment=(), usb=()"
			w.Header().Set("Permissions-Policy", permissionsPolicy)

			isSecure := r.TLS != nil || enableHTTPS
			if !isSecure && useProxy {
				remoteAddr := r.RemoteAddr
				if colonIndex := strings.LastIndex(remoteAddr, ":"); colonIndex != -1 {
					remoteAddr = remoteAddr[:colonIndex]
				}
				clientIP := net.ParseIP(remoteAddr)
				if clientIP != nil && utils.IsTrustedProxy(clientIP, useProxy, additionalProxies) {
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
