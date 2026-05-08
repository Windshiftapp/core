package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"windshift/internal/config"
	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/logbookapi"
	"windshift/internal/logger"
	"windshift/internal/middleware"
)

func main() {
	// Resolve all env vars through the shared config package — no inline
	// os.Getenv calls anywhere in this entrypoint.
	cfg := config.LoadLogbookSidecar()

	// Initialize logger
	logger.Init(cfg.Logging.Level, cfg.Logging.Format)

	slog.Info("starting logbook service",
		slog.String("port", cfg.Port),
		slog.String("storage", cfg.StoragePath),
	)

	// Ensure storage directory exists
	if err := os.MkdirAll(cfg.StoragePath, 0o750); err != nil { //nolint:gosec // G703: storagePath from env config, not user input
		slog.Error("failed to create storage directory", "error", err)
		os.Exit(1)
	}

	// Connect to logbook's own PostgreSQL
	db, err := database.NewDatabase("postgres", cfg.PostgresConn, 20, 5)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()
	slog.Info("connected to logbook database")

	// Create article generation LLM client (optional).
	// Prefers LOGBOOK_ARTICLE_ENDPOINT (main server's internal proxy) over the
	// direct LLM endpoint, so admins can configure the provider via the UI.
	var articleClient llm.Client
	if cfg.ArticleEndpoint != "" && cfg.MainServerSecret != "" {
		articleClient = llm.NewClient(llm.Config{
			Endpoint: cfg.ArticleEndpoint,
			APIKey:   cfg.MainServerSecret,
		})
		if articleClient.Available() {
			slog.Info("article generation LLM configured via internal proxy", slog.String("endpoint", cfg.ArticleEndpoint))
		}
	} else if cfg.LLMEndpoint != "" {
		articleClient = llm.NewClient(llm.Config{Endpoint: cfg.LLMEndpoint})
		if articleClient.Available() {
			slog.Info("article generation LLM configured via direct endpoint", slog.String("endpoint", cfg.LLMEndpoint))
		}
	}

	if cfg.MainServerURL != "" {
		slog.Info("main server URL configured for action execution", slog.String("url", cfg.MainServerURL))
	} else {
		slog.Warn("WINDSHIFT_URL not set — logbook actions that call the main server (e.g. create work item) will fail")
	}

	// Create and start logbook server
	srvCfg := logbookapi.ServerConfig{
		Port:             cfg.Port,
		StoragePath:      cfg.StoragePath,
		LLMEndpoint:      cfg.LLMEndpoint,
		MainServerURL:    cfg.MainServerURL,
		MainServerSecret: cfg.MainServerSecret,
		BaseURL:          cfg.BaseURL,
	}

	srv, err := logbookapi.NewServer(db, srvCfg, articleClient)
	if err != nil {
		slog.Error("failed to create logbook server", "error", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: acceptable in main()
	}

	// Apply recovery middleware
	handler := middleware.Recovery(srv.Handler())

	// Create HTTP server
	httpServer := &http.Server{
		Handler:        handler,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   120 * time.Second, // Long timeout for file uploads
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// Start listening
	addr := ":" + cfg.Port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("failed to listen", "address", addr, "error", err)
		os.Exit(1)
	}

	slog.Info("logbook HTTP server starting", "address", addr)
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Wait for shutdown signal
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-shutdownChan

	slog.Info("shutdown signal received")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	srv.Stop()

	slog.Info("logbook service stopped")
}
