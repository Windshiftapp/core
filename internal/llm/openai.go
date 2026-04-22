package llm

import (
	"context"
	"encoding/json"
	"fmt"
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
	body := baseChatBody(req, c.model)

	// OpenAI-compatible APIs take a response_format block for structured output.
	if req.StructuredOutput != nil && len(req.StructuredOutput.Schema) > 0 {
		var schemaObj interface{}
		if err := json.Unmarshal(req.StructuredOutput.Schema, &schemaObj); err == nil {
			body["response_format"] = map[string]interface{}{
				"type": "json_schema",
				"json_schema": map[string]interface{}{
					"name":   req.StructuredOutput.SchemaName,
					"schema": schemaObj,
					"strict": req.StructuredOutput.Strict,
				},
			}
		}
	}

	return postChatCompletion(ctx, c.http, c.endpoint+c.chatPath, c.apiKey, body)
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
