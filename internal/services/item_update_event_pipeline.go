package services

import (
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
)

func itemUpdateStatusName(db database.Database, statusID *int) (string, error) {
	if db == nil || statusID == nil {
		return "", nil
	}
	var statusName string
	err := db.QueryRow("SELECT name FROM statuses WHERE id = ?", *statusID).Scan(&statusName)
	return statusName, err
}

func emitItemUpdateNotifications(
	notify func(*NotificationEvent),
	original, updated *models.Item,
	statusChanged, assigneeChanged bool,
	actorUserID int,
	actorName, statusName string,
) {
	if statusChanged {
		event := newItemUpdateNotification(models.EventStatusChanged, "Status Changed", original, updated, actorUserID, actorName)
		event.TemplateData["status.name"] = statusName
		notify(event)
	}
	if assigneeChanged {
		notify(newItemUpdateNotification(models.EventItemAssigned, "Item Assigned", original, updated, actorUserID, actorName))
	}
	if !statusChanged && !assigneeChanged {
		notify(newItemUpdateNotification(models.EventItemUpdated, "Item Updated", original, updated, actorUserID, actorName))
	}
}

func newItemUpdateNotification(
	eventType, title string,
	original, updated *models.Item,
	actorUserID int,
	actorName string,
) *NotificationEvent {
	return &NotificationEvent{
		EventType:   eventType,
		WorkspaceID: updated.WorkspaceID,
		ActorUserID: actorUserID,
		ItemID:      updated.ID,
		AssigneeID:  updated.AssigneeID,
		CreatorID:   original.CreatorID,
		Title:       title,
		TemplateData: map[string]any{
			"item.title": updated.Title,
			"item.key":   fmt.Sprintf("%s-%d", updated.WorkspaceKey, updated.WorkspaceItemNumber),
			"item.id":    updated.ID,
			"user.name":  actorName,
		},
	}
}

func newStatusTransitionActionEvent(original, updated *models.Item, actorUserID int) *models.ActionEvent {
	return &models.ActionEvent{
		EventType:   models.ActionTriggerStatusTransition,
		WorkspaceID: updated.WorkspaceID,
		ItemID:      updated.ID,
		ActorUserID: actorUserID,
		OldValues:   map[string]any{"status_id": original.StatusID},
		NewValues: map[string]any{
			"status_id":   updated.StatusID,
			"title":       updated.Title,
			"assignee_id": updated.AssigneeID,
			"creator_id":  updated.CreatorID,
		},
	}
}

func dispatchItemUpdateWebhooks(dispatcher WebhookDispatcher, updated *models.Item, statusChanged, assigneeChanged bool) {
	if dispatcher == nil {
		return
	}
	if statusChanged {
		dispatcher.DispatchEvent("status.changed", updated)
	}
	if assigneeChanged {
		dispatcher.DispatchEvent("item.assigned", updated)
	}
	dispatcher.DispatchEvent("item.updated", updated)
}
