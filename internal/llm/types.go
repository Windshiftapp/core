package llm

import (
	"encoding/json"
	"strings"
)

// unmarshalExtras parses data into a raw map and removes the specified known keys,
// returning only the unknown fields.
func unmarshalExtras(data []byte, knownKeys ...string) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	for _, k := range knownKeys {
		delete(raw, k)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return raw, nil
}

// marshalWithExtras marshals base and merges extra fields into the resulting JSON object.
func marshalWithExtras(base any, extra map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		return data, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	for k, v := range extra {
		m[k] = v
	}
	return json.Marshal(m)
}

// Attachment holds a base64-encoded file to include in a message (e.g. a PDF for Anthropic document blocks).
type Attachment struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64-encoded
}

// Message represents a chat message in the OpenAI-compatible format.
type Message struct {
	Role        string       `json:"role"`
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments,omitempty"`
	// Tool calling fields
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role="tool" messages
	Name       string     `json:"name,omitempty"`         // function name for role="tool" messages

	// ProviderState carries opaque, provider-owned continuation data between
	// calls. ProviderBinding fingerprints the connection/model that produced it;
	// the run-scoped inference endpoint rejects replay under any other binding.
	// Neither field is persisted by Windshift.
	ProviderState   json.RawMessage `json:"provider_state,omitempty"`
	ProviderBinding string          `json:"provider_binding,omitempty"`
	// CacheBreakpoint marks the end of a caller-declared stable history prefix.
	// The broker honors it only for a provider with complete cache pricing.
	CacheBreakpoint bool `json:"cache_breakpoint,omitempty"`
}

// ToolDefinition describes a tool the LLM can call.
type ToolDefinition struct {
	Type     string      `json:"type"` // "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef describes a callable function.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict,omitempty"`
}

// ToolCall represents an LLM's request to call a tool.
// Custom JSON marshal/unmarshal preserves unknown fields (e.g. Gemini's
// thought_signature) so they survive the round-trip through conversation history.
type ToolCall struct {
	ID       string                     `json:"id"`
	Type     string                     `json:"type"` // "function"
	Function FunctionCall               `json:"function"`
	Extra    map[string]json.RawMessage `json:"-"` // unknown fields preserved for round-trip
}

func (tc *ToolCall) UnmarshalJSON(data []byte) error {
	type Alias ToolCall
	if err := json.Unmarshal(data, (*Alias)(tc)); err != nil {
		return err
	}
	extra, err := unmarshalExtras(data, "id", "type", "function")
	if err != nil {
		return err
	}
	tc.Extra = extra
	return nil
}

func (tc ToolCall) MarshalJSON() ([]byte, error) {
	type Alias ToolCall
	return marshalWithExtras(Alias(tc), tc.Extra)
}

// FunctionCall contains the function name and arguments from a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// StructuredOutputConfig configures structured output constraints.
// The Schema is a JSON Schema that the response must conform to.
type StructuredOutputConfig struct {
	Schema     json.RawMessage `json:"schema,omitempty"`
	SchemaName string          `json:"schema_name,omitempty"`
	Strict     bool            `json:"strict,omitempty"`
}

// CompletionRequest is Windshift's provider-neutral generation contract.
// Provider adapters translate it to their native wire format (OpenAI
// Responses, OpenAI-compatible Chat Completions, or Anthropic Messages).
type CompletionRequest struct {
	Model            string                  `json:"model,omitempty"`
	Messages         []Message               `json:"messages"`
	Temperature      float64                 `json:"temperature,omitempty"`
	MaxTokens        int                     `json:"max_tokens,omitempty"`
	StructuredOutput *StructuredOutputConfig `json:"structured_output,omitempty"`
	// Tool calling fields
	Tools      []ToolDefinition `json:"tools,omitempty"`
	ToolChoice any              `json:"tool_choice,omitempty"` // "auto", "none", or {"type":"function","function":{"name":"..."}}
	// Server-owned flags. They are never accepted from JSON callers.
	EnablePromptCache bool `json:"-"`
	CodingAgent       bool `json:"-"`
}

// ChatCompletionRequest remains as a source-compatible alias for consumers of
// Windshift's OpenAI-compatible proxy endpoint. New internal code should use
// CompletionRequest.
type ChatCompletionRequest = CompletionRequest

// CompletionRequestImageCount reports how many image parts a request sends.
// Images carry their own per-part rate, so metering needs this alongside the
// normalized token usage to price a call correctly.
func CompletionRequestImageCount(request CompletionRequest) int {
	count := 0
	for _, message := range request.Messages {
		for _, attachment := range message.Attachments {
			if strings.HasPrefix(strings.ToLower(attachment.MimeType), "image/") {
				count++
			}
		}
	}
	return count
}

// CompletionResponse is Windshift's normalized generation result.
type CompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// ChatCompletionResponse is the compatibility name used by the proxy wire
// contract. Provider adapters return CompletionResponse internally.
type ChatCompletionResponse = CompletionResponse

// Choice represents a single completion choice.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage contains token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
	// ProviderCostUSD is the cost the provider itself billed for this call,
	// when it reports one (OpenRouter does). It is authoritative over any rate
	// Windshift computes, since it already accounts for discounts and routing.
	// Server-side only: metering reads it, and it never crosses the broker.
	ProviderCostUSD *float64 `json:"-"`
}
