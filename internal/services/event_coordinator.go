package services

import (
	"fmt"
	"log/slog"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ActionEventEmitter is an interface for emitting action automation events.
type ActionEventEmitter interface {
	EmitActionEvent(event *models.ActionEvent)
}

// AssetActionEventEmitter is an interface for emitting asset action events.
type AssetActionEventEmitter interface {
	EmitAssetActionEvent(event *models.AssetActionEvent)
}

// ActionContext carries cascade context through event emission,
// enabling cross-application loop prevention.
type ActionContext struct {
	TriggeredByAction bool
	ExecutionChainID  string
	CascadeDepth      int
	SourceApplication string
}

// EventCoordinator centralizes side effect handling (notifications, webhooks, activity tracking, actions)
// for item operations. This ensures consistent behavior across both internal handlers and REST API.
type EventCoordinator struct {
	db                  database.Database
	notificationService *NotificationService
	activityTracker     *ActivityTracker
	webhookDispatcher   WebhookDispatcher
	actionService       ActionEventEmitter
	assetActionService  AssetActionEventEmitter
}

// NewEventCoordinator creates a new EventCoordinator.
func NewEventCoordinator(db database.Database) *EventCoordinator {
	return &EventCoordinator{
		db: db,
	}
}

// SetNotificationService sets the notification service for emitting events.
func (ec *EventCoordinator) SetNotificationService(ns *NotificationService) {
	ec.notificationService = ns
}

// SetActivityTracker sets the activity tracker for tracking user activity.
func (ec *EventCoordinator) SetActivityTracker(at *ActivityTracker) {
	ec.activityTracker = at
}

// SetWebhookDispatcher sets the webhook dispatcher for dispatching webhook events.
func (ec *EventCoordinator) SetWebhookDispatcher(wd WebhookDispatcher) {
	ec.webhookDispatcher = wd
}

// SetActionService sets the action service for automation workflows.
func (ec *EventCoordinator) SetActionService(as ActionEventEmitter) {
	ec.actionService = as
}

// SetAssetActionService sets the asset action service for asset automation workflows.
func (ec *EventCoordinator) SetAssetActionService(as AssetActionEventEmitter) {
	ec.assetActionService = as
}

// GetAssetActionService returns the asset action service, if set.
func (ec *EventCoordinator) GetAssetActionService() AssetActionEventEmitter {
	return ec.assetActionService
}

// EmitItemCreated emits events for a newly created item.
// The last variadic string arguments are treated as actor username, except that
// an ActionContext can be passed via EmitItemCreatedWithContext.
func (ec *EventCoordinator) EmitItemCreated(item *models.Item, actorUserID int, actorUsername ...string) {
	ec.emitItemCreatedInternal(item, actorUserID, nil, actorUsername...)
}

// EmitItemCreatedWithContext emits events for a newly created item with cascade context.
func (ec *EventCoordinator) EmitItemCreatedWithContext(item *models.Item, actorUserID int, ctx ActionContext, actorUsername ...string) {
	ec.emitItemCreatedInternal(item, actorUserID, &ctx, actorUsername...)
}

func (ec *EventCoordinator) emitItemCreatedInternal(item *models.Item, actorUserID int, actionCtx *ActionContext, actorUsername ...string) {
	actorName := resolveActorName(actorUserID, actorUsername)

	// Construct the item key (e.g., "TST-1")
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	// Emit notification event
	if ec.notificationService != nil {
		ec.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventItemCreated,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: actorUserID,
			ItemID:      item.ID,
			AssigneeID:  item.AssigneeID,
			CreatorID:   &actorUserID,
			Title:       "New Item Created",
			TemplateData: map[string]interface{}{
				"item.title":     item.Title,
				"item.key":       itemKey,
				"item.id":        item.ID,
				"user.name":      actorName,
				"workspace.name": item.WorkspaceName,
				"workspace.key":  item.WorkspaceKey,
			},
		})
	}

	// Emit action event for automation
	if ec.actionService != nil {
		event := &models.ActionEvent{
			EventType:   models.ActionTriggerItemCreated,
			WorkspaceID: item.WorkspaceID,
			ItemID:      item.ID,
			ActorUserID: actorUserID,
			NewValues: map[string]interface{}{
				"title":        item.Title,
				"status_id":    item.StatusID,
				"item_type_id": item.ItemTypeID,
				"assignee_id":  item.AssigneeID,
				"creator_id":   item.CreatorID,
				"priority_id":  item.PriorityID,
			},
		}
		if actionCtx != nil {
			event.TriggeredByAction = actionCtx.TriggeredByAction
			event.ExecutionChainID = actionCtx.ExecutionChainID
			event.CascadeDepth = actionCtx.CascadeDepth
			event.SourceApplication = actionCtx.SourceApplication
		}
		ec.actionService.EmitActionEvent(event)
	}

	// Dispatch webhook event
	if ec.webhookDispatcher != nil {
		go ec.webhookDispatcher.DispatchEvent("item.created", item)
	}
}

