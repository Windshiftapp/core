package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// CatchMeUpResponse is the response for the Catch Me Up endpoint.
type CatchMeUpResponse struct {
	Briefing string `json:"briefing"`
	ItemKey  string `json:"item_key"`
}

// FindSimilarResponse is the response for the Find Similar Items endpoint.
type FindSimilarResponse struct {
	SimilarItems []SimilarItem `json:"similar_items"`
	Summary      string        `json:"summary"`
}

// SimilarItem represents a similar item identified by the LLM.
type SimilarItem struct {
	ItemID      int    `json:"item_id"`
	ItemKey     string `json:"item_key"`
	Title       string `json:"title"`
	StatusName  string `json:"status_name"`
	Similarity  string `json:"similarity"`
	Reason      string `json:"reason"`
	WorkspaceID int    `json:"workspace_id"`
}

// DecomposeResponse is the response for the Decompose Item endpoint.
type DecomposeResponse struct {
	SubTasks  []SuggestedSubTask `json:"sub_tasks"`
	Reasoning string             `json:"reasoning"`
}

// SuggestedSubTask represents a suggested sub-task from item decomposition.
type SuggestedSubTask struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// loadItemWithPermission handles auth, item loading, permission checks, and LLM client resolution.
// Returns nil values and false if any step fails (response already written).
func (h *AIHandler) loadItemWithPermission(w http.ResponseWriter, r *http.Request, feature string) (*models.Item, llm.Client, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, nil, false
	}

	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return nil, nil, false
	}

	crudService := services.NewItemCRUDService(h.db)
	item, err := crudService.GetByID(itemID)
	if err != nil {
		respondNotFound(w, r, "item")
		return nil, nil, false
	}

	canView, err := h.permService.HasWorkspacePermission(user.ID, item.WorkspaceID, models.PermissionItemView)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check permissions: %w", err))
		return nil, nil, false
	}
	if !canView {
		respondForbidden(w, r)
		return nil, nil, false
	}

	llmClient := requireLLMClientForFeature(w, r, h.llmManager, feature, 0)
	if llmClient == nil {
		return nil, nil, false
	}

	return item, llmClient, true
}

const (
	// catchMeUpMaxChildren caps direct sub-items in a briefing. Grandchildren are
	// never included.
	catchMeUpMaxChildren = 20
	// catchMeUpMaxComments bounds comments quoted from any item.
	catchMeUpMaxComments = 20
	// catchMeUpMaxDescriptionBytes caps any item description in the context.
	catchMeUpMaxDescriptionBytes = 6 * 1024
	// catchMeUpMaxCommentBytes caps a single quoted comment.
	catchMeUpMaxCommentBytes = 300
)

// truncateBriefingText caps s at maxBytes without splitting a UTF-8 rune.
func truncateBriefingText(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := s[:maxBytes]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "..."
}

// appendHierarchyContext formats already-loaded parent and direct-sub-item
// context. It performs no lookups and never recurses. moreChildren marks that
// the child list was capped so the model knows the list is partial.
func appendHierarchyContext(lines []string, parent *models.Item, parentComments []services.ItemCommentSummary, children []repository.ChildItemSummary, moreChildren bool) []string {
	if parent != nil {
		parentKey := fmt.Sprintf("%s-%d", parent.WorkspaceKey, parent.WorkspaceItemNumber)
		lines = append(lines, fmt.Sprintf("\nParent item: %s - %s", parentKey, parent.Title))
		if parent.StatusName != "" {
			lines = append(lines, fmt.Sprintf("Parent status: %s", parent.StatusName))
		}
		if parent.Description != "" {
			lines = append(lines, fmt.Sprintf("\nParent description:\n%s", truncateBriefingText(parent.Description, catchMeUpMaxDescriptionBytes)))
		}
		if len(parentComments) > 0 {
			lines = append(lines, "\nParent recent comments:")
			for _, comment := range parentComments {
				lines = append(lines, fmt.Sprintf("- %s (%s): %s", comment.Author, comment.CreatedAt.Format("Jan 2"), truncateBriefingText(comment.Content, catchMeUpMaxCommentBytes)))
			}
		}
	}

	if len(children) > 0 {
		lines = append(lines, fmt.Sprintf("\nSub-items (%d):", len(children)))
		for _, child := range children {
			line := fmt.Sprintf("- %s: %s", child.ItemKey, child.Title)
			if child.ItemTypeName != "" {
				line += fmt.Sprintf(" [%s]", child.ItemTypeName)
			}
			if child.StatusName != "" {
				line += fmt.Sprintf(" status=%s", child.StatusName)
			}
			if child.AssigneeName != "" {
				line += fmt.Sprintf(" assignee=%s", child.AssigneeName)
			}
			lines = append(lines, line)
			if child.Description != "" {
				lines = append(lines, fmt.Sprintf("  Description: %s", truncateBriefingText(child.Description, catchMeUpMaxDescriptionBytes)))
			}
		}
		if moreChildren {
			lines = append(lines, fmt.Sprintf("(showing the first %d sub-items; more exist)", len(children)))
		}
	}
	return lines
}

