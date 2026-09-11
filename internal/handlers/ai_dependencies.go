package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/aitooladapter"
	"windshift/internal/aitools"
	"windshift/internal/auth"
	"windshift/internal/itemevents"
	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
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
		if err := newJSONDecoder(w, r).Decode(&req); err != nil {
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
		if errors.Is(err, repository.ErrNotFound) {
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
	ctx, cancel := context.WithTimeout(r.Context(), llm.DefaultRequestTimeout)
	defer cancel()

	result, err := llm.CompleteStructured[llmDependencyResult](ctx, llmClient, llm.CompletionRequest{
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

		linkID, err := linkService.CreateLinkWithMetadata(services.CreateItemLinkParams{
			LinkTypeID: s.LinkTypeID,
			SourceType: "item",
			SourceID:   s.SourceItemID,
			TargetType: "item",
			TargetID:   s.TargetItemID,
			CreatedBy:  &user.ID,
		}, itemevents.User(user.ID, "agent"))
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
	PageID      int    `json:"page_id,omitempty"`
	ItemID      int    `json:"item_id,omitempty"`
	ItemKey     string `json:"item_key,omitempty"`
}

// ChatRequest is the request body for the agentic chat endpoint.
type ChatRequest struct {
	Message      string `json:"message"`
	ConnectionID int    `json:"connection_id,omitempty"`
	SessionID    int    `json:"session_id,omitempty"`
	// HistoryAfterMessageID starts a fresh client conversation within the
	// durable General session without deleting its transcript.
	HistoryAfterMessageID int `json:"history_after_message_id,omitempty"`
	// History is accepted for one compatibility release but ignored. The
	// server-owned session transcript is authoritative.
	History []ChatMessage `json:"history,omitempty"`
	Context *ChatContext  `json:"context,omitempty"`
}

// buildChatContextHint returns the extra system-prompt text for the caller's
// current location, or "" when no surface-specific nudge applies. Kept as a
// pure function so it is trivial to unit-test.
func buildChatContextHint(ctx *ChatContext) string {
	if ctx == nil {
		return ""
	}
	switch ctx.View {
	case "workspace-actions":
		if ctx.ActionID > 0 {
			return fmt.Sprintf(
				"\n\nThe user is currently editing action %d in workspace %d. Workflow: (1) call get_action with workspace_id=%d, action_id=%d to read the current graph; (2) call describe_action_catalog with workspace_id=%d if you need to recall node configs; (3) compose the full replacement graph and call update_action — the editor live-reloads on success. Optionally validate non-trivial changes with validate_action before the write. update_action is a full replace (not a patch), so you must include every node and edge you want to keep. After update_action succeeds, do not call it again; summarize the completed change to the user.",
				ctx.ActionID, ctx.WorkspaceID, ctx.WorkspaceID, ctx.ActionID, ctx.WorkspaceID,
			)
		}
		if ctx.WorkspaceID > 0 {
			return fmt.Sprintf(
				"\n\nThe user is on the action settings page for workspace %d. If they ask you to build an automation, use describe_action_catalog to discover available triggers and nodes, list_action_templates for shipped blueprints, then create_action to persist a new automation in this workspace. After create_action succeeds, do not call it again; summarize the created automation to the user.",
				ctx.WorkspaceID,
			)
		}
	case "workspace-pages":
		if ctx.PageID > 0 {
			return fmt.Sprintf(
				"\n\nThe user is currently viewing knowledge page %d in workspace %d. If they ask you to read or summarize the current page, call get_page with page_id=%d before answering. If they ask you to change the current page, first call get_page with page_id=%d, then call update_page with page_id=%d and the complete replacement Markdown content and/or title. After update_page succeeds, do not call it again; summarize the completed change to the user.",
				ctx.PageID, ctx.WorkspaceID, ctx.PageID, ctx.PageID, ctx.PageID,
			)
		}
		if ctx.WorkspaceID > 0 {
			return fmt.Sprintf(
				"\n\nThe user is on the knowledge pages area for workspace %d. If they ask about pages or workspace docs without naming one, use list_pages or search_knowledge in workspace_id=%d to find the relevant page before answering. If they ask you to create a page, use create_page in this workspace.",
				ctx.WorkspaceID, ctx.WorkspaceID,
			)
		}
	case "item-detail":
		if ctx.ItemKey != "" {
			return fmt.Sprintf(
				"\n\nThe user is currently viewing work item %s. If they ask you to read, summarize, or change the current item, call get_item with item_key=%q before answering or mutating. Use update_item with item_key=%q for field changes, transition_item with item_key=%q for status changes, add_comment with item_key=%q for comments, and get_item_children if they ask about sub-tasks. After a mutating tool succeeds, do not call it again; summarize the completed change to the user.",
				ctx.ItemKey, ctx.ItemKey, ctx.ItemKey, ctx.ItemKey, ctx.ItemKey,
			)
		}
		if ctx.ItemID > 0 {
			location := fmt.Sprintf("work item %d", ctx.ItemID)
			if ctx.WorkspaceID > 0 {
				location = fmt.Sprintf("work item %d in workspace %d", ctx.ItemID, ctx.WorkspaceID)
			}
			return fmt.Sprintf(
				"\n\nThe user is currently viewing %s. If they ask you to read, summarize, or change the current item, call get_item with item_id=%d before answering or mutating. Use update_item with item_id=%d for field changes, transition_item with item_id=%d for status changes, add_comment with item_id=%d for comments, and get_item_children if they ask about sub-tasks. After a mutating tool succeeds, do not call it again; summarize the completed change to the user.",
				location, ctx.ItemID, ctx.ItemID, ctx.ItemID, ctx.ItemID,
			)
		}
	}
	return ""
}

func chatTerminalTools() map[string]bool {
	return map[string]bool{
		"add_comment":            true,
		"add_link":               true,
		"archive_page":           true,
		"create_action":          true,
		"create_diagram":         true,
		"create_item":            true,
		"create_page":            true,
		"delete_comment":         true,
		"delete_diagram":         true,
		"delete_item":            true,
		"end_test_run":           true,
		"grant_page_permission":  true,
		"log_time":               true,
		"move_page":              true,
		"record_test_result":     true,
		"remove_link":            true,
		"restore_page_revision":  true,
		"revoke_page_permission": true,
		"set_item_labels":        true,
		"set_page_inheritance":   true,
		"start_test_run":         true,
		"start_timer":            true,
		"stop_timer":             true,
		"transition_item":        true,
		"update_action":          true,
		"update_comment":         true,
		"update_diagram":         true,
		"update_item":            true,
		"update_page":            true,
	}
}

// ChatResponse is the response from the agentic chat endpoint.
type ChatResponse struct {
	SessionID     int                  `json:"session_id"`
	UserMessageID int                  `json:"user_message_id"`
	MessageID     int                  `json:"message_id"`
	RunID         int                  `json:"run_id"`
	Answer        string               `json:"answer"`
	ToolCalls     []llm.ToolCallRecord `json:"tool_calls,omitempty"`
	Iterations    int                  `json:"iterations"`
	MaxIterations int                  `json:"max_iterations"`
	StopReason    string               `json:"stop_reason"`
	// NeedsReview flags a recovery-aware, high-signal tool misuse (the model
	// invented a tool or could not satisfy a tool's schema, and never
	// recovered) — a correlate of hallucination worth a human glance. It is
	// not a claim that the answer is wrong. ReviewReasons explains why.
	NeedsReview   bool     `json:"needs_review,omitempty"`
	ReviewReasons []string `json:"review_reasons,omitempty"`
}

// reviewVerdictForToolCalls computes the recovery-aware review flag for a chat
// run's tool calls, reusing the same classifier + evaluator the coding agent
// drains into. Pure — exposed for testing the chat-side mapping without the
// full handler stack.
func reviewVerdictForToolCalls(calls []llm.ToolCallRecord) llm.ReviewVerdict {
	outcomes := make([]llm.ToolCallOutcome, len(calls))
	for i, tc := range calls {
		outcomes[i] = llm.ToolCallOutcome{Tool: tc.Name, Class: llm.Classify(tc.Name, tc.Result)}
	}
	return llm.EvaluateReview(outcomes, llm.DefaultReviewFlagConfig())
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
	if len(req.Message) > 256*1024 {
		respondBadRequest(w, r, "message is too long")
		return
	}
	persistedMessage := req.Message
	promptMessage := req.Message
	sanitize.Apply(&promptMessage, sanitize.Comment)
	if req.Context != nil {
		sanitize.Apply(&req.Context.View, sanitize.ShortIdentifier)
		sanitize.Apply(&req.Context.ItemKey, sanitize.ShortIdentifier)
	}
	if strings.TrimSpace(promptMessage) == "" {
		respondBadRequest(w, r, "message is required")
		return
	}

	session, err := h.resolveChatSession(r.Context(), user.ID, req.SessionID)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrAgentSessionNotFound):
			respondNotFound(w, r, "agent session")
		case errors.Is(err, repository.ErrAgentSessionArchived):
			respondConflict(w, r, "agent session is archived")
		default:
			respondInternalError(w, r, err)
		}
		return
	}

	accessibleWSIDs, err := GetAccessibleWorkspaceIDs(user, h.db, h.permService)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get accessible workspaces: %w", err))
		return
	}
	if len(accessibleWSIDs) == 0 {
		respondForbidden(w, r)
		return
	}

	mode, err := h.prepareChatMode(r.Context(), user, session, accessibleWSIDs, req.Context)
	if err != nil {
		switch {
		case errors.Is(err, errChatPermissionDenied):
			respondForbidden(w, r)
		case errors.Is(err, services.ErrBindingUnavailable):
			respondConflict(w, r, "the selected Standard agent is not Ready")
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	contextJSON := ""
	if req.Context != nil {
		if body, err := json.Marshal(req.Context); err == nil {
			contextJSON = string(body)
		}
	}
	begun, err := h.conversations.BeginTurn(r.Context(), repository.BeginAgentTurnInput{
		SessionID:           session.ID,
		SenderUserID:        user.ID,
		SenderUsername:      user.Username,
		ActingUserID:        mode.actingUserID,
		WorkspaceID:         mode.runWorkspaceID,
		BindingID:           mode.bindingID,
		JobKind:             mode.jobKind,
		ProfileVersion:      mode.profileVersion,
		ProfileSnapshotJSON: mode.profileSnapshotJSON,
		GrantsJSON:          mode.grantsJSON,
		Content:             persistedMessage,
		ContextJSON:         contextJSON,
	})
	if errors.Is(err, repository.ErrAgentSessionBusy) {
		respondConflict(w, r, "agent session already has an active turn")
		return
	}
	if errors.Is(err, repository.ErrAgentSessionNotFound) {
		respondNotFound(w, r, "agent session")
		return
	}
	if errors.Is(err, repository.ErrAgentSessionArchived) {
		respondConflict(w, r, "agent session is archived")
		return
	}
	if err != nil {
		// Fail closed: no LLM request occurs if message/run/audit correlation
		// could not be committed atomically.
		respondInternalError(w, r, fmt.Errorf("persist agent turn: %w", err))
		return
	}
	runRepo := repository.NewAgentRunRepository(h.db)
	if transitioned, err := runRepo.MarkRunningIfQueued(r.Context(), begun.RunID, "", time.Now().UTC()); err != nil || !transitioned {
		_ = runRepo.Finalize(r.Context(), begun.RunID, models.AgentRunStatusFailed,
			"Agent chat could not start", time.Now().UTC())
		respondInternalError(w, r, errors.New("agent chat run could not start"))
		return
	}

	chatTimezone := user.Timezone
	if chatTimezone == "" {
		chatTimezone = "UTC"
	}
	chatNow := time.Now()
	if chatLoc, locErr := time.LoadLocation(chatTimezone); locErr == nil {
		chatNow = chatNow.In(chatLoc)
	}

	systemPrompt := mode.systemPrompt
	if session.SessionType == models.AgentSessionGeneral {
		systemPrompt = fmt.Sprintf(h.promptStore.Get(llm.PromptAIChat),
			chatNow.Format("2006-01-02"), user.FullName, user.ID, user.ID,
		) + buildChatContextHint(req.Context)
	}
	historyAfterMessageID := 0
	if session.SessionType == models.AgentSessionGeneral {
		historyAfterMessageID = req.HistoryAfterMessageID
	}
	priorMessages, err := h.conversations.ListMessagesForParticipantAfter(
		r.Context(), session.ID, user.ID, historyAfterMessageID, begun.MessageID, 200)
	if err != nil {
		_ = runRepo.Finalize(r.Context(), begun.RunID, models.AgentRunStatusFailed,
			"Agent chat history could not be loaded", time.Now().UTC())
		respondInternalError(w, r, err)
		return
	}
	var history []llm.Message
	for _, message := range priorMessages {
		content := message.Content
		sanitize.Apply(&content, sanitize.Comment)
		history = append(history, llm.Message{Role: message.Role, Content: content})
	}

	llmClient, err := mode.resolveLLM(h.chatLLMs, req.ConnectionID)
	if err != nil {
		_ = runRepo.Finalize(r.Context(), begun.RunID, models.AgentRunStatusFailed,
			"Configured LLM is unavailable", time.Now().UTC())
		respondLLMError(w, r, err)
		return
	}

	// The agentic loop (up to MaxIterations of LLM round-trips + tool calls) is
	// the longest-running AI handler, so it needs the same write-deadline escape
	// and work bound as the one-shot handlers — otherwise the server's 30s
	// WriteTimeout severs the response mid-run.
	extendWriteDeadline(w)
	ctx, cancel := context.WithTimeout(r.Context(), llm.DefaultRequestTimeout)
	defer cancel()
	actingTimezone, err := services.LookupUserTimezone(h.db, mode.actingUserID)
	if err != nil {
		_ = runRepo.Finalize(r.Context(), begun.RunID, models.AgentRunStatusFailed,
			"Acting user timezone is invalid", time.Now().UTC())
		respondInternalError(w, r, err)
		return
	}
	systemPrompt += fmt.Sprintf("\n\nThe acting user's authoritative IANA timezone is %s. Interpret relative dates and unqualified wall-clock times in that timezone; never pre-offset values passed to time tools.", actingTimezone)

	env := &aitools.Env{
		DB:                     h.db,
		UserID:                 mode.actingUserID,
		Username:               mode.actingName,
		Timezone:               actingTimezone,
		Source:                 mode.source,
		AccessibleWorkspaceIDs: mode.accessibleWorkspaceIDs,
		AuditDetails: map[string]any{
			"agent_session_id":          session.ID,
			"agent_message_id":          begun.MessageID,
			"agent_run_id":              begun.RunID,
			"root_initiator_user_id":    user.ID,
			"immediate_trigger_user_id": user.ID,
			"acting_user_id":            mode.actingUserID,
			"workspace_id":              mode.runWorkspaceID,
		},
		PermService:            h.permService,
		TimePermService:        h.timePermService,
		TimerService:           h.timerService,
		CommentService:         h.commentService,
		ApprovalService:        h.approvalService,
		ActionService:          h.actionService,
		PageApplicationService: h.pageApplicationService,
		PageDiagramService:     h.pageDiagramService,
	}
	if env.CommentService == nil {
		env.CommentService = services.NewCommentService(h.db)
	}
	executor := aitooladapter.NewExecutor(env, mode.entries)
	result, err := llm.RunAgent(ctx, llmClient, llm.AgentConfig{
		SystemPrompt:  systemPrompt,
		Tools:         aitooladapter.BuildTools(mode.entries),
		MaxTokens:     2048,
		Temperature:   0.1,
		MaxIterations: 12,
		TerminalTools: mode.terminalTools,
	}, promptMessage, executor.Execute, history)
	if err != nil {
		_ = runRepo.Finalize(context.Background(), begun.RunID, models.AgentRunStatusFailed,
			"Agent chat execution failed", time.Now().UTC())
		slog.ErrorContext(r.Context(), "chat agent run failed",
			slog.Int("user_id", user.ID),
			slog.String("ctx_view", chatContextView(req.Context)),
			slog.Int("ctx_workspace_id", chatContextWorkspaceID(req.Context)),
			slog.Int("ctx_action_id", chatContextActionID(req.Context)),
			slog.Int("ctx_page_id", chatContextPageID(req.Context)),
			slog.Int("ctx_item_id", chatContextItemID(req.Context)),
			slog.String("ctx_item_key", chatContextItemKey(req.Context)),
			slog.String("error_type", fmt.Sprintf("%T", err)),
		)
		respondLLMError(w, r, err)
		return
	}

	slog.InfoContext(r.Context(), "chat agent run",
		slog.Int("user_id", user.ID),
		slog.String("ctx_view", chatContextView(req.Context)),
		slog.Int("ctx_workspace_id", chatContextWorkspaceID(req.Context)),
		slog.Int("ctx_action_id", chatContextActionID(req.Context)),
		slog.Int("ctx_page_id", chatContextPageID(req.Context)),
		slog.Int("ctx_item_id", chatContextItemID(req.Context)),
		slog.String("ctx_item_key", chatContextItemKey(req.Context)),
		slog.String("stop_reason", string(result.StopReason)),
		slog.Int("iterations", result.Iterations),
		slog.Int("max_iterations", result.MaxIter),
		slog.Int("tool_calls", len(result.ToolCalls)),
	)

	// Recovery-aware review flag over the run's tool calls (same classifier the
	// coding agent uses). Persist only the verdict and sanitized tool summaries;
	// exact user and assistant content belongs exclusively to the transcript.
	verdict := reviewVerdictForToolCalls(result.ToolCalls)
	toolSummaries := make([]map[string]string, 0, len(result.ToolCalls))
	for _, call := range result.ToolCalls {
		status := "succeeded"
		var body map[string]any
		if json.Unmarshal([]byte(call.Result), &body) == nil && body["error"] != nil {
			status = "failed"
		}
		toolSummaries = append(toolSummaries, map[string]string{"name": call.Name, "status": status})
		_ = runRepo.AppendEvent(context.Background(), begun.RunID, "tool",
			marshalChatMetadata(map[string]any{"name": call.Name, "status": status}))
	}
	metadata := marshalChatMetadata(map[string]any{
		"stop_reason":    result.StopReason,
		"iterations":     result.Iterations,
		"tool_summaries": toolSummaries,
		"needs_review":   verdict.Flagged,
		"review_reasons": verdict.Reasons,
	})
	assistantMessageID, err := h.conversations.CompleteTurn(
		context.Background(), session.ID, begun.RunID, mode.actingUserID,
		result.Answer, metadata)
	if err != nil {
		_ = runRepo.Finalize(context.Background(), begun.RunID, models.AgentRunStatusFailed,
			"Agent response could not be persisted", time.Now().UTC())
		respondInternalError(w, r, err)
		return
	}
	_ = runRepo.AppendEvent(context.Background(), begun.RunID, "succeeded",
		marshalChatMetadata(map[string]any{"message_id": assistantMessageID}))

	respondJSONOK(w, ChatResponse{
		SessionID:     session.ID,
		UserMessageID: begun.MessageID,
		MessageID:     assistantMessageID,
		RunID:         begun.RunID,
		Answer:        result.Answer,
		ToolCalls:     result.ToolCalls,
		Iterations:    result.Iterations,
		MaxIterations: result.MaxIter,
		StopReason:    string(result.StopReason),
		NeedsReview:   verdict.Flagged,
		ReviewReasons: verdict.Reasons,
	})
}

var errChatPermissionDenied = errors.New("agent chat permission denied")

type chatExecutionMode struct {
	actingUserID           int
	actingName             string
	runWorkspaceID         int
	bindingID              *int
	jobKind                string
	profileVersion         int
	profileSnapshotJSON    string
	grantsJSON             string
	source                 string
	accessibleWorkspaceIDs []int
	entries                []aitools.Entry
	terminalTools          map[string]bool
	systemPrompt           string
	llmConnectionID        int
	standard               bool
}

type chatLLMResolver interface {
	Resolve(connectionID int) (llm.Client, error)
	ResolveForFeatureWithOverride(featureKey string, userOverrideConnectionID int) (llm.Client, error)
}

func (m chatExecutionMode) resolveLLM(manager chatLLMResolver, overrideID int) (llm.Client, error) {
	if m.standard {
		return manager.Resolve(m.llmConnectionID)
	}
	return manager.ResolveForFeatureWithOverride("ai_chat", overrideID)
}

func (h *AIHandler) resolveChatSession(ctx context.Context, userID, requestedSessionID int) (*models.AgentSession, error) {
	if requestedSessionID <= 0 {
		return h.conversations.EnsureGeneralSession(ctx, userID)
	}
	session, err := h.conversations.GetForParticipant(ctx, requestedSessionID, userID)
	if err != nil {
		return nil, err
	}
	if session.ArchivedAt != nil {
		return nil, repository.ErrAgentSessionArchived
	}
	return session, nil
}

func (h *AIHandler) prepareChatMode(ctx context.Context, user *models.User, session *models.AgentSession, accessibleWSIDs []int, chatCtx *ChatContext) (chatExecutionMode, error) {
	containsWorkspace := func(id int) bool {
		for _, allowed := range accessibleWSIDs {
			if allowed == id {
				return true
			}
		}
		return false
	}
	if session.SessionType == models.AgentSessionGeneral {
		runWorkspaceID := accessibleWSIDs[0]
		if chatCtx != nil && chatCtx.WorkspaceID > 0 {
			if !containsWorkspace(chatCtx.WorkspaceID) {
				return chatExecutionMode{}, errChatPermissionDenied
			}
			runWorkspaceID = chatCtx.WorkspaceID
		}
		// The chat runs on the caller's cookie session, not an API token, so
		// there is no scope set to read off a credential. Gate it on
		// DefaultAgentScopes anyway (WI-962) so one agent surface can't quietly
		// do more than another: what chat can reach now matches what a `ws` CLI
		// or MCP token reaches by default. Per-workspace permission checks
		// inside each tool still apply on top.
		entries := aitooladapter.EntriesForScopes(aitools.Default, auth.DefaultAgentScopes)
		toolNames := make([]string, 0, len(entries))
		for _, entry := range entries {
			toolNames = append(toolNames, entry.Name)
		}
		grantsJSON := marshalChatMetadata(map[string]any{
			"workspace_ids": accessibleWSIDs,
			"tools":         toolNames,
		})
		return chatExecutionMode{
			actingUserID:           user.ID,
			actingName:             user.FullName,
			runWorkspaceID:         runWorkspaceID,
			jobKind:                models.JobKindGeneralAgent,
			profileSnapshotJSON:    `{"session_type":"general"}`,
			grantsJSON:             grantsJSON,
			source:                 aitools.SourceAIChat,
			accessibleWorkspaceIDs: append([]int(nil), accessibleWSIDs...),
			entries:                entries,
			terminalTools:          chatTerminalTools(),
		}, nil
	}
	if session.SessionType != models.AgentSessionStandard ||
		session.WorkspaceID == nil || session.AgentProfileID == nil {
		return chatExecutionMode{}, repository.ErrAgentSessionNotFound
	}
	if !containsWorkspace(*session.WorkspaceID) ||
		(chatCtx != nil && chatCtx.WorkspaceID > 0 && chatCtx.WorkspaceID != *session.WorkspaceID) {
		return chatExecutionMode{}, errChatPermissionDenied
	}
	canInvoke, err := h.permService.HasWorkspacePermission(user.ID, *session.WorkspaceID, models.PermissionItemEdit)
	if err != nil {
		return chatExecutionMode{}, err
	}
	if !canInvoke {
		return chatExecutionMode{}, errChatPermissionDenied
	}
	profile, err := h.agentBindings.Get(ctx, *session.AgentProfileID)
	if err != nil {
		return chatExecutionMode{}, err
	}
	if profile.WorkspaceID != *session.WorkspaceID ||
		profile.ProfileType != models.AgentProfileStandard ||
		profile.Lifecycle != models.AgentLifecycleReady ||
		profile.LLMConnectionID == nil {
		return chatExecutionMode{}, services.ErrBindingUnavailable
	}
	actingPermissions, err := h.permService.HasWorkspacePermissions(profile.ActingUserID, profile.WorkspaceID,
		[]string{models.PermissionItemView, models.PermissionItemComment})
	if err != nil {
		return chatExecutionMode{}, err
	}
	if !actingPermissions[models.PermissionItemView] || !actingPermissions[models.PermissionItemComment] {
		return chatExecutionMode{}, errChatPermissionDenied
	}
	entries := aitooladapter.EntriesForStandard(aitools.Default, profile.CapabilityGroups)
	toolNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		toolNames = append(toolNames, entry.Name)
	}
	profileSnapshotJSON := marshalChatMetadata(map[string]any{
		"binding_id":        profile.ID,
		"profile_version":   profile.ProfileVersion,
		"acting_user_id":    profile.ActingUserID,
		"acting_name":       profile.DisplayName,
		"llm_connection_id": *profile.LLMConnectionID,
		"instructions":      profile.Instructions,
		"purpose":           profile.Purpose,
		"capability_groups": profile.CapabilityGroups,
		"tool_names":        toolNames,
	})
	grantsJSON := marshalChatMetadata(map[string]any{
		"workspace_ids": []int{profile.WorkspaceID},
		"tools":         toolNames,
	})
	bindingID := profile.ID
	return chatExecutionMode{
		actingUserID:           profile.ActingUserID,
		actingName:             profile.DisplayName,
		runWorkspaceID:         profile.WorkspaceID,
		bindingID:              &bindingID,
		jobKind:                models.JobKindStandardAgent,
		profileVersion:         profile.ProfileVersion,
		profileSnapshotJSON:    profileSnapshotJSON,
		grantsJSON:             grantsJSON,
		source:                 aitools.SourceStandardAgent,
		accessibleWorkspaceIDs: []int{profile.WorkspaceID},
		entries:                entries,
		terminalTools:          aitooladapter.TerminalTools(entries),
		systemPrompt: fmt.Sprintf(`You are %s, a Standard workspace agent.
Act only through the provided tools and only within workspace %d.
Your response is part of a private participant conversation. Never claim a
mutation succeeded unless its tool call succeeded.

Purpose: %s

Profile instructions:
%s`, profile.DisplayName, profile.WorkspaceID,
			strings.TrimSpace(profile.Purpose), strings.TrimSpace(profile.Instructions)),
		llmConnectionID: *profile.LLMConnectionID,
		standard:        true,
	}, nil
}

func marshalChatMetadata(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(body)
}

// chatContextView / WorkspaceID / ActionID / PageID / Item safely read fields off a
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
func chatContextPageID(c *ChatContext) int {
	if c == nil {
		return 0
	}
	return c.PageID
}
func chatContextItemID(c *ChatContext) int {
	if c == nil {
		return 0
	}
	return c.ItemID
}
func chatContextItemKey(c *ChatContext) string {
	if c == nil {
		return ""
	}
	return c.ItemKey
}
