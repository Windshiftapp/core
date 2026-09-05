package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"windshift/internal/database"

	"uuid"
)

const maxStoredErrorBytes = 4096

const maxAppendBatchSize = 1000

// Store owns durable event and delivery persistence.
type Store struct {
	db database.Database
}

// NewStore creates an event store over an initialized Windshift database.
func NewStore(db database.Database) *Store {
	return &Store{db: db}
}

// Append writes an event through the caller's source transaction.
func (s *Store) Append(ctx context.Context, tx database.Tx, input NewEvent) (*Event, error) {
	events, err := s.AppendBatch(ctx, tx, []NewEvent{input})
	if err != nil {
		return nil, err
	}
	return events[0], nil
}

// AppendBatch writes one event for each distinct aggregate using two
// set-based statements. All events remain part of the caller's transaction.
func (s *Store) AppendBatch(ctx context.Context, tx database.Tx, inputs []NewEvent) ([]*Event, error) {
	if tx == nil {
		return nil, errors.New("append domain events: transaction is required")
	}
	if len(inputs) == 0 {
		return []*Event{}, nil
	}
	prepared := slices.Clone(inputs)
	aggregates := make(map[string]struct{}, len(prepared))
	eventKeys := make(map[string]struct{}, len(prepared))
	for i := range prepared {
		input := &prepared[i]
		if err := input.validate(); err != nil {
			return nil, fmt.Errorf("append domain event %d: %w", i, err)
		}
		aggregateKey := input.AggregateType + "\x00" + input.AggregateID
		if _, duplicate := aggregates[aggregateKey]; duplicate {
			return nil, fmt.Errorf("append domain events: aggregate %q/%q appears more than once", input.AggregateType, input.AggregateID)
		}
		aggregates[aggregateKey] = struct{}{}
		if input.Key == "" {
			input.Key = uuid.New().String()
		}
		if _, duplicate := eventKeys[input.Key]; duplicate {
			return nil, fmt.Errorf("append domain events: event key %q appears more than once", input.Key)
		}
		eventKeys[input.Key] = struct{}{}
		if input.OccurredAt.IsZero() {
			input.OccurredAt = time.Now().UTC()
		}
	}
	if len(prepared) <= maxAppendBatchSize {
		return s.appendPreparedBatch(ctx, tx, prepared)
	}
	eventsOut := make([]*Event, 0, len(prepared))
	for start := 0; start < len(prepared); start += maxAppendBatchSize {
		end := min(start+maxAppendBatchSize, len(prepared))
		batch, err := s.appendPreparedBatch(ctx, tx, prepared[start:end])
		if err != nil {
			return nil, err
		}
		eventsOut = append(eventsOut, batch...)
	}
	return eventsOut, nil
}

