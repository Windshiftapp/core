package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
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

// CompleteStructured calls Complete and parses the JSON response.
// On parse failure, it retries once before returning an error.
// T must be a struct type that matches the JSON Schema in req.StructuredOutput.
func CompleteStructured[T any](
	ctx context.Context,
	client Client,
	req CompletionRequest,
) (*T, error) {
	// The parse/schema retry below is the same logical request, so both
	// attempts share one provider session.
	if SessionIDFromContext(ctx) == "" {
		ctx = WithSessionID(ctx, NewSessionID("ws-structured"))
	}
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := client.Complete(ctx, req)
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
		if req.StructuredOutput != nil && len(req.StructuredOutput.Schema) > 0 {
			if err := validateStructuredJSON(req.StructuredOutput.Schema, []byte(content)); err != nil {
				if attempt == 0 {
					slog.Warn("structured output schema validation failed, retrying",
						slog.Any("error", err),
						slog.String("content_preview", truncate(content, 200)))
					continue
				}
				return nil, fmt.Errorf("response failed schema validation after retry: %w", err)
			}
		}
		return &result, nil
	}
	return nil, ErrNoResponse // unreachable
}

func validateStructuredJSON(schemaRaw, valueRaw []byte) error {
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaRaw))
	if err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("schema.json", schemaDoc); err != nil {
		return fmt.Errorf("add schema resource: %w", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("compile JSON schema: %w", err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(valueRaw))
	if err != nil {
		return fmt.Errorf("invalid JSON response: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("validate JSON response: %w", err)
	}
	return nil
}

// truncate returns the first n characters of s, or all of s if shorter.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
