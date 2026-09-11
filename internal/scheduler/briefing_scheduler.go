package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// briefingConcurrency caps how many users are briefed in parallel within one
// tick. The previous design serialized every user with a 3s sleep between
// them, so 1000 users took ~50 minutes of pure sleeping before any LLM work;
// a bounded worker pool paces the LLM naturally (each call is itself bounded
// by DefaultRequestTimeout) without an artificial inter-user delay. The LLM
// endpoint and the Postgres connection pool are the real ceilings, so this is
// deliberately modest.
const briefingConcurrency = 8

const dailyBriefingMaxTokens = 5000

// BriefingScheduler generates daily briefings for all users in the background.
type BriefingScheduler struct {
	db              database.Database
	llmManager      *llm.ConnectionManager
	permService     *services.PermissionService
	timePermService *services.TimePermissionService
	userService     *services.UserReadService
	promptStore     *llm.PromptStore
	aiRepo          *repository.AIRepository
	runRepo         *repository.SchedulerRunRepository
	ticker          *time.Ticker
	stopChan        chan struct{}
	mu              sync.RWMutex
	running         bool

	// now is overridable for tests; production uses wall-clock time.
	now func() time.Time
}

// NewBriefingScheduler creates a new briefing scheduler.
func NewBriefingScheduler(db database.Database, llmManager *llm.ConnectionManager, permService *services.PermissionService, timePermService *services.TimePermissionService, userService *services.UserReadService, promptStore *llm.PromptStore) *BriefingScheduler {
	return &BriefingScheduler{
		db:              db,
		llmManager:      llmManager,
		permService:     permService,
		timePermService: timePermService,
		userService:     userService,
		promptStore:     promptStore,
		aiRepo:          repository.NewAIRepository(db),
		runRepo:         repository.NewSchedulerRunRepository(db),
		stopChan:        make(chan struct{}),
		now:             time.Now,
	}
}

// Start begins the briefing scheduler.
func (bs *BriefingScheduler) Start() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.running {
		return
	}

	bs.ticker = time.NewTicker(6 * time.Hour)
	bs.stopChan = make(chan struct{})
	bs.running = true
	slog.Info("briefing scheduler started", slog.String("component", "scheduler"), slog.String("interval", "6h"))

	go bs.schedulerLoop(bs.ticker, bs.stopChan)
}

// Stop stops the briefing scheduler.
func (bs *BriefingScheduler) Stop() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if !bs.running {
		return
	}

	bs.running = false
	if bs.ticker != nil {
		bs.ticker.Stop()
		bs.ticker = nil
	}
	close(bs.stopChan)
	slog.Info("briefing scheduler stopped", slog.String("component", "scheduler"))
}

func (bs *BriefingScheduler) schedulerLoop(ticker *time.Ticker, stopChan <-chan struct{}) {
	bs.safeGenerateAllBriefings(stopChan)

	for {
		select {
		case <-ticker.C:
			bs.safeGenerateAllBriefings(stopChan)
		case <-stopChan:
			return
		}
	}
}

func (bs *BriefingScheduler) safeGenerateAllBriefings(stop <-chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("briefing: panic in generateAllBriefings", slog.Any("panic", r))
		}
	}()
	bs.generateAllBriefings(stop)
}

