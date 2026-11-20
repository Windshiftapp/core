package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

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

const (
	// defaultReleaseNotesItemLimit bounds how many completed work items are sent
	// to the model when the operator has not set AI_RELEASE_NOTES_ITEM_LIMIT.
	defaultReleaseNotesItemLimit = 200
	// maxReleaseNotesItemLimit clamps an operator override so the enrichment
	// queries cannot build an unbounded IN (...) list or an oversized prompt.
	maxReleaseNotesItemLimit = 1000
	// defaultReleaseNotesTokenBudget bounds the whole release-notes input
	// (system + user message) so a milestone with many verbose items cannot
	// exceed the context window. Operators override it with
	// AI_RELEASE_NOTES_TOKEN_BUDGET.
	defaultReleaseNotesTokenBudget = 128_000
	// releaseNotesSystemPromptReserveTokens covers the release_notes.txt system
	// prompt plus a margin for token-estimation error.
	releaseNotesSystemPromptReserveTokens = 1_000
	// releaseNotesTruncationNoteReserveTokens covers the "Only X of Y items are
	// shown" footer.
	releaseNotesTruncationNoteReserveTokens = 64
	// releaseNotesMinTokenBudget is a floor for a misconfigured budget so the
	// builder still emits a usable prompt.
	releaseNotesMinTokenBudget = 1_000
	// releaseNotesDescriptionLimit and releaseNotesCommentLimit bound each
	// free-text field so one verbose item cannot crowd the rest of the
	// milestone context out of the model's window.
	releaseNotesDescriptionLimit = 4000
	releaseNotesCommentLimit     = 2000
	// releaseNotesMilestoneDescriptionLimit bounds the milestone's own
	// description, which would otherwise be an uncapped term in the budget.
	releaseNotesMilestoneDescriptionLimit = 4000
	// releaseNotesCommentsPerItem caps how many public comments per item are
	// included, newest first.
	releaseNotesCommentsPerItem = 5
)

// releaseNotesItem is one completed work item selected for the release-notes
// prompt, carrying the milestone progress row plus the enrichment the model
// needs to describe, group, and summarize it.
type releaseNotesItem struct {
	ID               int
	Key              string
	ItemNumber       int
	Title            string
	ItemType         string
	ParentKey        string
	ParentItemNumber int
	ParentTitle      string
	Description      string
	Comments         []repository.ReleaseNoteComment
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

	// Select completed items, cap the list, then enrich only the selected items
	// so a large milestone does not pay for context the model never sees.
	items := collectReleaseNotesItems(progress)
	if len(items) == 0 {
		respondConflict(w, r, "This milestone has no completed work items to generate release notes from.")
		return
	}
	totalCompleted := len(items)
	if limit := h.releaseNotesLimit(); len(items) > limit {
		items = items[:limit]
	}
	if err := enrichReleaseNoteItems(r.Context(), repository.NewItemRepository(h.db), items); err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load release note item context: %w", err))
		return
	}

	userPrompt := buildReleaseNotesUserPrompt(milestone, progress, testStats, items, totalCompleted, h.releaseNotesBudget())

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

// SetReleaseNotesItemLimit overrides the maximum number of completed work
// items included in generated release notes. Zero or negative keeps the
// built-in default. Wired from AI_RELEASE_NOTES_ITEM_LIMIT at startup.
func (h *AIHandler) SetReleaseNotesItemLimit(limit int) {
	h.releaseNotesItemLimit = limit
}

// SetReleaseNotesTokenBudget overrides the estimated-token budget for the
// whole release-notes input. Zero or negative keeps the built-in default.
// Wired from AI_RELEASE_NOTES_TOKEN_BUDGET at startup.
func (h *AIHandler) SetReleaseNotesTokenBudget(budget int) {
	h.releaseNotesTokenBudget = budget
}

// releaseNotesBudget returns the configured input-token budget, falling back
// to the built-in default and enforcing a floor so a misconfigured value still
// produces a usable prompt.
func (h *AIHandler) releaseNotesBudget() int {
	budget := h.releaseNotesTokenBudget
	if budget <= 0 {
		budget = defaultReleaseNotesTokenBudget
	}
	if budget < releaseNotesMinTokenBudget {
		budget = releaseNotesMinTokenBudget
	}
	return budget
}

// releaseNotesLimit returns the configured item cap, falling back to the
// built-in default when unset and clamping an oversized override.
func (h *AIHandler) releaseNotesLimit() int {
	limit := h.releaseNotesItemLimit
	if limit <= 0 {
		limit = defaultReleaseNotesItemLimit
	}
	if limit > maxReleaseNotesItemLimit {
		limit = maxReleaseNotesItemLimit
	}
	return limit
}

// collectReleaseNotesItems returns the milestone's completed items in
// status-breakdown order, which keeps the prompt stable across requests even
// though the progress report groups items in a map. Incomplete categories are
// skipped, matching the progress report's completion definition.
func collectReleaseNotesItems(progress *services.MilestoneProgressReport) []releaseNotesItem {
	if progress == nil {
		return nil
	}
	var items []releaseNotesItem
	for _, bd := range progress.StatusBreakdown {
		if !bd.IsCompleted {
			continue
		}
		for _, pi := range progress.ItemsByCategory[bd.CategoryName] {
			items = append(items, releaseNotesItem{
				ID:         pi.ID,
				Key:        pi.WorkspaceKey,
				ItemNumber: pi.ItemNumber,
				Title:      pi.Title,
			})
		}
	}
	return items
}

