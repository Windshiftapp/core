package scheduler

import (
	"errors"
	"sync"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/plugins"
)

// stubInvoker is a SchedulePluginInvoker that returns a fixed set of due
// schedules and records every CallPluginFunction invocation. Used to drive
// PluginScheduleScheduler tests without spinning up an Extism runtime.
type stubInvoker struct {
	mu          sync.Mutex
	due         []plugins.DueSchedule
	callErr     error
	calls       int
	plugins     []string
	handlers    []string
}

func (s *stubInvoker) DueSchedules(time.Time) []plugins.DueSchedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Return a copy so consumers can't mutate our slice. After returning,
	// clear `due` so subsequent ticks see nothing — mimicking the real
	// Manager's atomic claim semantics.
	out := append([]plugins.DueSchedule(nil), s.due...)
	s.due = nil
	return out
}

func (s *stubInvoker) CallPluginFunction(pluginName, funcName string, _ any) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.plugins = append(s.plugins, pluginName)
	s.handlers = append(s.handlers, funcName)
	return nil, s.callErr
}

func (s *stubInvoker) queueDue(d ...plugins.DueSchedule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.due = append(s.due, d...)
}

func (s *stubInvoker) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// newSchedulerRunsDB returns an in-memory SQLite DB with just the
// scheduler_runs table created — enough for SchedulerRunRepository.Insert.
func newSchedulerRunsDB(t *testing.T) database.Database {
	t.Helper()
	dsn := "file:scheduler_test_" + t.Name() + "?mode=memory&cache=shared"
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE scheduler_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		scheduler_name TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		completed_at DATETIME,
		duration_ms INTEGER,
		items_processed INTEGER,
		success BOOLEAN NOT NULL DEFAULT 0,
		error_message TEXT
	)`); err != nil {
		t.Fatalf("create scheduler_runs: %v", err)
	}
	return db
}

// countSchedulerRuns returns the number of scheduler_runs rows for the named
// scheduler. Used to assert that processTick records its outcome regardless
// of fire success.
func countSchedulerRuns(t *testing.T, db database.Database, name string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM scheduler_runs WHERE scheduler_name = ?", name).Scan(&n); err != nil {
		t.Fatalf("count scheduler_runs: %v", err)
	}
	return n
}

func TestPluginScheduleScheduler_FiresQueuedSchedules(t *testing.T) {
	db := newSchedulerRunsDB(t)
	stub := &stubInvoker{}
	stub.queueDue(plugins.DueSchedule{PluginName: "p", ScheduleID: "drain", Handler: "on_tick"})

	s := NewPluginScheduleSchedulerWithInterval(stub, db, 5*time.Millisecond)
	s.Start()
	defer s.Stop()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if stub.callCount() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := stub.callCount(); got < 1 {
		t.Fatalf("CallPluginFunction call count = %d, want >= 1", got)
	}

	if stub.plugins[0] != "p" || stub.handlers[0] != "on_tick" {
		t.Errorf("call args = (%q, %q), want (\"p\", \"on_tick\")", stub.plugins[0], stub.handlers[0])
	}

	if got := countSchedulerRuns(t, db, pluginScheduleSchedulerName); got < 1 {
		t.Errorf("scheduler_runs rows = %d, want >= 1", got)
	}
}

func TestPluginScheduleScheduler_FailingFireDoesNotCrashOrBlock(t *testing.T) {
	db := newSchedulerRunsDB(t)
	stub := &stubInvoker{callErr: errors.New("synthetic failure")}
	stub.queueDue(
		plugins.DueSchedule{PluginName: "p", ScheduleID: "a", Handler: "h"},
		plugins.DueSchedule{PluginName: "p", ScheduleID: "b", Handler: "h"},
	)

	s := NewPluginScheduleSchedulerWithInterval(stub, db, 5*time.Millisecond)
	s.Start()
	defer s.Stop()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if stub.callCount() >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Both queued entries must have been attempted even though the first one
	// errored — a failing fire must not abort the tick.
	if got := stub.callCount(); got < 2 {
		t.Fatalf("CallPluginFunction call count = %d, want >= 2 (both fires must be attempted)", got)
	}

	// scheduler_runs row must still be written for the failing tick.
	if got := countSchedulerRuns(t, db, pluginScheduleSchedulerName); got < 1 {
		t.Errorf("scheduler_runs rows after failing fires = %d, want >= 1", got)
	}
}

func TestPluginScheduleScheduler_StartStopIdempotent(t *testing.T) {
	db := newSchedulerRunsDB(t)
	stub := &stubInvoker{}

	s := NewPluginScheduleSchedulerWithInterval(stub, db, time.Hour)
	s.Start()
	s.Start() // must be a no-op
	s.Stop()
	s.Stop() // must be a no-op (no panic)
}

func TestPluginScheduleScheduler_NoDueNoCall(t *testing.T) {
	db := newSchedulerRunsDB(t)
	stub := &stubInvoker{} // queueDue NOT called

	s := NewPluginScheduleSchedulerWithInterval(stub, db, 5*time.Millisecond)
	s.Start()
	time.Sleep(30 * time.Millisecond)
	s.Stop()

	if got := stub.callCount(); got != 0 {
		t.Errorf("CallPluginFunction called %d times with no due schedules, want 0", got)
	}
}
