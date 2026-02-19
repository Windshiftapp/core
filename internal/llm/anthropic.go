// Package llm provides interfaces and implementations for interacting with
// large language model APIs such as Anthropic's Claude.
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
	Tools       []anthropicTool    `json:"tools,omitempty"`
	ToolChoice  *anthropicChoice   `json:"tool_choice,omitempty"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []anthropicContentBlock
}

type anthropicContentBlock struct {
	Type   string           `json:"type"`             // "text" or "document"
	Text   string           `json:"text,omitempty"`   // for type="text"
	Source *anthropicSource `json:"source,omitempty"` // for type="document"
}

type anthropicSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // e.g. "application/pdf"
	Data      string `json:"data"`       // base64-encoded content
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
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
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
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
		http:     utils.NewSSRFSafeHTTPClient(timeout),
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
		if len(msg.Attachments) > 0 {
			var blocks []anthropicContentBlock
			for _, att := range msg.Attachments {
				blocks = append(blocks, anthropicContentBlock{
					Type:   "document",
					Source: &anthropicSource{Type: "base64", MediaType: att.MimeType, Data: att.Data},
				})
			}
			if msg.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: msg.Content})
			}
			messages = append(messages, anthropicMessage{Role: msg.Role, Content: blocks})
		} else {
			messages = append(messages, anthropicMessage{Role: msg.Role, Content: msg.Content})
		}
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

	// Add tool for structured output
	useToolOutput := false
	if req.StructuredOutput != nil && len(req.StructuredOutput.Schema) > 0 {
		toolName := req.StructuredOutput.SchemaName
		if toolName == "" {
			toolName = "structured_output"
		}
		anthropicReq.Tools = []anthropicTool{{
			Name:        toolName,
			Description: "Return the response in the specified JSON format",
			InputSchema: req.StructuredOutput.Schema,
		}}
		anthropicReq.ToolChoice = &anthropicChoice{
			Type: "tool",
			Name: toolName,
		}
		useToolOutput = true
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
		respBody, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
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
		} else if c.Type == "tool_use" && useToolOutput {
			// Extract JSON from tool input for structured output
			content = string(c.Input)
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
