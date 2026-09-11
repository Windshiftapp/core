package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"windshift/internal/auth"
	"windshift/internal/config"
	"windshift/internal/database"
	"windshift/internal/health"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/server"
	"windshift/internal/tui"
	"windshift/internal/utils"

	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	wishbubbletea "charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"github.com/charmbracelet/ssh"
)

//go:embed all:frontend/dist
var frontendFiles embed.FS

//go:embed assets/banner.txt
var bannerArt string

// ANSI color for startup banner
const colorTeal = "\033[38;5;37m"
const colorReset = "\033[0m"

// printBanner prints the windshift logo at startup
func printBanner() {
	fmt.Print(colorTeal)
	fmt.Print(bannerArt)
	fmt.Print(colorReset)
	fmt.Println()
	fmt.Println(colorTeal + "                                      W I N D S H I F T" + colorReset)
	fmt.Println("                                   Work Management Platform")
	fmt.Println()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		target := health.DefaultProbeTarget(os.Getenv("PORT"), os.Getenv("WINDSHIFT_CONTEXT_PATH"))
		if len(os.Args) > 2 {
			target = os.Args[2]
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := health.Probe(ctx, target)
		cancel()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// Setup signal handling for graceful shutdown
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Resolve all flags + env vars into a single canonical Config. No other
	// part of the app reads env vars or defines CLI flags directly.
	cfg := config.Load(frontendFiles, shutdownChan)

	// Initialize logger early
	logger.Init(cfg.Logging.Level, cfg.Logging.Format)
	memoryBudget, err := config.ResolveMemoryBudget(cfg.Memory.LimitMB)
	if err != nil {
		slog.Error("invalid memory configuration", "error", err)
		os.Exit(1)
	}
	debug.SetMemoryLimit(memoryBudget.GoLimitBytes)
	slog.Info("memory budget configured",
		"process_limit_mb", memoryBudget.ProcessLimitMB,
		"go_limit_bytes", memoryBudget.GoLimitBytes,
		"cache_limit_mb", memoryBudget.CacheLimitMB)

	// Apply the global SSRF-dialer setting before any client is built. Local and
	// private endpoints are reachable by default for self-hosted services;
	// operators can explicitly disable them to restore SSRF address blocking.
	utils.SetAllowLocalConnections(cfg.AllowLocalConnections)
	if !cfg.AllowLocalConnections {
		slog.Info("local connections disabled: server-side HTTP clients will block loopback/private addresses")
	}

	// Print startup banner
	printBanner()

	// Resolve security configuration: auto-detect proxy, derive CORS hosts/ports, validate
	resolved, err := server.ResolveSecurityConfig(cfg)
	if err != nil {
		slog.Error("security configuration error", "error", err)
		os.Exit(1)
	}
	resolved.LogDiagnostics()

	// Apply resolved values back to config
	cfg.UseProxy = resolved.UseProxy
	cfg.AllowedHosts = resolved.AllowedHosts
	cfg.AllowedPort = resolved.AllowedPort

	// Create and start the server
	srv, err := server.New(cfg)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	if err = srv.Start(); err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	// Setup SSH server if enabled
	var sshServer *ssh.Server
	var sshDB database.Database // Declared at function scope to allow explicit cleanup
	if cfg.SSH.Enabled {
		// Use the explicit IPv4 loopback address so the TUI client reaches the
		// same listener family on hosts where localhost resolves to IPv6 first.
		apiURL := fmt.Sprintf("http://127.0.0.1:%d", srv.Port())

		// Create a separate DB connection for SSH auth. This pool only services
		// public-key auth + session/token lookups, so it gets a small fixed cap
		// rather than cfg.DB.MaxReadConns — otherwise enabling SSH would double
		// the process's draw against the server's max_connections.
		if cfg.DB.PostgresConn != "" {
			sshDB, err = database.NewDatabase("postgres", cfg.DB.PostgresConn, config.SSHDatabaseMaxConnections, cfg.DB.MaxWriteConns)
		} else {
			sshDB, err = database.NewDatabase("sqlite3", cfg.DB.SQLitePath, config.SSHDatabaseMaxConnections, cfg.DB.MaxWriteConns)
		}
		if err != nil {
			slog.Error("failed to create SSH database connection", "error", err)
		} else {
			if registerErr := srv.RegisterDatabasePool("ssh", sshDB); registerErr != nil {
				slog.Warn("failed to register SSH database pool for diagnostics", "error", registerErr)
			}
			// nil tokenTracker: the SSH-minted temp tokens are short-lived
			// (24h) and we don't need last-used-at tracking for them.
			_, sshTokenCacheMB := config.SplitSSHCacheBudget(memoryBudget.APITokenCacheMB, true)
			tokenManager := auth.NewTokenManagerWithCacheBudget(sshDB, nil, "ssh_api_tokens", sshTokenCacheMB)

			serverOptions := make([]ssh.Option, 0, 4)
			serverOptions = append(serverOptions,
				wish.WithAddress(net.JoinHostPort(cfg.SSH.Host, cfg.SSH.Port)),
				wish.WithHostKeyPath(cfg.SSH.KeyPath),
			)

			slog.Info("SSH server starting with public key authentication enabled")
			sshAuthMiddleware := middleware.NewSSHAuthMiddleware(sshDB)
			serverOptions = append(serverOptions,
				wish.WithPublicKeyAuth(sshAuthMiddleware.PublicKeyHandler()),
				wish.WithIdleTimeout(30*time.Minute),
				wish.WithMaxTimeout(24*time.Hour),
				wish.WithMiddleware(
					wishbubbletea.Middleware(tui.NewTUIHandler(apiURL, tokenManager)),
					activeterm.Middleware(),
					logging.Middleware(),
				),
			)

			s, err := wish.NewServer(serverOptions...)
			if err != nil {
				slog.Error("failed to create SSH server", "error", err)
			} else {
				sshServer = s
				slog.Info("SSH TUI server starting", "host", cfg.SSH.Host, "port", cfg.SSH.Port)
				go func() {
					if err := sshServer.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
						slog.Error("SSH server error", "error", err)
					}
				}()
			}
		}
	}

	// Log startup info
	if cfg.SSH.Enabled {
		slog.Info("SSH TUI available", "command", "ssh "+cfg.SSH.Host+" -p "+cfg.SSH.Port)
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
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
		cancel()
		if sshDB != nil {
			_ = sshDB.Close()
		}
		os.Exit(1)
	}
	cancel()

	// Clean up SSH database connection
	if sshDB != nil {
		_ = sshDB.Close()
	}

	slog.Info("all servers stopped successfully")
}
