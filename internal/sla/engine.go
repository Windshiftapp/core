package sla

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"windshift/internal/businesstime"
	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// SLA lifecycle event types. They are appended in the source transaction and
// subscribe nothing in this initiative.
const (
	eventCycleStarted = "sla.cycle_started"
	eventPaused       = "sla.paused"
	eventResumed      = "sla.resumed"
	eventGoalChanged  = "sla.goal_changed"
	eventCompleted    = "sla.completed"
	eventAbandoned    = "sla.cycle_abandoned"
	eventWarning      = "sla.warning"
	eventBreached     = "sla.breached"
)

// Engine evaluates SLA state inline with item facts and repairs itself through
// durable recalculation jobs.
type Engine struct {
	db    database.Database
	repo  *repository.SLARepository
	items *repository.ItemRepository
	store *events.Store

	clock Clock
	cache *configCache
	nudge func(time.Time)
	owner string

	sideEffects    SideEffectEmitter
	inlineObserver InlineObserver
}

// InlineObserver records the cost of inline evaluation for runtime guardrails.
// Implementations must be safe for concurrent use.
type InlineObserver interface {
	ObserveSLAInlineDuration(time.Duration)
}

// NewEngine constructs an SLA engine.
func NewEngine(db database.Database) *Engine {
	return &Engine{
		db:    db,
		repo:  repository.NewSLARepository(db),
		items: repository.NewItemRepository(db),
		store: events.NewStore(db),
		clock: SystemClock{},
		cache: newConfigCache(),
	}
}

// SetClock replaces the engine clock. Tests use it to control effective_at.
func (e *Engine) SetClock(clock Clock) {
	if clock != nil {
		e.clock = clock
	}
}

// SetNudge installs the due-work loop nudge. It is optional.
func (e *Engine) SetNudge(nudge func(time.Time)) { e.nudge = nudge }

// SetInlineObserver installs the inline-evaluation cost observer. It is
// optional; nil disables the guardrail metric.
func (e *Engine) SetInlineObserver(observer InlineObserver) { e.inlineObserver = observer }

// Now returns the engine's current evaluation instant.
func (e *Engine) Now() time.Time { return e.clock.Now() }

// Wake nudges the due-work loop after new work is armed.
func (e *Engine) Wake() {
	if e.nudge != nil {
		e.nudge(e.clock.Now())
	}
}

// InvalidateWorkspace drops cached configuration for a workspace.
func (e *Engine) InvalidateWorkspace(workspaceID int) { e.cache.invalidate(workspaceID) }

type cycleKey struct {
	itemID   int
	metricID int
}

// ObserveItemFacts implements itemevents.FactObserver.
func (e *Engine) ObserveItemFacts(ctx context.Context, tx database.Tx, facts []itemevents.RecordedFact) error {
	started := time.Now()
	evaluated := false
	defer func() {
		if evaluated && e.inlineObserver != nil {
			e.inlineObserver.ObserveSLAInlineDuration(time.Since(started))
		}
	}()
	if len(facts) == 0 {
		return nil
	}
	byWorkspace := map[int][]itemevents.RecordedFact{}
	var order []int
	for _, fact := range facts {
		// Import writes its SLA facts directly; synthetic evaluation would
		// double-count imported cycles.
		if fact.Metadata.ActorKind == "import" {
			continue
		}
		if _, ok := byWorkspace[fact.WorkspaceID]; !ok {
			order = append(order, fact.WorkspaceID)
		}
		byWorkspace[fact.WorkspaceID] = append(byWorkspace[fact.WorkspaceID], fact)
	}
	if len(order) == 0 {
		return nil
	}

	generations, err := e.repo.GenerationsForWorkspacesTx(ctx, tx, order)
	if err != nil {
		// Never block the item write on an SLA read failure.
		slog.Warn("SLA workspace gate failed", slog.String("component", "sla"), slog.Any("error", err))
		return nil
	}

	for _, workspaceID := range order {
		generation, ok := generations[workspaceID]
		if !ok {
			continue
		}
		evaluated = true
		if err := e.observeWorkspace(ctx, tx, workspaceID, generation, byWorkspace[workspaceID]); err != nil {
			slog.Warn("SLA inline evaluation failed; enqueuing repair",
				slog.String("component", "sla"),
				slog.Int("workspace_id", workspaceID),
				slog.Any("error", err))
			e.enqueueRecalcItems(ctx, tx, byWorkspace[workspaceID], e.clock.Now())
		}
	}
	return nil
}

