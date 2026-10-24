package llm

import (
	"context"
	"strings"
	"time"
)

// DefaultRequestTimeout bounds a single in-product AI operation end to end. It
// is the one knob behind every in-product AI feature's time budget: the LLM
// client's per-call HTTP timeout, the agentic loop's overall budget (RunAgent),
// and the request/job context that the AI handlers and the briefing scheduler
// wrap around their LLM calls all derive from it. It is deliberately generous —
// multi-iteration agentic chat routinely runs longer than a minute — but
// bounded so a hung upstream cannot pin a connection or goroutine indefinitely.
//
// The coding-agent runner path (runner_broker) has its own, much longer budget
// and is intentionally independent of this value.
const DefaultRequestTimeout = 5 * time.Minute

// Client provides a provider-neutral interface to an LLM API.
type Client interface {
	// Complete sends a normalized generation request and returns its result.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	// Health checks if the LLM service is healthy.
	Health(ctx context.Context) error
	// Available returns true if the LLM service is configured.
	Available() bool
}

// Config contains configuration for the LLM client.
type Config struct {
	Endpoint string        // Base URL (e.g., http://llm:8081)
	APIKey   string        // Bearer token for authenticated endpoints
	Timeout  time.Duration // HTTP timeout (default: DefaultRequestTimeout)
}

// NewClient creates a new LLM client.
// Returns a noopClient if the endpoint is empty.
func NewClient(cfg Config) Client {
	endpoint := strings.TrimSuffix(cfg.Endpoint, "/")
	if endpoint == "" {
		return &noopClient{}
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultRequestTimeout
	}

	return newDynamicFantasyClient(endpoint, cfg.APIKey, timeout)
}

// ConnectionConfig holds configuration for creating a provider-specific client.
type ConnectionConfig struct {
	ProviderType   ProviderType
	Model          string
	APIKey         string
	BaseURL        string
	ProviderConfig string
	Timeout        time.Duration
}

// NewProviderClient creates a Client for a specific LLM provider.
func NewProviderClient(cfg ConnectionConfig) Client {
	provider := GetProvider(cfg.ProviderType)
	if provider == nil {
		return &noopClient{}
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = provider.BaseURL
	}

	return newFantasyClient(fantasyClientOptions{
		baseURL:        baseURL,
		model:          cfg.Model,
		apiKey:         cfg.APIKey,
		protocol:       ResolveGenerationProtocolForModel(cfg.ProviderType, cfg.Model, cfg.BaseURL, cfg.ProviderConfig),
		chatPath:       provider.ChatPath,
		providerConfig: cfg.ProviderConfig,
		timeout:        cfg.Timeout,
		userAgent:      provider.UserAgent,
		sessionHeader:  provider.SessionHeader,
	})
}

// ResolveGenerationProtocol resolves the connection once into the protocol
// its model client must speak, ignoring any per-model routing. It remains for
// callers that have no model context; prefer ResolveGenerationProtocolForModel.
func ResolveGenerationProtocol(providerType ProviderType, baseURL, providerConfig string) string {
	return ResolveGenerationProtocolForModel(providerType, "", baseURL, providerConfig)
}

// ResolveGenerationProtocolForModel resolves the protocol for a specific model.
// An explicit provider_config api_contract wins; otherwise a provider that
// routes by model (OpenCode Go) uses the model's format, falling back to the
// provider default. The coding-agent binding receives this value so it uses the
// same dispatch decision as in-process callers without learning a provider
// credential or upstream URL.
func ResolveGenerationProtocolForModel(providerType ProviderType, model, baseURL, providerConfig string) string {
	provider := GetProvider(providerType)
	if provider == nil {
		return APIContractChatCompletions
	}
	if provider.APIFormat == "anthropic" {
		return APIContractAnthropic
	}
	contract := ProviderConfigAPIContract(providerConfig)
	if contract == APIContractResponses || contract == APIContractChatCompletions {
		return contract
	}
	if format := ModelAPIFormat(providerType, model); format != "" {
		return format
	}
	usesCatalogEndpoint := baseURL == "" || strings.TrimRight(baseURL, "/") == strings.TrimRight(provider.BaseURL, "/")
	if provider.APIFormat == "openai-responses" && usesCatalogEndpoint {
		return APIContractResponses
	}
	return APIContractChatCompletions
}