// CatchMeUp generates a summary briefing for an item.
func (h *AIHandler) CatchMeUp(w http.ResponseWriter, r *http.Request) {
	item, llmClient, ok := h.loadItemWithPermission(w, r, "catch_me_up")
	if !ok {
		return
	}

	itemID := item.ID

	// Build item key
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	// Assemble context
	var contextLines []string
	contextLines = append(contextLines, fmt.Sprintf("Item: %s - %s", itemKey, item.Title))
	if item.StatusName != "" {
		contextLines = append(contextLines, fmt.Sprintf("Status: %s", item.StatusName))
	}
	if item.PriorityName != "" {
		contextLines = append(contextLines, fmt.Sprintf("Priority: %s", item.PriorityName))
	}
	if item.AssigneeName != "" {
		contextLines = append(contextLines, fmt.Sprintf("Assignee: %s", item.AssigneeName))
	}
	if item.ItemTypeName != "" {
		contextLines = append(contextLines, fmt.Sprintf("Type: %s", item.ItemTypeName))
	}
	if item.DueDate != nil {
		contextLines = append(contextLines, fmt.Sprintf("Due date: %s", item.DueDate.Format("2006-01-02")))
	}
	if item.Description != "" {
		contextLines = append(contextLines, fmt.Sprintf("\nDescription:\n%s", truncateBriefingText(item.Description, catchMeUpMaxDescriptionBytes)))
	}

	crudService := services.NewItemCRUDService(h.db)
	itemRepo := repository.NewItemRepository(h.db)
	commentService := h.commentService
	if commentService == nil {
		commentService = services.NewCommentService(h.db)
	}

	var parent *models.Item
	if item.ParentID != nil {
		loadedParent, err := crudService.GetByID(*item.ParentID)
		if err != nil {
			slog.Warn("failed to load parent item", slog.String("component", "ai"), slog.Any("error", err))
		} else {
			parent = loadedParent
		}
	}

	var parentComments []services.ItemCommentSummary
	if parent != nil {
		loadedComments, err := commentService.ListRecentSummaries(parent.ID, catchMeUpMaxComments)
		if err != nil {
			slog.Warn("failed to load parent comments", slog.String("component", "ai"), slog.Any("error", err))
		} else {
			parentComments = loadedComments
		}
	}

	// Direct children only, capped: key, title, type, status, and assignee.
	// Fetch one extra to detect a truncated list without a second query.
	children, err := itemRepo.ListChildBriefings(item.WorkspaceID, itemID, catchMeUpMaxChildren+1)
	moreChildren := false
	if err != nil {
		slog.Warn("failed to load child items", slog.String("component", "ai"), slog.Any("error", err))
	} else if len(children) > catchMeUpMaxChildren {
		children = children[:catchMeUpMaxChildren]
		moreChildren = true
	}

	contextLines = appendHierarchyContext(contextLines, parent, parentComments, children, moreChildren)

	commentRows, err := commentService.ListRecentSummaries(itemID, catchMeUpMaxComments)
	if err == nil {
		comments := make([]string, 0, len(commentRows))
		for _, comment := range commentRows {
			comments = append(comments, fmt.Sprintf("- %s (%s): %s", comment.Author, comment.CreatedAt.Format("Jan 2"), truncateBriefingText(comment.Content, catchMeUpMaxCommentBytes)))
		}
		if len(comments) > 0 {
			contextLines = append(contextLines, "\nRecent comments:")
			contextLines = append(contextLines, comments...)
		}
	} else {
		slog.Warn("failed to load recent comments", slog.String("component", "ai"), slog.Any("error", err))
	}

	// Load history (last 30 changes)
	history, err := crudService.GetHistory(itemID)
	if err == nil && len(history) > 0 {
		limit := 30
		if len(history) < limit {
			limit = len(history)
		}
		var historyLines []string
		for _, entry := range history[:limit] {
			line := fmt.Sprintf("- %s changed '%s'", entry.UserName, entry.FieldName)
			oldVal := ""
			newVal := ""
			if entry.ResolvedOldValue != nil {
				oldVal = *entry.ResolvedOldValue
			} else if entry.OldValue != nil {
				oldVal = *entry.OldValue
			}
			if entry.ResolvedNewValue != nil {
				newVal = *entry.ResolvedNewValue
			} else if entry.NewValue != nil {
				newVal = *entry.NewValue
			}
			if oldVal != "" || newVal != "" {
				line += fmt.Sprintf(": '%s' → '%s'", oldVal, newVal)
			}
			historyLines = append(historyLines, line)
		}
		if len(historyLines) > 0 {
			contextLines = append(contextLines, "\nRecent changes:")
			contextLines = append(contextLines, historyLines...)
		}
	}

	// Load item links
	linkRows, err := repository.NewItemLinkRepository(h.db).ListLinkedItemSummaries(itemID)
	if err == nil {
		links := make([]string, 0, len(linkRows))
		for _, link := range linkRows {
			links = append(links, fmt.Sprintf("- %s: [%s] %s", link.LinkType, link.ItemKey, link.Title))
		}
		if len(links) > 0 {
			contextLines = append(contextLines, "\nLinked items:")
			contextLines = append(contextLines, links...)
		}
	} else {
		slog.Warn("failed to load linked items", slog.String("component", "ai"), slog.Any("error", err))
	}

	// Load SCM links
	scmRows, err := repository.NewSCMWorkspaceRepository(h.db).ListItemSCMLinkSummaries(itemID)
	if err == nil {
		scmLinks := make([]string, 0, len(scmRows))
		for _, link := range scmRows {
			scmLinks = append(scmLinks, fmt.Sprintf("- PR: %s (branch: %s, state: %s)", link.Title, link.BranchName, link.State))
		}
		if len(scmLinks) > 0 {
			contextLines = append(contextLines, "\nSource control:")
			contextLines = append(contextLines, scmLinks...)
		}
	} else {
		slog.Warn("failed to load SCM links", slog.String("component", "ai"), slog.Any("error", err))
	}

	systemPrompt := h.promptStore.Get(llm.PromptCatchMeUp)

	userPrompt := fmt.Sprintf("Please catch me up on this work item:\n\n%s", strings.Join(contextLines, "\n"))

	extendWriteDeadline(w)
	ctx, cancel := context.WithTimeout(r.Context(), llm.DefaultRequestTimeout)
	defer cancel()

	resp, err := llmClient.Complete(ctx, llm.CompletionRequest{
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

	briefing := resp.Choices[0].Message.Content
	respondJSONOK(w, CatchMeUpResponse{
		Briefing: briefing,
		ItemKey:  itemKey,
	})
}

// FindSimilarItems identifies similar items in the same workspace.
func (h *AIHandler) FindSimilarItems(w http.ResponseWriter, r *http.Request) {
	item, llmClient, ok := h.loadItemWithPermission(w, r, "find_similar")
	if !ok {
		return
	}

	itemID := item.ID
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	// Load candidate items: last 100 open items in same workspace (excluding current)
	candidates, err := repository.NewItemRepository(h.db).ListOpenCandidatesInWorkspace(item.WorkspaceID, itemID, 100)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to query candidate items: %w", err))
		return
	}

	type candidateItem = repository.CandidateItem

	candidateMap := make(map[string]candidateItem, len(candidates)) // key → candidate
	var candidateLines []string
	for _, c := range candidates {
		candidateMap[c.ItemKey] = c
		desc := c.Description
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		candidateLines = append(candidateLines, fmt.Sprintf("- %s | %s | %s", c.ItemKey, c.Title, desc))
	}

	if len(candidates) == 0 {
		respondJSONOK(w, FindSimilarResponse{
			SimilarItems: []SimilarItem{},
			Summary:      "No other open items in this workspace to compare against.",
		})
		return
	}

	currentDesc := item.Description
	if len(currentDesc) > 500 {
		currentDesc = currentDesc[:500] + "..."
	}

	systemPrompt := h.promptStore.Get(llm.PromptFindSimilar)

	userPrompt := fmt.Sprintf(`Current item %s: %s
Description: %s

Candidate items in the same workspace:
%s

Find similar items.`, itemKey, item.Title, currentDesc, strings.Join(candidateLines, "\n"))

	extendWriteDeadline(w)
	ctx, cancel := context.WithTimeout(r.Context(), llm.DefaultRequestTimeout)
	defer cancel()

	result, err := llm.CompleteStructured[FindSimilarResponse](ctx, llmClient, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		StructuredOutput: &llm.StructuredOutputConfig{
			Schema:     llm.SchemaFindSimilar,
			SchemaName: "find_similar",
			Strict:     true,
		},
	})
	if err != nil {
		respondLLMError(w, r, err)
		return
	}

	// Enrich results from our candidate data (don't trust LLM for titles/IDs)
	enriched := make([]SimilarItem, 0, len(result.SimilarItems))
	for _, si := range result.SimilarItems {
		key := strings.TrimPrefix(strings.TrimSuffix(si.ItemKey, "]"), "[")
		if candidate, ok := candidateMap[key]; ok {
			enriched = append(enriched, SimilarItem{
				ItemID:      candidate.ID,
				ItemKey:     candidate.ItemKey,
				Title:       candidate.Title,
				StatusName:  candidate.StatusName,
				Similarity:  si.Similarity,
				Reason:      si.Reason,
				WorkspaceID: item.WorkspaceID,
			})
		}
	}
	result.SimilarItems = enriched

	respondJSONOK(w, *result)
}

