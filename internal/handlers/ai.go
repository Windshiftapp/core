package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
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

func (h *AIHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
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
	user := h.getUserFromContext(r)
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
	pwsRows, err := h.db.Query("SELECT id FROM workspaces WHERE is_personal = 1 AND owner_id = ? AND active = 1", user.ID)
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
		fmt.Sscan(cidStr, &connectionID) //nolint:errcheck // connection ID parsing is best-effort
	}

	llmClient, err := h.llmManager.ResolveForFeature("plan_my_day", connectionID)
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
	client, err := h.llmManager.ResolveForFeature("item_analysis", 0)
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
	user := h.getUserFromContext(r)
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
	llmClient, err := h.llmManager.ResolveForFeature("item_analysis", 0)
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
		`SELECT c.content, u.name, c.created_at FROM comments c
		 LEFT JOIN users u ON c.user_id = u.id
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
	user := h.getUserFromContext(r)
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
	llmClient, err := h.llmManager.ResolveForFeature("item_analysis", 0)
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
			candidateLines = append(candidateLines, fmt.Sprintf("- [%s] %s | %s", c.ItemKey, c.Title, desc))
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

	userPrompt := fmt.Sprintf(`Current item [%s]: %s
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
		if candidate, ok := candidateMap[si.ItemKey]; ok {
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
	user := h.getUserFromContext(r)
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
	llmClient, err := h.llmManager.ResolveForFeature("item_analysis", 0)
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
