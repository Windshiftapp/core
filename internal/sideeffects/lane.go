package sideeffects

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// DropReason explains why a candidate was not admitted to a lane.
type DropReason string

const (
	// DropFull means the lane's count or byte budget was exhausted.
	DropFull DropReason = "full"
	// DropClosed means the lane has stopped accepting candidates.
	DropClosed DropReason = "closed"
	// DropOversized means the payload exceeded the lane's payload limit.
	DropOversized DropReason = "oversized"
	// DropExpired means the event was older than the lane's age limit.
	DropExpired DropReason = "expired"
	// DropStagingLimit means the transaction's staging budget was exhausted.
	DropStagingLimit DropReason = "staging_limit"
	// DropNoLane means a staging buffer was asked to stage to no lane.
	DropNoLane DropReason = "no_lane"
)

// Candidate is an immutable side-effect snapshot admitted to a lane. Payload
// is copied on admission, so callers may reuse or mutate their own buffers.
type Candidate struct {
	// EventType identifies the side effect. It is diagnostic only; the lane
	// does not interpret it.
	EventType string
	// Key is an optional idempotency key carried through to the handler.
	Key string
	// Payload is the encoded effect. It is never retained by reference.
	Payload []byte
	// OccurredAt bounds how long an effect may wait before it is dropped.
	// A zero value is treated as the admission time.
	OccurredAt time.Time
}

func (c Candidate) size() int64 { return int64(len(c.Payload)) }

func cloneCandidate(c Candidate, now time.Time) Candidate {
	clone := Candidate{EventType: c.EventType, Key: c.Key, OccurredAt: c.OccurredAt}
	if len(c.Payload) > 0 {
		clone.Payload = append([]byte(nil), c.Payload...)
	}
	if clone.OccurredAt.IsZero() {
		clone.OccurredAt = now
	}
	return clone
}

// Handler executes one admitted candidate. A handler must honor context
// cancellation; the lane cancels its context at the execution deadline and
// during shutdown. An error counts as one failed dispatch and is not retried.
type Handler interface {
	Handle(ctx context.Context, candidate Candidate) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, candidate Candidate) error

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, candidate Candidate) error {
	return f(ctx, candidate)
}

// LaneConfig configures a bounded lane. Every field is required: a zero or
// negative limit is rejected rather than treated as unlimited.
type LaneConfig struct {
	// Name labels the lane in diagnostics.
	Name string
	// Capacity bounds the count of staged, queued, and active candidates.
	Capacity int
	// ByteCapacity bounds the retained bytes across staged, queued, and active
	// candidates.
	ByteCapacity int64
	// MaxPayloadBytes bounds a single candidate's payload.
	MaxPayloadBytes int
	// MaxEventAge bounds how long a candidate may wait before execution.
	MaxEventAge time.Duration
	// Workers is the fixed number of concurrent handlers.
	Workers int
	// ExecutionTimeout bounds one handler invocation.
	ExecutionTimeout time.Duration
	// Handler executes admitted candidates. It is required.
	Handler Handler
	// Clock supplies the current time. It defaults to time.Now and exists for
	// deterministic tests.
	Clock func() time.Time
}

// Lane is a bounded process-local queue with a fixed worker pool.
type Lane struct {
	cfg    LaneConfig
	clock  func() time.Time
	budget *budget
	queue  chan *queuedEvent

	// emitMu serializes admission against shutdown. Admission holds the read
	// lock for the duration of the nonblocking enqueue; shutdown holds the
	// write lock while it closes admission.
	emitMu sync.RWMutex
	closed bool

	startMu      sync.Mutex
	started      bool
	workerCtx    context.Context
	workerCancel context.CancelFunc
	wg           sync.WaitGroup

	shutdownMu   sync.Mutex
	shutdownDone bool

	queued      atomic.Int64
	queuedBytes atomic.Int64
	active      atomic.Int64

	stats laneStats
}

type queuedEvent struct {
	candidate   Candidate
	reservation *reservation
	enqueuedAt  time.Time
}

type laneStats struct {
	mu                sync.Mutex
	admitted          int64
	processed         int64
	failed            int64
	timedOut          int64
	shutdownDiscarded int64
	dropped           map[DropReason]int64
	lastQueueWait     time.Duration
	maxQueueWait      time.Duration
	lastExecution     time.Duration
	maxExecution      time.Duration
}

