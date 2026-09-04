package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/validation"
)

// ItemUpdatedEmitter receives the committed before/after state so user-facing
// transports trigger one notification, automation, and webhook pipeline.
type ItemUpdatedEmitter interface {
	EmitItemUpdated(original, updated *models.Item, statusChanged, assigneeChanged bool, actorUserID int, fieldChanges []HistoryEntry, actorUsername ...string)
}

type contextualItemUpdatedEmitter interface {
	EmitItemUpdatedWithContext(original, updated *models.Item, statusChanged, assigneeChanged bool, actorUserID int, fieldChanges []HistoryEntry, actionContext ActionContext, actorUsername ...string)
}

// ItemUpdateApplicationService owns the transport-neutral user-facing update
// pipeline. The lower-level ItemUpdateService remains usable by internal
// workflows that deliberately manage their own side effects.
type ItemUpdateApplicationService struct {
	update          *ItemUpdateService
	permission      *PermissionService
	activityTracker *ActivityTracker
	itemCache       *ItemCacheService
	hierarchy       *HierarchyService
	mentionService  *MentionService
	emitter         ItemUpdatedEmitter
	fallback        ItemUpdatedEmitter
}

func NewItemUpdateApplicationService(db database.Database, perm *PermissionService) *ItemUpdateApplicationService {
	return &ItemUpdateApplicationService{
		update:     NewItemUpdateService(db).WithPermissionService(perm),
		permission: perm,
		hierarchy:  NewHierarchyService(db),
	}
}

// SetPermissionService updates the permission checks used by item updates.
// This supports services that are constructed before the shared permission
// service is available during server bootstrap.
func (s *ItemUpdateApplicationService) SetPermissionService(perm *PermissionService) {
	s.permission = perm
	s.update.WithPermissionService(perm)
}

func (s *ItemUpdateApplicationService) SetActivityTracker(activityTracker *ActivityTracker) {
	s.activityTracker = activityTracker
}

func (s *ItemUpdateApplicationService) SetCache(itemCache *ItemCacheService, hierarchy *HierarchyService) {
	s.itemCache = itemCache
	if hierarchy != nil {
		s.hierarchy = hierarchy
	}
}

func (s *ItemUpdateApplicationService) SetMentionService(mentionService *MentionService) {
	s.mentionService = mentionService
}

func (s *ItemUpdateApplicationService) SetEmitter(emitter ItemUpdatedEmitter) {
	s.emitter = emitter
}

// SetFallbackEmitter installs the event pipeline used when no EventCoordinator
// is wired. Legacy transports configure it with the original per-service
// notifier so lightweight embeddings keep identical side effects.
func (s *ItemUpdateApplicationService) SetFallbackEmitter(emitter ItemUpdatedEmitter) {
	s.fallback = emitter
}

// SetFallbackWebhook forwards the webhook sender into the legacy fallback
// emitter when one is installed.
func (s *ItemUpdateApplicationService) SetFallbackWebhook(dispatcher WebhookDispatcher) {
	if legacy, ok := s.fallback.(*LegacyItemUpdatedEmitter); ok {
		legacy.SetWebhook(dispatcher)
	}
}

// SetFallbackAction forwards the automation action service into the legacy
// fallback emitter when one is installed.
func (s *ItemUpdateApplicationService) SetFallbackAction(action ActionEventEmitter) {
	if legacy, ok := s.fallback.(*LegacyItemUpdatedEmitter); ok {
		legacy.SetAction(action)
	}
}

// CanUserEditItem reports whether the actor may edit the item. It loads the
// item through the update pipeline's own repository access and propagates a
// not-found error so transports can keep their not-found denial contract.
func (s *ItemUpdateApplicationService) CanUserEditItem(userID, itemID int) (bool, error) {
	item, err := s.update.FindItem(itemID)
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}
	if s.permission == nil {
		return false, nil
	}
	return s.permission.HasWorkspacePermission(userID, item.WorkspaceID, models.PermissionItemEdit)
}

// UpdateJSONFields applies the public item-update patch format. Keeping the
// presence/null distinction here means every HTTP surface uses the same
// coercion rules before reaching the transactional update service.
func (s *ItemUpdateApplicationService) UpdateJSONFields(actorUserID int, actorUsername string, itemID int, fields map[string]json.RawMessage) (*UpdateItemResult, error) {
	if _, ok := fields["status_id"]; ok {
		return nil, &validation.ValidationError{
			Field:   "status_id",
			Message: "status_id may not be set via item update; use POST /rest/api/v2/items/{id}/transition",
		}
	}

	updateData, err := itemUpdateData(fields)
	if err != nil {
		return nil, err
	}
	return s.Update(actorUserID, actorUsername, itemID, updateData)
}

