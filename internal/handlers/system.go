package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"windshift/internal/operatorcontrols"
)

// SystemHandler handles system-level operations like shutdown
type SystemHandler struct {
	shutdownChan chan os.Signal
	controls     *operatorcontrols.Controls
}

// NewSystemHandler creates a new system handler with a shutdown channel
func NewSystemHandler(shutdownChan chan os.Signal, controls ...*operatorcontrols.Controls) *SystemHandler {
	h := &SystemHandler{
		shutdownChan: shutdownChan,
	}
	if len(controls) > 0 {
		h.controls = controls[0]
	}
	return h
}

// Shutdown handles graceful shutdown requests
// POST /api/shutdown
func (h *SystemHandler) Shutdown(w http.ResponseWriter, r *http.Request) {
	if err := h.controls.Check(operatorcontrols.Lifecycle); err != nil {
		respondInternalError(w, r, err)
		return
	}
	slog.Info("shutdown requested via API")

	// Send success response immediately
	respondJSONOK(w, map[string]string{
		"message": "Shutdown initiated",
	})

	// Trigger shutdown after a brief delay to allow response to be sent
	go func() {
		time.Sleep(100 * time.Millisecond)
		if !trySendShutdown(h.shutdownChan) {
			slog.Warn("shutdown signal channel is not ready")
		}
	}()
}

func trySendShutdown(shutdownChan chan os.Signal) bool {
	select {
	case shutdownChan <- os.Interrupt:
		return true
	default:
		return false
	}
}
