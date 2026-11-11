package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"windshift/internal/actionevents"
	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/models"
)

// SLASideEffectEmitter turns a breach or warning into the existing
// notification and automation compatibility events. It appends inside the
// caller's transaction and only when a subscription or rule references the
// SLA trigger, so the durable engine gains no new consumer key.
type SLASideEffectEmitter struct {
	store *events.Store
}

// NewSLASideEffectEmitter constructs the emitter.
func NewSLASideEffectEmitter(db database.Database) *SLASideEffectEmitter {
	return &SLASideEffectEmitter{store: events.NewStore(db)}
}

// EmitBreach emits the notification and automation events for a breach. The
// event occurrence time is the promised deadline, not the firing time.
func (e *SLASideEffectEmitter) EmitBreach(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle) error {
	occurredAt := time.Now().UTC()
	if cycle.BreachedAt != nil {
		occurredAt = *cycle.BreachedAt
	}
	return e.emit(ctx, tx, cycle, "", models.EventSLABreached, string(models.ActionTriggerSLABreached), occurredAt)
}

// EmitWarning emits the notification and automation events for a warning
// threshold.
func (e *SLASideEffectEmitter) EmitWarning(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle, thresholdKey string) error {
	return e.emit(ctx, tx, cycle, thresholdKey, models.EventSLAWarning, string(models.ActionTriggerSLAWarning), time.Now().UTC())
}

func (e *SLASideEffectEmitter) emit(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle, thresholdKey, notificationEvent, actionTrigger string, occurredAt time.Time) error {
	if cycle == nil || cycle.ItemID == 0 {
		return nil
	}
	var workspaceID, itemNumber int
	var title, workspaceKey, metricName string
	err := tx.QueryRowContext(ctx, `SELECT i.workspace_id, i.workspace_item_number, i.title, w.key, m.name
		FROM items i
		JOIN workspaces w ON w.id = i.workspace_id
		JOIN sla_metrics m ON m.id = ?
		WHERE i.id = ?`, cycle.MetricID, cycle.ItemID).
		Scan(&workspaceID, &itemNumber, &title, &workspaceKey, &metricName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load SLA side-effect target: %w", err)
	}

	notificationEnabled, err := e.notificationConfigured(ctx, tx, notificationEvent)
	if err != nil {
		return err
	}
	if notificationEnabled {
		if err := e.appendNotification(ctx, tx, cycle, workspaceID, itemNumber, workspaceKey, title, metricName, thresholdKey, notificationEvent, occurredAt); err != nil {
			return err
		}
	}

	actionEnabled, err := e.actionConfigured(ctx, tx, workspaceID, actionTrigger)
	if err != nil {
		return err
	}
	if actionEnabled {
		if err := e.appendAction(ctx, tx, cycle, workspaceID, itemNumber, workspaceKey, title, metricName, thresholdKey, actionTrigger, occurredAt); err != nil {
			return err
		}
	}
	return nil
}

func (e *SLASideEffectEmitter) notificationConfigured(ctx context.Context, tx database.Tx, eventType string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM notification_event_rules r
		JOIN notification_settings s ON s.id = r.notification_setting_id
		WHERE r.event_type = ? AND r.is_enabled = true AND s.is_active = true)`, eventType).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check SLA notification configuration: %w", err)
	}
	return exists, nil
}

func (e *SLASideEffectEmitter) actionConfigured(ctx context.Context, tx database.Tx, workspaceID int, trigger string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM actions WHERE workspace_id = ? AND trigger_type = ? AND is_enabled = true)`, workspaceID, trigger).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check SLA action configuration: %w", err)
	}
	return exists, nil
}

