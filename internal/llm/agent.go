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

// AgentResult contains the outcome of an agent run.
type AgentResult struct {
	Answer     string           `json:"answer"`
	ToolCalls  []ToolCallRecord `json:"tool_calls,omitempty"`
	Iterations int              `json:"iterations"`
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
			return &AgentResult{
				Answer:     choice.Message.Content,
				ToolCalls:  allToolCalls,
				Iterations: i + 1,
				Usage:      totalUsage,
			}, nil
		}

		// Append the assistant message (with tool_calls) to history
		messages = append(messages, choice.Message)

		// Execute each tool call
		for _, tc := range choice.Message.ToolCalls {
			slog.Debug("agent executing tool", slog.String("tool", tc.Function.Name), slog.String("id", tc.ID))

			result, execErr := executeTool(ctx, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				result = fmt.Sprintf(`{"error": "%s"}`, execErr.Error()) //nolint:gocritic // JSON string, not Go quoting
			}

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

	// Max iterations reached — return whatever we have
	return &AgentResult{
		Answer:     "I wasn't able to complete the task within the allowed number of steps. Here's what I found so far based on the tool calls I made.",
		ToolCalls:  allToolCalls,
		Iterations: maxIter,
		Usage:      totalUsage,
	}, nil
}