// EmitItemUpdated emits events for an updated item.
func (ec *EventCoordinator) EmitItemUpdated(original, updated *models.Item, statusChanged, assigneeChanged bool, actorUserID int, fieldChanges []HistoryEntry, actorUsername ...string) {
	actorName := resolveActorName(actorUserID, actorUsername)

	// Construct the item key (e.g., "TST-1")
	itemKey := fmt.Sprintf("%s-%d", updated.WorkspaceKey, updated.WorkspaceItemNumber)

	// Emit notification events
	if ec.notificationService != nil {
		// Get status name if status changed
		var statusName string
		if statusChanged && updated.StatusID != nil {
			_ = ec.db.QueryRow("SELECT name FROM statuses WHERE id = ?", *updated.StatusID).Scan(&statusName)
		}

		// Emit status changed notification
		if statusChanged {
			ec.notificationService.EmitEvent(&NotificationEvent{
				EventType:   models.EventStatusChanged,
				WorkspaceID: updated.WorkspaceID,
				ActorUserID: actorUserID,
				ItemID:      updated.ID,
				AssigneeID:  updated.AssigneeID,
				CreatorID:   original.CreatorID,
				Title:       "Status Changed",
				TemplateData: map[string]interface{}{
					"item.title":  updated.Title,
					"item.key":    itemKey,
					"item.id":     updated.ID,
					"status.name": statusName,
					"user.name":   actorName,
				},
			})
		}

		// Emit assignee changed notification
		if assigneeChanged {
			ec.notificationService.EmitEvent(&NotificationEvent{
				EventType:   models.EventItemAssigned,
				WorkspaceID: updated.WorkspaceID,
				ActorUserID: actorUserID,
				ItemID:      updated.ID,
				AssigneeID:  updated.AssigneeID,
				CreatorID:   original.CreatorID,
				Title:       "Item Assigned",
				TemplateData: map[string]interface{}{
					"item.title": updated.Title,
					"item.key":   itemKey,
					"item.id":    updated.ID,
					"user.name":  actorName,
				},
			})
		}

		// Emit item updated notification (when not status or assignee change)
		if !statusChanged && !assigneeChanged {
			ec.notificationService.EmitEvent(&NotificationEvent{
				EventType:   models.EventItemUpdated,
				WorkspaceID: updated.WorkspaceID,
				ActorUserID: actorUserID,
				ItemID:      updated.ID,
				AssigneeID:  updated.AssigneeID,
				CreatorID:   original.CreatorID,
				Title:       "Item Updated",
				TemplateData: map[string]interface{}{
					"item.title": updated.Title,
					"item.key":   itemKey,
					"item.id":    updated.ID,
					"user.name":  actorName,
				},
			})
		}
	}

	// Emit action events for automation
	if ec.actionService != nil {
		if statusChanged {
			ec.actionService.EmitActionEvent(&models.ActionEvent{
				EventType:   models.ActionTriggerStatusTransition,
				WorkspaceID: updated.WorkspaceID,
				ItemID:      updated.ID,
				ActorUserID: actorUserID,
				OldValues: map[string]interface{}{
					"status_id": original.StatusID,
				},
				NewValues: map[string]interface{}{
					"status_id":   updated.StatusID,
					"title":       updated.Title,
					"assignee_id": updated.AssigneeID,
					"creator_id":  updated.CreatorID,
				},
			})
		} else {
			// Build OldValues/NewValues dynamically from field changes
			oldVals := make(map[string]interface{})
			newVals := make(map[string]interface{})
			for _, fc := range fieldChanges {
				oldVals[fc.FieldName] = fc.OldValue
				newVals[fc.FieldName] = fc.NewValue
			}
			ec.actionService.EmitActionEvent(&models.ActionEvent{
				EventType:   models.ActionTriggerItemUpdated,
				WorkspaceID: updated.WorkspaceID,
				ItemID:      updated.ID,
				ActorUserID: actorUserID,
				OldValues:   oldVals,
				NewValues:   newVals,
			})
		}
	}

	// Dispatch webhook events
	if ec.webhookDispatcher != nil {
		if statusChanged {
			go ec.webhookDispatcher.DispatchEvent("status.changed", updated)
		}
		if assigneeChanged {
			go ec.webhookDispatcher.DispatchEvent("item.assigned", updated)
		}
		// Always dispatch item.updated for any update
		go ec.webhookDispatcher.DispatchEvent("item.updated", updated)
	}
}

