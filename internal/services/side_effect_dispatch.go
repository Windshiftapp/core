package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"windshift/internal/assetevents"
	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/sideeffects"
)

// Candidate envelope kinds. The notification and automation lanes share a
// single bounded queue each, so the payload tags what the worker must do.
const (
	envelopeKindEvent       = "event"
	envelopeKindFact        = "fact"
	envelopeKindAction      = "action"
	envelopeKindAssetAction = "asset_action"
)

// notificationEnvelope carries either an already-built notification event
// (approvals, reminders, SLA) or a canonical fact to convert in the worker.
type notificationEnvelope struct {
	Kind  string             `json:"kind"`
	Event *NotificationEvent `json:"event,omitempty"`
	Fact  *events.Event      `json:"fact,omitempty"`
}

// automationEnvelope carries a canonical fact, an SCM observation, or an SLA
// action trigger for the shared automation lane.
type automationEnvelope struct {
	Kind        string                   `json:"kind"`
	Fact        *events.Event            `json:"fact,omitempty"`
	Action      *models.ActionEvent      `json:"action,omitempty"`
	AssetAction *models.AssetActionEvent `json:"asset_action,omitempty"`
}

// actionEventStager stages a workspace action trigger inside the caller's
// transaction. The SCM ledger and SLA jobs use it.
type actionEventStager interface {
	StageActionEvent(event *models.ActionEvent)
}

// notificationEventStager stages a notification event inside the caller's
// transaction. SLA jobs use it.
type notificationEventStager interface {
	StageNotificationEvent(event *NotificationEvent)
}

// RetiredConsumerKeys lists the durable subscriptions replaced by the
// in-memory lanes. Startup deactivates them before the durable engine runs so
// they cannot be claimed or reactivated.
func RetiredConsumerKeys() []string {
	return []string{
		DurableItemNotificationConsumerKey,
		DurableNotificationRequestedConsumerKey,
		DurableItemActionConsumerKey,
		DurableSideEffectActionConsumerKey,
		DurableSCMActionConsumerKey,
		DurableAssetActionConsumerKey,
		DurableAssetSideEffectActionConsumerKey,
	}
}

// DeactivateRetiredConsumers prevents the durable engine from claiming work
// for the retired lanes. Missing rows are ignored; historical deliveries are
// left intact for the bounded cleanup pass.
func DeactivateRetiredConsumers(ctx context.Context, store *events.Store) error {
	if store == nil {
		return nil
	}
	for _, key := range RetiredConsumerKeys() {
		if err := store.SetConsumerActive(ctx, key, false); err != nil && !errors.Is(err, events.ErrConsumerMissing) {
			return fmt.Errorf("deactivate retired consumer %s: %w", key, err)
		}
	}
	return nil
}

// SideEffectBudget bounds the process-local notification and automation lanes
// and the per-transaction staging buffer. Historical buffer sizes are sizing
// references, not measurements; WI-2274 owns the final numbers.
type SideEffectBudget struct {
	NotificationCapacity int
	NotificationBytes    int64
	NotificationWorkers  int

	AutomationCapacity int
	AutomationBytes    int64
	AutomationWorkers  int

	MaxPayloadBytes  int
	MaxEventAge      time.Duration
	MaxStagedEvents  int
	MaxStagedBytes   int64
	ExecutionTimeout time.Duration
	ShutdownTimeout  time.Duration
}

// DefaultSideEffectBudget returns the starting budgets. Notification limits
// mirror NotificationService's historical queue; the automation lane is a
// single worker shared by workspace and asset triggers.
func DefaultSideEffectBudget() SideEffectBudget {
	return SideEffectBudget{
		NotificationCapacity: 1000,
		NotificationBytes:    8 << 20,
		NotificationWorkers:  4,
		AutomationCapacity:   500,
		AutomationBytes:      8 << 20,
		AutomationWorkers:    1,
		MaxPayloadBytes:      256 << 10,
		MaxEventAge:          5 * time.Minute,
		MaxStagedEvents:      256,
		MaxStagedBytes:       1 << 20,
		ExecutionTimeout:     15 * time.Second,
		ShutdownTimeout:      20 * time.Second,
	}
}

// SideEffectDispatcher owns the shared automation lane and builds the
// per-transaction staging buffer that both lanes publish through. The
// notification lane is owned by NotificationService; the dispatcher stages
// facts into it.
type SideEffectDispatcher struct {
	db            database.Database
	notifications *NotificationService
	actions       *ActionService
	assetActions  *AssetActionService
	budget        SideEffectBudget

	automationLane *sideeffects.Lane
	startOnce      sync.Once
	stopOnce       sync.Once
}

