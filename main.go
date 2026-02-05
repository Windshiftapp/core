package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/server"
	"windshift/internal/tui"

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
	var disablePlugins bool
	var enableAdminFallback bool
	var llmProvidersFile string
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
	flag.BoolVar(&disablePlugins, "disable-plugins", false, "Disable the plugin system (prevents loading and uploading plugins)")
	flag.BoolVar(&enableAdminFallback, "enable-fallback", false, "Enable admin password fallback for restrictive auth policies (disabled by default for security)")
	flag.StringVar(&llmProvidersFile, "llm-providers", "", "Path to custom LLM providers JSON file (overrides built-in provider list)")
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
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("PUBLIC_URL")
	}
	if baseURL != "" {
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

	// Plugin system environment variable
	if os.Getenv("DISABLE_PLUGINS") == "true" {
		disablePlugins = true
	}

	// Admin fallback environment variable
	if os.Getenv("ENABLE_ADMIN_FALLBACK") == "true" {
		enableAdminFallback = true
	}

	// LLM endpoint for AI features
	llmEndpoint := os.Getenv("LLM_ENDPOINT")

	// LLM providers file override
	if envLLMProviders := os.Getenv("LLM_PROVIDERS_FILE"); envLLMProviders != "" && llmProvidersFile == "" {
		llmProvidersFile = envLLMProviders
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

	// Setup signal handling for graceful shutdown
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Build server configuration
	cfg := server.Config{
		Port:                port,
		DBPath:              dbPath,
		PostgresConn:        postgresConn,
		DisableCSRF:         disableCSRF,
		AttachmentPath:      attachmentPath,
		AllowedHosts:        allowedHosts,
		AllowedPort:         allowedPort,
		UseProxy:            useProxy,
		AdditionalProxies:   additionalProxies,
		MaxReadConns:        maxReadConns,
		MaxWriteConns:       maxWriteConns,
		TLSCertPath:         tlsCertPath,
		TLSKeyPath:          tlsKeyPath,
		DisablePlugins:      disablePlugins,
		EnableAdminFallback: enableAdminFallback,
		BaseURL:             baseURL,
		LLMEndpoint:         llmEndpoint,
		LLMProvidersFile:    llmProvidersFile,
		FrontendFiles:       frontendFiles,
		ShutdownChan:        shutdownChan,
	}

	// Create and start the server
	srv, err := server.New(cfg)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	if err := srv.Start(); err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	// Setup SSH server if enabled
	var sshServer *ssh.Server
	if enableSSH {
		apiURL := fmt.Sprintf("http://localhost:%d", srv.Port())

		// We need to create a separate database connection for SSH
		// since the server's DB is internal
		var additionalProxyList []string
		if additionalProxies != "" {
			additionalProxyList = strings.Split(additionalProxies, ",")
		}
		enableHTTPS := tlsCertPath != "" && tlsKeyPath != ""

		// Create a separate DB connection for SSH auth
		var sshDB database.Database
		if postgresConn != "" {
			sshDB, err = database.NewDatabase("postgres", postgresConn, maxReadConns, maxWriteConns)
		} else {
			sshDB, err = database.NewDatabase("sqlite3", dbPath, maxReadConns, maxWriteConns)
		}
		if err != nil {
			slog.Error("failed to create SSH database connection", "error", err)
		} else {
			defer sshDB.Close()

			sessionManager := auth.NewSessionManager(sshDB, enableHTTPS, useProxy, additionalProxyList)

			var serverOptions []ssh.Option
			serverOptions = append(serverOptions,
				wish.WithAddress(net.JoinHostPort(sshHost, sshPort)),
				wish.WithHostKeyPath(sshKeyPath),
			)

			slog.Info("SSH server starting with public key authentication enabled")
			sshAuthMiddleware := middleware.NewSSHAuthMiddleware(sshDB)
			serverOptions = append(serverOptions, wish.WithPublicKeyAuth(sshAuthMiddleware.PublicKeyHandler()))

			serverOptions = append(serverOptions, wish.WithMiddleware(
				wishbubbletea.Middleware(tui.NewTUIHandler(apiURL, sessionManager)),
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
	}

	// Log startup info
	enableHTTPS := tlsCertPath != "" && tlsKeyPath != ""
	if enableHTTPS {
		if enableSSH {
			slog.Info("SSH TUI available", "command", "ssh "+sshHost+" -p "+sshPort)
		}
	} else {
		slog.Warn("⚠️  Running without HTTPS - credentials will be transmitted in plaintext. Use --tls-cert and --tls-key for production.")
		if enableSSH {
			slog.Info("SSH TUI available", "command", "ssh "+sshHost+" -p "+sshPort)
		}
	}

	// Wait for shutdown signal
	<-shutdownChan
	slog.Info("shutdown signal received, starting graceful shutdown")

	// Shutdown SSH server first
	if sshServer != nil {
		slog.Info("shutting down SSH server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := sshServer.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			slog.Error("SSH server shutdown error", "error", err)
		} else {
			slog.Info("SSH server shutdown complete")
		}
		cancel()
	}

	// Shutdown the main server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("all servers stopped successfully")
}
