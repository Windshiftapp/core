package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/services"
)

type ZammadSyncScheduler struct {
	service        zammadSyncService
	interval       time.Duration
	timer          *time.Timer
	triggerSyncAll chan struct{}
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.Mutex
	running        bool
	syncAllPending bool
}

type zammadSyncService interface {
	SyncDue(context.Context, int) error
	SyncAllTicketLinks(context.Context) (services.ZammadSyncSummary, error)
}

func NewZammadSyncScheduler(service *services.ZammadService) *ZammadSyncScheduler {
	return &ZammadSyncScheduler{service: service, interval: 2 * time.Minute, triggerSyncAll: make(chan struct{}, 1)}
}

func (s *ZammadSyncScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	// A shutdown can win the select race after a system-wide refresh was
	// queued. Drop that stale request before starting a fresh loop so future
	// refreshes are not permanently coalesced with work from the previous run.
	s.syncAllPending = false
	for {
		select {
		case <-s.triggerSyncAll:
			continue
		default:
			goto triggerDrained
		}
	}

triggerDrained:
	s.timer = time.NewTimer(s.interval)
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true
	s.wg.Add(1)
	go s.loop(ctx, s.timer)
}

// TriggerSyncAll queues one immediate system-wide refresh. A second request is
// coalesced while the first is queued or running.
func (s *ZammadSyncScheduler) TriggerSyncAll() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.syncAllPending {
		return false
	}
	s.syncAllPending = true
	s.triggerSyncAll <- struct{}{}
	return true
}

func (s *ZammadSyncScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.timer.Stop()
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *ZammadSyncScheduler) loop(ctx context.Context, timer *time.Timer) {
	defer s.wg.Done()
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			if err := s.service.SyncDue(ctx, 50); err != nil && ctx.Err() == nil {
				slog.Warn("Zammad ticket synchronization failed", slog.String("component", "zammad-sync"), slog.Any("error", err))
			}
			timer.Reset(s.interval)
		case <-s.triggerSyncAll:
			if ctx.Err() != nil {
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			summary, err := s.service.SyncAllTicketLinks(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Warn("System-wide Zammad ticket synchronization failed", slog.String("component", "zammad-sync"), slog.Any("error", err))
			} else if ctx.Err() == nil {
				slog.Info("System-wide Zammad ticket synchronization completed", slog.String("component", "zammad-sync"),
					slog.Int("selected", summary.Selected), slog.Int("succeeded", summary.Succeeded),
					slog.Int("failed", summary.Failed), slog.Int("skipped", summary.Skipped))
			}
			s.mu.Lock()
			s.syncAllPending = false
			s.mu.Unlock()
			timer.Reset(s.interval)
		case <-ctx.Done():
			return
		}
	}
}