// last review: ser, 300526
func (bs *BriefingScheduler) generateAllBriefings(stop <-chan struct{}) {
	start := time.Now()
	var usersProcessed int
	var runErr error
	defer recordSchedulerRun(bs.runRepo, "briefing", start, &usersProcessed, &runErr)

	// Check per-feature config for daily_briefing
	llmClient, err := bs.llmManager.ResolveForFeature("daily_briefing")
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
	users, err := bs.userService.ListAll()
	if err != nil {
		slog.Error("failed to list users for briefing generation", slog.Any("error", err))
		runErr = err
		return
	}
	usersProcessed = len(users)

	// The id→name reference maps are global (statuses/priorities/milestones/
	// iterations/users), not per-user. Load them ONCE for the whole tick and
	// share across every worker — the previous code re-ran the five SELECTs
	// once per user, i.e. ~5000 reference reads for 1000 users.
	lookups := repository.NewLookupRepository(bs.db).LoadNameMaps()
	// The item/workspace repositories are stateless wrappers over the shared
	// db handle, so one instance serves every worker.
	itemRepo := repository.NewItemRepository(bs.db)
	workspaceRepo := repository.NewWorkspaceRepository(bs.db)

	slog.Info("generating daily briefings",
		slog.String("component", "scheduler"),
		slog.Int("users", len(users)),
		slog.Int("concurrency", briefingConcurrency),
	)

	now := bs.now()
	// stop is the run's stopChan, captured once in Start() and threaded down
	// from schedulerLoop. Passing it as a parameter (rather than re-reading the
	// bs.stopChan field here) keeps every worker on a stable reference and
	// avoids racing Start()/Stop(), which mutate the field under bs.mu.

	// Bounded worker pool. A buffered channel is the semaphore; each worker
	// claims a slot before generating and releases it on return. This replaces
	// the old serial loop + 3s sleep between users, which scaled linearly and
	// spent most of its wall-clock asleep.
	var (
		wg       sync.WaitGroup
		failMu   sync.Mutex
		failures int
	)
	sem := make(chan struct{}, briefingConcurrency)
	for _, u := range users {
		wg.Add(1)
		go func(u models.User) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-stop:
				// Server shutting down — stop dispatching new work.
				return
			}
			defer func() { <-sem }()

			ok := func() (succeeded bool) {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("panic in briefing generation", slog.Int("user_id", u.ID), slog.Any("panic", r))
					}
				}()
				return bs.generateBriefingForUser(llmClient, lookups, itemRepo, workspaceRepo, u, regenerate, now)
			}()
			if !ok {
				failMu.Lock()
				failures++
				failMu.Unlock()
			}
		}(u)
	}
	wg.Wait()

	// Recovered panics count as failures in the scheduler result.
	if failures > 0 {
		runErr = fmt.Errorf("%d of %d daily briefings failed", failures, usersProcessed)
	}
}

