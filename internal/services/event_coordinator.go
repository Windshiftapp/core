package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ActionEventEmitter is an interface for emitting action automation events.
type ActionEventEmitter interface {
	EmitActionEvent(event *models.ActionEvent)
}

// ActionContext carries cascade context through event emission,
// enabling cross-application loop prevention.
type ActionContext struct {
	TriggeredByAction bool
	ExecutionChainID  string
	CausationEventKey string
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
	magicLinkService    *MagicLinkService
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

// SetMagicLinkService wires the magic-link service so approval steps that
// resolve to portal customers can send a magic-link email pointing at the
// customer-facing decide page.
func (ec *EventCoordinator) SetMagicLinkService(ml *MagicLinkService) {
	ec.magicLinkService = ml
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
			TemplateData: map[string]any{
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
			NewValues: map[string]any{
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
		ec.webhookDispatcher.DispatchEvent("item.created", item)
	}
}

// EmitItemUpdated emits events for an updated item.
func (ec *EventCoordinator) EmitItemUpdated(original, updated *models.Item, statusChanged, assigneeChanged bool, actorUserID int, fieldChanges []HistoryEntry, actorUsername ...string) {
	ec.emitItemUpdatedInternal(original, updated, statusChanged, assigneeChanged, actorUserID, fieldChanges, nil, actorUsername...)
}

// EmitItemUpdatedWithContext preserves automation cascade provenance while
// sharing the notification, action, and webhook pipeline with user updates.
func (ec *EventCoordinator) EmitItemUpdatedWithContext(original, updated *models.Item, statusChanged, assigneeChanged bool, actorUserID int, fieldChanges []HistoryEntry, actionContext ActionContext, actorUsername ...string) {
	ec.emitItemUpdatedInternal(original, updated, statusChanged, assigneeChanged, actorUserID, fieldChanges, &actionContext, actorUsername...)
}

func (ec *EventCoordinator) emitItemUpdatedInternal(original, updated *models.Item, statusChanged, assigneeChanged bool, actorUserID int, fieldChanges []HistoryEntry, actionContext *ActionContext, actorUsername ...string) {
	actorName := resolveActorName(actorUserID, actorUsername)

	if ec.notificationService != nil {
		var statusName string
		if statusChanged {
			statusName, _ = itemUpdateStatusName(ec.db, updated.StatusID)
		}
		emitItemUpdateNotifications(ec.notificationService.EmitEvent, original, updated,
			statusChanged, assigneeChanged, actorUserID, actorName, statusName)
	}

	if ec.actionService != nil {
		if statusChanged {
			event := newStatusTransitionActionEvent(original, updated, actorUserID)
			applyActionContext(event, actionContext)
			ec.actionService.EmitActionEvent(event)
		} else {
			oldVals := make(map[string]any)
			newVals := make(map[string]any)
			for _, fc := range fieldChanges {
				fieldName := actionEventFieldName(fc.FieldName)
				oldVals[fieldName] = fc.OldValue
				newVals[fieldName] = fc.NewValue
			}
			event := &models.ActionEvent{
				EventType:   models.ActionTriggerItemUpdated,
				WorkspaceID: updated.WorkspaceID,
				ItemID:      updated.ID,
				ActorUserID: actorUserID,
				OldValues:   oldVals,
				NewValues:   newVals,
			}
			applyActionContext(event, actionContext)
			ec.actionService.EmitActionEvent(event)
		}
	}

	dispatchItemUpdateWebhooks(ec.webhookDispatcher, updated, statusChanged, assigneeChanged)
}

func actionEventFieldName(historyFieldName string) string {
	if strings.HasPrefix(historyFieldName, "cf_") {
		return "custom_field_" + strings.TrimPrefix(historyFieldName, "cf_")
	}
	return historyFieldName
}

func applyActionContext(event *models.ActionEvent, actionContext *ActionContext) {
	if actionContext == nil {
		return
	}
	event.TriggeredByAction = actionContext.TriggeredByAction
	event.ExecutionChainID = actionContext.ExecutionChainID
	event.CascadeDepth = actionContext.CascadeDepth
	event.SourceApplication = actionContext.SourceApplication
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
			TemplateData: map[string]any{
				"item.title":  item.Title,
				"item.id":     item.ID,
				"user.name":   actorName,
				"descendants": descendantCount,
			},
		})
	}

	// Dispatch webhook event
	if ec.webhookDispatcher != nil {
		ec.webhookDispatcher.DispatchEvent("item.deleted", item)
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
			TemplateData: map[string]any{
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
			OldValues: map[string]any{
				"status_id": oldStatusID,
			},
			NewValues: map[string]any{
				"status_id":   newStatusID,
				"title":       item.Title,
				"assignee_id": item.AssigneeID,
				"creator_id":  item.CreatorID,
			},
		})
	}

	// Dispatch webhook event
	if ec.webhookDispatcher != nil {
		ec.webhookDispatcher.DispatchEvent("status.changed", item)
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

// Approval events use rule-based broadcasts for dynamic audiences and direct
// delivery for resolved approver pools. Emit calls also dispatch webhooks.

// EmitApprovalRequested notifies assignees, creators, and watchers by rule.
func (ec *EventCoordinator) EmitApprovalRequested(req *models.ApprovalRequest, item *models.Item, actorUserID int, actorUsername ...string) {
	if req == nil || item == nil {
		return
	}
	actorName := resolveActorName(actorUserID, actorUsername)
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	if ec.notificationService != nil {
		ec.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventApprovalRequested,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: actorUserID,
			ItemID:      item.ID,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       "Approval Requested",
			TemplateData: map[string]any{
				"item.title":         item.Title,
				"item.key":           itemKey,
				"item.id":            item.ID,
				"approval.id":        req.ID,
				"approval.status_id": req.StatusID,
				"user.name":          actorName,
				"workspace.key":      item.WorkspaceKey,
			},
		})
	}
	if ec.webhookDispatcher != nil {
		ec.webhookDispatcher.DispatchEvent("approval.requested", item)
	}
}

