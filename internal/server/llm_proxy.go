package server

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"

	"windshift/internal/llm"
)

// NewInternalLLMProxy creates an HTTP handler that proxies chat completion
// requests to the admin-configured LLM connection for the given feature.
// Authentication uses a shared secret (SSO_SECRET) with constant-time comparison.
func NewInternalLLMProxy(llmManager *llm.ConnectionManager, feature, secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validateInternalToken(r, secret) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		var req llm.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid request body"}`))
			return
		}

		client, err := llmManager.ResolveForFeature(feature, 0)
		if err != nil || client == nil || !client.Available() {
			slog.Warn("LLM proxy: no client available for feature", "feature", feature, "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"LLM service unavailable"}`))
			return
		}

		resp, err := client.ChatCompletion(r.Context(), req)
		if err != nil {
			slog.Error("LLM proxy: chat completion failed", "feature", feature, "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"LLM request failed"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// NewInternalLLMHealthCheck creates an HTTP handler that checks whether the
// admin-configured LLM connection for the given feature is available.
func NewInternalLLMHealthCheck(llmManager *llm.ConnectionManager, feature, secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validateInternalToken(r, secret) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		client, err := llmManager.ResolveForFeature(feature, 0)
		if err != nil || client == nil || !client.Available() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"LLM service unavailable"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}

// validateInternalToken extracts the bearer token from the Authorization header
// and compares it against the expected secret using constant-time comparison.
func validateInternalToken(r *http.Request, secret string) bool {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if len(auth) <= len(prefix) {
		return false
	}
	token := auth[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
}