func (e *Engine) observeWorkspace(ctx context.Context, tx database.Tx, workspaceID, generation int, facts []itemevents.RecordedFact) error {
	savepoint := "sla_observe_" + strconv.Itoa(workspaceID)
	if _, err := tx.Exec("SAVEPOINT " + savepoint); err != nil {
		return fmt.Errorf("open SLA savepoint: %w", err)
	}
	err := e.evaluateWorkspace(ctx, tx, workspaceID, generation, facts)
	if err != nil {
		_, _ = tx.Exec("ROLLBACK TO SAVEPOINT " + savepoint)
		_, _ = tx.Exec("RELEASE SAVEPOINT " + savepoint)
		return err
	}
	if _, releaseErr := tx.Exec("RELEASE SAVEPOINT " + savepoint); releaseErr != nil {
		return fmt.Errorf("release SLA savepoint: %w", releaseErr)
	}
	return nil
}

func (e *Engine) evaluateWorkspace(ctx context.Context, tx database.Tx, workspaceID, generation int, facts []itemevents.RecordedFact) error {
	config, err := e.configuration(ctx, workspaceID, generation)
	if err != nil {
		return err
	}
	if len(config.metrics) == 0 {
		return nil
	}

	itemFacts := map[int][]itemevents.RecordedFact{}
	var itemOrder []int
	for _, fact := range facts {
		if _, ok := itemFacts[fact.ItemID]; !ok {
			itemOrder = append(itemOrder, fact.ItemID)
		}
		itemFacts[fact.ItemID] = append(itemFacts[fact.ItemID], fact)
	}

	effectiveAt := e.clock.Now()
	relevantByMetric := make(map[int][]int, len(config.metrics))
	relevantItems := make(map[int]struct{}, len(itemOrder))
	var relevantOrder []int
	for _, metric := range config.metrics {
		relevant := make([]int, 0, len(itemOrder))
		for _, itemID := range itemOrder {
			for _, fact := range itemFacts[itemID] {
				if metric.relevantForFact(fact) {
					relevant = append(relevant, itemID)
					break
				}
			}
		}
		if len(relevant) == 0 {
			continue
		}
		relevantByMetric[metric.id] = relevant
		for _, itemID := range relevant {
			if _, ok := relevantItems[itemID]; !ok {
				relevantItems[itemID] = struct{}{}
				relevantOrder = append(relevantOrder, itemID)
			}
		}
	}
	// Relevance is decided in memory before any per-cycle lock, so a rank drag
	// or other irrelevant write costs only the workspace gate.
	if len(relevantOrder) == 0 {
		return nil
	}

	ongoingList, err := e.repo.LockOngoingCycles(ctx, tx, relevantOrder)
	if err != nil {
		return err
	}
	ongoing := make(map[cycleKey]*models.ItemSLACycle, len(ongoingList))
	for i := range ongoingList {
		cycle := &ongoingList[i]
		ongoing[cycleKey{cycle.ItemID, cycle.MetricID}] = cycle
	}

	goalResults := make(map[int]map[int]goalTarget)
	for _, metric := range config.metrics {
		relevant, ok := relevantByMetric[metric.id]
		if !ok || len(metric.goals) == 0 {
			continue
		}
		resolved, err := e.resolveGoals(ctx, tx, config, metric, relevant, effectiveAt)
		if err != nil {
			return err
		}
		goalResults[metric.id] = resolved
	}

	var earliest *time.Time
	for _, itemID := range itemOrder {
		for _, fact := range itemFacts[itemID] {
			for _, metric := range config.metrics {
				if !metric.relevantForFact(fact) {
					continue
				}
				key := cycleKey{itemID, metric.id}
				target := goalTarget{}
				if results, ok := goalResults[metric.id]; ok {
					target = results[itemID]
				}
				cycle, err := e.applyFact(ctx, tx, config, metric, workspaceID, itemID, fact, ongoing[key], target, effectiveAt)
				if err != nil {
					return err
				}
				if cycle == nil || cycle.Status != models.SLACycleOngoing {
					delete(ongoing, key)
				} else {
					ongoing[key] = cycle
					if cycle.NextDeadlineAt != nil && (earliest == nil || cycle.NextDeadlineAt.Before(*earliest)) {
						deadline := *cycle.NextDeadlineAt
						earliest = &deadline
					}
				}
			}
		}
	}
	if earliest != nil && e.nudge != nil {
		e.nudge(*earliest)
	}
	return nil
}

