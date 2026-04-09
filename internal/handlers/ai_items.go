package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/llm"
	"windshift/internal/models"
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

// CatchMeUp generates a summary briefing for an item.
func (h *AIHandler) CatchMeUp(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load item
	crudService := services.NewItemCRUDService(h.db)
	item, err := crudService.GetByID(itemID)
	if err != nil {
		respondNotFound(w, r, "item")
		return
	}

	// Check permission
	canView, err := h.permService.HasWorkspacePermission(user.ID, item.WorkspaceID, models.PermissionItemView)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check permissions: %w", err))
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	// Resolve LLM client
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, h.db, "catch_me_up", 0)
	if llmClient == nil {
		return
	}

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
		desc := item.Description
		if len(desc) > 2000 {
			desc = desc[:2000] + "..."
		}
		contextLines = append(contextLines, fmt.Sprintf("\nDescription:\n%s", desc))
	}

	// Load comments (last 20)
	commentRows, err := h.db.Query(
		`SELECT c.content, COALESCE(u.first_name || ' ' || u.last_name, 'Unknown'), c.created_at FROM comments c
		 LEFT JOIN users u ON c.author_id = u.id
		 WHERE c.item_id = ? ORDER BY c.created_at DESC LIMIT 20`, itemID)
	if err == nil {
		defer func() { _ = commentRows.Close() }()
		var comments []string
		for commentRows.Next() {
			var content, author string
			var createdAt time.Time
			if err = commentRows.Scan(&content, &author, &createdAt); err == nil {
				if len(content) > 300 {
					content = content[:300] + "..."
				}
				comments = append(comments, fmt.Sprintf("- %s (%s): %s", author, createdAt.Format("Jan 2"), content))
			}
		}
		if err := commentRows.Err(); err != nil {
			slog.Warn("error iterating comment rows", slog.String("component", "ai"), slog.Any("error", err))
		}
		if len(comments) > 0 {
			contextLines = append(contextLines, "\nRecent comments:")
			contextLines = append(contextLines, comments...)
		}
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
	linkRows, err := h.db.Query(
		`SELECT lt.name, i2.title, CONCAT(w.key, '-', i2.workspace_item_number) as item_key
		 FROM item_links il
		 JOIN link_types lt ON il.link_type_id = lt.id
		 JOIN items i2 ON (CASE WHEN il.source_item_id = ? THEN il.target_item_id ELSE il.source_item_id END) = i2.id
		 JOIN workspaces w ON i2.workspace_id = w.id
		 WHERE il.source_item_id = ? OR il.target_item_id = ?`, itemID, itemID, itemID)
	if err == nil {
		defer func() { _ = linkRows.Close() }()
		var links []string
		for linkRows.Next() {
			var linkType, title, key string
			if err = linkRows.Scan(&linkType, &title, &key); err == nil {
				links = append(links, fmt.Sprintf("- %s: [%s] %s", linkType, key, title))
			}
		}
		if err := linkRows.Err(); err != nil {
			slog.Warn("error iterating link rows", slog.String("component", "ai"), slog.Any("error", err))
		}
		if len(links) > 0 {
			contextLines = append(contextLines, "\nLinked items:")
			contextLines = append(contextLines, links...)
		}
	}

	// Load SCM links
	scmRows, err := h.db.Query(
		`SELECT title, branch_name, state FROM item_scm_links WHERE item_id = ?`, itemID)
	if err == nil {
		defer func() { _ = scmRows.Close() }()
		var scmLinks []string
		for scmRows.Next() {
			var title, branch, state string
			if err = scmRows.Scan(&title, &branch, &state); err == nil {
				scmLinks = append(scmLinks, fmt.Sprintf("- PR: %s (branch: %s, state: %s)", title, branch, state))
			}
		}
		if err := scmRows.Err(); err != nil {
			slog.Warn("error iterating SCM link rows", slog.String("component", "ai"), slog.Any("error", err))
		}
		if len(scmLinks) > 0 {
			contextLines = append(contextLines, "\nSource control:")
			contextLines = append(contextLines, scmLinks...)
		}
	}

	systemPrompt := h.promptStore.Get(llm.PromptCatchMeUp)

	userPrompt := fmt.Sprintf("Please catch me up on this work item:\n\n%s", strings.Join(contextLines, "\n"))

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

	briefing := resp.Choices[0].Message.Content
	respondJSONOK(w, CatchMeUpResponse{
		Briefing: briefing,
		ItemKey:  itemKey,
	})
}