// NewLane validates cfg and returns a lane. The lane does not process
// candidates until Start is called.
func NewLane(cfg LaneConfig) (*Lane, error) {
	if cfg.Name == "" {
		return nil, errors.New("sideeffects: lane name is required")
	}
	if cfg.Capacity <= 0 {
		return nil, fmt.Errorf("sideeffects: lane %q capacity must be positive", cfg.Name)
	}
	if cfg.ByteCapacity <= 0 {
		return nil, fmt.Errorf("sideeffects: lane %q byte capacity must be positive", cfg.Name)
	}
	if cfg.MaxPayloadBytes <= 0 {
		return nil, fmt.Errorf("sideeffects: lane %q max payload bytes must be positive", cfg.Name)
	}
	if cfg.MaxEventAge <= 0 {
		return nil, fmt.Errorf("sideeffects: lane %q max event age must be positive", cfg.Name)
	}
	if cfg.Workers <= 0 {
		return nil, fmt.Errorf("sideeffects: lane %q workers must be positive", cfg.Name)
	}
	if cfg.ExecutionTimeout <= 0 {
		return nil, fmt.Errorf("sideeffects: lane %q execution timeout must be positive", cfg.Name)
	}
	if cfg.Handler == nil {
		return nil, fmt.Errorf("sideeffects: lane %q handler is required", cfg.Name)
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Lane{
		cfg:    cfg,
		clock:  clock,
		budget: newBudget(cfg.Capacity, cfg.ByteCapacity),
		queue:  make(chan *queuedEvent, cfg.Capacity),
	}, nil
}

// Name returns the lane's diagnostic name.
func (l *Lane) Name() string { return l.cfg.Name }

// Start launches the worker pool. It is idempotent. The supplied context is
// the parent of the workers' context; canceling it stops execution.
func (l *Lane) Start(ctx context.Context) {
	l.startMu.Lock()
	defer l.startMu.Unlock()
	if l.started {
		return
	}
	l.started = true
	l.workerCtx, l.workerCancel = context.WithCancel(ctx)
	for i := 0; i < l.cfg.Workers; i++ {
		l.wg.Add(1)
		go l.worker()
	}
}

// TryAdmit offers a candidate to the lane without waiting. It reports whether
// the candidate was admitted and, when it was not, why.
func (l *Lane) TryAdmit(candidate Candidate) (bool, DropReason) {
	now := l.clock()
	reservation, reason := l.reserve(candidate.size(), true)
	if reason != "" {
		return false, reason
	}
	// The candidate is cloned only after its budget is reserved, so the lane
	// never retains a caller-owned buffer.
	return l.publishOwned(cloneCandidate(candidate, now), reservation)
}

// reserve checks the lane's limits and reserves budget without waiting. When
// record is true it also counts the rejection in the lane's diagnostics;
// staging callers record it themselves so a per-transaction drop is visible
// after the staging buffer is discarded.
func (l *Lane) reserve(size int64, record bool) (*reservation, DropReason) {
	l.emitMu.RLock()
	defer l.emitMu.RUnlock()
	if l.closed {
		if record {
			l.recordDrop(DropClosed)
		}
		return nil, DropClosed
	}
	if !l.budget.tryReserve(size) {
		if record {
			l.recordDrop(DropFull)
		}
		return nil, DropFull
	}
	return &reservation{lane: l, bytes: size}, ""
}

// reserveForStaging reserves budget for a staged candidate. Unlike reserve it
// also enforces payload and age limits, because a staging buffer may be handed
// a candidate that never reaches TryAdmit.
func (l *Lane) reserveForStaging(candidate Candidate, now time.Time) (*reservation, DropReason) {
	if reason := l.checkLimits(candidate, now); reason != "" {
		return nil, reason
	}
	return l.reserve(candidate.size(), false)
}

// publishOwned enqueues an immutable candidate whose budget is already
// reserved. The candidate is retained as-is; callers must not mutate it.
func (l *Lane) publishOwned(candidate Candidate, reserved *reservation) (bool, DropReason) {
	l.emitMu.RLock()
	defer l.emitMu.RUnlock()
	if l.closed {
		reserved.release()
		l.recordDrop(DropClosed)
		return false, DropClosed
	}
	now := l.clock()
	if reason := l.checkLimits(candidate, now); reason != "" {
		reserved.release()
		l.recordDrop(reason)
		return false, reason
	}
	queued := &queuedEvent{candidate: candidate, reservation: reserved, enqueuedAt: now}
	select {
	case l.queue <- queued:
		l.queued.Add(1)
		l.queuedBytes.Add(candidate.size())
		l.recordAdmitted()
		return true, ""
	default:
		reserved.release()
		l.recordDrop(DropFull)
		return false, DropFull
	}
}

// checkLimits applies the per-candidate payload and age bounds.
func (l *Lane) checkLimits(candidate Candidate, now time.Time) DropReason {
	if len(candidate.Payload) > l.cfg.MaxPayloadBytes {
		return DropOversized
	}
	if !candidate.OccurredAt.IsZero() && now.Sub(candidate.OccurredAt) > l.cfg.MaxEventAge {
		return DropExpired
	}
	return ""
}

func (l *Lane) worker() {
	defer l.wg.Done()
	for {
		select {
		case <-l.workerCtx.Done():
			return
		case queued := <-l.queue:
			// After cancellation a worker must not start new work; discard the
			// candidate it already pulled instead of running it.
			if l.workerCtx.Err() != nil {
				l.discard(queued)
				return
			}
			l.run(queued)
		}
	}
}

func (l *Lane) run(queued *queuedEvent) {
	l.queued.Add(-1)
	l.queuedBytes.Add(-queued.candidate.size())
	l.active.Add(1)
	defer l.active.Add(-1)
	defer queued.reservation.release()

	now := l.clock()
	l.recordQueueWait(now.Sub(queued.enqueuedAt))
	if now.Sub(queued.candidate.OccurredAt) > l.cfg.MaxEventAge {
		l.recordDrop(DropExpired)
		return
	}

	ctx, cancel := context.WithTimeout(l.workerCtx, l.cfg.ExecutionTimeout)
	defer cancel()

	start := l.clock()
	err := l.invoke(ctx, queued.candidate)
	l.recordExecution(l.clock().Sub(start))
	switch {
	case err == nil:
		l.recordProcessed()
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		l.recordTimeout()
	default:
		l.recordFailure()
	}
}

// invoke runs the handler with panic recovery so one bad handler cannot take
// down the worker pool.
func (l *Lane) invoke(ctx context.Context, candidate Candidate) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("sideeffects: lane %q handler panic: %v", l.cfg.Name, recovered)
		}
	}()
	return l.cfg.Handler.Handle(ctx, candidate)
}

