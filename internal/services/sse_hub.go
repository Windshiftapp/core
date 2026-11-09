package services

import (
	"sync"
	"sync/atomic"
)

// sseSubscriberBuffer bounds each subscriber's pending frames. A publish that
// finds the buffer full drops the frame and flags the subscriber stale, so a
// slow client never stalls the mutation path.
const sseSubscriberBuffer = 16

// ItemSSEEvent is one coarse item-change frame delivered to a subscriber of an
// item's event stream: either a change or an authoritative deletion.
type ItemSSEEvent struct {
	ItemID      int
	Deleted     bool
	WorkspaceID int
}

// WorkspaceSSEEvent is one coarse workspace-scope invalidation frame: item
// membership, ordering or display data in the workspace may have changed.
type WorkspaceSSEEvent struct {
	WorkspaceID int
}

// UserSSEEvent is one coarse per-user invalidation frame: the user's inbox or
// unread count changed.
type UserSSEEvent struct {
	UserID int
}

// SSESubscriber is one open SSE connection's view of a topic family. The
// handler ranges over Events() and writes a frame per event; if the buffer
// overflowed (a publish was dropped), TakeStale reports true so the handler can
// tell the client to reconcile.
type SSESubscriber[T any] struct {
	topics []int
	ch     chan T
	stale  atomic.Bool
}

// Events is the receive end of this subscriber's buffered event channel.
func (s *SSESubscriber[T]) Events() <-chan T { return s.ch }

// TakeStale atomically reports and clears the stale flag. A true result means at
// least one event was dropped since the last call, so the client must reconcile.
func (s *SSESubscriber[T]) TakeStale() bool { return s.stale.Swap(false) }

// sseTopic is a pub/sub topic family keyed by an integer id (item id, workspace
// id or user id). It never blocks on a subscriber.
type sseTopic[T any] struct {
	mu   sync.RWMutex
	subs map[int]map[*SSESubscriber[T]]struct{}
}

func newSSETopic[T any]() *sseTopic[T] {
	return &sseTopic[T]{subs: make(map[int]map[*SSESubscriber[T]]struct{})}
}

// subscribe registers one subscriber against every topic id. Duplicate and
// non-positive ids are ignored. The caller must unsubscribe when the connection
// closes.
func (t *sseTopic[T]) subscribe(topicIDs []int) *SSESubscriber[T] {
	sub := &SSESubscriber[T]{ch: make(chan T, sseSubscriberBuffer)}
	seen := make(map[int]struct{}, len(topicIDs))
	t.mu.Lock()
	for _, id := range topicIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		sub.topics = append(sub.topics, id)
		if t.subs[id] == nil {
			t.subs[id] = make(map[*SSESubscriber[T]]struct{})
		}
		t.subs[id][sub] = struct{}{}
	}
	t.mu.Unlock()
	return sub
}

// unsubscribe removes a subscriber from every topic and prunes empty topics.
func (t *sseTopic[T]) unsubscribe(sub *SSESubscriber[T]) {
	if sub == nil {
		return
	}
	t.mu.Lock()
	for _, id := range sub.topics {
		if set := t.subs[id]; set != nil {
			delete(set, sub)
			if len(set) == 0 {
				delete(t.subs, id)
			}
		}
	}
	t.mu.Unlock()
}

// publish fans an event out to every subscriber of topicID. It never blocks: a
// subscriber whose buffer is full is flagged stale and the event is dropped, so
// one slow client cannot stall the mutation path or other subscribers. The
// copy-under-RLock keeps sends off the lock.
func (t *sseTopic[T]) publish(topicID int, ev T) {
	if topicID <= 0 {
		return
	}
	t.mu.RLock()
	set := t.subs[topicID]
	if len(set) == 0 {
		t.mu.RUnlock()
		return
	}
	subs := make([]*SSESubscriber[T], 0, len(set))
	for s := range set {
		subs = append(subs, s)
	}
	t.mu.RUnlock()

	for _, s := range subs {
		select {
		case s.ch <- ev:
		default:
			s.stale.Store(true)
		}
	}
}

// subscriberCount returns the number of live subscribers for a topic
// (test/observability helper).
func (t *sseTopic[T]) subscriberCount(topicID int) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.subs[topicID])
}

// SSEHub is the in-memory fan-out for item-change, workspace-scope and
// per-user events (WI-484, WI-1624, WI-1625). It implements the corresponding
// publishers, so registering it via their setters turns mutation-chokepoint
// publishes into live pushes.
//
// Single-process only: subscribers live in this process's memory. A multi-replica
// deployment would need Postgres LISTEN/NOTIFY or Redis behind the same
// publisher interfaces; nothing else in the system would change.
type SSEHub struct {
	items      *sseTopic[ItemSSEEvent]
	workspaces *sseTopic[WorkspaceSSEEvent]
	users      *sseTopic[UserSSEEvent]

	mu             sync.Mutex
	userStreams    map[int]int // userID -> live stream count across every stream type
	maxUserStreams int
}

