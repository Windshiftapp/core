package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/utils"
)

// ModelRefresher fetches a provider's /models catalog and writes it to the
// ModelCache. Every call is admin-triggered — there is no background timer,
// so an airgapped deployment never makes outbound HTTP unless an admin asks.
//
// The refresher branches on ProviderInfo.AuthScheme()/ResponseFormat() so that
// providers whose catalog endpoint diverges from OpenAI's `/v1/models` shape
// (Anthropic's `data[].display_name`, Gemini's `models[].name` +
// `supportedGenerationMethods`) can share the same cache + error-recording
// plumbing.
type ModelRefresher struct {
	cache *ModelCache
	http  *http.Client
}

// NewModelRefresher constructs a ModelRefresher for admin-configured provider
// catalog URLs. Private/loopback endpoints are reachable unless the global
// --allow-local-connections switch is explicitly disabled.
func NewModelRefresher(cache *ModelCache) *ModelRefresher {
	return newModelRefresherWithClient(cache, newAdminConfiguredHTTPClient(30*time.Second))
}

// newModelRefresherWithClient lets tests substitute the HTTP client.
func newModelRefresherWithClient(cache *ModelCache, client *http.Client) *ModelRefresher {
	return &ModelRefresher{cache: cache, http: client}
}

// openaiModelsResponse matches `/v1/models` on OpenAI, OpenRouter, and other
// OpenAI-compatible servers. OpenRouter additionally returns an `architecture`
// block whose `input_modalities` is the only reliable vision signal across the
// catalog; other servers omit it (we fall back to the curated capability map).
// Pricing is parsed separately for cost metering.
type openaiModelsResponse struct {
	Data []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
		Architecture  struct {
			InputModalities []string `json:"input_modalities"`
		} `json:"architecture"`
		TopProvider struct {
			MaxCompletionTokens int `json:"max_completion_tokens"`
		} `json:"top_provider"`
		// OpenRouter advertises per-unit USD rates as strings (e.g. "0.000003").
		// Other OpenAI-compatible catalogs omit pricing → left nil downstream.
		Pricing struct {
			Prompt          string `json:"prompt"`
			Completion      string `json:"completion"`
			InputCacheRead  string `json:"input_cache_read"`
			InputCacheWrite string `json:"input_cache_write"`
			Image           string `json:"image"`
			Request         string `json:"request"`
		} `json:"pricing"`
	} `json:"data"`
}

// anthropicModelsResponse matches Anthropic's `/v1/models`. They use
// `display_name` (not `name`) and don't expose context window in the list.
type anthropicModelsResponse struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