// Shutdown stops admission, waits for queued and active work to drain until
// ctx is done, then cancels active handlers, discards remaining queued
// candidates, and waits for the workers to exit. It is safe to call
// concurrently and repeatedly; later calls return after the first completes.
func (l *Lane) Shutdown(ctx context.Context) error {
	l.shutdownMu.Lock()
	defer l.shutdownMu.Unlock()
	if l.shutdownDone {
		return nil
	}
	defer func() { l.shutdownDone = true }()

	l.closeAdmission()

	l.waitDrained(ctx)
	if l.workerCancel != nil {
		l.workerCancel()
	}
	l.wg.Wait()
	l.discardQueued()
	return nil
}

func (l *Lane) closeAdmission() {
	l.emitMu.Lock()
	l.closed = true
	l.emitMu.Unlock()
}

// waitDrained blocks until the queue and active handlers are empty or ctx is
// done. Workers keep processing while it waits.
func (l *Lane) waitDrained(ctx context.Context) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if l.queued.Load() == 0 && l.active.Load() == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (l *Lane) discardQueued() {
	for {
		select {
		case queued := <-l.queue:
			l.discard(queued)
		default:
			return
		}
	}
}

// discard releases one queued candidate without executing it.
func (l *Lane) discard(queued *queuedEvent) {
	l.queued.Add(-1)
	l.queuedBytes.Add(-queued.candidate.size())
	queued.reservation.release()
	l.recordShutdownDiscard()
}

