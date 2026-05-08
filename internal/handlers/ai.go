package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// AIHandler handles AI-powered endpoints.
type AIHandler struct {
	db              database.Database
	llmManager      *llm.ConnectionManager
	permService     *services.PermissionService
	timePermService *services.TimePermissionService
	promptStore     *llm.PromptStore
}

// NewAIHandler creates a new AI handler.
func NewAIHandler(db database.Database, llmManager *llm.ConnectionManager, permService *services.PermissionService, timePermService *services.TimePermissionService, promptStore *llm.PromptStore) *AIHandler {
	return &AIHandler{
		db:              db,
		llmManager:      llmManager,
		permService:     permService,
		timePermService: timePermService,
		promptStore:     promptStore,
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
	user, ok := RequireAuth(w, r)
	if !ok {
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
	statusFilter := "NOT EXISTS (SELECT 1 FROM status_categories sc WHERE sc.id = st.category_id AND COALESCE(sc.is_completed, FALSE) = TRUE) OR i.status_id IS NULL"
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
		} else {
			line += " | Due: none"
		}
		if item.StatusName != "" {
			line += fmt.Sprintf(" | Status: %s", item.StatusName)
		}
		if len(item.Milestones) > 0 {
			names := make([]string, 0, len(item.Milestones))
			for _, m := range item.Milestones {
				if m.TargetDate != nil && *m.TargetDate != "" {
					names = append(names, fmt.Sprintf("%s (target: %s)", m.Name, *m.TargetDate))
				} else {
					names = append(names, m.Name)
				}
			}
			line += " | Milestones: " + strings.Join(names, ", ")
		}
		if item.IterationName != "" {
			it := fmt.Sprintf(" | Iteration: %s", item.IterationName)
			if item.IterationEndDate != "" {
				it += fmt.Sprintf(" (ends: %s)", item.IterationEndDate)
			}
			line += it
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

	systemPrompt := h.promptStore.Get(llm.PromptPlanMyDay)

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
	llmClient := requireLLMClientForFeature(w, r, h.llmManager, "plan_my_day", parseConnectionIDParam(r))
	if llmClient == nil {
		return
	}

	// Call the LLM with structured output
	extendWriteDeadline(w)
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
		respondLLMError(w, r, err)
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

	// Load per-feature config
	cfg, _ := llm.LoadAIFeaturesConfig(h.db)
	featureKeys := []string{"ai_chat", "daily_briefing", "plan_my_day", "catch_me_up", "find_similar", "decompose", "release_notes", "dependency_analysis"}
	type featureStatus struct {
		Enabled bool   `json:"enabled"`
		Mode    string `json:"mode"`
	}
	features := make(map[string]featureStatus, len(featureKeys))
	for _, key := range featureKeys {
		fc, ok := cfg[key]
		if !ok {
			features[key] = featureStatus{Enabled: true, Mode: string(models.AIFeatureModeDefault)}
		} else {
			features[key] = featureStatus{
				Enabled: fc.Mode != models.AIFeatureModeDisabled,
				Mode:    string(fc.Mode),
			}
		}
	}

	// chat_enabled derived from per-feature config for backward compatibility
	chatEnabled := features["ai_chat"].Enabled

	respondJSONOK(w, map[string]interface{}{
		"available":    available,
		"chat_enabled": chatEnabled,
		"features":     features,
	})
}

// GetDailyBriefing returns the most recent successful daily briefing for the current user.
func (h *AIHandler) GetDailyBriefing(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var id int
	var content, date, updatedAtStr string
	err := h.db.QueryRow(
		`SELECT id, content, date, updated_at FROM daily_briefings WHERE user_id = ? AND error IS NULL ORDER BY date DESC LIMIT 1`,
		user.ID,
	).Scan(&id, &content, &date, &updatedAtStr)

	if err != nil {
		slog.Warn("GetDailyBriefing: no briefing found", slog.Int("user_id", user.ID), slog.Any("error", err))
		respondJSONOK(w, map[string]interface{}{"content": ""})
		return
	}

	generatedAt := updatedAtStr
	if t, parseErr := time.Parse("2006-01-02 15:04:05", updatedAtStr); parseErr == nil {
		generatedAt = t.Format(time.RFC3339)
	}

	slog.Info("GetDailyBriefing: returning briefing",
		slog.Int("user_id", user.ID),
		slog.Int("id", id),
		slog.String("date", date),
		slog.String("updated_at_str", updatedAtStr),
		slog.String("generated_at", generatedAt),
		slog.Int("content_len", len(content)),
	)

	// Extract and resolve item key references from content
	itemKeyRe := regexp.MustCompile(`[A-Z]{2,10}-\d+`)
	keys := itemKeyRe.FindAllString(content, -1)

	references := map[string]interface{}{}
	if len(keys) > 0 {
		seen := map[string]bool{}
		unique := make([]string, 0, len(keys))
		for _, k := range keys {
			if !seen[k] {
				seen[k] = true
				unique = append(unique, k)
			}
		}

		if refs, qErr := repository.NewItemRepository(h.db).ResolveItemKeyReferences(unique); qErr == nil {
			for _, ref := range refs {
				references[ref.ItemKey] = map[string]interface{}{
					"item_id":      ref.ItemID,
					"workspace_id": ref.WorkspaceID,
				}
			}
		}
	}

	respondJSONOK(w, map[string]interface{}{
		"id":           id,
		"content":      content,
		"date":         date,
		"generated_at": generatedAt,
		"references":   references,
	})
}
