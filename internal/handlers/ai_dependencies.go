package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/repository"
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

// iterationItemInfo aliases the repository projection so the handler doesn't
// need to mint its own type for the same fields.
type iterationItemInfo = repository.IterationItemInfo

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

	iterationIDs := make([]int, len(allIterations))
	for i, it := range allIterations {
		iterationIDs[i] = it.ID
	}

	items, err := repository.NewItemRepository(h.db).ListIterationItems(iterationIDs, accessibleWSIDs)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to query iteration items: %w", err))
		return
	}

	itemByKey := make(map[string]*iterationItemInfo, len(items))
	workspaceNames := make(map[string]bool)
	for i := range items {
		itemByKey[items[i].Key] = &items[i]
		workspaceNames[items[i].WorkspaceName] = true
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
	itemIDs := make([]int, len(items))
	for i, item := range items {
		itemIDs[i] = item.ID
	}
	existingLinks := make(map[string]bool)
	linkPairs, _ := repository.NewItemLinkRepository(h.db).FindItemToItemLinksWithin(itemIDs)
	for _, p := range linkPairs {
		existingLinks[fmt.Sprintf("%d-%d", p.SourceID, p.TargetID)] = true
		existingLinks[fmt.Sprintf("%d-%d", p.TargetID, p.SourceID)] = true
	}

	// Resolve link types by name
	linkTypeRepo := repository.NewLinkTypeRepository(h.db)
	dependsOnLinkTypeID, _ := linkTypeRepo.FindActiveIDByName("Depends On")
	relatesToLinkTypeID, _ := linkTypeRepo.FindActiveIDByName("Relates To")

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
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, "dependency_analysis", parseConnectionIDParam(r))
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
	itemRepo := repository.NewItemRepository(h.db)
	created := 0
	skipped := 0

	for _, s := range req.Suggestions {
		// Verify user has edit permission on the source item's workspace
		srcWorkspaceID, err := itemRepo.GetWorkspaceID(s.SourceItemID)
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
		tgtWorkspaceID, err := itemRepo.GetWorkspaceID(s.TargetItemID)
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

// ChatContext describes where the user is in the app when they send a chat
// message. The frontend supplies it; the backend uses it only to append
// narrow, surface-specific hints to the system prompt. It is never used as
// an authorization input — workspace access is re-checked inside each tool
// from the authenticated user's accessibleWorkspaceIDs.
type ChatContext struct {
	View        string `json:"view,omitempty"`
	WorkspaceID int    `json:"workspace_id,omitempty"`
	ActionID    int    `json:"action_id,omitempty"`
}

// ChatRequest is the request body for the agentic chat endpoint.
type ChatRequest struct {
	Message      string        `json:"message"`
	ConnectionID int           `json:"connection_id,omitempty"`
	History      []ChatMessage `json:"history,omitempty"`
	Context      *ChatContext  `json:"context,omitempty"`
}

// buildChatContextHint returns the extra system-prompt text for the caller's
// current location, or "" when no surface-specific nudge applies. Kept as a
// pure function so it is trivial to unit-test.
func buildChatContextHint(ctx *ChatContext) string {
	if ctx == nil {
		return ""
	}
	if ctx.View == "workspace-actions" {
		if ctx.ActionID > 0 {
			return fmt.Sprintf(
				"\n\nThe user is currently editing action %d in workspace %d. Workflow: (1) call get_action with workspace_id=%d, action_id=%d to read the current graph; (2) call describe_action_catalog with workspace_id=%d if you need to recall node configs; (3) compose the full replacement graph and call update_action — the editor live-reloads on success. Optionally validate non-trivial changes with validate_action before the write. update_action is a full replace (not a patch), so you must include every node and edge you want to keep.",
				ctx.ActionID, ctx.WorkspaceID, ctx.WorkspaceID, ctx.ActionID, ctx.WorkspaceID,
			)
		}
		if ctx.WorkspaceID > 0 {
			return fmt.Sprintf(
				"\n\nThe user is on the action settings page for workspace %d. If they ask you to build an automation, use describe_action_catalog to discover available triggers and nodes, list_action_templates for shipped blueprints, then create_action to persist a new automation in this workspace.",
				ctx.WorkspaceID,
			)
		}
	}
	return ""
}

// ChatResponse is the response from the agentic chat endpoint.
type ChatResponse struct {
	Answer        string               `json:"answer"`
	ToolCalls     []llm.ToolCallRecord `json:"tool_calls,omitempty"`
	Iterations    int                  `json:"iterations"`
	MaxIterations int                  `json:"max_iterations"`
	StopReason    string               `json:"stop_reason"`
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
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, "ai_chat", req.ConnectionID)
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
	executor := NewToolExecutor(h.db, accessibleWSIDs, user.ID, user.FullName, h.timePermService, h.permService, services.NewCommentService(h.db), h.timerService, h.actionService)

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
	) + buildChatContextHint(req.Context)

	// Convert client history to LLM messages (only user/assistant roles allowed)
	var history []llm.Message
	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			history = append(history, llm.Message{Role: h.Role, Content: h.Content})
		}
	}

	result, err := llm.RunAgent(r.Context(), llmClient, llm.AgentConfig{
		SystemPrompt:  systemPrompt,
		Tools:         BuildLLMTools(),
		MaxTokens:     2048,
		Temperature:   0.1,
		MaxIterations: 12,
	}, req.Message, executor.Execute, history)
	if err != nil {
		slog.ErrorContext(r.Context(), "chat agent run failed",
			slog.Int("user_id", user.ID),
			slog.String("ctx_view", chatContextView(req.Context)),
			slog.String("error", err.Error()),
		)
		respondLLMError(w, r, err)
		return
	}

	slog.InfoContext(r.Context(), "chat agent run",
		slog.Int("user_id", user.ID),
		slog.String("ctx_view", chatContextView(req.Context)),
		slog.Int("ctx_workspace_id", chatContextWorkspaceID(req.Context)),
		slog.Int("ctx_action_id", chatContextActionID(req.Context)),
		slog.String("stop_reason", string(result.StopReason)),
		slog.Int("iterations", result.Iterations),
		slog.Int("max_iterations", result.MaxIter),
		slog.Int("tool_calls", len(result.ToolCalls)),
	)

	respondJSONOK(w, ChatResponse{
		Answer:        result.Answer,
		ToolCalls:     result.ToolCalls,
		Iterations:    result.Iterations,
		MaxIterations: result.MaxIter,
		StopReason:    string(result.StopReason),
	})
}

// chatContextView / WorkspaceID / ActionID safely read fields off a
// possibly-nil ChatContext for slog calls.
func chatContextView(c *ChatContext) string {
	if c == nil {
		return ""
	}
	return c.View
}
func chatContextWorkspaceID(c *ChatContext) int {
	if c == nil {
		return 0
	}
	return c.WorkspaceID
}
func chatContextActionID(c *ChatContext) int {
	if c == nil {
		return 0
	}
	return c.ActionID
}