func (e *SLASideEffectEmitter) appendNotification(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle, workspaceID, itemNumber int, workspaceKey, title, metricName, thresholdKey, eventType string, occurredAt time.Time) error {
	itemKey := fmt.Sprintf("%s-%d", workspaceKey, itemNumber)
	notificationEvent := &NotificationEvent{
		EventType:   eventType,
		WorkspaceID: workspaceID,
		ItemID:      cycle.ItemID,
		Title:       fmt.Sprintf("SLA %s: %s", slaOutcomeLabel(eventType), metricName),
		TemplateData: map[string]any{
			"item.id":      cycle.ItemID,
			"item.key":     itemKey,
			"item.title":   title,
			"sla.metric":   metricName,
			"sla.elapsed":  time.Duration(cycle.ElapsedMs) * time.Millisecond,
			"sla.goal":     time.Duration(cycle.GoalDurationMs) * time.Millisecond,
			"sla.breached": cycle.BreachedAt,
		},
	}
	if thresholdKey != "" {
		notificationEvent.TemplateData["sla.threshold"] = thresholdKey
	}
	input, err := actionevents.NewSideEffectEvent(actionevents.SideEffectInput{
		Payload: notificationEvent, WorkspaceID: &workspaceID,
		AggregateType: "notification_item", AggregateID: strconv.Itoa(cycle.ItemID),
		EventType: DurableNotificationRequestedEvent,
		ActorKind: slaCycleActorKind(cycle), SourceKind: slaCycleSourceKind(cycle),
	})
	if err != nil {
		return fmt.Errorf("encode SLA notification event: %w", err)
	}
	input.OccurredAt = occurredAt
	if _, err := e.store.Append(ctx, tx, input); err != nil {
		return fmt.Errorf("append SLA notification event: %w", err)
	}
	return nil
}

func (e *SLASideEffectEmitter) appendAction(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle, workspaceID, itemNumber int, workspaceKey, title, metricName, thresholdKey, trigger string, occurredAt time.Time) error {
	newValues := map[string]any{
		"metric_id":        cycle.MetricID,
		"cycle_id":         cycle.ID,
		"sla.breached":     cycle.BreachedAt != nil,
		"elapsed_ms":       cycle.ElapsedMs,
		"goal_duration_ms": cycle.GoalDurationMs,
		"item_key":         fmt.Sprintf("%s-%d", workspaceKey, itemNumber),
		"item_title":       title,
		"metric_name":      metricName,
	}
	if thresholdKey != "" {
		newValues["threshold_key"] = thresholdKey
	}
	actionEvent := &models.ActionEvent{
		EventType:   models.ActionTriggerType(trigger),
		WorkspaceID: workspaceID,
		ItemID:      cycle.ItemID,
		NewValues:   newValues,
	}
	input, err := actionevents.NewSideEffectEvent(actionevents.SideEffectInput{
		Payload: actionEvent, WorkspaceID: &workspaceID,
		AggregateType: "action_item", AggregateID: strconv.Itoa(cycle.ItemID),
		EventType: DurableActionRequestedEvent,
		ActorKind: slaCycleActorKind(cycle), SourceKind: slaCycleSourceKind(cycle),
	})
	if err != nil {
		return fmt.Errorf("encode SLA action event: %w", err)
	}
	input.OccurredAt = occurredAt
	if _, err := e.store.Append(ctx, tx, input); err != nil {
		return fmt.Errorf("append SLA action event: %w", err)
	}
	return nil
}

func slaOutcomeLabel(eventType string) string {
	if eventType == models.EventSLAWarning {
		return "warning"
	}
	return "breached"
}

// slaCycleActorKind and slaCycleSourceKind preserve the cycle's import origin
// on the side-effect event so an imported SLA can never drive live output.
func slaCycleActorKind(cycle *models.ItemSLACycle) string {
	if cycle.Origin == models.SLAOriginImport {
		return "import"
	}
	return "system"
}

func slaCycleSourceKind(cycle *models.ItemSLACycle) string {
	if cycle.Origin == models.SLAOriginImport {
		return "import"
	}
	return "sla"
}
