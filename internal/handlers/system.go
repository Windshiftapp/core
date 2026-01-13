package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// SystemHandler handles system-level operations like shutdown
type SystemHandler struct {
	shutdownChan chan os.Signal
}

// NewSystemHandler creates a new system handler with a shutdown channel
func NewSystemHandler(shutdownChan chan os.Signal) *SystemHandler {
	return &SystemHandler{
		shutdownChan: shutdownChan,
	}
}

// Shutdown handles graceful shutdown requests
// POST /api/shutdown
func (h *SystemHandler) Shutdown(w http.ResponseWriter, r *http.Request) {
	slog.Info("shutdown requested via API")

	// Send success response immediately
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Shutdown initiated",
	})

	// Trigger shutdown after a brief delay to allow response to be sent
	go func() {
		time.Sleep(100 * time.Millisecond)
		h.shutdownChan <- os.Interrupt
	}()
}