func (e *Engine) ValidateGoalQL(ctx context.Context, workspaceID int, query string) error {
	ast, err := parseQLL(query)
	if err != nil {
		return err
	}
	workspaceKey, err := e.repo.WorkspaceKey(ctx, workspaceID)
	if err != nil {
		return err
	}
	customFields, err := e.items.GetCQLCustomFieldMapContext(ctx)
	if err != nil {
		return err
	}
	workspaceMap := map[string]int{}
	if workspaceKey != "" {
		workspaceMap[strings.ToLower(workspaceKey)] = workspaceID
	}
	generator := cql.NewSQLGenerator(workspaceMap, customFields, e.db.GetDriverName())
	generator.EnableLegacyCustomFieldNameFallback()
	_, _, err = generator.GenerateSQLAt(ast, e.clock.Now())
	return err
}

func (e *Engine) configuration(ctx context.Context, workspaceID, generation int) (*compiledConfig, error) {
	if config, ok := e.cache.get(workspaceID, generation); ok {
		return config, nil
	}
	config, err := e.loadConfig(ctx, workspaceID, generation)
	if err != nil {
		return nil, err
	}
	e.cache.put(config)
	return config, nil
}

func (e *Engine) applyFact(ctx context.Context, tx database.Tx, config *compiledConfig, metric *compiledMetric, workspaceID, itemID int, fact itemevents.RecordedFact, cycle *models.ItemSLACycle, target goalTarget, effectiveAt time.Time) (*models.ItemSLACycle, error) {
	stop := metric.matchPhase(models.SLAPhaseStop, fact, config)
	pause := metric.matchPhase(models.SLAPhasePause, fact, config)
	start := metric.matchPhase(models.SLAPhaseStart, fact, config)

	if cycle == nil {
		if start && !stop {
			return e.startCycle(ctx, tx, config, metric, workspaceID, itemID, target, pause, effectiveAt, models.SLAOriginNative)
		}
		return nil, nil
	}

	effective := effectiveAt
	if cycle.LastCalculatedAt.After(effective) {
		effective = cycle.LastCalculatedAt
	}
	if stop {
		if err := e.completeCycle(ctx, tx, config, workspaceID, cycle, effective); err != nil {
			return nil, err
		}
		return nil, nil
	}

	if err := e.advanceCycle(config, cycle, effective); err != nil {
		return nil, err
	}

	switch {
	case cycle.Paused && metric.pauseRelevant(fact) && !pause:
		if err := e.resumeCycle(ctx, tx, config, metric, workspaceID, cycle, target, effective); err != nil {
			return nil, err
		}
	case !cycle.Paused && metric.pauseRelevant(fact) && pause:
		if err := e.pauseCycle(ctx, tx, workspaceID, cycle, effective); err != nil {
			return nil, err
		}
	}

	if !cycle.Paused && metric.goalInputsChanged(fact) {
		if err := e.reGoalCycle(ctx, tx, config, metric, workspaceID, cycle, target, effective); err != nil {
			return nil, err
		}
	}

	if err := e.repo.UpdateCycle(ctx, tx, cycle); err != nil {
		return nil, err
	}
	return cycle, nil
}

