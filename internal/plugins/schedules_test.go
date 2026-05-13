//go:build !noplugins

package plugins

import (
	"errors"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{
		schedules: make(map[string][]*scheduledPlugin),
	}
}

func TestParsePluginSchedules_Valid(t *testing.T) {
	parsed, err := parsePluginSchedules([]PluginSchedule{
		{ID: "drain", Every: "5m", Handler: "on_drain"},
		{ID: "ping", Every: "30s", Handler: "on_ping"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("got %d parsed entries, want 2", len(parsed))
	}
	if parsed[0].Every != 5*time.Minute {
		t.Errorf("entry 0 Every = %v, want 5m", parsed[0].Every)
	}
	if parsed[1].Every != 30*time.Second {
		t.Errorf("entry 1 Every = %v, want 30s", parsed[1].Every)
	}
}

func TestParsePluginSchedules_Invalid(t *testing.T) {
	tests := []struct {
		name      string
		schedules []PluginSchedule
	}{
		{"empty id", []PluginSchedule{{ID: "", Every: "5m", Handler: "h"}}},
		{"empty handler", []PluginSchedule{{ID: "x", Every: "5m", Handler: ""}}},
		{"unparseable every", []PluginSchedule{{ID: "x", Every: "huh?", Handler: "h"}}},
		{"zero every", []PluginSchedule{{ID: "x", Every: "0s", Handler: "h"}}},
		{"negative every", []PluginSchedule{{ID: "x", Every: "-1s", Handler: "h"}}},
		{"duplicate id within plugin", []PluginSchedule{
			{ID: "dup", Every: "5m", Handler: "a"},
			{ID: "dup", Every: "10m", Handler: "b"},
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePluginSchedules(tc.schedules)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidSchedule) {
				t.Errorf("error %v is not ErrInvalidSchedule", err)
			}
		})
	}
}

func TestRegisterSchedules_ReplacesExisting(t *testing.T) {
	m := newTestManager(t)

	if err := m.registerSchedules("p", []PluginSchedule{
		{ID: "old", Every: "1s", Handler: "h"},
	}); err != nil {
		t.Fatalf("first register: %v", err)
	}

	// Re-register with a different schedule set — the old entry must disappear.
	if err := m.registerSchedules("p", []PluginSchedule{
		{ID: "new", Every: "1s", Handler: "h"},
	}); err != nil {
		t.Fatalf("second register: %v", err)
	}

	due := m.DueSchedules(time.Now().Add(2 * time.Second))
	if len(due) != 1 {
		t.Fatalf("got %d due entries, want 1", len(due))
	}
	if due[0].ScheduleID != "new" {
		t.Errorf("ScheduleID = %q, want %q (old schedule should have been replaced)", due[0].ScheduleID, "new")
	}
}

func TestRegisterSchedules_EmptyClearsExisting(t *testing.T) {
	m := newTestManager(t)
	if err := m.registerSchedules("p", []PluginSchedule{
		{ID: "x", Every: "1s", Handler: "h"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Re-register with an empty schedule list — should clear the registry for p.
	if err := m.registerSchedules("p", nil); err != nil {
		t.Fatalf("clear-register: %v", err)
	}

	due := m.DueSchedules(time.Now().Add(time.Hour))
	if len(due) != 0 {
		t.Errorf("got %d due entries, want 0 after clearing", len(due))
	}
}

func TestUnregisterSchedules(t *testing.T) {
	m := newTestManager(t)
	if err := m.registerSchedules("p", []PluginSchedule{
		{ID: "x", Every: "1s", Handler: "h"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	m.unregisterSchedules("p")

	due := m.DueSchedules(time.Now().Add(time.Hour))
	if len(due) != 0 {
		t.Errorf("got %d due entries after unregister, want 0", len(due))
	}

	// Unregistering an unknown plugin must be a no-op, not a panic.
	m.unregisterSchedules("never-registered")
}

func TestDueSchedules_NotDueImmediately(t *testing.T) {
	m := newTestManager(t)
	if err := m.registerSchedules("p", []PluginSchedule{
		{ID: "x", Every: "1h", Handler: "h"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Immediately after registration, nothing should be due — LastFired was
	// seeded to "now" so the first fire happens one Every later.
	due := m.DueSchedules(time.Now())
	if len(due) != 0 {
		t.Errorf("got %d due entries immediately after register, want 0", len(due))
	}
}

func TestDueSchedules_AtomicDedup(t *testing.T) {
	m := newTestManager(t)
	if err := m.registerSchedules("p", []PluginSchedule{
		{ID: "x", Every: "10ms", Handler: "h"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Advance past Every by claiming due once.
	future := time.Now().Add(50 * time.Millisecond)
	first := m.DueSchedules(future)
	if len(first) != 1 {
		t.Fatalf("first DueSchedules: got %d entries, want 1", len(first))
	}

	// A second immediate call must not re-deliver the same entry — LastFired
	// was advanced atomically inside the first call.
	second := m.DueSchedules(future)
	if len(second) != 0 {
		t.Errorf("second DueSchedules at same wall time: got %d entries, want 0 (must not double-fire)", len(second))
	}
}

func TestDueSchedules_FiresAgainAfterInterval(t *testing.T) {
	m := newTestManager(t)
	if err := m.registerSchedules("p", []PluginSchedule{
		{ID: "x", Every: "10ms", Handler: "h"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	t1 := time.Now().Add(20 * time.Millisecond)
	if len(m.DueSchedules(t1)) != 1 {
		t.Fatalf("first tick should produce 1 due entry")
	}

	t2 := t1.Add(20 * time.Millisecond)
	if len(m.DueSchedules(t2)) != 1 {
		t.Errorf("second tick after another interval should produce 1 due entry")
	}
}

func TestDueSchedules_MultiplePlugins(t *testing.T) {
	m := newTestManager(t)
	if err := m.registerSchedules("plugin-a", []PluginSchedule{
		{ID: "drain", Every: "1ms", Handler: "h"},
	}); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := m.registerSchedules("plugin-b", []PluginSchedule{
		{ID: "ping", Every: "1ms", Handler: "h"},
	}); err != nil {
		t.Fatalf("register b: %v", err)
	}

	future := time.Now().Add(10 * time.Millisecond)
	due := m.DueSchedules(future)
	if len(due) != 2 {
		t.Fatalf("got %d due, want 2 (one per plugin)", len(due))
	}

	seen := map[string]string{}
	for _, d := range due {
		seen[d.PluginName] = d.ScheduleID
	}
	if seen["plugin-a"] != "drain" || seen["plugin-b"] != "ping" {
		t.Errorf("unexpected due entries: %+v", seen)
	}
}