func itemUpdateData(fields map[string]json.RawMessage) (map[string]any, error) {
	updateData := make(map[string]any)
	if raw, ok := fields["title"]; ok && string(raw) != "null" {
		var value string
		if err := decodeItemUpdateField(raw, "title", &value); err != nil {
			return nil, err
		}
		value, err := validation.NormalizeTitle(value)
		if err != nil {
			return nil, err
		}
		updateData["title"] = value
	}
	if raw, ok := fields["description"]; ok && string(raw) != "null" {
		var value string
		if err := decodeItemUpdateField(raw, "description", &value); err != nil {
			return nil, err
		}
		if err := validation.ValidateMarkdownSource("description", value, validation.MarkdownMaxBytes, false); err != nil {
			return nil, err
		}
		updateData["description"] = value
	}
	for _, field := range []string{"priority_id", "assignee_id", "parent_id", "iteration_id", "project_id"} {
		if raw, ok := fields[field]; ok {
			value, err := decodeNullableItemUpdateInt(raw, field)
			if err != nil {
				return nil, err
			}
			updateData[field] = value
		}
	}
	if raw, ok := fields["item_type_id"]; ok && string(raw) != "null" {
		var value int
		if err := decodeItemUpdateField(raw, "item_type_id", &value); err != nil {
			return nil, err
		}
		updateData["item_type_id"] = value
	}
	if raw, ok := fields["milestone_ids"]; ok && string(raw) != "null" {
		var value []int
		if err := decodeItemUpdateField(raw, "milestone_ids", &value); err != nil {
			return nil, err
		}
		updateData["milestone_ids"] = value
	}
	for _, field := range []string{"due_date", "start_date", "end_date"} {
		if raw, ok := fields[field]; ok {
			value, err := decodeNullableItemUpdateTime(raw, field)
			if err != nil {
				return nil, err
			}
			updateData[field] = value
		}
	}
	if raw, ok := fields["is_task"]; ok {
		if string(raw) == "null" {
			return nil, &validation.ValidationError{Field: "is_task", Message: "is_task cannot be null"}
		}
		var value bool
		if err := decodeItemUpdateField(raw, "is_task", &value); err != nil {
			return nil, err
		}
		updateData["is_task"] = value
	}
	if raw, ok := fields["custom_fields"]; ok && string(raw) != "null" {
		var value map[string]any
		if err := decodeItemUpdateField(raw, "custom_fields", &value); err != nil {
			return nil, err
		}
		updateData["custom_field_values"] = value
	}
	return updateData, nil
}

func decodeItemUpdateField(raw json.RawMessage, field string, target any) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return &validation.ValidationError{Field: field, Message: fmt.Sprintf("invalid %s", field)}
	}
	return nil
}

