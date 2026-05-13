//go:build !noplugins

package plugins

import (
	"errors"
	"fmt"
	"time"
)

// scheduledPlugin is the in-memory record for one registered periodic
// invocation. LastFired is the timestamp the global scheduler last claimed
// this entry for firing (via DueSchedules), NOT the timestamp the WASM call
// completed — that distinction lets the registry stay invocation-result
// agnostic while still preventing concurrent ticks from double-firing.
type scheduledPlugin struct {
	ID        string
	Every     time.Duration
	Handler   string
	LastFired time.Time
}

// ErrInvalidSchedule reports a malformed PluginSchedule (empty fields or an
// unparseable Every duration). Returned from registerSchedules so plugin load
// can surface a clear diagnostic to operators.
var ErrInvalidSchedule = errors.New("invalid plugin schedule")

// registerSchedules validates and installs the plugin's declared schedules
// into the in-memory registry. Existing entries for the same plugin are
// replaced wholesale, which covers the reload path (a schedule removed from
// the manifest stops firing immediately on reload). LastFired is seeded to
// "now" so a freshly registered schedule does not fire instantly on the next
// tick; the first fire happens one Every interval later.
//
// Validation errors short-circuit: if any entry is malformed, no entries for
// the plugin are registered. This avoids "partially registered" plugin state.
// Callers in the plugin-load path should call validatePluginSchedules first
// so they can fail load atomically before any state mutation.
func (m *Manager) registerSchedules(pluginName string, schedules []PluginSchedule) error {
	if len(schedules) == 0 {
		// No schedules to register; ensure any prior entries are cleared
		// (covers reload-removed-schedule path).
		m.unregisterSchedules(pluginName)
		return nil
	}

	parsed, err := parsePluginSchedules(schedules)
	if err != nil {
		return err
	}

	m.schedulesMu.Lock()
	m.schedules[pluginName] = parsed
	m.schedulesMu.Unlock()

	return nil
}

// parsePluginSchedules validates a manifest's schedules and returns the
// parsed in-memory records. Separated from registerSchedules so the plugin
// load path can fail fast on malformed entries before compiling WASM.
func parsePluginSchedules(schedules []PluginSchedule) ([]*scheduledPlugin, error) {
	parsed := make([]*scheduledPlugin, 0, len(schedules))
	seen := make(map[string]struct{}, len(schedules))
	now := time.Now()

	for _, s := range schedules {
		if s.ID == "" {
			return nil, fmt.Errorf("%w: schedule id is empty", ErrInvalidSchedule)
		}
		if s.Handler == "" {
			return nil, fmt.Errorf("%w: schedule %q handler is empty", ErrInvalidSchedule, s.ID)
		}
		if _, dup := seen[s.ID]; dup {
			return nil, fmt.Errorf("%w: duplicate schedule id %q within plugin", ErrInvalidSchedule, s.ID)
		}
		seen[s.ID] = struct{}{}

		every, err := time.ParseDuration(s.Every)
		if err != nil {
			return nil, fmt.Errorf("%w: schedule %q every=%q is not a valid duration: %v", ErrInvalidSchedule, s.ID, s.Every, err)
		}
		if every <= 0 {
			return nil, fmt.Errorf("%w: schedule %q every=%q must be positive", ErrInvalidSchedule, s.ID, s.Every)
		}

		parsed = append(parsed, &scheduledPlugin{
			ID:        s.ID,
			Every:     every,
			Handler:   s.Handler,
			LastFired: now,
		})
	}
	return parsed, nil
}

// unregisterSchedules removes all schedules for the given plugin. Safe to call
// on a plugin that has no schedules registered.
func (m *Manager) unregisterSchedules(pluginName string) {
	m.schedulesMu.Lock()
	delete(m.schedules, pluginName)
	m.schedulesMu.Unlock()
}

// DueSchedules returns every schedule whose Every interval has elapsed since
// its LastFired timestamp, and atomically advances those entries' LastFired so
// the next tick will not re-claim them. The actual WASM invocation happens
// outside this method — the scheduler iterates the returned list and calls
// Manager.CallPluginFunction per entry.
//
// The single critical-section guarantee: between the "is due" check and the
// LastFired update, no other caller can observe the same entry as due. That
// makes the registry safe against concurrent ticks (e.g. an admin manually
// triggers a tick while the periodic ticker also fires).
func (m *Manager) DueSchedules(now time.Time) []DueSchedule {
	m.schedulesMu.Lock()
	defer m.schedulesMu.Unlock()

	var due []DueSchedule
	for pluginName, entries := range m.schedules {
		for _, e := range entries {
			if now.Sub(e.LastFired) < e.Every {
				continue
			}
			due = append(due, DueSchedule{
				PluginName: pluginName,
				ScheduleID: e.ID,
				Handler:    e.Handler,
			})
			e.LastFired = now
		}
	}
	return due
}
