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
	Type      string           `json:"type"`                  // "text", "document", "tool_use", "tool_result"
	Text      string           `json:"text,omitempty"`        // for type="text"
	Source    *anthropicSource `json:"source,omitempty"`      // for type="document"
	ID        string           `json:"id,omitempty"`          // for type="tool_use"
	Name      string           `json:"name,omitempty"`        // for type="tool_use"
	Input     json.RawMessage  `json:"input,omitempty"`       // for type="tool_use"
	ToolUseID string           `json:"tool_use_id,omitempty"` // for type="tool_result"
	Content   interface{}      `json:"content,omitempty"`     // for type="tool_result" (string or blocks)
	IsError   *bool            `json:"is_error,omitempty"`    // for type="tool_result"
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
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []anthropicContent `json:"content"`
	Model      string             `json:"model"`
	StopReason string             `json:"stop_reason"`
	Usage      anthropicUsage     `json:"usage"`
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
	// Extract system message and convert messages to Anthropic format
	var systemPrompt string
	var messages []anthropicMessage
	// Collect consecutive tool result messages to merge into a single "user" message
	var pendingToolResults []anthropicContentBlock
	flushToolResults := func() {
		if len(pendingToolResults) > 0 {
			messages = append(messages, anthropicMessage{Role: "user", Content: pendingToolResults})
			pendingToolResults = nil
		}
	}
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
			continue
		}
		// Convert role="tool" messages to Anthropic tool_result blocks
		if msg.Role == "tool" {
			pendingToolResults = append(pendingToolResults, anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			})
			continue
		}
		flushToolResults()
		// Convert assistant messages with tool_calls to Anthropic content blocks
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			var blocks []anthropicContentBlock
			if msg.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var input json.RawMessage
				if tc.Function.Arguments != "" {
					input = json.RawMessage(tc.Function.Arguments)
				} else {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
			messages = append(messages, anthropicMessage{Role: "assistant", Content: blocks})
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
	flushToolResults()

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

	// Add real tools for function calling
	if len(req.Tools) > 0 {
		for _, t := range req.Tools {
			anthropicReq.Tools = append(anthropicReq.Tools, anthropicTool{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				InputSchema: t.Function.Parameters,
			})
		}
		if req.ToolChoice != nil {
			if s, ok := req.ToolChoice.(string); ok {
				switch s {
				case "auto":
					anthropicReq.ToolChoice = &anthropicChoice{Type: "auto"}
				case "none":
					// Anthropic doesn't have "none" — omit tools instead
					anthropicReq.Tools = nil
				}
			}
		}
	}

	// Add tool for structured output (overrides real tools if both set)
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
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(httpReq) //nolint:gosec // G704: admin-configured LLM endpoint
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

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert Anthropic response to standard format
	var content string
	var toolCalls []ToolCall
	for _, c := range result.Content {
		switch {
		case c.Type == "text":
			content += c.Text
		case c.Type == "tool_use" && useToolOutput:
			// Extract JSON from tool input for structured output
			content = string(c.Input)
		case c.Type == "tool_use" && !useToolOutput:
			// Real tool call from function calling
			toolCalls = append(toolCalls, ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      c.Name,
					Arguments: string(c.Input),
				},
			})
		}
	}

	finishReason := "stop"
	if result.StopReason == "tool_use" && !useToolOutput {
		finishReason = "tool_calls"
	}

	return &ChatCompletionResponse{
		ID:     result.ID,
		Object: "chat.completion",
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:      "assistant",
				Content:   content,
				ToolCalls: toolCalls,
			},
			FinishReason: finishReason,
		}},
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
		return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	return nil
}

func (c *anthropicClient) Available() bool {
	return true
}