func (s *Store) appendPreparedBatch(ctx context.Context, tx database.Tx, prepared []NewEvent) ([]*Event, error) {
	streamOrder := make([]int, len(prepared))
	for i := range streamOrder {
		streamOrder[i] = i
	}
	sort.Slice(streamOrder, func(i, j int) bool {
		left, right := prepared[streamOrder[i]], prepared[streamOrder[j]]
		if left.AggregateType != right.AggregateType {
			return left.AggregateType < right.AggregateType
		}
		return left.AggregateID < right.AggregateID
	})
	streamValues := make([]string, len(prepared))
	streamArgs := make([]any, 0, len(prepared)*2)
	for i, inputIndex := range streamOrder {
		input := prepared[inputIndex]
		streamValues[i] = "(?, ?, 1, CURRENT_TIMESTAMP)"
		streamArgs = append(streamArgs, input.AggregateType, input.AggregateID)
	}
	streamRows, err := tx.QueryContext(ctx, `
		INSERT INTO domain_event_streams (
			aggregate_type, aggregate_id, last_sequence, updated_at
		) VALUES `+strings.Join(streamValues, ",")+`
		ON CONFLICT (aggregate_type, aggregate_id) DO UPDATE SET
			last_sequence = domain_event_streams.last_sequence + 1,
			updated_at = CURRENT_TIMESTAMP
		RETURNING aggregate_type, aggregate_id, last_sequence
	`, streamArgs...)
	if err != nil {
		return nil, fmt.Errorf("allocate domain event sequences: %w", err)
	}
	sequences := make(map[string]int64, len(prepared))
	for streamRows.Next() {
		var aggregateType, aggregateID string
		var sequence int64
		if err := streamRows.Scan(&aggregateType, &aggregateID, &sequence); err != nil {
			_ = streamRows.Close()
			return nil, fmt.Errorf("scan allocated domain event sequence: %w", err)
		}
		sequences[aggregateType+"\x00"+aggregateID] = sequence
	}
	if err := streamRows.Close(); err != nil {
		return nil, fmt.Errorf("close allocated domain event sequences: %w", err)
	}
	if err := streamRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate allocated domain event sequences: %w", err)
	}

	eventsOut := make([]*Event, len(prepared))
	eventValues := make([]string, len(prepared))
	eventArgs := make([]any, 0, len(prepared)*15)
	for i, input := range prepared {
		sequence, ok := sequences[input.AggregateType+"\x00"+input.AggregateID]
		if !ok {
			return nil, fmt.Errorf("allocated domain event sequence missing for %q/%q", input.AggregateType, input.AggregateID)
		}
		event := &Event{
			Key: input.Key, WorkspaceID: input.WorkspaceID,
			AggregateType: input.AggregateType, AggregateID: input.AggregateID, AggregateSequence: sequence,
			Type: input.Type, PayloadVersion: input.PayloadVersion, OccurredAt: input.OccurredAt.UTC(),
			ActorKind: input.ActorKind, ActorRef: input.ActorRef,
			SourceKind: input.SourceKind, SourceRef: input.SourceRef,
			CorrelationID: input.CorrelationID, CausationEventKey: input.CausationEventKey,
			Payload: slices.Clone(input.Payload),
		}
		eventsOut[i] = event
		eventValues[i] = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
		eventArgs = append(eventArgs,
			event.Key, event.WorkspaceID, event.AggregateType, event.AggregateID,
			event.AggregateSequence, event.Type, event.PayloadVersion, event.OccurredAt,
			event.ActorKind, nullableString(event.ActorRef), event.SourceKind, nullableString(event.SourceRef),
			nullableString(event.CorrelationID), nullableString(event.CausationEventKey), string(event.Payload),
		)
	}
	eventRows, err := tx.QueryContext(ctx, `
		INSERT INTO domain_events (
			event_key, workspace_id, aggregate_type, aggregate_id,
			aggregate_sequence, event_type, payload_version, occurred_at,
			actor_kind, actor_ref, source_kind, source_ref,
			correlation_id, causation_event_key, payload
		) VALUES `+strings.Join(eventValues, ",")+`
		RETURNING event_key, id, recorded_at
	`, eventArgs...)
	if err != nil {
		return nil, fmt.Errorf("insert domain events: %w", err)
	}
	eventsByKey := make(map[string]*Event, len(eventsOut))
	for _, event := range eventsOut {
		eventsByKey[event.Key] = event
	}
	for eventRows.Next() {
		var key string
		var id int64
		var recordedAt time.Time
		if err := eventRows.Scan(&key, &id, &recordedAt); err != nil {
			_ = eventRows.Close()
			return nil, fmt.Errorf("scan inserted domain event: %w", err)
		}
		event, ok := eventsByKey[key]
		if !ok {
			_ = eventRows.Close()
			return nil, fmt.Errorf("inserted unexpected domain event %q", key)
		}
		event.ID = id
		event.RecordedAt = recordedAt
	}
	if err := eventRows.Close(); err != nil {
		return nil, fmt.Errorf("close inserted domain events: %w", err)
	}
	if err := eventRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inserted domain events: %w", err)
	}
	for _, event := range eventsOut {
		if event.ID == 0 {
			return nil, fmt.Errorf("inserted domain event %q was not returned", event.Key)
		}
	}
	return eventsOut, nil
}

// AppendStandalone appends an external fact in its own transaction.
func (s *Store) AppendStandalone(ctx context.Context, input NewEvent) (*Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin domain event transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	event, err := s.Append(ctx, tx, input)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit domain event: %w", err)
	}
	return event, nil
}

// Event loads one event by database ID.
func (s *Store) Event(ctx context.Context, eventID int64) (*Event, error) {
	return scanEvent(s.db.QueryRowContext(ctx, eventSelectSQL+" WHERE id = ?", eventID))
}

