package scheduler

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/plugins"
	"windshift/internal/repository"
)

// pluginScheduleSchedulerName is the value persisted in scheduler_runs.scheduler_name
// for every tick of the plugin schedule scheduler. Keeping it as a package
// constant means admin diagnostics queries can pin this string without
// duplicating it.
const pluginScheduleSchedulerName = "plugin_schedules"

// defaultPluginScheduleInterval is how often the tick scans the registry for
// due schedules. Production default; tests override via the constructor.
const defaultPluginScheduleInterval = 30 * time.Second

// SchedulePluginInvoker is the subset of plugins.Manager that this scheduler
// depends on. Exposed as an interface to let tests substitute a stub that
// records fires without spinning up an Extism runtime.
type SchedulePluginInvoker interface {
	DueSchedules(now time.Time) []plugins.DueSchedule
	CallPluginFunction(pluginName, funcName string, payload any) ([]byte, error)
}

// PluginScheduleScheduler is the bridge between the in-memory schedule
// registry on plugins.Manager and the existing scheduler_runs observability
// surface. On each tick it asks the manager which schedules are due and fires
// each via Manager.CallPluginFunction. One scheduler_runs row is written per
// tick (covering all fires in that tick); each individual fire emits a
// structured slog line.
//
// There is intentionally NO per-fire DB row in v1. Use slog (or the future
// plugin_schedule_runs table) when admins demand a queryable per-fire UI.
type PluginScheduleScheduler struct {
	manager  SchedulePluginInvoker
	runRepo  *repository.SchedulerRunRepository
	interval time.Duration

	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
}

// NewPluginScheduleScheduler wires the scheduler to its production manager
// and DB. The tick interval defaults to 30 seconds — see
// NewPluginScheduleSchedulerWithInterval for the test-only knob.
func NewPluginScheduleScheduler(manager SchedulePluginInvoker, db database.Database) *PluginScheduleScheduler {
	return NewPluginScheduleSchedulerWithInterval(manager, db, defaultPluginScheduleInterval)
}

// NewPluginScheduleSchedulerWithInterval is the same as NewPluginScheduleScheduler
// but lets the caller pick the tick interval. Intended for tests that want
// sub-second cadence; production code should use the default constructor.
func NewPluginScheduleSchedulerWithInterval(manager SchedulePluginInvoker, db database.Database, interval time.Duration) *PluginScheduleScheduler {
	return &PluginScheduleScheduler{
		manager:  manager,
		runRepo:  repository.NewSchedulerRunRepository(db),
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start begins the scheduler loop. Calling Start twice is a no-op; calling
// Start after Stop reuses the same instance is NOT supported (the stop
// channel is closed and would re-trigger shutdown).
func (s *PluginScheduleScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}
	s.running = true
	slog.Info("Starting plugin schedule scheduler", "interval", s.interval)
	go s.loop()
}

// Stop signals the scheduler loop to exit. Idempotent for the
// already-not-running case.
func (s *PluginScheduleScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}
	s.running = false
	close(s.stopChan)
	slog.Info("Plugin schedule scheduler stopped")
}

// loop runs the periodic tick. We fire once immediately on start (consistent
// with the other 5 schedulers) so plugins that just declared a schedule don't
// have to wait a full interval to be invoked once after server startup.
func (s *PluginScheduleScheduler) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.processTick()

	for {
		select {
		case <-ticker.C:
			s.processTick()
		case <-s.stopChan:
			return
		}
	}
}

// processTick claims all currently-due schedules and fires each one
// sequentially. Sequential firing keeps the implementation simple; with the
// default 30 s tick and 5 s Extism timeout, up to ~6 fires per tick fit in
// budget. If a plugin handler hangs near the timeout we just slow down the
// tick — we never lose schedules because LastFired is already advanced by
// DueSchedules before any invocation runs.
func (s *PluginScheduleScheduler) processTick() {
	start := time.Now()
	itemsProcessed := 0
	var runErr error
	defer recordSchedulerRun(s.runRepo, pluginScheduleSchedulerName, start, &itemsProcessed, &runErr)

	due := s.manager.DueSchedules(start)
	if len(due) == 0 {
		return
	}

	failures := 0
	for _, d := range due {
		fireStart := time.Now()
		_, err := s.manager.CallPluginFunction(d.PluginName, d.Handler, newSchedulePayload(d, fireStart))
		duration := time.Since(fireStart)

		if err != nil {
			failures++
			slog.Warn("plugin_schedule_fire failed",
				"plugin", d.PluginName,
				"schedule_id", d.ScheduleID,
				"handler", d.Handler,
				"duration_ms", duration.Milliseconds(),
				"error", err,
			)
			continue
		}

		slog.Info("plugin_schedule_fire",
			"plugin", d.PluginName,
			"schedule_id", d.ScheduleID,
			"handler", d.Handler,
			"duration_ms", duration.Milliseconds(),
		)
	}

	itemsProcessed = len(due)
	if failures > 0 {
		runErr = fmt.Errorf("%d of %d plugin schedule fires failed", failures, len(due))
	}
}

// schedulePayload is the JSON-serializable envelope passed to the WASM handler.
// Stable shape — extending it is fine, renaming fields is breaking. Plugin
// developers can ignore the payload entirely if they only care about being
// woken up.
type schedulePayload struct {
	ScheduleID string `json:"schedule_id"`
	PluginName string `json:"plugin_name"`
	FireTime   string `json:"fire_time"` // RFC3339Nano
}

func newSchedulePayload(d plugins.DueSchedule, fireTime time.Time) schedulePayload {
	return schedulePayload{
		ScheduleID: d.ScheduleID,
		PluginName: d.PluginName,
		FireTime:   fireTime.Format(time.RFC3339Nano),
	}
}
