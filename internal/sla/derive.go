// Package sla implements the SLA engine: derivation, evaluation, and the
// durable due-work loop.
//
// The read surfaces are internal-only. Portal customers receive no SLA data
// until a separate portal-display policy is designed (WI-584 slice 11); when
// that policy lands, gate exposure here rather than branching in the
// derivation functions, which must stay a faithful internal projection.
package sla

import (
	"encoding/json"
	"fmt"
	"time"

	"windshift/internal/businesstime"
	"windshift/internal/models"
)

// Derive computes the read-time, Jira-shaped state of a cycle. Reads never
// write: the stored columns stay the scheduling truth and elapsed time is
// extended from last_calculated_at to now with pure calendar math.
func Derive(cycle *models.ItemSLACycle, now time.Time) (models.DerivedSLACycle, error) {
	derived := models.DerivedSLACycle{
		CycleNo:        cycle.CycleNo,
		Status:         cycle.Status,
		GoalID:         cycle.GoalID,
		StartedAt:      cycle.StartedAt,
		StoppedAt:      cycle.StoppedAt,
		BreachTime:     cycle.BreachTime,
		GoalDurationMs: cycle.GoalDurationMs,
		Paused:         cycle.PauseStartedAt != nil,
		PauseStartedAt: cycle.PauseStartedAt,
		NextDeadlineAt: cycle.NextDeadlineAt,
	}
	var calendar businesstime.RawCalendar
	if err := json.Unmarshal(cycle.CalendarSnapshot, &calendar); err == nil {
		derived.CalendarTimezone = calendar.Timezone
	}

	elapsed := time.Duration(cycle.ElapsedMs) * time.Millisecond
	if cycle.Status == models.SLACycleOngoing {
		calendar, err := businesstime.CompileJSON(string(cycle.CalendarSnapshot))
		if err != nil {
			return models.DerivedSLACycle{}, fmt.Errorf("compile cycle %d calendar: %w", cycle.ID, err)
		}
		if cycle.PauseStartedAt == nil && cycle.GoalDurationMs > 0 {
			elapsed += calendar.ElapsedCalendarTime(cycle.LastCalculatedAt, now)
		}
		// A cycle with no matched target carries the synthetic 24/7 snapshot and
		// has nothing to measure against; reporting it as service hours would
		// claim a schedule it never had.
		derived.WithinCalendarHours = cycle.GoalDurationMs > 0 && calendar.WithinCalendarHours(now)
	} else {
		derived.WithinCalendarHours = cycle.WithinCalendarHours
	}
	derived.ElapsedMs = elapsed.Milliseconds()

	// A cycle without a target has no remaining time and no breach, matching
	// Jira's "no target" state. Gate on the stored duration, not the goal
	// reference: deleting a goal nulls goal_id via ON DELETE SET NULL but keeps
	// goal_duration_ms, so historical breach and remaining-time display survive.
	if cycle.GoalDurationMs <= 0 {
		derived.Breached = false
		return derived, nil
	}
	remaining := time.Duration(cycle.GoalDurationMs)*time.Millisecond - elapsed
	remainingMs := remaining.Milliseconds()
	derived.RemainingMs = &remainingMs
	derived.Breached = elapsed >= time.Duration(cycle.GoalDurationMs)*time.Millisecond
	return derived, nil
}
