package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/services"
)

// BriefingScheduler generates daily briefings for all users in the background.
type BriefingScheduler struct {
	db              database.Database
	llmManager      *llm.ConnectionManager
	permService     *services.PermissionService
	timePermService *services.TimePermissionService
	promptStore     *llm.PromptStore
	ticker          *time.Ticker
	stopChan        chan struct{}
	mu              sync.RWMutex
	running         bool
}

// NewBriefingScheduler creates a new briefing scheduler.
func NewBriefingScheduler(db database.Database, llmManager *llm.ConnectionManager, permService *services.PermissionService, timePermService *services.TimePermissionService, promptStore *llm.PromptStore) *BriefingScheduler {
	return &BriefingScheduler{
		db:              db,
		llmManager:      llmManager,
		permService:     permService,
		timePermService: timePermService,
		promptStore:     promptStore,
		ticker:          time.NewTicker(6 * time.Hour),
		stopChan:        make(chan struct{}),
	}
}

// Start begins the briefing scheduler.
func (bs *BriefingScheduler) Start() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.running {
		return
	}

	bs.running = true
	slog.Info("briefing scheduler started", slog.String("component", "scheduler"), slog.String("interval", "6h"))

	go bs.schedulerLoop()
}

// Stop stops the briefing scheduler.
func (bs *BriefingScheduler) Stop() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if !bs.running {
		return
	}

	bs.running = false
	bs.ticker.Stop()
	close(bs.stopChan)
	slog.Info("briefing scheduler stopped", slog.String("component", "scheduler"))
}

func (bs *BriefingScheduler) schedulerLoop() {
	bs.safeGenerateAllBriefings()

	for {
		select {
		case <-bs.ticker.C:
			bs.safeGenerateAllBriefings()
		case <-bs.stopChan:
			return
		}
	}
}

func (bs *BriefingScheduler) safeGenerateAllBriefings() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("briefing: panic in generateAllBriefings", slog.Any("panic", r))
		}
	}()
	bs.generateAllBriefings()
}

func (bs *BriefingScheduler) generateAllBriefings() {
	// Check per-feature config for daily_briefing
	llmClient, err := bs.llmManager.ResolveForFeature("daily_briefing", bs.db)
	if err != nil {
		slog.Info("briefing: generation skipped", slog.Any("reason", err))
		return
	}
	if llmClient == nil || !llmClient.Available() {
		slog.Info("briefing: generation skipped, AI not available")
		return
	}

	// Check schedule: "every_6h" allows regeneration on the same day
	regenerate := false
	if cfg, err := llm.LoadAIFeaturesConfig(bs.db); err == nil {
		regenerate = cfg["daily_briefing"].Schedule == "every_6h"
	}

	// Get active users – empty-context filtering happens in generateBriefingForUser
	rows, err := bs.db.Query(`SELECT id, first_name, last_name, COALESCE(timezone, 'UTC') FROM users WHERE is_active = 1`)
	if err != nil {
		slog.Error("failed to list users for briefing generation", slog.Any("error", err))
		return
	}
	defer func() { _ = rows.Close() }()

	type userInfo struct {
		ID        int
		FirstName string
		LastName  string
		Timezone  string
	}
	var users []userInfo
	for rows.Next() {
		var u userInfo
		if err := rows.Scan(&u.ID, &u.FirstName, &u.LastName, &u.Timezone); err != nil {
			continue
		}
		users = append(users, u)
	}

	slog.Info("generating daily briefings",
		slog.String("component", "scheduler"),
		slog.Int("users", len(users)),
		slog.Int("delay_seconds", 3),
	)

	for i, u := range users {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in briefing generation", slog.Int("user_id", u.ID), slog.Any("panic", r))
				}
			}()
			bs.generateBriefingForUser(llmClient, u.ID, u.FirstName, u.Timezone, regenerate)
		}()
		if i < len(users)-1 {
			time.Sleep(3 * time.Second)
		}
	}
}