// NewSideEffectDispatcher wires the automation lane. The notification lane
// already exists on the notification service.
func NewSideEffectDispatcher(
	db database.Database,
	notifications *NotificationService,
	actions *ActionService,
	assetActions *AssetActionService,
	budget SideEffectBudget,
) (*SideEffectDispatcher, error) {
	if notifications == nil {
		return nil, errors.New("notification service is required")
	}
	if actions == nil {
		return nil, errors.New("action service is required")
	}
	if assetActions == nil {
		return nil, errors.New("asset action service is required")
	}
	dispatcher := &SideEffectDispatcher{
		db:            db,
		notifications: notifications,
		actions:       actions,
		assetActions:  assetActions,
		budget:        budget,
	}
	lane, err := sideeffects.NewLane(sideeffects.LaneConfig{
		Name:             "automation",
		Capacity:         budget.AutomationCapacity,
		ByteCapacity:     budget.AutomationBytes,
		MaxPayloadBytes:  budget.MaxPayloadBytes,
		MaxEventAge:      budget.MaxEventAge,
		Workers:          budget.AutomationWorkers,
		ExecutionTimeout: budget.ExecutionTimeout,
		Handler:          sideeffects.HandlerFunc(dispatcher.handleAutomationCandidate),
	})
	if err != nil {
		return nil, fmt.Errorf("create automation lane: %w", err)
	}
	dispatcher.automationLane = lane
	actions.SetAutomationLane(lane)
	// Canonical item notifications now come from staged facts, so the legacy
	// coordinator's direct emits become no-ops.
	notifications.canonicalFacts = true
	return dispatcher, nil
}

// Start launches the automation worker. The notification lane starts with the
// notification service; the server calls Start after all producers are wired.
func (d *SideEffectDispatcher) Start(ctx context.Context) {
	d.startOnce.Do(func() { d.automationLane.Start(ctx) })
}

// Shutdown drains then discards the automation lane. The notification lane is
// shut down by NotificationService.CloseContext.
func (d *SideEffectDispatcher) Shutdown(ctx context.Context) {
	d.stopOnce.Do(func() { _ = d.automationLane.Shutdown(ctx) })
}

// NewTransactionEffects builds one transaction's staging buffer. It is
// registered with database.SetTxEffectsFactory at startup.
func (d *SideEffectDispatcher) NewTransactionEffects() database.TxEffects {
	staging, err := sideeffects.NewStaging(sideeffects.StagingConfig{
		MaxEvents:       d.budget.MaxStagedEvents,
		MaxBytes:        d.budget.MaxStagedBytes,
		MaxPayloadBytes: d.budget.MaxPayloadBytes,
		MaxEventAge:     d.budget.MaxEventAge,
	})
	if err != nil {
		slog.Error("failed to create side-effect staging", slog.String("component", "sideeffects"), slog.Any("error", err))
		return nil
	}
	return &transactionEffects{
		staging:          staging,
		notificationLane: d.notifications.Lane(),
		automationLane:   d.automationLane,
	}
}

// Stats returns the automation lane's counters for operator diagnostics.
func (d *SideEffectDispatcher) Stats() sideeffects.LaneStats {
	return d.automationLane.Stats()
}

// Diagnostics reports per-lane limits, staged/queued/active counts and bytes,
// queue age, execution duration, drops by reason, and shutdown discards. It
// distinguishes a dropped trigger from a started action that failed.
func (d *SideEffectDispatcher) Diagnostics() map[string]any {
	automation := d.automationLane.Stats()
	dropped := make(map[string]int64, len(automation.Dropped))
	for reason, count := range automation.Dropped {
		dropped[string(reason)] = count
	}
	return map[string]any{
		"notification": d.notifications.GetStats(),
		"automation": map[string]any{
			"capacity":           automation.Capacity,
			"byte_capacity":      automation.ByteCapacity,
			"queued":             automation.Queued,
			"queued_bytes":       automation.QueuedBytes,
			"active":             automation.Active,
			"count_used":         automation.CountUsed,
			"bytes_used":         automation.BytesUsed,
			"admitted":           automation.Admitted,
			"processed":          automation.Processed,
			"failed":             automation.Failed,
			"timed_out":          automation.TimedOut,
			"shutdown_discarded": automation.ShutdownDiscarded,
			"dropped":            dropped,
			"last_queue_wait_ms": automation.LastQueueWait.Milliseconds(),
			"max_queue_wait_ms":  automation.MaxQueueWait.Milliseconds(),
			"last_execution_ms":  automation.LastExecution.Milliseconds(),
			"max_execution_ms":   automation.MaxExecution.Milliseconds(),
		},
	}
}

