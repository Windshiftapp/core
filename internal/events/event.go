// Package events persists domain events and delivers them to durable consumers.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DeliveryState is the durable state of one event-consumer delivery.
type DeliveryState string

const (
	StatePending   DeliveryState = "pending"
	StateLeased    DeliveryState = "leased"
	StateRetry     DeliveryState = "retry"
	StateFailed    DeliveryState = "failed"
	StateCompleted DeliveryState = "completed"
	StateSkipped   DeliveryState = "skipped"
)

var (
	ErrLeaseLost        = errors.New("domain event delivery lease lost")
	ErrDeliveryState    = errors.New("domain event delivery is not in the required state")
	ErrEventMissing     = errors.New("domain event not found")
	ErrConsumerMissing  = errors.New("domain event consumer not found")
	ErrConsumerContract = errors.New("domain event consumer contract cannot change after delivery begins")
)

// Event is an immutable persisted domain fact.
type Event struct {
	ID                int64
	Key               string
	WorkspaceID       *int
	AggregateType     string
	AggregateID       string
	AggregateSequence int64
	Type              string
	PayloadVersion    int
	OccurredAt        time.Time
	RecordedAt        time.Time
	ActorKind         string
	ActorRef          string
	SourceKind        string
	SourceRef         string
	CorrelationID     string
	CausationEventKey string
	Payload           json.RawMessage
}

// NewEvent contains the caller-owned fields used to append an event.
type NewEvent struct {
	Key               string
	WorkspaceID       *int
	AggregateType     string
	AggregateID       string
	Type              string
	PayloadVersion    int
	OccurredAt        time.Time
	ActorKind         string
	ActorRef          string
	SourceKind        string
	SourceRef         string
	CorrelationID     string
	CausationEventKey string
	Payload           json.RawMessage
}

func (e NewEvent) validate() error {
	for field, value := range map[string]string{
		"aggregate_type": e.AggregateType,
		"aggregate_id":   e.AggregateID,
		"event_type":     e.Type,
		"actor_kind":     e.ActorKind,
		"source_kind":    e.SourceKind,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field)
		}
	}
	if e.PayloadVersion <= 0 {
		return errors.New("payload_version must be positive")
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return errors.New("payload must be valid JSON")
	}
	return nil
}

// Consumer defines a durable subscription. EventTypes accepts exact event
// names and "*" for all event types.
type Consumer struct {
	Key            string
	HandlerVersion int
	Active         bool
	StartEventID   int64
	EventTypes     []string
}

func (c Consumer) validate() error {
	if strings.TrimSpace(c.Key) == "" {
		return errors.New("consumer key is required")
	}
	if c.HandlerVersion <= 0 {
		return errors.New("handler version must be positive")
	}
	if c.StartEventID <= 0 {
		return errors.New("start event id must be positive")
	}
	if len(c.EventTypes) == 0 {
		return errors.New("at least one event type is required")
	}
	for _, eventType := range c.EventTypes {
		if strings.TrimSpace(eventType) == "" {
			return errors.New("event type must not be empty")
		}
	}
	return nil
}

// Delivery is one fenced claim returned to a worker.
type Delivery struct {
	Event          Event
	ConsumerKey    string
	AttemptCount   int
	LeaseOwner     string
	LeaseToken     string
	LeaseExpiresAt time.Time
}

// Operator identifies an administrator or system actor replaying or skipping
// a failed delivery.
type Operator struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref,omitempty"`
}

func (o Operator) validate() error {
	if strings.TrimSpace(o.Kind) == "" {
		return errors.New("operator kind is required")
	}
	return nil
}

// ConsumerStats summarizes durable queue health for one consumer.
type ConsumerStats struct {
	ConsumerKey       string
	Active            bool
	Pending           int64
	Leased            int64
	ActiveLeases      int64
	ExpiredLeases     int64
	Retrying          int64
	RetryAttempts     int64
	Failed            int64
	BlockedAggregates int64
	Completed         int64
	Skipped           int64
	OldestPendingAt   *time.Time
	OldestPendingAge  time.Duration
}

// DiagnosticsFilter narrows operational data without changing delivery state.
type DiagnosticsFilter struct {
	ConsumerKey string
	WorkspaceID *int
}

// FailedDelivery describes terminal work that requires replay or skip.
type FailedDelivery struct {
	EventID           int64
	EventKey          string
	WorkspaceID       *int
	ConsumerKey       string
	AggregateType     string
	AggregateID       string
	AggregateSequence int64
	EventType         string
	PayloadVersion    int
	OccurredAt        time.Time
	RecordedAt        time.Time
	AttemptCount      int
	LastError         string
	FailedAt          time.Time
}

// PruneResult reports rows removed by one bounded retention pass.
type PruneResult struct {
	Deliveries int64
	Events     int64
}

type permanentFailure struct {
	err error
}

func (e permanentFailure) Error() string { return e.err.Error() }
func (e permanentFailure) Unwrap() error { return e.err }

// Permanent marks a handler error as non-retryable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentFailure{err: err}
}

// IsPermanent reports whether an error disables automatic delivery retries.
func IsPermanent(err error) bool {
	var target permanentFailure
	return errors.As(err, &target)
}
