package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// SummarizeTestPlanResponse is the structured response for the test plan
// description summarizer.
type SummarizeTestPlanResponse struct {
	Description string `json:"description"`
}

// SummarizeTestPlanDescription generates a plain-text description for a test
// plan from its name, optional milestone, and member test cases.
func (h *AIHandler) SummarizeTestPlanDescription(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	testSetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	repo := repository.NewTestSetRepository(h.db)

	// Resolve workspace from the test set itself (callers don't pass workspaceId).
	workspaceID, err := repo.GetWorkspaceID(testSetID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "test_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Permission: the result is for editing — gate on test.manage. On failure
	// return 404 to avoid disclosing test set existence to unauthorized users.
	hasPerm, permErr := h.permService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if permErr != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check test permissions: %w", permErr))
		return
	}
	if !hasPerm {
		respondNotFound(w, r, "test_set")
		return
	}

	set, err := repo.FindByID(testSetID, workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "test_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load test set: %w", err))
		return
	}

	cases, err := repo.FindTestCases(testSetID, workspaceID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load test cases: %w", err))
		return
	}

	llmClient := requireLLMClientForFeature(w, r, h.llmManager, llm.PromptSummarizeTestPlan, parseConnectionIDParam(r))
	if llmClient == nil {
		return
	}

	// Build prompt context
	var lines []string
	lines = append(lines, fmt.Sprintf("Test plan name: %s", set.Name))
	if set.MilestoneName != "" {
		lines = append(lines, fmt.Sprintf("Milestone: %s", set.MilestoneName))
	}
	if strings.TrimSpace(set.Description) != "" {
		lines = append(lines, fmt.Sprintf("Current description: %s", set.Description))
	}
	lines = append(lines, fmt.Sprintf("Number of test cases: %d", len(cases)))

	if len(cases) > 0 {
		lines = append(lines, "", "Test cases:")
		const maxCases = 50
		const maxPrecondition = 120
		for i, tc := range cases {
			if i >= maxCases {
				lines = append(lines, fmt.Sprintf("  ...and %d more", len(cases)-maxCases))
				break
			}
			line := fmt.Sprintf("  - %s", tc.Title)
			pc := strings.TrimSpace(tc.Preconditions)
			if pc != "" {
				if len(pc) > maxPrecondition {
					pc = pc[:maxPrecondition] + "..."
				}
				line += fmt.Sprintf(" (preconditions: %s)", pc)
			}
			lines = append(lines, line)
		}
	}

	systemPrompt := h.promptStore.Get(llm.PromptSummarizeTestPlan)
	userPrompt := fmt.Sprintf("Write a description for this test plan:\n\n%s", strings.Join(lines, "\n"))

	extendWriteDeadline(w)
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	resp, err := llmClient.ChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.5,
	})
	if err != nil {
		respondLLMError(w, r, err)
		return
	}
	if len(resp.Choices) == 0 {
		respondServiceUnavailable(w, r, "AI service returned no response.")
		return
	}

	description := strings.TrimSpace(resp.Choices[0].Message.Content)
	respondJSONOK(w, SummarizeTestPlanResponse{Description: description})
}
