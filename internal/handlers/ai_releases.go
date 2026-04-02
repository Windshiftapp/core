package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/services"
)

// GenerateReleaseNotesResponse is the structured LLM response for release notes generation.
type GenerateReleaseNotesResponse struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Notes   string `json:"notes"`
}

// GenerateReleaseNotes generates release notes for a milestone using the LLM.
func (h *AIHandler) GenerateReleaseNotes(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	milestoneID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load the milestone
	planningService := services.NewPlanningService(h.db)
	milestone, err := planningService.GetMilestone(milestoneID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to load milestone: %w", err))
		return
	}

	// Check permission based on milestone scope
	if milestone.IsGlobal {
		hasPerm, permErr := h.permService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
		if permErr != nil || !hasPerm {
			respondForbidden(w, r)
			return
		}
	} else if milestone.WorkspaceID != nil {
		canView, permErr := h.permService.HasWorkspacePermission(user.ID, *milestone.WorkspaceID, models.PermissionItemView)
		if permErr != nil || !canView {
			respondForbidden(w, r)
			return
		}
	}

	// Load progress report for item counts and breakdown
	progress, err := planningService.GetMilestoneProgress(milestoneID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load milestone progress: %w", err))
		return
	}

	// Resolve LLM client
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, h.db, "release_notes", parseConnectionIDParam(r))
	if llmClient == nil {
		return
	}

	// Build prompt context
	var contextLines []string
	contextLines = append(contextLines, fmt.Sprintf("Milestone: %s", milestone.Name))
	if milestone.Description != "" {
		contextLines = append(contextLines, fmt.Sprintf("Description: %s", milestone.Description))
	}
	if milestone.TargetDate != "" {
		contextLines = append(contextLines, fmt.Sprintf("Target Date: %s", milestone.TargetDate))
	}
	contextLines = append(contextLines, fmt.Sprintf("Progress: %d/%d items completed (%.0f%%)",
		progress.CompletedItems, progress.TotalItems, progress.PercentComplete))

	// Include status breakdown
	if len(progress.StatusBreakdown) > 0 {
		contextLines = append(contextLines, "\nStatus breakdown:")
		for _, bd := range progress.StatusBreakdown {
			contextLines = append(contextLines, fmt.Sprintf("  - %s: %d items", bd.CategoryName, bd.ItemCount))
		}
	}

	// Include completed item titles (cap at 50 total)
	totalItemsListed := 0
	if len(progress.ItemsByCategory) > 0 {
		contextLines = append(contextLines, "\nCompleted work items:")
		for categoryName, items := range progress.ItemsByCategory {
			// Only include completed-category items
			isCompleted := false
			for _, bd := range progress.StatusBreakdown {
				if bd.CategoryName == categoryName && bd.IsCompleted {
					isCompleted = true
					break
				}
			}
			if !isCompleted {
				continue
			}
			for _, item := range items {
				if totalItemsListed >= 50 {
					break
				}
				contextLines = append(contextLines, fmt.Sprintf("  - %s-%d: %s", item.WorkspaceKey, item.ItemNumber, item.Title))
				totalItemsListed++
			}
			if totalItemsListed >= 50 {
				break
			}
		}
	}

	// Load test stats if available
	testStats, testErr := planningService.GetMilestoneTestStatistics(milestoneID)
	if testErr == nil && testStats.TotalTestPlans > 0 {
		contextLines = append(contextLines, fmt.Sprintf("\nTest coverage: %d test plans, %d runs (%d successful, %d failed)",
			testStats.TotalTestPlans, testStats.TotalTestRuns, testStats.SuccessfulTestRuns, testStats.FailedTestRuns))
	}

	systemPrompt := h.promptStore.Get(llm.PromptReleaseNotes)

	userPrompt := fmt.Sprintf("Generate release notes for this milestone:\n\n%s", strings.Join(contextLines, "\n"))

	extendWriteDeadline(w)
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	resp, err := llmClient.ChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.7,
	})
	if err != nil {
		respondLLMError(w, r, err)
		return
	}
	if len(resp.Choices) == 0 {
		respondServiceUnavailable(w, r, "AI service returned no response.")
		return
	}

	notes := strings.TrimSpace(resp.Choices[0].Message.Content)
	respondJSONOK(w, GenerateReleaseNotesResponse{Notes: notes})
}