// EmitItemDeleted emits events for a deleted item.
func (ec *EventCoordinator) EmitItemDeleted(item *models.Item, actorUserID, descendantCount int, actorUsername ...string) {
	actorName := resolveActorName(actorUserID, actorUsername)

	// Emit notification event
	if ec.notificationService != nil {
		ec.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventItemDeleted,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: actorUserID,
			ItemID:      item.ID,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       "Item Deleted",
			TemplateData: map[string]interface{}{
				"item.title":  item.Title,
				"item.id":     item.ID,
				"user.name":   actorName,
				"descendants": descendantCount,
			},
		})
	}

	// Dispatch webhook event
	if ec.webhookDispatcher != nil {
		go ec.webhookDispatcher.DispatchEvent("item.deleted", item)
	}
}

// EmitStatusChanged emits events specifically for status changes.
func (ec *EventCoordinator) EmitStatusChanged(item *models.Item, oldStatusID, newStatusID *int, actorUserID int, actorUsername ...string) {
	actorName := resolveActorName(actorUserID, actorUsername)
	var newStatusName string
	if newStatusID != nil {
		_ = ec.db.QueryRow("SELECT name FROM statuses WHERE id = ?", *newStatusID).Scan(&newStatusName)
	}

	// Construct the item key
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	// Emit notification event
	if ec.notificationService != nil {
		ec.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventStatusChanged,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: actorUserID,
			ItemID:      item.ID,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       "Status Changed",
			TemplateData: map[string]interface{}{
				"item.title":  item.Title,
				"item.key":    itemKey,
				"item.id":     item.ID,
				"status.name": newStatusName,
				"user.name":   actorName,
			},
		})
	}

	// Emit action event for automation
	if ec.actionService != nil {
		ec.actionService.EmitActionEvent(&models.ActionEvent{
			EventType:   models.ActionTriggerStatusTransition,
			WorkspaceID: item.WorkspaceID,
			ItemID:      item.ID,
			ActorUserID: actorUserID,
			OldValues: map[string]interface{}{
				"status_id": oldStatusID,
			},
			NewValues: map[string]interface{}{
				"status_id":   newStatusID,
				"title":       item.Title,
				"assignee_id": item.AssigneeID,
				"creator_id":  item.CreatorID,
			},
		})
	}

	// Dispatch webhook event
	if ec.webhookDispatcher != nil {
		go ec.webhookDispatcher.DispatchEvent("status.changed", item)
	}
}

// TrackItemActivity tracks user activity on an item (view, edit, comment).
func (ec *EventCoordinator) TrackItemActivity(userID, itemID int, activityType ActivityType) {
	if ec.activityTracker != nil {
		if err := ec.activityTracker.TrackItemActivity(userID, itemID, activityType); err != nil {
			slog.Warn("failed to track item activity",
				slog.String("component", "event_coordinator"),
				slog.Int("user_id", userID),
				slog.Int("item_id", itemID),
				slog.String("activity_type", string(activityType)),
				slog.Any("error", err),
			)
		}
	}
}

// GetItemForWebhook loads an item with full details for webhook payloads.
func (ec *EventCoordinator) GetItemForWebhook(itemID int) (*models.Item, error) {
	itemRepo := repository.NewItemRepository(ec.db)
	return itemRepo.FindByIDWithDetails(itemID)
}

// resolveActorName returns the username from the variadic param, or a fallback.
func resolveActorName(actorUserID int, actorUsername []string) string {
	if len(actorUsername) > 0 && actorUsername[0] != "" {
		return actorUsername[0]
	}
	return fmt.Sprintf("User #%d", actorUserID)
}
