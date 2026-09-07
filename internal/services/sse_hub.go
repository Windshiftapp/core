package services

import (
	"sync"
	"sync/atomic"
)

// ItemSSEEvent is one item-change frame delivered to a subscriber of an item's
// event stream.
type ItemSSEEvent struct {
	ItemID      int
	Kind        ItemChangeKind
	WorkspaceID int
}

// ItemSubscriber is one open SSE connection's view of an item topic. The SSE
// handler ranges over Events() and writes a frame per event; if the buffer
// overflowed (a publish was dropped), Stale reports true so the handler can tell
// the client to do a full reload instead of trusting incremental events.
type ItemSubscriber struct {
	itemID int
	ch     chan ItemSSEEvent
	stale  atomic.Bool
}

// Events is the receive end of this subscriber's buffered event channel.
func (s *ItemSubscriber) Events() <-chan ItemSSEEvent { return s.ch }

// TakeStale atomically reports and clears the stale flag. A true result means at
// least one event was dropped since the last call, so the client must reconcile
// with a full reload.
func (s *ItemSubscriber) TakeStale() bool { return s.stale.Swap(false) }

// ItemID is the topic this subscriber is attached to.
func (s *ItemSubscriber) ItemID() int { return s.itemID }

// SSEHub is the in-memory fan-out for item-change events (WI-484). It implements
// ItemChangePublisher (WI-483), so registering it via SetItemChangePublisher
// turns every mutation chokepoint's publish into a live push.
//
// Single-process only: subscribers live in this process's memory. A multi-replica
// deployment would need Postgres LISTEN/NOTIFY or Redis behind the same
// ItemChangePublisher interface; nothing else in the system would change.
type SSEHub struct {
	mu   sync.RWMutex
	subs map[int]map[*ItemSubscriber]struct{} // itemID -> set of subscribers
}

// NewSSEHub creates an empty hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{subs: make(map[int]map[*ItemSubscriber]struct{})}
}

// PublishItemChange fans an item-change out to every subscriber of that item.
// It never blocks: a subscriber whose buffer is full is flagged stale and the
// event is dropped, so one slow client cannot stall the mutation path or other
// subscribers. The copy-under-RLock keeps sends off the lock.
func (h *SSEHub) PublishItemChange(itemID int, kind ItemChangeKind) {
	h.publish(ItemSSEEvent{ItemID: itemID, Kind: kind})
}

// PublishItemDeletion carries ownership after the item row has been removed.
func (h *SSEHub) PublishItemDeletion(itemID, workspaceID int) {
	h.publish(ItemSSEEvent{ItemID: itemID, Kind: ItemChangeDeleted, WorkspaceID: workspaceID})
}

func (h *SSEHub) publish(ev ItemSSEEvent) {
	itemID := ev.ItemID
	if itemID <= 0 {
		return
	}
	h.mu.RLock()
	set := h.subs[itemID]
	if len(set) == 0 {
		h.mu.RUnlock()
		return
	}
	subs := make([]*ItemSubscriber, 0, len(set))
	for s := range set {
		subs = append(subs, s)
	}
	h.mu.RUnlock()

	for _, s := range subs {
		select {
		case s.ch <- ev:
		default:
			s.stale.Store(true)
		}
	}
}

// Subscribe registers a new subscriber for an item topic. The caller must
// Unsubscribe when the connection closes.
func (h *SSEHub) Subscribe(itemID int) *ItemSubscriber {
	sub := &ItemSubscriber{itemID: itemID, ch: make(chan ItemSSEEvent, 16)}
	h.mu.Lock()
	if h.subs[itemID] == nil {
		h.subs[itemID] = make(map[*ItemSubscriber]struct{})
	}
	h.subs[itemID][sub] = struct{}{}
	h.mu.Unlock()
	return sub
}

// Unsubscribe removes a subscriber and prunes the topic if it becomes empty.
func (h *SSEHub) Unsubscribe(sub *ItemSubscriber) {
	if sub == nil {
		return
	}
	h.mu.Lock()
	if set := h.subs[sub.itemID]; set != nil {
		delete(set, sub)
		if len(set) == 0 {
			delete(h.subs, sub.itemID)
		}
	}
	h.mu.Unlock()
}

// SubscriberCount returns the number of live subscribers for an item (test/observability helper).
func (h *SSEHub) SubscriberCount(itemID int) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[itemID])
}
