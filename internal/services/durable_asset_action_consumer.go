package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"windshift/internal/actionevents"
	"windshift/internal/assetevents"
	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/models"
	"windshift/internal/repository"
)

const (
	DurableAssetActionCompatibilityEvent = "asset.action_triggered.v1"

	DurableAssetCompatibilityActionConsumerKey = "actions.assets.compatibility.v1"
	DurableAssetActionConsumerKey              = "actions.assets.v1"

	durableAssetActionCutoverKey = "actions.assets.canonical.v1"
)

// DurableAssetActionIngress persists compatibility events until the recorded
// canonical asset cutover.
type DurableAssetActionIngress struct {
	db    database.Database
	store *events.Store
}

func NewDurableAssetActionIngress(db database.Database) *DurableAssetActionIngress {
	return &DurableAssetActionIngress{db: db, store: events.NewStore(db)}
}

func (i *DurableAssetActionIngress) Emit(ctx context.Context, actionEvent *models.AssetActionEvent) error {
	if actionEvent == nil {
		return errors.New("asset action event is required")
	}
	cutover, err := i.CurrentCanonicalAssetsCutover(ctx)
	if err != nil {
		return err
	}
	if cutover != nil {
		return nil
	}
	input, err := durableAssetActionEventInput(actionEvent)
	if err != nil {
		return err
	}
	return appendStandaloneDurableActionEvent(ctx, i.db, i.store, input, "asset action")
}

func (i *DurableAssetActionIngress) EmitInTx(ctx context.Context, tx database.Tx, actionEvent *models.AssetActionEvent) error {
	if actionEvent == nil {
		return errors.New("asset action event is required")
	}
	cutover, err := currentActionCutover(ctx, tx, durableAssetActionCutoverKey)
	if err != nil {
		return err
	}
	if cutover != nil {
		return nil
	}
	input, err := durableAssetActionEventInput(actionEvent)
	if err != nil {
		return err
	}
	if _, err := i.store.Append(ctx, tx, input); err != nil {
		return fmt.Errorf("append transactional asset action event: %w", err)
	}
	return nil
}

func durableAssetActionEventInput(actionEvent *models.AssetActionEvent) (events.NewEvent, error) {
	input, err := actionevents.NewCompatibilityEvent(actionevents.CompatibilityInput{
		Payload: actionEvent, AggregateType: "asset", AggregateID: strconv.Itoa(actionEvent.AssetID),
		EventType: DurableAssetActionCompatibilityEvent, ActorUserID: actionEvent.ActorUserID,
		CorrelationID: actionEvent.ExecutionChainID, CausationEventKey: actionEvent.CausationEventKey,
	})
	if err != nil {
		return events.NewEvent{}, fmt.Errorf("encode durable asset action event: %w", err)
	}
	return input, nil
}

func (i *DurableAssetActionIngress) ActivateCanonicalAssets(ctx context.Context) (*ActionCutover, error) {
	return activateActionCutover(ctx, i.db, durableAssetActionCutoverKey, "asset action")
}

func (i *DurableAssetActionIngress) CurrentCanonicalAssetsCutover(ctx context.Context) (*ActionCutover, error) {
	return currentActionCutover(ctx, i.db, durableAssetActionCutoverKey)
}

func ConfigureDurableAssetActionConsumers(ctx context.Context, store *events.Store, cutover *ActionCutover) error {
	return actionevents.ConfigureCutoverConsumers(ctx, store, cutover, events.Consumer{
		Key: DurableAssetActionConsumerKey, HandlerVersion: 1,
		EventTypes: []string{assetevents.Created, assetevents.Updated, assetevents.StatusChanged, assetevents.Deleted},
	},
		events.Consumer{Key: DurableAssetCompatibilityActionConsumerKey, HandlerVersion: 1, Active: true, StartEventID: 1, EventTypes: []string{DurableAssetActionCompatibilityEvent}},
	)
}

// PrepareDurableAssetActionEngine installs asset subscriptions and handlers on
// the same engine used by item and SCM actions.
func PrepareDurableAssetActionEngine(ctx context.Context, engine *events.Engine, actions *AssetActionService, activate bool) error {
	if engine == nil || actions == nil {
		return errors.New("domain event engine and asset action service are required")
	}
	cutover, err := actions.durableIngress.CurrentCanonicalAssetsCutover(ctx)
	if err != nil {
		return err
	}
	if activate && cutover == nil {
		cutover, err = actions.durableIngress.ActivateCanonicalAssets(ctx)
		if err != nil {
			return err
		}
	}
	if err := ConfigureDurableAssetActionConsumers(ctx, engine.Store(), cutover); err != nil {
		return err
	}
	handler := NewDurableAssetActionConsumer(actions.db, actions)
	for _, key := range []string{DurableAssetCompatibilityActionConsumerKey, DurableAssetActionConsumerKey} {
		if err := engine.RegisterHandler(key, handler); err != nil {
			return err
		}
	}
	return nil
}

// DurableAssetActionConsumer adapts canonical asset facts to asset automation
// semantics while sharing the durable frozen-target state machine.
type DurableAssetActionConsumer struct {
	actions        *AssetActionService
	targets        *actionevents.TargetStore
	cutoverStartID int64
}

func NewDurableAssetActionConsumer(db database.Database, actions *AssetActionService) *DurableAssetActionConsumer {
	consumer := &DurableAssetActionConsumer{actions: actions, targets: actionevents.NewTargetStore(db)}
	if cutover, err := actions.durableIngress.CurrentCanonicalAssetsCutover(context.Background()); err == nil && cutover != nil {
		consumer.cutoverStartID = cutover.StartEventID
	}
	return consumer
}

