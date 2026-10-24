package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/openai"
	sdkoption "github.com/openai/openai-go/v3/option"
)

type dynamicFantasyClient struct {
	endpoint string
	apiKey   string
	timeout  time.Duration
	mu       sync.Mutex
	models   map[string]*fantasyClient
}

func newDynamicFantasyClient(endpoint, apiKey string, timeout time.Duration) *dynamicFantasyClient {
	return &dynamicFantasyClient{endpoint: endpoint, apiKey: apiKey, timeout: timeout, models: make(map[string]*fantasyClient)}
}

func (c *dynamicFantasyClient) modelClient(model string) *fantasyClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if client := c.models[model]; client != nil {
		return client
	}
	client := newFantasyClient(fantasyClientOptions{
		baseURL:  c.endpoint,
		model:    model,
		apiKey:   c.apiKey,
		protocol: APIContractChatCompletions,
		chatPath: "/v1/chat/completions",
		timeout:  c.timeout,
	})
	c.models[model] = client
	return client
}

func (c *dynamicFantasyClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	return c.modelClient(req.Model).Complete(ctx, req)
}

func (c *dynamicFantasyClient) Health(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/health", http.NoBody)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	response, err := newAdminConfiguredHTTPClient(c.timeout).Do(request) //nolint:gosec // admin-configured endpoint
	if err != nil {
		return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return ErrServiceNotReady
	}
	return nil
}

func (c *dynamicFantasyClient) Available() bool { return true }

// fantasyClient is the only provider-generation adapter in core. Windshift's
// neutral request and response types stay at this boundary; Fantasy owns the
// OpenAI Chat Completions, OpenAI Responses, and Anthropic Messages protocols.
type fantasyClient struct {
	model            fantasy.LanguageModel
	modelID          string
	protocol         string
	providerConfig   string
	initErr          error
	replay           *reasoningReplayState
	completionTokens completionTokenNegotiator
}

// fantasyClientOptions is the resolved configuration for one provider/model
// generation client. It is a struct rather than positional arguments because
// the header fields are only meaningful for some providers.
type fantasyClientOptions struct {
	baseURL        string
	model          string
	apiKey         string
	protocol       string
	chatPath       string
	providerConfig string
	timeout        time.Duration
	userAgent      string
	sessionHeader  string
}

func newFantasyClient(opts fantasyClientOptions) *fantasyClient {
	if opts.timeout <= 0 {
		opts.timeout = DefaultRequestTimeout
	}
	client := &fantasyClient{
		modelID:        opts.model,
		protocol:       opts.protocol,
		providerConfig: opts.providerConfig,
		replay:         &reasoningReplayState{},
	}
	httpClient := newAdminConfiguredHTTPClient(opts.timeout)
	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	httpClient.Transport = &providerHeaderTransport{
		next:          &reasoningReplayTransport{next: transport, state: client.replay},
		userAgent:     opts.userAgent,
		sessionHeader: opts.sessionHeader,
	}

	var provider fantasy.Provider
	var err error
	switch opts.protocol {
	case APIContractAnthropic:
		provider, err = anthropic.New(
			anthropic.WithBaseURL(strings.TrimRight(opts.baseURL, "/")),
			anthropic.WithAPIKey(opts.apiKey),
			anthropic.WithHTTPClient(httpClient),
		)
	case APIContractResponses:
		provider, err = openai.New(
			openai.WithBaseURL(fantasyOpenAIBaseURL(opts.baseURL, "/v1/responses")),
			openai.WithAPIKey(opts.apiKey),
			openai.WithHTTPClient(httpClient),
			openai.WithUseResponsesAPI(),
			openai.WithResponsesAPIFunc(func(string) bool { return true }),
			openai.WithSDKOptions(providerConfigSDKOptions(opts.providerConfig, opts.protocol)...),
		)
	default:
		provider, err = openai.New(
			openai.WithBaseURL(fantasyOpenAIBaseURL(opts.baseURL, opts.chatPath)),
			openai.WithAPIKey(opts.apiKey),
			openai.WithHTTPClient(httpClient),
			openai.WithSDKOptions(providerConfigSDKOptions(opts.providerConfig, opts.protocol)...),
		)
	}
	if err != nil {
		client.initErr = err
		return client
	}
	client.model, client.initErr = provider.LanguageModel(context.Background(), opts.model)
	return client
}