// DecomposeItem suggests sub-tasks for an item.
func (h *AIHandler) DecomposeItem(w http.ResponseWriter, r *http.Request) {
	item, llmClient, ok := h.loadItemWithPermission(w, r, "decompose")
	if !ok {
		return
	}

	itemID := item.ID
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	var childTypeNames []string
	var err error
	if item.ItemTypeID != nil {
		childTypeNames, err = repository.NewItemTypeRepository(h.db).ListChildNames(*item.ItemTypeID, item.WorkspaceID)
		if err != nil {
			slog.Warn("error loading child item types", slog.String("component", "ai"), slog.Any("error", err))
			childTypeNames = nil
		}
	}

	// Get existing children titles
	existingChildren, err := repository.NewItemRepository(h.db).ListChildTitles(itemID)
	if err != nil {
		slog.Warn("error loading child titles", slog.String("component", "ai"), slog.Any("error", err))
		existingChildren = nil
	}

	desc := item.Description
	if len(desc) > 3000 {
		desc = desc[:3000] + "..."
	}

	var contextParts []string
	contextParts = append(contextParts, fmt.Sprintf("Item [%s]: %s", itemKey, item.Title))
	if item.ItemTypeName != "" {
		contextParts = append(contextParts, fmt.Sprintf("Type: %s", item.ItemTypeName))
	}
	if desc != "" {
		contextParts = append(contextParts, fmt.Sprintf("\nDescription:\n%s", desc))
	}
	if len(childTypeNames) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("\nAvailable child item types: %s", strings.Join(childTypeNames, ", ")))
	}
	if len(existingChildren) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("\nExisting children (avoid duplicates): %s", strings.Join(existingChildren, "; ")))
	}

	systemPrompt := h.promptStore.Get(llm.PromptDecompose)

	userPrompt := fmt.Sprintf("Break this work item into sub-tasks:\n\n%s", strings.Join(contextParts, "\n"))

	extendWriteDeadline(w)
	ctx, cancel := context.WithTimeout(r.Context(), llm.DefaultRequestTimeout)
	defer cancel()

	result, err := llm.CompleteStructured[DecomposeResponse](ctx, llmClient, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.7,
		StructuredOutput: &llm.StructuredOutputConfig{
			Schema:     llm.SchemaDecompose,
			SchemaName: "decompose",
			Strict:     true,
		},
	})
	if err != nil {
		respondLLMError(w, r, err)
		return
	}

	respondJSONOK(w, *result)
}
