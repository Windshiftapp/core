package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/webhook"
)

// WebhookHandler handles HTTP requests for webhook operations
type WebhookHandler struct {
	db                database.Database
	webhookSender     *webhook.WebhookSender
	permissionService *services.PermissionService
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(db database.Database, webhookSender *webhook.WebhookSender, permissionService *services.PermissionService) *WebhookHandler {
	return &WebhookHandler{
		db:                db,
		webhookSender:     webhookSender,
		permissionService: permissionService,
	}
}

// TriggerWebhook manually triggers a webhook for a specific item
// POST /api/webhooks/{webhookId}/trigger
// Body: { "item_id": 123 }
func (h *WebhookHandler) TriggerWebhook(w http.ResponseWriter, r *http.Request) {
	// Get current user for permission check
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	webhookID, err := strconv.Atoi(r.PathValue("webhookId"))
	if err != nil {
		http.Error(w, "Invalid webhook ID", http.StatusBadRequest)
		return
	}

	var request struct {
		ItemID int `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if request.ItemID == 0 {
		http.Error(w, "item_id is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify webhook exists and is active
	var status string
	var channelType string
	checkQuery := "SELECT type, status FROM channels WHERE id = ?"
	err = h.db.QueryRowContext(ctx, checkQuery, webhookID).Scan(&channelType, &status)
	if err != nil {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}

	if channelType != "webhook" {
		http.Error(w, "Channel is not a webhook", http.StatusBadRequest)
		return
	}

	// Get item workspace for permission check
	var itemWorkspaceID int
	itemQuery := "SELECT workspace_id FROM items WHERE id = ?"
	err = h.db.QueryRowContext(ctx, itemQuery, request.ItemID).Scan(&itemWorkspaceID)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	// Check user has permission to the item's workspace
	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, itemWorkspaceID, "read")
	if err != nil || !hasPermission {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	// Trigger the webhook
	err = h.webhookSender.TriggerManually(ctx, webhookID, request.ItemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Webhook triggered successfully",
	})
}

// GetWebhooksForItem returns all webhooks that can be triggered for a specific item
// GET /api/items/{id}/webhooks
func (h *WebhookHandler) GetWebhooksForItem(w http.ResponseWriter, r *http.Request) {
	// Get current user for permission check
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get item workspace for permission check
	var itemWorkspaceID int
	itemQuery := "SELECT workspace_id FROM items WHERE id = ?"
	err = h.db.QueryRowContext(ctx, itemQuery, itemID).Scan(&itemWorkspaceID)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	// Check user has permission to the item's workspace
	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, itemWorkspaceID, "read")
	if err != nil || !hasPermission {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	// Get all active webhooks
	query := `
		SELECT id, name, config
		FROM channels
		WHERE type = 'webhook' AND direction = 'outbound' AND status = 'enabled'
	`

	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		http.Error(w, "Failed to query webhooks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type WebhookInfo struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		ScopeType   string `json:"scope_type"`
		AutoTrigger bool   `json:"auto_trigger"`
		CanTrigger  bool   `json:"can_trigger"`
	}

	var webhooks []WebhookInfo
	for rows.Next() {
		var id int
		var name string
		var configJSON string

		if err := rows.Scan(&id, &name, &configJSON); err != nil {
			continue
		}

		var config models.ChannelConfig
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
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
			ID:          id,
			Name:        name,
			ScopeType:   config.WebhookScopeType,
			AutoTrigger: config.WebhookAutoTrigger,
			CanTrigger:  canTrigger,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhooks)
}