// ConfigureConsumer atomically replaces one consumer's durable subscription.
func (s *Store) ConfigureConsumer(ctx context.Context, consumer Consumer) error {
	consumer.EventTypes = uniqueSortedStrings(consumer.EventTypes)
	if err := consumer.validate(); err != nil {
		return fmt.Errorf("configure domain event consumer: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin consumer configuration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := verifyConsumerContract(ctx, tx, consumer); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO domain_event_consumers (
			consumer_key, handler_version, is_active, start_event_id, updated_at
		) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (consumer_key) DO UPDATE SET
			handler_version = excluded.handler_version,
			is_active = excluded.is_active,
			start_event_id = excluded.start_event_id,
			updated_at = CURRENT_TIMESTAMP
	`, consumer.Key, consumer.HandlerVersion, consumer.Active, consumer.StartEventID); err != nil {
		return fmt.Errorf("upsert domain event consumer %q: %w", consumer.Key, err)
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM domain_event_subscriptions WHERE consumer_key = ?",
		consumer.Key,
	); err != nil {
		return fmt.Errorf("replace domain event subscriptions for %q: %w", consumer.Key, err)
	}
	for _, eventType := range consumer.EventTypes {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO domain_event_subscriptions (consumer_key, event_type)
			VALUES (?, ?)
		`, consumer.Key, eventType); err != nil {
			return fmt.Errorf("subscribe consumer %q to %q: %w", consumer.Key, eventType, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit consumer configuration for %q: %w", consumer.Key, err)
	}
	return nil
}

func verifyConsumerContract(ctx context.Context, tx database.Tx, consumer Consumer) error {
	var existingStart int64
	err := tx.QueryRowContext(ctx, `
		SELECT start_event_id
		FROM domain_event_consumers
		WHERE consumer_key = ?
	`, consumer.Key).Scan(&existingStart)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load domain event consumer %q: %w", consumer.Key, err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT event_type
		FROM domain_event_subscriptions
		WHERE consumer_key = ?
		ORDER BY event_type
	`, consumer.Key)
	if err != nil {
		return fmt.Errorf("load domain event subscriptions for %q: %w", consumer.Key, err)
	}
	var existingTypes []string
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan domain event subscription for %q: %w", consumer.Key, err)
		}
		existingTypes = append(existingTypes, eventType)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close domain event subscriptions for %q: %w", consumer.Key, err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate domain event subscriptions for %q: %w", consumer.Key, err)
	}
	if existingStart == consumer.StartEventID && slices.Equal(existingTypes, consumer.EventTypes) {
		return nil
	}

	var progressed bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM domain_event_deliveries WHERE consumer_key = ?
			UNION ALL
			SELECT 1 FROM domain_event_consumer_streams WHERE consumer_key = ?
		)
	`, consumer.Key, consumer.Key).Scan(&progressed); err != nil {
		return fmt.Errorf("check domain event consumer %q progress: %w", consumer.Key, err)
	}
	if progressed {
		return fmt.Errorf("%w: %q; register a new consumer key", ErrConsumerContract, consumer.Key)
	}
	return nil
}

// SetConsumerActive changes admission without changing a consumer's filters.
func (s *Store) SetConsumerActive(ctx context.Context, consumerKey string, active bool) error {
	result, err := s.db.ExecWriteContext(ctx, `
		UPDATE domain_event_consumers
		SET is_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE consumer_key = ?
	`, active, consumerKey)
	if err != nil {
		return fmt.Errorf("set domain event consumer %q active=%t: %w", consumerKey, active, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read consumer activation result: %w", err)
	}
	if rows == 0 {
		return ErrConsumerMissing
	}
	return nil
}

// Reconcile creates missing deliveries from the durable subscription catalog.
func (s *Store) Reconcile(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		return 0, errors.New("reconcile limit must be positive")
	}

	query := `
		INSERT INTO domain_event_deliveries (
			event_id, consumer_key, state, next_attempt_at, created_at, updated_at
		)
		SELECT e.id, c.consumer_key, 'pending', CURRENT_TIMESTAMP,
		       CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		FROM domain_events e
		JOIN domain_event_consumers c
		  ON c.is_active = true AND e.id >= c.start_event_id
		LEFT JOIN domain_event_consumer_streams stream
		  ON stream.consumer_key = c.consumer_key
		 AND stream.aggregate_type = e.aggregate_type
		 AND stream.aggregate_id = e.aggregate_id
		WHERE e.aggregate_sequence > COALESCE(stream.completed_sequence, 0)
		  AND EXISTS (
			SELECT 1 FROM domain_event_subscriptions subscription
			WHERE subscription.consumer_key = c.consumer_key
			  AND subscription.event_type IN (e.event_type, '*')
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM domain_event_deliveries delivery
			WHERE delivery.event_id = e.id
			  AND delivery.consumer_key = c.consumer_key
		  )
		ORDER BY e.id, c.consumer_key
		LIMIT ?
	`
	if s.db.GetDriverName() == "sqlite" {
		query = strings.Replace(query, "INSERT INTO domain_event_deliveries", "INSERT OR IGNORE INTO domain_event_deliveries", 1)
	} else {
		query += " ON CONFLICT (event_id, consumer_key) DO NOTHING"
	}
	result, err := s.db.ExecWriteContext(ctx, query, limit)
	if err != nil {
		return 0, fmt.Errorf("reconcile domain event deliveries: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read reconciled delivery count: %w", err)
	}
	return rows, nil
}