func providerConfigSDKOptions(raw, protocol string) []sdkoption.RequestOption {
	var config map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &config) != nil {
		return nil
	}
	generated := map[string]bool{
		"model": true, "messages": true, "input": true, "temperature": true,
		"top_p": true, "max_tokens": true, "max_completion_tokens": true,
		"max_output_tokens": true, "tools": true, "tool_choice": true,
		"response_format": true, "text": true,
		"include": true, "store": true,
	}
	if protocol == APIContractResponses {
		generated["metadata"] = true
		generated["parallel_tool_calls"] = true
	}
	var options []sdkoption.RequestOption
	// The normalized reasoning_level is translated to the Chat Completions
	// field. A provider-native reasoning field supplied by the operator wins and
	// is forwarded verbatim by the generic loop below.
	if protocol == APIContractChatCompletions && !ProviderConfigHasReasoningOverride(raw) {
		if level := ProviderConfigReasoningLevel(raw); level != "" {
			options = append(options, sdkoption.WithJSONSet("reasoning_effort", level))
		}
	}
	for key, rawValue := range config {
		if reservedProviderConfigKeys[key] || generated[key] {
			continue
		}
		var value any
		if json.Unmarshal(rawValue, &value) == nil {
			options = append(options, sdkoption.WithJSONSet(key, value))
		}
	}
	return options
}

func fantasyOpenAIBaseURL(baseURL, endpointPath string) string {
	base := strings.TrimRight(baseURL, "/")
	if endpointPath == "" {
		endpointPath = "/v1/chat/completions"
	}
	path := "/" + strings.Trim(endpointPath, "/")
	for _, suffix := range []string{"/chat/completions", "/responses"} {
		path = strings.TrimSuffix(path, suffix)
	}
	if path == "" || path == "/" || strings.HasSuffix(base, path) {
		return base
	}
	return base + path
}

func (c *fantasyClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if c.initErr != nil {
		return nil, mapFantasyError(c.initErr)
	}
	// Guarantee one session per logical call so an internal retry (the
	// max_tokens negotiation below) stays on the same provider session even when
	// the caller attached none.
	if SessionIDFromContext(ctx) == "" {
		ctx = WithSessionID(ctx, NewSessionID("ws-request"))
	}
	prompt, err := toFantasyPrompt(req.Messages, req.EnablePromptCache)
	if err != nil {
		return nil, err
	}
	replays, err := reasoningReplays(req.Messages)
	if err != nil {
		return nil, err
	}
	c.replay.set(replays, responseRequestOverrides(req, c.protocol))

	maxTokens := int64(req.MaxTokens)
	call := fantasy.Call{
		Prompt:          prompt,
		Tools:           toFantasyTools(req.Tools, req.EnablePromptCache),
		ToolChoice:      toFantasyToolChoice(req.ToolChoice),
		ProviderOptions: c.callProviderOptions(req),
	}
	if req.MaxTokens > 0 && (c.protocol != APIContractChatCompletions || c.completionTokens.useLegacy.Load()) {
		call.MaxOutputTokens = &maxTokens
	}
	if req.Temperature != 0 {
		temperature := req.Temperature
		call.Temperature = &temperature
	}
	applyGenericCallOptions(&call, c.providerConfig)

	if req.StructuredOutput != nil && len(req.StructuredOutput.Schema) > 0 && len(req.Tools) == 0 {
		return c.generateObject(ctx, call, req.StructuredOutput)
	}
	response, err := c.generate(ctx, call)
	if err != nil && c.protocol == APIContractChatCompletions && req.MaxTokens > 0 {
		mapped := mapFantasyError(err)
		if rejectsModernCompletionTokenParameter(mapped) && !c.completionTokens.useLegacy.Load() {
			c.completionTokens.useLegacy.Store(true)
			call.ProviderOptions = c.callProviderOptions(req)
			call.MaxOutputTokens = &maxTokens
			response, err = c.generate(ctx, call)
		}
	}
	if err != nil {
		return nil, mapFantasyError(err)
	}
	return fromFantasyResponse(response)
}