func (e *Engine) startCycle(ctx context.Context, tx database.Tx, config *compiledConfig, metric *compiledMetric, workspaceID, itemID int, target goalTarget, paused bool, effective time.Time, origin string) (*models.ItemSLACycle, error) {
	cycleNo, err := e.repo.NextCycleNo(ctx, tx, itemID, metric.id)
	if err != nil {
		return nil, err
	}
	cycle := &models.ItemSLACycle{
		ItemID:            itemID,
		MetricID:          metric.id,
		CycleNo:           cycleNo,
		Status:            models.SLACycleOngoing,
		StartedAt:         effective,
		LastCalculatedAt:  effective,
		Origin:            origin,
		GoalQuerySnapshot: target.query,
		CalendarSnapshot:  defaultCalendarSnapshot(),
	}
	e.applyTarget(cycle, config, target, effective)

	if paused {
		cycle.Paused = true
		cycle.PauseStartedAt = &effective
		cycle.NextDeadlineAt = nil
		cycle.BreachTime = nil
		cycle.RemainingAtPauseMs = &cycle.RemainingMs
	}

	id, err := e.repo.InsertCycle(ctx, tx, cycle)
	if err != nil {
		return nil, err
	}
	cycle.ID = id

	if err := e.appendLifecycle(ctx, tx, workspaceID, cycle, eventCycleStarted, effective); err != nil {
		return nil, err
	}
	if cycle.Paused {
		if err := e.appendLifecycle(ctx, tx, workspaceID, cycle, eventPaused, effective); err != nil {
			return nil, err
		}
		return cycle, nil
	}
	if err := e.syncCycleJobs(ctx, tx, config, metric, cycle, effective); err != nil {
		return nil, err
	}
	return cycle, nil
}

func (e *Engine) completeCycle(ctx context.Context, tx database.Tx, config *compiledConfig, workspaceID int, cycle *models.ItemSLACycle, effective time.Time) error {
	if err := e.advanceCycle(config, cycle, effective); err != nil {
		return err
	}
	cycle.Status = models.SLACycleCompleted
	stopped := effective
	cycle.StoppedAt = &stopped
	cycle.Paused = false
	cycle.PauseStartedAt = nil
	cycle.RemainingAtPauseMs = nil
	cycle.NextDeadlineAt = nil

	if cycle.GoalID != nil {
		remaining := cycle.GoalDurationMs - cycle.ElapsedMs
		cycle.RemainingMs = remaining
		if remaining <= 0 {
			breachedAt := effective
			if cycle.BreachTime != nil {
				breachedAt = *cycle.BreachTime
			}
			if cycle.BreachedAt == nil {
				cycle.BreachedAt = &breachedAt
				if err := e.appendLifecycle(ctx, tx, workspaceID, cycle, eventBreached, breachedAt); err != nil {
					return err
				}
				// A completion that crosses the deadline records the first
				// breach, so it must request the same notification/action
				// side effects a due breach job would have delivered.
				if e.sideEffects != nil {
					if err := e.sideEffects.EmitBreach(ctx, tx, cycle); err != nil {
						return err
					}
				}
			}
		}
	} else {
		cycle.RemainingMs = 0
	}

	if err := e.repo.DeleteJobsForCycle(ctx, tx, cycle.ID); err != nil {
		return err
	}
	if err := e.repo.UpdateCycle(ctx, tx, cycle); err != nil {
		return err
	}
	return e.appendLifecycle(ctx, tx, workspaceID, cycle, eventCompleted, effective)
}

