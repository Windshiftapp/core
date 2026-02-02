package server

import (
	"database/sql"
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
func checkSetupStatusWithRetry(db database.Database, maxRetries int, initialDelay time.Duration) (bool, error) {
	delay := initialDelay

	for attempt := 1; attempt <= maxRetries; attempt++ {
		slog.Info("checking setup status", "attempt", attempt, "max_retries", maxRetries)

		query := `SELECT value FROM system_settings WHERE key = 'setup_completed'`
		var value string
		err := db.QueryRow(query).Scan(&value)

		if err == nil {
			setupCompleted := strings.ToLower(value) == "true"
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

func createCORSMiddleware(allowedHosts string, serverPort string, disableCSRF bool, useProxy bool) func(http.Handler) http.Handler {
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

			origins = append(origins, "https://"+host)

			// Only add http:// origins when NOT behind a trusted proxy
			// The jub0bs/cors library rejects insecure origins with credentialed requests
			if !useProxy {
				if serverPort != "80" && serverPort != "443" {
					origins = append(origins, "http://"+host+":"+serverPort)
				}
				origins = append(origins, "http://"+host)
			}
		}
	}

	if len(origins) == 0 {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if origin := r.Header.Get("Origin"); origin != "" {
					http.Error(w, "CORS: Origin not allowed. Configure --allowed-hosts for cross-origin requests.", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	cfg := cors.Config{
		Origins:         origins,
		Methods:         []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
		RequestHeaders:  []string{"Content-Type", "X-CSRF-Token", "Authorization"},
		Credentialed:    !disableCSRF,
		MaxAgeInSeconds: 86400,
	}

	corsMw, err := cors.NewMiddleware(cfg)
	if err != nil {
		slog.Error("Failed to create CORS middleware", "error", err)
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if origin := r.Header.Get("Origin"); origin != "" {
					http.Error(w, "CORS configuration error", http.StatusInternalServerError)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	return corsMw.Wrap
}

func createSecurityHeaders(enableHTTPS bool, useProxy bool, additionalProxies []net.IP) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			csp := "default-src 'self'; " +
				"script-src 'self' 'unsafe-inline'; " +
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
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}
