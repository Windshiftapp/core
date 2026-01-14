package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/handlers"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/middleware"
	"windshift/internal/plugins"
	"windshift/internal/restapi"
	v1 "windshift/internal/restapi/v1"
	"windshift/internal/scheduler"
	"windshift/internal/email"
	"windshift/internal/scm"
	"windshift/internal/services"
	"windshift/internal/smtp"
	"windshift/internal/utils"
	"windshift/internal/tui"
	"windshift/internal/webauthn"
	"windshift/internal/webhook"
	"windshift/internal/router"
	"windshift/internal/routes"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	wishbubbletea "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
)

//go:embed all:frontend/dist
var frontendFiles embed.FS

// ANSI color for startup banner
const colorTeal = "\033[38;5;37m"
const colorReset = "\033[0m"

// printBanner prints the windshift logo at startup
func printBanner() {
	logo := `
                                         x&&&&&&&&&&&&x:
                                      &&&&&&&&&&&&&&&&&&&&&&:
                                 :&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&X
                              x&&&&&&&&&&&&              x&&&&&&&&&&&&
                            &&&&&&&&&&                        :&&&&&&&&&+
                         .&&&&&&&&                                X&&&&&&&$
                        &&&&&&&                                      &&&&&&&$
                      &&&&&&&                                          &&&&&&&
                     &&&&&&     X&&&&&&&&&&&&&&+                         &&&&&&
                   X&&&&&x  &&&&&&&&&&&&&&&&&&&&&&&$                       &&&&&&
                  &&&&&&:&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&                     &&&&&&
                 &&&&&&&&&&&&&&$               .&&&&&&&&&                    X&&&&&
                +&&&&&&&&&&&:                      &&&&&&&&                   X&&&&&
                &&&&&&&&&&                            &&&&&&&                  &&&&&
               &&&&&&&&&                                &&&&&&                  &&&&&:
              :&&&&&&&X                                  &&&&&&                 .&&&&&
              &&&&&&&                                     X&&&&&                 &&&&&.
              &&&&&&:                                      X&&&&&                .&&&&X
             ;&&&&&x                                  ..;+:.&&&&&;                &&&&&
             x&&&&&                             :&&&&&&&&&&&&&&&&&                &&&&&
             X&&&&X                           &&&&&&&&&&&&&&&&&&&&                &&&&&
             X&&&&.                        :&&&&&&&&&x       &&&&:                &&&&&
             +&&&&+                       &&&&&&&.                                &&&&&
              &&&&&                     ;&&&&&&                                  .&&&&$
              &&&&&                    +&&&&&:                                   &&&&&;
              +&&&&x                   &&&&&                                     &&&&&
               &&&&&                  &&&&&                                     &&&&&+
               .&&&&&                 &&&&&                                    x&&&&&
                &&&&&&                &&&&&                                   :&&&&&
                 &&&&&&               &&&&&                                   &&&&&
                  &&&&&&              &&&&&                                 +&&&&&:
                   &&&&&&             &&&&&                                &&&&&&.
                    X&&&&&&           X&&&&&                             ;&&&&&&
                      &&&&&&X          &&&&&&                          .&&&&&&$
                       &&&&&&&         &&&&&&                        &&&&&&&
                         &&&&&&&&;       &&&&&&&                   &&&&&&&&.
                           x&&&&&&&&&      &&&&&&&&            +&&&&&&&&&
                              &&&&&&&&&&&+  x&&&&&&&&&&&&&&&&&&&&&&&&&X
                                .&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&x
                                     &&&&&&&&&&&&&&&&&&&&&&&&&&+
                                          :&&&&&&&&&&&&x
`
	fmt.Print(colorTeal)
	fmt.Print(logo)
	fmt.Print(colorReset)
	fmt.Println()
	fmt.Println(colorTeal + "                                      W I N D S H I F T" + colorReset)
	fmt.Println("                                   Work Management Platform")
	fmt.Println()
}

// checkSetupStatusWithRetry checks the setup_completed status with exponential backoff retry logic.
// This function implements fail-closed security: if the setup status cannot be determined,
// the server should refuse to start rather than potentially run in an unsafe mode.
func checkSetupStatusWithRetry(db database.Database, maxRetries int, initialDelay time.Duration) (bool, error) {
	delay := initialDelay

	for attempt := 1; attempt <= maxRetries; attempt++ {
		slog.Info("checking setup status", "attempt", attempt, "max_retries", maxRetries)

		query := `SELECT value FROM system_settings WHERE key = 'setup_completed'`
		var value string
		err := db.QueryRow(query).Scan(&value)

		if err == nil {
			// Successfully retrieved value
			setupCompleted := strings.ToLower(value) == "true"
			if setupCompleted {
				slog.Info("✓ Setup status determined: COMPLETED - server will run in production mode with authentication required")
			} else {
				slog.Warn("✓ Setup status determined: NOT COMPLETED - server will run in setup mode without authentication")
			}
			return setupCompleted, nil
		}

		if err == sql.ErrNoRows {
			// No row means setup not completed (this is expected on fresh install)
			slog.Warn("✓ Setup status determined: system_settings row missing - assuming setup NOT COMPLETED")
			return false, nil
		}

		// Database error - retry with exponential backoff
		slog.Warn("failed to check setup status, will retry",
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", err,
			"retry_delay", delay)

		if attempt < maxRetries {
			time.Sleep(delay)
			delay *= 2 // Exponential backoff
		}
	}

	// All retries exhausted - fail closed for security
	return false, fmt.Errorf("failed to determine setup status after %d attempts - refusing to start for security", maxRetries)
}

func createCORSMiddleware(allowedHosts string, serverPort string, disableCSRF bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Development mode: Allow all origins when CSRF is disabled
			if disableCSRF {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, Authorization")
				// Note: Cannot set Allow-Credentials with wildcard origin

				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}

				next.ServeHTTP(w, r)
				return
			}

			// Production mode: Strict origin validation

			// If no origin header, it's a same-origin request - allow it
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// If origin header present but no allowed hosts configured, deny
			if allowedHosts == "" {
				http.Error(w, "CORS: Origin not allowed. Configure --allowed-hosts for cross-origin requests.", http.StatusForbidden)
				return
			}

			// Check if origin is in allowed hosts list
			if !isOriginAllowed(origin, allowedHosts, serverPort) {
				http.Error(w, "CORS: Origin not allowed", http.StatusForbidden)
				return
			}

			// Origin is allowed - set CORS headers with specific origin
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isOriginAllowed(origin, allowedHosts, serverPort string) bool {
	// Parse origin URL
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}

	// Split allowed hosts
	hosts := strings.Split(allowedHosts, ",")

	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}

		// Check if host matches (with or without port)
		if originURL.Hostname() == host {
			// Check port
			originPort := originURL.Port()
			if originPort == "" {
				// Default ports based on scheme
				if originURL.Scheme == "https" {
					originPort = "443"
				} else {
					originPort = "80"
				}
			}

			// Allow if same port as server or if origin uses standard HTTP/HTTPS ports
			if originPort == serverPort || originPort == "80" || originPort == "443" {
				return true
			}
		}

		// Also check if full host:port matches
		if originURL.Host == host {
			return true
		}
	}

	return false
}

