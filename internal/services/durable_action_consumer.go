package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"windshift/internal/actionevents"
	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/itemevents"
	"windshift/internal/models"
)

const (
	DurableActionCompatibilityEvent = "action.triggered.v1"
	DurableSCMObservationEvent      = "scm.action_observed.v1"

	DurableCompatibilityActionConsumerKey = "actions.compatibility.v1"
	DurableItemActionConsumerKey          = "actions.items.v1"
	DurableSCMActionConsumerKey           = "actions.scm.v1"

	durableItemActionCutoverKey = "actions.items.canonical.v1"
)

// ActionCutover is the durable boundary after which canonical domain events
// are authoritative for automation admission.
type ActionCutover = actionevents.Cutover

// DurableActionIngress persists compatibility and SCM action facts without an
// in-memory admission queue.
type DurableActionIngress struct {
	db    database.Database
	store *events.Store
}

func NewDurableActionIngress(db database.Database) *DurableActionIngress {
	return &DurableActionIngress{db: db, store: events.NewStore(db)}
}

// Emit appends one durable trigger. Canonical item events replace legacy item
// trigger shapes after the recorded cutover.
func (i *DurableActionIngress) Emit(ctx context.Context, actionEvent *models.ActionEvent) error {
	if actionEvent == nil {
		return errors.New("action event is required")
	}
	if isItemActionTrigger(actionEvent.EventType) {
		cutover, err := i.CurrentCanonicalItemsCutover(ctx)
		if err != nil {
			return err
		}
		if cutover != nil {
			return nil
		}
	}

	input, err := durableActionEventInput(actionEvent)
	if err != nil {
		return err
	}

	return appendStandaloneDurableActionEvent(ctx, i.db, i.store, input, "action")
}

// EmitInTx appends an SCM observation in the transaction that admits its
// provider identity to the SCM ledger.
func (i *DurableActionIngress) EmitInTx(ctx context.Context, tx database.Tx, actionEvent *models.ActionEvent) error {
	if actionEvent == nil || !isSCMActionTrigger(actionEvent.EventType) {
		return errors.New("SCM action event is required")
	}
	input, err := durableActionEventInput(actionEvent)
	if err != nil {
		return err
	}
	if _, err := i.store.Append(ctx, tx, input); err != nil {
		return fmt.Errorf("append transactional SCM observation: %w", err)
	}
	return nil
}

func durableActionEventInput(actionEvent *models.ActionEvent) (events.NewEvent, error) {
	workspaceID := actionEvent.WorkspaceID
	input, err := actionevents.NewCompatibilityEvent(actionevents.CompatibilityInput{
		Payload: actionEvent, WorkspaceID: &workspaceID,
		AggregateType: "action_workspace", AggregateID: strconv.Itoa(workspaceID),
		EventType: DurableActionCompatibilityEvent, ActorUserID: actionEvent.ActorUserID,
		CorrelationID: actionEvent.ExecutionChainID,
	})
	if err != nil {
		return events.NewEvent{}, fmt.Errorf("encode durable action event: %w", err)
	}
	if actionEvent.ItemID > 0 {
		input.AggregateType = "action_item"
		input.AggregateID = strconv.Itoa(actionEvent.ItemID)
	}
	if isSCMActionTrigger(actionEvent.EventType) {
		input.Key = stableSCMEventKey(actionEvent)
		input.AggregateType = "scm_repository"
		input.AggregateID = fmt.Sprint(actionEvent.NewValues["repo.workspace_repository_id"])
		if input.AggregateID == "<nil>" || input.AggregateID == "" {
			input.AggregateID = strconv.Itoa(workspaceID)
		}
		input.Type = DurableSCMObservationEvent
		input.ActorKind = "integration"
		input.ActorRef = "scm"
		input.SourceKind = "scm"
	}
	return input, nil
}

// ActivateCanonicalItems records the boundary exactly once.
func (i *DurableActionIngress) ActivateCanonicalItems(ctx context.Context) (*ActionCutover, error) {
	return activateActionCutover(ctx, i.db, durableItemActionCutoverKey, "item action")
}

func activateActionCutover(ctx context.Context, db database.Database, cutoverKey, label string) (*ActionCutover, error) {
	return actionevents.ActivateCutover(ctx, db, cutoverKey, label)
}

func (i *DurableActionIngress) CurrentCanonicalItemsCutover(ctx context.Context) (*ActionCutover, error) {
	return currentActionCutover(ctx, i.db, durableItemActionCutoverKey)
}

func currentActionCutover(ctx context.Context, query actionCutoverQuery, cutoverKey string) (*ActionCutover, error) {
	return actionevents.CurrentCutover(ctx, query, cutoverKey)
}

type actionCutoverQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ConfigureDurableActionConsumers installs stable subscriptions for the
// compatibility, canonical item, and SCM action paths.
func ConfigureDurableActionConsumers(ctx context.Context, store *events.Store, cutover *ActionCutover) error {
	return actionevents.ConfigureCutoverConsumers(ctx, store, cutover, events.Consumer{
		Key: DurableItemActionConsumerKey, HandlerVersion: 1,
		EventTypes: []string{itemevents.Created, itemevents.Updated, itemevents.StatusChanged, itemevents.Linked},
	},
		events.Consumer{Key: DurableCompatibilityActionConsumerKey, HandlerVersion: 1, Active: true, StartEventID: 1, EventTypes: []string{DurableActionCompatibilityEvent}},
		events.Consumer{Key: DurableSCMActionConsumerKey, HandlerVersion: 1, Active: true, StartEventID: 1, EventTypes: []string{DurableSCMObservationEvent}},
	)
}

// PrepareDurableActionEngine installs subscriptions and handlers before the
// shared engine starts. Activation is a one-way deployment operation recorded
// in the database, so later replicas do not need the activation flag.
func PrepareDurableActionEngine(ctx context.Context, engine *events.Engine, actions *ActionService, activate bool) error {
	if engine == nil || actions == nil {
		return errors.New("domain event engine and action service are required")
	}
	cutover, err := actions.durableIngress.CurrentCanonicalItemsCutover(ctx)
	if err != nil {
		return err
	}
	if activate && cutover == nil {
		cutover, err = actions.durableIngress.ActivateCanonicalItems(ctx)
		if err != nil {
			return err
		}
	}
	if err := ConfigureDurableActionConsumers(ctx, engine.Store(), cutover); err != nil {
		return err
	}
	handler := NewDurableActionConsumer(actions.db, actions)
	for _, key := range []string{
		DurableCompatibilityActionConsumerKey,
		DurableItemActionConsumerKey,
		DurableSCMActionConsumerKey,
	} {
		if err := engine.RegisterHandler(key, handler); err != nil {
			return err
		}
	}
	return nil
}

// DurableActionConsumer adapts durable facts to existing action trigger
// semantics and owns frozen target execution state.
type DurableActionConsumer struct {
	actions *ActionService
	targets *actionevents.TargetStore
}

func NewDurableActionConsumer(db database.Database, actions *ActionService) *DurableActionConsumer {
	return &DurableActionConsumer{actions: actions, targets: actionevents.NewTargetStore(db)}
}

func (c *DurableActionConsumer) Handle(ctx context.Context, event events.Event) error {
	return handleDurableActionEvent(ctx, event, c.targets,
		actionEventFromDomainEvent,
		c.materializeTargets,
		func(actionEvent *models.ActionEvent) actionevents.Callbacks {
			return actionevents.Callbacks{
				Completed: func(actionID int) (bool, error) {
					existing, err := c.actions.repo.GetExecutionLogByDurableTarget(event.Key, actionID)
					return err == nil && existing.Status == models.ActionStatusCompleted, err
				},
				Execute: func(actionID int) (bool, error) {
					action, err := c.actions.repo.GetByID(actionID)
					if err != nil {
						return errors.Is(err, sql.ErrNoRows), fmt.Errorf("load frozen action %d: %w", actionID, err)
					}
					if err := c.actions.executeActionForEvent(ctx, action, actionEvent, nil, event.Key); err != nil {
						return false, fmt.Errorf("execute frozen action %d: %w", actionID, err)
					}
					return false, nil
				},
			}
		},
		&c.actions.actionsExecuted,
		&c.actions.eventsProcessed,
	)
}

func (c *DurableActionConsumer) materializeTargets(ctx context.Context, event events.Event, consumerKey string, actionEvent *models.ActionEvent) error {
	c.actions.cacheMu.RLock()
	actions := append([]*models.Action(nil), c.actions.actionCache[actionEvent.WorkspaceID]...)
	c.actions.cacheMu.RUnlock()
	var chain *ExecutionChain
	if actionEvent.ExecutionChainID != "" {
		chain = c.actions.chainStore.GetChain(actionEvent.ExecutionChainID)
	}
	matching := selectDurableActionTargets(actions, actionEvent.CascadeDepth, chain,
		func(action *models.Action) int { return action.ID },
		func(action *models.Action) string { return fmt.Sprintf("workspace:%d", action.ID) },
		func(action *models.Action) bool { return c.actions.matchesTrigger(action, actionEvent) },
	)

	return c.targets.Materialize(ctx, event, consumerKey, string(actionEvent.EventType), matching)
}