// Stats returns a consistent snapshot of the lane's counters.
func (l *Lane) Stats() LaneStats {
	l.stats.mu.Lock()
	defer l.stats.mu.Unlock()
	dropped := make(map[DropReason]int64, len(l.stats.dropped))
	for reason, count := range l.stats.dropped {
		dropped[reason] = count
	}
	return LaneStats{
		Name:              l.cfg.Name,
		Capacity:          l.cfg.Capacity,
		ByteCapacity:      l.cfg.ByteCapacity,
		Workers:           l.cfg.Workers,
		Queued:            int(l.queued.Load()),
		QueuedBytes:       l.queuedBytes.Load(),
		Active:            int(l.active.Load()),
		CountUsed:         l.budget.count(),
		BytesUsed:         l.budget.bytes(),
		Admitted:          l.stats.admitted,
		Processed:         l.stats.processed,
		Failed:            l.stats.failed,
		TimedOut:          l.stats.timedOut,
		ShutdownDiscarded: l.stats.shutdownDiscarded,
		Dropped:           dropped,
		LastQueueWait:     l.stats.lastQueueWait,
		MaxQueueWait:      l.stats.maxQueueWait,
		LastExecution:     l.stats.lastExecution,
		MaxExecution:      l.stats.maxExecution,
	}
}

// LaneStats is a point-in-time view of a lane's bounded resources and
// outcomes. Dropped separates pre-start and overload drops by reason.
type LaneStats struct {
	Name              string
	Capacity          int
	ByteCapacity      int64
	Workers           int
	Queued            int
	QueuedBytes       int64
	Active            int
	CountUsed         int
	BytesUsed         int64
	Admitted          int64
	Processed         int64
	Failed            int64
	TimedOut          int64
	ShutdownDiscarded int64
	Dropped           map[DropReason]int64
	LastQueueWait     time.Duration
	MaxQueueWait      time.Duration
	LastExecution     time.Duration
	MaxExecution      time.Duration
}

func (l *Lane) recordAdmitted() {
	l.stats.mu.Lock()
	l.stats.admitted++
	l.stats.mu.Unlock()
}

func (l *Lane) recordProcessed() {
	l.stats.mu.Lock()
	l.stats.processed++
	l.stats.mu.Unlock()
}

func (l *Lane) recordFailure() {
	l.stats.mu.Lock()
	l.stats.failed++
	l.stats.mu.Unlock()
}

func (l *Lane) recordTimeout() {
	l.stats.mu.Lock()
	l.stats.timedOut++
	l.stats.mu.Unlock()
}

func (l *Lane) recordShutdownDiscard() {
	l.stats.mu.Lock()
	l.stats.shutdownDiscarded++
	l.stats.mu.Unlock()
}

func (l *Lane) recordDrop(reason DropReason) {
	l.stats.mu.Lock()
	if l.stats.dropped == nil {
		l.stats.dropped = make(map[DropReason]int64)
	}
	l.stats.dropped[reason]++
	l.stats.mu.Unlock()
}

func (l *Lane) recordQueueWait(wait time.Duration) {
	l.stats.mu.Lock()
	l.stats.lastQueueWait = wait
	if wait > l.stats.maxQueueWait {
		l.stats.maxQueueWait = wait
	}
	l.stats.mu.Unlock()
}

func (l *Lane) recordExecution(duration time.Duration) {
	l.stats.mu.Lock()
	l.stats.lastExecution = duration
	if duration > l.stats.maxExecution {
		l.stats.maxExecution = duration
	}
	l.stats.mu.Unlock()
}

// budget tracks count and byte reservations. Reservations are released exactly
// once, so a candidate dropped after reservation cannot leak capacity.
type budget struct {
	mu         sync.Mutex
	countLimit int
	countUsed  int
	byteLimit  int64
	byteUsed   int64
}

func newBudget(countLimit int, byteLimit int64) *budget {
	return &budget{countLimit: countLimit, byteLimit: byteLimit}
}

func (b *budget) tryReserve(size int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.countUsed >= b.countLimit || b.byteUsed+size > b.byteLimit {
		return false
	}
	b.countUsed++
	b.byteUsed += size
	return true
}

func (b *budget) release(size int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.countUsed--
	b.byteUsed -= size
}

func (b *budget) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.countUsed
}

func (b *budget) bytes() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.byteUsed
}

// reservation holds one event's lane budget until it is executed or dropped.
type reservation struct {
	lane     *Lane
	bytes    int64
	released atomic.Bool
}

func (r *reservation) release() {
	if r == nil {
		return
	}
	if r.released.CompareAndSwap(false, true) {
		r.lane.budget.release(r.bytes)
	}
}
