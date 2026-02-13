package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/logbook"
	"windshift/internal/logger"
	"windshift/internal/middleware"
)

func main() {
	// Read logbook database URL (preferred) or fall back to individual POSTGRES_* vars
	postgresConn := os.Getenv("LOGBOOK_DATABASE_URL")
	if postgresConn == "" {
		postgresConn = os.Getenv("POSTGRES_CONNECTION_STRING")
	}
	if postgresConn == "" {
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

	port := os.Getenv("LOGBOOK_PORT")
	if port == "" {
		port = "8090"
	}

	storagePath := os.Getenv("LOGBOOK_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "/data/logbook"
	}

	llmEndpoint := os.Getenv("LOGBOOK_LLM_ENDPOINT")
	if llmEndpoint == "" {
		llmEndpoint = os.Getenv("LLM_ENDPOINT")
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "text"
	}

	// Initialize logger
	logger.Init(logLevel, logFormat)

	slog.Info("starting logbook service",
		slog.String("port", port),
		slog.String("storage", storagePath),
	)

	// Ensure storage directory exists
	if err := os.MkdirAll(storagePath, 0750); err != nil {
		slog.Error("failed to create storage directory", "error", err)
		os.Exit(1)
	}

	// Connect to logbook's own PostgreSQL
	db, err := database.NewDatabase("postgres", postgresConn, 20, 5)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("connected to logbook database")

	// Create article generation LLM client (optional).
	// Prefers LOGBOOK_ARTICLE_ENDPOINT (main server's internal proxy) over the
	// direct LLM endpoint, so admins can configure the provider via the UI.
	var articleClient llm.Client
	articleEndpoint := os.Getenv("LOGBOOK_ARTICLE_ENDPOINT")
	ssoSecret := os.Getenv("SSO_SECRET")
	if articleEndpoint != "" && ssoSecret != "" {
		articleClient = llm.NewClient(llm.Config{
			Endpoint: articleEndpoint,
			APIKey:   ssoSecret,
		})
		if articleClient.Available() {
			slog.Info("article generation LLM configured via internal proxy", slog.String("endpoint", articleEndpoint))
		}
	} else if llmEndpoint != "" {
		articleClient = llm.NewClient(llm.Config{Endpoint: llmEndpoint})
		if articleClient.Available() {
			slog.Info("article generation LLM configured via direct endpoint", slog.String("endpoint", llmEndpoint))
		}
	}

	// Create and start logbook server
	cfg := logbook.ServerConfig{
		Port:        port,
		StoragePath: storagePath,
		LLMEndpoint: llmEndpoint,
	}

	srv, err := logbook.NewServer(db, cfg, articleClient)
	if err != nil {
		slog.Error("failed to create logbook server", "error", err)
		os.Exit(1)
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
	addr := ":" + port
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

	slog.Info("logbook service stopped")
}
