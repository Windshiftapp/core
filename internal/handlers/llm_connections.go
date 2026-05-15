package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"windshift/internal/llm"
	"windshift/internal/logger"
	"windshift/internal/restapi"
	"windshift/internal/utils"
)

// LLMConnectionHandler handles admin CRUD for LLM connections and user queries.
type LLMConnectionHandler struct {
	manager *llm.ConnectionManager
	auditor *logger.Auditor
}

// NewLLMConnectionHandler creates a new LLM connection handler.
func NewLLMConnectionHandler(manager *llm.ConnectionManager, auditor *logger.Auditor) *LLMConnectionHandler {
	return &LLMConnectionHandler{manager: manager, auditor: auditor}
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

// validateConnectionRequest checks that name, provider_type, and model are
// non-empty and that base_url (when provided) is a valid admin-configured HTTP(S) URL.
// Returns true when validation passes; on failure it writes the error response
// and returns false.
func validateConnectionRequest(w http.ResponseWriter, r *http.Request, name string, providerType llm.ProviderType, model, baseURL string) bool {
	if name == "" || providerType == "" || model == "" {
		respondBadRequest(w, r, "name, provider_type, and model are required")
		return false
	}
	if baseURL != "" {
		if err := utils.ValidateHTTPBaseURL(baseURL); err != nil {
			respondBadRequest(w, r, "invalid base URL: "+err.Error())
			return false
		}
	}
	return true
}

// CreateConnection creates a new LLM connection (admin).
func (h *LLMConnectionHandler) CreateConnection(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[llm.CreateConnectionRequest](w, r)
	if !ok {
		return
	}
	if !validateConnectionRequest(w, r, req.Name, req.ProviderType, req.Model, req.BaseURL) {
		return
	}

	conn, err := h.manager.CreateConnection(req)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionLLMConnectionCreate, logger.ResourceLLMConnection, &conn.ID, req.Name)
	}
	respondJSONCreated(w, conn)
}

// UpdateConnection updates an existing LLM connection (admin).
func (h *LLMConnectionHandler) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[llm.UpdateConnectionRequest](w, r)
	if !ok {
		return
	}
	if !validateConnectionRequest(w, r, req.Name, req.ProviderType, req.Model, req.BaseURL) {
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
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionLLMConnectionUpdate, logger.ResourceLLMConnection, &id, req.Name)
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
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionLLMConnectionDelete, logger.ResourceLLMConnection, &id, "")
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
		slog.Warn("LLM connection test failed", slog.Int("connection_id", id), slog.Any("error", err))
		respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, restapi.ErrCodeConnectionTestFailed,
			fmt.Sprintf("Connection test failed: %s", err.Error())))
		return
	}
	respondJSONOK(w, map[string]string{"status": "ok"})
}

// GetProviders returns the hardcoded list of known LLM providers (user).
func (h *LLMConnectionHandler) GetProviders(w http.ResponseWriter, _ *http.Request) {
	respondJSONOK(w, llm.KnownProviders())
}

// GetEnabledConnections returns all enabled connections (user).
//
// Returns the slim PublicConnectionInfo (no BaseURL, HasAPIKey, timestamps,
// or IsEnabled) — admin-side endpoint configuration must not leak to every
// authenticated user. See bughunt8 finding 4.
func (h *LLMConnectionHandler) GetEnabledConnections(w http.ResponseWriter, r *http.Request) {
	connections, err := h.manager.ListEnabledPublic()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, connections)
}