func decodeNullableItemUpdateInt(raw json.RawMessage, field string) (any, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var value int
	if err := decodeItemUpdateField(raw, field, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func decodeNullableItemUpdateTime(raw json.RawMessage, field string) (any, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var value time.Time
	if err := decodeItemUpdateField(raw, field, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (s *ItemUpdateApplicationService) Update(actorUserID int, actorUsername string, itemID int, updateData map[string]any) (*UpdateItemResult, error) {
	return s.updateItem(actorUserID, actorUsername, itemID, updateData, nil)
}

// UpdateWithContext runs the standard update pipeline while preserving the
// automation chain that caused the mutation.
func (s *ItemUpdateApplicationService) UpdateWithContext(
	actorUserID int,
	actorUsername string,
	itemID int,
	updateData map[string]any,
	actionContext ActionContext,
) (*UpdateItemResult, error) {
	return s.updateItem(actorUserID, actorUsername, itemID, updateData, &actionContext)
}

func (s *ItemUpdateApplicationService) updateItem(
	actorUserID int,
	actorUsername string,
	itemID int,
	updateData map[string]any,
	actionContext *ActionContext,
) (*UpdateItemResult, error) {
	if s.activityTracker != nil {
		if err := s.activityTracker.TrackItemActivity(actorUserID, itemID, ActivityEdit); err != nil {
			slog.Warn("failed to track item edit activity", slog.Int("user_id", actorUserID), slog.Int("item_id", itemID), slog.Any("error", err))
		}
	}

	result, err := s.update.UpdateItem(UpdateItemRequest{
		ItemID:        itemID,
		UpdateData:    updateData,
		UserID:        actorUserID,
		EventMetadata: itemEventMetadata(actorUserID, "application", actionContext),
	})
	if err != nil {
		return nil, err
	}

	if s.itemCache != nil && itemProjectResolutionChanged(result.OriginalItem, result.Item) {
		s.invalidateEffectiveProjectSubtree(result.Item.ID)
	}

	assigneeChanged := !itemIntPtrEqual(result.OriginalItem.AssigneeID, result.Item.AssigneeID)
	if contextual, ok := s.emitter.(contextualItemUpdatedEmitter); ok && actionContext != nil {
		contextual.EmitItemUpdatedWithContext(
			result.OriginalItem,
			result.Item,
			result.StatusChanged,
			assigneeChanged,
			actorUserID,
			result.FieldChanges,
			*actionContext,
			actorUsername,
		)
	} else if s.emitter != nil {
		s.emitter.EmitItemUpdated(
			result.OriginalItem,
			result.Item,
			result.StatusChanged,
			assigneeChanged,
			actorUserID,
			result.FieldChanges,
			actorUsername,
		)
	} else if s.fallback != nil {
		s.fallback.EmitItemUpdated(
			result.OriginalItem,
			result.Item,
			result.StatusChanged,
			assigneeChanged,
			actorUserID,
			result.FieldChanges,
			actorUsername,
		)
	}

	if s.mentionService != nil && result.OriginalItem.Description != result.Item.Description {
		if err := s.mentionService.ProcessMentions(ProcessMentionsParams{
			SourceType:  "item_description",
			SourceID:    result.Item.ID,
			Content:     result.Item.Description,
			ItemID:      result.Item.ID,
			WorkspaceID: result.Item.WorkspaceID,
			ActorUserID: actorUserID,
		}); err != nil {
			slog.Warn("failed to process description mentions", slog.Int("item_id", result.Item.ID), slog.Any("error", err))
		}
	}

	return result, nil
}

// AddMilestoneWithContext atomically adds one milestone without replacing
// concurrent attachments. Duplicate deliveries are no-ops: they produce no
// history, live refresh, automation event, notification, or webhook.
func (s *ItemUpdateApplicationService) AddMilestoneWithContext(
	actorUserID int,
	actorUsername string,
	itemID int,
	milestoneID int,
	actionContext ActionContext,
) (*UpdateItemResult, bool, error) {
	result, changed, err := s.update.AddMilestone(UpdateItemRequest{
		ItemID: itemID,
		UserID: actorUserID,
	}, milestoneID)
	if err != nil || !changed {
		return result, changed, err
	}
	if contextual, ok := s.emitter.(contextualItemUpdatedEmitter); ok {
		contextual.EmitItemUpdatedWithContext(
			result.OriginalItem,
			result.Item,
			false,
			false,
			actorUserID,
			result.FieldChanges,
			actionContext,
			actorUsername,
		)
	} else if s.emitter != nil {
		s.emitter.EmitItemUpdated(
			result.OriginalItem,
			result.Item,
			false,
			false,
			actorUserID,
			result.FieldChanges,
			actorUsername,
		)
	}
	return result, true, nil
}

func itemProjectResolutionChanged(original, updated *models.Item) bool {
	return original.InheritProject != updated.InheritProject ||
		!itemIntPtrEqual(original.ProjectID, updated.ProjectID) ||
		!itemIntPtrEqual(original.ParentID, updated.ParentID)
}

func itemIntPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *ItemUpdateApplicationService) invalidateEffectiveProjectSubtree(itemID int) {
	_ = s.itemCache.InvalidateItemHierarchy(itemID, nil)
	if s.hierarchy == nil {
		return
	}
	descendants, err := s.hierarchy.GetDescendants(itemID, 0)
	if err != nil {
		slog.Warn("failed to load descendants for cache invalidation", slog.Int("item_id", itemID), slog.Any("error", err))
		return
	}
	for i := range descendants {
		_ = s.itemCache.InvalidateItemHierarchy(descendants[i].ID, nil)
	}
}
