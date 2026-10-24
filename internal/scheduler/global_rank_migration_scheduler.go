package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
)

const (
	globalRankMigrationSchedulerName   = "global_rank_migration"
	defaultGlobalRankMigrationInterval = time.Minute
	// Keep enough headroom for a 100k-row migration without continuously
	// rewriting the rank index underneath deep-page list scans. With one set
	// update per 128-row batch, a 500ms cadence drains 100k rows in roughly seven
	// minutes while leaving substantial headroom for list traffic.
	defaultGlobalRankMigrationActive = 500 * time.Millisecond
	globalRankMigrationTickTimeout   = 30 * time.Second
)

// GlobalRankMigrationScheduler resumes an explicitly active global rank
// migration in bounded worker transactions. Stable state is deliberately a
// no-op: the worker can start a migration when called directly, but a periodic
// scheduler must not rotate buckets forever without an operator or service
// requesting that work.
type GlobalRankMigrationScheduler struct {
	db      database.Database
	worker  *repository.GlobalRankMigrationWorker
	runRepo *repository.SchedulerRunRepository
	owner   string

	ticker  *time.Ticker
	wake    chan struct{}
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.RWMutex
	running bool

	interval time.Duration
	active   time.Duration
	timeout  time.Duration
	// afterTick is a deterministic white-box test notification. Production
	// constructors leave it nil.
	afterTick func()
}

// NewGlobalRankMigrationScheduler builds the in-process resumable migration
// scheduler. owner must identify the application instance; it is persisted in
// the lease so concurrent instances do not process the same batch.
func NewGlobalRankMigrationScheduler(db database.Database, owner string) *GlobalRankMigrationScheduler {
	if owner == "" {
		owner = "global-rank-migration"
	}
	return &GlobalRankMigrationScheduler{
		db:       db,
		worker:   repository.NewGlobalRankMigrationWorker(db, owner, repository.DefaultGlobalRankMigrationBatchSize, repository.DefaultGlobalRankMigrationLease),
		runRepo:  repository.NewSchedulerRunRepository(db),
		owner:    owner,
		interval: defaultGlobalRankMigrationInterval,
		active:   defaultGlobalRankMigrationActive,
		timeout:  globalRankMigrationTickTimeout,
	}
}

// Start begins the scheduler loop. It is safe to call more than once.
func (s *GlobalRankMigrationScheduler) Start() {
	for {
		s.mu.Lock()
		if s.running {
			s.mu.Unlock()
			return
		}
		// A concurrent Stop marks running false before the old loop has joined.
		// Wait for that loop before installing a replacement ticker/wake channel.
		if s.done != nil {
			done := s.done
			select {
			case <-done:
				s.done = nil
			default:
				s.mu.Unlock()
				<-done
				continue
			}
		}
		break
	}
	defer s.mu.Unlock()
	interval := s.interval
	if interval <= 0 {
		interval = defaultGlobalRankMigrationInterval
	}
	s.ticker = time.NewTicker(interval)
	s.wake = make(chan struct{}, 1)
	loopContext, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	s.running = true
	slog.Info("starting global rank migration scheduler", "interval", interval, "owner", s.owner)
	go s.loop(loopContext, s.ticker, s.wake, s.done)
}

// Stop halts the scheduler loop. It is safe to call more than once.
func (s *GlobalRankMigrationScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		done := s.done
		s.mu.Unlock()
		if done != nil {
			<-done
		}
		return
	}
	s.running = false
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	slog.Info("global rank migration scheduler stopped", "owner", s.owner)
}

func (s *GlobalRankMigrationScheduler) loop(ctx context.Context, ticker *time.Ticker, wake <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	// Resume a migration left active by an earlier process immediately after
	// startup; do not wait for the first interval.
	activeDelay := s.active
	if activeDelay <= 0 {
		activeDelay = defaultGlobalRankMigrationActive
	}
	var activeTimer *time.Timer
	var activeTick <-chan time.Time
	scheduleActive := func(shouldContinue bool) {
		if !shouldContinue {
			if activeTimer != nil {
				activeTimer.Stop()
			}
			activeTick = nil
			return
		}
		if activeTimer == nil {
			activeTimer = time.NewTimer(activeDelay)
		} else {
			if !activeTimer.Stop() {
				select {
				case <-activeTimer.C:
				default:
				}
			}
			activeTimer.Reset(activeDelay)
		}
		activeTick = activeTimer.C
	}
	defer func() {
		if activeTimer != nil {
			activeTimer.Stop()
		}
	}()
	scheduleActive(s.tick(ctx))
	for {
		select {
		case <-ticker.C:
			scheduleActive(s.tick(ctx))
		case <-wake:
			scheduleActive(s.tick(ctx))
		case <-activeTick:
			activeTick = nil
			scheduleActive(s.tick(ctx))
		case <-ctx.Done():
			return
		}
	}
}

// Wake requests an immediate scheduler pass after an operator starts or
// resumes a migration. It is non-blocking and coalesces duplicate requests.
func (s *GlobalRankMigrationScheduler) Wake() {
	if s == nil {
		return
	}
	s.mu.RLock()
	wake := s.wake
	running := s.running
	s.mu.RUnlock()
	if !running || wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

// RunOnce performs one scheduler pass. The bool reports whether the durable
// state was runnable. Only migrating state is runnable; stable, paused, legacy,
// and failed states are observed without invoking the worker. It is exported
// for admin/diagnostic callers and deterministic tests.
func (s *GlobalRankMigrationScheduler) RunOnce(ctx context.Context) (result repository.GlobalRankMigrationBatchResult, runnable bool, err error) {
	if s == nil || s.db == nil || s.worker == nil {
		return result, false, errors.New("global rank migration scheduler requires a database")
	}
	if ctx == nil {
		return result, false, errors.New("global rank migration scheduler requires a context")
	}
	state, err := repository.LoadGlobalRankState(s.db)
	if err != nil {
		return result, false, err
	}
	if state.Phase != repository.GlobalRankPhaseMigrating {
		return repository.GlobalRankMigrationBatchResult{State: state}, false, nil
	}
	result, err = s.worker.Run(ctx)
	return result, true, err
}

func (s *GlobalRankMigrationScheduler) tick(parent context.Context) (continueActive bool) {
	start := time.Now()
	itemsProcessed := 0
	var runErr error
	defer recordSchedulerRun(s.runRepo, globalRankMigrationSchedulerName, start, &itemsProcessed, &runErr, time.Now)
	defer func() {
		if s.afterTick != nil {
			s.afterTick()
		}
	}()

	timeout := s.timeout
	if timeout <= 0 {
		timeout = globalRankMigrationTickTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	result, active, err := s.RunOnce(ctx)
	if err != nil {
		runErr = err
		slog.Error("global rank migration scheduler tick", "owner", s.owner, "error", err)
		return false
	}
	if !active {
		return false
	}
	itemsProcessed = result.Migrated
	if result.Completed {
		slog.Info("global rank migration completed", "owner", s.owner, "active_bucket", result.State.ActiveBucket, "migrated", result.Migrated)
	}
	// A live lease held by another instance is observed on the ordinary idle
	// interval. The owning instance alone uses the faster active cadence.
	return result.LeaseAcquired && !result.Completed
}