// isPrivateIP checks if an IP is a private/internal address
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// isTrustedProxy checks if a request comes from a trusted proxy
func isTrustedProxy(ip net.IP, useProxy bool, additionalProxies []net.IP) bool {
	if !useProxy {
		return false // Proxy mode disabled - trust nothing
	}
	if isPrivateIP(ip) {
		return true
	}
	for _, trusted := range additionalProxies {
		if ip.Equal(trusted) {
			return true
		}
	}
	return false
}

func createSecurityHeaders(enableHTTPS bool, useProxy bool, additionalProxies []net.IP) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// XSS Protection
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Content Type Options - prevents MIME type sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// Frame Options - prevents clickjacking (allow same-origin for plugins)
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")

			// Referrer Policy - controls referrer information
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content Security Policy - comprehensive XSS protection
			csp := "default-src 'self'; " +
				"script-src 'self' 'unsafe-inline'; " + // Allow inline scripts for Svelte (removed unsafe-eval)
				"style-src 'self' 'unsafe-inline'; " + // Allow inline styles for Tailwind
				"img-src 'self' data: blob:; " + // Allow data URLs for images
				"font-src 'self'; " +
				"connect-src 'self'; " +
				"media-src 'self'; " +
				"object-src 'none'; " +
				"frame-ancestors 'self'; " + // Allow same-origin iframes for plugins
				"frame-src 'self'; " + // Allow loading iframes from same origin
				"base-uri 'self'; " +
				"form-action 'self'"
			w.Header().Set("Content-Security-Policy", csp)

			// Permissions Policy - restrict browser features
			permissionsPolicy := "geolocation=(), microphone=(), camera=(), payment=(), usb=()"
			w.Header().Set("Permissions-Policy", permissionsPolicy)

			// HSTS header - only set for HTTPS connections (direct or via proxy)
			isSecure := r.TLS != nil || enableHTTPS
			if !isSecure && useProxy {
				// Check if request came via HTTPS through a trusted proxy
				remoteAddr := r.RemoteAddr
				if colonIndex := strings.LastIndex(remoteAddr, ":"); colonIndex != -1 {
					remoteAddr = remoteAddr[:colonIndex]
				}
				clientIP := net.ParseIP(remoteAddr)
				if clientIP != nil && isTrustedProxy(clientIP, useProxy, additionalProxies) {
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

func main() {
	// Command line flags
	var port string
	var dbPath string
	var postgresConn string
	var attachmentPath string
	var disableCSRF bool
	var allowedHosts string
	var allowedPort string
	var useProxy bool
	var additionalProxies string
	var enableSSH bool
	var sshPort string
	var sshHost string
	var sshKeyPath string
	var maxReadConns int
	var maxWriteConns int
	var logLevel string
	var logFormat string
	var tlsCertPath string
	var tlsKeyPath string
	flag.StringVar(&port, "port", "8080", "Port to run the HTTP server on")
	flag.StringVar(&port, "p", "8080", "Port to run the HTTP server on (shorthand)")
	flag.StringVar(&dbPath, "db", "windshift.db", "Database file path (SQLite)")
	flag.StringVar(&postgresConn, "postgres-connection-string", "", "PostgreSQL connection string (e.g., postgresql://user:password@localhost:5432/windshift)")
	flag.StringVar(&postgresConn, "pg-conn", "", "PostgreSQL connection string (shorthand)")
	flag.StringVar(&attachmentPath, "attachment-path", "", "Path to store attachments (enables attachment feature if specified)")
	flag.BoolVar(&disableCSRF, "no-csrf", false, "Disable CSRF protection (for development only)")
	flag.StringVar(&allowedHosts, "allowed-hosts", "", "Comma-separated list of allowed hostnames for CSRF (e.g., 192.168.1.30,myserver.local)")
	flag.StringVar(&allowedPort, "allowed-port", "", "Port for CSRF trusted origins (defaults to server port, useful for reverse proxy setups)")
	flag.BoolVar(&useProxy, "use-proxy", false, "Enable proxy mode: trust X-Forwarded-Proto from private IPs. WARNING: Only enable when behind a reverse proxy that terminates TLS. Server must NOT be directly accessible from the internet.")
	flag.StringVar(&additionalProxies, "additional-proxies", "", "Additional proxy IPs to trust beyond private ranges (requires --use-proxy)")
	flag.BoolVar(&enableSSH, "ssh", false, "Enable SSH TUI server")
	flag.StringVar(&sshPort, "ssh-port", "23234", "Port to run the SSH server on")
	flag.StringVar(&sshHost, "ssh-host", "localhost", "Host for SSH server")
	flag.StringVar(&sshKeyPath, "ssh-key", ".ssh/windshift_host_key", "Path to SSH host key file")
	flag.IntVar(&maxReadConns, "max-read-conns", 120, "Maximum number of read connections (PocketBase default: 120)")
	flag.IntVar(&maxWriteConns, "max-write-conns", 1, "Maximum number of write connections (PocketBase default: 1)")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.StringVar(&logFormat, "log-format", "text", "Log format (text, json, logfmt)")
	flag.StringVar(&tlsCertPath, "tls-cert", "", "Path to TLS certificate file (enables HTTPS)")
	flag.StringVar(&tlsKeyPath, "tls-key", "", "Path to TLS key file (enables HTTPS)")
	flag.Parse()

	// Initialize logger early, before any other operations
	logger.Init(logLevel, logFormat)

	// Print startup banner
	printBanner()

	// Check for environment variables (common in deployment environments)
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}
	if envPostgres := os.Getenv("POSTGRES_CONNECTION_STRING"); envPostgres != "" {
		postgresConn = envPostgres
	}
	if envMaxReadConns := os.Getenv("MAX_READ_CONNS"); envMaxReadConns != "" {
		if parsed, err := strconv.Atoi(envMaxReadConns); err == nil {
			maxReadConns = parsed
		}
	}
	if envMaxWriteConns := os.Getenv("MAX_WRITE_CONNS"); envMaxWriteConns != "" {
		if parsed, err := strconv.Atoi(envMaxWriteConns); err == nil {
			maxWriteConns = parsed
		}
	}

	// Additional Docker environment variables (for scratch/distroless images without shell)
	if envDBPath := os.Getenv("DB_PATH"); envDBPath != "" {
		dbPath = envDBPath
	}
	if envAttachmentPath := os.Getenv("ATTACHMENT_PATH"); envAttachmentPath != "" {
		attachmentPath = envAttachmentPath
	}
	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" {
		logLevel = envLogLevel
		logger.Init(logLevel, logFormat) // Re-init with new level
	}
	if envLogFormat := os.Getenv("LOG_FORMAT"); envLogFormat != "" {
		logFormat = envLogFormat
		logger.Init(logLevel, logFormat) // Re-init with new format
	}

	// Build PostgreSQL connection from individual env vars if not already set
	if postgresConn == "" && os.Getenv("DB_TYPE") == "postgres" {
		pgHost := os.Getenv("POSTGRES_HOST")
		if pgHost == "" {
			pgHost = "postgres"
		}
		pgPort := os.Getenv("POSTGRES_PORT")
		if pgPort == "" {
			pgPort = "5432"
		}
		pgUser := os.Getenv("POSTGRES_USER")
		if pgUser == "" {
			pgUser = "windshift"
		}
		pgPassword := os.Getenv("POSTGRES_PASSWORD")
		pgDB := os.Getenv("POSTGRES_DB")
		if pgDB == "" {
			pgDB = "windshift"
		}
		postgresConn = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable", pgUser, pgPassword, pgHost, pgPort, pgDB)
	}

	// Parse BASE_URL to derive allowed-hosts and allowed-port
	if baseURL := os.Getenv("BASE_URL"); baseURL != "" {
		parsedURL, err := url.Parse(baseURL)
		if err == nil {
			if allowedHosts == "" {
				allowedHosts = parsedURL.Hostname()
			}
			if allowedPort == "" {
				if parsedURL.Port() != "" {
					allowedPort = parsedURL.Port()
				} else if parsedURL.Scheme == "https" {
					allowedPort = "443"
				} else {
					allowedPort = "80"
				}
			}
		}
	}

	// SSH environment variables
	if os.Getenv("SSH_ENABLED") == "true" {
		enableSSH = true
	}
	if envSSHPort := os.Getenv("SSH_PORT"); envSSHPort != "" {
		sshPort = envSSHPort
	}
	if envSSHHost := os.Getenv("SSH_HOST"); envSSHHost != "" {
		sshHost = envSSHHost
	}

	// Proxy environment variables
	if os.Getenv("USE_PROXY") == "true" {
		useProxy = true
	}
	if envAdditionalProxies := os.Getenv("ADDITIONAL_PROXIES"); envAdditionalProxies != "" {
		additionalProxies = envAdditionalProxies
	}

	// Determine which database to use
	var db database.Database
	var err error

	if postgresConn != "" {
		// Use PostgreSQL if connection string is provided
		slog.Info("connecting to PostgreSQL database")
		db, err = database.NewDatabase("postgres", postgresConn, maxReadConns, maxWriteConns)
		if err != nil {
			slog.Error("failed to connect to PostgreSQL database", "error", err)
			os.Exit(1)
		}
		slog.Info("PostgreSQL database initialized", "max_read_conns", maxReadConns, "max_write_conns", maxWriteConns)
	} else {
		// Default to SQLite
		slog.Info("connecting to SQLite database", "path", dbPath)
		db, err = database.NewDatabase("sqlite3", dbPath, maxReadConns, maxWriteConns)
		if err != nil {
			slog.Error("failed to connect to SQLite database", "error", err)
			os.Exit(1)
		}
		slog.Info("SQLite database initialized", "max_read_conns", maxReadConns, "max_write_conns", maxWriteConns, "mode", "WAL")

		// Initialize audit log batcher for SQLite (batches writes every 30s)
		logger.InitAuditBatcher(db)
		defer logger.StopAuditBatcher()
	}

	defer db.Close()

	if err := db.Initialize(); err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}

	// Ensure default notification settings exist
	if err := db.EnsureDefaultNotificationSettings(); err != nil {
		slog.Warn("failed to ensure notification settings", "error", err)
		// Don't exit - this is a non-critical initialization
	}

	// Determine setup status at startup with retry logic (fail closed on errors)
	setupCompleted, err := checkSetupStatusWithRetry(db, 5, time.Second)
	if err != nil {
		slog.Error("CRITICAL: Cannot determine setup status after multiple retries - refusing to start server for security", "error", err)
		slog.Error("Fix: Ensure database is accessible and system_settings table exists, then restart")
		os.Exit(1)
	}

	// Get underlying *sql.DB for legacy handlers
	sqlDB := db.GetDB()

	// Initialize permission service for caching
	permService, err := services.NewPermissionService(db, services.PermissionCacheConfig{
		TTL:          15 * time.Minute,
		MaxCacheSize: 512, // 512MB
	})
	if err != nil {
		slog.Error("failed to initialize permission service", "error", err)
		os.Exit(1)
	}

	// Initialize activity tracker for homepage and notifications
	activityTracker, err := services.NewActivityTracker(db, services.DefaultActivityTrackerConfig())
	if err != nil {
		slog.Error("failed to initialize activity tracker", "error", err)
		os.Exit(1)
	}
	defer activityTracker.Close() // Ensure graceful shutdown with pending activity flush

	// Start daily cleanup scheduler for expired activities
	cleanupTicker := time.NewTicker(24 * time.Hour)
	cleanupStopChan := make(chan struct{})
	go func() {
		// Run initial cleanup after 1 hour to avoid startup overhead
		select {
		case <-time.After(1 * time.Hour):
			slog.Info("running initial activity cleanup")
			if err := activityTracker.CleanupExpiredActivities(); err != nil {
				slog.Error("failed to cleanup expired activities", "error", err)
			} else {
				slog.Info("initial activity cleanup completed successfully")
			}
		case <-cleanupStopChan:
			return
		}

		// Then run cleanup daily
		for {
			select {
			case <-cleanupTicker.C:
				slog.Info("running scheduled activity cleanup")
				if err := activityTracker.CleanupExpiredActivities(); err != nil {
					slog.Error("failed to cleanup expired activities", "error", err)
				} else {
					slog.Info("scheduled activity cleanup completed successfully")
				}
			case <-cleanupStopChan:
				slog.Info("cleanup scheduler stopped")
				return
			}
		}
	}()
	defer cleanupTicker.Stop()
	defer close(cleanupStopChan)

	// Determine if HTTPS is enabled (both cert and key must be provided)
	enableHTTPS := tlsCertPath != "" && tlsKeyPath != ""

	// Parse additional proxies (beyond auto-trusted private IPs)
	var additionalProxyList []string
	if additionalProxies != "" {
		additionalProxyList = strings.Split(additionalProxies, ",")
	}

	// Validate additional proxies requires use-proxy
	if additionalProxies != "" && !useProxy {
		slog.Warn("--additional-proxies ignored: --use-proxy not enabled")
	}

	// Log proxy mode status
	if useProxy {
		slog.Info("Proxy mode enabled: trusting X-Forwarded-Proto from private IPs")
		slog.Warn("⚠️  Ensure this server is NOT directly accessible from the internet")
		if additionalProxies != "" {
			slog.Info("Additional trusted proxies configured", "proxies", additionalProxies)
		}
	}

	// Create shared IP extractor for handlers that need proxy-aware IP extraction
	ipExtractor := utils.NewIPExtractor(useProxy, additionalProxyList)

	// Authentication management - need this early for middleware
	// Use secure cookies when HTTPS is enabled or detected via X-Forwarded-Proto from trusted proxies
	sessionManager := auth.NewSessionManager(db, enableHTTPS, useProxy, additionalProxyList)

	// Determine the effective port for CORS and WebAuthn origin validation
	// If --allowed-port is set, use it (for reverse proxy scenarios with non-standard ports)
	// Otherwise use the server's actual port
	effectivePort := port
	if allowedPort != "" {
		effectivePort = allowedPort
	}

	// Initialize WebAuthn configuration
	// Development mode detection: use CSRF disable flag as indicator
	isDevelopment := disableCSRF
	webAuthnConfig, err := webauthn.NewConfig("", "", nil, isDevelopment, allowedHosts, effectivePort, enableHTTPS, useProxy)
	if err != nil {
		slog.Error("failed to initialize WebAuthn configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("WebAuthn configuration initialized",
		"rp_id", webAuthnConfig.RPID,
		"rp_name", webAuthnConfig.RPName,
		"development_mode", isDevelopment,
		"origins", webAuthnConfig.RPOrigins)

	// Create rate limiters for authentication endpoints (in-memory, zero DB writes)
	// Pass proxy config so rate limiters validate X-Forwarded-For properly
	// Login: 5 requests per minute with burst of 10
	loginRateLimiter := middleware.NewRateLimiter(5.0/60.0, 10, useProxy, additionalProxyList)
	// FIDO: 10 requests per minute with burst of 15
	fidoRateLimiter := middleware.NewRateLimiter(10.0/60.0, 15, useProxy, additionalProxyList)
	// General auth: 20 requests per minute with burst of 30
	authRateLimiter := middleware.NewRateLimiter(20.0/60.0, 30, useProxy, additionalProxyList)
	// SCIM: 600 requests per minute with burst of 100
	scimRateLimiter := middleware.NewRateLimiter(10.0, 100, useProxy, additionalProxyList)
	// Portal submit: 5 requests per minute with burst of 10
	portalSubmitLimiter := middleware.NewRateLimiter(5.0/60.0, 10, useProxy, additionalProxyList)
	// Portal search: 10 requests per minute with burst of 15
	portalSearchLimiter := middleware.NewRateLimiter(10.0/60.0, 15, useProxy, additionalProxyList)
	// Email verification: 10 requests per minute with burst of 15
	emailVerifyLimiter := middleware.NewRateLimiter(10.0/60.0, 15, useProxy, additionalProxyList)
	// Setup: 5 requests per minute with burst of 10
	setupLimiter := middleware.NewRateLimiter(5.0/60.0, 10, useProxy, additionalProxyList)

	defer loginRateLimiter.Stop()
	defer scimRateLimiter.Stop()
	defer fidoRateLimiter.Stop()
	defer authRateLimiter.Stop()
	defer portalSubmitLimiter.Stop()
	defer portalSearchLimiter.Stop()
	defer emailVerifyLimiter.Stop()
	defer setupLimiter.Stop()

	// Note: emailVerificationService is initialized later after smtpSender
	var authHandler *handlers.AuthHandler

	// Initialize token tracker for batched API token last_used_at updates
	tokenTracker := services.NewTokenTracker(db, services.DefaultTokenTrackerConfig())
	defer tokenTracker.Close() // Ensure graceful shutdown with pending token update flush

	// Create shared token manager (handles bearer token authentication)
	tokenManager := auth.NewTokenManager(db, tokenTracker)

	authMiddleware := middleware.NewAuthMiddleware(sessionManager, tokenManager, db, useProxy, additionalProxyList, setupCompleted)

	// Parse additional proxy IPs for security headers
	var additionalProxyIPs []net.IP
	for _, proxyStr := range additionalProxyList {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalProxyIPs = append(additionalProxyIPs, ip)
		}
	}

	mux := http.NewServeMux()

	// Security headers will be applied as wrapper at the end (see httpServer Handler)

	// Notification management - needs the full Database interface
	notificationManager, err := handlers.NewNotificationManager(db)
	if err != nil {
		slog.Error("failed to create notification manager", "error", err)
		os.Exit(1)
	}

	// Initialize notification service for async notification processing
	notificationService := services.NewNotificationService(
		db,
		notificationManager,
		services.DefaultNotificationServiceConfig(),
	)

	// Initialize SMTP sender and notification scheduler
	smtpSender := smtp.NewNotificationSMTPSender(db)
	notificationScheduler := scheduler.NewNotificationScheduler(db, smtpSender)
	notificationScheduler.Start()
	slog.Info("notification scheduler started")

	// Initialize recurrence scheduler for recurring tasks
	recurrenceScheduler := scheduler.NewRecurrenceScheduler(db)
	recurrenceScheduler.Start()
	slog.Info("recurrence scheduler started")

	// Initialize email scheduler for inbound email channel IMAP polling
	// Uses the same encryption as SCM providers for OAuth token storage
	var emailScheduler *scheduler.EmailScheduler

	// Initialize email verification service for SSO users
	// baseURL is used to construct verification links in emails
	emailVerificationBaseURL := os.Getenv("BASE_URL")
	if emailVerificationBaseURL == "" {
		emailVerificationBaseURL = os.Getenv("PUBLIC_URL")
	}
	if emailVerificationBaseURL == "" {
		// Construct from server port (for development/local)
		emailVerificationBaseURL = fmt.Sprintf("http://localhost:%d", port)
	}
	emailVerificationService := services.NewEmailVerificationService(db, smtpSender, emailVerificationBaseURL)

	// Initialize auth handler with email verification service
	authHandler = handlers.NewAuthHandler(db, sessionManager, loginRateLimiter, permService, emailVerificationService, ipExtractor)

	itemHandler := handlers.NewItemHandler(db, permService, activityTracker, notificationService)
	customFieldHandler := handlers.NewCustomFieldHandler(db)
	workspaceFieldReqHandler := handlers.NewWorkspaceFieldRequirementHandler(db)
	workspaceHandler := handlers.NewWorkspaceHandler(db, permService, activityTracker)
	screenHandler := handlers.NewScreenHandler(db)
	configSetHandler := handlers.NewConfigurationSetHandler(db, notificationService)
	itemTypeHandler := handlers.NewItemTypeHandler(db)
	priorityHandler := handlers.NewPriorityHandler(db)

	// Generic enum handlers using the new service layer
	hierarchyLevelHandler := handlers.NewEnumHandler(
		services.NewEnumService(db, services.NewHierarchyLevelConfig()),
		func() interface{} { return &models.HierarchyLevel{} })
	// Note: request_types has specialized methods (GetAllForChannel, GetFields, UpdateFields)
	// that EnumHandler doesn't support, so it keeps its original handler
	requestTypeHandler := handlers.NewRequestTypeHandler(db)
	statusCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(db, services.NewStatusCategoryConfig()),
		func() interface{} { return &models.StatusCategory{} })
	statusHandler := handlers.NewEnumHandler(
		services.NewEnumService(db, services.NewStatusConfig()),
		func() interface{} { return &models.Status{} })
	// Keep old status handler for the custom GetNonDoneStatusIDs endpoint
	statusHandlerLegacy := handlers.NewStatusHandler(db)
	workflowHandler := handlers.NewWorkflowHandler(db)
	userHandler := handlers.NewUserHandler(db, permService)
	groupHandler := handlers.NewGroupHandler(db, permService)
	credentialHandler := handlers.NewCredentialHandler(db, permService)
	webAuthnHandler := handlers.NewWebAuthnHandler(db, permService, sessionManager, webAuthnConfig, ipExtractor)
	appTokenHandler := handlers.NewAppTokenHandler(db, permService)
	collectionHandler := handlers.NewCollectionHandler(db)
	boardConfigHandler := handlers.NewBoardConfigurationHandler(db)
	permissionHandler := handlers.NewPermissionHandlerWithCache(db, permService)
	apiTokenHandler := handlers.NewApiTokenHandler(db, tokenManager, permService)

	// SCIM handlers for user/group provisioning
	scimTokenManager := auth.NewSCIMTokenManager(db)
	scimAuthMiddleware := middleware.NewSCIMAuthMiddleware(scimTokenManager)
	scimBaseURL := os.Getenv("BASE_URL")
	if scimBaseURL == "" {
		scimBaseURL = os.Getenv("PUBLIC_URL")
	}
	if scimBaseURL == "" {
		scimBaseURL = fmt.Sprintf("http://localhost:%d", port)
	}
	scimHandler := handlers.NewSCIMHandler(db, scimBaseURL)
	scimTokenHandler := handlers.NewSCIMTokenHandler(scimTokenManager)

	permissionSetHandler := handlers.NewPermissionSetHandlerWithPool(db, permService)
	workspaceRoleHandler := handlers.NewWorkspaceRoleHandlerWithPool(db, permService)

	// Time tracking handlers
	// Note: time_customers uses EnumService, but time_projects and time_project_categories
	// have additional specialized methods (GetByCustomer, GetByWorkspace, ReorderCategories)
	// that EnumHandler doesn't support, so they keep their original handlers
	timeCustomerHandler := handlers.NewEnumHandler(
		services.NewEnumService(db, services.NewTimeCustomerConfig()),
		func() interface{} { return &models.CustomerOrganisation{} })
	timeProjectHandler := handlers.NewTimeProjectHandler(db)
	timeProjectCategoryHandler := handlers.NewTimeProjectCategoryHandler(db)
	timeWorklogHandler := handlers.NewTimeWorklogHandler(db, permService)
	activeTimerHandler := handlers.NewActiveTimerHandler(db)

	// Test management handlers
	testFolderHandler := handlers.NewTestFolderHandlerWithPool(db, permService)
	testCaseHandler := handlers.NewTestCaseHandlerWithPool(db, permService)
	testSetHandler := handlers.NewTestSetHandlerWithPool(db, permService)
	testRunTemplateHandler := handlers.NewTestRunTemplateHandlerWithPool(db, permService)
	testRunHandler := handlers.NewTestRunHandlerWithPool(db, permService)
	testSummaryHandler := handlers.NewTestSummaryHandlerWithPool(db, permService)
	// defectHandler removed - defects are now created as regular items and linked via test_result_items

	// Link management handlers
	// Note: link_types has include_inactive query param filtering that EnumHandler doesn't support
	linkTypeHandler := handlers.NewLinkTypeHandler(db)
	itemLinkHandler := handlers.NewItemLinkHandler(db, notificationService)

	// Recurrence handler for recurring tasks
	recurrenceHandler := handlers.NewRecurrenceHandler(db, recurrenceScheduler)

	milestoneCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(db, services.NewMilestoneCategoryConfig()),
		func() interface{} { return &models.MilestoneCategory{} })
	milestoneHandler := handlers.NewMilestoneHandler(db)
	channelCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(db, services.NewChannelCategoryConfig()),
		func() interface{} { return &models.ChannelCategory{} })
	collectionCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(db, services.NewCollectionCategoryConfig()),
		func() interface{} { return &models.CollectionCategory{} })
	iterationTypeHandler := handlers.NewEnumHandler(
		services.NewEnumService(db, services.NewIterationTypeConfig()),
		func() interface{} { return &models.IterationType{} })
	iterationHandler := handlers.NewIterationHandler(db)
	personalLabelHandler := handlers.NewPersonalLabelHandler(db)
	commentHandler := handlers.NewCommentHandler(db, permService, activityTracker, notificationService)
	reviewHandler := handlers.NewReviewHandler(db)
	calendarFeedHandler := handlers.NewCalendarFeedHandler(db, permService)
	securitySettingsHandler := handlers.NewSecuritySettingsHandler(db)
	themeHandler := handlers.NewThemeHandler(db)
	userPreferencesHandler := handlers.NewUserPreferencesHandler(db)
	homepageHandler := handlers.NewHomepageHandler(db, activityTracker)

	// Notification HTTP handlers
	notificationHandler := handlers.NewNotificationHandler(notificationManager, notificationService)
	notificationTemplateHandler := handlers.NewNotificationTemplateHandlerWithPool(db)

	permissionMiddleware := middleware.NewPermissionMiddleware(db)

	// CSRF middleware
	csrfMiddleware := middleware.NewCSRFMiddleware()

	// Setup handler for initial configuration (needs sessionManager and authMiddleware)
	setupHandler := handlers.NewSetupHandler(db, sessionManager, authMiddleware)

	// SSO handler for Single Sign-On
	// Pass allowedHosts, emailVerificationService, and disableCSRF (dev mode) for secure redirect URI handling
	ssoHandler := handlers.NewSSOHandler(db, sessionManager, permService, emailVerificationService, allowedHosts, disableCSRF, ipExtractor)

	// SCM provider handler for GitHub, GitLab, Gitea, Bitbucket integration
	scmProviderHandler := handlers.NewSCMProviderHandler(db)
	scmWorkspaceHandler := handlers.NewSCMWorkspaceHandler(db, scmProviderHandler.GetEncryption(), scmProviderHandler)
	scmItemLinksHandler := handlers.NewSCMItemLinksHandler(db, scmProviderHandler.GetEncryption())
	userSCMTokenHandler := handlers.NewUserSCMTokenHandler(db, scmProviderHandler.GetEncryption())

	// Asset management handlers
	assetHandler := handlers.NewAssetHandler(db, permService)
	assetTypeHandler := handlers.NewAssetTypeHandler(db, permService)
	assetCategoryHandler := handlers.NewAssetCategoryHandler(db, permService)
	assetStatusHandler := handlers.NewAssetStatusHandler(db, permService)

	// Jira import handler
	jiraImportHandler := handlers.NewJiraImportHandler(db)

	// Email provider handler for inbound email channels (OAuth + basic auth)
	// Use scimBaseURL which contains BASE_URL/PUBLIC_URL or localhost fallback
	emailProviderHandler := handlers.NewEmailProviderHandler(db, scmProviderHandler.GetEncryption(), scimBaseURL)

	// Start email scheduler now that we have the encryption from SCM provider handler
	emailCredManager := email.NewCredentialManager(db, scmProviderHandler.GetEncryption())
	emailScheduler = scheduler.NewEmailScheduler(db, emailCredManager, attachmentPath)
	emailScheduler.Start()
	slog.Info("email scheduler started (IMAP polling)")

	// SCM sync service for periodic repository synchronization
	scmSyncService := scm.NewSyncService(db, scmProviderHandler.GetEncryption())

	// Start SCM sync scheduler (every 5 minutes)
	scmSyncStopChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		slog.Info("SCM sync scheduler started (5-minute interval)")
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
				if err := scmSyncService.SyncAllRepositories(ctx); err != nil {
					slog.Error("SCM sync error", "error", err)
				}
				if err := scmSyncService.RefreshAllPRLinkStates(ctx); err != nil {
					slog.Error("PR state refresh error", "error", err)
				}
				cancel()
			case <-scmSyncStopChan:
				slog.Info("SCM sync scheduler stopped")
				return
			}
		}
	}()

	// Webhook sender
	webhookSender := webhook.NewWebhookSender(db)

	// Wire up webhook sender to item and comment handlers for auto-trigger events
	itemHandler.SetWebhookSender(webhookSender)
	commentHandler.SetWebhookSender(webhookSender)

	// Mention service for @mentions in comments and descriptions
	mentionService := services.NewMentionService(db, notificationService)
	itemHandler.SetMentionService(mentionService)
	commentHandler.SetMentionService(mentionService)

	// Channels management
	channelHandler := handlers.NewChannelHandler(db, permService, webhookSender)
	channelHandler.SetEmailScheduler(emailScheduler)
	channelHandler.SetEncryption(scmProviderHandler.GetEncryption())
	channelHandler.SetBaseURL(scimBaseURL)

	// Webhook handler for manual triggers
	webhookHandler := handlers.NewWebhookHandler(db, webhookSender, permService)
	portalHandler := handlers.NewPortalHandler(db, sessionManager, ipExtractor)
	portalCustomersHandler := handlers.NewPortalCustomersHandler(db)
	contactRolesHandler := handlers.NewEnumHandler(
		services.NewEnumService(db, services.NewContactRoleConfig()),
		func() interface{} { return &models.ContactRole{} })

	// Notification settings management
	notificationSettingsHandler := handlers.NewNotificationSettingsHandler(db)
	configSetNotificationHandler := handlers.NewConfigurationSetNotificationHandler(db)

	// We'll stop notification manager during shutdown, not in defer

	// Initialize attachment handlers if attachment path is provided
	var attachmentHandler *handlers.AttachmentHandler
	var attachmentSettingsHandler *handlers.AttachmentSettingsHandler
	if attachmentPath != "" {
		slog.Info("attachments enabled", "path", attachmentPath)
		attachmentHandler = handlers.NewAttachmentHandler(db, attachmentPath, permService)
		attachmentSettingsHandler = handlers.NewAttachmentSettingsHandlerWithPool(db)

		// Initialize attachment settings in database
		if err := initializeAttachmentSettings(sqlDB, attachmentPath); err != nil {
			slog.Warn("failed to initialize attachment settings", "error", err)
		}
	} else {
		slog.Info("attachments disabled (no --attachment-path specified)")
	}

	// Initialize diagram handler
	diagramHandler := handlers.NewDiagramHandler(db)

	// Initialize plugin system
	// PLUGIN_DIRS env var allows loading plugins from additional directories (opt-in)
	var pluginOpts []plugins.Option
	pluginOpts = append(pluginOpts, plugins.WithDatabase(db), plugins.WithSCMService(scmSyncService))

	if pluginDirsEnv := os.Getenv("PLUGIN_DIRS"); pluginDirsEnv != "" {
		var additionalDirs []string
		for _, dir := range strings.Split(pluginDirsEnv, ",") {
			dir = strings.TrimSpace(dir)
			if dir != "" && dir != "plugins" {
				additionalDirs = append(additionalDirs, dir)
			}
		}
		if len(additionalDirs) > 0 {
			slog.Info("loading plugins from additional directories", "dirs", additionalDirs)
			pluginOpts = append(pluginOpts, plugins.WithAdditionalPluginDirs(additionalDirs...))
		}
	}

	pluginManager := plugins.NewManager("plugins", pluginOpts...)
	slog.Info("initializing plugin system")
	if err := pluginManager.LoadPlugins(); err != nil {
		slog.Warn("failed to load plugins", "error", err)
	}

	// Create webhook dispatcher and wire to webhook sender
	webhookDispatcher := plugins.NewWebhookDispatcher(pluginManager, db)
	webhookSender.SetPluginDispatcher(webhookDispatcher)

	// Register webhooks for loaded plugins
	ctx := context.Background()
	for _, plugin := range pluginManager.ListPlugins() {
		if err := pluginManager.RegisterPluginWebhooks(ctx, db, plugin); err != nil {
			slog.Warn("failed to register plugin webhooks", "plugin", plugin.Manifest.Name, "error", err)
		}
	}

	pluginRouter := plugins.NewRouter(pluginManager)
	pluginHandler := handlers.NewPluginHandler(db, pluginManager)

	// System handler for shutdown endpoint (created early but will use shutdown channel later)
	shutdownChan := make(chan os.Signal, 1)
	systemHandler := handlers.NewSystemHandler(shutdownChan)

	// Build API middleware chain
	corsMiddleware := createCORSMiddleware(allowedHosts, effectivePort, disableCSRF)
	apiMiddleware := router.MiddlewareChain{corsMiddleware, authMiddleware.OptionalAuth}

	// Apply CSRF protection to API routes (after OptionalAuth so csrf_exempt is set)
	if !disableCSRF {
		slog.Info("CSRF protection enabled with bearer token bypass")
		apiMiddleware = append(apiMiddleware, csrfMiddleware.AddCSRFTokenToContext, csrfMiddleware.CSRFProtection)
	} else {
		slog.Warn("CSRF protection disabled (development mode)")
	}

	// Create API route group with middleware chain
	api := router.NewRouteGroup(mux, "/api", apiMiddleware...)

	// SCIM 2.0 routes (separate from /api, uses SCIM token authentication)
	scimMiddleware := router.MiddlewareChain{corsMiddleware, scimRateLimiter.Limit}
	scim := router.NewRouteGroup(mux, "/scim/v2", scimMiddleware...)

	// Build route dependencies and register all routes
	routeDeps := &routes.Deps{
		API:       api,
		SCIMGroup: scim,
		Mux:       mux,

		AuthMiddleware:       authMiddleware,
		PermissionMiddleware: permissionMiddleware,
		SCIMAuthMiddleware:   scimAuthMiddleware,
		CSRFMiddleware:       csrfMiddleware,
		DisableCSRF:          disableCSRF,

		LoginRateLimiter:    loginRateLimiter,
		AuthRateLimiter:     authRateLimiter,
		FIDORateLimiter:     fidoRateLimiter,
		PortalSubmitLimiter: portalSubmitLimiter,
		PortalSearchLimiter: portalSearchLimiter,
		EmailVerifyLimiter:  emailVerifyLimiter,
		SetupLimiter:        setupLimiter,

		Auth: routes.AuthHandlers{
			Auth:     authHandler,
			SSO:      ssoHandler,
			WebAuthn: webAuthnHandler,
		},
		SCIM: routes.SCIMHandlers{
			SCIM:      scimHandler,
			SCIMToken: scimTokenHandler,
		},
		SCM: routes.SCMHandlers{
			Provider:      scmProviderHandler,
			Workspace:     scmWorkspaceHandler,
			ItemLinks:     scmItemLinksHandler,
			UserToken:     userSCMTokenHandler,
			EmailProvider: emailProviderHandler,
		},
		Items: routes.ItemHandlers{
			Item:               itemHandler,
			Recurrence:         recurrenceHandler,
			Comment:            commentHandler,
			Attachment:         attachmentHandler,
			AttachmentSettings: attachmentSettingsHandler,
			Diagram:            diagramHandler,
			ItemLink:           itemLinkHandler,
			LinkType:           linkTypeHandler,
		},
		Workspaces: routes.WorkspaceHandlers{
			Workspace:             workspaceHandler,
			FieldRequirement:      workspaceFieldReqHandler,
			Screen:                screenHandler,
			ConfigSet:             configSetHandler,
			ConfigSetNotification: configSetNotificationHandler,
			NotificationSettings:  notificationSettingsHandler,
			ItemType:              itemTypeHandler,
			Priority:              priorityHandler,
			HierarchyLevel:        hierarchyLevelHandler,
			RequestType:           requestTypeHandler,
			StatusCategory:        statusCategoryHandler,
			Status:                statusHandler,
			StatusLegacy:          statusHandlerLegacy,
			Workflow:              workflowHandler,
		},
		Users: routes.UserHandlers{
			User:          userHandler,
			Group:         groupHandler,
			Permission:    permissionHandler,
			PermissionSet: permissionSetHandler,
			WorkspaceRole: workspaceRoleHandler,
			Credential:    credentialHandler,
			AppToken:      appTokenHandler,
			APIToken:      apiTokenHandler,
		},
		Admin: routes.AdminHandlers{
			SecuritySettings: securitySettingsHandler,
			Theme:            themeHandler,
			UserPreferences:  userPreferencesHandler,
			JiraImport:       jiraImportHandler,
			Plugin:           pluginHandler,
			Setup:            setupHandler,
			System:           systemHandler,
		},
		Planning: routes.PlanningHandlers{
			MilestoneCategory: milestoneCategoryHandler,
			Milestone:         milestoneHandler,
			IterationType:     iterationTypeHandler,
			Iteration:         iterationHandler,
			PersonalLabel:     personalLabelHandler,
		},
		TimeTracking: routes.TimeTrackingHandlers{
			Customer:        timeCustomerHandler,
			ProjectCategory: timeProjectCategoryHandler,
			Project:         timeProjectHandler,
			Worklog:         timeWorklogHandler,
			ActiveTimer:     activeTimerHandler,
		},
		TestMgmt: routes.TestManagementHandlers{
			Folder:      testFolderHandler,
			Case:        testCaseHandler,
			Set:         testSetHandler,
			RunTemplate: testRunTemplateHandler,
			Run:         testRunHandler,
			Summary:     testSummaryHandler,
		},
		Channels: routes.ChannelHandlers{
			ChannelCategory:      channelCategoryHandler,
			Channel:              channelHandler,
			Notification:         notificationHandler,
			NotificationTemplate: notificationTemplateHandler,
			Webhook:              webhookHandler,
		},
		Portal: routes.PortalHandlers{
			Portal:         portalHandler,
			PortalCustomer: portalCustomersHandler,
			ContactRole:    contactRolesHandler,
		},
		Assets: routes.AssetHandlers{
			Asset:    assetHandler,
			Type:     assetTypeHandler,
			Category: assetCategoryHandler,
			Status:   assetStatusHandler,
		},
		Collections: routes.CollectionHandlers{
			Category:    collectionCategoryHandler,
			Collection:  collectionHandler,
			BoardConfig: boardConfigHandler,
		},
		Misc: routes.MiscHandlers{
			Homepage:     homepageHandler,
			Review:       reviewHandler,
			CalendarFeed: calendarFeedHandler,
			CustomField:  customFieldHandler,
		},
	}
	routes.RegisterAll(routeDeps)

	// Register dynamic plugin routes (uses /api/plugins/{plugin}/{path...} pattern)
	pluginRouter.RegisterRoutes(mux)

	// ============================================
	// Public REST API v1
	// ============================================
	restapi.SetupRoutes(mux, db, tokenManager, permService, v1.RegisterRoutes)

	distFS, err := fs.Sub(frontendFiles, "frontend/dist")
	if err != nil {
		slog.Warn("frontend files not found, serving API only")
	} else {
		fileServer := http.FileServer(http.FS(distFS))

		// Serve Module Federation remote entry
		mux.Handle("GET /remoteEntry.js", fileServer)

		// Serve static assets directly
		mux.Handle("GET /assets/", fileServer)
		mux.Handle("GET /vite.svg", fileServer)
		mux.Handle("GET /cmicon-2.svg", fileServer)

		// Handle SPA routing - serve index.html for all other routes
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			// If it's an API request that wasn't matched, return 404
			if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
				http.NotFound(w, r)
				return
			}
			// If it's a REST API request that wasn't matched, return 404
			if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/rest" {
				http.NotFound(w, r)
				return
			}

			// For all other routes, serve the SPA
			indexFile, err := distFS.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer indexFile.Close()

			w.Header().Set("Content-Type", "text/html")
			http.ServeContent(w, r, "index.html", time.Time{}, indexFile.(io.ReadSeeker))
		})
	}

	// Setup SSH server if enabled
	var sshServer *ssh.Server
	if enableSSH {
		apiURL := "http://localhost:" + port

		// Create server options
		var serverOptions []ssh.Option
		serverOptions = append(serverOptions,
			wish.WithAddress(net.JoinHostPort(sshHost, sshPort)),
			wish.WithHostKeyPath(sshKeyPath),
		)

		// Add public key authentication
		slog.Info("SSH server starting with public key authentication enabled")
		sshAuthMiddleware := middleware.NewSSHAuthMiddleware(db)
		serverOptions = append(serverOptions, wish.WithPublicKeyAuth(sshAuthMiddleware.PublicKeyHandler()))

		// Add middleware
		serverOptions = append(serverOptions, wish.WithMiddleware(
			wishbubbletea.Middleware(tui.NewTUIHandler(apiURL, tokenManager)),
			logging.Middleware(),
		))

		s, err := wish.NewServer(serverOptions...)
		if err != nil {
			slog.Error("failed to create SSH server", "error", err)
		} else {
			sshServer = s
			slog.Info("SSH TUI server starting", "host", sshHost, "port", sshPort)
			go func() {
				if err := sshServer.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
					slog.Error("SSH server error", "error", err)
				}
			}()
		}
	}

	// Setup signal handling for graceful shutdown
	// Added SIGHUP for macOS .app bundle quit support (Cmd+Q)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Apply security headers to all routes as final wrapper
	securityMiddleware := createSecurityHeaders(enableHTTPS, useProxy, additionalProxyIPs)
	handler := securityMiddleware(mux)

	// Start HTTP or HTTPS server
	httpServer := &http.Server{
		Addr:           ":" + port,
		Handler:        handler,
		ReadTimeout:    15 * time.Second, // Max time to read request from client
		WriteTimeout:   30 * time.Second, // Max time to write response to client
		IdleTimeout:    60 * time.Second, // Max time for keep-alive connections
		MaxHeaderBytes: 1 << 20,          // 1 MB max header size
	}

	if enableHTTPS {
		slog.Info("HTTPS server starting", "port", port, "cert", tlsCertPath, "key", tlsKeyPath)
		if enableSSH {
			slog.Info("SSH TUI available", "command", "ssh "+sshHost+" -p "+sshPort)
		}

		go func() {
			if err := httpServer.ListenAndServeTLS(tlsCertPath, tlsKeyPath); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("HTTPS server error", "error", err)
			}
		}()
	} else {
		slog.Info("HTTP server starting (no TLS)", "port", port)
		slog.Warn("⚠️  Running without HTTPS - credentials will be transmitted in plaintext. Use --tls-cert and --tls-key for production.")
		if enableSSH {
			slog.Info("SSH TUI available", "command", "ssh "+sshHost+" -p "+sshPort)
		}

		go func() {
			if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("HTTP server error", "error", err)
			}
		}()
	}

	// Wait for shutdown signal
	<-shutdownChan
	slog.Info("shutdown signal received, starting graceful shutdown")

	// Stop SCM sync scheduler
	slog.Info("stopping SCM sync scheduler")
	close(scmSyncStopChan)

	// Stop notification scheduler and service
	slog.Info("stopping notification scheduler")
	notificationScheduler.Stop()
	slog.Info("notification scheduler stopped")

	// Stop recurrence scheduler
	slog.Info("stopping recurrence scheduler")
	recurrenceScheduler.Stop()
	slog.Info("recurrence scheduler stopped")

	// Stop email scheduler
	if emailScheduler != nil {
		slog.Info("stopping email scheduler")
		emailScheduler.Stop()
		slog.Info("email scheduler stopped")
	}

	slog.Info("stopping notification service")
	notificationService.Close()
	slog.Info("notification service stopped")

	// Stop notification manager to prevent database connections from being held
	slog.Info("stopping notification manager")
	notificationManager.Stop()
	slog.Info("notification manager stopped")

	// Close idle connections to allow faster shutdown
	httpServer.SetKeepAlivesEnabled(false)

	// Shutdown servers gracefully with shorter timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown HTTP server
	slog.Info("shutting down HTTP server")
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Warn("HTTP server graceful shutdown timed out, forcing close", "error", err)
		httpServer.Close()
	}
	slog.Info("HTTP server shutdown complete")

	// Shutdown SSH server
	if sshServer != nil {
		slog.Info("shutting down SSH server")
		if err := sshServer.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			slog.Error("SSH server shutdown error", "error", err)
		} else {
			slog.Info("SSH server shutdown complete")
		}
	}

	slog.Info("all servers stopped successfully")
}

// initializeAttachmentSettings creates initial attachment settings in the database
func initializeAttachmentSettings(db *sql.DB, attachmentPath string) error {
	// Check if settings already exist
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM attachment_settings)").Scan(&exists)
	if err != nil {
		return err
	}

	if !exists {
		// Create initial settings
		_, err = db.Exec(`
			INSERT INTO attachment_settings (max_file_size, allowed_mime_types, attachment_path, enabled)
			VALUES (52428800, '[]', ?, true)
		`, attachmentPath)
		return err
	}

	// Update attachment path if it has changed
	_, err = db.Exec(`
		UPDATE attachment_settings 
		SET attachment_path = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE id = (SELECT MIN(id) FROM attachment_settings)
	`, attachmentPath)
	return err
}
