package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"windshift/internal/database"
)

// RetiredBacklogSummary accounts for abandoned work left by the retired
// durable consumers. It is taken before any deletion so diagnostics can
// distinguish abandonment from success.
type RetiredBacklogSummary struct {
	DeliveriesByState map[string]int64
	FrozenBatches     int64
	FrozenTargets     int64
}

// RetiredCleanupResult reports one bounded cleanup pass. Exhausted is true when
// the batch budget ran out before the backlog drained; the next pass resumes.
type RetiredCleanupResult struct {
	Summary           RetiredBacklogSummary
	DeliveriesDeleted int64
	BatchesDeleted    int64
	Exhausted         bool
}

// RetiredCleanupOptions bounds one cleanup pass. Zero values take defaults.
type RetiredCleanupOptions struct {
	BatchSize  int
	MaxBatches int
}

// SummarizeRetiredBacklog counts the retired consumers' remaining deliveries by
// state plus their frozen action batches and targets.
func SummarizeRetiredBacklog(ctx context.Context, db database.Database, keys []string) (RetiredBacklogSummary, error) {
	summary := RetiredBacklogSummary{DeliveriesByState: make(map[string]int64)}
	if db == nil || len(keys) == 0 {
		return summary, nil
	}
	placeholders, args := keyPlaceholders(keys)

	rows, err := db.QueryContext(ctx, `
		SELECT state, COUNT(*) FROM domain_event_deliveries
		WHERE consumer_key IN (`+placeholders+`)
		GROUP BY state
	`, args...)
	if err != nil {
		return summary, fmt.Errorf("summarize retired deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var state string
		var count int64
		if err := rows.Scan(&state, &count); err != nil {
			return summary, fmt.Errorf("scan retired delivery state: %w", err)
		}
		summary.DeliveriesByState[state] = count
	}
	if err := rows.Err(); err != nil {
		return summary, fmt.Errorf("iterate retired delivery states: %w", err)
	}

	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM action_event_batches WHERE consumer_key IN (`+placeholders+`)
	`, args...).Scan(&summary.FrozenBatches); err != nil {
		return summary, fmt.Errorf("count retired frozen batches: %w", err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM action_event_targets target
		JOIN action_event_batches batch ON batch.event_key = target.event_key
		WHERE batch.consumer_key IN (`+placeholders+`)
	`, args...).Scan(&summary.FrozenTargets); err != nil {
		return summary, fmt.Errorf("count retired frozen targets: %w", err)
	}
	return summary, nil
}

// PurgeRetiredDeliveries deletes up to limit retired deliveries. Delivery
// action records cascade with the delivery; canonical domain_events are left
// for the surviving consumer and normal retention.
func PurgeRetiredDeliveries(ctx context.Context, db database.Database, keys []string, limit int) (int64, error) {
	if db == nil || len(keys) == 0 || limit <= 0 {
		return 0, nil
	}
	placeholders, args := keyPlaceholders(keys)
	args = append(args, limit)
	result, err := db.ExecContext(ctx, `
		DELETE FROM domain_event_deliveries
		WHERE (event_id, consumer_key) IN (
			SELECT event_id, consumer_key
			FROM domain_event_deliveries
			WHERE consumer_key IN (`+placeholders+`)
			ORDER BY event_id, consumer_key
			LIMIT ?
		)
	`, args...)
	if err != nil {
		return 0, fmt.Errorf("purge retired deliveries: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read purged delivery count: %w", err)
	}
	return deleted, nil
}

// PurgeRetiredActionBatches deletes up to limit frozen action batches for the
// retired consumers. Targets cascade with their batch. Execution and
// reconciliation logs are preserved.
func PurgeRetiredActionBatches(ctx context.Context, db database.Database, keys []string, limit int) (int64, error) {
	if db == nil || len(keys) == 0 || limit <= 0 {
		return 0, nil
	}
	placeholders, args := keyPlaceholders(keys)
	args = append(args, limit)
	result, err := db.ExecContext(ctx, `
		DELETE FROM action_event_batches
		WHERE event_key IN (
			SELECT event_key FROM action_event_batches
			WHERE consumer_key IN (`+placeholders+`)
			ORDER BY materialized_at, event_key
			LIMIT ?
		)
	`, args...)
	if err != nil {
		return 0, fmt.Errorf("purge retired action batches: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read purged action batch count: %w", err)
	}
	return deleted, nil
}

// RunRetiredCleanup logs the abandoned-backlog summary and removes it in
// bounded, resumable batches. It never marks unfinished work successful and
// leaves retained consumers' rows untouched.
func RunRetiredCleanup(ctx context.Context, db database.Database, keys []string, opts RetiredCleanupOptions) (RetiredCleanupResult, error) {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 500
	}
	if opts.MaxBatches <= 0 {
		opts.MaxBatches = 20
	}
	summary, err := SummarizeRetiredBacklog(ctx, db, keys)
	if err != nil {
		return RetiredCleanupResult{}, err
	}
	result := RetiredCleanupResult{Summary: summary}
	slog.Info("retired side-effect backlog",
		slog.String("component", "sideeffects"),
		slog.Any("deliveries_by_state", summary.DeliveriesByState),
		slog.Int64("frozen_batches", summary.FrozenBatches),
		slog.Int64("frozen_targets", summary.FrozenTargets),
	)
	for batch := 0; batch < opts.MaxBatches; batch++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		deliveries, err := PurgeRetiredDeliveries(ctx, db, keys, opts.BatchSize)
		if err != nil {
			return result, err
		}
		batches, err := PurgeRetiredActionBatches(ctx, db, keys, opts.BatchSize)
		if err != nil {
			return result, err
		}
		result.DeliveriesDeleted += deliveries
		result.BatchesDeleted += batches
		if deliveries < int64(opts.BatchSize) && batches < int64(opts.BatchSize) {
			return result, nil
		}
	}
	result.Exhausted = true
	slog.Info("retired side-effect cleanup paused at its batch budget",
		slog.String("component", "sideeffects"),
		slog.Int64("deliveries_deleted", result.DeliveriesDeleted),
		slog.Int64("batches_deleted", result.BatchesDeleted),
	)
	return result, nil
}

func keyPlaceholders(keys []string) (placeholders string, args []any) {
	placeholders = strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	args = make([]any, len(keys))
	for i, key := range keys {
		args[i] = key
	}
	return placeholders, args
}