// transactionEffects stages bounded immutable candidates during one
// transaction and publishes them once after the outer commit succeeds.
type transactionEffects struct {
	staging          *sideeffects.Staging
	notificationLane *sideeffects.Lane
	automationLane   *sideeffects.Lane
}

func (e *transactionEffects) Publish() { e.staging.Publish() }
func (e *transactionEffects) Discard() { e.staging.Discard() }

// StageFact routes a canonical fact to every eligible lane. A fact may feed
// both lanes; each lane has an independent budget.
func (e *transactionEffects) StageFact(candidate sideeffects.Candidate) {
	var fact events.Event
	if err := json.Unmarshal(candidate.Payload, &fact); err != nil {
		return
	}
	if isNotificationFact(candidate.EventType) && e.notificationLane != nil {
		e.stage(e.notificationLane, notificationEnvelope{Kind: envelopeKindFact, Fact: &fact}, candidate)
	}
	if isAutomationFact(candidate.EventType) && e.automationLane != nil {
		e.stage(e.automationLane, automationEnvelope{Kind: envelopeKindFact, Fact: &fact}, candidate)
	}
}

// StageActionEvent stages an SCM observation or SLA trigger for the automation
// lane inside the caller's transaction.
func (e *transactionEffects) StageActionEvent(event *models.ActionEvent) {
	if event == nil || e.automationLane == nil {
		return
	}
	e.stage(e.automationLane, automationEnvelope{Kind: envelopeKindAction, Action: event}, sideeffects.Candidate{
		EventType: string(event.EventType),
		Key:       stableSCMEventKey(event),
	})
}

// StageNotificationEvent stages an SLA warning or breach for the notification
// lane inside the caller's transaction.
func (e *transactionEffects) StageNotificationEvent(event *NotificationEvent) {
	if event == nil || e.notificationLane == nil {
		return
	}
	e.stage(e.notificationLane, notificationEnvelope{Kind: envelopeKindEvent, Event: event}, sideeffects.Candidate{
		EventType: event.EventType,
	})
}

func (e *transactionEffects) stage(lane *sideeffects.Lane, envelope any, candidate sideeffects.Candidate) {
	payload, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	e.staging.Stage(lane, sideeffects.Candidate{
		EventType:  candidate.EventType,
		Key:        candidate.Key,
		Payload:    payload,
		OccurredAt: candidate.OccurredAt,
	})
}

// handleAutomationCandidate runs one admitted automation effect. Automatic
// dispatch is attempted at most once per admitted event; a failed match does
// not replay earlier successes or stop later matches within the same event.
func (d *SideEffectDispatcher) handleAutomationCandidate(ctx context.Context, candidate sideeffects.Candidate) error {
	var envelope automationEnvelope
	if err := json.Unmarshal(candidate.Payload, &envelope); err != nil {
		return fmt.Errorf("decode automation candidate: %w", err)
	}
	switch envelope.Kind {
	case envelopeKindFact:
		if envelope.Fact == nil {
			return errors.New("automation candidate missing fact")
		}
		return d.runAutomationFact(ctx, *envelope.Fact)
	case envelopeKindAction:
		if envelope.Action == nil {
			return errors.New("automation candidate missing action event")
		}
		return d.runWorkspaceActions(ctx, candidate.Key, envelope.Action)
	case envelopeKindAssetAction:
		if envelope.AssetAction == nil {
			return errors.New("automation candidate missing asset action event")
		}
		return d.runAssetActions(ctx, candidate.Key, envelope.AssetAction)
	default:
		return fmt.Errorf("unknown automation candidate kind %q", envelope.Kind)
	}
}

func (d *SideEffectDispatcher) runAutomationFact(ctx context.Context, event events.Event) error {
	if itemevents.IsImportOrigin(event.ActorKind, event.SourceKind) {
		return nil
	}
	if itemevents.SuppressesSideEffects(event.Type, event.Payload) {
		return nil
	}
	if isAssetFact(event.Type) {
		actionEvent, _, err := assetActionEventFromDomainEvent(event)
		if err != nil {
			return err
		}
		return d.runAssetActions(ctx, event.Key, actionEvent)
	}
	actionEvent, _, err := actionEventFromDomainEvent(event)
	if err != nil {
		return err
	}
	return d.runWorkspaceActions(ctx, event.Key, actionEvent)
}

