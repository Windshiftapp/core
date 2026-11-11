package llm

import "encoding/json"

// ChatTemperature is the sampling temperature the in-product chat sends. The
// admin connection test reuses it so a provider that rejects chat's sampling
// parameters rejects the test too.
const ChatTemperature = 0.1

// connectionProbeTool gives the connection test a real tool definition to
// serialize. The provider validates the schema exactly as it would for chat
// tools, so a connection that cannot accept tool definitions fails the test.
var connectionProbeTool = ToolDefinition{
	Type: "function",
	Function: FunctionDef{
		Name:        "windshift_connection_probe",
		Description: "Confirms the connection accepts tool definitions. Never call it.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	},
}

// ConnectionProbeRequest is the request the admin connection test sends. It
// mirrors the parameters in-product chat sends — sampling temperature and a
// tool definition — while the adapter applies the connection's configured
// reasoning option from provider_config. A provider that would reject a chat
// request therefore rejects the test.
func ConnectionProbeRequest() CompletionRequest {
	return CompletionRequest{
		Messages:    []Message{{Role: "user", Content: "Reply with the single word: ok"}},
		Temperature: ChatTemperature,
		MaxTokens:   1,
		Tools:       []ToolDefinition{connectionProbeTool},
		ToolChoice:  "auto",
	}
}
