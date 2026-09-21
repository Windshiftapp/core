package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"windshift/internal/services"
)

// testHistoryBackdateHandler serves the WINDSHIFT_E2E_TEST_HOOKS-only
// history backdate route. It only rewrites timestamps of rows the
// production API already created, letting Playwright specs place seeded
// changes on a realistic timeline (e.g. a two-week burndown window).
type testHistoryBackdateHandler struct {
	svc *services.TestHistoryHookService
}

// NewTestHistoryBackdate builds the handler. Server.go mounts the
// returned http.Handler only when WINDSHIFT_E2E_TEST_HOOKS=1 — the gate
// lives at the call site, mirroring the SCM test hooks.
func NewTestHistoryBackdate(svc *services.TestHistoryHookService) http.Handler {
	return &testHistoryBackdateHandler{svc: svc}
}

func (h *testHistoryBackdateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[services.HistoryBackdateRequest](w, r)
	if !ok {
		return
	}
	updated, err := h.svc.Backdate(r.Context(), req)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("history backdate failed: %w", err))
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]int{"updated": updated})
}