// geminiModelsResponse matches Google's `/v1beta/models`. Names are prefixed
// with `models/` and entries also cover embedding-only models that can't be
// used for chat, so we filter on supportedGenerationMethods.
type geminiModelsResponse struct {
	Models []struct {
		Name                       string   `json:"name"`
		DisplayName                string   `json:"displayName"`
		InputTokenLimit            int      `json:"inputTokenLimit"`
		OutputTokenLimit           int      `json:"outputTokenLimit"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
}

// Refresh fetches and caches the model list for one provider. On failure it
// also writes the error string to the cache so the UI can render "Last attempt
// failed: …" without losing the previously cached models.
func (r *ModelRefresher) Refresh(ctx context.Context, provider ProviderInfo, apiKey, baseURLOverride string) ([]ModelInfo, error) {
	if !provider.HasDynamicModels() {
		return nil, fmt.Errorf("provider %q has no models_endpoint configured", provider.Type)
	}
	url := provider.ModelsURLForBase(baseURLOverride)
	if err := utils.ValidateHTTPBaseURL(url); err != nil {
		return nil, fmt.Errorf("invalid models URL: %w", err)
	}

	models, err := r.fetch(ctx, provider, url, apiKey)
	if err != nil {
		if cacheErr := r.cache.SaveFailure(provider.Type, err, time.Now()); cacheErr != nil {
			return nil, fmt.Errorf("%w (also failed to persist error: %v)", err, cacheErr)
		}
		return nil, err
	}
	// Fill SupportsVision from the curated map for models the catalog didn't
	// mark, so the persisted cache carries vision capability even when the
	// provider doesn't advertise input modalities. Per-model wire protocols are
	// classified here too because the /models payload carries no protocol hint.
	EnrichModelsVision(provider.Type, models)
	EnrichModelsAPIFormat(provider.Type, models)
	if err := r.cache.SaveSuccess(provider.Type, models, time.Now()); err != nil {
		return nil, err
	}
	return models, nil
}

func (r *ModelRefresher) fetch(ctx context.Context, provider ProviderInfo, url, apiKey string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrConnectionFailed, err)
	}
	setCatalogAuth(req, provider.AuthScheme(), apiKey)
	req.Header.Set("Accept", "application/json")
	if provider.UserAgent != "" {
		req.Header.Set("User-Agent", provider.UserAgent)
	}
	if provider.SessionHeader != "" {
		// A catalog fetch is its own short-lived operation, distinct from any
		// user conversation, so it gets a dedicated session id.
		req.Header.Set(provider.SessionHeader, NewSessionID("ws-model-refresh"))
	}

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // cap at 4 MiB
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrConnectionFailed, err)
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("%w: HTTP 503", ErrServiceNotReady)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrAPIError, resp.StatusCode, truncateBody(body))
	}

	switch provider.ResponseFormat() {
	case "anthropic":
		return parseAnthropicModels(body)
	case "google":
		return parseGeminiModels(body)
	default:
		return parseOpenAIModels(body)
	}
}

// setCatalogAuth applies the right auth header for the provider's catalog
// endpoint. The OpenAI-bearer path also accepts an empty key — OpenRouter's
// /models is unauthenticated, and we want refresh to keep working there.
func setCatalogAuth(req *http.Request, scheme, apiKey string) {
	switch scheme {
	case "anthropic":
		if apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
		}
		req.Header.Set("anthropic-version", "2023-06-01")
	case "google":
		if apiKey != "" {
			req.Header.Set("x-goog-api-key", apiKey)
		}
	default: // "bearer"
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}
}

func parseOpenAIModels(body []byte) ([]ModelInfo, error) {
	var parsed openaiModelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", ErrAPIError, err)
	}
	out := make([]ModelInfo, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID == "" {
			continue
		}
		name := m.Name
		if name == "" {
			name = m.ID
		}
		out = append(out, ModelInfo{
			ID:             m.ID,
			Name:           name,
			MaxTokens:      m.TopProvider.MaxCompletionTokens,
			ContextWindow:  m.ContextLength,
			SupportsVision: hasImageModality(m.Architecture.InputModalities),
			Pricing: parsePricing(
				m.Pricing.Prompt, m.Pricing.Completion,
				m.Pricing.InputCacheRead, m.Pricing.InputCacheWrite,
				m.Pricing.Image, m.Pricing.Request,
			),
		})
	}
	return out, nil
}

// parsePricing builds a *Pricing from the catalog's string rates. It returns
// nil when no rate is advertised (all empty/zero) so "cost unknown" stays
// distinct from "free". Unparseable values are treated as zero.
func parsePricing(prompt, completion, cacheRead, cacheWrite, image, request string) *Pricing {
	p := Pricing{
		PromptUSD:     parseFloat(prompt),
		CompletionUSD: parseFloat(completion),
		CacheReadUSD:  parseFloat(cacheRead),
		CacheWriteUSD: parseFloat(cacheWrite),
		ImageUSD:      parseFloat(image),
		RequestUSD:    parseFloat(request),
	}
	if p == (Pricing{}) {
		return nil
	}
	return &p
}

func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

func parseAnthropicModels(body []byte) ([]ModelInfo, error) {
	var parsed anthropicModelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", ErrAPIError, err)
	}
	out := make([]ModelInfo, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID == "" {
			continue
		}
		name := m.DisplayName
		if name == "" {
			name = m.ID
		}
		out = append(out, ModelInfo{ID: m.ID, Name: name})
	}
	return out, nil
}

func parseGeminiModels(body []byte) ([]ModelInfo, error) {
	var parsed geminiModelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", ErrAPIError, err)
	}
	out := make([]ModelInfo, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		if !supportsGenerateContent(m.SupportedGenerationMethods) {
			continue
		}
		// Gemini returns "models/gemini-…"; strip the prefix so the ID matches
		// what the OpenAI-compat chat endpoint expects.
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" {
			continue
		}
		name := m.DisplayName
		if name == "" {
			name = id
		}
		out = append(out, ModelInfo{ID: id, Name: name, MaxTokens: m.OutputTokenLimit, ContextWindow: m.InputTokenLimit})
	}
	return out, nil
}

func supportsGenerateContent(methods []string) bool {
	for _, m := range methods {
		if m == "generateContent" {
			return true
		}
	}
	return false
}

func truncateBody(b []byte) string {
	const maxLen = 256
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "…"
}
