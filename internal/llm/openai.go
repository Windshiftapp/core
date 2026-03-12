package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"windshift/internal/utils"
)

// openaiClient implements Client for OpenAI-compatible APIs (OpenAI, Z.AI, local).
type openaiClient struct {
	endpoint string
	chatPath string
	model    string
	apiKey   string
	http     *http.Client
}

// newOpenAIClient creates a client for OpenAI-compatible endpoints.
// chatPath overrides the default "/v1/chat/completions" path appended to the base URL.
func newOpenAIClient(baseURL, model, apiKey string, timeout time.Duration, chatPath string) *openaiClient {
	endpoint := strings.TrimSuffix(baseURL, "/")
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	if chatPath == "" {
		chatPath = "/v1/chat/completions"
	}
	return &openaiClient{
		endpoint: endpoint,
		chatPath: chatPath,
		model:    model,
		apiKey:   apiKey,
		http:     utils.NewSSRFSafeHTTPClient(timeout),
	}
}

func (c *openaiClient) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Build request body as map to allow adding response_format
	bodyMap := map[string]interface{}{
		"model":    c.model,
		"messages": req.Messages,
	}
	if req.Temperature != 0 {
		bodyMap["temperature"] = req.Temperature
	}
	if req.MaxTokens != 0 {
		bodyMap["max_tokens"] = req.MaxTokens
	}

	// Add response_format for structured output
	if req.StructuredOutput != nil && len(req.StructuredOutput.Schema) > 0 {
		// Unmarshal schema to interface{} so it embeds correctly in the JSON
		var schemaObj interface{}
		if err := json.Unmarshal(req.StructuredOutput.Schema, &schemaObj); err == nil {
			bodyMap["response_format"] = map[string]interface{}{
				"type": "json_schema",
				"json_schema": map[string]interface{}{
					"name":   req.StructuredOutput.SchemaName,
					"schema": schemaObj,
					"strict": req.StructuredOutput.Strict,
				},
			}
		}
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+c.chatPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq) //nolint:gosec // URL from server-configured LLM endpoint
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
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

func (c *openaiClient) Health(ctx context.Context) error {
	// Try a minimal completion to verify the connection works
	_, err := c.ChatCompletion(ctx, ChatCompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 1,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	return nil
}

func (c *openaiClient) Available() bool {
	return true
}