func (c *DurableAssetActionConsumer) Handle(ctx context.Context, event events.Event) error {
	if event.Type == DurableAssetActionCompatibilityEvent && c.cutoverStartID > 0 && event.ID >= c.cutoverStartID {
		return nil
	}
	return handleDurableActionEvent(ctx, event, c.targets,
		assetActionEventFromDomainEvent,
		c.materializeTargets,
		func(actionEvent *models.AssetActionEvent) actionevents.Callbacks {
			return actionevents.Callbacks{
				Completed: func(actionID int) (bool, error) {
					existing, err := c.actions.repo.GetExecutionLogByDurableTarget(event.Key, actionID)
					return err == nil && (existing.Status == models.ActionStatusCompleted || existing.Status == models.ActionStatusSkipped), err
				},
				Execute: func(actionID int) (bool, error) {
					action, err := c.actions.repo.GetByID(actionID)
					if err != nil {
						return errors.Is(err, repository.ErrNotFound), fmt.Errorf("load frozen asset action %d: %w", actionID, err)
					}
					result, err := c.actions.executeActionWithResultForEvent(ctx, action, actionEvent, nil, event.Key)
					if err == nil && result.Status == models.ActionStatusFailed {
						err = fmt.Errorf("asset action %d completed with failed steps: %s", action.ID, result.ErrorMessage)
					}
					if err != nil {
						return false, fmt.Errorf("execute frozen asset action %d: %w", actionID, err)
					}
					return false, nil
				},
			}
		},
		&c.actions.actionsExecuted,
		&c.actions.eventsProcessed,
	)
}

func (c *DurableAssetActionConsumer) materializeTargets(ctx context.Context, event events.Event, consumerKey string, actionEvent *models.AssetActionEvent) error {
	c.actions.cacheMu.RLock()
	actions := append([]*models.AssetAction(nil), c.actions.actionCache[actionEvent.SetID]...)
	c.actions.cacheMu.RUnlock()
	var chain *ExecutionChain
	if actionEvent.ExecutionChainID != "" {
		chain = c.actions.chainStore.GetChain(actionEvent.ExecutionChainID)
	}
	matching := selectDurableActionTargets(actions, actionEvent.CascadeDepth, chain,
		func(action *models.AssetAction) int { return action.ID },
		func(action *models.AssetAction) string { return fmt.Sprintf("asset:%d", action.ID) },
		func(action *models.AssetAction) bool { return c.actions.matchesTrigger(action, actionEvent) },
	)
	return c.targets.Materialize(ctx, event, consumerKey, string(actionEvent.EventType), matching)
}

func assetActionEventFromDomainEvent(event events.Event) (*models.AssetActionEvent, string, error) {
	if event.PayloadVersion != assetevents.PayloadVersion {
		return nil, "", fmt.Errorf("unsupported asset action event %s payload version %d", event.Type, event.PayloadVersion)
	}
	if event.Type == DurableAssetActionCompatibilityEvent {
		var actionEvent models.AssetActionEvent
		if err := json.Unmarshal(event.Payload, &actionEvent); err != nil {
			return nil, "", fmt.Errorf("decode %s payload: %w", event.Type, err)
		}
		return &actionEvent, DurableAssetCompatibilityActionConsumerKey, nil
	}
	actionEvent := &models.AssetActionEvent{ActorUserID: eventActorUserID(event), CausationEventKey: event.Key}
	switch event.Type {
	case assetevents.Created:
		var payload assetevents.CreatedV1
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, "", fmt.Errorf("decode asset.created: %w", err)
		}
		applyAssetSnapshot(actionEvent, payload.Asset)
		actionEvent.EventType = models.AssetTriggerAssetCreated
		actionEvent.NewValues = payload.NewValues
		applyAssetEventAutomation(actionEvent, payload.Automation)
	case assetevents.Updated:
		var payload assetevents.UpdatedV1
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, "", fmt.Errorf("decode asset.updated: %w", err)
		}
		applyAssetSnapshot(actionEvent, payload.Asset)
		actionEvent.EventType = models.AssetTriggerAssetUpdated
		actionEvent.OldValues, actionEvent.NewValues = payload.OldValues, payload.NewValues
		applyAssetEventAutomation(actionEvent, payload.Automation)
	case assetevents.StatusChanged:
		var payload assetevents.StatusChangedV1
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, "", fmt.Errorf("decode asset.status_changed: %w", err)
		}
		applyAssetSnapshot(actionEvent, payload.Asset)
		actionEvent.EventType = models.AssetTriggerAssetStatusChanged
		actionEvent.OldValues, actionEvent.NewValues = payload.OldValues, payload.NewValues
		applyAssetEventAutomation(actionEvent, payload.Automation)
	case assetevents.Deleted:
		var payload assetevents.DeletedV1
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, "", fmt.Errorf("decode asset.deleted: %w", err)
		}
		applyAssetSnapshot(actionEvent, payload.Asset)
		actionEvent.EventType = models.AssetTriggerAssetDeleted
		actionEvent.OldValues = payload.OldValues
		applyAssetEventAutomation(actionEvent, payload.Automation)
	default:
		return nil, "", fmt.Errorf("unsupported durable asset action event type %q", event.Type)
	}
	return actionEvent, DurableAssetActionConsumerKey, nil
}

func applyAssetSnapshot(event *models.AssetActionEvent, asset assetevents.AssetSnapshot) {
	event.SetID = asset.SetID
	event.AssetID = asset.ID
}

func applyAssetEventAutomation(event *models.AssetActionEvent, automation *assetevents.AutomationContext) {
	if automation == nil {
		return
	}
	event.TriggeredByAction = automation.TriggeredByAction
	event.ExecutionChainID = automation.ExecutionChainID
	event.CascadeDepth = automation.CascadeDepth
	event.SourceApplication = automation.SourceApplication
}
