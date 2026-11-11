package sla

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// SideEffectEmitter emits the notification and automation compatibility events
// for a breach or warning. It is optional; without it the durable state and
// lifecycle history still advance.
type SideEffectEmitter interface {
	EmitBreach(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle) error
	EmitWarning(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle, thresholdKey string) error
}

// SetSideEffectEmitter installs the breach/warning side-effect emitter.
func (e *Engine) SetSideEffectEmitter(emitter SideEffectEmitter) { e.sideEffects = emitter }

// SetJobOwner records the due-work loop owner this engine runs under. Claimed
// deadline jobs are then retired with a lease fence so a stale runner cannot
// delete a job that an inline re-arm or another claim replaced.
func (e *Engine) SetJobOwner(owner string) { e.owner = owner }

// retireClaimedJob deletes the claimed job row while this runner still owns
// its lease. Without a configured owner the delete falls back to the subject
// identity (single-instance semantics).
func (e *Engine) retireClaimedJob(ctx context.Context, tx database.Tx, job models.SLAJob) error {
	if e.owner == "" {
		return e.repo.DeleteJob(ctx, tx, job.Kind, job.ThresholdKey, job.CycleID, job.ItemID, job.MetricID)
	}
	return e.repo.DeleteClaimedJob(ctx, tx, job, e.owner)
}

// RunJob executes one claimed due-work job. A successful run must delete or
// re-arm the job; the loop only reschedules on a returned error.
func (e *Engine) RunJob(ctx context.Context, job models.SLAJob) error {
	switch job.Kind {
	case models.SLAJobBreach:
		return e.runBreachJob(ctx, job)
	case models.SLAJobWarning:
		return e.runWarningJob(ctx, job)
	case models.SLAJobRecalcItem:
		return e.runRecalcItemJob(ctx, job)
	case models.SLAJobRecalcMetric:
		return e.runRecalcMetricJob(ctx, job)
	case models.SLAJobRecalcCalendar:
		return e.runRecalcCalendarJob(ctx, job)
	default:
		return e.repo.FailJob(ctx, job.ID, "unknown job kind "+job.Kind)
	}
}

