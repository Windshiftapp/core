package logbook

import (
	"errors"
	"log/slog"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/logbookauth"
	"windshift/internal/repository"
)

// errMissingMainServerSecret is returned when the sidecar is started without
// SSO_SECRET. The shared secret is mandatory — it is the HMAC key used to
// verify inbound X-Logbook-* signatures.
var errMissingMainServerSecret = errors.New("logbook: MainServerSecret (SSO_SECRET) is required")

// ServerConfig holds configuration for the logbook server.
type ServerConfig struct {
	Port             string
	StoragePath      string
	LLMEndpoint      string
	MainServerURL    string
	MainServerSecret string
	BaseURL          string
}

// Server represents the logbook HTTP server.
type Server struct {
	mux           *http.ServeMux
	handlers      *Handlers
	config        ServerConfig
	actionService *LogbookActionService
}

// NewServer creates and wires all logbook components.
// The logbook authenticates via trusted X-Logbook-* headers injected by the
// main server proxy; requests must carry a valid HMAC signature keyed on
// MainServerSecret (SSO_SECRET), so network reachability alone is not
// sufficient to forge identities.
func NewServer(db database.Database, cfg ServerConfig, articleClient llm.Client) (*Server, error) {
	if cfg.MainServerSecret == "" {
		return nil, errMissingMainServerSecret
	}
	// Initialize logbook schema
	if err := InitializeSchema(db); err != nil {
		return nil, err
	}
	slog.Info("logbook schema initialized")

	// Create logbook-specific services
	repo := NewRepository(db)
	logbookPermService := NewPermissionService(repo)

	// Create action service and handlers
	actionRepo := repository.NewLogbookActionRepository(db)
	actionService := NewLogbookActionService(db, actionRepo, cfg.MainServerURL, cfg.MainServerSecret, cfg.BaseURL)

	ingestionService := NewIngestionService(repo, articleClient, actionService)
	handlers := NewHandlers(repo, logbookPermService, ingestionService, cfg.StoragePath)
	actionHandlers := NewActionHandlers(actionRepo, logbookPermService, actionService, repo)

	if articleClient != nil && articleClient.Available() {
		slog.Info("article generation LLM configured")
	} else {
		slog.Info("article generation LLM not configured, article generation will be skipped")
	}

	// Create router
	mux := http.NewServeMux()

	// Register routes with header auth middleware
	registerRoutes(mux, handlers, actionHandlers, cfg.MainServerSecret)

	slog.Info("logbook routes registered")

	return &Server{
		mux:           mux,
		handlers:      handlers,
		config:        cfg,
		actionService: actionService,
	}, nil
}

// Stop gracefully shuts down the logbook server components.
func (s *Server) Stop() {
	if s.handlers != nil {
		s.handlers.Shutdown()
	}
	if s.actionService != nil {
		s.actionService.Stop()
	}
}

// Handler returns the HTTP handler for the logbook server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// registerRoutes sets up all logbook API routes.
func registerRoutes(mux *http.ServeMux, h *Handlers, ah *ActionHandlers, sharedSecret string) {
	// One nonce cache shared by every authenticated route. Replay correctness
	// does not require this (path is signed, so a nonce replay against a
	// different path already fails verification), but sharing saves memory
	// and keeps diagnostics consistent.
	nonces := newNonceCache(logbookauth.MaxSkew, nonceCacheSize)
	auth := func(handler http.HandlerFunc) http.Handler {
		return headerAuthMiddlewareWithCache(sharedSecret, nonces, handler)
	}

	// Bucket routes
	mux.Handle("GET /api/logbook/buckets", auth(h.GetBuckets))
	mux.Handle("POST /api/logbook/buckets", auth(h.CreateBucket))
	mux.Handle("GET /api/logbook/buckets/{bucketID}", auth(h.GetBucket))
	mux.Handle("PUT /api/logbook/buckets/{bucketID}", auth(h.UpdateBucket))
	mux.Handle("DELETE /api/logbook/buckets/{bucketID}", auth(h.DeleteBucket))

	// Bucket permission routes
	mux.Handle("GET /api/logbook/buckets/{bucketID}/permissions", auth(h.GetBucketPermissions))
	mux.Handle("PUT /api/logbook/buckets/{bucketID}/permissions", auth(h.SetBucketPermissions))

	// Action CRUD routes (bucket-scoped)
	mux.Handle("GET /api/logbook/buckets/{bucketID}/actions", auth(ah.ListActions))
	mux.Handle("POST /api/logbook/buckets/{bucketID}/actions", auth(ah.CreateAction))
	mux.Handle("GET /api/logbook/buckets/{bucketID}/actions/{actionID}", auth(ah.GetAction))
	mux.Handle("PUT /api/logbook/buckets/{bucketID}/actions/{actionID}", auth(ah.UpdateAction))
	mux.Handle("DELETE /api/logbook/buckets/{bucketID}/actions/{actionID}", auth(ah.DeleteAction))
	mux.Handle("POST /api/logbook/buckets/{bucketID}/actions/{actionID}/toggle", auth(ah.ToggleAction))
	mux.Handle("POST /api/logbook/buckets/{bucketID}/actions/{actionID}/execute", auth(ah.ExecuteAction))
	mux.Handle("GET /api/logbook/buckets/{bucketID}/actions/{actionID}/logs", auth(ah.GetActionLogs))
	mux.Handle("GET /api/logbook/buckets/{bucketID}/action-logs", auth(ah.GetBucketLogs))

	// Document routes
	mux.Handle("POST /api/logbook/buckets/{bucketID}/documents/upload", auth(h.UploadDocument))
	mux.Handle("POST /api/logbook/buckets/{bucketID}/documents/notes", auth(h.CreateNote))
	mux.Handle("GET /api/logbook/buckets/{bucketID}/documents", auth(h.ListDocuments))
	mux.Handle("GET /api/logbook/documents", auth(h.ListAllDocuments))
	mux.Handle("GET /api/logbook/documents/{documentID}", auth(h.GetDocument))
	mux.Handle("PUT /api/logbook/documents/{documentID}", auth(h.UpdateDocument))
	mux.Handle("DELETE /api/logbook/documents/{documentID}", auth(h.ArchiveDocument))
	mux.Handle("GET /api/logbook/documents/{documentID}/thumbnail", auth(h.GetDocumentThumbnail))
	mux.Handle("GET /api/logbook/documents/{documentID}/preview", auth(h.GetDocumentPreview))
	mux.Handle("GET /api/logbook/documents/{documentID}/file", auth(h.GetDocumentFile))

	// Attachment routes
	mux.Handle("POST /api/logbook/documents/{documentID}/attachments", auth(h.UploadAttachment))
	mux.Handle("GET /api/logbook/attachments/{attachmentID}/download", auth(h.DownloadAttachment))

	// Search routes
	mux.Handle("GET /api/logbook/search", auth(h.KeywordSearch))

	// Health endpoint (no auth)
	mux.HandleFunc("GET /api/logbook/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}
