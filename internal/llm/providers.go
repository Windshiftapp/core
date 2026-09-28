package llm

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ProviderType identifies an LLM provider.
type ProviderType string

//go:embed llm_providers.json
var defaultProvidersJSON []byte

// providerRegistry holds the loaded provider list.
var (
	providerMu       sync.RWMutex
	providerRegistry []ProviderInfo
)

// ModelInfo describes a model offered by a provider.
//
// SupportsVision reports whether the model accepts image input. Provider
// catalogs do not advertise this consistently, so it is resolved from two
// sources (see vision_capability.go): an authoritative signal when the catalog
// exposes one (OpenRouter's architecture.input_modalities), falling back to a
// curated capability map keyed by model id. A per-connection override
// (provider_config vision_mode) wins over both at resolution time.
type ModelInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	MaxTokens      int    `json:"max_tokens"`
	ContextWindow  int    `json:"context_window"`
	SupportsVision bool   `json:"supports_vision"`
	// Pricing carries per-unit USD rates when the provider catalog advertises
	// them (OpenRouter today). nil means rates are unknown — usage is metered in
	// tokens but cost is left unknown rather than guessed.
	Pricing *Pricing `json:"pricing,omitempty"`
}

// Pricing is a model's per-unit cost in USD, as advertised by a provider
// catalog. Zero on any field means "not charged / not advertised".
type Pricing struct {
	PromptUSD     float64 `json:"prompt"`                // per prompt (input) token
	CompletionUSD float64 `json:"completion"`            // per completion (output) token
	CacheReadUSD  float64 `json:"cache_read,omitempty"`  // per cache-read input token
	CacheWriteUSD float64 `json:"cache_write,omitempty"` // per cache-write input token
	ImageUSD      float64 `json:"image"`                 // per image part
	RequestUSD    float64 `json:"request"`               // per request (flat)
}

// CostUSD computes the cost of one call from normalized usage + image counts.
// Reasoning tokens are already included in CompletionTokens and are reported
// separately for visibility, so charging them again would double bill them.
func (p *Pricing) CostUSD(usage Usage, images int) float64 {
	return p.CostUSDForCalls(usage, images, 1)
}

// CostUSDForCalls extends CostUSD to an agentic turn that made several provider
// round-trips. Per-token and per-image rates scale with the aggregated usage
// and image count, but the flat per-request rate applies once per call — an
// agent loop that took six steps paid the request fee six times, and pricing it
// once would under-report the spend.
func (p *Pricing) CostUSDForCalls(usage Usage, images, calls int) float64 {
	if p == nil {
		return 0
	}
	if calls < 1 {
		calls = 1
	}
	return float64(usage.PromptTokens)*p.PromptUSD +
		float64(usage.CacheReadTokens)*p.CacheReadUSD +
		float64(usage.CacheWriteTokens)*p.CacheWriteUSD +
		float64(usage.CompletionTokens)*p.CompletionUSD +
		float64(images)*p.ImageUSD +
		float64(calls)*p.RequestUSD
}

// HasCompleteCacheRates reports whether cache placement can be enabled without
// silently under-billing a cache write or read. Prompt is included because the
// uncached portion remains billable whenever caching is active.
func (p *Pricing) HasCompleteCacheRates() bool {
	return p != nil && p.PromptUSD > 0 && p.CacheReadUSD > 0 && p.CacheWriteUSD > 0
}

// CanPriceUsage rejects partial pricing for token classes that actually
// occurred. This keeps an unknown cache rate from silently becoming a free
// cache read/write in a computed usage row.
func (p *Pricing) CanPriceUsage(usage Usage) bool {
	if p == nil {
		return false
	}
	return (usage.PromptTokens == 0 || p.PromptUSD > 0) &&
		(usage.CompletionTokens == 0 || p.CompletionUSD > 0) &&
		(usage.CacheReadTokens == 0 || p.CacheReadUSD > 0) &&
		(usage.CacheWriteTokens == 0 || p.CacheWriteUSD > 0)
}