// generateBriefingForUser returns false only for generation or storage failure.
// Shared lookups and the tick time are injected. A leased per-user/day claim
// deduplicates instances and self-heals after a crashed holder.
func (bs *BriefingScheduler) generateBriefingForUser(llmClient llm.Client, lookups *repository.NameMaps, itemRepo *repository.ItemRepository, workspaceRepo *repository.WorkspaceRepository, u models.User, regenerate bool, nowUTC time.Time) bool {
	userID := u.ID
	firstName := u.FirstName
	// Compute briefing boundaries in the user's timezone, not the server's.
	timezone, loc := services.ResolveTimezoneOrUTC(u.Timezone)
	nowLocal := nowUTC.In(loc)
	todayStart := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	today := todayStart.Format("2006-01-02")

	// Claim atomically deduplicates this user's daily generation.
	claimed, err := bs.aiRepo.ClaimBriefing(userID, today, nowUTC, regenerate)
	if err != nil {
		if errors.Is(err, repository.ErrBriefingAlreadyRunning) {
			slog.Debug("briefing: already generated or held by another instance",
				slog.Int("user_id", userID), slog.Bool("regenerate", regenerate))
			return true
		}
		slog.Warn("briefing: failed to claim generation lock", slog.Int("user_id", userID), slog.Any("error", err))
		return false
	}
	// Release unpersisted claims on every exit, including panic recovery.
	stored := false
	defer func() {
		if claimed && !stored {
			if relErr := bs.aiRepo.ReleaseBriefingLock(userID, today); relErr != nil {
				slog.Warn("briefing: failed to release generation lock", slog.Int("user_id", userID), slog.Any("error", relErr))
			}
		}
	}()

	start := time.Now()

	// Get accessible workspace IDs (gated-aware item.view check, shared with the
	// HTTP and MCP surfaces via PermissionService).
	accessibleWSIDs, err := bs.permService.AccessibleWorkspaceIDs(userID)
	if err != nil || len(accessibleWSIDs) == 0 {
		slog.Info("briefing: no accessible workspaces",
			slog.Int("user_id", userID),
			slog.Int("workspaces", len(accessibleWSIDs)),
			slog.Any("error", err),
		)
		if err != nil {
			return false
		}
		bs.storeBriefing(userID, today, "", time.Since(start).Milliseconds(), "", accessibleWSIDs)
		stored = true
		return true
	}

	// Gather context: recent activity
	var activityLines []string
	changes, err := itemRepo.RecentItemChanges(accessibleWSIDs, yesterdayStart, 50)
	if err != nil {
		slog.Warn("briefing: changes query failed", slog.Int("user_id", userID), slog.Any("error", err))
	}
	for _, c := range changes {
		displayField := strings.TrimSuffix(c.FieldName, "_id")
		displayOld := resolveLookup(lookups, c.FieldName, c.OldValue)
		displayNew := resolveLookup(lookups, c.FieldName, c.NewValue)
		line := fmt.Sprintf("- [%s] %s: %s changed '%s'", c.ItemKey, c.Title, c.ChangedBy, displayField)
		if displayOld != "" || displayNew != "" {
			line += fmt.Sprintf(" from '%s' to '%s'", displayOld, displayNew)
		}
		activityLines = append(activityLines, line)
	}

	// Gather context: recent comments
	var commentLines []string
	comments, err := itemRepo.RecentComments(accessibleWSIDs, yesterdayStart, 30)
	if err != nil {
		slog.Warn("briefing: comments query failed", slog.Int("user_id", userID), slog.Any("error", err))
	}
	for _, c := range comments {
		content := c.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		commentLines = append(commentLines, fmt.Sprintf("- [%s] %s commented on '%s': %s", c.ItemKey, c.Author, c.Title, content))
	}

	// Gather context: assigned open items, plus everything in the user's personal workspaces
	personalWSIDs, err := workspaceRepo.ListActivePersonalWorkspaceIDs(userID)
	if err != nil {
		slog.Warn("briefing: personal workspaces query failed", slog.Int("user_id", userID), slog.Any("error", err))
		personalWSIDs = nil
	}

	var itemLines []string
	openItems, err := itemRepo.OpenItemsForUser(accessibleWSIDs, personalWSIDs, userID, 50)
	if err != nil {
		slog.Warn("briefing: items query failed", slog.Int("user_id", userID), slog.Any("error", err))
	}
	for _, it := range openItems {
		line := fmt.Sprintf("- [%s-%d] %s", it.WorkspaceKey, it.ItemNumber, it.Title)
		if it.Priority != "" {
			line += fmt.Sprintf(" | Priority: %s", it.Priority)
		}
		if it.DueDate != "" {
			line += fmt.Sprintf(" | Due: %s", it.DueDate)
		} else {
			line += " | Due: none"
		}
		if it.Status != "" {
			line += fmt.Sprintf(" | Status: %s", it.Status)
		}
		if it.MilestoneName != "" {
			ms := fmt.Sprintf(" | Milestone: %s", it.MilestoneName)
			if it.MilestoneTargetDate != "" {
				ms += fmt.Sprintf(" (target: %s)", it.MilestoneTargetDate)
			}
			line += ms
		}
		if it.IterationName != "" {
			iter := fmt.Sprintf(" | Iteration: %s", it.IterationName)
			if it.IterationEndDate != "" {
				iter += fmt.Sprintf(" (ends: %s)", it.IterationEndDate)
			}
			line += iter
		}
		itemLines = append(itemLines, line)
	}

	// Gather context: yesterday's worklogs. time_worklogs.date is INTEGER (Unix
	// epoch), so we need actual instants, not date strings — and they must be
	// anchored at midnight in the *user's* tz, not midnight UTC, otherwise users
	// outside UTC see worklogs from the wrong window (or none at all).
	var worklogLines []string
	if bs.timePermService != nil {
		worklogs, err := repository.NewTimeWorklogRepository(bs.db).ListBriefingWorklogs(userID, yesterdayStart, todayStart)
		if err != nil {
			slog.Warn("briefing: worklogs query failed", slog.Int("user_id", userID), slog.Any("error", err))
		} else {
			for _, worklog := range worklogs {
				worklogLines = append(worklogLines, fmt.Sprintf("- %s (%s): %dm", worklog.Description, worklog.ProjectName, worklog.DurationMinutes))
			}
		}
	}

	// Build the data block with open work first. Recent activity is supporting
	// context, not the briefing's main subject.
	var contextParts []string
	if len(itemLines) > 0 {
		contextParts = append(contextParts, "### Open Items (primary context)\n"+strings.Join(itemLines, "\n"))
	}
	if len(activityLines) > 0 {
		contextParts = append(contextParts, "### Recent Changes (supporting context; do not recap exhaustively)\n"+strings.Join(activityLines, "\n"))
	}
	if len(commentLines) > 0 {
		contextParts = append(contextParts, "### Recent Comments (supporting context; include only when actionable)\n"+strings.Join(commentLines, "\n"))
	}
	if len(worklogLines) > 0 {
		contextParts = append(contextParts, "### Yesterday's Worklogs (background only)\n"+strings.Join(worklogLines, "\n"))
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
		bs.storeBriefing(userID, today, "", time.Since(start).Milliseconds(), "", accessibleWSIDs)
		stored = true
		return true
	}
	systemPrompt := bs.promptStore.Get(llm.PromptDailyBriefing)

	userPrompt := fmt.Sprintf("Good morning %s! Today is %s (%s timezone).\n\nHere is your project data:\n\n%s",
		firstName, nowLocal.Format("Monday, January 2, 2006"), timezone, strings.Join(contextParts, "\n\n"))

	ctx, cancel := context.WithTimeout(context.Background(), llm.DefaultRequestTimeout)
	defer cancel()

	resp, err := llmClient.Complete(ctx, dailyBriefingCompletionRequest(systemPrompt, userPrompt))

	durationMs := time.Since(start).Milliseconds()

	if err != nil || len(resp.Choices) == 0 {
		errMsg := "no response from LLM"
		if err != nil {
			errMsg = err.Error()
		}
		slog.Warn("briefing generation failed", slog.Int("user_id", userID), slog.String("error", errMsg))
		bs.storeBriefing(userID, today, "", durationMs, errMsg, accessibleWSIDs)
		stored = true
		return false
	}

	content := resp.Choices[0].Message.Content
	bs.storeBriefing(userID, today, content, durationMs, "", accessibleWSIDs)
	stored = true

	slog.Info("briefing: generated",
		slog.Int("user_id", userID),
		slog.Int("content_len", len(content)),
		slog.Int64("duration_ms", durationMs),
	)
	return true
}

