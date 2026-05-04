package scheduler

import (
	"context"
	"log/slog"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// recordSchedulerRun persists one tick's outcome to scheduler_runs.
// Called from each scheduler's tick function as a deferred call so the result
// is recorded on every code path including early returns.
//
// Best-effort: a DB failure here is logged but never propagated, so a hiccup
// in scheduler_runs cannot break an actual scheduler tick.
//
// Pass items as a pointer so the caller can mutate it inside the tick body
// before this defer fires; same for runErr.
//
//nolint:gocritic // ptrToRefParam: pointer is intentional — caller mutates after defer is registered
func recordSchedulerRun(repo *repository.SchedulerRunRepository, name string, start time.Time, items *int, runErr *error) {
	if repo == nil {
		return
	}
	completed := time.Now()
	dur := int(completed.Sub(start).Milliseconds())
	msg := ""
	success := true
	if runErr != nil && *runErr != nil {
		msg = (*runErr).Error()
		success = false
	}
	run := &models.SchedulerRun{
		SchedulerName:  name,
		StartedAt:      start,
		CompletedAt:    &completed,
		DurationMs:     &dur,
		Success:        success,
		ErrorMessage:   msg,
		ItemsProcessed: items,
	}
	if err := repo.Insert(context.Background(), run); err != nil {
		slog.Warn("failed to record scheduler run", "scheduler", name, "error", err)
	}
}
