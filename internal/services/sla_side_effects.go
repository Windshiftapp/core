package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// SLASideEffectEmitter turns a breach or warning into notification and
// automation candidates staged on the caller's transaction. They publish only
// after the outer SLA job commits, so a rolled-back lifecycle update emits
// nothing. It stages only when a subscription or rule references the trigger.
type SLASideEffectEmitter struct{}

// NewSLASideEffectEmitter constructs the emitter.
func NewSLASideEffectEmitter() *SLASideEffectEmitter {
	return &SLASideEffectEmitter{}
}

// EmitBreach emits the notification and automation events for a breach.
func (e *SLASideEffectEmitter) EmitBreach(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle) error {
	return e.emit(ctx, tx, cycle, "", models.EventSLABreached, string(models.ActionTriggerSLABreached))
}

// EmitWarning emits the notification and automation events for a warning
// threshold.
func (e *SLASideEffectEmitter) EmitWarning(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle, thresholdKey string) error {
	return e.emit(ctx, tx, cycle, thresholdKey, models.EventSLAWarning, string(models.ActionTriggerSLAWarning))
}

func (e *SLASideEffectEmitter) emit(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle, thresholdKey, notificationEvent, actionTrigger string) error {
	if cycle == nil || cycle.ItemID == 0 {
		return nil
	}
	// Imported cycles are historical; they must never drive live output.
	if cycle.Origin == models.SLAOriginImport {
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
		if err := e.appendNotification(tx, cycle, workspaceID, itemNumber, workspaceKey, title, metricName, thresholdKey, notificationEvent); err != nil {
			return err
		}
	}

	actionEnabled, err := e.actionConfigured(ctx, tx, workspaceID, actionTrigger)
	if err != nil {
		return err
	}
	if actionEnabled {
		if err := e.appendAction(tx, cycle, workspaceID, itemNumber, workspaceKey, title, metricName, thresholdKey, actionTrigger); err != nil {
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

func (e *SLASideEffectEmitter) appendNotification(tx database.Tx, cycle *models.ItemSLACycle, workspaceID, itemNumber int, workspaceKey, title, metricName, thresholdKey, eventType string) error {
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
	stager, ok := database.TxEffectsOf(tx).(notificationEventStager)
	if !ok {
		return nil
	}
	stager.StageNotificationEvent(notificationEvent)
	return nil
}

func (e *SLASideEffectEmitter) appendAction(tx database.Tx, cycle *models.ItemSLACycle, workspaceID, itemNumber int, workspaceKey, title, metricName, thresholdKey, trigger string) error {
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
	stager, ok := database.TxEffectsOf(tx).(actionEventStager)
	if !ok {
		return nil
	}
	stager.StageActionEvent(actionEvent)
	return nil
}

func slaOutcomeLabel(eventType string) string {
	if eventType == models.EventSLAWarning {
		return "warning"
	}
	return "breached"
}
