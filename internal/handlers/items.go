package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/authz"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/services"
	"windshift/internal/webhook"
)

type ItemHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	authz             *authz.Authz
	itemCache         *services.ItemCacheService
	activityTracker   *services.ActivityTracker
	itemCRUD          *services.ItemCRUDService
	itemCreation      *services.ItemCreationService
	itemUpdate        *services.ItemUpdateApplicationService
	itemDeletion      *services.ItemDeletionApplicationService
	mentionService    *services.MentionService
	webhookSender     *webhook.WebhookSender
	eventCoordinator  *services.EventCoordinator
	sseHub            *services.SSEHub
}

func NewItemHandler(db database.Database, permissionService *services.PermissionService, activityTracker *services.ActivityTracker, notificationService interface {
	EmitEvent(event *services.NotificationEvent)
}, cacheSizeMB ...int) *ItemHandler {
	cacheConfig := services.DefaultItemCacheConfig()
	if len(cacheSizeMB) > 0 && cacheSizeMB[0] > 0 {
		cacheConfig.MaxCacheSize = cacheSizeMB[0]
	}
	itemCache, err := services.NewItemCacheService(db, cacheConfig)
	if err != nil {
		slog.Warn("failed to initialize item cache, continuing without cache", "error", err)
	}
	hierarchy := services.NewHierarchyService(db)
	itemUpdate := services.NewItemUpdateApplicationService(db, permissionService)
	itemUpdate.SetActivityTracker(activityTracker)
	itemUpdate.SetCache(itemCache, hierarchy)
	var notify func(*services.NotificationEvent)
	if notificationService != nil {
		notify = notificationService.EmitEvent
	}
	itemUpdate.SetFallbackEmitter(services.NewLegacyItemUpdatedEmitter(db, notify, nil, nil))
	itemDeletion := services.NewItemDeletionApplicationService(db, permissionService)
	itemDeletion.SetCache(itemCache, hierarchy)
	return &ItemHandler{
		db: db, permissionService: permissionService, authz: authz.New(db, permissionService),
		itemCache: itemCache, activityTracker: activityTracker,
		itemCRUD: services.NewItemCRUDService(db), itemCreation: services.NewItemCreationService(db, permissionService),
		itemUpdate: itemUpdate, itemDeletion: itemDeletion,
	}
}

func (h *ItemHandler) SetWebhookSender(sender *webhook.WebhookSender) {
	h.webhookSender = sender
	h.itemUpdate.SetFallbackWebhook(sender)
}

func (h *ItemHandler) SetMentionService(mentionService *services.MentionService) {
	h.mentionService = mentionService
	h.itemUpdate.SetMentionService(mentionService)
}

func (h *ItemHandler) SetActionService(actionService interface {
	EmitActionEvent(event *models.ActionEvent)
}) {
	h.itemUpdate.SetFallbackAction(actionService)
}

func (h *ItemHandler) SetEventCoordinator(coordinator *services.EventCoordinator) {
	h.eventCoordinator = coordinator
	h.itemCreation.SetEmitter(coordinator)
	h.itemUpdate.SetEmitter(coordinator)
	h.itemDeletion.SetEmitter(coordinator)
}

func (h *ItemHandler) ItemCreationService() *services.ItemCreationService { return h.itemCreation }

func (h *ItemHandler) ItemUpdateApplicationService() *services.ItemUpdateApplicationService {
	return h.itemUpdate
}

func (h *ItemHandler) ItemDeletionApplicationService() *services.ItemDeletionApplicationService {
	return h.itemDeletion
}

func (h *ItemHandler) ItemCacheService() *services.ItemCacheService { return h.itemCache }

func (h *ItemHandler) GetCacheStats(w http.ResponseWriter, r *http.Request) {
	if h.itemCache == nil {
		respondError(w, r, &restapi.APIError{StatusCode: http.StatusServiceUnavailable, Code: "SERVICE_UNAVAILABLE", Message: "Item cache is not enabled"})
		return
	}
	respondJSONOK(w, map[string]any{
		"cache_enabled": true,
		"statistics":    h.itemCache.GetStats(),
		"timestamp":     time.Now().Format(time.RFC3339),
	})
}
