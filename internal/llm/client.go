package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Client provides methods to interact with an OpenAI-compatible LLM API.
type Client interface {
	// ChatCompletion sends a chat completion request and returns the response.
	ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error)
	// Health checks if the LLM service is healthy.
	Health(ctx context.Context) error
	// Available returns true if the LLM service is configured.
	Available() bool
}

// Config contains configuration for the LLM client.
type Config struct {
	Endpoint string        // Base URL (e.g., http://llm:8081)
	APIKey   string        // Bearer token for authenticated endpoints
	Timeout  time.Duration // HTTP timeout (default: 120s)
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
		timeout = 120 * time.Second
	}

	return &httpClient{
		endpoint: endpoint,
		apiKey:   cfg.APIKey,
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

// httpClient implements Client using HTTP requests to an OpenAI-compatible API.
type httpClient struct {
	endpoint string
	apiKey   string
	http     *http.Client
}

func (c *httpClient) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Build request body as map to allow adding grammar parameter
	bodyMap := map[string]interface{}{
		"messages": req.Messages,
	}
	if req.Model != "" {
		bodyMap["model"] = req.Model
	}
	if req.Temperature != 0 {
		bodyMap["temperature"] = req.Temperature
	}
	if req.MaxTokens != 0 {
		bodyMap["max_tokens"] = req.MaxTokens
	}

	// Add grammar for structured output (llama.cpp)
	if req.StructuredOutput != nil && len(req.StructuredOutput.Schema) > 0 {
		grammar, err := JSONSchemaToGBNF(req.StructuredOutput.Schema)
		if err != nil {
			slog.Warn("failed to generate GBNF grammar", slog.Any("error", err))
		} else if grammar != "" {
			slog.Debug("applying GBNF grammar", slog.Int("length", len(grammar)))
			bodyMap["grammar"] = grammar
		}
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, ErrServiceNotReady
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
		return nil, fmt.Errorf("%w: status %d - %s", ErrAPIError, resp.StatusCode, string(respBody))
	}

	var result ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

func (c *httpClient) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.endpoint+"/health", http.NoBody)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ErrServiceNotReady
	}
	return nil
}

func (c *httpClient) Available() bool {
	return true
}

// ConnectionConfig holds configuration for creating a provider-specific client.
type ConnectionConfig struct {
	ProviderType ProviderType
	Model        string
	APIKey       string
	BaseURL      string
	Timeout      time.Duration
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

	switch provider.APIFormat {
	case "anthropic":
		return newAnthropicClient(baseURL, cfg.Model, cfg.APIKey, cfg.Timeout)
	default:
		return newOpenAIClient(baseURL, cfg.Model, cfg.APIKey, cfg.Timeout, provider.ChatPath)
	}
}