// EmitApprovalStepStarted notifies the resolved approver pool that a step is
// open for their decision. Uses NotifyUsers (direct) so approvers in the
// snapshot are always reached, regardless of workspace notification rules.
// Portal-customer approvers are routed separately to a magic-link email.
func (ec *EventCoordinator) EmitApprovalStepStarted(req *models.ApprovalRequest, step *models.ApprovalStepInstance, approverUserIDs, approverPortalCustomerIDs []int, item *models.Item, actorUserID int, actorUsername ...string) {
	if req == nil || step == nil || item == nil || (len(approverUserIDs) == 0 && len(approverPortalCustomerIDs) == 0) {
		return
	}
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
	title := "Approval Required"
	message := fmt.Sprintf("Your approval is required on %s: %s", itemKey, item.Title)

	if ec.notificationService != nil && len(approverUserIDs) > 0 {
		_ = ec.notificationService.NotifyUsers(approverUserIDs, item.WorkspaceID, item.ID, actorUserID,
			models.EventApprovalStepStarted, title, message)
		// Also fire a rule-based broadcast for watchers/dashboards.
		ec.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventApprovalStepStarted,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: actorUserID,
			ItemID:      item.ID,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       title,
			TemplateData: map[string]any{
				"item.title":              item.Title,
				"item.key":                itemKey,
				"approval.id":             req.ID,
				"approval.step_id":        step.ApprovalStepID,
				"approval.step_instance":  step.ID,
				"approval.approver_count": len(approverUserIDs) + len(approverPortalCustomerIDs),
			},
		})
	}
	if len(approverPortalCustomerIDs) > 0 {
		ec.notifyPortalCustomersOfApprovalStep(req, approverPortalCustomerIDs, item, itemKey)
	}
	if ec.webhookDispatcher != nil {
		ec.webhookDispatcher.DispatchEvent("approval.step_started", item)
	}
}

// notifyPortalCustomersOfApprovalStep emails active portal approvers; missing portal slugs are logged and skipped.
func (ec *EventCoordinator) notifyPortalCustomersOfApprovalStep(req *models.ApprovalRequest, customerIDs []int, item *models.Item, itemKey string) {
	if ec.magicLinkService == nil || len(customerIDs) == 0 {
		return
	}
	portalSlug := ec.lookupPortalSlugForItem(item)
	if portalSlug == "" {
		slog.Warn("approval step opened for portal customer but no portal slug found for item",
			slog.String("component", "event_coordinator"),
			slog.Int("item_id", item.ID),
			slog.Int("approval_request_id", req.ID))
		return
	}
	for _, cid := range customerIDs {
		var email, name string
		err := ec.db.QueryRow(`SELECT email, COALESCE(name, '') FROM portal_customers WHERE id = ?`, cid).Scan(&email, &name)
		if err != nil || email == "" {
			slog.Warn("could not load portal customer for approval email",
				slog.String("component", "event_coordinator"),
				slog.Int("portal_customer_id", cid),
				slog.Any("error", err))
			continue
		}
		token, err := ec.magicLinkService.GenerateApprovalMagicLink(cid, item.ChannelID)
		if err != nil {
			slog.Error("failed to generate magic link for approval email",
				slog.String("component", "event_coordinator"),
				slog.Int("portal_customer_id", cid),
				slog.Any("error", err))
			continue
		}
		if err := ec.magicLinkService.SendApprovalRequestEmail(email, name, token, portalSlug, req.ID, itemKey, item.Title); err != nil {
			slog.Error("failed to send approval magic-link email",
				slog.String("component", "event_coordinator"),
				slog.Int("portal_customer_id", cid),
				slog.Any("error", err))
		}
	}
}

// lookupPortalSlugForItem reads channels.config -> portal_slug for the item's
// channel. The JSON is parsed in Go (rather than via SQL JSON functions) so
// the lookup works on both SQLite and Postgres. Returns empty string if the
// item has no channel or the channel has no portal_slug configured.
func (ec *EventCoordinator) lookupPortalSlugForItem(item *models.Item) string {
	if item.ChannelID == nil {
		return ""
	}
	var rawConfig sql.NullString
	if err := ec.db.QueryRow(`SELECT COALESCE(config, '{}') FROM channels WHERE id = ?`, *item.ChannelID).Scan(&rawConfig); err != nil {
		return ""
	}
	if !rawConfig.Valid || rawConfig.String == "" {
		return ""
	}
	var cfg struct {
		PortalSlug string `json:"portal_slug"`
	}
	if err := json.Unmarshal([]byte(rawConfig.String), &cfg); err != nil {
		return ""
	}
	return cfg.PortalSlug
}

