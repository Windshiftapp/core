package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"windshift/internal/events"
	"windshift/internal/operatorcontrols"
	"windshift/internal/utils"
)

const (
	defaultDomainEventFailureLimit = 100
	maxDomainEventFailureLimit     = 500
	maxDomainEventReasonLength     = 2000
)

type domainEventDiagnosticsResponse struct {
	GeneratedAt time.Time                       `json:"generated_at"`
	Filter      domainEventDiagnosticsFilter    `json:"filter"`
	Consumers   []domainEventConsumerDiagnostic `json:"consumers"`
	Failures    []domainEventFailureDiagnostic  `json:"failures"`
}

type domainEventDiagnosticsFilter struct {
	ConsumerKey string `json:"consumer_key,omitempty"`
	WorkspaceID *int   `json:"workspace_id,omitempty"`
}

type domainEventConsumerDiagnostic struct {
	ConsumerKey             string     `json:"consumer_key"`
	Active                  bool       `json:"active"`
	Pending                 int64      `json:"pending"`
	Retrying                int64      `json:"retrying"`
	RetryAttempts           int64      `json:"retry_attempts"`
	ActiveLeases            int64      `json:"active_leases"`
	ExpiredLeases           int64      `json:"expired_leases"`
	TerminalFailures        int64      `json:"terminal_failures"`
	BlockedAggregates       int64      `json:"blocked_aggregates"`
	Completed               int64      `json:"completed"`
	Skipped                 int64      `json:"skipped"`
	OldestPendingAt         *time.Time `json:"oldest_pending_at,omitempty"`
	OldestPendingAgeSeconds int64      `json:"oldest_pending_age_seconds"`
}

type domainEventFailureDiagnostic struct {
	EventID           int64     `json:"event_id"`
	EventKey          string    `json:"event_key"`
	WorkspaceID       *int      `json:"workspace_id,omitempty"`
	ConsumerKey       string    `json:"consumer_key"`
	AggregateType     string    `json:"aggregate_type"`
	AggregateID       string    `json:"aggregate_id"`
	AggregateSequence int64     `json:"aggregate_sequence"`
	EventType         string    `json:"event_type"`
	PayloadVersion    int       `json:"payload_version"`
	OccurredAt        time.Time `json:"occurred_at"`
	RecordedAt        time.Time `json:"recorded_at"`
	AttemptCount      int       `json:"attempt_count"`
	LastError         string    `json:"last_error"`
	FailedAt          time.Time `json:"failed_at"`
}

