package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/llm"
	"windshift/internal/logger"
	"windshift/internal/restapi"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

// LLMConnectionHandler handles admin CRUD for LLM connections and user queries.
type LLMConnectionHandler struct {
	manager   *llm.ConnectionManager
	auditor   *logger.Auditor
	cache     *llm.ModelCache
	refresher *llm.ModelRefresher
}

// NewLLMConnectionHandler creates a new LLM connection handler.
func NewLLMConnectionHandler(manager *llm.ConnectionManager, auditor *logger.Auditor, cache *llm.ModelCache, refresher *llm.ModelRefresher) *LLMConnectionHandler {
	return &LLMConnectionHandler{manager: manager, auditor: auditor, cache: cache, refresher: refresher}
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
func validateConnectionRequest(w http.ResponseWriter, r *http.Request, name string, providerType llm.ProviderType, model, baseURL, providerConfig string) bool {
	if name == "" || providerType == "" || model == "" {
		respondBadRequest(w, r, "name, provider_type, and model are required")
		return false
	}
	if llm.GetProvider(providerType) == nil {
		respondBadRequest(w, r, fmt.Sprintf("unknown provider_type %q", providerType))
		return false
	}
	if providerType == llm.ProviderType("local") && baseURL == "" {
		respondBadRequest(w, r, "base URL is required for Local / Custom providers")
		return false
	}
	if baseURL != "" {
		if err := utils.ValidateHTTPBaseURL(baseURL); err != nil {
			respondBadRequest(w, r, "invalid base URL: "+err.Error())
			return false
		}
	}
	if err := sanitize.ValidateJSONPayload("provider_config", providerConfig); err != nil {
		respondBadRequest(w, r, err.Error())
		return false
	}
	if err := llm.ValidateProviderConfig(providerConfig); err != nil {
		respondBadRequest(w, r, err.Error())
		return false
	}
	return true
}

// CreateConnection creates a new LLM connection (admin).
func (h *LLMConnectionHandler) CreateConnection(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[llm.CreateConnectionRequest](w, r)
	if !ok {
		return
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Model, Policy: sanitize.ShortIdentifier},
	)
	req.ProviderConfig = strings.TrimSpace(req.ProviderConfig)
	if !validateConnectionRequest(w, r, req.Name, req.ProviderType, req.Model, req.BaseURL, req.ProviderConfig) {
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
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Model, Policy: sanitize.ShortIdentifier},
	)
	req.ProviderConfig = strings.TrimSpace(req.ProviderConfig)
	if !validateConnectionRequest(w, r, req.Name, req.ProviderType, req.Model, req.BaseURL, req.ProviderConfig) {
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

// providerResponse is the wire shape returned by GetProviders. It extends
// ProviderInfo with the cached model list + refresh state for providers that
// expose a /models endpoint, so the frontend can render the searchable
// picker + "Last refreshed" indicator from a single round trip.
type providerResponse struct {
	llm.ProviderInfo
	CachedModels    []llm.ModelInfo `json:"cached_models,omitempty"`
	LastRefreshedAt *time.Time      `json:"last_refreshed_at,omitempty"`
	LastError       string          `json:"last_error,omitempty"`
}

// GetProviders returns the catalog of known LLM providers (user). For
// dynamic-models providers the response also embeds the cached models +
// refresh metadata.
func (h *LLMConnectionHandler) GetProviders(w http.ResponseWriter, _ *http.Request) {
	providers := llm.KnownProviders()
	out := make([]providerResponse, 0, len(providers))
	for _, p := range providers {
		entry := providerResponse{ProviderInfo: p}
		if p.HasDynamicModels() && h.cache != nil {
			cached, err := h.cache.Get(p.Type)
			if err != nil {
				slog.Warn("read model cache", slog.String("provider", string(p.Type)), slog.Any("error", err))
			} else {
				entry.CachedModels = cached.Models
				entry.LastRefreshedAt = cached.LastRefreshedAt
				entry.LastError = cached.LastError
			}
		}
		if p.HasDynamicModels() && len(entry.CachedModels) == 0 && len(p.Models) > 0 {
			entry.CachedModels = p.Models
		}
		out = append(out, entry)
	}
	respondJSONOK(w, out)
}

type refreshProviderModelsRequest struct {
	ConnectionID int    `json:"connection_id,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
}

// RefreshProviderModels triggers a network fetch of a dynamic-models provider's
// catalog and writes it to the cache. Admin-only. The response carries the
// fresh list on success; on failure the error is surfaced to the caller AND
// recorded in the cache so the UI can render "Last attempt failed: …".
func (h *LLMConnectionHandler) RefreshProviderModels(w http.ResponseWriter, r *http.Request) {
	providerType := llm.ProviderType(r.PathValue("type"))
	provider := llm.GetProvider(providerType)
	if provider == nil {
		respondNotFound(w, r, "provider")
		return
	}
	if !provider.HasDynamicModels() {
		respondBadRequest(w, r, fmt.Sprintf("provider %q does not support dynamic model refresh", providerType))
		return
	}
	if h.refresher == nil {
		respondInternalError(w, r, fmt.Errorf("model refresher not configured"))
		return
	}

	req, ok := decodeOptionalJSON[refreshProviderModelsRequest](w, r)
	if !ok {
		return
	}

	apiKey, baseURL, ok := h.resolveCatalogRefreshConfig(w, r, providerType, *provider, req)
	if !ok {
		return
	}

	models, err := h.refresher.Refresh(r.Context(), *provider, apiKey, baseURL)
	if err != nil {
		slog.Warn("LLM model refresh failed", slog.String("provider", string(providerType)), slog.Any("error", err))
		respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, restapi.ErrCodeConnectionTestFailed,
			fmt.Sprintf("Refresh failed: %s", err.Error())))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionLLMConnectionRefreshModels, logger.ResourceLLMConnection, nil, string(providerType))
	}
	respondJSONOK(w, map[string]any{
		"provider_type":     providerType,
		"models":            models,
		"last_refreshed_at": time.Now(),
	})
}

func (h *LLMConnectionHandler) resolveCatalogRefreshConfig(w http.ResponseWriter, r *http.Request, providerType llm.ProviderType, provider llm.ProviderInfo, req refreshProviderModelsRequest) (apiKey, baseURL string, ok bool) {
	var runtime *llm.CatalogRuntime
	if req.ConnectionID > 0 {
		connProvider, rt, err := h.manager.GetCatalogRuntimeForConnection(req.ConnectionID)
		if err != nil {
			respondInternalError(w, r, err)
			return "", "", false
		}
		if rt == nil {
			respondBadRequest(w, r, "connection not found or disabled")
			return "", "", false
		}
		if connProvider != providerType {
			respondBadRequest(w, r, fmt.Sprintf("connection %d is %s, not %s", req.ConnectionID, connProvider, providerType))
			return "", "", false
		}
		runtime = rt
	} else {
		rt, err := h.manager.GetCatalogRuntimeForProvider(providerType)
		if err != nil {
			respondInternalError(w, r, err)
			return "", "", false
		}
		runtime = rt
	}
	if runtime != nil {
		apiKey = runtime.APIKey
		baseURL = runtime.BaseURL
	}
	if req.APIKey != "" {
		apiKey = req.APIKey
	}
	if req.BaseURL != "" {
		if err := utils.ValidateHTTPBaseURL(req.BaseURL); err != nil {
			respondBadRequest(w, r, "invalid base URL: "+err.Error())
			return "", "", false
		}
		baseURL = req.BaseURL
	}
	if baseURL == "" && provider.BaseURL == "" {
		respondBadRequest(w, r, fmt.Sprintf("configure a base URL before refreshing %s models", provider.Name))
		return "", "", false
	}
	if apiKey == "" && providerRequiresCatalogAPIKey(providerType) {
		respondBadRequest(w, r, fmt.Sprintf(
			"configure an enabled %s connection with an API key before refreshing its model catalog",
			provider.Name,
		))
		return "", "", false
	}
	return apiKey, baseURL, true
}

func providerRequiresCatalogAPIKey(providerType llm.ProviderType) bool {
	switch providerType {
	case llm.ProviderType("openrouter"), llm.ProviderType("local"):
		return false
	default:
		return true
	}
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
