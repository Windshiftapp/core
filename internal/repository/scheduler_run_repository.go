package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// SchedulerRunRepository persists scheduler tick history and reads it back for
// the admin Diagnostics page.
type SchedulerRunRepository struct {
	db database.Database
}

// NewSchedulerRunRepository constructs a new repository.
func NewSchedulerRunRepository(db database.Database) *SchedulerRunRepository {
	return &SchedulerRunRepository{db: db}
}

// Insert records a single scheduler run. Errors are returned but the caller
// (the scheduler itself) typically just logs them — recording must never
// block a scheduler tick.
func (r *SchedulerRunRepository) Insert(ctx context.Context, run *models.SchedulerRun) error {
	_, err := r.db.ExecWriteContext(ctx, `
		INSERT INTO scheduler_runs
			(scheduler_name, started_at, completed_at, duration_ms, items_processed, success, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		run.SchedulerName, run.StartedAt, run.CompletedAt,
		nullIntArg(run.DurationMs), nullIntArg(run.ItemsProcessed),
		run.Success, nullStringArg(run.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("failed to insert scheduler run: %w", err)
	}
	return nil
}

// RecentSchedulerRunsOpts controls queries for recent scheduler runs.
type RecentSchedulerRunsOpts struct {
	Scheduler string    // "" for any; e.g. "briefing", "email", "recurrence", "notification"
	Status    string    // "" for any; "success" or "failed"
	Since     time.Time // started_at >= Since (zero = no lower bound)
	Limit     int       // capped at 200; 25 default
}

// GetRecent returns scheduler runs ordered by start time (newest first).
func (r *SchedulerRunRepository) GetRecent(opts RecentSchedulerRunsOpts) ([]*models.SchedulerRun, error) {
	conds := []string{"1=1"}
	args := []any{}
	if opts.Scheduler != "" {
		conds = append(conds, "scheduler_name = ?")
		args = append(args, opts.Scheduler)
	}
	switch opts.Status {
	case "success":
		conds = append(conds, "success = ?")
		args = append(args, true)
	case "failed":
		conds = append(conds, "success = ?")
		args = append(args, false)
	}
	if !opts.Since.IsZero() {
		conds = append(conds, "started_at >= ?")
		args = append(args, opts.Since)
	}

	limit := opts.Limit
	if limit <= 0 || limit > 200 {
		limit = 25
	}

	query := fmt.Sprintf(`
		SELECT id, scheduler_name, started_at, completed_at, duration_ms,
		       items_processed, success, error_message
		FROM scheduler_runs
		WHERE %s
		ORDER BY started_at DESC
		LIMIT ?
	`, strings.Join(conds, " AND "))
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduler runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*models.SchedulerRun
	for rows.Next() {
		run := &models.SchedulerRun{}
		var completedAt sql.NullTime
		var durationMs, itemsProcessed sql.NullInt64
		var errorMessage sql.NullString

		if err := rows.Scan(
			&run.ID, &run.SchedulerName, &run.StartedAt, &completedAt,
			&durationMs, &itemsProcessed, &run.Success, &errorMessage,
		); err != nil {
			return nil, fmt.Errorf("failed to scan scheduler run: %w", err)
		}

		if completedAt.Valid {
			run.CompletedAt = &completedAt.Time
		}
		if durationMs.Valid {
			v := int(durationMs.Int64)
			run.DurationMs = &v
		}
		if itemsProcessed.Valid {
			v := int(itemsProcessed.Int64)
			run.ItemsProcessed = &v
		}
		if errorMessage.Valid {
			run.ErrorMessage = errorMessage.String
		}

		result = append(result, run)
	}

	return result, nil
}

// SchedulerStats aggregates one scheduler's activity inside a time window.
type SchedulerStats struct {
	SchedulerName  string  `json:"scheduler_name"`
	Total          int     `json:"total"`
	Successes      int     `json:"successes"`
	Failures       int     `json:"failures"`
	AvgDurationMs  *int    `json:"avg_duration_ms,omitempty"`
	LastSuccessAt  *string `json:"last_success_at,omitempty"`
	LastFailureAt  *string `json:"last_failure_at,omitempty"`
	TotalProcessed *int    `json:"total_processed,omitempty"`
}

// Stats returns per-scheduler aggregates for runs since the given time.
func (r *SchedulerRunRepository) Stats(since time.Time) ([]*SchedulerStats, error) {
	rows, err := r.db.Query(`
		SELECT scheduler_name,
		       COUNT(*),
		       SUM(CASE WHEN success THEN 1 ELSE 0 END),
		       SUM(CASE WHEN success THEN 0 ELSE 1 END),
		       AVG(duration_ms),
		       MAX(CASE WHEN success THEN started_at END),
		       MAX(CASE WHEN NOT success THEN started_at END),
		       SUM(items_processed)
		FROM scheduler_runs
		WHERE started_at >= ?
		GROUP BY scheduler_name
		ORDER BY scheduler_name
	`, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduler stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*SchedulerStats
	for rows.Next() {
		s := &SchedulerStats{}
		var avgDur sql.NullFloat64
		var lastSuccess, lastFailure sql.NullTime
		var totalProcessed sql.NullInt64

		if err := rows.Scan(
			&s.SchedulerName, &s.Total, &s.Successes, &s.Failures,
			&avgDur, &lastSuccess, &lastFailure, &totalProcessed,
		); err != nil {
			return nil, fmt.Errorf("failed to scan scheduler stats: %w", err)
		}

		if avgDur.Valid {
			v := int(avgDur.Float64)
			s.AvgDurationMs = &v
		}
		if lastSuccess.Valid {
			v := lastSuccess.Time.Format(time.RFC3339)
			s.LastSuccessAt = &v
		}
		if lastFailure.Valid {
			v := lastFailure.Time.Format(time.RFC3339)
			s.LastFailureAt = &v
		}
		if totalProcessed.Valid {
			v := int(totalProcessed.Int64)
			s.TotalProcessed = &v
		}
		result = append(result, s)
	}

	return result, nil
}

// Purge deletes scheduler run rows older than the cutoff. Returns count.
func (r *SchedulerRunRepository) Purge(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := r.db.ExecWriteContext(ctx, `DELETE FROM scheduler_runs WHERE started_at < ?`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to purge scheduler runs: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read purge row count: %w", err)
	}
	return rows, nil
}