// GetDomainEvents returns durable delivery health and terminal failures.
//
// GET /api/admin/diagnostics/domain-events
func (h *DiagnosticsHandler) GetDomainEvents(w http.ResponseWriter, r *http.Request) {
	if err := h.controls.Check(operatorcontrols.RuntimeDiagnostics); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if h.eventStore == nil {
		respondInternalError(w, r, errors.New("domain event store not configured"))
		return
	}
	filter, limit, ok := parseDomainEventDiagnosticsQuery(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	stats, err := h.eventStore.StatsFiltered(r.Context(), filter, now)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	failures, err := h.eventStore.FailedDeliveries(r.Context(), filter, limit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	consumers := make([]domainEventConsumerDiagnostic, 0, len(stats))
	for _, stat := range stats {
		consumers = append(consumers, domainEventConsumerDiagnostic{
			ConsumerKey:             stat.ConsumerKey,
			Active:                  stat.Active,
			Pending:                 stat.Pending,
			Retrying:                stat.Retrying,
			RetryAttempts:           stat.RetryAttempts,
			ActiveLeases:            stat.ActiveLeases,
			ExpiredLeases:           stat.ExpiredLeases,
			TerminalFailures:        stat.Failed,
			BlockedAggregates:       stat.BlockedAggregates,
			Completed:               stat.Completed,
			Skipped:                 stat.Skipped,
			OldestPendingAt:         stat.OldestPendingAt,
			OldestPendingAgeSeconds: int64(stat.OldestPendingAge / time.Second),
		})
	}
	failureRows := make([]domainEventFailureDiagnostic, 0, len(failures))
	for _, failure := range failures {
		failureRows = append(failureRows, domainEventFailureDiagnostic{
			EventID:           failure.EventID,
			EventKey:          failure.EventKey,
			WorkspaceID:       failure.WorkspaceID,
			ConsumerKey:       failure.ConsumerKey,
			AggregateType:     failure.AggregateType,
			AggregateID:       failure.AggregateID,
			AggregateSequence: failure.AggregateSequence,
			EventType:         failure.EventType,
			PayloadVersion:    failure.PayloadVersion,
			OccurredAt:        failure.OccurredAt,
			RecordedAt:        failure.RecordedAt,
			AttemptCount:      failure.AttemptCount,
			LastError:         failure.LastError,
			FailedAt:          failure.FailedAt,
		})
	}
	respondJSONOK(w, domainEventDiagnosticsResponse{
		GeneratedAt: now,
		Filter: domainEventDiagnosticsFilter{
			ConsumerKey: filter.ConsumerKey,
			WorkspaceID: filter.WorkspaceID,
		},
		Consumers: consumers,
		Failures:  failureRows,
	})
}

func parseDomainEventDiagnosticsQuery(
	w http.ResponseWriter,
	r *http.Request,
) (events.DiagnosticsFilter, int, bool) {
	query := r.URL.Query()
	filter := events.DiagnosticsFilter{ConsumerKey: strings.TrimSpace(query.Get("consumer_key"))}
	if workspaceValue := strings.TrimSpace(query.Get("workspace_id")); workspaceValue != "" {
		workspaceID, err := strconv.Atoi(workspaceValue)
		if err != nil || workspaceID <= 0 {
			respondValidationError(w, r, "workspace_id must be a positive integer")
			return events.DiagnosticsFilter{}, 0, false
		}
		filter.WorkspaceID = &workspaceID
	}
	limit := defaultDomainEventFailureLimit
	if limitValue := strings.TrimSpace(query.Get("limit")); limitValue != "" {
		parsed, err := strconv.Atoi(limitValue)
		if err != nil || parsed <= 0 || parsed > maxDomainEventFailureLimit {
			respondValidationError(w, r, "limit must be between 1 and 500")
			return events.DiagnosticsFilter{}, 0, false
		}
		limit = parsed
	}
	return filter, limit, true
}

type domainEventActionRequest struct {
	Reason string `json:"reason"`
}

type domainEventActionResponse struct {
	EventID        int64           `json:"event_id"`
	ConsumerKey    string          `json:"consumer_key"`
	Action         string          `json:"action"`
	Operator       events.Operator `json:"operator"`
	Reason         string          `json:"reason"`
	PerformedAt    time.Time       `json:"performed_at"`
	OrderingImpact string          `json:"ordering_impact"`
}

// ReplayDomainEvent schedules a terminal delivery for another attempt.
//
// POST /api/admin/diagnostics/domain-events/{eventID}/{consumerKey}/replay
func (h *DiagnosticsHandler) ReplayDomainEvent(w http.ResponseWriter, r *http.Request) {
	h.changeDomainEventDelivery(w, r, "replay")
}

// SkipDomainEvent explicitly unblocks a terminal delivery without handling it.
//
// POST /api/admin/diagnostics/domain-events/{eventID}/{consumerKey}/skip
func (h *DiagnosticsHandler) SkipDomainEvent(w http.ResponseWriter, r *http.Request) {
	h.changeDomainEventDelivery(w, r, "skip")
}

func (h *DiagnosticsHandler) changeDomainEventDelivery(w http.ResponseWriter, r *http.Request, action string) {
	if err := h.controls.Check(operatorcontrols.Maintenance); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if h.eventStore == nil {
		respondInternalError(w, r, errors.New("domain event store not configured"))
		return
	}
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}
	eventID, err := strconv.ParseInt(r.PathValue("eventID"), 10, 64)
	if err != nil || eventID <= 0 {
		respondValidationError(w, r, "eventID must be a positive integer")
		return
	}
	consumerKey := strings.TrimSpace(r.PathValue("consumerKey"))
	if consumerKey == "" {
		respondValidationError(w, r, "consumerKey is required")
		return
	}
	var request domainEventActionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		respondValidationError(w, r, "reason is required")
		return
	}
	if utf8.RuneCountInString(reason) > maxDomainEventReasonLength {
		respondValidationError(w, r, "reason must be at most 2000 characters")
		return
	}
	operator := events.Operator{Kind: "user", Ref: strconv.Itoa(user.ID)}
	now := time.Now().UTC()
	if action == "replay" {
		err = h.eventStore.Replay(r.Context(), eventID, consumerKey, operator, reason, now)
	} else {
		err = h.eventStore.Skip(r.Context(), eventID, consumerKey, operator, reason, now)
	}
	if err != nil {
		switch {
		case errors.Is(err, events.ErrEventMissing):
			respondNotFound(w, r, "Domain event")
		case errors.Is(err, events.ErrDeliveryState):
			respondConflict(w, r, "Domain event delivery is not in terminal failure state")
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	impact := "Delivery is eligible for retry; later events for this consumer and aggregate remain blocked until it completes or is skipped."
	if action == "skip" {
		impact = "Delivery was explicitly skipped; later events for this consumer and aggregate are unblocked."
	}
	respondJSONOK(w, domainEventActionResponse{
		EventID:        eventID,
		ConsumerKey:    consumerKey,
		Action:         action,
		Operator:       operator,
		Reason:         reason,
		PerformedAt:    now,
		OrderingImpact: impact,
	})
}