// generate keeps Windshift's neutral endpoint request/response while allowing
// the Anthropic SDK to stream internally when a large max_tokens value crosses
// its mandatory-streaming threshold. Provider chunks never cross the broker.
func (c *fantasyClient) generate(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
	if c.protocol != APIContractAnthropic {
		return c.model.Generate(ctx, call)
	}
	stream, err := c.model.Stream(ctx, call)
	if err != nil {
		return nil, err
	}
	return collectFantasyStream(stream)
}

func collectFantasyStream(stream fantasy.StreamResponse) (*fantasy.Response, error) {
	response := &fantasy.Response{}
	text := map[string]string{}
	reasoning := map[string]string{}
	for part := range stream {
		switch part.Type {
		case fantasy.StreamPartTypeTextStart:
			text[part.ID] = part.Delta
		case fantasy.StreamPartTypeTextDelta:
			text[part.ID] += part.Delta
		case fantasy.StreamPartTypeTextEnd:
			response.Content = append(response.Content, fantasy.TextContent{Text: text[part.ID], ProviderMetadata: part.ProviderMetadata})
			delete(text, part.ID)
		case fantasy.StreamPartTypeReasoningStart:
			reasoning[part.ID] = part.Delta
		case fantasy.StreamPartTypeReasoningDelta:
			reasoning[part.ID] += part.Delta
		case fantasy.StreamPartTypeReasoningEnd:
			response.Content = append(response.Content, fantasy.ReasoningContent{Text: reasoning[part.ID], ProviderMetadata: part.ProviderMetadata})
			delete(reasoning, part.ID)
		case fantasy.StreamPartTypeToolCall:
			response.Content = append(response.Content, fantasy.ToolCallContent{
				ToolCallID: part.ID, ToolName: part.ToolCallName, Input: part.ToolCallInput,
				ProviderExecuted: part.ProviderExecuted, ProviderMetadata: part.ProviderMetadata,
			})
		case fantasy.StreamPartTypeFinish:
			response.Usage = part.Usage
			response.FinishReason = part.FinishReason
			response.ProviderMetadata = part.ProviderMetadata
		case fantasy.StreamPartTypeWarnings:
			response.Warnings = append(response.Warnings, part.Warnings...)
		case fantasy.StreamPartTypeError:
			if part.Error != nil {
				return nil, part.Error
			}
			return nil, errors.New("provider stream failed")
		}
	}
	return response, nil
}

func (c *fantasyClient) generateObject(ctx context.Context, call fantasy.Call, config *StructuredOutputConfig) (*CompletionResponse, error) {
	var schema fantasy.Schema
	if err := json.Unmarshal(config.Schema, &schema); err != nil {
		return nil, fmt.Errorf("invalid structured output schema: %w", err)
	}
	response, err := c.model.GenerateObject(ctx, fantasy.ObjectCall{
		Prompt:          call.Prompt,
		Schema:          schema,
		SchemaName:      config.SchemaName,
		MaxOutputTokens: call.MaxOutputTokens,
		Temperature:     call.Temperature,
		TopP:            call.TopP,
		ProviderOptions: call.ProviderOptions,
	})
	if err != nil {
		return nil, mapFantasyError(err)
	}
	content := response.RawText
	if content == "" {
		encoded, err := json.Marshal(response.Object)
		if err != nil {
			return nil, err
		}
		content = string(encoded)
	}
	return normalizedFantasyResponse(content, nil, nil, normalizeFantasyUsage(response.Usage, response.ProviderMetadata), response.FinishReason), nil
}

func (c *fantasyClient) Health(ctx context.Context) error {
	_, err := c.Complete(ctx, CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 1})
	if err != nil {
		if isCompletionTokenLimitError(err) {
			return nil
		}
		return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	return nil
}

func (c *fantasyClient) Available() bool { return c.initErr == nil }

