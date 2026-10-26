package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
)

const (
	// defaultOutboxInterval is the dedicated outbound email pool cadence. It is
	// deliberately faster than the 5-minute notification digest: a portal reply
	// or @mention email should reach the customer within about a minute, not on
	// the digest schedule. Resolved from WINDSHIFT_EMAIL_OUTBOX_INTERVAL.
	defaultOutboxInterval = time.Minute

	// defaultOutboxBatchSize is how many outbox rows one drain pass claims.
	// Resolved from WINDSHIFT_EMAIL_OUTBOX_BATCH_SIZE.
	defaultOutboxBatchSize = 100

	// defaultOutboxDrainBudget bounds how long one tick keeps draining a
	// backlog. The loop stops early once both queues report fewer than one
	// batch, so the budget only matters under sustained load.
	defaultOutboxDrainBudget = time.Minute

	// outboundEmailSchedulerName is the scheduler_runs key shown in admin
	// diagnostics.
	outboundEmailSchedulerName = "email_outbox"
)

// OutboundEmailScheduler drains the durable threaded-reply and @mention
// outboxes on a fast, bounded cadence. Sending is separated from the
// 5-minute notification digest because the digest intentionally batches
// per-user email, while a customer reply is a one-to-one message that should
// go out promptly. Both queues are idempotent and row-leased, so this
// scheduler and the digest's backstop pass can never double-send.
type OutboundEmailScheduler struct {
	db          database.Database
	runRepo     *repository.SchedulerRunRepository
	smtpSender  SMTPSender
	interval    time.Duration
	batchSize   int
	drainBudget time.Duration
	now         func() time.Time

	mu            sync.RWMutex
	replyOutbox   emailReplyOutbox
	mentionOutbox mentionEmailOutbox

	ticker    *time.Ticker
	stopChan  chan struct{}
	running   bool
	stopping  bool
	runCancel context.CancelFunc
	runDone   chan struct{}
}

// NewOutboundEmailScheduler creates the outbound email pool. A non-positive
// interval or batch size falls back to the built-in defaults.
func NewOutboundEmailScheduler(db database.Database, smtpSender SMTPSender, interval time.Duration, batchSize int) *OutboundEmailScheduler {
	if interval <= 0 {
		interval = defaultOutboxInterval
	}
	if batchSize <= 0 {
		batchSize = defaultOutboxBatchSize
	}
	return &OutboundEmailScheduler{
		db:          db,
		runRepo:     repository.NewSchedulerRunRepository(db),
		smtpSender:  smtpSender,
		interval:    interval,
		batchSize:   batchSize,
		drainBudget: defaultOutboxDrainBudget,
		now:         time.Now,
		stopChan:    make(chan struct{}),
	}
}

// SetEmailReplyOutbox wires the durable threaded-reply queue. The scheduler
// may already be running.
func (os *OutboundEmailScheduler) SetEmailReplyOutbox(outbox emailReplyOutbox) {
	os.mu.Lock()
	os.replyOutbox = outbox
	os.mu.Unlock()
}

// SetMentionEmailOutbox wires the durable @mention email queue.
func (os *OutboundEmailScheduler) SetMentionEmailOutbox(outbox mentionEmailOutbox) {
	os.mu.Lock()
	os.mentionOutbox = outbox
	os.mu.Unlock()
}

// Start begins the outbound email pool.
func (os *OutboundEmailScheduler) Start() {
	os.mu.Lock()
	defer os.mu.Unlock()

	if os.running {
		return
	}

	os.ticker = time.NewTicker(os.interval)
	os.stopChan = make(chan struct{})
	runCtx, cancel := context.WithCancel(context.Background())
	os.runCancel = cancel
	os.runDone = make(chan struct{})
	os.running = true
	os.stopping = false
	slog.Debug("Starting outbound email scheduler",
		slog.String("component", "scheduler"),
		slog.String("interval", os.interval.String()),
		slog.Int("batch_size", os.batchSize),
	)

	go os.schedulerLoop(runCtx, os.ticker, os.stopChan, os.runDone)
}

// Stop stops the outbound email pool and waits for the in-flight drain.
func (os *OutboundEmailScheduler) Stop() {
	os.mu.Lock()
	if !os.running {
		os.mu.Unlock()
		return
	}
	if os.stopping {
		done := os.runDone
		os.mu.Unlock()
		<-done
		return
	}
	os.stopping = true
	if os.ticker != nil {
		os.ticker.Stop()
	}
	if os.runCancel != nil {
		os.runCancel()
	}
	close(os.stopChan)
	done := os.runDone
	os.mu.Unlock()

	<-done

	os.mu.Lock()
	os.running = false
	os.stopping = false
	os.ticker = nil
	os.runCancel = nil
	os.runDone = nil
	os.mu.Unlock()
	slog.Debug("Outbound email scheduler stopped", slog.String("component", "scheduler"))
}

func (os *OutboundEmailScheduler) schedulerLoop(ctx context.Context, ticker *time.Ticker, stopChan <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ticker.C:
			os.processOutboundEmails(ctx)
		case <-stopChan:
			return
		}
	}
}

// processOutboundEmails drains both outboxes until they fall below one batch
// or the drain budget/context expires, then records one scheduler run.
func (os *OutboundEmailScheduler) processOutboundEmails(ctx context.Context) {
	start := os.now()
	var delivered int
	var runErr error
	defer recordSchedulerRun(os.runRepo, outboundEmailSchedulerName, start, &delivered, &runErr, os.now)

	if os.smtpSender != nil && !os.smtpSender.IsSMTPConfigured() {
		slog.Debug("SMTP not configured, skipping outbound email drain", slog.String("component", "scheduler"))
		return
	}
	if err := ctx.Err(); err != nil {
		runErr = err
		return
	}

	os.mu.RLock()
	replyOutbox := os.replyOutbox
	mentionOutbox := os.mentionOutbox
	os.mu.RUnlock()

	deadline := start.Add(os.drainBudget)
	// Pass a deadline into each outbox pass so one hanging SMTP send cannot
	// outlive the drain budget (WI-1772). The outbox loop checks the context
	// between rows and the sender propagates it through dial, auth, and DATA.
	drainCtx, cancel := context.WithTimeout(ctx, os.drainBudget)
	defer cancel()
	for {
		if err := drainCtx.Err(); err != nil {
			if ctx.Err() == nil && !errors.Is(err, context.DeadlineExceeded) {
				runErr = err
			}
			break
		}
		if os.now().After(deadline) {
			break
		}

		pass := 0
		if replyOutbox != nil {
			sent, err := replyOutbox.ProcessPendingReplies(drainCtx, os.batchSize)
			pass += sent
			if err != nil {
				slog.Error("failed to process pending threaded email replies", slog.Any("error", err))
				runErr = err
			}
		}
		if mentionOutbox != nil {
			sent, err := mentionOutbox.ProcessPendingMentions(drainCtx, os.batchSize)
			pass += sent
			if err != nil {
				slog.Error("failed to process pending mention emails", slog.Any("error", err))
				runErr = err
			}
		}
		delivered += pass

		// Fewer than one full batch across both queues means nothing was left
		// behind the limit; stop rather than re-query empty tables.
		if pass < os.batchSize {
			break
		}
	}

	if delivered > 0 {
		slog.Debug("drained outbound email outboxes",
			slog.String("component", "scheduler"),
			slog.Int("delivered", delivered),
			slog.Duration("budget", os.drainBudget),
		)
	}
	if runErr != nil && delivered > 0 {
		runErr = fmt.Errorf("delivered %d before error: %w", delivered, runErr)
	}
}
