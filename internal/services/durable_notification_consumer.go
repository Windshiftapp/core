package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"windshift/internal/actionevents"
	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
)

const (
	DurableNotificationCompatibilityEvent = "notification.requested.v1"

	DurableNotificationCompatibilityConsumerKey = "notifications.compatibility.v1"
	DurableItemNotificationConsumerKey          = "notifications.items.v1"

	durableItemNotificationCutoverKey = "notifications.items.canonical.v1"
)

// DurableNotificationIngress journals notification types that do not yet have
// a canonical mutation event. Canonical item events are admitted in their
// source transaction after the cutover.
type DurableNotificationIngress struct {
	engine    *events.Engine
	canonical bool
}

func (i *DurableNotificationIngress) Emit(ctx context.Context, notificationEvent *NotificationEvent) error {
	if notificationEvent == nil {
		return errors.New("notification event is required")
	}
	if i.canonical && isCanonicalItemNotification(notificationEvent.EventType) {
		return nil
	}
	workspaceID := notificationEvent.WorkspaceID
	input, err := actionevents.NewCompatibilityEvent(actionevents.CompatibilityInput{
		Payload: notificationEvent, WorkspaceID: &workspaceID,
		AggregateType: "notification_item", AggregateID: fmt.Sprint(notificationEvent.ItemID),
		EventType: DurableNotificationCompatibilityEvent, ActorUserID: notificationEvent.ActorUserID,
	})
	if err != nil {
		return fmt.Errorf("encode durable notification event: %w", err)
	}
	if _, err := i.engine.AppendStandalone(ctx, input); err != nil {
		return fmt.Errorf("append durable notification event: %w", err)
	}
	return nil
}

// PrepareDurableNotificationEngine records the item cutover and installs both
// canonical and compatibility consumers before the shared engine starts.
func PrepareDurableNotificationEngine(ctx context.Context, engine *events.Engine, notifications *NotificationService) error {
	if engine == nil || notifications == nil {
		return errors.New("domain event engine and notification service are required")
	}
	cutover, err := actionevents.CurrentCutover(ctx, notifications.db, durableItemNotificationCutoverKey)
	if err != nil {
		return fmt.Errorf("load notification cutover: %w", err)
	}
	if cutover == nil {
		cutover, err = actionevents.ActivateCutover(ctx, notifications.db, durableItemNotificationCutoverKey, "item notification")
		if err != nil {
			return err
		}
	}
	if err := actionevents.ConfigureCutoverConsumers(ctx, engine.Store(), cutover, events.Consumer{
		Key: DurableItemNotificationConsumerKey, HandlerVersion: 1,
		EventTypes: []string{itemevents.Created, itemevents.Updated, itemevents.StatusChanged, itemevents.Deleted, itemevents.CommentCreated, itemevents.Linked, itemevents.Unlinked},
	}, events.Consumer{
		Key: DurableNotificationCompatibilityConsumerKey, HandlerVersion: 1,
		Active: true, StartEventID: 1, EventTypes: []string{DurableNotificationCompatibilityEvent},
	}); err != nil {
		return err
	}
	handler := &DurableNotificationConsumer{db: notifications.db, notifications: notifications}
	for _, key := range []string{DurableNotificationCompatibilityConsumerKey, DurableItemNotificationConsumerKey} {
		if err := engine.RegisterHandler(key, handler); err != nil {
			return err
		}
	}
	notifications.durableIngress = &DurableNotificationIngress{engine: engine, canonical: true}
	return nil
}

// DurableNotificationConsumer turns committed facts into idempotent recipient rows.
type DurableNotificationConsumer struct {
	db            database.Database
	notifications *NotificationService
}

func (c *DurableNotificationConsumer) Handle(ctx context.Context, event events.Event) error {
	notificationEvents, err := c.notificationEvents(ctx, event)
	if err != nil {
		return err
	}
	for _, notificationEvent := range notificationEvents {
		if err := c.notifications.processEventWithKey(ctx, notificationEvent, event.Key); err != nil {
			return err
		}
	}
	return nil
}