func toFantasyPrompt(messages []Message, enablePromptCache bool) (fantasy.Prompt, error) {
	prompt := make(fantasy.Prompt, 0, len(messages))
	lastSystem := -1
	lastBreakpoint := -1
	if enablePromptCache {
		for i, message := range messages {
			if message.Role == "system" {
				lastSystem = i
			}
			if message.CacheBreakpoint {
				lastBreakpoint = i
			}
		}
	}
	for i, message := range messages {
		if len(message.ProviderState) > 0 {
			var preserved fantasy.Message
			if err := json.Unmarshal(message.ProviderState, &preserved); err != nil {
				return nil, fmt.Errorf("decode provider continuation: %w", err)
			}
			if i == lastSystem || i == lastBreakpoint {
				applyAnthropicCacheBreakpoint(&preserved)
			}
			prompt = append(prompt, preserved)
			continue
		}
		mapped := fantasy.Message{Role: fantasy.MessageRole(message.Role)}
		if message.Role == "tool" {
			mapped.Content = []fantasy.MessagePart{fantasy.ToolResultPart{
				ToolCallID: message.ToolCallID,
				Output:     fantasy.ToolResultOutputContentText{Text: message.Content},
			}}
			prompt = append(prompt, mapped)
			continue
		}
		if message.Content != "" {
			mapped.Content = append(mapped.Content, fantasy.TextPart{Text: message.Content})
		}
		for _, attachment := range message.Attachments {
			data, err := base64.StdEncoding.DecodeString(attachment.Data)
			if err != nil {
				return nil, fmt.Errorf("decode %s attachment: %w", attachment.MimeType, err)
			}
			mapped.Content = append(mapped.Content, fantasy.FilePart{MediaType: attachment.MimeType, Data: data})
		}
		for _, toolCall := range message.ToolCalls {
			mapped.Content = append(mapped.Content, fantasy.ToolCallPart{
				ToolCallID: toolCall.ID,
				ToolName:   toolCall.Function.Name,
				Input:      toolCall.Function.Arguments,
			})
		}
		if i == lastSystem || i == lastBreakpoint {
			applyAnthropicCacheBreakpoint(&mapped)
		}
		prompt = append(prompt, mapped)
	}
	return prompt, nil
}

func toFantasyTools(tools []ToolDefinition, enablePromptCache bool) []fantasy.Tool {
	out := make([]fantasy.Tool, 0, len(tools))
	for i, tool := range tools {
		var schema map[string]any
		_ = json.Unmarshal(tool.Function.Parameters, &schema)
		mapped := fantasy.FunctionTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: schema,
		}
		if enablePromptCache && i == len(tools)-1 {
			mapped.ProviderOptions = anthropicCacheOptions()
		}
		out = append(out, mapped)
	}
	return out
}

func anthropicCacheOptions() fantasy.ProviderOptions {
	return anthropic.NewProviderCacheControlOptions(&anthropic.ProviderCacheControlOptions{
		CacheControl: anthropic.CacheControl{Type: "ephemeral"},
	})
}

// applyAnthropicCacheBreakpoint places cache control at the message level.
// Anthropic's adapter falls back to message options for ordinary text and tool
// blocks, while signed reasoning keeps its part-level metadata and therefore
// cannot be overwritten by the cache marker.
func applyAnthropicCacheBreakpoint(message *fantasy.Message) {
	if message.ProviderOptions == nil {
		message.ProviderOptions = anthropicCacheOptions()
		return
	}
	if _, exists := message.ProviderOptions[anthropic.Name]; !exists {
		message.ProviderOptions[anthropic.Name] = anthropicCacheOptions()[anthropic.Name]
		return
	}
	// ProviderState may already carry message-level Anthropic metadata. Attach
	// the marker to the last non-reasoning part whose provider metadata is free.
	for i := len(message.Content) - 1; i >= 0; i-- {
		if message.Content[i].GetType() == fantasy.ContentTypeReasoning {
			continue
		}
		switch part := message.Content[i].(type) {
		case fantasy.TextPart:
			if _, exists := part.ProviderOptions[anthropic.Name]; !exists {
				part.ProviderOptions = mergeProviderOptions(part.ProviderOptions, anthropicCacheOptions())
				message.Content[i] = part
				return
			}
		case fantasy.ToolCallPart:
			if _, exists := part.ProviderOptions[anthropic.Name]; !exists {
				part.ProviderOptions = mergeProviderOptions(part.ProviderOptions, anthropicCacheOptions())
				message.Content[i] = part
				return
			}
		}
	}
}

func mergeProviderOptions(dst, src fantasy.ProviderOptions) fantasy.ProviderOptions {
	if dst == nil {
		dst = fantasy.ProviderOptions{}
	}
	for name, option := range src {
		dst[name] = option
	}
	return dst
}

