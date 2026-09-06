package services

import "sync"

// ItemChangeKind enumerates the kinds of item-detail mutations that should push
// a live update to subscribers of an item's event stream. The set is
// deliberately coarse: a subscriber maps a kind to a targeted reload, and any
// unrecognized kind falls back to a full reload, so adding a kind never breaks
// an older client.
type ItemChangeKind string

const (
	ItemChangeCreated ItemChangeKind = "created"
	ItemChangeUpdated ItemChangeKind = "updated"
	ItemChangeStatus  ItemChangeKind = "status"
	ItemChangeDeleted ItemChangeKind = "deleted"
	ItemChangeComment ItemChangeKind = "comment"
	ItemChangeLink    ItemChangeKind = "link"
	ItemChangeZammad  ItemChangeKind = "zammad"
)

// ItemChangePublisher receives item-change notifications after a mutation has
// committed. Plan 2 (WI-484) supplies an in-memory SSE hub implementation; until
// then the process default is a no-op, so wiring publish calls into the mutation
// chokepoints changes no behavior and Plan 1 (WI-483) ships independently.
//
// Implementations must be safe for concurrent use: PublishItemChange is called
// from request goroutines, schedulers, and background workers.
type ItemChangePublisher interface {
	// PublishItemChange announces that the item identified by itemID changed in
	// the given way. It must be cheap and non-blocking; a hub implementation
	// fans out to in-memory subscribers without touching the database.
	PublishItemChange(itemID int, kind ItemChangeKind)
}

type noopItemChangePublisher struct{}

func (noopItemChangePublisher) PublishItemChange(int, ItemChangeKind) {}

var (
	itemChangePubMu sync.RWMutex
	itemChangePub   ItemChangePublisher = noopItemChangePublisher{}
)

// SetItemChangePublisher installs the process-wide item-change publisher. It is
// called once during server startup (Plan 2 passes the SSE hub) and may be
// swapped by tests. Passing nil restores the no-op default.
func SetItemChangePublisher(p ItemChangePublisher) {
	itemChangePubMu.Lock()
	defer itemChangePubMu.Unlock()
	if p == nil {
		p = noopItemChangePublisher{}
	}
	itemChangePub = p
}

// PublishItemChange routes an item-change notification to the installed
// publisher. It is the single entry point every mutation chokepoint calls, so
// coverage is auditable from one symbol.
//
// IMPORTANT: call this only AFTER the underlying database mutation has
// committed — never inside an open transaction that might still roll back. For
// destructive writes (delete), capture the item id (and any parent id) BEFORE
// the write and publish afterwards.
//
// itemID <= 0 is ignored, so callers can pass an optional parent id
// unconditionally.
func PublishItemChange(itemID int, kind ItemChangeKind) {
	if itemID <= 0 {
		return
	}
	itemChangePubMu.RLock()
	p := itemChangePub
	itemChangePubMu.RUnlock()
	p.PublishItemChange(itemID, kind)
}
