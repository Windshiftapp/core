package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// baseChatBody assembles the fields every OpenAI-compatible chat completion request
// shares: messages, optional model, temperature, max_tokens, tools, and tool_choice.
// Provider-specific extras (grammar, response_format, etc.) are added by the caller.
func baseChatBody(req ChatCompletionRequest, model string) map[string]interface{} {
	body := map[string]interface{}{
		"messages": req.Messages,
	}
	if model != "" {
		body["model"] = model
	}
	if req.Temperature != 0 {
		body["temperature"] = req.Temperature
	}
	if req.MaxTokens != 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
		if req.ToolChoice != nil {
			body["tool_choice"] = req.ToolChoice
		}
	}
	return body
}

// postChatCompletion marshals body, POSTs it with the given auth token, and
// decodes the response. The caller is responsible for setting Content-Type via
// the Authorization flag (empty apiKey means the Authorization header is omitted).
func postChatCompletion(ctx context.Context, hc *http.Client, url, apiKey string, body map[string]interface{}) (*ChatCompletionResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := hc.Do(httpReq) //nolint:gosec // URL from server-configured LLM endpoint
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	return decodeCompletionResponse(resp)
}