func actionEventFromDomainEvent(event events.Event) (*models.ActionEvent, string, error) {
	if event.PayloadVersion != 1 {
		return nil, "", fmt.Errorf("unsupported action event %s payload version %d", event.Type, event.PayloadVersion)
	}
	if event.Type == DurableActionCompatibilityEvent || event.Type == DurableSCMObservationEvent {
		var actionEvent models.ActionEvent
		if err := json.Unmarshal(event.Payload, &actionEvent); err != nil {
			return nil, "", fmt.Errorf("decode %s payload: %w", event.Type, err)
		}
		consumerKey := DurableCompatibilityActionConsumerKey
		if event.Type == DurableSCMObservationEvent {
			consumerKey = DurableSCMActionConsumerKey
		}
		return &actionEvent, consumerKey, nil
	}

	actionEvent := &models.ActionEvent{ActorUserID: eventActorUserID(event)}
	switch event.Type {
	case itemevents.Created:
		var payload itemevents.CreatedV1
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, "", fmt.Errorf("decode item.created: %w", err)
		}
		actionEvent.EventType = models.ActionTriggerItemCreated
		actionEvent.WorkspaceID = payload.Item.WorkspaceID
		actionEvent.ItemID = payload.Item.ID
		actionEvent.NewValues = itemSnapshotValues(payload.Item)
		applyAutomation(actionEvent, payload.Automation)
	case itemevents.Updated:
		var payload itemevents.UpdatedV1
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, "", fmt.Errorf("decode item.updated: %w", err)
		}
		actionEvent.EventType = models.ActionTriggerItemUpdated
		actionEvent.WorkspaceID = payload.Item.WorkspaceID
		actionEvent.ItemID = payload.Item.ID
		actionEvent.OldValues, actionEvent.NewValues = changedValues(payload.Changes)
		applyAutomation(actionEvent, payload.Automation)
	case itemevents.StatusChanged:
		var payload itemevents.StatusChangedV1
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, "", fmt.Errorf("decode item.status_changed: %w", err)
		}
		actionEvent.EventType = models.ActionTriggerStatusTransition
		actionEvent.WorkspaceID = payload.Item.WorkspaceID
		actionEvent.ItemID = payload.Item.ID
		actionEvent.OldValues = map[string]any{"status_id": payload.OldStatusID}
		actionEvent.NewValues = itemSnapshotValues(payload.Item)
		actionEvent.NewValues["status_id"] = payload.NewStatusID
		applyAutomation(actionEvent, payload.Automation)
	case itemevents.Linked:
		var payload itemevents.LinkChangedV1
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, "", fmt.Errorf("decode item.linked: %w", err)
		}
		actionEvent.EventType = models.ActionTriggerItemLinked
		actionEvent.ItemID = payload.ItemID
		if event.WorkspaceID != nil {
			actionEvent.WorkspaceID = *event.WorkspaceID
		}
		actionEvent.NewValues = map[string]any{
			"link_id": payload.LinkID, "link_type_id": payload.LinkTypeID,
			"direction": payload.Direction, "other_type": payload.OtherType, "other_id": payload.OtherID,
		}
		applyAutomation(actionEvent, payload.Automation)
	default:
		return nil, "", fmt.Errorf("unsupported durable action event type %q", event.Type)
	}
	return actionEvent, DurableItemActionConsumerKey, nil
}

func itemSnapshotValues(item itemevents.ItemSnapshot) map[string]any {
	return map[string]any{
		"id": item.ID, "workspace_id": item.WorkspaceID, "workspace_item_number": item.WorkspaceItemNumber,
		"item_type_id": item.ItemTypeID, "title": item.Title, "description": item.Description,
		"status_id": item.StatusID, "priority_id": item.PriorityID, "assignee_id": item.AssigneeID,
		"creator_id": item.CreatorID, "parent_id": item.ParentID, "iteration_id": item.IterationID,
		"project_id": item.ProjectID, "request_type_id": item.RequestTypeID,
	}
}

func changedValues(changes []itemevents.FieldChange) (oldValues, newValues map[string]any) {
	oldValues = make(map[string]any, len(changes))
	newValues = make(map[string]any, len(changes))
	for _, change := range changes {
		oldValues[change.Field] = change.OldValue
		newValues[change.Field] = change.NewValue
	}
	return oldValues, newValues
}

func applyAutomation(event *models.ActionEvent, automation *itemevents.AutomationContext) {
	if automation == nil {
		return
	}
	event.TriggeredByAction = automation.TriggeredByAction
	event.ExecutionChainID = automation.ExecutionChainID
	event.CascadeDepth = automation.CascadeDepth
	event.SourceApplication = automation.SourceApplication
}

func eventActorUserID(event events.Event) int {
	if event.ActorKind != "user" {
		return 0
	}
	userID, _ := strconv.Atoi(event.ActorRef)
	return userID
}

func isItemActionTrigger(trigger models.ActionTriggerType) bool {
	switch trigger {
	case models.ActionTriggerItemCreated, models.ActionTriggerItemUpdated,
		models.ActionTriggerStatusTransition, models.ActionTriggerItemLinked:
		return true
	default:
		return false
	}
}

func isSCMActionTrigger(trigger models.ActionTriggerType) bool {
	switch trigger {
	case models.ActionTriggerSCMTagCreated, models.ActionTriggerSCMReleaseBranchCreated,
		models.ActionTriggerSCMPRLinked, models.ActionTriggerSCMPRMerged:
		return true
	default:
		return false
	}
}

func stableSCMEventKey(event *models.ActionEvent) string {
	identity := fmt.Sprintf("%s\x00%d\x00%d\x00%v\x00%v\x00%v\x00%v\x00%v",
		event.EventType, event.WorkspaceID, event.ItemID,
		event.NewValues["repo.workspace_repository_id"], event.NewValues["ref.type"],
		event.NewValues["ref.name"], event.NewValues["ref.sha"], event.NewValues["pr.number"],
	)
	sum := sha256.Sum256([]byte(identity))
	return "scm-action-" + hex.EncodeToString(sum[:])
}
