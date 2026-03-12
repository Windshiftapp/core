package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// AIHandler handles AI-powered endpoints.
type AIHandler struct {
	db          database.Database
	llmManager  *llm.ConnectionManager
	permService *services.PermissionService
}

// NewAIHandler creates a new AI handler.
func NewAIHandler(db database.Database, llmManager *llm.ConnectionManager, permService *services.PermissionService) *AIHandler {
	return &AIHandler{
		db:          db,
		llmManager:  llmManager,
		permService: permService,
	}
}

// PlanMyDayResponse is the response for the Plan My Day endpoint.
type PlanMyDayResponse struct {
	Activities   []PlannedActivity `json:"activities"`
	Summary      string            `json:"summary"`
	SystemPrompt string            `json:"system_prompt,omitempty"`
	Prompt       string            `json:"prompt,omitempty"`
}

// PlannedActivity represents a single planned activity in the day schedule.
type PlannedActivity struct {
	Time            string `json:"time"`
	DurationMinutes int    `json:"duration_minutes"`
	ItemKey         string `json:"item_key"`
	ItemID          int    `json:"item_id"`
	WorkspaceID     int    `json:"workspace_id"`
	Title           string `json:"title"`
	Reason          string `json:"reason"`
}

// PlanMyDay generates a prioritized daily plan based on the user's assigned items.
func (h *AIHandler) PlanMyDay(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get accessible workspace IDs for this user
	accessibleWorkspaceIDs, err := GetAccessibleWorkspaceIDs(user, h.db, h.permService)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get accessible workspaces: %w", err))
		return
	}
	if len(accessibleWorkspaceIDs) == 0 {
		respondJSONOK(w, PlanMyDayResponse{
			Activities: []PlannedActivity{},
			Summary:    "No accessible workspaces found.",
		})
		return
	}

	// Find user's personal workspace IDs so we include all items from them
	var personalWSIDs []int
	pwsRows, err := h.db.Query("SELECT id FROM workspaces WHERE is_personal = true AND owner_id = ? AND active = true", user.ID)
	if err == nil {
		defer func() { _ = pwsRows.Close() }()
		for pwsRows.Next() {
			var id int
			if err = pwsRows.Scan(&id); err == nil {
				personalWSIDs = append(personalWSIDs, id)
			}
		}
	}

	// Build filter: include items assigned to user OR items in their personal workspace(s)
	statusFilter := "NOT EXISTS (SELECT 1 FROM status_categories sc WHERE sc.id = st.category_id AND sc.is_completed = 1) OR i.status_id IS NULL"
	qlArgs := []interface{}{user.ID}
	ownershipFilter := "i.assignee_id = ?"

	if len(personalWSIDs) > 0 {
		placeholders := make([]string, len(personalWSIDs))
		for i, id := range personalWSIDs {
			placeholders[i] = "?"
			qlArgs = append(qlArgs, id)
		}
		ownershipFilter = fmt.Sprintf("i.assignee_id = ? OR i.workspace_id IN (%s)", strings.Join(placeholders, ","))
	}

	qlQuery := fmt.Sprintf("(%s) AND (%s)", statusFilter, ownershipFilter)

	// Query user's open items (assigned to them or in their personal workspace)
	crudService := services.NewItemCRUDService(h.db)
	items, _, err := crudService.List(services.ItemListParams{
		WorkspaceIDs: accessibleWorkspaceIDs,
		Filters: services.ItemFilters{
			QLQuery: qlQuery,
			QLArgs:  qlArgs,
		},
		SortBy:  "due_date",
		SortAsc: true,
		Pagination: services.PaginationParams{
			Limit: 50,
		},
	})
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to list items: %w", err))
		return
	}

	if len(items) == 0 {
		respondJSONOK(w, PlanMyDayResponse{
			Activities: []PlannedActivity{},
			Summary:    "No open items assigned to you.",
		})
		return
	}

	// Build the item context for the prompt
	var itemLines []string
	for _, item := range items {
		key := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
		line := fmt.Sprintf("- [%s] %s", key, item.Title)
		if item.PriorityName != "" {
			line += fmt.Sprintf(" | Priority: %s", item.PriorityName)
		}
		if item.DueDate != nil {
			line += fmt.Sprintf(" | Due: %s", item.DueDate.Format("2006-01-02"))
		}
		if item.StatusName != "" {
			line += fmt.Sprintf(" | Status: %s", item.StatusName)
		}
		desc := item.Description
		if len(desc) > 120 {
			desc = desc[:120] + "..."
		}
		if desc != "" {
			line += fmt.Sprintf(" | Desc: %s", desc)
		}
		itemLines = append(itemLines, line)
	}

	// Determine user timezone
	timezone := user.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	now := time.Now()
	var loc *time.Location
	if loc, err = time.LoadLocation(timezone); err == nil {
		now = now.In(loc)
	}

	systemPrompt := `You are a work planning assistant. Given a list of work items assigned to a user, suggest a prioritized schedule for today. Consider due dates, priorities, and logical task ordering.

Return a JSON object with:
- activities: array of objects with time (HH:MM format), duration_minutes, item_key (exact key from provided list), title, and reason
- summary: a short overview of the planned day

Schedule tasks across the full workday, not all at the same time. Use only item keys from the provided list.`

	userPrompt := fmt.Sprintf("Today is %s (%s timezone). Here are my open work items:\n\n%s\n\nPlease plan my day.",
		now.Format("Monday, January 2, 2006"), timezone, strings.Join(itemLines, "\n"))

	// Preview mode: return prompts without calling the LLM
	if r.URL.Query().Get("preview") == "true" {
		respondJSONOK(w, PlanMyDayResponse{
			Activities:   []PlannedActivity{},
			SystemPrompt: systemPrompt,
			Prompt:       userPrompt,
		})
		return
	}

	// Resolve LLM client (optionally from connection_id query param)
	var connectionID int
	if cidStr := r.URL.Query().Get("connection_id"); cidStr != "" {
		fmt.Sscan(cidStr, &connectionID) //nolint:errcheck,gosec // connection ID parsing is best-effort
	}

	llmClient, err := h.llmManager.Resolve(connectionID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to resolve LLM connection: %w", err))
		return
	}

	if !llmClient.Available() {
		respondServiceUnavailable(w, r, "AI features are not available. LLM service is not configured.")
		return
	}

	// Call the LLM with structured output
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	plan, err := llm.ChatCompletionStructured[PlanMyDayResponse](ctx, llmClient, llm.ChatCompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.7,
		StructuredOutput: &llm.StructuredOutputConfig{
			Schema:     llm.SchemaPlanMyDay,
			SchemaName: "plan_my_day",
			Strict:     true,
		},
	})
	if err != nil {
		slog.Error("LLM chat completion failed", slog.Any("error", err))
		respondServiceUnavailable(w, r, "AI service is temporarily unavailable. Please try again later.")
		return
	}

	// Enrich activities with item IDs and workspace IDs from our data
	itemKeyToID := make(map[string]int)
	itemKeyToWSID := make(map[string]int)
	for _, item := range items {
		key := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
		itemKeyToID[key] = item.ID
		itemKeyToWSID[key] = item.WorkspaceID
	}
	for i := range plan.Activities {
		if id, ok := itemKeyToID[plan.Activities[i].ItemKey]; ok {
			plan.Activities[i].ItemID = id
		}
		if wsID, ok := itemKeyToWSID[plan.Activities[i].ItemKey]; ok {
			plan.Activities[i].WorkspaceID = wsID
		}
	}

	plan.SystemPrompt = systemPrompt
	plan.Prompt = userPrompt
	respondJSONOK(w, *plan)
}

