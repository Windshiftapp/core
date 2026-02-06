package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// stripMarkdownCodeBlock extracts JSON content from markdown code blocks.
// Handles formats like: ```json\n{...}\n``` or ```\n{...}\n```
func stripMarkdownCodeBlock(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Skip the opening ``` line (may include language hint)
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[idx+1:]
	}
	// Remove closing ```
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// ErrNoResponse is returned when the LLM returns no choices.
var ErrNoResponse = errors.New("LLM returned no response")

// ChatCompletionStructured calls ChatCompletion and parses the JSON response.
// On parse failure, it retries once before returning an error.
// T must be a struct type that matches the JSON Schema in req.StructuredOutput.
func ChatCompletionStructured[T any](
	ctx context.Context,
	client Client,
	req ChatCompletionRequest,
) (*T, error) {
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := client.ChatCompletion(ctx, req)
		if err != nil {
			return nil, err
		}
		if len(resp.Choices) == 0 {
			return nil, ErrNoResponse
		}

		content := resp.Choices[0].Message.Content
		content = stripMarkdownCodeBlock(content)
		var result T
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			if attempt == 0 {
				slog.Warn("structured output parse failed, retrying",
					slog.Any("error", err),
					slog.String("content_preview", truncate(content, 200)))
				continue // retry once
			}
			return nil, fmt.Errorf("failed to parse response after retry: %w", err)
		}
		return &result, nil
	}
	return nil, ErrNoResponse // unreachable
}

// truncate returns the first n characters of s, or all of s if shorter.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
