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
)

// anthropicClient implements Client for the Anthropic Messages API.
type anthropicClient struct {
	endpoint string
	model    string
	apiKey   string
	http     *http.Client
}

// Anthropic Messages API request/response types
type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
	Model   string             `json:"model"`
	Usage   anthropicUsage     `json:"usage"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// newAnthropicClient creates a client for the Anthropic Messages API.
func newAnthropicClient(baseURL, model, apiKey string, timeout time.Duration) *anthropicClient {
	endpoint := strings.TrimSuffix(baseURL, "/")
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &anthropicClient{
		endpoint: endpoint,
		model:    model,
		apiKey:   apiKey,
		http:     &http.Client{Timeout: timeout},
	}
}

func (c *anthropicClient) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Extract system message from the messages array
	var systemPrompt string
	var messages []anthropicMessage
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
			continue
		}
		messages = append(messages, anthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	anthropicReq := anthropicRequest{
		Model:       c.model,
		MaxTokens:   maxTokens,
		System:      systemPrompt,
		Messages:    messages,
		Temperature: req.Temperature,
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, ErrServiceNotReady
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body) //nolint:errcheck
		return nil, fmt.Errorf("%w: status %d - %s", ErrAPIError, resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert Anthropic response to standard format
	var content string
	for _, c := range result.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	return &ChatCompletionResponse{
		ID:      result.ID,
		Object:  "chat.completion",
		Choices: []Choice{{Index: 0, Message: Message{Role: "assistant", Content: content}, FinishReason: "stop"}},
		Usage: Usage{
			PromptTokens:     result.Usage.InputTokens,
			CompletionTokens: result.Usage.OutputTokens,
			TotalTokens:      result.Usage.InputTokens + result.Usage.OutputTokens,
		},
	}, nil
}

func (c *anthropicClient) Health(ctx context.Context) error {
	// Try a minimal completion to verify the connection works
	_, err := c.ChatCompletion(ctx, ChatCompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 1,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	return nil
}

func (c *anthropicClient) Available() bool {
	return true
}