// Status checks whether AI features are available by resolving the LLM client
// through the same path used by actual AI handlers (including LLM_ENDPOINT fallback).
func (h *AIHandler) Status(w http.ResponseWriter, r *http.Request) {
	client, err := h.llmManager.Resolve(0)
	available := err == nil && client != nil && client.Available()
	respondJSONOK(w, map[string]bool{"available": available})
}

// --- Item AI Actions ---

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
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
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
	llmClient, err := h.llmManager.Resolve(0)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to resolve LLM connection: %w", err))
		return
	}
	if !llmClient.Available() {
		respondServiceUnavailable(w, r, "AI features are not available. LLM service is not configured.")
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
		if len(scmLinks) > 0 {
			contextLines = append(contextLines, "\nSource control:")
			contextLines = append(contextLines, scmLinks...)
		}
	}

	systemPrompt := `You are a project management assistant. Given context about a work item (description, comments, history, links, source control activity), provide a concise briefing that catches someone up on the current state.

Write in markdown format. Structure the briefing with:
1. A one-sentence summary of what this item is about
2. Current status and recent activity
3. Key decisions or discussions from comments
4. Any blockers or dependencies from linked items
5. Source control progress if applicable

Be concise and factual. Focus on what someone needs to know to understand the current state.`

	userPrompt := fmt.Sprintf("Please catch me up on this work item:\n\n%s", strings.Join(contextLines, "\n"))

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
		slog.Error("LLM chat completion failed", slog.Any("error", err))
		respondServiceUnavailable(w, r, "AI service is temporarily unavailable. Please try again later.")
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
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
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
	llmClient, err := h.llmManager.Resolve(0)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to resolve LLM connection: %w", err))
		return
	}
	if !llmClient.Available() {
		respondServiceUnavailable(w, r, "AI features are not available. LLM service is not configured.")
		return
	}

	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	// Load candidate items: last 100 open items in same workspace (excluding current)
	candidateRows, err := h.db.Query(
		`SELECT i.id, CONCAT(w.key, '-', i.workspace_item_number) as item_key, i.title,
		        COALESCE(s.name, '') as status_name, COALESCE(i.description, '') as description
		 FROM items i
		 JOIN workspaces w ON i.workspace_id = w.id
		 LEFT JOIN statuses s ON i.status_id = s.id
		 LEFT JOIN status_categories sc ON s.category_id = sc.id
		 WHERE i.workspace_id = ? AND i.id != ?
		   AND (sc.is_completed IS NULL OR sc.is_completed = 0)
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

	systemPrompt := `You are a project management assistant. Given a work item and a list of other items in the same workspace, identify items that are similar, potentially duplicates, or closely related.

Return a JSON object with:
- similar_items: array with item_key (exact key from candidate list), similarity (duplicate/closely_related/somewhat_related), and reason
- summary: a one-sentence summary of findings

Only include genuinely similar items. If none are similar, return an empty array. Maximum 10 results.`

	userPrompt := fmt.Sprintf(`Current item %s: %s
Description: %s

Candidate items in the same workspace:
%s

Find similar items.`, itemKey, item.Title, currentDesc, strings.Join(candidateLines, "\n"))

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
		slog.Error("LLM chat completion failed", slog.Any("error", err))
		respondServiceUnavailable(w, r, "AI service is temporarily unavailable. Please try again later.")
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
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
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
	llmClient, err := h.llmManager.Resolve(0)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to resolve LLM connection: %w", err))
		return
	}
	if !llmClient.Available() {
		respondServiceUnavailable(w, r, "AI features are not available. LLM service is not configured.")
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

	systemPrompt := `You are a project management assistant. Given a work item, suggest how to break it down into smaller sub-tasks.

Return a JSON object with:
- sub_tasks: array of objects, each with "title" (string, imperative form) and "description" (string, 1-2 sentences)
- reasoning: brief explanation of the decomposition approach

Suggest 3-8 meaningful, actionable sub-tasks. Don't create trivially small tasks. If existing children are listed, don't duplicate them.`

	userPrompt := fmt.Sprintf("Break this work item into sub-tasks:\n\n%s", strings.Join(contextParts, "\n"))

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
		slog.Error("LLM chat completion failed", slog.Any("error", err))
		respondServiceUnavailable(w, r, "AI service is temporarily unavailable. Please try again later.")
		return
	}

	respondJSONOK(w, *result)
}

// GenerateReleaseNotesResponse is the structured LLM response for release notes generation.
type GenerateReleaseNotesResponse struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Notes   string `json:"notes"`
}

// GenerateReleaseNotes generates release notes for a milestone using the LLM.
func (h *AIHandler) GenerateReleaseNotes(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
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
	var connectionID int
	if cidStr := r.URL.Query().Get("connection_id"); cidStr != "" {
		fmt.Sscan(cidStr, &connectionID) //nolint:errcheck,gosec // connection ID parsing is best-effort
	}

	llmClient, err := h.llmManager.Resolve(connectionID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to resolve LLM connection: %w", err))
		return
	}
	if !llmClient.Available() {
		respondServiceUnavailable(w, r, "AI features are not available. LLM service is not configured.")
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

	systemPrompt := `You are a software release manager. Given information about a project milestone, write professional release notes in markdown format.

Include an introductory paragraph summarizing the release, then use ## section headers (e.g. "## What's New", "## Bug Fixes", "## Improvements") with bullet points under each. Write in a professional tone with enough detail that users understand the impact of each change.

Return ONLY the markdown text — no JSON, no code block fences, no preamble.`

	userPrompt := fmt.Sprintf("Generate release notes for this milestone:\n\n%s", strings.Join(contextLines, "\n"))

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
		slog.Error("LLM chat completion failed", slog.Any("error", err))
		respondServiceUnavailable(w, r, "AI service is temporarily unavailable. Please try again later.")
		return
	}
	if len(resp.Choices) == 0 {
		respondServiceUnavailable(w, r, "AI service is temporarily unavailable. Please try again later.")
		return
	}

	notes := strings.TrimSpace(resp.Choices[0].Message.Content)
	respondJSONOK(w, GenerateReleaseNotesResponse{Notes: notes})
}

// --- Dependency Analysis ---

// AnalyzeDependenciesRequest is the optional request body for dependency analysis.
type AnalyzeDependenciesRequest struct {
	CompareIterationIDs []int `json:"compare_iteration_ids,omitempty"`
}

// DependencySuggestion represents a suggested dependency link between two items.
type DependencySuggestion struct {
	SourceItemID      int    `json:"source_item_id"`
	SourceItemKey     string `json:"source_item_key"`
	SourceItemTitle   string `json:"source_item_title"`
	SourceWSID        int    `json:"source_workspace_id"`
	SourceIterationID int    `json:"source_iteration_id"`
	TargetItemID      int    `json:"target_item_id"`
	TargetItemKey     string `json:"target_item_key"`
	TargetItemTitle   string `json:"target_item_title"`
	TargetWSID        int    `json:"target_workspace_id"`
	TargetIterationID int    `json:"target_iteration_id"`
	Relationship      string `json:"relationship"`
	Reason            string `json:"reason"`
	LinkTypeID        int    `json:"link_type_id"`
	LinkTypeName      string `json:"link_type_name"`
	CrossIteration    bool   `json:"cross_iteration"`
}

// AnalyzeDependenciesResponse is the response for the dependency analysis endpoint.
type AnalyzeDependenciesResponse struct {
	IterationID           int                    `json:"iteration_id"`
	IterationName         string                 `json:"iteration_name"`
	Suggestions           []DependencySuggestion `json:"suggestions"`
	ItemsAnalyzed         int                    `json:"items_analyzed"`
	WorkspacesIncluded    []string               `json:"workspaces_included"`
	IterationsIncluded    []string               `json:"iterations_included"`
	ExistingLinksFiltered int                    `json:"existing_links_filtered"`
	SystemPrompt          string                 `json:"system_prompt,omitempty"`
	Prompt                string                 `json:"prompt,omitempty"`
}

// AcceptDependenciesRequest contains the suggestions to accept.
type AcceptDependenciesRequest struct {
	Suggestions []AcceptSuggestion `json:"suggestions"`
}

// AcceptSuggestion is a single suggestion to accept.
type AcceptSuggestion struct {
	SourceItemID int `json:"source_item_id"`
	TargetItemID int `json:"target_item_id"`
	LinkTypeID   int `json:"link_type_id"`
}

// AcceptDependenciesResponse is the response for accepting dependency suggestions.
type AcceptDependenciesResponse struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

// llmDependencyResult matches the structured JSON output from the LLM.
type llmDependencyResult struct {
	Dependencies []struct {
		SourceKey    string `json:"source_key"`
		TargetKey    string `json:"target_key"`
		Relationship string `json:"relationship"`
		Reason       string `json:"reason"`
	} `json:"dependencies"`
}

// iterationItemInfo holds item data collected for the dependency analysis prompt.
type iterationItemInfo struct {
	ID            int
	Key           string
	Title         string
	Description   string
	StatusName    string
	PriorityName  string
	ItemTypeName  string
	AssigneeName  string
	WorkspaceID   int
	WorkspaceKey  string
	WorkspaceName string
	IterationID   int
}

// AnalyzeDependencies analyzes items in an iteration and suggests dependency links.
func (h *AIHandler) AnalyzeDependencies(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	iterationID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Parse optional request body
	var req AnalyzeDependenciesRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondBadRequest(w, r, "Invalid request body")
			return
		}
	}

	// Cap compare iterations at 4 (+ primary = 5 total)
	if len(req.CompareIterationIDs) > 4 {
		respondBadRequest(w, r, "Maximum 4 compare iteration IDs allowed")
		return
	}

	// Load primary iteration
	planningService := services.NewPlanningService(h.db)
	iteration, err := planningService.GetIteration(iterationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "iteration")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to load iteration: %w", err))
		return
	}

	// Check permission on primary iteration
	accessibleWSIDs, err := GetAccessibleWorkspaceIDs(user, h.db, h.permService)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get accessible workspaces: %w", err))
		return
	}
	if len(accessibleWSIDs) == 0 {
		respondForbidden(w, r)
		return
	}

	if !iteration.IsGlobal && iteration.WorkspaceID != nil {
		hasAccess := false
		for _, wsID := range accessibleWSIDs {
			if wsID == *iteration.WorkspaceID {
				hasAccess = true
				break
			}
		}
		if !hasAccess {
			respondNotFound(w, r, "iteration")
			return
		}
	}

	// Collect all iteration IDs and metadata
	type iterationMeta struct {
		ID        int
		Name      string
		StartDate string
		EndDate   string
		IsPrimary bool
	}
	allIterations := []iterationMeta{{
		ID: iteration.ID, Name: iteration.Name,
		StartDate: iteration.StartDate, EndDate: iteration.EndDate,
		IsPrimary: true,
	}}

	for _, cid := range req.CompareIterationIDs {
		if cid == iterationID {
			continue
		}
		cIter, cErr := planningService.GetIteration(cid)
		if cErr != nil {
			continue // skip silently
		}
		// Check permission on compared iteration
		if !cIter.IsGlobal && cIter.WorkspaceID != nil {
			hasAccess := false
			for _, wsID := range accessibleWSIDs {
				if wsID == *cIter.WorkspaceID {
					hasAccess = true
					break
				}
			}
			if !hasAccess {
				continue
			}
		}
		allIterations = append(allIterations, iterationMeta{
			ID: cIter.ID, Name: cIter.Name,
			StartDate: cIter.StartDate, EndDate: cIter.EndDate,
			IsPrimary: false,
		})
	}

	// Build workspace ID placeholders for SQL
	wsPlaceholders := make([]string, len(accessibleWSIDs))
	wsArgs := make([]interface{}, len(accessibleWSIDs))
	for i, id := range accessibleWSIDs {
		wsPlaceholders[i] = "?"
		wsArgs[i] = id
	}

	// Build iteration ID placeholders
	iterIDs := make([]interface{}, len(allIterations))
	iterPlaceholders := make([]string, len(allIterations))
	for i, it := range allIterations {
		iterIDs[i] = it.ID
		iterPlaceholders[i] = "?"
	}

	// Load items across all iterations and accessible workspaces
	query := fmt.Sprintf(`
		SELECT i.id, CONCAT(w.key, '-', i.workspace_item_number) as item_key,
		       i.title, COALESCE(i.description, '') as description,
		       COALESCE(s.name, '') as status_name,
		       COALESCE(p.name, '') as priority_name,
		       COALESCE(it.name, '') as item_type_name,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
		       i.workspace_id, w.key as workspace_key, w.name as workspace_name,
		       i.iteration_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.iteration_id IN (%s)
		  AND i.workspace_id IN (%s)
		ORDER BY i.iteration_id, i.workspace_id, i.workspace_item_number
		LIMIT 100`,
		strings.Join(iterPlaceholders, ","),
		strings.Join(wsPlaceholders, ","))

	queryArgs := make([]interface{}, 0, len(iterIDs)+len(wsArgs))
	queryArgs = append(queryArgs, iterIDs...)
	queryArgs = append(queryArgs, wsArgs...)
	rows, err := h.db.Query(query, queryArgs...)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to query iteration items: %w", err))
		return
	}
	defer func() { _ = rows.Close() }()

	var items []iterationItemInfo
	itemByKey := make(map[string]*iterationItemInfo)
	workspaceNames := make(map[string]bool)
	for rows.Next() {
		var item iterationItemInfo
		if err := rows.Scan(&item.ID, &item.Key, &item.Title, &item.Description,
			&item.StatusName, &item.PriorityName, &item.ItemTypeName, &item.AssigneeName,
			&item.WorkspaceID, &item.WorkspaceKey, &item.WorkspaceName,
			&item.IterationID); err != nil {
			continue
		}
		items = append(items, item)
		itemByKey[item.Key] = &items[len(items)-1]
		workspaceNames[item.WorkspaceName] = true
	}

	if len(items) == 0 {
		respondJSONOK(w, AnalyzeDependenciesResponse{
			IterationID:   iterationID,
			IterationName: iteration.Name,
			Suggestions:   []DependencySuggestion{},
			ItemsAnalyzed: 0,
		})
		return
	}

	// Load existing links between items in this set
	itemIDs := make([]interface{}, len(items))
	itemIDPlaceholders := make([]string, len(items))
	for i, item := range items {
		itemIDs[i] = item.ID
		itemIDPlaceholders[i] = "?"
	}
	existingLinks := make(map[string]bool)
	linkQuery := fmt.Sprintf(`
		SELECT source_id, target_id FROM item_links
		WHERE source_type = 'item' AND target_type = 'item'
		  AND source_id IN (%s) AND target_id IN (%s)`,
		strings.Join(itemIDPlaceholders, ","),
		strings.Join(itemIDPlaceholders, ","))
	linkArgs := make([]interface{}, 0, len(itemIDs)*2)
	linkArgs = append(linkArgs, itemIDs...)
	linkArgs = append(linkArgs, itemIDs...)
	linkRows, err := h.db.Query(linkQuery, linkArgs...)
	if err == nil {
		defer func() { _ = linkRows.Close() }()
		for linkRows.Next() {
			var srcID, tgtID int
			if err := linkRows.Scan(&srcID, &tgtID); err == nil {
				existingLinks[fmt.Sprintf("%d-%d", srcID, tgtID)] = true
				existingLinks[fmt.Sprintf("%d-%d", tgtID, srcID)] = true
			}
		}
	}

	// Resolve link types by name
	var dependsOnLinkTypeID, relatesToLinkTypeID int
	_ = h.db.QueryRow("SELECT id FROM link_types WHERE name = 'Depends On' AND active = true").Scan(&dependsOnLinkTypeID)
	_ = h.db.QueryRow("SELECT id FROM link_types WHERE name = 'Relates To' AND active = true").Scan(&relatesToLinkTypeID)

	// Build prompt grouped by iteration then workspace
	iterationNameMap := make(map[int]string)
	var promptSections []string
	for idx, iterMeta := range allIterations {
		iterationNameMap[iterMeta.ID] = iterMeta.Name
		label := "current sprint"
		if !iterMeta.IsPrimary {
			label = "compared sprint"
		}
		header := fmt.Sprintf("# %s (%s to %s) — %s", iterMeta.Name, iterMeta.StartDate, iterMeta.EndDate, label)

		// Group items by workspace for this iteration
		type wsGroup struct {
			name  string
			key   string
			lines []string
		}
		wsGroups := make(map[int]*wsGroup)
		var wsOrder []int
		for i := range items {
			item := &items[i]
			if item.IterationID != iterMeta.ID {
				continue
			}
			g, exists := wsGroups[item.WorkspaceID]
			if !exists {
				g = &wsGroup{name: item.WorkspaceName, key: item.WorkspaceKey}
				wsGroups[item.WorkspaceID] = g
				wsOrder = append(wsOrder, item.WorkspaceID)
			}
			desc := item.Description
			if len(desc) > 80 {
				desc = desc[:80] + "..."
			}
			line := fmt.Sprintf("- %s | %s | %s | %s | %s | %s",
				item.Key, item.Title, desc, item.StatusName, item.ItemTypeName, item.AssigneeName)
			g.lines = append(g.lines, line)
		}

		if len(wsGroups) > 0 {
			section := header
			for _, wsID := range wsOrder {
				g := wsGroups[wsID]
				section += fmt.Sprintf("\n## Team: %s (%s)\n%s", g.name, g.key, strings.Join(g.lines, "\n"))
			}
			promptSections = append(promptSections, section)
		}
		_ = idx
	}

	systemPrompt := `You are a project management dependency analyst. Given work items organized by team and sprint, identify dependencies between them.

A dependency exists when:
- One item must complete before another can start
- Two items modify the same system/component and need coordination
- One item produces output another consumes
- Items share infrastructure, API, or data requirements

When items span multiple sprints, pay special attention to schedule risks:
- A current sprint item depending on a future sprint item is a BLOCKER (cannot complete on time)
- A future sprint item depending on a current sprint item is normal sequencing

Focus on cross-team and cross-sprint dependencies first, then within-team. Only suggest genuine dependencies, not superficial similarities. Maximum 20 suggestions.

Return a JSON object with:
- dependencies: array of objects with source_key (item key like "PROJ-123"), target_key, relationship (one of: "depends_on", "blocks", "relates_to"), and reason`

	userPrompt := strings.Join(promptSections, "\n\n") + "\n\nIdentify dependencies between these items."

	// Preview mode
	if r.URL.Query().Get("preview") == "true" {
		wsNameList := make([]string, 0, len(workspaceNames))
		for name := range workspaceNames {
			wsNameList = append(wsNameList, name)
		}
		iterNameList := make([]string, 0, len(allIterations))
		for _, it := range allIterations {
			iterNameList = append(iterNameList, it.Name)
		}
		respondJSONOK(w, AnalyzeDependenciesResponse{
			IterationID:        iterationID,
			IterationName:      iteration.Name,
			Suggestions:        []DependencySuggestion{},
			ItemsAnalyzed:      len(items),
			WorkspacesIncluded: wsNameList,
			IterationsIncluded: iterNameList,
			SystemPrompt:       systemPrompt,
			Prompt:             userPrompt,
		})
		return
	}

	// Resolve LLM client
	var connectionID int
	if cidStr := r.URL.Query().Get("connection_id"); cidStr != "" {
		fmt.Sscan(cidStr, &connectionID) //nolint:errcheck,gosec // best-effort parse, zero-value fallback is fine
	}

	llmClient, err := h.llmManager.Resolve(connectionID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to resolve LLM connection: %w", err))
		return
	}
	if !llmClient.Available() {
		respondServiceUnavailable(w, r, "AI features are not available. LLM service is not configured.")
		return
	}

	// Call LLM
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	result, err := llm.ChatCompletionStructured[llmDependencyResult](ctx, llmClient, llm.ChatCompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		StructuredOutput: &llm.StructuredOutputConfig{
			Schema:     llm.SchemaAnalyzeDependencies,
			SchemaName: "analyze_dependencies",
			Strict:     true,
		},
	})
	if err != nil {
		slog.Error("LLM chat completion failed", slog.Any("error", err))
		respondServiceUnavailable(w, r, "AI service is temporarily unavailable. Please try again later.")
		return
	}

	// Enrich LLM results with DB data
	existingFiltered := 0
	var suggestions []DependencySuggestion
	for _, dep := range result.Dependencies {
		srcKey := strings.TrimPrefix(strings.TrimSuffix(dep.SourceKey, "]"), "[")
		tgtKey := strings.TrimPrefix(strings.TrimSuffix(dep.TargetKey, "]"), "[")

		srcItem, srcOK := itemByKey[srcKey]
		tgtItem, tgtOK := itemByKey[tgtKey]
		if !srcOK || !tgtOK {
			continue // hallucinated key
		}
		if srcItem.ID == tgtItem.ID {
			continue // self-link
		}

		// Determine link type and direction based on relationship
		linkTypeID := relatesToLinkTypeID
		linkTypeName := "Relates To"
		finalSrcItem := srcItem
		finalTgtItem := tgtItem

		switch dep.Relationship {
		case "depends_on":
			linkTypeID = dependsOnLinkTypeID
			linkTypeName = "Depends On"
			// source = dependent, target = prerequisite (as-is from LLM)
		case "blocks":
			linkTypeID = dependsOnLinkTypeID
			linkTypeName = "Depends On"
			// LLM says "source blocks target" → swap: target depends on source
			finalSrcItem = tgtItem
			finalTgtItem = srcItem
		case "relates_to":
			// defaults already set
		}

		if linkTypeID == 0 {
			continue // link type not found in DB
		}

		// Check for existing link
		linkKey := fmt.Sprintf("%d-%d", finalSrcItem.ID, finalTgtItem.ID)
		if existingLinks[linkKey] {
			existingFiltered++
			continue
		}

		suggestions = append(suggestions, DependencySuggestion{
			SourceItemID:      finalSrcItem.ID,
			SourceItemKey:     finalSrcItem.Key,
			SourceItemTitle:   finalSrcItem.Title,
			SourceWSID:        finalSrcItem.WorkspaceID,
			SourceIterationID: finalSrcItem.IterationID,
			TargetItemID:      finalTgtItem.ID,
			TargetItemKey:     finalTgtItem.Key,
			TargetItemTitle:   finalTgtItem.Title,
			TargetWSID:        finalTgtItem.WorkspaceID,
			TargetIterationID: finalTgtItem.IterationID,
			Relationship:      dep.Relationship,
			Reason:            dep.Reason,
			LinkTypeID:        linkTypeID,
			LinkTypeName:      linkTypeName,
			CrossIteration:    finalSrcItem.IterationID != finalTgtItem.IterationID,
		})

		if len(suggestions) >= 20 {
			break
		}
	}

	wsNameList := make([]string, 0, len(workspaceNames))
	for name := range workspaceNames {
		wsNameList = append(wsNameList, name)
	}
	iterNameList := make([]string, 0, len(allIterations))
	for _, it := range allIterations {
		iterNameList = append(iterNameList, it.Name)
	}

	respondJSONOK(w, AnalyzeDependenciesResponse{
		IterationID:           iterationID,
		IterationName:         iteration.Name,
		Suggestions:           suggestions,
		ItemsAnalyzed:         len(items),
		WorkspacesIncluded:    wsNameList,
		IterationsIncluded:    iterNameList,
		ExistingLinksFiltered: existingFiltered,
		SystemPrompt:          systemPrompt,
		Prompt:                userPrompt,
	})
}

// AcceptDependencies creates item links from accepted dependency suggestions.
func (h *AIHandler) AcceptDependencies(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	_, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var req AcceptDependenciesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}
	if len(req.Suggestions) == 0 {
		respondJSONOK(w, AcceptDependenciesResponse{Created: 0, Skipped: 0})
		return
	}

	linkService := services.NewItemLinkService(h.db)
	created := 0
	skipped := 0

	for _, s := range req.Suggestions {
		// Verify user has edit permission on the source item's workspace
		var srcWorkspaceID int
		err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", s.SourceItemID).Scan(&srcWorkspaceID)
		if err != nil {
			skipped++
			continue
		}
		canEdit, err := h.permService.HasWorkspacePermission(user.ID, srcWorkspaceID, models.PermissionItemEdit)
		if err != nil || !canEdit {
			skipped++
			continue
		}

		// Verify user has view permission on target item's workspace
		var tgtWorkspaceID int
		err = h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", s.TargetItemID).Scan(&tgtWorkspaceID)
		if err != nil {
			skipped++
			continue
		}
		canView, err := h.permService.HasWorkspacePermission(user.ID, tgtWorkspaceID, models.PermissionItemView)
		if err != nil || !canView {
			skipped++
			continue
		}

		linkID, err := linkService.CreateLink(services.CreateItemLinkParams{
			LinkTypeID: s.LinkTypeID,
			SourceType: "item",
			SourceID:   s.SourceItemID,
			TargetType: "item",
			TargetID:   s.TargetItemID,
			CreatedBy:  &user.ID,
		})
		if err != nil {
			skipped++
			continue
		}
		if linkID == 0 {
			skipped++ // duplicate
		} else {
			created++
		}
	}

	respondJSONOK(w, AcceptDependenciesResponse{Created: created, Skipped: skipped})
}