// EmitApprovalDecided fires when an approver records a decision (approve, reject,
// or comment). Broadcast to requestor + watchers.
func (ec *EventCoordinator) EmitApprovalDecided(req *models.ApprovalRequest, decision *models.ApprovalDecision, item *models.Item, actorUsername ...string) {
	if req == nil || decision == nil || item == nil {
		return
	}
	actorID := 0
	if decision.ActorUserID != nil {
		actorID = *decision.ActorUserID
	}
	actorName := resolveActorName(actorID, actorUsername)
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	if ec.notificationService != nil {
		ec.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventApprovalDecided,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: actorID,
			ItemID:      item.ID,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       fmt.Sprintf("Approval %s", decision.Decision),
			TemplateData: map[string]any{
				"item.title":        item.Title,
				"item.key":          itemKey,
				"approval.id":       req.ID,
				"approval.decision": decision.Decision,
				"user.name":         actorName,
			},
		})
	}
	if ec.webhookDispatcher != nil {
		ec.webhookDispatcher.DispatchEvent("approval.decided", item)
	}
}

// EmitApprovalCompleted fires when an approval reaches its final outcome
// (approved or rejected). The corresponding status change still fires a
// regular EmitStatusChanged from CommitTransition.
func (ec *EventCoordinator) EmitApprovalCompleted(req *models.ApprovalRequest, item *models.Item, actorUserID int, actorUsername ...string) {
	if req == nil || item == nil {
		return
	}
	actorName := resolveActorName(actorUserID, actorUsername)
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	if ec.notificationService != nil {
		ec.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventApprovalCompleted,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: actorUserID,
			ItemID:      item.ID,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       fmt.Sprintf("Approval %s", req.Status),
			TemplateData: map[string]any{
				"item.title":      item.Title,
				"item.key":        itemKey,
				"approval.id":     req.ID,
				"approval.status": req.Status,
				"user.name":       actorName,
			},
		})
	}
	if ec.webhookDispatcher != nil {
		ec.webhookDispatcher.DispatchEvent("approval.completed", item)
	}
}

// EmitApprovalCancelled fires when a pending approval is canceled (left_status,
// manual, superseded, etc.).
func (ec *EventCoordinator) EmitApprovalCancelled(req *models.ApprovalRequest, item *models.Item, reason string, actorUserID int, actorUsername ...string) {
	if req == nil || item == nil {
		return
	}
	actorName := resolveActorName(actorUserID, actorUsername)
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	if ec.notificationService != nil {
		ec.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventApprovalCancelled,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: actorUserID,
			ItemID:      item.ID,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       "Approval Cancelled", //nolint:misspell // British spelling consistent with event_type value
			TemplateData: map[string]any{
				"item.title":      item.Title,
				"item.key":        itemKey,
				"approval.id":     req.ID,
				"approval.reason": reason,
				"user.name":       actorName,
			},
		})
	}
	if ec.webhookDispatcher != nil {
		ec.webhookDispatcher.DispatchEvent("approval.cancelled", item)
	}
}

// EmitApprovalEscalated fires when a step is escalated by the sweeper or a
// manual admin action. Notifies the new approver pool directly + broadcasts.
// Wired in slice 9 alongside the Escalate service method.
func (ec *EventCoordinator) EmitApprovalEscalated(req *models.ApprovalRequest, step *models.ApprovalStepInstance, action string, newApproverIDs []int, item *models.Item, actorUserID int, actorUsername ...string) {
	if req == nil || step == nil || item == nil {
		return
	}
	actorName := resolveActorName(actorUserID, actorUsername)
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

	if ec.notificationService != nil {
		if len(newApproverIDs) > 0 {
			_ = ec.notificationService.NotifyUsers(newApproverIDs, item.WorkspaceID, item.ID, actorUserID,
				models.EventApprovalEscalated,
				"Approval Escalated to You",
				fmt.Sprintf("An approval has been escalated to you on %s: %s", itemKey, item.Title))
		}
		ec.notificationService.EmitEvent(&NotificationEvent{
			EventType:   models.EventApprovalEscalated,
			WorkspaceID: item.WorkspaceID,
			ActorUserID: actorUserID,
			ItemID:      item.ID,
			AssigneeID:  item.AssigneeID,
			CreatorID:   item.CreatorID,
			Title:       "Approval Escalated",
			TemplateData: map[string]any{
				"item.title":       item.Title,
				"item.key":         itemKey,
				"approval.id":      req.ID,
				"approval.step_id": step.ApprovalStepID,
				"approval.action":  action,
				"user.name":        actorName,
			},
		})
	}
	if ec.webhookDispatcher != nil {
		ec.webhookDispatcher.DispatchEvent("approval.escalated", item)
	}
}
