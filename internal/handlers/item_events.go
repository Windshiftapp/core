package handlers

import (
	"fmt"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

const sseHeartbeatInterval = 20 * time.Second

// SetSSEHub wires the in-memory hub that backs the item event stream. When nil
// (live updates disabled), the Events endpoint returns 503.
func (h *ItemHandler) SetSSEHub(hub *services.SSEHub) { h.sseHub = hub }

// Events streams item-change events for one item as Server-Sent Events
// (WI-484). GET /items/{id}/events.
//
// The client (useItemEventStream) maps each event's `kind` to a targeted
// reload, and treats `connected`/`reload`/unknown kinds as a full reload, so
// any section not covered by a granular event (attachments, diagrams, worklogs)
// is still reconciled on connect/reconnect/drop.
//
// Gated on item.view, returning 404 on a missing item or missing permission
// (the item-existence-non-leak invariant). The handler holds NO database
// connection while idle: after the connect-time permission check it blocks on
// the subscriber channel and a heartbeat ticker only.
func (h *ItemHandler) Events(w http.ResponseWriter, r *http.Request) {
	if h.sseHub == nil {
		respondServiceUnavailable(w, r, "live updates are not enabled on this server")
		return
	}
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	itemRepo := repository.NewItemRepository(h.db)
	workspaceID, err := itemRepo.GetWorkspaceID(itemID)
	if err != nil {
		respondNotFound(w, r, "Item")
		return
	}
	allowed, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemView)
	if err != nil || !allowed {
		respondNotFound(w, r, "Item")
		return
	}

	// Re-resolve ownership and item.view before every post-connect write so a
	// workspace move or membership revocation takes effect before disclosure.
	authorized := func() bool {
		currentWorkspaceID, err := itemRepo.GetWorkspaceID(itemID)
		if err != nil {
			return false
		}
		ok, err := h.permissionService.HasWorkspacePermission(user.ID, currentWorkspaceID, models.PermissionItemView)
		return err == nil && ok
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondServiceUnavailable(w, r, "streaming is unsupported on this connection")
		return
	}

	// SSE response headers. X-Accel-Buffering disables response buffering for
	// nginx-class proxies; the gzip wrapper is told to skip text/event-stream
	// (middleware/compression.go) so small frames are flushed immediately.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// The server's 30s WriteTimeout is a hard wall-clock deadline that would
	// sever this long-lived response; lift it for the stream.
	unbindStreamDeadlines(w)

	sub := h.sseHub.Subscribe(itemID)
	defer h.sseHub.Unsubscribe(sub)

	// Initial frame: a jittered reconnect hint (thundering-herd guard on a mass
	// disconnect) and a `connected` event so the client runs its first full
	// reconcile.
	_, _ = fmt.Fprintf(w, "retry: %d\n\n", sseRetryMillis(itemID)) //nolint:gosec // G705: SSE control line, numeric only; response is text/event-stream, not HTML
	writeSSEEvent(w, "connected", itemID)
	flusher.Flush()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()
	ctx := r.Context()

	for {
		select {
		case ev := <-sub.Events():
			if !authorized() {
				return
			}
			writeSSEEvent(w, string(ev.Kind), ev.ItemID)
			if sub.TakeStale() {
				// A later event was dropped (buffer overflow); tell the client to
				// reconcile fully so nothing is missed.
				writeSSEEvent(w, "reload", itemID)
			}
			flusher.Flush()
		case <-heartbeat.C:
			// Re-authorize before the next heartbeat write.
			if !authorized() {
				return
			}
			if sub.TakeStale() {
				writeSSEEvent(w, "reload", itemID)
			}
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

// writeSSEEvent writes one SSE frame: an `event:` line naming the kind and a
// `data:` line carrying the item id and kind as JSON.
func writeSSEEvent(w http.ResponseWriter, kind string, itemID int) {
	_, _ = fmt.Fprintf(w, "event: %s\ndata: {\"item_id\":%d,\"kind\":%q}\n\n", kind, itemID, kind) //nolint:gosec // G705: kind is a controlled enum and itemID an int; response is text/event-stream, not HTML
}

// sseRetryMillis returns a reconnect delay in 3000–6999ms, spread by item id so
// a deploy/restart doesn't make every client reconnect at the same instant.
func sseRetryMillis(itemID int) int {
	return 3000 + (itemID*1327)%4000
}