func dailyBriefingCompletionRequest(systemPrompt, userPrompt string) llm.CompletionRequest {
	return llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		MaxTokens:   dailyBriefingMaxTokens,
	}
}

// resolveLookup returns a human-readable value for the given history field/raw
// value pair, using the centralized id→name maps. Non-*_id fields and
// unparseable values pass through unchanged.
func resolveLookup(maps *repository.NameMaps, field, raw string) string {
	if raw == "" {
		return raw
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return raw
	}
	lookup := func(m map[int]string) string {
		if name, ok := m[id]; ok && name != "" {
			return name
		}
		return fmt.Sprintf("unknown (%d)", id)
	}
	switch field {
	case "status_id":
		return lookup(maps.Statuses)
	case "priority_id":
		return lookup(maps.Priorities)
	case "milestone_id":
		return lookup(maps.Milestones)
	case "iteration_id":
		return lookup(maps.Iterations)
	case "assignee_id", "creator_id":
		return lookup(maps.Users)
	}
	return raw
}

func (bs *BriefingScheduler) storeBriefing(userID int, date, content string, durationMs int64, errMsg string, sourceWorkspaceIDs []int) {
	var errVal any
	if errMsg != "" {
		errVal = errMsg
	}

	// Writing the final result also releases the generation lease: lock_until
	// is cleared alongside the content/error so the row isn't left claimed.
	// This UPSERT covers both the "we claimed the row first" path (UPDATE
	// branch) and a defensive "no row yet" path (INSERT branch).
	sourceJSON, err := json.Marshal(sourceWorkspaceIDs)
	if err != nil {
		slog.Error("failed to encode briefing workspace provenance", slog.Int("user_id", userID), slog.Any("error", err))
		return
	}
	_, err = bs.db.ExecWrite(`INSERT INTO daily_briefings (user_id, date, content, source_workspace_ids, generation_duration_ms, error, lock_until)
		VALUES (?, ?, ?, ?, ?, ?, NULL)
		ON CONFLICT (user_id, date) DO UPDATE SET
		content = excluded.content, source_workspace_ids = excluded.source_workspace_ids,
		generation_duration_ms = excluded.generation_duration_ms,
		error = excluded.error, lock_until = NULL, updated_at = CURRENT_TIMESTAMP`,
		userID, date, content, string(sourceJSON), durationMs, errVal)
	if err != nil {
		slog.Error("failed to store briefing", slog.Int("user_id", userID), slog.Any("error", err))
	}
}