func toFantasyToolChoice(choice any) *fantasy.ToolChoice {
	if choice == nil {
		return nil
	}
	if value, ok := choice.(string); ok {
		mapped := fantasy.ToolChoice(value)
		return &mapped
	}
	encoded, err := json.Marshal(choice)
	if err != nil {
		return nil
	}
	var selected struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
		Name string `json:"name"`
	}
	if json.Unmarshal(encoded, &selected) != nil {
		return nil
	}
	name := selected.Function.Name
	if name == "" {
		name = selected.Name
	}
	if name == "" {
		return nil
	}
	mapped := fantasy.SpecificToolChoice(name)
	return &mapped
}

func (c *fantasyClient) callProviderOptions(req CompletionRequest) fantasy.ProviderOptions {
	if c.protocol == APIContractResponses {
		store := false
		options := &openai.ResponsesProviderOptions{
			Include: []openai.IncludeType{openai.IncludeReasoningEncryptedContent},
			Store:   &store,
		}
		for _, tool := range req.Tools {
			if tool.Function.Strict {
				strict := true
				options.StrictJSONSchema = &strict
				break
			}
		}
		if !ProviderConfigHasReasoningOverride(c.providerConfig) {
			if level := ProviderConfigReasoningLevel(c.providerConfig); level != "" {
				effort := openai.ReasoningEffort(level)
				options.ReasoningEffort = &effort
			}
		}
		applyResponsesProviderConfig(options, c.providerConfig)
		return openai.NewResponsesProviderOptions(options)
	}
	if c.protocol == APIContractAnthropic {
		options := &anthropic.ProviderOptions{}
		// A provider-native reasoning field supplied by the operator wins over
		// the normalized reasoning_level and is forwarded verbatim via ExtraBody.
		options.ExtraBody = providerConfigReasoningOverrideBody(c.providerConfig)
		if options.ExtraBody == nil {
			if level := ProviderConfigReasoningLevel(c.providerConfig); level != "" {
				effort := anthropic.Effort(level)
				options.Effort = &effort
			} else if req.CodingAgent {
				effort := anthropic.EffortHigh
				options.Effort = &effort
			}
		}
		if options.Effort != nil || options.ExtraBody != nil {
			sendReasoning := true
			options.SendReasoning = &sendReasoning
		}
		return anthropic.NewProviderOptions(options)
	}
	options := &openai.ProviderOptions{}
	if req.MaxTokens > 0 && !c.completionTokens.useLegacy.Load() {
		maxTokens := int64(req.MaxTokens)
		options.MaxCompletionTokens = &maxTokens
	}
	return openai.NewProviderOptions(options)
}

func applyGenericCallOptions(call *fantasy.Call, raw string) {
	var config map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &config) != nil {
		return
	}
	if value := config["top_p"]; len(value) > 0 {
		var topP float64
		if json.Unmarshal(value, &topP) == nil {
			call.TopP = &topP
		}
	}
	if call.Temperature == nil {
		if value := config["temperature"]; len(value) > 0 {
			var temperature float64
			if json.Unmarshal(value, &temperature) == nil {
				call.Temperature = &temperature
			}
		}
	}
}

func applyResponsesProviderConfig(options *openai.ResponsesProviderOptions, raw string) {
	var config struct {
		Metadata          map[string]any `json:"metadata"`
		ParallelToolCalls *bool          `json:"parallel_tool_calls"`
	}
	if json.Unmarshal([]byte(raw), &config) == nil {
		options.Metadata = config.Metadata
		options.ParallelToolCalls = config.ParallelToolCalls
	}
}

func responseRequestOverrides(req CompletionRequest, protocol string) map[string]json.RawMessage {
	if protocol != APIContractResponses || req.StructuredOutput == nil || len(req.StructuredOutput.Schema) == 0 || len(req.Tools) == 0 {
		return nil
	}
	format := map[string]any{
		"type":   "json_schema",
		"name":   req.StructuredOutput.SchemaName,
		"schema": req.StructuredOutput.Schema,
		"strict": req.StructuredOutput.Strict,
	}
	encoded, err := json.Marshal(map[string]any{"format": format})
	if err != nil {
		return nil
	}
	return map[string]json.RawMessage{"text": encoded}
}