// ProviderInfo describes a known LLM provider and its available models.
//
// When ModelsEndpoint is set, the provider supports a `/models`-style catalog
// that the admin can refresh on demand; the static Models slice then acts as
// a seed (typically empty) and the live picker reads from the cache table.
//
// ModelsBaseURL / ModelsAuthScheme / ModelsResponseFormat let providers whose
// catalog endpoint diverges from the chat endpoint (different host, different
// auth, different response shape) plug into the same refresher. Gemini is the
// motivating case — its OpenAI-compatible chat lives under /v1beta/openai, but
// the catalog is at /v1beta/models with a Google-specific shape.
type ProviderInfo struct {
	Type                 ProviderType `json:"type"`
	Name                 string       `json:"name"`
	APIFormat            string       `json:"api_format"`
	ChatPath             string       `json:"chat_path,omitempty"`
	BaseURL              string       `json:"base_url"`
	ModelsEndpoint       string       `json:"models_endpoint,omitempty"`
	ModelsBaseURL        string       `json:"models_base_url,omitempty"`
	ModelsAuthScheme     string       `json:"models_auth_scheme,omitempty"`
	ModelsResponseFormat string       `json:"models_response_format,omitempty"`
	Models               []ModelInfo  `json:"models"`
}

// HasDynamicModels reports whether the provider exposes a `/models` catalog
// that we can refresh into the cache.
func (p *ProviderInfo) HasDynamicModels() bool {
	return p != nil && p.ModelsEndpoint != ""
}

// ModelsURL returns the absolute URL of the provider's catalog endpoint,
// preferring ModelsBaseURL when set (Gemini) and falling back to BaseURL.
func (p *ProviderInfo) ModelsURL() string {
	return p.ModelsURLForBase("")
}

// ModelsURLForBase returns the catalog URL using baseOverride when provided.
// This is used for local/custom LLM connections and provider base URL overrides.
func (p *ProviderInfo) ModelsURLForBase(baseOverride string) string {
	if p == nil || p.ModelsEndpoint == "" {
		return ""
	}
	base := baseOverride
	if base == "" {
		base = p.ModelsBaseURL
	}
	if base == "" {
		base = p.BaseURL
	}
	return joinProviderPath(base, p.ModelsEndpoint)
}

// AuthScheme returns the auth scheme used for the catalog request; defaults
// to "bearer" so existing OpenAI-compatible providers keep working unchanged.
func (p *ProviderInfo) AuthScheme() string {
	if p == nil || p.ModelsAuthScheme == "" {
		return "bearer"
	}
	return p.ModelsAuthScheme
}

// ResponseFormat returns the catalog response shape; defaults to "openai"
// (the OpenAI/OpenRouter `{"data":[…]}` shape).
func (p *ProviderInfo) ResponseFormat() string {
	if p == nil || p.ModelsResponseFormat == "" {
		return "openai"
	}
	return p.ModelsResponseFormat
}

// providersFile is the JSON structure for the providers file.
type providersFile struct {
	Providers []ProviderInfo `json:"providers"`
}

// LoadProviders reads and parses an LLM providers JSON file.
func LoadProviders(filePath string) error {
	data, err := os.ReadFile(filePath) //nolint:gosec // G304 — filePath from trusted CLI flag (-llm-providers)
	if err != nil {
		return fmt.Errorf("read providers file: %w", err)
	}
	return loadProvidersFromJSON(data)
}

// LoadDefaultProviders loads providers from the embedded default JSON.
func LoadDefaultProviders() {
	if err := loadProvidersFromJSON(defaultProvidersJSON); err != nil {
		// This should never happen since the embedded JSON is compiled in.
		panic(fmt.Sprintf("failed to parse embedded llm_providers.json: %v", err))
	}
}

// loadProvidersFromJSON parses JSON bytes into the provider registry.
func loadProvidersFromJSON(data []byte) error {
	var f providersFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse providers JSON: %w", err)
	}
	// Enrich static seed models with the curated vision map so a providers file
	// listing a known vision model (e.g. gpt-4o) without an explicit
	// supports_vision still resolves vision-capable — matching the enrichment
	// dynamic catalogs already get on refresh/cache-read.
	for i := range f.Providers {
		EnrichModelsVision(f.Providers[i].Type, f.Providers[i].Models)
	}
	providerMu.Lock()
	providerRegistry = f.Providers
	providerMu.Unlock()
	return nil
}

// GetProviders returns the loaded list of providers.
func GetProviders() []ProviderInfo {
	providerMu.RLock()
	defer providerMu.RUnlock()
	return providerRegistry
}

// GetProvider looks up a provider by type. Returns nil if not found.
func GetProvider(pt ProviderType) *ProviderInfo {
	providerMu.RLock()
	defer providerMu.RUnlock()
	for i := range providerRegistry {
		if providerRegistry[i].Type == pt {
			return &providerRegistry[i]
		}
	}
	return nil
}

// KnownProviders returns the list of supported LLM providers.
// Kept for backward compatibility; delegates to GetProviders.
func KnownProviders() []ProviderInfo {
	return GetProviders()
}
