package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"windshift/internal/database"

	"uuid"
)

// Handler processes one durable event delivery.
type Handler interface {
	Handle(context.Context, Event) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(context.Context, Event) error

func (f HandlerFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Config controls bounded worker behavior.
type Config struct {
	WorkerCount                int
	PollInterval               time.Duration
	LeaseDuration              time.Duration
	HandlerTimeout             time.Duration
	ReconcileBatch             int
	MaxAttempts                int
	BaseRetryDelay             time.Duration
	MaxRetryDelay              time.Duration
	RetentionInterval          time.Duration
	CompletedDeliveryRetention time.Duration
	EventRetention             time.Duration
	RetentionBatch             int
}

// DefaultConfig returns production defaults for durable event delivery.
func DefaultConfig() Config {
	return Config{
		WorkerCount:                4,
		PollInterval:               500 * time.Millisecond,
		LeaseDuration:              45 * time.Second,
		HandlerTimeout:             30 * time.Second,
		ReconcileBatch:             250,
		MaxAttempts:                8,
		BaseRetryDelay:             time.Second,
		MaxRetryDelay:              5 * time.Minute,
		RetentionInterval:          6 * time.Hour,
		CompletedDeliveryRetention: 30 * 24 * time.Hour,
		EventRetention:             90 * 24 * time.Hour,
		RetentionBatch:             500,
	}
}

func (c Config) validate() error {
	if c.WorkerCount <= 0 {
		return errors.New("worker count must be positive")
	}
	if c.PollInterval <= 0 {
		return errors.New("poll interval must be positive")
	}
	if c.LeaseDuration <= 0 || c.HandlerTimeout <= 0 {
		return errors.New("lease duration and handler timeout must be positive")
	}
	if c.LeaseDuration <= c.HandlerTimeout {
		return errors.New("lease duration must exceed handler timeout")
	}
	if c.ReconcileBatch <= 0 || c.MaxAttempts <= 0 {
		return errors.New("reconcile batch and max attempts must be positive")
	}
	if c.BaseRetryDelay <= 0 || c.MaxRetryDelay < c.BaseRetryDelay {
		return errors.New("retry delays are invalid")
	}
	if c.RetentionInterval != 0 {
		if c.RetentionInterval < time.Minute || c.CompletedDeliveryRetention <= 0 ||
			c.EventRetention < c.CompletedDeliveryRetention || c.RetentionBatch <= 0 {
			return errors.New("retention settings are invalid")
		}
	}
	return nil
}

// Engine reconciles subscription deliveries and runs their handlers.
type Engine struct {
	store  *Store
	config Config
	owner  string

	mu            sync.Mutex
	handlers      map[string]Handler
	nextHandler   int
	started       bool
	used          bool
	cancel        context.CancelFunc
	done          chan struct{}
	reconcileWake chan struct{}
	workerWake    chan struct{}
	wg            sync.WaitGroup

	now    func() time.Time
	jitter func(time.Duration) time.Duration
	// leaseRenewed is a test observation hook set before engine startup.
	leaseRenewed func(Delivery, time.Time)
}

// NewEngine creates a stopped engine. Register handlers before Start.
func NewEngine(db database.Database, config Config) *Engine {
	return &Engine{
		store:         NewStore(db),
		config:        config,
		owner:         "domain-events-" + uuid.New().String(),
		handlers:      make(map[string]Handler),
		reconcileWake: make(chan struct{}, 1),
		workerWake:    make(chan struct{}, max(config.WorkerCount, 1)),
		now:           time.Now,
		jitter: func(delay time.Duration) time.Duration {
			window := max(delay/4, time.Nanosecond)
			return rand.N(window) //nolint:gosec // Retry jitter does not protect a secret.
		},
	}
}

// Store exposes administrative and transactional persistence operations.
func (e *Engine) Store() *Store {
	return e.store
}

// RegisterHandler binds code to a database consumer key before startup.
func (e *Engine) RegisterHandler(consumerKey string, handler Handler) error {
	if consumerKey == "" || handler == nil {
		return errors.New("consumer key and handler are required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started || e.used {
		return errors.New("cannot register domain event handler after startup")
	}
	if _, exists := e.handlers[consumerKey]; exists {
		return fmt.Errorf("domain event handler %q already registered", consumerKey)
	}
	e.handlers[consumerKey] = handler
	return nil
}

// Start begins reconciliation and bounded delivery workers.
func (e *Engine) Start(ctx context.Context) error {
	if err := e.config.validate(); err != nil {
		return fmt.Errorf("start domain event engine: %w", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		return errors.New("domain event engine already started")
	}
	if e.used {
		return errors.New("domain event engine cannot be restarted")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.started = true
	e.used = true
	e.done = make(chan struct{})
	if len(e.handlers) == 0 {
		close(e.done)
		return nil
	}

	e.wg.Go(func() { e.reconcileLoop(workerCtx) })
	if e.config.RetentionInterval > 0 {
		e.wg.Go(func() { e.retentionLoop(workerCtx) })
	}
	for range e.config.WorkerCount {
		e.wg.Go(func() { e.workerLoop(workerCtx) })
	}
	go func() {
		e.wg.Wait()
		close(e.done)
	}()
	return nil
}

func (e *Engine) retentionLoop(ctx context.Context) {
	if _, err := e.pruneOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("domain event retention failed", "error", err)
	}
	ticker := time.NewTicker(e.config.RetentionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := e.pruneOnce(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					slog.Error("domain event retention failed", "error", err)
				}
				continue
			}
			if result.Deliveries > 0 || result.Events > 0 {
				slog.Info("domain event retention completed",
					"deliveries", result.Deliveries,
					"events", result.Events,
				)
			}
		}
	}
}

func (e *Engine) pruneOnce(ctx context.Context) (PruneResult, error) {
	now := e.now().UTC()
	return e.store.Prune(
		ctx,
		now.Add(-e.config.CompletedDeliveryRetention),
		now.Add(-e.config.EventRetention),
		e.config.RetentionBatch,
	)
}

// Append writes through a source transaction and wakes the reconciler.
func (e *Engine) Append(ctx context.Context, tx database.Tx, input NewEvent) (*Event, error) {
	event, err := e.store.Append(ctx, tx, input)
	if err == nil {
		e.signalReconcile()
	}
	return event, err
}

// AppendStandalone persists an external fact in its own transaction.
func (e *Engine) AppendStandalone(ctx context.Context, input NewEvent) (*Event, error) {
	event, err := e.store.AppendStandalone(ctx, input)
	if err == nil {
		e.signalReconcile()
	}
	return event, err
}

// Shutdown stops new claims and waits for workers until ctx expires.
func (e *Engine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	done := e.done
	if done == nil {
		e.mu.Unlock()
		return nil
	}
	if e.started {
		e.cancel()
		e.started = false
		e.cancel = nil
	}
	e.mu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown domain event engine: %w", ctx.Err())
	}
}

func (e *Engine) reconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(e.config.PollInterval)
	defer ticker.Stop()
	for {
		if err := e.reconcile(ctx); err != nil && ctx.Err() == nil {
			slog.Error("domain event reconciliation failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-e.reconcileWake:
		}
	}
}

func (e *Engine) reconcile(ctx context.Context) error {
	for {
		created, err := e.store.Reconcile(ctx, e.config.ReconcileBatch)
		if err != nil {
			return err
		}
		if created > 0 {
			e.signalWorkers()
		}
		if created < int64(e.config.ReconcileBatch) {
			return nil
		}
	}
}

func (e *Engine) workerLoop(ctx context.Context) {
	ticker := time.NewTicker(e.config.PollInterval)
	defer ticker.Stop()
	for {
		worked, err := e.processOne(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("domain event delivery failed", "error", err)
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-e.workerWake:
		}
	}
}

func (e *Engine) processOne(ctx context.Context) (bool, error) {
	keys := e.handlerKeys()
	for _, consumerKey := range keys {
		delivery, err := e.store.Claim(ctx, consumerKey, e.owner, e.now(), e.config.LeaseDuration)
		if err != nil {
			return false, err
		}
		if delivery == nil {
			continue
		}
		handler := e.handler(consumerKey)
		if handler == nil {
			return false, fmt.Errorf("domain event consumer %q has no handler", consumerKey)
		}
		return true, e.handle(ctx, handler, *delivery)
	}
	return false, nil
}

func (e *Engine) handle(ctx context.Context, handler Handler, delivery Delivery) error {
	handlerCtx, cancel := context.WithTimeout(ctx, e.config.HandlerTimeout)
	renewCtx, stopRenewing := context.WithCancel(context.Background())
	renewalDone := make(chan error, 1)
	go func() {
		renewalDone <- e.renewLease(renewCtx, delivery, cancel)
	}()
	err := handler.Handle(handlerCtx, delivery.Event)
	stopRenewing()
	renewalErr := <-renewalDone
	cancel()
	if renewalErr != nil {
		return fmt.Errorf("renew delivery %d/%s: %w", delivery.Event.ID, delivery.ConsumerKey, renewalErr)
	}
	if ctx.Err() != nil {
		// Leave the lease to expire so another process can reclaim it.
		return ctx.Err()
	}
	if err == nil {
		if err := e.store.Complete(ctx, delivery, e.now()); err != nil {
			return fmt.Errorf("complete delivery %d/%s: %w", delivery.Event.ID, delivery.ConsumerKey, err)
		}
		return nil
	}

	retry := !IsPermanent(err) && delivery.AttemptCount < e.config.MaxAttempts
	nextAttempt := e.now()
	if retry {
		nextAttempt = nextAttempt.Add(e.retryDelay(delivery.AttemptCount))
	}
	if failErr := e.store.Fail(ctx, delivery, err, retry, nextAttempt); failErr != nil {
		return fmt.Errorf("record delivery %d/%s failure: %w", delivery.Event.ID, delivery.ConsumerKey, failErr)
	}
	return nil
}

func (e *Engine) renewLease(ctx context.Context, delivery Delivery, cancelHandler context.CancelFunc) error {
	interval := e.config.LeaseDuration / 3
	if interval <= 0 {
		interval = time.Nanosecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			now := e.now().UTC()
			renewCtx, cancel := context.WithTimeout(context.Background(), interval)
			expiresAt, err := e.store.Renew(renewCtx, delivery, now, e.config.LeaseDuration)
			cancel()
			if err != nil {
				cancelHandler()
				return err
			}
			if e.leaseRenewed != nil {
				e.leaseRenewed(delivery, expiresAt)
			}
		}
	}
}

func (e *Engine) retryDelay(attempt int) time.Duration {
	shift := min(max(attempt-1, 0), 30)
	delay := e.config.BaseRetryDelay * time.Duration(1<<shift)
	if delay > e.config.MaxRetryDelay || delay < 0 {
		delay = e.config.MaxRetryDelay
	}
	jitter := e.jitter(delay)
	if delay > e.config.MaxRetryDelay-jitter {
		return e.config.MaxRetryDelay
	}
	return delay + jitter
}

func (e *Engine) handlerKeys() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	keys := make([]string, 0, len(e.handlers))
	for key := range e.handlers {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if len(keys) == 0 {
		return keys
	}
	start := e.nextHandler % len(keys)
	e.nextHandler = (start + 1) % len(keys)
	if start == 0 {
		return keys
	}
	return append(keys[start:], keys[:start]...)
}

func (e *Engine) handler(key string) Handler {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.handlers[key]
}

func (e *Engine) signalReconcile() {
	select {
	case e.reconcileWake <- struct{}{}:
	default:
	}
}

func (e *Engine) signalWorkers() {
	for range e.config.WorkerCount {
		select {
		case e.workerWake <- struct{}{}:
		default:
			return
		}
	}
}
