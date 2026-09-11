// Package actionevents provides durable admission and frozen-target execution
// shared by every automation domain.
package actionevents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/events"

	"uuid"
)

// Cutover is the durable event boundary where a canonical producer becomes
// authoritative for an automation domain.
type Cutover struct {
	StartEventID int64
	RecordedAt   time.Time
}

type cutoverQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ActivateCutover records a one-way canonical event boundary exactly once.
func ActivateCutover(ctx context.Context, db database.Database, key, label string) (*Cutover, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin %s cutover: %w", label, err)
	}
	defer func() { _ = tx.Rollback() }()

	cutover, err := loadCutover(ctx, tx, key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if cutover != nil {
		return cutover, nil
	}
	var startEventID int64
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM domain_events").Scan(&startEventID); err != nil {
		return nil, fmt.Errorf("select %s cutover boundary: %w", label, err)
	}
	var recordedAt time.Time
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO action_event_cutovers (cutover_key, start_event_id, recorded_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (cutover_key) DO NOTHING
		RETURNING recorded_at
	`, key, startEventID).Scan(&recordedAt); errors.Is(err, sql.ErrNoRows) {
		cutover, err = loadCutover(ctx, tx, key)
		if err != nil {
			return nil, fmt.Errorf("load concurrently recorded %s cutover: %w", label, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit %s cutover lookup: %w", label, err)
		}
		return cutover, nil
	} else if err != nil {
		return nil, fmt.Errorf("record %s cutover: %w", label, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit %s cutover: %w", label, err)
	}
	return &Cutover{StartEventID: startEventID, RecordedAt: recordedAt}, nil
}

// CurrentCutover returns nil until the named canonical boundary is active.
// The query may be a database or the source transaction being admitted.
func CurrentCutover(ctx context.Context, query cutoverQuery, key string) (*Cutover, error) {
	cutover, err := loadCutover(ctx, query, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return cutover, err
}

func loadCutover(ctx context.Context, query cutoverQuery, key string) (*Cutover, error) {
	cutover := &Cutover{}
	err := query.QueryRowContext(ctx, `
		SELECT start_event_id, recorded_at
		FROM action_event_cutovers
		WHERE cutover_key = ?
	`, key).Scan(&cutover.StartEventID, &cutover.RecordedAt)
	if err != nil {
		return nil, err
	}
	return cutover, nil
}

// AppendStandalone stores a compatibility trigger idempotently.
func AppendStandalone(ctx context.Context, db database.Database, store *events.Store, input events.NewEvent, label string) error {
	if _, err := store.AppendStandalone(ctx, input); err != nil {
		var existing int
		if lookupErr := db.QueryRowContext(ctx, "SELECT 1 FROM domain_events WHERE event_key = ?", input.Key).Scan(&existing); lookupErr == nil {
			return nil
		}
		return fmt.Errorf("append durable %s event: %w", label, err)
	}
	return nil
}

// CompatibilityInput describes one legacy trigger admitted to the journal.
type CompatibilityInput struct {
	Payload           any
	WorkspaceID       *int
	AggregateType     string
	AggregateID       string
	EventType         string
	ActorUserID       int
	CorrelationID     string
	CausationEventKey string
}

// NewCompatibilityEvent encodes the common envelope used during domain
// producer cutovers.
func NewCompatibilityEvent(input CompatibilityInput) (events.NewEvent, error) {
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return events.NewEvent{}, fmt.Errorf("encode durable compatibility event: %w", err)
	}
	event := events.NewEvent{
		Key: uuid.New().String(), WorkspaceID: input.WorkspaceID,
		AggregateType: input.AggregateType, AggregateID: input.AggregateID,
		Type: input.EventType, PayloadVersion: 1, OccurredAt: time.Now().UTC(),
		ActorKind: "system", SourceKind: "compatibility",
		CorrelationID: input.CorrelationID, CausationEventKey: input.CausationEventKey,
		Payload: payload,
	}
	if input.ActorUserID > 0 {
		event.ActorKind = "user"
		event.ActorRef = strconv.Itoa(input.ActorUserID)
	}
	return event, nil
}

// ConfigureCutoverConsumers installs always-on compatibility subscriptions
// followed by the canonical subscription at its recorded boundary.
func ConfigureCutoverConsumers(ctx context.Context, store *events.Store, cutover *Cutover, canonical events.Consumer, always ...events.Consumer) error {
	canonical.Active = cutover != nil
	canonical.StartEventID = 1
	if cutover != nil {
		canonical.StartEventID = cutover.StartEventID
	}
	for _, consumer := range append(always, canonical) {
		if err := store.ConfigureConsumer(ctx, consumer); err != nil {
			return err
		}
	}
	return nil
}

type target struct {
	ActionID int
	State    string
}

// TargetStore owns the frozen action set and per-target retry state.
type TargetStore struct {
	db database.Database
}

// NewTargetStore creates a frozen-target store on an initialized database.
func NewTargetStore(db database.Database) *TargetStore {
	return &TargetStore{db: db}
}

// Materialize freezes matching action IDs the first time an event is handled.
func (s *TargetStore) Materialize(ctx context.Context, event events.Event, consumerKey, triggerEvent string, actionIDs []int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin action target materialization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO action_event_batches (
			event_key, event_id, consumer_key, workspace_id, trigger_event, materialized_at
		) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (event_key) DO NOTHING
	`, event.Key, event.ID, consumerKey, event.WorkspaceID, triggerEvent)
	if err != nil {
		return fmt.Errorf("materialize action event batch: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read action event batch result: %w", err)
	}
	if inserted > 0 {
		for _, actionID := range actionIDs {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO action_event_targets (event_key, action_id, state, created_at, updated_at)
				VALUES (?, ?, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			`, event.Key, actionID); err != nil {
				return fmt.Errorf("materialize action %d for event %s: %w", actionID, event.Key, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit action target materialization: %w", err)
	}
	return nil
}

func (s *TargetStore) targets(ctx context.Context, eventKey string) ([]target, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT action_id, state
		FROM action_event_targets
		WHERE event_key = ?
		ORDER BY action_id
	`, eventKey)
	if err != nil {
		return nil, fmt.Errorf("load action event targets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var targets []target
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.ActionID, &item.State); err != nil {
			return nil, fmt.Errorf("scan action event target: %w", err)
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate action event targets: %w", err)
	}
	return targets, nil
}

func (s *TargetStore) markRunning(ctx context.Context, eventKey string, actionID int) error {
	_, err := s.db.ExecWriteContext(ctx, `
		UPDATE action_event_targets
		SET state = 'running', attempt_count = attempt_count + 1,
		    last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE event_key = ? AND action_id = ? AND state IN ('pending', 'failed', 'running')
	`, eventKey, actionID)
	if err != nil {
		return fmt.Errorf("mark action target running: %w", err)
	}
	return nil
}

func (s *TargetStore) markFailed(ctx context.Context, eventKey string, actionID int, failure error) error {
	_, err := s.db.ExecWriteContext(ctx, `
		UPDATE action_event_targets
		SET state = 'failed', last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE event_key = ? AND action_id = ?
	`, failure.Error(), eventKey, actionID)
	if err != nil {
		return fmt.Errorf("mark action target failed: %w", err)
	}
	return nil
}

func (s *TargetStore) markCompleted(ctx context.Context, eventKey string, actionID int) error {
	_, err := s.db.ExecWriteContext(ctx, `
		UPDATE action_event_targets
		SET state = 'completed', completed_at = CURRENT_TIMESTAMP,
		    last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE event_key = ? AND action_id = ?
	`, eventKey, actionID)
	if err != nil {
		return fmt.Errorf("mark action target completed: %w", err)
	}
	return nil
}

// Callbacks adapt one domain's execution log and action executor.
type Callbacks struct {
	Completed func(actionID int) (bool, error)
	Execute   func(actionID int) (permanent bool, err error)
}

// RunTargets resumes every unfinished frozen target and returns the number
// completed during this attempt.
func RunTargets(ctx context.Context, store *TargetStore, eventKey string, callbacks Callbacks) (int64, error) {
	targets, err := store.targets(ctx, eventKey)
	if err != nil {
		return 0, err
	}
	if callbacks.Completed == nil || callbacks.Execute == nil {
		return 0, errors.New("durable action target callbacks are required")
	}

	var targetErrors []error
	hasTransientFailure := false
	var executed int64
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return executed, err
		}
		if target.State == "completed" || target.State == "skipped" {
			continue
		}
		completed, completionErr := callbacks.Completed(target.ActionID)
		if completionErr == nil && completed {
			if err := store.markCompleted(ctx, eventKey, target.ActionID); err != nil {
				targetErrors = append(targetErrors, err)
				hasTransientFailure = true
			}
			continue
		}
		if completionErr != nil && !errors.Is(completionErr, sql.ErrNoRows) {
			targetErrors = append(targetErrors, fmt.Errorf("load durable action execution: %w", completionErr))
			hasTransientFailure = true
			continue
		}
		if err := store.markRunning(ctx, eventKey, target.ActionID); err != nil {
			targetErrors = append(targetErrors, err)
			hasTransientFailure = true
			continue
		}
		isPermanent, err := callbacks.Execute(target.ActionID)
		if err != nil {
			if !isPermanent {
				hasTransientFailure = true
			}
			if markErr := store.markFailed(ctx, eventKey, target.ActionID, err); markErr != nil {
				targetErrors = append(targetErrors, markErr)
				hasTransientFailure = true
			}
			targetErrors = append(targetErrors, err)
			continue
		}
		if err := store.markCompleted(ctx, eventKey, target.ActionID); err != nil {
			targetErrors = append(targetErrors, err)
			hasTransientFailure = true
			continue
		}
		executed++
	}
	if len(targetErrors) == 0 {
		return executed, nil
	}
	err = errors.Join(targetErrors...)
	if hasTransientFailure {
		return executed, err
	}
	return executed, events.Permanent(err)
}
