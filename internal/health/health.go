// Package health provides the process and dependency probes used by
// orchestrators and the scratch container image.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const readinessTimeout = 2 * time.Second

// Pinger is the database capability required by the readiness probe.
type Pinger interface {
	PingContext(context.Context) error
}

// AttachmentProbe is the attachment-storage capability required by the
// readiness probe. Disabled storage is reported as healthy. Implementations own
// the short-lived result cache.
type AttachmentProbe interface {
	Enabled() bool
	CachedHealthCheck(context.Context) error
}

// Handler serves the liveness and readiness contracts.
type Handler struct {
	database    Pinger
	attachments AttachmentProbe

	mu sync.Mutex
}

type livenessResponse struct {
	Status string `json:"status"`
}

type readinessResponse struct {
	Status      string `json:"status"`
	Database    string `json:"database"`
	Attachments string `json:"attachments,omitempty"`
}

// NewHandler creates health handlers backed by the supplied database and an
// optional attachment backend. The attachment probe is deliberately optional
// so callers that only care about process/database health keep working.
func NewHandler(database Pinger, attachments ...AttachmentProbe) *Handler {
	var probe AttachmentProbe
	if len(attachments) > 0 {
		probe = attachments[0]
	}
	return &Handler{database: database, attachments: probe}
}

// SetAttachmentProbe wires the attachment backend after construction. Safe to
// call before the HTTP server starts serving.
func (h *Handler) SetAttachmentProbe(probe AttachmentProbe) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attachments = probe
}

// Liveness reports whether the HTTP process and router are serving requests.
// It intentionally does not touch the database or object storage: a liveness
// failure should restart the container, not react to a dependency outage.
//
// @Summary      Check server liveness
// @Description  Reports whether the Windshift HTTP process and router are serving requests. Public; no authentication required.
// @Tags         operations
// @Produce      json
// @Success      200  {object}  livenessResponse
// @Router       /healthz [get]
func (h *Handler) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, livenessResponse{Status: "ok"})
}

// Readiness reports whether the application can reach its database and
// attachment storage. A failure here removes the replica from traffic without
// restarting it.
//
// @Summary      Check server readiness
// @Description  Reports whether Windshift can serve requests, reach its database, and reach attachment storage. Public; no authentication required.
// @Tags         operations
// @Produce      json
// @Success      200  {object}  readinessResponse
// @Failure      503  {object}  readinessResponse
// @Router       /readyz [get]
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	if h.database == nil {
		writeUnavailable(w, "unavailable", "unknown")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()
	if err := h.database.PingContext(ctx); err != nil {
		attachmentStatus, _ := h.attachmentStatus(ctx)
		writeUnavailable(w, "unavailable", attachmentStatus)
		return
	}

	attachmentStatus, attachmentOK := h.attachmentStatus(ctx)
	if !attachmentOK {
		writeUnavailable(w, "ok", attachmentStatus)
		return
	}

	writeJSON(w, http.StatusOK, readinessResponse{
		Status:      "ready",
		Database:    "ok",
		Attachments: attachmentStatus,
	})
}

// attachmentStatus maps the shared cached attachment-storage state to the
// readiness payload. The cache lives on the store so /readyz, the settings
// status endpoint, and shell bootstrap all share one probe result.
func (h *Handler) attachmentStatus(ctx context.Context) (string, bool) {
	h.mu.Lock()
	probe := h.attachments
	h.mu.Unlock()

	if probe == nil || !probe.Enabled() {
		return "disabled", true
	}
	if err := probe.CachedHealthCheck(ctx); err != nil {
		return "unavailable", false
	}
	return "ok", true
}

func writeUnavailable(w http.ResponseWriter, database, attachments string) {
	writeJSON(w, http.StatusServiceUnavailable, readinessResponse{
		Status:      "not_ready",
		Database:    database,
		Attachments: attachments,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Probe performs the HTTP check used by the container's healthcheck command.
func Probe(ctx context.Context, target string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return fmt.Errorf("create readiness request: %w", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("request readiness endpoint: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness endpoint returned %s", response.Status)
	}
	return nil
}

// DefaultProbeTarget returns the in-container readiness URL for the configured
// HTTP port and optional public context path.
func DefaultProbeTarget(port, contextPath string) string {
	if port == "" {
		port = "8080"
	}
	contextPath = strings.TrimSpace(contextPath)
	if contextPath == "" || contextPath == "/" {
		contextPath = ""
	} else {
		contextPath = "/" + strings.Trim(contextPath, "/")
	}
	return "http://127.0.0.1:" + port + contextPath + "/readyz"
}
