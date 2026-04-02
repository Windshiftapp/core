package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/services"
)

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
	user, ok := RequireAuth(w, r)
	if !ok {
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

	systemPrompt := h.promptStore.Get(llm.PromptDependencyAnalysis)

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
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, h.db, "dependency_analysis", parseConnectionIDParam(r))
	if llmClient == nil {
		return
	}

	// Call LLM
	extendWriteDeadline(w)
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
		respondLLMError(w, r, err)
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
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if _, ok := requireIDParam(w, r, "id"); !ok {
		return
	}

	req, ok := decodeJSON[AcceptDependenciesRequest](w, r)
	if !ok {
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

// ChatMessage represents a single message in conversation history.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the request body for the agentic chat endpoint.
type ChatRequest struct {
	Message      string        `json:"message"`
	ConnectionID int           `json:"connection_id,omitempty"`
	History      []ChatMessage `json:"history,omitempty"`
}

// ChatResponse is the response from the agentic chat endpoint.
type ChatResponse struct {
	Answer     string               `json:"answer"`
	ToolCalls  []llm.ToolCallRecord `json:"tool_calls,omitempty"`
	Iterations int                  `json:"iterations"`
}

// Chat handles agentic chat where the LLM can query workspaces and items via tool calls.
func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[ChatRequest](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		respondBadRequest(w, r, "message is required")
		return
	}

	// Resolve LLM client (Chat allows user to override connection via the UI selector)
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, h.db, "ai_chat", req.ConnectionID)
	if llmClient == nil {
		return
	}

	// Pre-compute accessible workspace IDs (immutable for the duration of this request)
	accessibleWSIDs, err := GetAccessibleWorkspaceIDs(user, h.db, h.permService)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get accessible workspaces: %w", err))
		return
	}

	// Build tool executor
	executor := NewToolExecutor(h.db, accessibleWSIDs, user.ID, h.timePermService, h.permService)

	// Determine current date in user's timezone
	chatTimezone := user.Timezone
	if chatTimezone == "" {
		chatTimezone = "UTC"
	}
	chatNow := time.Now()
	if chatLoc, locErr := time.LoadLocation(chatTimezone); locErr == nil {
		chatNow = chatNow.In(chatLoc)
	}

	systemPrompt := fmt.Sprintf(h.promptStore.Get(llm.PromptAIChat),
		chatNow.Format("2006-01-02"), user.FullName, user.ID, user.ID,
	)

	// Convert client history to LLM messages (only user/assistant roles allowed)
	var history []llm.Message
	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			history = append(history, llm.Message{Role: h.Role, Content: h.Content})
		}
	}

	result, err := llm.RunAgent(r.Context(), llmClient, llm.AgentConfig{
		SystemPrompt:  systemPrompt,
		Tools:         llm.BuiltinTools(),
		MaxTokens:     2048,
		Temperature:   0.1,
		MaxIterations: 6,
	}, req.Message, executor.Execute, history)
	if err != nil {
		respondLLMError(w, r, err)
		return
	}

	respondJSONOK(w, ChatResponse{
		Answer:     result.Answer,
		ToolCalls:  result.ToolCalls,
		Iterations: result.Iterations,
	})
}