func (bs *BriefingScheduler) generateBriefingForUser(llmClient llm.Client, userID int, firstName, timezone string, regenerate bool) {
	today := time.Now().Format("2006-01-02")

	// Skip if today's briefing already exists (successful), unless regeneration is enabled
	if !regenerate {
		var exists int
		if err := bs.db.QueryRow("SELECT 1 FROM daily_briefings WHERE user_id = ? AND date = ? AND error IS NULL", userID, today).Scan(&exists); err == nil {
			slog.Debug("briefing: already generated today", slog.Int("user_id", userID))
			return
		}
	}

	start := time.Now()

	// Get accessible workspace IDs (inline to avoid import cycle with handlers)
	accessibleWSIDs, err := bs.getAccessibleWorkspaceIDs(userID)
	if err != nil || len(accessibleWSIDs) == 0 {
		slog.Info("briefing: no accessible workspaces",
			slog.Int("user_id", userID),
			slog.Int("workspaces", len(accessibleWSIDs)),
			slog.Any("error", err),
		)
		return
	}

	placeholders := make([]string, len(accessibleWSIDs))
	wsArgs := make([]interface{}, len(accessibleWSIDs))
	for i, id := range accessibleWSIDs {
		placeholders[i] = "?"
		wsArgs[i] = id
	}
	wsIn := strings.Join(placeholders, ",")

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// Gather context: recent activity
	var activityLines []string
	changeQuery := fmt.Sprintf(`SELECT ih.field_name, COALESCE(ih.old_value, ''), COALESCE(ih.new_value, ''),
		w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
		COALESCE(u.first_name || ' ' || u.last_name, 'Unknown') as changed_by
		FROM item_history ih
		JOIN items i ON ih.item_id = i.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN users u ON ih.user_id = u.id
		WHERE i.workspace_id IN (%s) AND ih.changed_at >= ?
		ORDER BY ih.changed_at DESC LIMIT 50`, wsIn)
	changeArgs := append(append([]interface{}{}, wsArgs...), yesterday)
	changeRows, err := bs.db.Query(changeQuery, changeArgs...)
	if err != nil {
		slog.Warn("briefing: changes query failed", slog.Int("user_id", userID), slog.Any("error", err))
	} else {
		defer func() { _ = changeRows.Close() }()
		for changeRows.Next() {
			var field, oldVal, newVal, itemKey, title, changedBy string
			if err := changeRows.Scan(&field, &oldVal, &newVal, &itemKey, &title, &changedBy); err == nil {
				line := fmt.Sprintf("- [%s] %s: %s changed '%s'", itemKey, title, changedBy, field)
				if oldVal != "" || newVal != "" {
					line += fmt.Sprintf(" from '%s' to '%s'", oldVal, newVal)
				}
				activityLines = append(activityLines, line)
			}
		}
	}

	// Gather context: recent comments
	var commentLines []string
	commentQuery := fmt.Sprintf(`SELECT c.content,
		w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
		COALESCE(u.first_name || ' ' || u.last_name, 'Unknown') as author
		FROM comments c
		JOIN items i ON c.item_id = i.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN users u ON c.author_id = u.id
		WHERE i.workspace_id IN (%s) AND c.created_at >= ? AND c.is_private = false
		ORDER BY c.created_at DESC LIMIT 30`, wsIn)
	commentArgs := append(append([]interface{}{}, wsArgs...), yesterday)
	commentRows, err := bs.db.Query(commentQuery, commentArgs...)
	if err != nil {
		slog.Warn("briefing: comments query failed", slog.Int("user_id", userID), slog.Any("error", err))
	} else {
		defer func() { _ = commentRows.Close() }()
		for commentRows.Next() {
			var content, itemKey, title, author string
			if err := commentRows.Scan(&content, &itemKey, &title, &author); err == nil {
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				commentLines = append(commentLines, fmt.Sprintf("- [%s] %s commented on '%s': %s", itemKey, author, title, content))
			}
		}
	}

	// Gather context: assigned open items
	personalWSIDs := []int{}
	pwsRows, err := bs.db.Query("SELECT id FROM workspaces WHERE is_personal = true AND owner_id = ? AND active = true", userID)
	if err != nil {
		slog.Warn("briefing: personal workspaces query failed", slog.Int("user_id", userID), slog.Any("error", err))
	} else {
		defer func() { _ = pwsRows.Close() }()
		for pwsRows.Next() {
			var id int
			if err := pwsRows.Scan(&id); err == nil {
				personalWSIDs = append(personalWSIDs, id)
			}
		}
	}

	var itemLines []string
	itemQuery := fmt.Sprintf(`SELECT w.key, i.workspace_item_number, i.title,
		COALESCE(st.name, ''), COALESCE(p.name, ''), COALESCE(CAST(i.due_date AS TEXT), ''),
		COALESCE(m.name, ''), COALESCE(CAST(m.target_date AS TEXT), ''),
		COALESCE(iter.name, ''), COALESCE(CAST(iter.end_date AS TEXT), '')
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		WHERE i.workspace_id IN (%s) AND (i.assignee_id = ?%s)
		AND COALESCE(sc.is_completed, FALSE) = FALSE
		ORDER BY i.due_date ASC NULLS LAST LIMIT 50`, wsIn, func() string {
		if len(personalWSIDs) > 0 {
			pph := make([]string, len(personalWSIDs))
			for i := range personalWSIDs {
				pph[i] = "?"
			}
			return fmt.Sprintf(" OR i.workspace_id IN (%s)", strings.Join(pph, ","))
		}
		return ""
	}())
	itemArgs := append(append([]interface{}{}, wsArgs...), userID)
	for _, pid := range personalWSIDs {
		itemArgs = append(itemArgs, pid)
	}
	itemRows, err := bs.db.Query(itemQuery, itemArgs...)
	if err != nil {
		slog.Warn("briefing: items query failed", slog.Int("user_id", userID), slog.Any("error", err))
	} else {
		defer func() { _ = itemRows.Close() }()
		for itemRows.Next() {
			var wsKey string
			var itemNum int
			var title, status, priority, dueDate string
			var milestoneName, milestoneTargetDate, iterationName, iterationEndDate string
			if err := itemRows.Scan(&wsKey, &itemNum, &title, &status, &priority, &dueDate, &milestoneName, &milestoneTargetDate, &iterationName, &iterationEndDate); err == nil {
				line := fmt.Sprintf("- [%s-%d] %s", wsKey, itemNum, title)
				if priority != "" {
					line += fmt.Sprintf(" | Priority: %s", priority)
				}
				if dueDate != "" {
					line += fmt.Sprintf(" | Due: %s", dueDate)
				} else {
					line += " | Due: none"
				}
				if status != "" {
					line += fmt.Sprintf(" | Status: %s", status)
				}
				if milestoneName != "" {
					ms := fmt.Sprintf(" | Milestone: %s", milestoneName)
					if milestoneTargetDate != "" {
						ms += fmt.Sprintf(" (target: %s)", milestoneTargetDate)
					}
					line += ms
				}
				if iterationName != "" {
					it := fmt.Sprintf(" | Iteration: %s", iterationName)
					if iterationEndDate != "" {
						it += fmt.Sprintf(" (ends: %s)", iterationEndDate)
					}
					line += it
				}
				itemLines = append(itemLines, line)
			}
		}
	}

	// Gather context: yesterday's worklogs
	var worklogLines []string
	if bs.timePermService != nil {
		yesterdayTime, _ := time.Parse("2006-01-02", yesterday)
		todayTime, _ := time.Parse("2006-01-02", today)
		wlRows, err := bs.db.Query(`SELECT tw.description, tw.duration_minutes, tp.name
			FROM time_worklogs tw
			JOIN time_projects tp ON tw.project_id = tp.id
			WHERE tw.user_id = ? AND tw.date >= ? AND tw.date < ?
			ORDER BY tw.date DESC`,
			userID, yesterdayTime.Unix(), todayTime.Unix())
		if err != nil {
			slog.Warn("briefing: worklogs query failed", slog.Int("user_id", userID), slog.Any("error", err))
		} else {
			defer func() { _ = wlRows.Close() }()
			for wlRows.Next() {
				var desc, projectName string
				var durationMins int
				if err := wlRows.Scan(&desc, &durationMins, &projectName); err == nil {
					worklogLines = append(worklogLines, fmt.Sprintf("- %s (%s): %dm", desc, projectName, durationMins))
				}
			}
		}
	}

	// Build the data block
	var contextParts []string
	if len(activityLines) > 0 {
		contextParts = append(contextParts, "### Recent Changes (last 24h)\n"+strings.Join(activityLines, "\n"))
	}
	if len(commentLines) > 0 {
		contextParts = append(contextParts, "### Recent Comments (last 24h)\n"+strings.Join(commentLines, "\n"))
	}
	if len(itemLines) > 0 {
		contextParts = append(contextParts, "### Your Open Items\n"+strings.Join(itemLines, "\n"))
	}
	if len(worklogLines) > 0 {
		contextParts = append(contextParts, "### Yesterday's Worklogs\n"+strings.Join(worklogLines, "\n"))
	}

	slog.Info("briefing: context gathered",
		slog.Int("user_id", userID),
		slog.Int("changes", len(activityLines)),
		slog.Int("comments", len(commentLines)),
		slog.Int("items", len(itemLines)),
		slog.Int("worklogs", len(worklogLines)),
	)

	if len(contextParts) == 0 {
		slog.Info("briefing: no context found", slog.Int("user_id", userID))
		bs.storeBriefing(userID, today, "", time.Since(start).Milliseconds(), "")
		return
	}

	systemPrompt := bs.promptStore.Get(llm.PromptDailyBriefing)

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)

	userPrompt := fmt.Sprintf("Good morning %s! Today is %s (%s timezone).\n\nHere is your project data:\n\n%s",
		firstName, now.Format("Monday, January 2, 2006"), timezone, strings.Join(contextParts, "\n\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resp, err := llmClient.ChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		MaxTokens:   2048,
	})

	durationMs := time.Since(start).Milliseconds()

	if err != nil || len(resp.Choices) == 0 {
		errMsg := "no response from LLM"
		if err != nil {
			errMsg = err.Error()
		}
		slog.Warn("briefing generation failed", slog.Int("user_id", userID), slog.String("error", errMsg))
		bs.storeBriefing(userID, today, "", durationMs, errMsg)
		return
	}

	content := resp.Choices[0].Message.Content
	bs.storeBriefing(userID, today, content, durationMs, "")

	slog.Info("briefing: generated",
		slog.Int("user_id", userID),
		slog.Int("content_len", len(content)),
		slog.Int64("duration_ms", durationMs),
	)
}

// getAccessibleWorkspaceIDs returns IDs of active workspaces the user can view.
// This duplicates handlers.GetAccessibleWorkspaceIDs to avoid an import cycle.
func (bs *BriefingScheduler) getAccessibleWorkspaceIDs(userID int) ([]int, error) {
	rows, err := bs.db.Query("SELECT id FROM workspaces WHERE active = true")
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		hasView, err := bs.permService.HasWorkspacePermission(userID, id, models.PermissionItemView)
		if err != nil {
			continue
		}
		if hasView {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func (bs *BriefingScheduler) storeBriefing(userID int, date, content string, durationMs int64, errMsg string) {
	var errVal interface{}
	if errMsg != "" {
		errVal = errMsg
	}

	_, err := bs.db.Exec(`INSERT INTO daily_briefings (user_id, date, content, generation_duration_ms, error)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id, date) DO UPDATE SET
		content = excluded.content, generation_duration_ms = excluded.generation_duration_ms,
		error = excluded.error, updated_at = CURRENT_TIMESTAMP`,
		userID, date, content, durationMs, errVal)
	if err != nil {
		slog.Error("failed to store briefing", slog.Int("user_id", userID), slog.Any("error", err))
	}
}