// DefaultMaxUserStreams caps concurrent SSE connections per authenticated user.
// Without a cap, one client can retain handlers, sockets, and subscriptions
// indefinitely because stream routes are exempt from the request concurrency
// limiter and write deadlines are lifted for the connection lifetime.
//
// A tab currently holds one stream per topic family (notifications,
// workspace/collection, item detail), so the bound has to cover several tabs
// plus a brief reconnect overlap: 3 x 10 tabs + headroom. The real fix is
// multiplexing those topics onto one connection per tab (WI-1656); until then
// this is a bounded stopgap, overridable with MAX_USER_STREAMS.
const DefaultMaxUserStreams = 32

// NewSSEHub creates an empty hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		items:          newSSETopic[ItemSSEEvent](),
		workspaces:     newSSETopic[WorkspaceSSEEvent](),
		users:          newSSETopic[UserSSEEvent](),
		userStreams:    make(map[int]int),
		maxUserStreams: DefaultMaxUserStreams,
	}
}

// SetMaxUserStreams overrides the per-user stream cap; a non-positive value
// disables the cap.
func (h *SSEHub) SetMaxUserStreams(limit int) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.maxUserStreams = limit
	h.mu.Unlock()
}

// AcquireUserStream reserves one of the user's stream slots. The returned
// release function must be called exactly once when the connection closes. ok
// is false when the per-user cap is already reached, so the caller can reject
// the connection instead of retaining another handler.
func (h *SSEHub) AcquireUserStream(userID int) (release func(), ok bool) {
	if h == nil {
		return func() {}, true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.maxUserStreams > 0 && h.userStreams[userID] >= h.maxUserStreams {
		return nil, false
	}
	h.userStreams[userID]++
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.userStreams[userID] <= 1 {
				delete(h.userStreams, userID)
				return
			}
			h.userStreams[userID]--
		})
	}, true
}

// UserStreamCount returns the number of live streams for a user
// (test/observability helper).
func (h *SSEHub) UserStreamCount(userID int) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.userStreams[userID]
}

// PublishItemChange fans an item-change out to every subscriber of that item.
func (h *SSEHub) PublishItemChange(itemID int) {
	h.items.publish(itemID, ItemSSEEvent{ItemID: itemID})
}

// PublishItemDeletion carries ownership after the item row has been removed.
func (h *SSEHub) PublishItemDeletion(itemID, workspaceID int) {
	h.items.publish(itemID, ItemSSEEvent{ItemID: itemID, Deleted: true, WorkspaceID: workspaceID})
}

// Subscribe registers a new subscriber for an item topic. The caller must
// Unsubscribe when the connection closes.
func (h *SSEHub) Subscribe(itemID int) *SSESubscriber[ItemSSEEvent] {
	return h.items.subscribe([]int{itemID})
}

// Unsubscribe removes an item subscriber.
func (h *SSEHub) Unsubscribe(sub *SSESubscriber[ItemSSEEvent]) {
	h.items.unsubscribe(sub)
}

// SubscriberCount returns the number of live subscribers for an item
// (test/observability helper).
func (h *SSEHub) SubscriberCount(itemID int) int {
	return h.items.subscriberCount(itemID)
}

// PublishWorkspaceChange fans a workspace invalidation out to every subscriber
// of that workspace.
func (h *SSEHub) PublishWorkspaceChange(workspaceID int) {
	h.workspaces.publish(workspaceID, WorkspaceSSEEvent{WorkspaceID: workspaceID})
}

// SubscribeWorkspaces registers one subscriber against every workspace topic in
// workspaceIDs. The caller must UnsubscribeWorkspaces when the connection closes.
func (h *SSEHub) SubscribeWorkspaces(workspaceIDs []int) *SSESubscriber[WorkspaceSSEEvent] {
	return h.workspaces.subscribe(workspaceIDs)
}

// UnsubscribeWorkspaces removes a workspace subscriber from every topic.
func (h *SSEHub) UnsubscribeWorkspaces(sub *SSESubscriber[WorkspaceSSEEvent]) {
	h.workspaces.unsubscribe(sub)
}

// WorkspaceSubscriberCount returns the number of live subscribers for a
// workspace (test/observability helper).
func (h *SSEHub) WorkspaceSubscriberCount(workspaceID int) int {
	return h.workspaces.subscriberCount(workspaceID)
}

// PublishUserChange fans a per-user invalidation out to that user's subscribers.
func (h *SSEHub) PublishUserChange(userID int) {
	h.users.publish(userID, UserSSEEvent{UserID: userID})
}

// SubscribeUser registers a new subscriber for a user's topic.
func (h *SSEHub) SubscribeUser(userID int) *SSESubscriber[UserSSEEvent] {
	return h.users.subscribe([]int{userID})
}

// UnsubscribeUser removes a user subscriber.
func (h *SSEHub) UnsubscribeUser(sub *SSESubscriber[UserSSEEvent]) {
	h.users.unsubscribe(sub)
}

// UserSubscriberCount returns the number of live subscribers for a user
// (test/observability helper).
func (h *SSEHub) UserSubscriberCount(userID int) int {
	return h.users.subscriberCount(userID)
}