func (c *DurableNotificationConsumer) notificationEvents(ctx context.Context, event events.Event) ([]*NotificationEvent, error) {
	if event.PayloadVersion != 1 {
		return nil, events.Permanent(fmt.Errorf("unsupported notification event %s payload version %d", event.Type, event.PayloadVersion))
	}
	if event.Type == DurableNotificationCompatibilityEvent {
		var notificationEvent NotificationEvent
		if err := json.Unmarshal(event.Payload, &notificationEvent); err != nil {
			return nil, events.Permanent(fmt.Errorf("decode durable notification event: %w", err))
		}
		return []*NotificationEvent{&notificationEvent}, nil
	}

	actorID := eventActorUserID(event)
	actorName := c.actorName(ctx, actorID, event.ActorRef)
	switch event.Type {
	case itemevents.Created:
		var payload itemevents.CreatedV1
		if err := decodeNotificationPayload(event, &payload); err != nil {
			return nil, err
		}
		base, err := c.itemSnapshotEvent(ctx, payload.Item, actorID, actorName)
		if err != nil {
			return nil, err
		}
		base.EventType, base.Title = models.EventItemCreated, "New Item Created"
		return []*NotificationEvent{base}, nil
	case itemevents.Updated:
		var payload itemevents.UpdatedV1
		if err := decodeNotificationPayload(event, &payload); err != nil {
			return nil, err
		}
		base, err := c.itemSnapshotEvent(ctx, payload.Item, actorID, actorName)
		if err != nil {
			return nil, err
		}
		if changedNotificationField(payload.Changes, "assignee_id") {
			assigned := cloneNotificationEvent(base)
			assigned.EventType, assigned.Title = models.EventItemAssigned, "Item Assigned"
			return []*NotificationEvent{assigned}, nil
		}
		base.EventType, base.Title = models.EventItemUpdated, "Item Updated"
		return []*NotificationEvent{base}, nil
	case itemevents.StatusChanged:
		var payload itemevents.StatusChangedV1
		if err := decodeNotificationPayload(event, &payload); err != nil {
			return nil, err
		}
		base, err := c.itemSnapshotEvent(ctx, payload.Item, actorID, actorName)
		if err != nil {
			return nil, err
		}
		statusName := ""
		if payload.NewStatusID != nil {
			if err := c.db.QueryRowContext(ctx, "SELECT name FROM statuses WHERE id = ?", *payload.NewStatusID).Scan(&statusName); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("load notification status: %w", err)
			}
		}
		base.EventType, base.Title = models.EventStatusChanged, "Status Changed"
		base.TemplateData["status.name"] = statusName
		out := []*NotificationEvent{base}
		if changedNotificationField(payload.Changes, "assignee_id") {
			assigned := cloneNotificationEvent(base)
			assigned.EventType, assigned.Title = models.EventItemAssigned, "Item Assigned"
			delete(assigned.TemplateData, "status.name")
			out = append(out, assigned)
		}
		return out, nil
	case itemevents.Deleted:
		var payload itemevents.DeletedV1
		if err := decodeNotificationPayload(event, &payload); err != nil {
			return nil, err
		}
		base, err := c.itemSnapshotEvent(ctx, payload.Item, actorID, actorName)
		if err != nil {
			return nil, err
		}
		base.EventType, base.Title = models.EventItemDeleted, "Item Deleted"
		base.TemplateData["descendants"] = payload.DescendantCount
		return []*NotificationEvent{base}, nil
	case itemevents.CommentCreated:
		var payload itemevents.CommentCreatedV1
		if err := decodeNotificationPayload(event, &payload); err != nil {
			return nil, err
		}
		if payload.IsPrivate || payload.SuppressSideEffects {
			return nil, nil
		}
		item, err := repository.NewItemRepository(c.db).FindByIDWithDetailsContext(ctx, payload.ItemID)
		if err != nil {
			return nil, fmt.Errorf("load comment notification item: %w", err)
		}
		base := notificationEventFromItem(item, actorID, actorName)
		base.EventType, base.Title = models.EventCommentCreated, "New Comment Added"
		return []*NotificationEvent{base}, nil
	case itemevents.Linked, itemevents.Unlinked:
		var payload itemevents.LinkChangedV1
		if err := decodeNotificationPayload(event, &payload); err != nil {
			return nil, err
		}
		if payload.SourceType != "item" || payload.Direction != "outgoing" {
			return nil, nil
		}
		item, err := repository.NewItemRepository(c.db).FindByIDWithDetailsContext(ctx, payload.ItemID)
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("load link notification item: %w", err)
		}
		base := notificationEventFromItem(item, actorID, actorName)
		if event.Type == itemevents.Linked {
			base.EventType, base.Title = models.EventItemLinked, "Item Linked"
		} else {
			base.EventType, base.Title = models.EventItemUnlinked, "Item Unlinked"
		}
		base.ReferencedEntityType = payload.OtherType
		base.ReferencedEntityID = payload.OtherID
		base.ReferencedWorkspaceID, base.ReferencedWorkspacePermission = (&ItemLinkService{db: c.db}).notificationReferenceAccess(payload.OtherType, payload.OtherID)
		base.TemplateData["target.id"] = payload.OtherID
		base.TemplateData["target.title"] = fmt.Sprintf("%s #%d", payload.OtherType, payload.OtherID)
		return []*NotificationEvent{base}, nil
	default:
		return nil, events.Permanent(fmt.Errorf("unsupported durable notification event type %q", event.Type))
	}
}

