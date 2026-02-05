package handlers

import (
	"encoding/json"
	"net/http"

	"windshift/internal/llm"
)

// LLMConnectionHandler handles admin CRUD for LLM connections and user queries.
type LLMConnectionHandler struct {
	manager *llm.ConnectionManager
}

// NewLLMConnectionHandler creates a new LLM connection handler.
func NewLLMConnectionHandler(manager *llm.ConnectionManager) *LLMConnectionHandler {
	return &LLMConnectionHandler{manager: manager}
}

// ListConnections returns all LLM connections (admin).
func (h *LLMConnectionHandler) ListConnections(w http.ResponseWriter, r *http.Request) {
	connections, err := h.manager.ListConnections()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, connections)
}

// GetConnection returns a single LLM connection (admin).
func (h *LLMConnectionHandler) GetConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	conn, err := h.manager.GetConnection(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if conn == nil {
		respondNotFound(w, r, "LLM connection")
		return
	}
	respondJSONOK(w, conn)
}

// CreateConnection creates a new LLM connection (admin).
func (h *LLMConnectionHandler) CreateConnection(w http.ResponseWriter, r *http.Request) {
	var req llm.CreateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	if req.Name == "" || req.ProviderType == "" || req.Model == "" {
		respondBadRequest(w, r, "name, provider_type, and model are required")
		return
	}

	conn, err := h.manager.CreateConnection(req)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONCreated(w, conn)
}

// UpdateConnection updates an existing LLM connection (admin).
func (h *LLMConnectionHandler) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var req llm.UpdateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	if req.Name == "" || req.ProviderType == "" || req.Model == "" {
		respondBadRequest(w, r, "name, provider_type, and model are required")
		return
	}

	conn, err := h.manager.UpdateConnection(id, req)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if conn == nil {
		respondNotFound(w, r, "LLM connection")
		return
	}
	respondJSONOK(w, conn)
}

// DeleteConnection deletes an LLM connection (admin).
func (h *LLMConnectionHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.manager.DeleteConnection(id); err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSON(w, http.StatusNoContent, nil)
}

// TestConnection tests an LLM connection (admin).
func (h *LLMConnectionHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.manager.TestConnection(id); err != nil {
		respondJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "connection_test_failed",
			"message": err.Error(),
		})
		return
	}
	respondJSONOK(w, map[string]string{"status": "ok"})
}

// SetFeatures sets the feature assignments for a connection (admin).
func (h *LLMConnectionHandler) SetFeatures(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var req struct {
		Features []string `json:"features"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}

	if err := h.manager.SetFeatures(id, req.Features); err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, map[string]string{"status": "ok"})
}

// GetProviders returns the hardcoded list of known LLM providers (user).
func (h *LLMConnectionHandler) GetProviders(w http.ResponseWriter, _ *http.Request) {
	respondJSONOK(w, llm.KnownProviders())
}

// GetConnectionsForFeature returns enabled connections for a specific feature (user).
func (h *LLMConnectionHandler) GetConnectionsForFeature(w http.ResponseWriter, r *http.Request) {
	feature := r.URL.Query().Get("feature")
	if feature == "" {
		respondBadRequest(w, r, "feature query parameter is required")
		return
	}

	connections, err := h.manager.ListForFeature(feature)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, connections)
}
