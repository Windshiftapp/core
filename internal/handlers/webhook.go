package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/webhook"
)

// WebhookHandler handles HTTP requests for webhook operations
type WebhookHandler struct {
	channelRepo       *repository.ChannelRepository
	itemRepo          *repository.ItemRepository
	webhookSender     *webhook.WebhookSender
	permissionService *services.PermissionService
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(
	channelRepo *repository.ChannelRepository,
	itemRepo *repository.ItemRepository,
	webhookSender *webhook.WebhookSender,
	permissionService *services.PermissionService,
) *WebhookHandler {
	return &WebhookHandler{
		channelRepo:       channelRepo,
		itemRepo:          itemRepo,
		webhookSender:     webhookSender,
		permissionService: permissionService,
	}
}

// TriggerWebhook manually triggers a webhook for a specific item
// POST /api/webhooks/{webhookId}/trigger
// Body: { "item_id": 123 }
func (h *WebhookHandler) TriggerWebhook(w http.ResponseWriter, r *http.Request) {
	// Get current user for permission check
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	webhookID, ok := requireIDParam(w, r, "webhookId")
	if !ok {
		return
	}

	var request struct {
		ItemID int `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	if request.ItemID == 0 {
		respondValidationError(w, r, "item_id is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify webhook exists and is active
	channel, err := h.channelRepo.FindByID(ctx, webhookID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "webhook")
		return
	}
	if err != nil {
		respondNotFound(w, r, "webhook")
		return
	}

	if channel.Type != "webhook" {
		respondBadRequest(w, r, "Channel is not a webhook")
		return
	}

	// Get item workspace for permission check
	itemWorkspaceID, err := h.itemRepo.GetWorkspaceIDCtx(ctx, request.ItemID)
	if err != nil {
		respondNotFound(w, r, "item")
		return
	}

	// Check user has permission to the item's workspace
	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, itemWorkspaceID, models.PermissionItemView)
	if err != nil || !hasPermission {
		respondNotFound(w, r, "item")
		return
	}

	// Trigger the webhook
	if err := h.webhookSender.TriggerManually(ctx, webhookID, request.ItemID); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Webhook triggered successfully",
	})
}

// GetWebhooksForItem returns all webhooks that can be triggered for a specific item
// GET /api/items/{id}/webhooks
func (h *WebhookHandler) GetWebhooksForItem(w http.ResponseWriter, r *http.Request) {
	// Get current user for permission check
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get item workspace for permission check
	itemWorkspaceID, err := h.itemRepo.GetWorkspaceIDCtx(ctx, itemID)
	if err != nil {
		respondNotFound(w, r, "item")
		return
	}

	// Check user has permission to the item's workspace
	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, itemWorkspaceID, models.PermissionItemView)
	if err != nil || !hasPermission {
		respondNotFound(w, r, "item")
		return
	}

	// Get all enabled outbound webhook channels
	channels, err := h.channelRepo.ListEnabledByTypeAndDirection(ctx, "webhook", "outbound")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	type WebhookInfo struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		ScopeType   string `json:"scope_type"`
		AutoTrigger bool   `json:"auto_trigger"`
		CanTrigger  bool   `json:"can_trigger"`
	}

	webhooks := make([]WebhookInfo, 0, len(channels))
	for _, c := range channels {
		var config models.ChannelConfig
		if err := json.Unmarshal([]byte(c.Config), &config); err != nil {
			continue
		}

		// Check if webhook can be triggered for this item (scope matching)
		canTrigger := false
		switch config.WebhookScopeType {
		case "all", "":
			canTrigger = true
		case "workspaces":
			for _, wsID := range config.WebhookWorkspaceIDs {
				if wsID == itemWorkspaceID {
					canTrigger = true
					break
				}
			}
		case "collections":
			// For collections, we need more complex checking
			// For now, allow manual trigger if scope is collections
			canTrigger = true
		}

		webhooks = append(webhooks, WebhookInfo{
			ID:          c.ID,
			Name:        c.Name,
			ScopeType:   config.WebhookScopeType,
			AutoTrigger: config.WebhookAutoTrigger,
			CanTrigger:  canTrigger,
		})
	}

	respondJSONOK(w, webhooks)
}
