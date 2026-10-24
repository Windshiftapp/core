package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"windshift/internal/customwidgets"
	"windshift/internal/llm"
)

// customWidgetMaxRequestRunes bounds the user's description of the widget.
const customWidgetMaxRequestRunes = 2000

// customWidgetMaxAttempts is the number of generation attempts: the first try
// plus one repair round that feeds the validation error back to the model.
const customWidgetMaxAttempts = 2

// CustomWidgetGenerateHandler turns a description into a widget program.
type CustomWidgetGenerateHandler struct {
	llm     llmFeatureResolver
	prompts *llm.PromptStore
}

func NewCustomWidgetGenerateHandler(llmResolver llmFeatureResolver, prompts *llm.PromptStore) *CustomWidgetGenerateHandler {
	return &CustomWidgetGenerateHandler{llm: llmResolver, prompts: prompts}
}

type customWidgetGenerateRequest struct {
	Prompt string `json:"prompt"`
	// CurrentProgram and CurrentQueries are set when refining an existing
	// widget, so the model edits rather than restarts.
	CurrentProgram string          `json:"current_program"`
	CurrentQueries json.RawMessage `json:"current_queries"`
	// WorkspaceID lets the prompt describe the workspace the widget will live
	// in. It never reaches a query: scope comes from the viewer.
	WorkspaceID *int `json:"workspace_id"`
}

type customWidgetDraft struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Program     string          `json:"program"`
	Queries     json.RawMessage `json:"queries"`
}

type customWidgetGenerateResponse struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Program     string          `json:"program"`
	Queries     json.RawMessage `json:"queries"`
	Attempts    int             `json:"attempts"`
}

// GenerateCustomWidget handles POST /api/custom-widgets/generate.
//
// The generated program and queries are validated with the same code that
// guards persistence, so a draft the builder can preview is a draft that can be
// saved. One repair attempt feeds the validation error back to the model.
func (h *CustomWidgetGenerateHandler) GenerateCustomWidget(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	request, ok := decodeJSON[customWidgetGenerateRequest](w, r)
	if !ok {
		return
	}
	userPrompt := strings.TrimSpace(request.Prompt)
	if userPrompt == "" {
		respondValidationError(w, r, "prompt is required")
		return
	}
	if len([]rune(userPrompt)) > customWidgetMaxRequestRunes {
		respondValidationError(w, r, fmt.Sprintf("prompt exceeds %d characters", customWidgetMaxRequestRunes))
		return
	}

	client := requireLLMClientForFeature(w, r, h.llm, "custom_widget", 0)
	if client == nil {
		return
	}

	extendWriteDeadline(w)
	ctx, cancel := context.WithTimeout(r.Context(), llm.DefaultRequestTimeout)
	defer cancel()

	systemPrompt := h.prompts.Get(llm.PromptCustomWidget)
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: buildWidgetRequest(userPrompt, request)},
	}

	var lastErr error
	for attempt := 1; attempt <= customWidgetMaxAttempts; attempt++ {
		draft, err := llm.CompleteStructured[customWidgetDraft](ctx, client, llm.CompletionRequest{
			Messages:    messages,
			Temperature: 0.2,
			StructuredOutput: &llm.StructuredOutputConfig{
				Schema:     llm.SchemaCustomWidget,
				SchemaName: "custom_widget",
				Strict:     true,
			},
		})
		if err != nil {
			respondLLMError(w, r, err)
			return
		}

		queries, err := validateWidgetDraft(draft)
		if err == nil {
			respondJSONOK(w, customWidgetGenerateResponse{
				Name:        draft.Name,
				Description: draft.Description,
				Program:     draft.Program,
				Queries:     queries,
				Attempts:    attempt,
			})
			return
		}
		lastErr = err

		slog.Info("custom widget draft rejected",
			"component", "custom_widget_generate",
			"attempt", attempt,
			"error", err,
		)
		if attempt == customWidgetMaxAttempts {
			break
		}
		// Feed the error back so the repair is targeted rather than a retry.
		messages = append(messages,
			llm.Message{Role: "assistant", Content: string(mustJSON(draft))},
			llm.Message{Role: "user", Content: "That draft was rejected: " + err.Error() + "\nReturn a corrected JSON object."},
		)
	}

	respondValidationError(w, r, fmt.Sprintf("the model could not produce a valid widget: %v", lastErr))
}

// validateWidgetDraft runs the same gates as saving a widget.
func validateWidgetDraft(draft *customWidgetDraft) (json.RawMessage, error) {
	if draft == nil {
		return nil, errors.New("empty draft")
	}
	if strings.TrimSpace(draft.Name) == "" {
		return nil, errors.New("name is required")
	}
	if len([]rune(draft.Name)) > customwidgets.MaxNameRunes {
		return nil, errors.New("name is too long")
	}
	if err := customwidgets.ValidateProgram(draft.Program); err != nil {
		return nil, err
	}
	queries, err := customwidgets.NormalizeQueries(draft.Queries)
	if err != nil {
		return nil, err
	}
	return queries, nil
}

func buildWidgetRequest(userPrompt string, request customWidgetGenerateRequest) string {
	var builder strings.Builder
	builder.WriteString("Build a dashboard widget for this request:\n\n")
	builder.WriteString(userPrompt)

	if strings.TrimSpace(request.CurrentProgram) != "" {
		builder.WriteString("\n\nThe user already has this widget and wants it changed. Edit it rather than starting over:\n\n")
		builder.WriteString(request.CurrentProgram)
		if len(request.CurrentQueries) > 0 && string(request.CurrentQueries) != "{}" {
			builder.WriteString("\n\nIts current queries:\n")
			builder.Write(request.CurrentQueries)
		}
	}
	return builder.String()
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}