func (e *Engine) pauseCycle(ctx context.Context, tx database.Tx, workspaceID int, cycle *models.ItemSLACycle, effective time.Time) error {
	if err := e.recordOverdueBreach(ctx, tx, workspaceID, cycle, effective); err != nil {
		return err
	}
	cycle.Paused = true
	pausedAt := effective
	cycle.PauseStartedAt = &pausedAt
	remaining := cycle.GoalDurationMs - cycle.ElapsedMs
	if cycle.GoalID != nil {
		cycle.RemainingAtPauseMs = &remaining
	}
	cycle.NextDeadlineAt = nil
	if err := e.repo.DeleteJobsForCycle(ctx, tx, cycle.ID); err != nil {
		return err
	}
	return e.appendLifecycle(ctx, tx, workspaceID, cycle, eventPaused, effective)
}

// recordOverdueBreach records an armed deadline that has already passed before
// a pause or target change can clear it. It mirrors the breach job's
// first-breach bookkeeping and side effects, so the original breach survives
// whether the worker or the mutation wins the race. Both paths lock the cycle
// row, and only a nil BreachedAt emits, so the side effects fire exactly once.
func (e *Engine) recordOverdueBreach(ctx context.Context, tx database.Tx, workspaceID int, cycle *models.ItemSLACycle, effective time.Time) error {
	if cycle.Status != models.SLACycleOngoing || cycle.Paused || cycle.GoalID == nil || cycle.GoalDurationMs <= 0 {
		return nil
	}
	if cycle.BreachedAt != nil || cycle.NextDeadlineAt == nil {
		return nil
	}
	deadline := *cycle.NextDeadlineAt
	if deadline.After(effective) {
		return nil
	}
	cycle.BreachedAt = &deadline
	cycle.NextDeadlineAt = nil
	cycle.RemainingMs = cycle.GoalDurationMs - cycle.ElapsedMs
	if err := e.repo.UpdateCycle(ctx, tx, cycle); err != nil {
		return err
	}
	if err := e.repo.DeleteJobsForCycleKind(ctx, tx, cycle.ID, models.SLAJobWarning); err != nil {
		return err
	}
	if err := e.appendLifecycle(ctx, tx, workspaceID, cycle, eventBreached, deadline); err != nil {
		return err
	}
	if e.sideEffects != nil {
		if err := e.sideEffects.EmitBreach(ctx, tx, cycle); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) resumeCycle(ctx context.Context, tx database.Tx, config *compiledConfig, metric *compiledMetric, workspaceID int, cycle *models.ItemSLACycle, target goalTarget, effective time.Time) error {
	cycle.Paused = false
	cycle.PauseStartedAt = nil
	cycle.RemainingAtPauseMs = nil
	e.armDeadline(config, cycle, effective)
	if err := e.reGoalCycle(ctx, tx, config, metric, workspaceID, cycle, target, effective); err != nil {
		return err
	}
	if err := e.repo.UpdateCycle(ctx, tx, cycle); err != nil {
		return err
	}
	if err := e.syncCycleJobs(ctx, tx, config, metric, cycle, effective); err != nil {
		return err
	}
	return e.appendLifecycle(ctx, tx, workspaceID, cycle, eventResumed, effective)
}

// reGoalCycle carries elapsed time into a newly matched goal and recomputes the
// deadline ("freeze and forward"). Completed cycles are never regoaled.
func (e *Engine) reGoalCycle(ctx context.Context, tx database.Tx, config *compiledConfig, metric *compiledMetric, workspaceID int, cycle *models.ItemSLACycle, target goalTarget, effective time.Time) error {
	if err := e.recordOverdueBreach(ctx, tx, workspaceID, cycle, effective); err != nil {
		return err
	}
	newGoalID := (*int)(nil)
	newDurationMs := int64(0)
	newCalendarID := (*int)(nil)
	newCalendarSnapshot := json.RawMessage(nil)
	if target.matched && target.target != nil {
		goalID := target.goalID
		calendarID := target.target.CalendarID
		newGoalID = &goalID
		newCalendarID = &calendarID
		newDurationMs = target.target.TargetMs
		if calendar := config.calendars[calendarID]; calendar != nil {
			if encoded, err := json.Marshal(calendar.raw); err == nil {
				newCalendarSnapshot = encoded
			}
		}
	}
	changed := !equalOptionalInt(cycle.GoalID, newGoalID) || target.query != cycle.GoalQuerySnapshot || cycle.GoalDurationMs != newDurationMs || !equalOptionalInt(cycle.CalendarID, newCalendarID)
	if len(newCalendarSnapshot) > 0 && string(newCalendarSnapshot) != string(cycle.CalendarSnapshot) {
		changed = true
	}
	if !changed {
		return nil
	}
	e.applyTarget(cycle, config, target, effective)
	cycle.GoalQuerySnapshot = target.query
	if err := e.syncCycleJobs(ctx, tx, config, metric, cycle, effective); err != nil {
		return err
	}
	return e.appendLifecycle(ctx, tx, workspaceID, cycle, eventGoalChanged, effective)
}

// applyTarget sets the cycle's goal, calendar, duration, remaining, and
// deadline from a resolved target. breached_at is first-breach history: a
// re-goal that grants fresh headroom (or drops the goal entirely) never
// erases it, so compliance counts and lifecycle events stay consistent. The
// fresh chance is the newly armed deadline; a second breach appends its own
// lifecycle event.
func (e *Engine) applyTarget(cycle *models.ItemSLACycle, config *compiledConfig, target goalTarget, effective time.Time) {
	if !target.matched || target.target == nil {
		cycle.GoalID = nil
		cycle.CalendarID = nil
		cycle.GoalDurationMs = 0
		cycle.RemainingMs = 0
		cycle.NextDeadlineAt = nil
		cycle.BreachTime = nil
		cycle.CalendarSnapshot = defaultCalendarSnapshot()
		cycle.WithinCalendarHours = false
		cycle.GoalQuerySnapshot = target.query
		return
	}
	goalID := target.goalID
	calendarID := target.target.CalendarID
	cycle.GoalID = &goalID
	cycle.CalendarID = &calendarID
	cycle.GoalDurationMs = target.target.TargetMs
	cycle.GoalQuerySnapshot = target.query
	if calendar := config.calendars[calendarID]; calendar != nil {
		if encoded, err := json.Marshal(calendar.raw); err == nil {
			cycle.CalendarSnapshot = encoded
		}
		cycle.WithinCalendarHours = calendar.compiled.WithinCalendarHours(effective)
	}
	e.armDeadline(config, cycle, effective)
}

// armDeadline computes next_deadline_at from the remaining business time.
func (e *Engine) armDeadline(config *compiledConfig, cycle *models.ItemSLACycle, effective time.Time) {
	cycle.NextDeadlineAt = nil
	cycle.BreachTime = nil
	if cycle.GoalID == nil {
		cycle.RemainingMs = 0
		return
	}
	remaining := time.Duration(cycle.GoalDurationMs-cycle.ElapsedMs) * time.Millisecond
	if remaining < 0 {
		remaining = 0
	}
	cycle.RemainingMs = remaining.Milliseconds()
	calendar, err := calendarForCycle(config, cycle)
	if err != nil || calendar == nil {
		// Without a usable calendar the deadline is the effective instant, so
		// the breach still fires rather than silently disappearing.
		deadline := effective
		cycle.NextDeadlineAt = &deadline
		cycle.BreachTime = &deadline
		return
	}
	if remaining == 0 {
		deadline := effective
		cycle.NextDeadlineAt = &deadline
		cycle.BreachTime = &deadline
		return
	}
	deadline, ok := calendar.AddCalendarTime(effective, remaining)
	if !ok {
		return
	}
	cycle.NextDeadlineAt = &deadline
	cycle.BreachTime = &deadline
}

func (e *Engine) advanceCycle(config *compiledConfig, cycle *models.ItemSLACycle, effective time.Time) error {
	if cycle.Paused || cycle.Status != models.SLACycleOngoing {
		cycle.LastCalculatedAt = effective
		return nil
	}
	if !effective.After(cycle.LastCalculatedAt) {
		return nil
	}
	// A cycle without a matched goal carries the synthetic 24/7 snapshot. It has
	// no target to measure against, so it must not accrue wall-clock time that a
	// goal matched later would inherit and immediately breach.
	if cycle.GoalDurationMs <= 0 {
		cycle.LastCalculatedAt = effective
		return nil
	}
	calendar, err := calendarForCycle(config, cycle)
	if err != nil {
		return err
	}
	if calendar != nil {
		cycle.ElapsedMs += calendar.ElapsedCalendarTime(cycle.LastCalculatedAt, effective).Milliseconds()
	}
	cycle.LastCalculatedAt = effective
	return nil
}

func (e *Engine) armBreachJob(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle) error {
	if cycle.NextDeadlineAt == nil {
		return nil
	}
	deadline := *cycle.NextDeadlineAt
	cycleID := cycle.ID
	return e.repo.UpsertJob(ctx, tx, &models.SLAJob{
		Kind:       models.SLAJobBreach,
		CycleID:    &cycleID,
		DueAt:      deadline,
		DeadlineAt: &deadline,
	})
}

// syncCycleJobs keeps the cycle's durable jobs in lockstep with its state:
// a breach job at the deadline plus one warning job per configured threshold,
// or nothing when the cycle is paused, closed, or has no goal.
func (e *Engine) syncCycleJobs(ctx context.Context, tx database.Tx, config *compiledConfig, metric *compiledMetric, cycle *models.ItemSLACycle, effective time.Time) error {
	if cycle.Status != models.SLACycleOngoing || cycle.Paused || cycle.GoalID == nil || cycle.NextDeadlineAt == nil {
		return e.repo.DeleteJobsForCycle(ctx, tx, cycle.ID)
	}
	if err := e.armBreachJob(ctx, tx, cycle); err != nil {
		return err
	}
	return e.armWarningJobs(ctx, tx, config, metric, cycle, effective)
}

// armWarningJobs replaces the cycle's warning jobs with the configured
// thresholds. A warning is due when the configured percentage of the goal has
// elapsed; warnings at or after the breach deadline are skipped.
func (e *Engine) armWarningJobs(ctx context.Context, tx database.Tx, config *compiledConfig, metric *compiledMetric, cycle *models.ItemSLACycle, effective time.Time) error {
	if err := e.repo.DeleteJobsForCycleKind(ctx, tx, cycle.ID, models.SLAJobWarning); err != nil {
		return err
	}
	if cycle.NextDeadlineAt == nil || cycle.GoalDurationMs <= 0 {
		return nil
	}
	thresholds := config.thresholdsFor(metric.id)
	if len(thresholds) == 0 {
		return nil
	}
	calendar, err := calendarForCycle(config, cycle)
	if err != nil || calendar == nil {
		return err
	}
	deadline := *cycle.NextDeadlineAt
	for _, threshold := range thresholds {
		targetMs := int64(threshold.percent) * cycle.GoalDurationMs / 100
		remainingMs := targetMs - cycle.ElapsedMs
		dueAt := effective
		if remainingMs > 0 {
			computed, ok := calendar.AddCalendarTime(effective, time.Duration(remainingMs)*time.Millisecond)
			if !ok {
				continue
			}
			dueAt = computed
		}
		if !dueAt.Before(deadline) {
			continue
		}
		if err := e.repo.UpsertJob(ctx, tx, &models.SLAJob{
			Kind:         models.SLAJobWarning,
			ThresholdKey: threshold.key,
			CycleID:      &cycle.ID,
			DueAt:        dueAt,
			DeadlineAt:   &deadline,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) appendLifecycle(ctx context.Context, tx database.Tx, workspaceID int, cycle *models.ItemSLACycle, eventType string, occurredAt time.Time) error {
	payload := map[string]any{
		"cycle_id":   cycle.ID,
		"item_id":    cycle.ItemID,
		"metric_id":  cycle.MetricID,
		"cycle_no":   cycle.CycleNo,
		"status":     cycle.Status,
		"elapsed_ms": cycle.ElapsedMs,
		"breached":   cycle.BreachedAt != nil,
	}
	if cycle.GoalID != nil {
		payload["goal_id"] = *cycle.GoalID
	}
	if cycle.NextDeadlineAt != nil {
		payload["next_deadline_at"] = cycle.NextDeadlineAt.UTC().Format(time.RFC3339)
	}
	if cycle.StoppedAt != nil {
		payload["stopped_at"] = cycle.StoppedAt.UTC().Format(time.RFC3339)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s payload: %w", eventType, err)
	}
	var workspace *int
	if workspaceID != 0 {
		value := workspaceID
		workspace = &value
	}
	_, err = e.store.Append(ctx, tx, events.NewEvent{
		WorkspaceID:    workspace,
		AggregateType:  "sla_cycle",
		AggregateID:    strconv.FormatInt(cycle.ID, 10),
		Type:           eventType,
		PayloadVersion: 1,
		OccurredAt:     occurredAt,
		ActorKind:      "system",
		SourceKind:     "sla",
		Payload:        encoded,
	})
	if err != nil {
		return fmt.Errorf("append %s for cycle %d: %w", eventType, cycle.ID, err)
	}
	return nil
}

// workspaceForItemTx resolves an item's workspace for lifecycle events. A
// missing item yields 0, which records the event without a workspace scope.
func (e *Engine) workspaceForItemTx(ctx context.Context, tx database.Tx, itemID int) int {
	var workspaceID int
	if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM items WHERE id = ?`, itemID).Scan(&workspaceID); err != nil {
		return 0
	}
	return workspaceID
}

func (e *Engine) enqueueRecalcItems(ctx context.Context, tx database.Tx, facts []itemevents.RecordedFact, now time.Time) {
	seen := map[int]struct{}{}
	for _, fact := range facts {
		if fact.ItemID == 0 {
			continue
		}
		if _, ok := seen[fact.ItemID]; ok {
			continue
		}
		seen[fact.ItemID] = struct{}{}
		itemID := fact.ItemID
		if err := e.repo.UpsertJob(ctx, tx, &models.SLAJob{Kind: models.SLAJobRecalcItem, ItemID: &itemID, DueAt: now}); err != nil {
			slog.Error("SLA failed to enqueue repair job", slog.String("component", "sla"), slog.Int("item_id", itemID), slog.Any("error", err))
		}
	}
	if len(seen) > 0 && e.nudge != nil {
		e.nudge(now)
	}
}

func defaultCalendarSnapshot() json.RawMessage {
	encoded, err := json.Marshal(businesstime.AlwaysOpenRaw("UTC"))
	if err != nil {
		return json.RawMessage(`{"timezone":"UTC"}`)
	}
	return encoded
}

func calendarForCycle(config *compiledConfig, cycle *models.ItemSLACycle) (*businesstime.Calendar, error) {
	if len(cycle.CalendarSnapshot) > 0 {
		calendar, err := businesstime.CompileJSON(string(cycle.CalendarSnapshot))
		if err == nil {
			return calendar, nil
		}
		return nil, err
	}
	if cycle.CalendarID != nil && config != nil {
		if compiled, ok := config.calendars[*cycle.CalendarID]; ok {
			return compiled.compiled, nil
		}
	}
	return nil, nil
}

func equalOptionalInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
