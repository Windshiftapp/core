package llm

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// AgentConfig configures the agent loop.
type AgentConfig struct {
	SystemPrompt  string
	Tools         []ToolDefinition
	MaxIterations int
	Timeout       time.Duration
	MaxTokens     int
	Temperature   float64
}

// ToolExecutorFunc executes a tool call and returns the result as a string.
type ToolExecutorFunc func(ctx context.Context, name string, arguments string) (string, error)

// ToolCallRecord records a tool call made during the agent loop.
type ToolCallRecord struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result"`
}

// StopReason describes why the agent loop ended. Callers use it to
// distinguish a clean answer from a budget exhaustion, so the chat UI can
// flag the latter instead of silently passing off the boilerplate "I
// wasn't able to complete..." message as a normal reply.
type StopReason string

const (
	StopReasonDone          StopReason = "done"
	StopReasonMaxIterations StopReason = "max_iterations"
)

// AgentResult contains the outcome of an agent run.
type AgentResult struct {
	Answer     string           `json:"answer"`
	ToolCalls  []ToolCallRecord `json:"tool_calls,omitempty"`
	Iterations int              `json:"iterations"`
	MaxIter    int              `json:"max_iterations"`
	StopReason StopReason       `json:"stop_reason"`
	Usage      Usage            `json:"usage"`
}

// RunAgent runs an agentic loop: sends the user message to the LLM with tools,
// executes any tool calls server-side, feeds results back, and repeats until
// the LLM produces a final text answer or limits are reached.
func RunAgent(ctx context.Context, client Client, cfg AgentConfig, userMessage string, executeTool ToolExecutorFunc, history []Message) (*AgentResult, error) {
	maxIter := cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	messages := []Message{
		{Role: "system", Content: cfg.SystemPrompt},
	}
	messages = append(messages, history...)
	messages = append(messages, Message{Role: "user", Content: userMessage})

	var allToolCalls []ToolCallRecord
	var totalUsage Usage

	for i := 0; i < maxIter; i++ {
		resp, err := client.ChatCompletion(ctx, ChatCompletionRequest{
			Messages:    messages,
			Tools:       cfg.Tools,
			ToolChoice:  "auto",
			MaxTokens:   cfg.MaxTokens,
			Temperature: cfg.Temperature,
		})
		if err != nil {
			return nil, fmt.Errorf("LLM request failed (iteration %d): %w", i+1, err)
		}

		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("LLM returned no choices (iteration %d)", i+1)
		}

		choice := resp.Choices[0]

		// If no tool calls, this is the final answer
		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) == 0 {
			slog.Info("agent loop finished",
				slog.String("stop_reason", string(StopReasonDone)),
				slog.Int("iterations", i+1),
				slog.Int("max_iterations", maxIter),
				slog.Int("tool_calls", len(allToolCalls)),
				slog.Int("total_tokens", totalUsage.TotalTokens),
			)
			return &AgentResult{
				Answer:     choice.Message.Content,
				ToolCalls:  allToolCalls,
				Iterations: i + 1,
				MaxIter:    maxIter,
				StopReason: StopReasonDone,
				Usage:      totalUsage,
			}, nil
		}

		// Append the assistant message (with tool_calls) to history
		messages = append(messages, choice.Message)

		// Execute each tool call
		for _, tc := range choice.Message.ToolCalls {
			start := time.Now()
			result, execErr := executeTool(ctx, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				result = fmt.Sprintf(`{"error": "%s"}`, execErr.Error()) //nolint:gocritic // JSON string, not Go quoting
			}
			slog.Info("agent tool call",
				slog.Int("iteration", i+1),
				slog.String("tool", tc.Function.Name),
				slog.Int("arg_bytes", len(tc.Function.Arguments)),
				slog.Int("result_bytes", len(result)),
				slog.Duration("duration", time.Since(start)),
				slog.Bool("exec_error", execErr != nil),
				slog.Bool("tool_returned_error", toolReturnedError(result)),
			)

			allToolCalls = append(allToolCalls, ToolCallRecord{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
				Result:    result,
			})

			// Append tool result message
			messages = append(messages, Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
			})
		}
	}

	// Max iterations reached — return whatever we have. Callers should
	// surface this as a visible warning, not a normal answer.
	slog.Warn("agent loop exhausted iteration budget",
		slog.String("stop_reason", string(StopReasonMaxIterations)),
		slog.Int("iterations", maxIter),
		slog.Int("max_iterations", maxIter),
		slog.Int("tool_calls", len(allToolCalls)),
		slog.Int("total_tokens", totalUsage.TotalTokens),
	)
	return &AgentResult{
		Answer:     "I wasn't able to complete the task within the allowed number of steps. Here's what I found so far based on the tool calls I made.",
		ToolCalls:  allToolCalls,
		Iterations: maxIter,
		MaxIter:    maxIter,
		StopReason: StopReasonMaxIterations,
		Usage:      totalUsage,
	}, nil
}

// toolReturnedError is a best-effort check for the "soft error" convention
// used by the aitools registry: a tool returns success at the Go level but
// signals a user-facing problem by setting an "error" field on its JSON
// result. Used only for logging — never for control flow.
func toolReturnedError(result string) bool {
	// Avoid pulling in encoding/json for a single substring probe on the
	// hot path. The convention is `{"error":` (optionally preceded by
	// whitespace and `{`), which this catches without false positives on
	// keys like "errors" that contain it as a substring inside another
	// JSON value would require escaping.
	if len(result) < 8 {
		return false
	}
	for i := 0; i < len(result) && i < 8; i++ {
		c := result[i]
		if c == '{' {
			rest := result[i+1:]
			// Trim leading whitespace.
			for rest != "" && (rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n') {
				rest = rest[1:]
			}
			return len(rest) >= 8 && (rest[:8] == `"error":` || rest[:8] == `"error" `)
		}
	}
	return false
}
