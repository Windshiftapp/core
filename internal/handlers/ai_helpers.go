package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/llm"
	"windshift/internal/restapi"
)

// parseConnectionIDParam extracts connection_id from the query string.
// Returns 0 if absent or unparseable (zero triggers default resolution).
func parseConnectionIDParam(r *http.Request) int {
	var connectionID int
	if cidStr := r.URL.Query().Get("connection_id"); cidStr != "" {
		fmt.Sscan(cidStr, &connectionID) //nolint:errcheck,gosec // best-effort parse, zero-value fallback is fine
	}
	return connectionID
}

// requireLLMClient resolves and validates an LLM client. On failure it writes
// a structured error response and returns nil — the caller should return immediately.
func requireLLMClient(w http.ResponseWriter, r *http.Request, manager *llm.ConnectionManager, connectionID int) llm.Client {
	client, err := manager.Resolve(connectionID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to resolve LLM connection: %w", err))
		return nil
	}
	if !client.Available() {
		respondServiceUnavailable(w, r, "AI features are not available. LLM service is not configured.")
		return nil
	}
	return client
}

// requireLLMClientForFeature resolves an LLM client respecting per-feature admin
// configuration. If the user provides an explicit connection override (> 0) it
// takes precedence, preserving the Chat UI's connection selector.
func requireLLMClientForFeature(w http.ResponseWriter, r *http.Request, manager *llm.ConnectionManager, featureKey string, userOverrideConnectionID int) llm.Client {
	if userOverrideConnectionID > 0 {
		return requireLLMClient(w, r, manager, userOverrideConnectionID)
	}
	client, err := manager.ResolveForFeature(featureKey)
	if err != nil {
		if errors.Is(err, llm.ErrFeatureDisabled) {
			restapi.RespondErrorWithMessage(w, r, http.StatusForbidden, "feature_disabled", err.Error())
			return nil
		}
		respondInternalError(w, r, fmt.Errorf("failed to resolve LLM connection for feature %s: %w", featureKey, err))
		return nil
	}
	if !client.Available() {
		respondServiceUnavailable(w, r, "AI features are not available. LLM service is not configured.")
		return nil
	}
	return client
}

// extendWriteDeadline pushes the HTTP server's per-request write deadline
// forward so that long-running AI calls aren't killed by WriteTimeout.
func extendWriteDeadline(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(130 * time.Second))
}

// respondLLMError logs an LLM call failure and writes a structured 503 response.
func respondLLMError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("LLM chat completion failed", slog.Any("error", err))
	msg := "AI service error: " + err.Error()
	respondServiceUnavailable(w, r, msg)
}
