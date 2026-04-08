package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// SandboxedAnalysisRequest configures a sandboxed (tool-less) LLM analysis.
type SandboxedAnalysisRequest struct {
	// SystemPrompt is the extraction instruction. The injection defense preamble
	// is automatically prepended.
	SystemPrompt string

	// Input is the untrusted data to analyze. It will be wrapped in <data> tags.
	Input string

	// OutputSchema is a JSON Schema defining the expected output structure.
	// The LLM response is validated against this schema.
	OutputSchema json.RawMessage
}

// RunSandboxedAnalysis processes untrusted input through an LLM with no tools
// and structured output only. This is the "information bottleneck" — the LLM
// can only produce a typed struct, never take actions.
//
// Safety properties:
//   - No tools are provided to the LLM
//   - The response is validated against the JSON Schema
//   - The injection defense preamble is automatically prepended
//   - Input is wrapped in <data> tags
func RunSandboxedAnalysis[T any](
	ctx context.Context,
	client Client,
	req SandboxedAnalysisRequest,
) (*T, error) {
	systemPrompt := InjectionDefensePreamble() + "\n\n" + req.SystemPrompt

	userMessage := WrapUntrustedData(req.Input)

	chatReq := ChatCompletionRequest{
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		// No tools — this is the critical safety property
		Tools: nil,
	}

	if len(req.OutputSchema) > 0 {
		chatReq.StructuredOutput = &StructuredOutputConfig{
			Schema:     req.OutputSchema,
			SchemaName: "extraction_result",
			Strict:     true,
		}
	}

	result, err := ChatCompletionStructured[T](ctx, client, chatReq)
	if err != nil {
		return nil, fmt.Errorf("sandboxed analysis failed: %w", err)
	}
	return result, nil
}
