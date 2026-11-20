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

// SLASideEffectEmitter stages the minimal committed description of an SLA
// warning or breach on the caller's transaction. It performs no database reads,
// so an enrichment or configuration failure can never roll back the lifecycle
// update or prevent the job from being retired. The lane worker enriches the
// occurrence and matches rules and actions.
type SLASideEffectEmitter struct{}

// NewSLASideEffectEmitter constructs the emitter.
func NewSLASideEffectEmitter() *SLASideEffectEmitter {
	return &SLASideEffectEmitter{}
}

// EmitBreach stages the breach occurrence.
func (e *SLASideEffectEmitter) EmitBreach(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle) error {
	return e.stage(tx, cycle, "", models.EventSLABreached, string(models.ActionTriggerSLABreached))
}

// EmitWarning stages the warning occurrence.
func (e *SLASideEffectEmitter) EmitWarning(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle, thresholdKey string) error {
	return e.stage(tx, cycle, thresholdKey, models.EventSLAWarning, string(models.ActionTriggerSLAWarning))
}

func (e *SLASideEffectEmitter) stage(tx database.Tx, cycle *models.ItemSLACycle, thresholdKey, eventType, trigger string) error {
	if cycle == nil || cycle.ItemID == 0 {
		return nil
	}
	// Imported cycles are historical; they must never drive live output.
	if cycle.Origin == models.SLAOriginImport {
		return nil
	}
	stager, ok := database.TxEffectsOf(tx).(slaOccurrenceStager)
	if !ok {
		return nil
	}
	stager.StageSLAOccurrence(SLAOccurrence{
		EventType:      eventType,
		Trigger:        trigger,
		CycleID:        cycle.ID,
		ItemID:         cycle.ItemID,
		MetricID:       cycle.MetricID,
		ThresholdKey:   thresholdKey,
		BreachedAt:     cycle.BreachedAt,
		ElapsedMs:      cycle.ElapsedMs,
		GoalDurationMs: cycle.GoalDurationMs,
	})
	return nil
}

// slaOccurrenceTarget is the enriched item/metric/workspace data a lane worker
// needs to build the notification or action event.
type slaOccurrenceTarget struct {
	workspaceID  int
	itemNumber   int
	workspaceKey string
	title        string
	metricName   string
}

func loadSLAOccurrenceTarget(ctx context.Context, db database.Database, occurrence SLAOccurrence) (*slaOccurrenceTarget, error) {
	var target slaOccurrenceTarget
	err := db.QueryRowContext(ctx, `SELECT i.workspace_id, i.workspace_item_number, i.title, w.key, m.name
		FROM items i
		JOIN workspaces w ON w.id = i.workspace_id
		JOIN sla_metrics m ON m.id = ?
		WHERE i.id = ?`, occurrence.MetricID, occurrence.ItemID).
		Scan(&target.workspaceID, &target.itemNumber, &target.title, &target.workspaceKey, &target.metricName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load SLA side-effect target: %w", err)
	}
	return &target, nil
}

// buildSLANotificationEvent enriches an SLA occurrence into a rule-based
// notification event. It returns nil when the item or metric no longer exists.
func buildSLANotificationEvent(ctx context.Context, db database.Database, occurrence SLAOccurrence) (*NotificationEvent, error) {
	target, err := loadSLAOccurrenceTarget(ctx, db, occurrence)
	if err != nil || target == nil {
		return nil, err
	}
	itemKey := fmt.Sprintf("%s-%d", target.workspaceKey, target.itemNumber)
	event := &NotificationEvent{
		EventType:   occurrence.EventType,
		WorkspaceID: target.workspaceID,
		ItemID:      occurrence.ItemID,
		Title:       fmt.Sprintf("SLA %s: %s", slaOutcomeLabel(occurrence.EventType), target.metricName),
		TemplateData: map[string]any{
			"item.id":      occurrence.ItemID,
			"item.key":     itemKey,
			"item.title":   target.title,
			"sla.metric":   target.metricName,
			"sla.elapsed":  time.Duration(occurrence.ElapsedMs) * time.Millisecond,
			"sla.goal":     time.Duration(occurrence.GoalDurationMs) * time.Millisecond,
			"sla.breached": occurrence.BreachedAt,
		},
	}
	if occurrence.ThresholdKey != "" {
		event.TemplateData["sla.threshold"] = occurrence.ThresholdKey
	}
	return event, nil
}

// buildSLAActionEvent enriches an SLA occurrence into an automatic trigger.
func buildSLAActionEvent(ctx context.Context, db database.Database, occurrence SLAOccurrence) (*models.ActionEvent, error) {
	target, err := loadSLAOccurrenceTarget(ctx, db, occurrence)
	if err != nil || target == nil {
		return nil, err
	}
	newValues := map[string]any{
		"metric_id":        occurrence.MetricID,
		"cycle_id":         occurrence.CycleID,
		"sla.breached":     occurrence.BreachedAt != nil,
		"elapsed_ms":       occurrence.ElapsedMs,
		"goal_duration_ms": occurrence.GoalDurationMs,
		"item_key":         fmt.Sprintf("%s-%d", target.workspaceKey, target.itemNumber),
		"item_title":       target.title,
		"metric_name":      target.metricName,
	}
	if occurrence.ThresholdKey != "" {
		newValues["threshold_key"] = occurrence.ThresholdKey
	}
	return &models.ActionEvent{
		EventType:   models.ActionTriggerType(occurrence.Trigger),
		WorkspaceID: target.workspaceID,
		ItemID:      occurrence.ItemID,
		NewValues:   newValues,
	}, nil
}

func slaOutcomeLabel(eventType string) string {
	if eventType == models.EventSLAWarning {
		return "warning"
	}
	return "breached"
}
