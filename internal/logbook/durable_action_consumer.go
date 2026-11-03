package logbook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"

	"windshift/internal/actionevents"
	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/itemevents"
	"windshift/internal/logbookevents"
	"windshift/internal/models"
	"windshift/internal/repository"
)

const (
	DurableLogbookActionConsumerKey = "actions.logbook.v1"

	durableLogbookActionCutoverKey = "actions.logbook.canonical.v1"
)

// DocumentEventRecorder records a classified document inside the transaction
// that publishes its ready state and chunks.
type DocumentEventRecorder interface {
	RecordClassified(context.Context, database.Tx, logbookevents.ClassifiedInput) error
}

// DurableDocumentEventRecorder records a classified document's canonical fact.
type DurableDocumentEventRecorder struct {
	canonical *logbookevents.Recorder
}

func NewDurableDocumentEventRecorder(db database.Database) *DurableDocumentEventRecorder {
	return &DurableDocumentEventRecorder{canonical: logbookevents.NewRecorder(db)}
}

func (r *DurableDocumentEventRecorder) RecordClassified(ctx context.Context, tx database.Tx, input logbookevents.ClassifiedInput) error {
	_, err := r.canonical.RecordClassified(ctx, tx, input)
	return err
}

func ConfigureDurableLogbookActionConsumers(ctx context.Context, store *events.Store, cutover *actionevents.Cutover) error {
	return actionevents.ConfigureCutoverConsumers(ctx, store, cutover, events.Consumer{
		Key: DurableLogbookActionConsumerKey, HandlerVersion: 1,
		EventTypes: []string{logbookevents.Classified},
	})
}

// PrepareDurableLogbookActionEngine installs the canonical logbook document
// consumer on the shared event engine implementation. The canonical consumer is
// always active; the cutover record only anchors its start event id.
func PrepareDurableLogbookActionEngine(ctx context.Context, engine *events.Engine, actions *LogbookActionService) error {
	if engine == nil || actions == nil {
		return errors.New("domain event engine and logbook action service are required")
	}
	cutover, err := actionevents.CurrentCutover(ctx, actions.db, durableLogbookActionCutoverKey)
	if err != nil {
		return err
	}
	if cutover == nil {
		cutover, err = actionevents.ActivateCutover(ctx, actions.db, durableLogbookActionCutoverKey, "logbook action")
		if err != nil {
			return err
		}
	}
	if err := ConfigureDurableLogbookActionConsumers(ctx, engine.Store(), cutover); err != nil {
		return err
	}
	handler := NewDurableLogbookActionConsumer(actions.db, actions)
	if err := engine.RegisterHandler(DurableLogbookActionConsumerKey, handler); err != nil {
		return err
	}
	return nil
}

// DurableLogbookActionConsumer adapts document facts to the existing action
// matcher and executor while using shared frozen-target state.
type DurableLogbookActionConsumer struct {
	actions *LogbookActionService
	targets *actionevents.TargetStore
}

func NewDurableLogbookActionConsumer(db database.Database, actions *LogbookActionService) *DurableLogbookActionConsumer {
	return &DurableLogbookActionConsumer{actions: actions, targets: actionevents.NewTargetStore(db)}
}

func (c *DurableLogbookActionConsumer) Handle(ctx context.Context, event events.Event) error {
	// Imported facts are historical: never let them trigger automation.
	if itemevents.IsImportOrigin(event.ActorKind, event.SourceKind) {
		return nil
	}
	actionEvent, consumerKey, err := logbookActionEventFromDomainEvent(event)
	if err != nil {
		return events.Permanent(err)
	}
	if err := c.materializeTargets(ctx, event, consumerKey, actionEvent); err != nil {
		return err
	}
	executed, err := actionevents.RunTargets(ctx, c.targets, event.Key, actionevents.Callbacks{
		Completed: func(actionID int) (bool, error) {
			existing, err := c.actions.repo.GetExecutionLogByDurableTarget(event.Key, actionID)
			return err == nil && (existing.Status == models.ActionStatusCompleted || existing.Status == models.ActionStatusSkipped), err
		},
		Execute: func(actionID int) (bool, error) {
			action, err := c.actions.repo.GetByID(actionID)
			if err != nil {
				return errors.Is(err, repository.ErrNotFound), fmt.Errorf("load frozen logbook action %d: %w", actionID, err)
			}
			executionEvent := *actionEvent
			executionEvent.EventType = action.TriggerType
			result, err := c.actions.executeActionForEvent(action, &executionEvent, event.Key)
			if err == nil && result.Status == models.ActionStatusFailed {
				err = fmt.Errorf("logbook action %d completed with failed steps: %s", action.ID, result.ErrorMessage)
			}
			if err != nil {
				return false, fmt.Errorf("execute frozen logbook action %d: %w", actionID, err)
			}
			return false, nil
		},
	})
	atomic.AddInt64(&c.actions.actionsExecuted, executed)
	if err != nil {
		atomic.AddInt64(&c.actions.errors, 1)
		return err
	}
	atomic.AddInt64(&c.actions.eventsProcessed, 1)
	return nil
}

func (c *DurableLogbookActionConsumer) materializeTargets(ctx context.Context, event events.Event, consumerKey string, actionEvent *models.LogbookActionEvent) error {
	c.actions.cacheMu.RLock()
	actions := append([]*models.LogbookAction(nil), c.actions.actionCache[actionEvent.BucketID]...)
	c.actions.cacheMu.RUnlock()
	matching := make([]int, 0, len(actions))
	if actionEvent.CascadeDepth < maxCascadeDepth {
		for _, action := range actions {
			if action.TriggerType == models.LogbookTriggerManual {
				continue
			}
			candidate := *actionEvent
			candidate.EventType = action.TriggerType
			if c.actions.matchesTrigger(action, &candidate) {
				matching = append(matching, action.ID)
			}
		}
	}
	return c.targets.Materialize(ctx, event, consumerKey, event.Type, matching)
}

func logbookActionEventFromDomainEvent(event events.Event) (*models.LogbookActionEvent, string, error) {
	if event.PayloadVersion != logbookevents.PayloadVersion {
		return nil, "", fmt.Errorf("unsupported logbook action event %s payload version %d", event.Type, event.PayloadVersion)
	}
	if event.Type != logbookevents.Classified {
		return nil, "", fmt.Errorf("unsupported durable logbook action event type %q", event.Type)
	}
	var payload logbookevents.ClassifiedV1
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, "", fmt.Errorf("decode logbook document classified payload: %w", err)
	}
	actionEvent := &models.LogbookActionEvent{
		EventType: models.LogbookTriggerDocumentClassified,
		BucketID:  payload.Document.BucketID, DocumentID: payload.Document.ID,
		ActorUserID: eventActorUserID(event), ContentType: payload.Document.ContentType,
		MimeType: payload.Document.MimeType, Title: payload.Document.Title,
		SourceType: payload.Document.SourceType, Author: payload.Document.Author,
		RawContent: payload.Document.RawContent, CausationEventKey: event.Key,
	}
	if payload.Automation != nil {
		actionEvent.TriggeredByAction = payload.Automation.TriggeredByAction
		actionEvent.ExecutionChainID = payload.Automation.ExecutionChainID
		actionEvent.CascadeDepth = payload.Automation.CascadeDepth
		actionEvent.SourceApplication = payload.Automation.SourceApplication
	}
	return actionEvent, DurableLogbookActionConsumerKey, nil
}

func eventActorUserID(event events.Event) int {
	if event.ActorKind != "user" {
		return 0
	}
	userID, _ := strconv.Atoi(event.ActorRef)
	return userID
}