func decodeNotificationPayload(event events.Event, target any) error {
	if err := json.Unmarshal(event.Payload, target); err != nil {
		return events.Permanent(fmt.Errorf("decode %s notification payload: %w", event.Type, err))
	}
	return nil
}

func (c *DurableNotificationConsumer) actorName(ctx context.Context, actorID int, fallback string) string {
	if actorID <= 0 {
		return fallback
	}
	var username string
	if err := c.db.QueryRowContext(ctx, "SELECT username FROM users WHERE id = ?", actorID).Scan(&username); err == nil {
		return username
	}
	return resolveActorName(actorID, nil)
}

func (c *DurableNotificationConsumer) itemSnapshotEvent(ctx context.Context, item itemevents.ItemSnapshot, actorID int, actorName string) (*NotificationEvent, error) {
	var workspaceName, workspaceKey string
	if err := c.db.QueryRowContext(ctx, "SELECT name, key FROM workspaces WHERE id = ?", item.WorkspaceID).Scan(&workspaceName, &workspaceKey); err != nil {
		return nil, fmt.Errorf("load notification workspace: %w", err)
	}
	return &NotificationEvent{
		WorkspaceID: item.WorkspaceID, ActorUserID: actorID, ItemID: item.ID,
		AssigneeID: item.AssigneeID, CreatorID: item.CreatorID,
		TemplateData: map[string]any{
			"item.title": item.Title, "item.key": fmt.Sprintf("%s-%d", workspaceKey, item.WorkspaceItemNumber),
			"item.id": item.ID, "user.name": actorName, "workspace.name": workspaceName, "workspace.key": workspaceKey,
		},
	}, nil
}

func notificationEventFromItem(item *models.Item, actorID int, actorName string) *NotificationEvent {
	return &NotificationEvent{
		WorkspaceID: item.WorkspaceID, ActorUserID: actorID, ItemID: item.ID,
		AssigneeID: item.AssigneeID, CreatorID: item.CreatorID,
		TemplateData: map[string]any{
			"item.title": item.Title, "item.key": fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber),
			"item.id": item.ID, "user.name": actorName, "workspace.name": item.WorkspaceName, "workspace.key": item.WorkspaceKey,
		},
	}
}

func changedNotificationField(changes []itemevents.FieldChange, field string) bool {
	for _, change := range changes {
		if change.Field == field {
			return true
		}
	}
	return false
}

func cloneNotificationEvent(source *NotificationEvent) *NotificationEvent {
	clone := *source
	clone.TemplateData = make(map[string]any, len(source.TemplateData))
	for key, value := range source.TemplateData {
		clone.TemplateData[key] = value
	}
	return &clone
}

func isCanonicalItemNotification(eventType string) bool {
	switch eventType {
	case models.EventItemCreated, models.EventItemUpdated, models.EventItemDeleted,
		models.EventItemAssigned, models.EventStatusChanged, models.EventCommentCreated,
		models.EventItemLinked, models.EventItemUnlinked:
		return true
	default:
		return false
	}
}
