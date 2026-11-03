package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// GenerateReleaseNotesResponse is the structured LLM response for release notes generation.
type GenerateReleaseNotesResponse struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Notes   string `json:"notes"`
}

var errNoCompletedReleaseItems = errors.New("milestone has no completed work items to use as release-note sources")

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
		if errors.Is(err, repository.ErrNotFound) {
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

	workspaceIDs, err := h.permService.AccessibleWorkspaceIDs(user.ID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load visible workspaces: %w", err))
		return
	}

	// Load progress report for item counts and breakdown
	progress, err := planningService.GetMilestoneProgress(milestoneID, workspaceIDs)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load milestone progress: %w", err))
		return
	}

	// Resolve LLM client
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, "release_notes", parseConnectionIDParam(r))
	if llmClient == nil {
		return
	}

	// Load test stats if available
	var testStats *services.MilestoneTestStats
	loadedTestStats, testErr := planningService.GetMilestoneTestStatistics(milestoneID, workspaceIDs)
	if testErr == nil && loadedTestStats.TotalTestPlans > 0 {
		testStats = loadedTestStats
	}

	userPrompt, err := buildReleaseNotesUserPrompt(milestone, progress, testStats)
	if errors.Is(err, errNoCompletedReleaseItems) {
		respondConflict(w, r, "This milestone has no completed work items to generate release notes from.")
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to build release notes prompt: %w", err))
		return
	}

	systemPrompt := h.promptStore.Get(llm.PromptReleaseNotes)

	extendWriteDeadline(w)
	ctx, cancel := context.WithTimeout(r.Context(), llm.DefaultRequestTimeout)
	defer cancel()

	resp, err := llmClient.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
	})
	if err != nil {
		respondLLMError(w, r, err, h.permService)
		return
	}
	if len(resp.Choices) == 0 {
		respondServiceUnavailable(w, r, "AI service returned no response.")
		return
	}

	notes := strings.TrimSpace(resp.Choices[0].Message.Content)
	respondJSONOK(w, GenerateReleaseNotesResponse{Notes: notes})
}

func buildReleaseNotesUserPrompt(
	milestone *services.MilestoneResult,
	progress *services.MilestoneProgressReport,
	testStats *services.MilestoneTestStats,
) (string, error) {
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
	if totalItemsListed == 0 {
		return "", errNoCompletedReleaseItems
	}

	if testStats != nil {
		contextLines = append(contextLines, fmt.Sprintf("\nTest coverage: %d test plans, %d runs (%d successful, %d failed)",
			testStats.TotalTestPlans, testStats.TotalTestRuns, testStats.SuccessfulTestRuns, testStats.FailedTestRuns))
	}

	return fmt.Sprintf(`Generate release notes for this milestone using only the facts below.
Every release-note bullet must cite one or more supplied work-item keys. Do not infer features,
implementation details, user impact, or test results that are not explicitly supported by the input.

%s`, strings.Join(contextLines, "\n")), nil
}
