package logbook

import (
	"log/slog"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/llm"
)

// ServerConfig holds configuration for the logbook server.
type ServerConfig struct {
	Port        string
	StoragePath string
	LLMEndpoint string
}

// Server represents the logbook HTTP server.
type Server struct {
	mux    *http.ServeMux
	config ServerConfig
}

// NewServer creates and wires all logbook components.
// The logbook authenticates via trusted X-Logbook-* headers injected by the
// main server proxy — no session/token managers needed.
func NewServer(db database.Database, cfg ServerConfig, articleClient llm.Client) (*Server, error) {
	// Initialize logbook schema
	if err := InitializeSchema(db); err != nil {
		return nil, err
	}
	slog.Info("logbook schema initialized")

	// Create logbook-specific services
	repo := NewRepository(db)
	logbookPermService := NewPermissionService(repo)
	ingestionService := NewIngestionService(repo, articleClient)
	handlers := NewHandlers(repo, logbookPermService, ingestionService, cfg.StoragePath)

	if articleClient != nil && articleClient.Available() {
		slog.Info("article generation LLM configured")
	} else {
		slog.Info("article generation LLM not configured, article generation will be skipped")
	}

	// Create router
	mux := http.NewServeMux()

	// Register routes with header auth middleware
	registerRoutes(mux, handlers)

	slog.Info("logbook routes registered")

	return &Server{
		mux:    mux,
		config: cfg,
	}, nil
}

// Handler returns the HTTP handler for the logbook server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// registerRoutes sets up all logbook API routes.
func registerRoutes(mux *http.ServeMux, h *Handlers) {
	// Wrap handler with header-based auth middleware
	auth := func(handler http.HandlerFunc) http.Handler {
		return headerAuthMiddleware(handler)
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

	// Document routes
	mux.Handle("POST /api/logbook/buckets/{bucketID}/documents/upload", auth(h.UploadDocument))
	mux.Handle("POST /api/logbook/buckets/{bucketID}/documents/notes", auth(h.CreateNote))
	mux.Handle("GET /api/logbook/buckets/{bucketID}/documents", auth(h.ListDocuments))
	mux.Handle("GET /api/logbook/documents", auth(h.ListAllDocuments))
	mux.Handle("GET /api/logbook/documents/{documentID}", auth(h.GetDocument))
	mux.Handle("PUT /api/logbook/documents/{documentID}", auth(h.UpdateDocument))
	mux.Handle("DELETE /api/logbook/documents/{documentID}", auth(h.ArchiveDocument))
	mux.Handle("GET /api/logbook/documents/{documentID}/thumbnail", auth(h.GetDocumentThumbnail))
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
