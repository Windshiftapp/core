package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"windshift/internal/reporting"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// CustomWidgetDataHandler resolves the query slots of a widget. It is separate
// from CustomWidgetHandler because the reporting service needs the item
// application service, which is built after the CRUD routes are registered.
type CustomWidgetDataHandler struct {
	repo              *repository.CustomWidgetRepository
	permissionService *services.PermissionService
	data              *reporting.Service
}

func NewCustomWidgetDataHandler(
	repo *repository.CustomWidgetRepository,
	permissionService *services.PermissionService,
	data *reporting.Service,
) *CustomWidgetDataHandler {
	return &CustomWidgetDataHandler{repo: repo, permissionService: permissionService, data: data}
}

type customWidgetDataRequest struct {
	// WidgetID scopes the request to a saved widget: its workspace bounds item
	// queries and the caller must be able to read it.
	WidgetID *int `json:"widget_id"`
	// WorkspaceID is used for unsaved previews only. The item service still
	// intersects it with the viewer's accessible workspaces.
	WorkspaceID *int                        `json:"workspace_id"`
	Slots       []customWidgetDataSlotInput `json:"slots"`
}

type customWidgetDataSlotInput struct {
	Key            string                          `json:"key"`
	Entity         string                          `json:"entity"`
	QL             string                          `json:"ql"`
	Sort           string                          `json:"sort"`
	Limit          int                             `json:"limit"`
	AssetSetID     int                             `json:"asset_set_id"`
	LinkProjection *customWidgetDataLinkProjection `json:"link_projection"`
	Links          *customWidgetDataLinksInput     `json:"links"`
}

// customWidgetDataLinksInput is the bipartite edge request: two seed sets plus
// an edge cap.
type customWidgetDataLinksInput struct {
	Item     *customWidgetDataQueryPart `json:"item"`
	Asset    *customWidgetDataQueryPart `json:"asset"`
	MaxEdges int                        `json:"max_edges"`
}

type customWidgetDataQueryPart struct {
	QL         string `json:"ql"`
	Sort       string `json:"sort"`
	Limit      int    `json:"limit"`
	AssetSetID int    `json:"asset_set_id"`
}

type customWidgetDataLinkProjection struct {
	Entity string   `json:"entity"`
	Fields []string `json:"fields"`
	Mode   string   `json:"mode"`
}

// ResolveCustomWidgetData handles POST /api/custom-widgets/data.
func (h *CustomWidgetDataHandler) ResolveCustomWidgetData(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	request, ok := decodeJSON[customWidgetDataRequest](w, r)
	if !ok {
		return
	}
	if len(request.Slots) == 0 || len(request.Slots) > reporting.MaxSlots {
		respondValidationError(w, r, fmt.Sprintf("slots must contain between 1 and %d entries", reporting.MaxSlots))
		return
	}

	workspaceID := 0
	if request.WidgetID != nil {
		widget, err := h.repo.Get(*request.WidgetID)
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Custom widget")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		readable, err := canReadCustomWidget(h.permissionService, user.ID, widget)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !readable {
			respondNotFound(w, r, "Custom widget")
			return
		}
		if widget.WorkspaceID != nil {
			workspaceID = *widget.WorkspaceID
		}
	} else if request.WorkspaceID != nil && *request.WorkspaceID > 0 {
		workspaceID = *request.WorkspaceID
	}

	slots := make([]reporting.Slot, 0, len(request.Slots))
	seen := make(map[string]bool, len(request.Slots))
	for _, raw := range request.Slots {
		slot, err := reporting.NormalizeSlot(reporting.Slot{
			Key:        raw.Key,
			Entity:     reporting.Entity(raw.Entity),
			QL:         raw.QL,
			Sort:       raw.Sort,
			Limit:      raw.Limit,
			AssetSetID: raw.AssetSetID,
			Projection: toProjection(raw.LinkProjection),
			Links:      toLinksSpec(raw.Links),
		})
		if err != nil {
			respondValidationError(w, r, err.Error())
			return
		}
		if seen[slot.Key] {
			respondValidationError(w, r, fmt.Sprintf("duplicate slot key %q", slot.Key))
			return
		}
		seen[slot.Key] = true
		slots = append(slots, slot)
	}

	// Link projections are the only path that needs the workspace set, so it is
	// only fetched when a slot actually asks for one.
	request2 := reporting.Request{UserID: user.ID, WorkspaceID: workspaceID, Slots: slots}
	for _, slot := range slots {
		if slot.Projection != nil {
			accessible, err := h.permissionService.AccessibleWorkspaceIDs(user.ID)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			request2.AccessibleWorkspaceIDs = accessible
			break
		}
	}

	results := h.data.Resolve(r.Context(), request2)
	respondJSONOK(w, map[string]any{"results": results})
}

func toLinksSpec(raw *customWidgetDataLinksInput) *reporting.LinksSpec {
	if raw == nil {
		return nil
	}
	return &reporting.LinksSpec{
		Item:     toQueryPart(raw.Item),
		Asset:    toQueryPart(raw.Asset),
		MaxEdges: raw.MaxEdges,
	}
}

func toQueryPart(raw *customWidgetDataQueryPart) *reporting.QueryPart {
	if raw == nil {
		return nil
	}
	return &reporting.QueryPart{QL: raw.QL, Sort: raw.Sort, Limit: raw.Limit, AssetSetID: raw.AssetSetID}
}

func toProjection(raw *customWidgetDataLinkProjection) *reporting.Projection {
	if raw == nil {
		return nil
	}
	return &reporting.Projection{Entity: raw.Entity, Fields: raw.Fields, Mode: raw.Mode}
}