// FindSimilarItems identifies similar items in the same workspace.
func (h *AIHandler) FindSimilarItems(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load item
	crudService := services.NewItemCRUDService(h.db)
	item, err := crudService.GetByID(itemID)
	if err != nil {
		respondNotFound(w, r, "item")
		return
	}

	// Check permission
	canView, err := h.permService.HasWorkspacePermission(user.ID, item.WorkspaceID, models.PermissionItemView)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check permissions: %w", err))
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	// Resolve LLM client
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, h.db, "find_similar", 0)
	if llmClient == nil {
		return
	}

	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	// Load candidate items: last 100 open items in same workspace (excluding current)
	candidateRows, err := h.db.Query(
		`SELECT i.id, w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
		        COALESCE(s.name, '') as status_name, COALESCE(i.description, '') as description
		 FROM items i
		 JOIN workspaces w ON i.workspace_id = w.id
		 LEFT JOIN statuses s ON i.status_id = s.id
		 LEFT JOIN status_categories sc ON s.category_id = sc.id
		 WHERE i.workspace_id = ? AND i.id != ?
		   AND COALESCE(sc.is_completed, FALSE) = FALSE
		 ORDER BY i.created_at DESC LIMIT 100`, item.WorkspaceID, itemID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to query candidate items: %w", err))
		return
	}
	defer func() { _ = candidateRows.Close() }()

	type candidateItem struct {
		ID          int
		ItemKey     string
		Title       string
		StatusName  string
		Description string
	}

	var candidates []candidateItem
	candidateMap := make(map[string]candidateItem) // key → candidate
	var candidateLines []string
	for candidateRows.Next() {
		var c candidateItem
		if err = candidateRows.Scan(&c.ID, &c.ItemKey, &c.Title, &c.StatusName, &c.Description); err == nil {
			candidates = append(candidates, c)
			candidateMap[c.ItemKey] = c
			desc := c.Description
			if len(desc) > 100 {
				desc = desc[:100] + "..."
			}
			candidateLines = append(candidateLines, fmt.Sprintf("- %s | %s | %s", c.ItemKey, c.Title, desc))
		}
	}
	if err := candidateRows.Err(); err != nil {
		respondInternalError(w, r, fmt.Errorf("error iterating candidate rows: %w", err))
		return
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
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	result, err := llm.ChatCompletionStructured[FindSimilarResponse](ctx, llmClient, llm.ChatCompletionRequest{
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
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load item
	crudService := services.NewItemCRUDService(h.db)
	item, err := crudService.GetByID(itemID)
	if err != nil {
		respondNotFound(w, r, "item")
		return
	}

	// Check permission
	canView, err := h.permService.HasWorkspacePermission(user.ID, item.WorkspaceID, models.PermissionItemView)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check permissions: %w", err))
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	// Resolve LLM client
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, h.db, "decompose", 0)
	if llmClient == nil {
		return
	}

	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	// Get available child item types
	typeRows, err := h.db.Query(
		`SELECT it.name FROM item_types it
		 JOIN workspace_hierarchy wh ON wh.child_type_id = it.id
		 WHERE wh.parent_type_id = ? AND it.workspace_id = ?`, item.ItemTypeID, item.WorkspaceID)
	var childTypeNames []string
	if err == nil {
		defer func() { _ = typeRows.Close() }()
		for typeRows.Next() {
			var name string
			if err = typeRows.Scan(&name); err == nil {
				childTypeNames = append(childTypeNames, name)
			}
		}
		if err := typeRows.Err(); err != nil {
			slog.Warn("error iterating type rows", slog.String("component", "ai"), slog.Any("error", err))
		}
	}

	// Get existing children titles
	childRows, err := h.db.Query(
		`SELECT title FROM items WHERE parent_id = ?`, itemID)
	var existingChildren []string
	if err == nil {
		defer func() { _ = childRows.Close() }()
		for childRows.Next() {
			var title string
			if err = childRows.Scan(&title); err == nil {
				existingChildren = append(existingChildren, title)
			}
		}
		if err := childRows.Err(); err != nil {
			slog.Warn("error iterating child rows", slog.String("component", "ai"), slog.Any("error", err))
		}
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
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	result, err := llm.ChatCompletionStructured[DecomposeResponse](ctx, llmClient, llm.ChatCompletionRequest{
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