// enrichReleaseNoteItems fills each item in place with its description, type,
// parent and comments. Enrichment is keyed by item ID and bounded by the
// already-capped slice, so the reads stay proportional to the prompt size.
func enrichReleaseNoteItems(ctx context.Context, repo *repository.ItemRepository, items []releaseNotesItem) error {
	if len(items) == 0 {
		return nil
	}
	itemIDs := make([]int, 0, len(items))
	for _, item := range items {
		itemIDs = append(itemIDs, item.ID)
	}
	enrichment, err := repo.ReleaseNoteItemsContext(ctx, itemIDs, releaseNotesCommentsPerItem)
	if err != nil {
		return err
	}
	for i := range items {
		extra, ok := enrichment[items[i].ID]
		if !ok {
			continue
		}
		items[i].Description = extra.Description
		items[i].ItemType = extra.ItemTypeName
		items[i].ParentKey = extra.ParentKey
		items[i].ParentItemNumber = extra.ParentItemNumber
		items[i].ParentTitle = extra.ParentTitle
		items[i].Comments = extra.Comments
	}
	return nil
}

// buildReleaseNotesUserPrompt renders the milestone facts and the enriched work
// items as plain data. All generation instructions live in the overridable
// system prompt (release_notes.txt) so an operator override fully controls the
// output format.
//
// tokenBudget caps the whole input (system prompt included) using a
// script-aware estimate. Items are added whole until the next one would exceed
// the budget; the omitted count is disclosed so the model does not invent
// details for work it never saw.
func buildReleaseNotesUserPrompt(
	milestone *services.MilestoneResult,
	progress *services.MilestoneProgressReport,
	testStats *services.MilestoneTestStats,
	items []releaseNotesItem,
	totalCompleted int,
	tokenBudget int,
) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Milestone: %s\n", milestone.Name)
	if milestone.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", truncateReleaseNoteText(milestone.Description, releaseNotesMilestoneDescriptionLimit))
	}
	if milestone.TargetDate != "" {
		fmt.Fprintf(&b, "Target Date: %s\n", milestone.TargetDate)
	}
	fmt.Fprintf(&b, "Progress: %d/%d items completed (%.0f%%)\n",
		progress.CompletedItems, progress.TotalItems, progress.PercentComplete)

	if len(progress.StatusBreakdown) > 0 {
		b.WriteString("\nStatus breakdown:\n")
		for _, bd := range progress.StatusBreakdown {
			fmt.Fprintf(&b, "  - %s: %d items\n", bd.CategoryName, bd.ItemCount)
		}
	}

	b.WriteString("\nCompleted work items:\n")

	testLine := ""
	if testStats != nil {
		testLine = fmt.Sprintf("\nTest coverage: %d test plans, %d runs (%d successful, %d failed)\n",
			testStats.TotalTestPlans, testStats.TotalTestRuns, testStats.SuccessfulTestRuns, testStats.FailedTestRuns)
	}

	// The footer is always emitted, so reserve its estimated cost before adding
	// items. The system prompt shares the same context window.
	budget := tokenBudget - releaseNotesSystemPromptReserveTokens
	if budget < releaseNotesMinTokenBudget {
		budget = releaseNotesMinTokenBudget
	}
	reserve := estimateReleaseNotesTokens(testLine) + releaseNotesTruncationNoteReserveTokens
	used := estimateReleaseNotesTokens(b.String())

	included := 0
	for _, item := range items {
		block := releaseNotesItemBlock(item)
		blockTokens := estimateReleaseNotesTokens(block)
		if used+blockTokens+reserve > budget {
			break
		}
		b.WriteString(block)
		used += blockTokens
		included++
	}

	if totalCompleted > included {
		fmt.Fprintf(&b, "\nOnly %d of %d completed items are shown; do not invent details for the omitted items.\n",
			included, totalCompleted)
	}
	b.WriteString(testLine)

	return b.String()
}

// releaseNotesItemBlock renders one work item and its optional parent,
// description, and comments, applying the per-field truncation limits.
func releaseNotesItemBlock(item releaseNotesItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- %s-%d", item.Key, item.ItemNumber)
	if item.ItemType != "" {
		fmt.Fprintf(&b, " [%s]", item.ItemType)
	}
	fmt.Fprintf(&b, ": %s\n", item.Title)
	if item.ParentKey != "" {
		fmt.Fprintf(&b, "  Parent: %s-%d %s\n", item.ParentKey, item.ParentItemNumber, item.ParentTitle)
	}
	if item.Description != "" {
		fmt.Fprintf(&b, "  Description: %s\n", truncateReleaseNoteText(item.Description, releaseNotesDescriptionLimit))
	}
	if len(item.Comments) > 0 {
		b.WriteString("  Comments (newest first):\n")
		for _, comment := range item.Comments {
			fmt.Fprintf(&b, "    - %s %s: %s\n", comment.CreatedAt.Format("2006-01-02"), comment.Author, truncateReleaseNoteText(comment.Content, releaseNotesCommentLimit))
		}
	}
	return b.String()
}

// estimateReleaseNotesTokens approximates the tokenizer cost of the prompt.
// ASCII text averages roughly four characters per token, while non-ASCII
// scripts (CJK, emoji, etc.) tokenize at close to one token per rune. Counting
// non-ASCII runes one-for-one keeps the estimate conservative for exactly the
// scripts where a character-based estimate would overflow the context window.
func estimateReleaseNotesTokens(s string) int {
	ascii := 0
	nonASCII := 0
	for _, r := range s {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+3)/4 + nonASCII
}

// truncateReleaseNoteText flattens a description or comment to a single line
// and bounds it to limit runes, appending an ellipsis when content is dropped.
// Flattening keeps the line-oriented prompt structure intact, and runes are
// counted rather than bytes so multi-byte text is never cut mid-character.
func truncateReleaseNoteText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}
