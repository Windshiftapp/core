package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// muxScope is one scope token a multiplexed frame is emitted for: a requested
// workspace or collection whose backing workspace changed.
type muxScope struct {
	kind string // "workspace" or "collection"
	id   int
}

// UserEvents multiplexes every live-update topic one browser tab needs onto a
// single SSE connection (WI-1656): the caller's notification invalidations,
// workspace/collection invalidations, and the open item's changes. Query params
// select the scopes (`items`, `workspaces`, `collections`); the client replaces
// the connection as it navigates instead of opening one stream per topic.
//
// Wire format: one named event per topic family, each carrying its scope id:
//
//	notification.changed {}
//	workspace.changed    {"workspace_id":3}
//	collection.changed   {"collection_id":5}
//	item.changed         {"item_id":2895}
//	item.deleted         {"item_id":2895}
//	resync               {}
//
// `resync` means a subscriber buffer overflowed and the client must reconcile
// every scope it holds. Every requested scope is permission-checked at connect
// time and re-checked on each frame, matching the per-topic endpoints.
func (h *ItemHandler) UserEvents(w http.ResponseWriter, r *http.Request) {
	if h.sseHub == nil {
		respondServiceUnavailable(w, r, "live updates are not enabled on this server")
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	itemIDs, ok := parseScopeIDList(w, r, r.URL.Query().Get("items"), "items")
	if !ok {
		return
	}
	workspaceIDs, ok := parseScopeIDList(w, r, r.URL.Query().Get("workspaces"), "workspaces")
	if !ok {
		return
	}
	collectionIDs, ok := parseScopeIDList(w, r, r.URL.Query().Get("collections"), "collections")
	if !ok {
		return
	}

	// Resolve every requested scope to the workspace topics that back it.
	// workspaceScopes maps a backing workspace to the scope tokens a frame must
	// be emitted for, so a workspace event becomes a `workspace.changed` for the
	// workspace scope and a `collection.changed` for each collection scope that
	// covers it.
	workspaceScopes := map[int][]muxScope{}
	backingWorkspaces := map[int]bool{}
	authorized := func(workspaceID int) bool {
		allowed, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemView)
		return err == nil && allowed
	}

	for _, id := range workspaceIDs {
		if !authorized(id) {
			respondNotFound(w, r, "Workspace")
			return
		}
		backingWorkspaces[id] = true
		workspaceScopes[id] = append(workspaceScopes[id], muxScope{kind: "workspace", id: id})
	}

	collectionRepo := repository.NewCollectionRepository(h.db)
	for _, id := range collectionIDs {
		collection, err := collectionRepo.GetByID(id)
		if err != nil {
			respondNotFound(w, r, "Collection")
			return
		}
		var covered []int
		if collection.WorkspaceID != nil {
			if !authorized(*collection.WorkspaceID) {
				respondNotFound(w, r, "Collection")
				return
			}
			covered = []int{*collection.WorkspaceID}
		} else {
			accessible, err := GetAccessibleWorkspaceIDs(user, h.db, h.permissionService)
			if err != nil {
				respondServiceUnavailable(w, r, "could not resolve workspace access")
				return
			}
			covered = accessible
		}
		for _, wid := range covered {
			backingWorkspaces[wid] = true
			workspaceScopes[wid] = append(workspaceScopes[wid], muxScope{kind: "collection", id: id})
		}
	}

	itemRepo := repository.NewItemRepository(h.db)
	for _, id := range itemIDs {
		workspaceID, err := itemRepo.GetWorkspaceID(id)
		if err != nil || !authorized(workspaceID) {
			respondNotFound(w, r, "Item")
			return
		}
	}

	release, ok := h.sseHub.AcquireUserStream(user.ID)
	if !ok {
		respondTooManyRequests(w, r, "Too many live update streams are already open for this account")
		return
	}
	defer release()

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondServiceUnavailable(w, r, "streaming is unsupported on this connection")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	unbindStreamDeadlines(w)

	backing := make([]int, 0, len(backingWorkspaces))
	for id := range backingWorkspaces {
		backing = append(backing, id)
	}
	itemSub := h.sseHub.SubscribeItems(itemIDs)
	defer h.sseHub.UnsubscribeItems(itemSub)
	wsSub := h.sseHub.SubscribeWorkspaces(backing)
	defer h.sseHub.UnsubscribeWorkspaces(wsSub)
	// The notification topic is the caller's own user id, so it needs no extra
	// permission check beyond authentication.
	userSub := h.sseHub.SubscribeUser(user.ID)
	defer h.sseHub.UnsubscribeUser(userSub)

	_, _ = fmt.Fprintf(w, "retry: %d\n\n", sseRetryMillis(user.ID)) //nolint:gosec // G705: SSE control line, numeric only; response is text/event-stream, not HTML
	writeMultiplexSSE(w, "connected", "{}")
	flusher.Flush()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()
	ctx := r.Context()

	for {
		select {
		case ev := <-itemSub.Events():
			if ev.Deleted {
				if !authorized(ev.WorkspaceID) {
					continue
				}
				writeMultiplexSSE(w, "item.deleted", fmt.Sprintf(`{"item_id":%d}`, ev.ItemID))
				flusher.Flush()
				continue
			}
			// Re-resolve the workspace: a move can carry the item beyond this
			// user's reach.
			current, err := itemRepo.GetWorkspaceID(ev.ItemID)
			if err != nil || !authorized(current) {
				continue
			}
			writeMultiplexSSE(w, "item.changed", fmt.Sprintf(`{"item_id":%d}`, ev.ItemID))
			flusher.Flush()
		case ev := <-wsSub.Events():
			if !authorized(ev.WorkspaceID) {
				continue
			}
			for _, scope := range workspaceScopes[ev.WorkspaceID] {
				writeMultiplexSSE(w, scope.kind+".changed", fmt.Sprintf(`{"%s_id":%d}`, scope.kind, scope.id))
			}
			flusher.Flush()
		case <-userSub.Events():
			writeMultiplexSSE(w, "notification.changed", "{}")
			flusher.Flush()
		case <-heartbeat.C:
			if itemSub.TakeStale() || wsSub.TakeStale() || userSub.TakeStale() {
				// A frame was dropped; the client reconciles every scope.
				writeMultiplexSSE(w, "resync", "{}")
			}
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

// parseScopeIDList parses a comma-separated positive-int scope list. An empty
// value means no scopes.
func parseScopeIDList(w http.ResponseWriter, r *http.Request, raw, field string) ([]int, bool) {
	if raw == "" {
		return nil, true
	}
	parts := strings.Split(raw, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			respondBadRequest(w, r, "invalid "+field+" scope")
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

// writeMultiplexSSE writes one multiplexed SSE frame. Both event and data are
// controlled constants or integers, so neither can inject a frame.
func writeMultiplexSSE(w http.ResponseWriter, event, data string) {
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data) //nolint:gosec // G705: event is a controlled constant and data a JSON literal of ints; response is text/event-stream, not HTML
}
