package services

import (
	"database/sql"
	"errors"
	"log/slog"

	"windshift/internal/database"
	"windshift/internal/models"
)

// LegacyItemUpdatedEmitter preserves the original per-service event pipeline
// (notifications, automation actions, webhooks) for server embeddings that do
// not install an EventCoordinator. The HTTP transport installs it as the
// default item-update emitter; wiring a coordinator replaces it. The action
// and webhook sinks are attached through setters when the server wiring
// provides them.
type LegacyItemUpdatedEmitter struct {
	db      database.Database
	notify  func(*NotificationEvent)
	action  ActionEventEmitter
	webhook WebhookDispatcher
}

func NewLegacyItemUpdatedEmitter(db database.Database, notify func(*NotificationEvent), action ActionEventEmitter, webhook WebhookDispatcher) *LegacyItemUpdatedEmitter {
	return &LegacyItemUpdatedEmitter{db: db, notify: notify, action: action, webhook: webhook}
}

func (e *LegacyItemUpdatedEmitter) SetAction(action ActionEventEmitter) {
	e.action = action
}

func (e *LegacyItemUpdatedEmitter) SetWebhook(webhook WebhookDispatcher) {
	e.webhook = webhook
}

func (e *LegacyItemUpdatedEmitter) EmitItemUpdated(original, updated *models.Item, statusChanged, assigneeChanged bool, actorUserID int, fieldChanges []HistoryEntry, actorUsername ...string) {
	actorName := resolveActorName(actorUserID, actorUsername)

	if e.notify != nil {
		var statusName string
		if statusChanged {
			var err error
			statusName, err = itemUpdateStatusName(e.db, updated.StatusID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				slog.Warn("failed to load status name", slog.Int("status_id", *updated.StatusID), slog.Any("error", err))
			}
		}
		emitItemUpdateNotifications(e.notify, original, updated,
			statusChanged, assigneeChanged, actorUserID, actorName, statusName)
	}

	if e.action != nil {
		if statusChanged {
			e.action.EmitActionEvent(newStatusTransitionActionEvent(original, updated, actorUserID))
		} else {
			e.action.EmitActionEvent(buildItemUpdatedActionEvent(updated, actorUserID, fieldChanges))
		}
	}

	dispatchItemUpdateWebhooks(e.webhook, updated, statusChanged, assigneeChanged)
}