// runWorkspaceActions matches cached actions once and executes each unfinished
// match directly. It never materializes action_event_batches/targets.
func (d *SideEffectDispatcher) runWorkspaceActions(ctx context.Context, eventKey string, actionEvent *models.ActionEvent) error {
	d.actions.cacheMu.RLock()
	cached := append([]*models.Action(nil), d.actions.actionCache[actionEvent.WorkspaceID]...)
	d.actions.cacheMu.RUnlock()

	var chain *ExecutionChain
	if actionEvent.ExecutionChainID != "" {
		chain = d.actions.chainStore.GetChain(actionEvent.ExecutionChainID)
	}
	matching := selectDurableActionTargets(cached, actionEvent.CascadeDepth, chain,
		func(action *models.Action) int { return action.ID },
		func(action *models.Action) string { return fmt.Sprintf("workspace:%d", action.ID) },
		func(action *models.Action) bool { return d.actions.matchesTrigger(action, actionEvent) },
	)

	var failures []error
	for _, actionID := range matching {
		if err := ctx.Err(); err != nil {
			return err
		}
		if done, err := d.workspaceActionCompleted(eventKey, actionID); err != nil {
			failures = append(failures, err)
			continue
		} else if done {
			continue
		}
		action, err := d.actions.repo.GetByID(actionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			failures = append(failures, fmt.Errorf("load action %d: %w", actionID, err))
			continue
		}
		if err := d.actions.executeActionForEvent(ctx, action, actionEvent, nil, eventKey); err != nil {
			failures = append(failures, fmt.Errorf("execute action %d: %w", actionID, err))
			continue
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	return nil
}

func (d *SideEffectDispatcher) workspaceActionCompleted(eventKey string, actionID int) (bool, error) {
	existing, err := d.actions.repo.GetExecutionLogByDurableTarget(eventKey, actionID)
	if err == nil {
		return existing.Status == models.ActionStatusCompleted || existing.Status == models.ActionStatusReconciliationRequired, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("load action execution for %s: %w", eventKey, err)
}

// runAssetActions mirrors runWorkspaceActions for the asset domain. Both share
// one lane and one execution chain store.
func (d *SideEffectDispatcher) runAssetActions(ctx context.Context, eventKey string, actionEvent *models.AssetActionEvent) error {
	d.assetActions.cacheMu.RLock()
	cached := append([]*models.AssetAction(nil), d.assetActions.actionCache[actionEvent.SetID]...)
	d.assetActions.cacheMu.RUnlock()

	var chain *ExecutionChain
	if actionEvent.ExecutionChainID != "" {
		chain = d.assetActions.chainStore.GetChain(actionEvent.ExecutionChainID)
	}
	matching := selectDurableActionTargets(cached, actionEvent.CascadeDepth, chain,
		func(action *models.AssetAction) int { return action.ID },
		func(action *models.AssetAction) string { return fmt.Sprintf("asset:%d", action.ID) },
		func(action *models.AssetAction) bool { return d.assetActions.matchesTrigger(action, actionEvent) },
	)

	var failures []error
	for _, actionID := range matching {
		if err := ctx.Err(); err != nil {
			return err
		}
		if done, err := d.assetActionCompleted(eventKey, actionID); err != nil {
			failures = append(failures, err)
			continue
		} else if done {
			continue
		}
		action, err := d.assetActions.repo.GetByID(actionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			failures = append(failures, fmt.Errorf("load asset action %d: %w", actionID, err))
			continue
		}
		result, err := d.assetActions.executeActionWithResultForEvent(ctx, action, actionEvent, nil, eventKey)
		if err == nil && result != nil && result.Status == models.ActionStatusFailed {
			err = fmt.Errorf("asset action %d completed with failed steps: %s", action.ID, result.ErrorMessage)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("execute asset action %d: %w", actionID, err))
			continue
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	return nil
}

func (d *SideEffectDispatcher) assetActionCompleted(eventKey string, actionID int) (bool, error) {
	existing, err := d.assetActions.repo.GetExecutionLogByDurableTarget(eventKey, actionID)
	if err == nil {
		return existing.Status == models.ActionStatusCompleted || existing.Status == models.ActionStatusSkipped, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("load asset action execution for %s: %w", eventKey, err)
}

// isNotificationFact reports whether a canonical fact feeds rule-based
// notifications. Status self-transitions and asset facts do not.
func isNotificationFact(eventType string) bool {
	switch eventType {
	case itemevents.Created, itemevents.Updated, itemevents.StatusChanged, itemevents.Deleted,
		itemevents.CommentCreated, itemevents.Linked, itemevents.Unlinked:
		return true
	default:
		return false
	}
}

// isAutomationFact reports whether a canonical fact feeds automatic triggers.
func isAutomationFact(eventType string) bool {
	switch eventType {
	case itemevents.Created, itemevents.Updated, itemevents.StatusChanged, itemevents.StatusLooped, itemevents.Linked:
		return true
	case assetevents.Created, assetevents.Updated, assetevents.StatusChanged, assetevents.Deleted:
		return true
	default:
		return false
	}
}

func isAssetFact(eventType string) bool {
	return strings.HasPrefix(eventType, "asset.")
}