// Claim leases the next causally eligible delivery for one consumer.
func (s *Store) Claim(
	ctx context.Context,
	consumerKey string,
	owner string,
	now time.Time,
	leaseDuration time.Duration,
) (*Delivery, error) {
	if strings.TrimSpace(consumerKey) == "" || strings.TrimSpace(owner) == "" {
		return nil, errors.New("consumer key and lease owner are required")
	}
	if leaseDuration <= 0 {
		return nil, errors.New("lease duration must be positive")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin domain event claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `
		SELECT delivery.event_id, delivery.attempt_count
		FROM domain_event_deliveries delivery
		JOIN domain_events event ON event.id = delivery.event_id
		JOIN domain_event_consumers consumer
		  ON consumer.consumer_key = delivery.consumer_key
		WHERE delivery.consumer_key = ?
		  AND consumer.is_active = true
		  AND (
			(delivery.state IN ('pending', 'retry') AND delivery.next_attempt_at <= ?)
			OR (delivery.state = 'leased' AND delivery.lease_expires_at <= ?)
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM domain_event_deliveries earlier_delivery
			JOIN domain_events earlier_event ON earlier_event.id = earlier_delivery.event_id
			WHERE earlier_delivery.consumer_key = delivery.consumer_key
			  AND earlier_event.aggregate_type = event.aggregate_type
			  AND earlier_event.aggregate_id = event.aggregate_id
			  AND earlier_event.aggregate_sequence < event.aggregate_sequence
			  AND earlier_delivery.state NOT IN ('completed', 'skipped')
		  )
		ORDER BY delivery.next_attempt_at, event.id
		LIMIT 1
	`
	if s.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE OF delivery SKIP LOCKED"
	}

	var eventID int64
	var previousAttempts int
	if err := tx.QueryRowContext(ctx, query, consumerKey, now.UTC(), now.UTC()).Scan(&eventID, &previousAttempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("select domain event delivery for %q: %w", consumerKey, err)
	}

	token := uuid.New().String()
	expiresAt := now.UTC().Add(leaseDuration)
	result, err := tx.ExecContext(ctx, `
		UPDATE domain_event_deliveries
		SET state = 'leased', attempt_count = attempt_count + 1,
		    lease_owner = ?, lease_token = ?, lease_expires_at = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE event_id = ? AND consumer_key = ?
		  AND (
			(state IN ('pending', 'retry') AND next_attempt_at <= ?)
			OR (state = 'leased' AND lease_expires_at <= ?)
		  )
	`, owner, token, expiresAt, eventID, consumerKey, now.UTC(), now.UTC())
	if err != nil {
		return nil, fmt.Errorf("lease domain event delivery: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read domain event lease result: %w", err)
	}
	if rows != 1 {
		return nil, ErrLeaseLost
	}

	event, err := scanEvent(tx.QueryRowContext(ctx, eventSelectSQL+" WHERE id = ?", eventID))
	if err != nil {
		return nil, fmt.Errorf("load leased domain event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit domain event lease: %w", err)
	}
	return &Delivery{
		Event:          *event,
		ConsumerKey:    consumerKey,
		AttemptCount:   previousAttempts + 1,
		LeaseOwner:     owner,
		LeaseToken:     token,
		LeaseExpiresAt: expiresAt,
	}, nil
}

