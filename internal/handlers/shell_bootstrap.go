package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/services"
)

type channelManagementCapability interface {
	ManagesChannels(context.Context, int) (bool, error)
}

type assetSetCapability interface {
	HasAccessibleAssetSets(int) (bool, error)
}

// ShellBootstrapHandler composes the capability snapshots needed by the
// authenticated application shell. Keeping this as one request prevents every
// optional feature from competing for a per-user concurrency slot at login.
type ShellBootstrapHandler struct {
	features    *FeaturesHandler
	setup       *SetupHandler
	attachments *AttachmentSettingsHandler
	ai          *AIHandler
	assets      assetSetCapability
	hub         *HubHandler
	channels    channelManagementCapability
	staleness   *WorkItemStalenessHandler
}

type ShellBootstrapResponse struct {
	Features          FeaturesResponse                   `json:"features"`
	ModuleSettings    *models.ModuleSettings             `json:"module_settings,omitempty"`
	AttachmentStatus  *services.AttachmentStatus         `json:"attachment_status"`
	AI                AIStatusResponse                   `json:"ai"`
	HasAssetSets      bool                               `json:"has_asset_sets"`
	HasActivePortals  bool                               `json:"has_active_portals"`
	ManagesChannels   bool                               `json:"manages_channels"`
	WorkItemStaleness services.WorkItemStalenessSettings `json:"work_item_staleness"`
}

func NewShellBootstrapHandler(
	features *FeaturesHandler,
	setup *SetupHandler,
	attachments *AttachmentSettingsHandler,
	ai *AIHandler,
	assets assetSetCapability,
	hub *HubHandler,
	channels channelManagementCapability,
	staleness *WorkItemStalenessHandler,
) *ShellBootstrapHandler {
	return &ShellBootstrapHandler{
		features: features, setup: setup, attachments: attachments,
		ai: ai, assets: assets, hub: hub, channels: channels, staleness: staleness,
	}
}

func (h *ShellBootstrapHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	response := ShellBootstrapResponse{
		AttachmentStatus:  &services.AttachmentStatus{},
		WorkItemStaleness: services.DefaultWorkItemStalenessSettings(),
	}
	if h.features != nil {
		response.Features = h.features.Snapshot()
	}
	if h.setup != nil {
		settings, err := h.setup.ModuleSettings()
		if err != nil {
			slog.Warn("shell bootstrap: module settings unavailable", "error", err)
		} else {
			response.ModuleSettings = &settings
		}
	}
	if h.attachments != nil {
		status, err := h.attachments.Status()
		if err != nil {
			slog.Warn("shell bootstrap: attachment status unavailable", "error", err)
		} else {
			response.AttachmentStatus = status
		}
	}
	if h.ai != nil {
		response.AI = h.ai.StatusSnapshot()
	}
	if h.assets != nil {
		hasSets, err := h.assets.HasAccessibleAssetSets(user.ID)
		if err != nil {
			slog.Warn("shell bootstrap: asset availability unavailable", "user_id", user.ID, "error", err)
		} else {
			response.HasAssetSets = hasSets
		}
	}
	if h.hub != nil {
		hasPortals, err := h.hub.HasActivePortals(r.Context())
		if err != nil {
			slog.Warn("shell bootstrap: portal availability unavailable", "user_id", user.ID, "error", err)
		} else {
			response.HasActivePortals = hasPortals
		}
	}
	if h.channels != nil {
		managesChannels, err := h.channels.ManagesChannels(r.Context(), user.ID)
		if err != nil {
			slog.Warn("shell bootstrap: channel management availability unavailable", "user_id", user.ID, "error", err)
		} else {
			response.ManagesChannels = managesChannels
		}
	}
	if h.staleness != nil {
		settings, err := h.staleness.Settings()
		if err != nil {
			slog.Warn("shell bootstrap: work item staleness settings unavailable", "error", err)
		} else {
			response.WorkItemStaleness = settings
		}
	}

	respondJSONOK(w, response)
}
