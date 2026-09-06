package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"windshift/internal/services"
)

type fakeZammadSyncService struct {
	mu          sync.Mutex
	dueCalls    int
	dueStarted  chan int
	releaseDue  chan struct{}
	dueDone     chan struct{}
	allStarted  chan struct{}
	releaseAll  chan struct{}
	allDone     chan struct{}
	allDoneOnce sync.Once
}

func (f *fakeZammadSyncService) SyncDue(context.Context, int) error {
	f.mu.Lock()
	f.dueCalls++
	call := f.dueCalls
	f.mu.Unlock()
	f.dueStarted <- call
	if call == 1 && f.releaseDue != nil {
		<-f.releaseDue
		close(f.dueDone)
	}
	return nil
}

func (f *fakeZammadSyncService) SyncAllTicketLinks(context.Context) (services.ZammadSyncSummary, error) {
	select {
	case f.allStarted <- struct{}{}:
	default:
	}
	if f.releaseAll != nil {
		<-f.releaseAll
	}
	f.allDoneOnce.Do(func() { close(f.allDone) })
	return services.ZammadSyncSummary{Selected: 1, Succeeded: 1}, nil
}

func TestZammadSchedulerWaitsFullIntervalAfterSlowRun(t *testing.T) {
	fake := &fakeZammadSyncService{
		dueStarted: make(chan int, 4), releaseDue: make(chan struct{}), dueDone: make(chan struct{}),
		allStarted: make(chan struct{}, 1), allDone: make(chan struct{}),
	}
	s := &ZammadSyncScheduler{service: fake, interval: 20 * time.Millisecond, triggerSyncAll: make(chan struct{}, 1)}
	s.Start()
	t.Cleanup(s.Stop)

	select {
	case <-fake.dueStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("first scheduled synchronization did not start")
	}
	time.Sleep(50 * time.Millisecond)
	close(fake.releaseDue)
	select {
	case <-fake.dueDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("first scheduled synchronization did not finish")
	}
	select {
	case <-fake.dueStarted:
		t.Fatal("ticker backlog started another synchronization without waiting")
	case <-time.After(10 * time.Millisecond):
	}
	select {
	case <-fake.dueStarted:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("next synchronization did not start after the configured interval")
	}
}

func TestZammadSchedulerCoalescesSystemRefreshRequests(t *testing.T) {
	fake := &fakeZammadSyncService{
		dueStarted: make(chan int, 1), dueDone: make(chan struct{}),
		allStarted: make(chan struct{}, 2), releaseAll: make(chan struct{}), allDone: make(chan struct{}),
	}
	s := &ZammadSyncScheduler{service: fake, interval: time.Hour, triggerSyncAll: make(chan struct{}, 1)}
	s.Start()
	t.Cleanup(s.Stop)

	if !s.TriggerSyncAll() || s.TriggerSyncAll() {
		t.Fatal("queued system refresh was not coalesced")
	}
	select {
	case <-fake.allStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("system refresh did not start")
	}
	if s.TriggerSyncAll() {
		t.Fatal("running system refresh accepted a duplicate trigger")
	}
	close(fake.releaseAll)
	select {
	case <-fake.allDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("system refresh did not finish")
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for !s.TriggerSyncAll() {
		if time.Now().After(deadline) {
			t.Fatal("system refresh trigger remained stuck after completion")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestZammadSchedulerStartDropsStaleSystemRefresh(t *testing.T) {
	fake := &fakeZammadSyncService{
		dueStarted: make(chan int, 1), dueDone: make(chan struct{}),
		allStarted: make(chan struct{}, 1), releaseAll: make(chan struct{}), allDone: make(chan struct{}),
	}
	trigger := make(chan struct{}, 1)
	trigger <- struct{}{}
	s := &ZammadSyncScheduler{
		service: fake, interval: time.Hour, triggerSyncAll: trigger,
		syncAllPending: true,
	}
	s.Start()
	t.Cleanup(s.Stop)

	if !s.TriggerSyncAll() {
		t.Fatal("fresh system refresh was coalesced with a stale request")
	}
	select {
	case <-fake.allStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("fresh system refresh did not start")
	}
	close(fake.releaseAll)
}