// Complete acknowledges a fenced delivery and advances its stream checkpoint.
func (s *Store) Complete(ctx context.Context, delivery Delivery, completedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin domain event completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE domain_event_deliveries
		SET state = 'completed', completed_at = ?, last_error = NULL,
		    lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
		    updated_at = CURRENT_TIMESTAMP
		WHERE event_id = ? AND consumer_key = ? AND state = 'leased'
		  AND lease_owner = ? AND lease_token = ?
	`, completedAt.UTC(), delivery.Event.ID, delivery.ConsumerKey, delivery.LeaseOwner, delivery.LeaseToken)
	if err != nil {
		return fmt.Errorf("complete domain event delivery: %w", err)
	}
	if err := requireOneAffected(result, ErrLeaseLost); err != nil {
		return err
	}
	if err := advanceCheckpoint(ctx, tx, delivery.ConsumerKey, delivery.Event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit domain event completion: %w", err)
	}
	return nil
}

// Renew extends a live fenced lease. An expired or reclaimed lease cannot be
// revived by its previous owner.
func (s *Store) Renew(ctx context.Context, delivery Delivery, now time.Time, leaseDuration time.Duration) (time.Time, error) {
	if leaseDuration <= 0 {
		return time.Time{}, errors.New("lease duration must be positive")
	}
	expiresAt := now.UTC().Add(leaseDuration)
	result, err := s.db.ExecWriteContext(ctx, `
		UPDATE domain_event_deliveries
		SET lease_expires_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE event_id = ? AND consumer_key = ? AND state = 'leased'
		  AND lease_owner = ? AND lease_token = ? AND lease_expires_at > ?
	`, expiresAt, delivery.Event.ID, delivery.ConsumerKey, delivery.LeaseOwner, delivery.LeaseToken, now.UTC())
	if err != nil {
		return time.Time{}, fmt.Errorf("renew domain event delivery lease: %w", err)
	}
	if err := requireOneAffected(result, ErrLeaseLost); err != nil {
		return time.Time{}, err
	}
	return expiresAt, nil
}

// Fail releases a fenced lease into retry or terminal failure state.
func (s *Store) Fail(
	ctx context.Context,
	delivery Delivery,
	deliveryErr error,
	retry bool,
	nextAttemptAt time.Time,
) error {
	state := StateFailed
	if retry {
		state = StateRetry
	}
	message := "delivery failed"
	if deliveryErr != nil {
		message = deliveryErr.Error()
	}
	message = truncateUTF8(message, maxStoredErrorBytes)

	result, err := s.db.ExecWriteContext(ctx, `
		UPDATE domain_event_deliveries
		SET state = ?, next_attempt_at = ?, last_error = ?,
		    lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
		    updated_at = CURRENT_TIMESTAMP
		WHERE event_id = ? AND consumer_key = ? AND state = 'leased'
		  AND lease_owner = ? AND lease_token = ?
	`, state, nextAttemptAt.UTC(), message, delivery.Event.ID, delivery.ConsumerKey, delivery.LeaseOwner, delivery.LeaseToken)
	if err != nil {
		return fmt.Errorf("fail domain event delivery: %w", err)
	}
	return requireOneAffected(result, ErrLeaseLost)
}

// Replay schedules one failed delivery again and records the operator action.
func (s *Store) Replay(
	ctx context.Context,
	eventID int64,
	consumerKey string,
	operator Operator,
	reason string,
	now time.Time,
) error {
	if err := operator.validate(); err != nil {
		return fmt.Errorf("replay domain event delivery: %w", err)
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("replay domain event delivery: reason is required")
	}
	return s.changeFailedDelivery(ctx, eventID, consumerKey, operator, reason, now, "replay")
}

// Skip unblocks a failed aggregate after recording an operator and reason.
func (s *Store) Skip(
	ctx context.Context,
	eventID int64,
	consumerKey string,
	operator Operator,
	reason string,
	now time.Time,
) error {
	if err := operator.validate(); err != nil {
		return fmt.Errorf("skip domain event delivery: %w", err)
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("skip domain event delivery: reason is required")
	}
	return s.changeFailedDelivery(ctx, eventID, consumerKey, operator, reason, now, "skip")
}

func (s *Store) changeFailedDelivery(
	ctx context.Context,
	eventID int64,
	consumerKey string,
	operator Operator,
	reason string,
	now time.Time,
	action string,
) error {
	reason = strings.TrimSpace(reason)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin domain event %s: %w", action, err)
	}
	defer func() { _ = tx.Rollback() }()

	event, err := scanEvent(tx.QueryRowContext(ctx, eventSelectSQL+" WHERE id = ?", eventID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %w", ErrEventMissing, err)
		}
		return fmt.Errorf("load domain event for %s: %w", action, err)
	}

	var result sql.Result
	if action == "replay" {
		result, err = tx.ExecContext(ctx, `
			UPDATE domain_event_deliveries
			SET state = 'retry', attempt_count = 0, next_attempt_at = ?,
			    last_error = NULL, completed_at = NULL,
			    lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
			    updated_at = CURRENT_TIMESTAMP
			WHERE event_id = ? AND consumer_key = ? AND state = 'failed'
		`, now.UTC(), eventID, consumerKey)
	} else {
		result, err = tx.ExecContext(ctx, `
			UPDATE domain_event_deliveries
			SET state = 'skipped', completed_at = ?, skipped_by_kind = ?,
			    skipped_by_ref = ?, skip_reason = ?, skipped_at = ?,
			    lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
			    updated_at = CURRENT_TIMESTAMP
			WHERE event_id = ? AND consumer_key = ? AND state = 'failed'
		`, now.UTC(), operator.Kind, nullableString(operator.Ref), reason, now.UTC(), eventID, consumerKey)
	}
	if err != nil {
		return fmt.Errorf("%s domain event delivery: %w", action, err)
	}
	if err := requireOneAffected(result, ErrDeliveryState); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO domain_event_delivery_actions (
			event_id, consumer_key, action, operator_kind, operator_ref, reason, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, eventID, consumerKey, action, operator.Kind, nullableString(operator.Ref), nullableString(strings.TrimSpace(reason)), now.UTC()); err != nil {
		return fmt.Errorf("audit domain event %s: %w", action, err)
	}
	if action == "skip" {
		if err := advanceCheckpoint(ctx, tx, consumerKey, *event); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit domain event %s: %w", action, err)
	}
	return nil
}

// Stats returns queue-health counters for configured consumers.
func (s *Store) Stats(ctx context.Context, now time.Time) ([]ConsumerStats, error) {
	return s.StatsFiltered(ctx, DiagnosticsFilter{}, now)
}

// StatsFiltered returns queue health by consumer within an optional workspace.
func (s *Store) StatsFiltered(
	ctx context.Context,
	filter DiagnosticsFilter,
	now time.Time,
) ([]ConsumerStats, error) {
	workspaceID := nullableInt(filter.WorkspaceID)
	rows, err := s.db.QueryContext(ctx, `
		SELECT consumer.consumer_key, consumer.is_active,
		       COALESCE(SUM(CASE WHEN delivery.state = 'pending' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN delivery.state = 'leased' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN delivery.state = 'leased' AND delivery.lease_expires_at > ? THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN delivery.state = 'leased' AND (delivery.lease_expires_at IS NULL OR delivery.lease_expires_at <= ?) THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN delivery.state = 'retry' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN delivery.state = 'retry' THEN delivery.attempt_count ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN delivery.state = 'failed' THEN 1 ELSE 0 END), 0),
		       COUNT(DISTINCT CASE WHEN delivery.state = 'failed'
		                           THEN CAST(LENGTH(delivery.aggregate_type) AS TEXT) || ':' ||
		                                delivery.aggregate_type || delivery.aggregate_id END),
		       COALESCE(SUM(CASE WHEN delivery.state = 'completed' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN delivery.state = 'skipped' THEN 1 ELSE 0 END), 0),
		       MIN(CASE WHEN delivery.state IN ('pending', 'leased', 'retry', 'failed')
		                THEN event.recorded_at END)
		FROM domain_event_consumers consumer
		LEFT JOIN (
			SELECT queued.*, queued_event.aggregate_type, queued_event.aggregate_id
			FROM domain_event_deliveries queued
			JOIN domain_events queued_event ON queued_event.id = queued.event_id
			WHERE (CAST(? AS INTEGER) IS NULL OR queued_event.workspace_id = ?)
		) delivery ON delivery.consumer_key = consumer.consumer_key
		LEFT JOIN domain_events event ON event.id = delivery.event_id
		WHERE (? = '' OR consumer.consumer_key = ?)
		GROUP BY consumer.consumer_key, consumer.is_active
		ORDER BY consumer.consumer_key
	`, now.UTC(), now.UTC(), workspaceID, workspaceID, filter.ConsumerKey, filter.ConsumerKey)
	if err != nil {
		return nil, fmt.Errorf("query domain event stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []ConsumerStats
	for rows.Next() {
		var value ConsumerStats
		var oldest nullableDBTime
		if err := rows.Scan(
			&value.ConsumerKey,
			&value.Active,
			&value.Pending,
			&value.Leased,
			&value.ActiveLeases,
			&value.ExpiredLeases,
			&value.Retrying,
			&value.RetryAttempts,
			&value.Failed,
			&value.BlockedAggregates,
			&value.Completed,
			&value.Skipped,
			&oldest,
		); err != nil {
			return nil, fmt.Errorf("scan domain event stats: %w", err)
		}
		if oldest.Valid {
			oldestAt := oldest.Time.UTC()
			value.OldestPendingAt = &oldestAt
			value.OldestPendingAge = max(now.UTC().Sub(oldestAt), 0)
		}
		stats = append(stats, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate domain event stats: %w", err)
	}
	return stats, nil
}

// FailedDeliveries lists terminal failures in deterministic event order.
func (s *Store) FailedDeliveries(
	ctx context.Context,
	filter DiagnosticsFilter,
	limit int,
) ([]FailedDelivery, error) {
	if limit <= 0 {
		return nil, errors.New("failed delivery limit must be positive")
	}
	workspaceID := nullableInt(filter.WorkspaceID)
	rows, err := s.db.QueryContext(ctx, `
		SELECT event.id, event.event_key, event.workspace_id, delivery.consumer_key,
		       event.aggregate_type, event.aggregate_id, event.aggregate_sequence,
		       event.event_type, event.payload_version, event.occurred_at,
		       event.recorded_at, delivery.attempt_count, delivery.last_error,
		       delivery.updated_at
		FROM domain_event_deliveries delivery
		JOIN domain_events event ON event.id = delivery.event_id
		WHERE delivery.state = 'failed'
		  AND (? = '' OR delivery.consumer_key = ?)
		  AND (CAST(? AS INTEGER) IS NULL OR event.workspace_id = ?)
		ORDER BY event.id, delivery.consumer_key
		LIMIT ?
	`, filter.ConsumerKey, filter.ConsumerKey, workspaceID, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query failed domain event deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	failures := make([]FailedDelivery, 0)
	for rows.Next() {
		var failure FailedDelivery
		var workspace sql.NullInt64
		var lastError sql.NullString
		if err := rows.Scan(
			&failure.EventID,
			&failure.EventKey,
			&workspace,
			&failure.ConsumerKey,
			&failure.AggregateType,
			&failure.AggregateID,
			&failure.AggregateSequence,
			&failure.EventType,
			&failure.PayloadVersion,
			&failure.OccurredAt,
			&failure.RecordedAt,
			&failure.AttemptCount,
			&lastError,
			&failure.FailedAt,
		); err != nil {
			return nil, fmt.Errorf("scan failed domain event delivery: %w", err)
		}
		if workspace.Valid {
			value := int(workspace.Int64)
			failure.WorkspaceID = &value
		}
		failure.LastError = lastError.String
		failure.OccurredAt = failure.OccurredAt.UTC()
		failure.RecordedAt = failure.RecordedAt.UTC()
		failure.FailedAt = failure.FailedAt.UTC()
		failures = append(failures, failure)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate failed domain event deliveries: %w", err)
	}
	return failures, nil
}

// Prune removes completed deliveries and fully consumed events in bounded batches.
func (s *Store) Prune(
	ctx context.Context,
	completedBefore time.Time,
	eventsBefore time.Time,
	limit int,
) (PruneResult, error) {
	if limit <= 0 {
		return PruneResult{}, errors.New("prune limit must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PruneResult{}, fmt.Errorf("begin domain event retention: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	deliveryResult, err := tx.ExecContext(ctx, `
		DELETE FROM domain_event_deliveries
		WHERE (event_id, consumer_key) IN (
			SELECT event_id, consumer_key
			FROM domain_event_deliveries
			WHERE state IN ('completed', 'skipped') AND updated_at < ?
			ORDER BY updated_at, event_id, consumer_key
			LIMIT ?
		)
	`, completedBefore.UTC(), limit)
	if err != nil {
		return PruneResult{}, fmt.Errorf("prune domain event deliveries: %w", err)
	}

	eventResult, err := tx.ExecContext(ctx, `
		DELETE FROM domain_events
		WHERE id IN (
			SELECT event.id
			FROM domain_events event
			WHERE event.recorded_at < ?
			  AND NOT EXISTS (
				SELECT 1 FROM domain_event_deliveries delivery
				WHERE delivery.event_id = event.id
			  )
			  AND NOT EXISTS (
				SELECT 1
				FROM domain_event_consumers consumer
				LEFT JOIN domain_event_consumer_streams stream
				  ON stream.consumer_key = consumer.consumer_key
				 AND stream.aggregate_type = event.aggregate_type
				 AND stream.aggregate_id = event.aggregate_id
				WHERE consumer.is_active = true
				  AND event.id >= consumer.start_event_id
				  AND EXISTS (
					SELECT 1 FROM domain_event_subscriptions subscription
					WHERE subscription.consumer_key = consumer.consumer_key
					  AND subscription.event_type IN (event.event_type, '*')
				  )
				  AND COALESCE(stream.completed_sequence, 0) < event.aggregate_sequence
			  )
			ORDER BY event.id
			LIMIT ?
		)
	`, eventsBefore.UTC(), limit)
	if err != nil {
		return PruneResult{}, fmt.Errorf("prune domain events: %w", err)
	}

	deliveries, err := deliveryResult.RowsAffected()
	if err != nil {
		return PruneResult{}, fmt.Errorf("read pruned delivery count: %w", err)
	}
	events, err := eventResult.RowsAffected()
	if err != nil {
		return PruneResult{}, fmt.Errorf("read pruned event count: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PruneResult{}, fmt.Errorf("commit domain event retention: %w", err)
	}
	return PruneResult{Deliveries: deliveries, Events: events}, nil
}

const eventSelectSQL = `
	SELECT id, event_key, workspace_id, aggregate_type, aggregate_id,
	       aggregate_sequence, event_type, payload_version, occurred_at,
	       recorded_at, actor_kind, actor_ref, source_kind, source_ref,
	       correlation_id, causation_event_key, payload
	FROM domain_events`

type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(row scanner) (*Event, error) {
	var event Event
	var workspaceID sql.NullInt64
	var actorRef, sourceRef, correlationID, causationKey sql.NullString
	var payload string
	if err := row.Scan(
		&event.ID,
		&event.Key,
		&workspaceID,
		&event.AggregateType,
		&event.AggregateID,
		&event.AggregateSequence,
		&event.Type,
		&event.PayloadVersion,
		&event.OccurredAt,
		&event.RecordedAt,
		&event.ActorKind,
		&actorRef,
		&event.SourceKind,
		&sourceRef,
		&correlationID,
		&causationKey,
		&payload,
	); err != nil {
		return nil, err
	}
	if workspaceID.Valid {
		value := int(workspaceID.Int64)
		event.WorkspaceID = &value
	}
	event.ActorRef = actorRef.String
	event.SourceRef = sourceRef.String
	event.CorrelationID = correlationID.String
	event.CausationEventKey = causationKey.String
	event.Payload = []byte(payload)
	return &event, nil
}

func advanceCheckpoint(ctx context.Context, tx database.Tx, consumerKey string, event Event) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO domain_event_consumer_streams (
			consumer_key, aggregate_type, aggregate_id, completed_sequence, updated_at
		) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (consumer_key, aggregate_type, aggregate_id) DO UPDATE SET
			completed_sequence = CASE
				WHEN excluded.completed_sequence > domain_event_consumer_streams.completed_sequence
				THEN excluded.completed_sequence
				ELSE domain_event_consumer_streams.completed_sequence
			END,
			updated_at = CURRENT_TIMESTAMP
	`, consumerKey, event.AggregateType, event.AggregateID, event.AggregateSequence)
	if err != nil {
		return fmt.Errorf("advance domain event consumer checkpoint: %w", err)
	}
	return nil
}

func requireOneAffected(result sql.Result, missing error) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read domain event update result: %w", err)
	}
	if rows != 1 {
		return missing
	}
	return nil
}

func uniqueSortedStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	values = values[:0]
	for value := range set {
		values = append(values, value)
	}
	slices.Sort(values)
	return values
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

type nullableDBTime struct {
	Time  time.Time
	Valid bool
}

func (value *nullableDBTime) Scan(source any) error {
	switch typed := source.(type) {
	case nil:
		value.Time = time.Time{}
		value.Valid = false
		return nil
	case time.Time:
		value.Time = typed
		value.Valid = true
		return nil
	case string:
		return value.scanString(typed)
	case []byte:
		return value.scanString(string(typed))
	default:
		return fmt.Errorf("scan domain event timestamp from %T", source)
	}
}

func (value *nullableDBTime) scanString(source string) error {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		parsed, err := time.Parse(layout, source)
		if err == nil {
			value.Time = parsed
			value.Valid = true
			return nil
		}
	}
	return fmt.Errorf("scan domain event timestamp %q", source)
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
