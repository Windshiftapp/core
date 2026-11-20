package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/events"
	"windshift/internal/sideeffects"
)

// Lane returns the bounded notification lane. The transaction-effects
// dispatcher stages canonical facts into it.
func (ns *NotificationService) Lane() *sideeffects.Lane { return ns.lane }

// admitEnvelope offers one encoded candidate to the notification lane without
// waiting. It reports whether the candidate was admitted.
func (ns *NotificationService) admitEnvelope(envelope notificationEnvelope, eventType, key string) bool {
	payload, err := json.Marshal(envelope)
	if err != nil {
		return false
	}
	admitted, _ := ns.lane.TryAdmit(sideeffects.Candidate{
		EventType:  eventType,
		Key:        key,
		Payload:    payload,
		OccurredAt: time.Now().UTC(),
	})
	return admitted
}

// handleLaneCandidate runs one admitted notification effect. Canonical facts
// are converted here so DB enrichment stays off the writer.
func (ns *NotificationService) handleLaneCandidate(ctx context.Context, candidate sideeffects.Candidate) error {
	var envelope notificationEnvelope
	if err := json.Unmarshal(candidate.Payload, &envelope); err != nil {
		return fmt.Errorf("decode notification candidate: %w", err)
	}
	switch envelope.Kind {
	case envelopeKindEvent:
		if envelope.Event == nil {
			return errors.New("notification candidate missing event")
		}
		return ns.processEvent(ctx, envelope.Event)
	case envelopeKindFact:
		if envelope.Fact == nil {
			return errors.New("notification candidate missing fact")
		}
		return ns.processFact(ctx, envelope.Fact)
	case envelopeKindSLA:
		if envelope.SLA == nil {
			return errors.New("notification candidate missing SLA occurrence")
		}
		return ns.processSLAOccurrence(ctx, *envelope.SLA)
	default:
		return fmt.Errorf("unknown notification candidate kind %q", envelope.Kind)
	}
}

// processFact converts a canonical fact and dispatches the resulting
// rule-based notifications.
func (ns *NotificationService) processFact(ctx context.Context, event *events.Event) error {
	converter := &DurableNotificationConsumer{db: ns.db, notifications: ns}
	notificationEvents, err := converter.notificationEvents(ctx, *event)
	if err != nil {
		return err
	}
	for _, notificationEvent := range notificationEvents {
		if err := ns.processEventWithKey(ctx, notificationEvent, event.Key); err != nil {
			return err
		}
	}
	return nil
}

// processSLAOccurrence enriches a staged SLA occurrence in the worker and
// dispatches the resulting rule-based notification. Enrichment happens after
// the SLA job committed, so a missing item or metric only drops the effect.
func (ns *NotificationService) processSLAOccurrence(ctx context.Context, occurrence SLAOccurrence) error {
	event, err := buildSLANotificationEvent(ctx, ns.db, occurrence)
	if err != nil {
		return err
	}
	if event == nil {
		return nil
	}
	return ns.processEventWithKey(ctx, event, "")
}