func fromFantasyResponse(response *fantasy.Response) (*CompletionResponse, error) {
	var message Message
	preserved := fantasy.Message{Role: fantasy.MessageRoleAssistant, ProviderOptions: fantasy.ProviderOptions(response.ProviderMetadata)}
	for _, content := range response.Content {
		switch content.GetType() {
		case fantasy.ContentTypeText:
			part, _ := fantasy.AsContentType[fantasy.TextContent](content)
			message.Content += part.Text
			preserved.Content = append(preserved.Content, fantasy.TextPart{Text: part.Text, ProviderOptions: fantasy.ProviderOptions(part.ProviderMetadata)})
		case fantasy.ContentTypeReasoning:
			part, _ := fantasy.AsContentType[fantasy.ReasoningContent](content)
			preserved.Content = append(preserved.Content, fantasy.ReasoningPart{Text: part.Text, ProviderOptions: fantasy.ProviderOptions(part.ProviderMetadata)})
		case fantasy.ContentTypeToolCall:
			part, _ := fantasy.AsContentType[fantasy.ToolCallContent](content)
			message.ToolCalls = append(message.ToolCalls, ToolCall{ID: part.ToolCallID, Type: "function", Function: FunctionCall{Name: part.ToolName, Arguments: part.Input}})
			preserved.Content = append(preserved.Content, fantasy.ToolCallPart{ToolCallID: part.ToolCallID, ToolName: part.ToolName, Input: part.Input, ProviderExecuted: part.ProviderExecuted, ProviderOptions: fantasy.ProviderOptions(part.ProviderMetadata)})
		}
	}
	if len(preserved.Content) > 0 {
		state, err := json.Marshal(preserved)
		if err != nil {
			return nil, err
		}
		message.ProviderState = state
	}
	return normalizedFantasyResponse(message.Content, message.ToolCalls, message.ProviderState, normalizeFantasyUsage(response.Usage, response.ProviderMetadata), response.FinishReason), nil
}

func normalizeFantasyUsage(usage fantasy.Usage, metadata fantasy.ProviderMetadata) Usage {
	baseInput := usage.InputTokens
	// Fantasy normalizes provider-specific counters before they reach this
	// boundary: InputTokens is the uncached/base class and the cache classes are
	// disjoint, including for APIs whose raw input total includes cache hits.
	total := baseInput + usage.CacheReadTokens + usage.CacheCreationTokens + usage.OutputTokens
	return Usage{
		PromptTokens:     int(baseInput),
		CompletionTokens: int(usage.OutputTokens),
		TotalTokens:      int(total),
		CacheReadTokens:  int(usage.CacheReadTokens),
		CacheWriteTokens: int(usage.CacheCreationTokens),
		ReasoningTokens:  int(usage.ReasoningTokens),
		ProviderCostUSD:  fantasyProviderCost(metadata),
	}
}

// fantasyProviderCost recovers a provider-billed call cost. Gateways that
// resell several upstreams report one (OpenRouter emits usage.cost), and it
// beats any locally computed rate because it already reflects the discounts and
// routing decisions the gateway applied. Fantasy carries such non-standard
// usage fields through as raw extras on the OpenAI-compatible contracts.
func fantasyProviderCost(metadata fantasy.ProviderMetadata) *float64 {
	if metadata == nil {
		return nil
	}
	openaiMetadata, ok := metadata[openai.Name].(*openai.ProviderMetadata)
	if !ok {
		return nil
	}
	var cost float64
	if !openaiMetadata.ExtraField("cost", &cost) || cost < 0 {
		return nil
	}
	return &cost
}

func normalizedFantasyResponse(content string, toolCalls []ToolCall, providerState json.RawMessage, usage Usage, finish fantasy.FinishReason) *CompletionResponse {
	finishReason := string(finish)
	if finish == fantasy.FinishReasonToolCalls {
		finishReason = "tool_calls"
	}
	return &CompletionResponse{
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Choices: []Choice{{
			Index:        0,
			Message:      Message{Role: "assistant", Content: content, ToolCalls: toolCalls, ProviderState: providerState},
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
}

func mapFantasyError(err error) error {
	if err == nil {
		return nil
	}
	var providerError *fantasy.ProviderError
	if errors.As(err, &providerError) {
		if providerError.StatusCode == 0 {
			return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
		}
		return fmt.Errorf("%w: status %d: %w", ErrAPIError, providerError.StatusCode, err)
	}
	return err
}