func (e *Engine) runBreachJob(ctx context.Context, job models.SLAJob) error {
	if job.CycleID == nil || job.DeadlineAt == nil {
		return e.repo.FailJob(ctx, job.ID, "malformed SLA job")
	}
	return database.WithTx(e.db, func(tx database.Tx) error {
		if err := e.repo.LockItemForCycle(ctx, tx, *job.CycleID); errors.Is(err, repository.ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		cycle, err := e.repo.GetCycleForUpdate(ctx, tx, *job.CycleID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		// The cycle moved on: inline evaluation already re-armed whatever is
		// current, so retire this job. breached_at is first-breach history, not
		// current state; an armed deadline under a re-goaled longer target is a
		// live job, not a stale one.
		if cycle.Status != models.SLACycleOngoing ||
			cycle.NextDeadlineAt == nil || !cycle.NextDeadlineAt.Equal(*job.DeadlineAt) {
			return e.retireClaimedJob(ctx, tx, job)
		}
		// A deactivated metric (or one imported inactive) must not emit a
		// breach. Retire the job; activation re-enqueues recalculation.
		compiled, err := e.metricCompiled(ctx, tx, cycle)
		if err != nil {
			return err
		}
		if !compiled {
			return e.retireClaimedJob(ctx, tx, job)
		}
		config, workspaceID, stoppedAt, stopped, err := e.stopPrecedesDeadline(ctx, tx, cycle, *job.DeadlineAt)
		if err != nil {
			return err
		}
		if stopped {
			return e.completeCycle(ctx, tx, config, workspaceID, cycle, stoppedAt)
		}
		deadline := *job.DeadlineAt
		// The breach event represents the promised deadline, so extend elapsed
		// to it before recording and emitting; an untouched cycle would
		// otherwise report zero elapsed time.
		if err := e.advanceCycle(config, cycle, deadline); err != nil {
			return err
		}
		if cycle.BreachedAt == nil || deadline.Before(*cycle.BreachedAt) {
			cycle.BreachedAt = &deadline
		}
		cycle.NextDeadlineAt = nil
		cycle.RemainingMs = cycle.GoalDurationMs - cycle.ElapsedMs
		if err := e.repo.UpdateCycle(ctx, tx, cycle); err != nil {
			return err
		}
		// A breach supersedes any pending warning for the cycle.
		if err := e.repo.DeleteJobsForCycleKind(ctx, tx, cycle.ID, models.SLAJobWarning); err != nil {
			return err
		}
		if err := e.appendLifecycle(ctx, tx, e.workspaceForItemTx(ctx, tx, cycle.ItemID), cycle, eventBreached, deadline); err != nil {
			return err
		}
		if e.sideEffects != nil {
			if err := e.sideEffects.EmitBreach(ctx, tx, cycle); err != nil {
				return err
			}
		}
		return e.retireClaimedJob(ctx, tx, job)
	})
}

func (e *Engine) stopPrecedesDeadline(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle, deadline time.Time) (config *compiledConfig, workspaceID int, stoppedAt time.Time, stopped bool, err error) {
	var updatedAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT updated_at FROM items WHERE id = ?`, cycle.ItemID).Scan(&updatedAt); err != nil {
		return nil, 0, time.Time{}, false, err
	}
	if updatedAt.After(deadline) {
		return nil, 0, time.Time{}, false, nil
	}
	snapshot, err := loadItemSnapshotTx(ctx, tx, cycle.ItemID)
	if err != nil {
		return nil, 0, time.Time{}, false, err
	}
	if snapshot.WorkspaceID == 0 {
		return nil, 0, time.Time{}, false, nil
	}
	generations, err := e.repo.GenerationsForWorkspacesTx(ctx, tx, []int{snapshot.WorkspaceID})
	if err != nil {
		return nil, 0, time.Time{}, false, err
	}
	generation, ok := generations[snapshot.WorkspaceID]
	if !ok {
		return nil, 0, time.Time{}, false, nil
	}
	config, err = e.configuration(ctx, snapshot.WorkspaceID, generation)
	if err != nil {
		return nil, 0, time.Time{}, false, err
	}
	metric := config.metricsByID[cycle.MetricID]
	if metric == nil || !metric.matchPhaseCurrent(models.SLAPhaseStop, snapshot, config) {
		return nil, 0, time.Time{}, false, nil
	}
	return config, snapshot.WorkspaceID, updatedAt, true, nil
}

// metricCompiled reports whether a cycle's metric is part of the workspace's
// active compiled configuration. A job can outlive a metric deactivation or
// belong to a metric imported inactive; such jobs must retire silently instead
// of emitting a breach or warning for a metric the workspace does not run.
func (e *Engine) metricCompiled(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle) (bool, error) {
	snapshot, err := loadItemSnapshotTx(ctx, tx, cycle.ItemID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if snapshot.WorkspaceID == 0 {
		return false, nil
	}
	generations, err := e.repo.GenerationsForWorkspacesTx(ctx, tx, []int{snapshot.WorkspaceID})
	if err != nil {
		return false, err
	}
	generation, ok := generations[snapshot.WorkspaceID]
	if !ok {
		return false, nil
	}
	config, err := e.configuration(ctx, snapshot.WorkspaceID, generation)
	if err != nil {
		return false, err
	}
	return config.metricsByID[cycle.MetricID] != nil, nil
}

func (e *Engine) runWarningJob(ctx context.Context, job models.SLAJob) error {
	if job.CycleID == nil || job.DeadlineAt == nil {
		return e.repo.FailJob(ctx, job.ID, "malformed SLA job")
	}
	return database.WithTx(e.db, func(tx database.Tx) error {
		cycle, err := e.repo.GetCycleForUpdate(ctx, tx, *job.CycleID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if cycle.Status != models.SLACycleOngoing ||
			cycle.NextDeadlineAt == nil || !cycle.NextDeadlineAt.Equal(*job.DeadlineAt) {
			return e.retireClaimedJob(ctx, tx, job)
		}
		// A deactivated metric (or one imported inactive) must not emit a
		// warning. Retire the job; activation re-enqueues recalculation.
		compiled, err := e.metricCompiled(ctx, tx, cycle)
		if err != nil {
			return err
		}
		if !compiled {
			return e.retireClaimedJob(ctx, tx, job)
		}
		firedAt := e.clock.Now()
		// Extend elapsed to the firing instant so the warning carries the
		// current time rather than the last stored value.
		if err := e.advanceCycle(nil, cycle, firedAt); err != nil {
			return err
		}
		fired, err := e.repo.MarkThresholdFired(ctx, tx, cycle.ID, job.ThresholdKey, firedAt)
		if err != nil {
			return err
		}
		if !fired {
			return e.retireClaimedJob(ctx, tx, job)
		}
		if err := e.appendLifecycle(ctx, tx, e.workspaceForItemTx(ctx, tx, cycle.ItemID), cycle, eventWarning, firedAt); err != nil {
			return err
		}
		if e.sideEffects != nil {
			if err := e.sideEffects.EmitWarning(ctx, tx, cycle, job.ThresholdKey); err != nil {
				return err
			}
		}
		return e.retireClaimedJob(ctx, tx, job)
	})
}

func (e *Engine) runRecalcItemJob(ctx context.Context, job models.SLAJob) error {
	if job.ItemID == nil {
		return e.repo.FailJob(ctx, job.ID, "malformed SLA job")
	}
	return database.WithTx(e.db, func(tx database.Tx) error {
		if err := e.recalcItemTx(ctx, tx, *job.ItemID); err != nil {
			return err
		}
		return e.repo.DeleteJob(ctx, tx, job.Kind, job.ThresholdKey, job.CycleID, job.ItemID, job.MetricID)
	})
}

func (e *Engine) runRecalcCalendarJob(ctx context.Context, job models.SLAJob) error {
	calendarID, err := strconv.Atoi(job.ThresholdKey)
	if err != nil || calendarID <= 0 || job.MetricID == nil {
		return e.repo.FailJob(ctx, job.ID, "malformed calendar recalculation job")
	}
	cursor := int64(0)
	if job.Cursor != nil {
		parsed, err := strconv.ParseInt(*job.Cursor, 10, 64)
		if err != nil || parsed < 0 {
			return e.repo.FailJob(ctx, job.ID, "malformed calendar recalculation cursor")
		}
		cursor = parsed
	}
	return database.WithTx(e.db, func(tx database.Tx) error {
		next, done, err := e.recalcCalendarPageTx(ctx, tx, *job.MetricID, calendarID, cursor, DefaultRecalcPageSize)
		if err != nil {
			return err
		}
		if done {
			return e.repo.DeleteJob(ctx, tx, job.Kind, job.ThresholdKey, job.CycleID, job.ItemID, job.MetricID)
		}
		nextCursor := strconv.FormatInt(next, 10)
		return e.repo.UpsertJob(ctx, tx, &models.SLAJob{
			Kind: models.SLAJobRecalcCalendar, ThresholdKey: job.ThresholdKey, MetricID: job.MetricID,
			DueAt: e.clock.Now(), Cursor: &nextCursor,
		})
	})
}

func (e *Engine) runRecalcMetricJob(ctx context.Context, job models.SLAJob) error {
	if job.MetricID == nil {
		return e.repo.FailJob(ctx, job.ID, "malformed SLA job")
	}
	return database.WithTx(e.db, func(tx database.Tx) error {
		cursor := ""
		if job.Cursor != nil {
			cursor = *job.Cursor
		}
		next, done, err := e.recalcMetricPageTx(ctx, tx, *job.MetricID, cursor, DefaultRecalcPageSize)
		if err != nil {
			return err
		}
		if done {
			return e.repo.DeleteJob(ctx, tx, job.Kind, job.ThresholdKey, job.CycleID, job.ItemID, job.MetricID)
		}
		return e.repo.UpsertJob(ctx, tx, &models.SLAJob{
			Kind:     models.SLAJobRecalcMetric,
			MetricID: job.MetricID,
			DueAt:    e.clock.Now(),
			Cursor:   &next,
		})
	})
}
