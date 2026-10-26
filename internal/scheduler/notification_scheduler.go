package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// defaultBatchInterval is the production cadence: every 5 minutes the
// scheduler scans for unread notifications and emails one batch per user.
// The interval is supplied by the caller (resolved from config, which reads
// WINDSHIFT_NOTIFICATION_BATCH_INTERVAL); a non-positive value falls back here.
const defaultBatchInterval = 5 * time.Minute

// maxBatchSize caps how many notifications one email can carry. Without this,
// a user with a thousand unread notifications gets a single oversize email on
// every 5-minute tick (and many SMTP servers will reject it).
const maxBatchSize = 50

// maxConsecutiveFailures is the per-user circuit breaker. After K failed
// sends in a row, we stop trying and let the user's notifications accumulate
// until the cooldown expires, instead of re-flooding the SMTP server with
// a growing batch every tick.
const maxConsecutiveFailures = 5

// failureCooldown is how long to skip a user after hitting the failure cap.
const failureCooldown = 30 * time.Minute

// notificationClaimLease allows another scheduler instance to recover a batch
// abandoned before or during SMTP delivery.
const notificationClaimLease = 15 * time.Minute

// NotificationScheduler handles batching and sending of notifications every 5 minutes
type NotificationScheduler struct {
	db            database.Database
	ticker        *time.Ticker
	stopChan      chan struct{}
	mu            sync.RWMutex
	running       bool
	stopping      bool
	smtpSender    SMTPSender
	runRepo       *repository.SchedulerRunRepository
	notifSvc      *services.NotificationService
	batchInterval time.Duration
	// tickBudget bounds one digest tick. Without it a slow SMTP server can keep
	// a tick running for hours across the recipient cap.
	tickBudget time.Duration
	// outboxBatchSize is how many reply/mention outbox rows the digest's
	// backstop pass claims. Set from WINDSHIFT_EMAIL_OUTBOX_BATCH_SIZE.
	outboxBatchSize int
	replyOutbox     emailReplyOutbox
	mentionOutbox   mentionEmailOutbox
	claimOwner      string
	claimLease      time.Duration
	now             func() time.Time
	runCancel       context.CancelFunc
	runDone         chan struct{}
	runNow          chan chan struct{}

	// Per-user failure tracking for the circuit breaker. Keyed by user email
	// (matching how we fan-out batches today). Resets on restart — the goal
	// is to prevent hot-loop flooding within a process, not to persist state.
	failMu     sync.Mutex
	failCounts map[string]int
	skipUntil  map[string]time.Time
}

// SMTPSender interface for sending emails
type SMTPSender interface {
	SendBatchedNotificationsContext(ctx context.Context, userEmail, userName string, notifications []models.Notification) error
	IsSMTPConfigured() bool
}

type emailReplyOutbox interface {
	ProcessPendingReplies(ctx context.Context, limit int) (int, error)
}

type mentionEmailOutbox interface {
	ProcessPendingMentions(ctx context.Context, limit int) (int, error)
}

// SetEmailReplyOutbox wires the durable threaded-reply queue after the
// comment service is constructed. The scheduler may already be running.
func (ns *NotificationScheduler) SetEmailReplyOutbox(outbox emailReplyOutbox) {
	ns.mu.Lock()
	ns.replyOutbox = outbox
	ns.mu.Unlock()
}

// SetMentionEmailOutbox wires the durable @mention email queue. It runs
// before the digest claims unread notifications so a delivered mention email
// suppresses the duplicate digest entry.
func (ns *NotificationScheduler) SetMentionEmailOutbox(outbox mentionEmailOutbox) {
	ns.mu.Lock()
	ns.mentionOutbox = outbox
	ns.mu.Unlock()
}

// SetTickBudget overrides how long one digest tick may keep sending. A
// non-positive value keeps the current setting.
func (ns *NotificationScheduler) SetTickBudget(budget time.Duration) {
	if budget <= 0 {
		return
	}
	ns.mu.Lock()
	ns.tickBudget = budget
	ns.mu.Unlock()
}

// SetOutboxBatchSize overrides how many outbox rows the digest's backstop
// pass claims. A non-positive value keeps the built-in default.
func (ns *NotificationScheduler) SetOutboxBatchSize(size int) {
	if size <= 0 {
		return
	}
	ns.mu.Lock()
	ns.outboxBatchSize = size
	ns.mu.Unlock()
}

// NewNotificationScheduler creates a new notification scheduler. batchInterval
// is the tick cadence (resolved from config); a non-positive value falls back
// to defaultBatchInterval. notifSvc supplies the unread-notification batches so
// the scheduler never queries the notifications table directly.
func NewNotificationScheduler(db database.Database, smtpSender SMTPSender, batchInterval time.Duration, notifSvc *services.NotificationService) *NotificationScheduler {
	if batchInterval <= 0 {
		batchInterval = defaultBatchInterval
	}
	scheduler := &NotificationScheduler{
		db:              db,
		stopChan:        make(chan struct{}),
		running:         false,
		smtpSender:      smtpSender,
		runRepo:         repository.NewSchedulerRunRepository(db),
		notifSvc:        notifSvc,
		batchInterval:   batchInterval,
		tickBudget:      batchInterval,
		failCounts:      make(map[string]int),
		skipUntil:       make(map[string]time.Time),
		claimLease:      notificationClaimLease,
		now:             time.Now,
		runNow:          make(chan chan struct{}),
		outboxBatchSize: defaultOutboxBatchSize,
	}
	scheduler.claimOwner = fmt.Sprintf("notification-scheduler-%d-%p", time.Now().UnixNano(), scheduler)
	// Hold users in an SMTP failure cooldown out of the per-tick recipient scan
	// instead of claiming and releasing their rows every tick (WI-1768).
	if notifSvc != nil {
		notifSvc.SetRecipientEligibility(func(email string) bool { return !scheduler.inCooldown(email) })
	}
	return scheduler
}

// Start begins the notification batching scheduler
func (ns *NotificationScheduler) Start() {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if ns.running {
		return
	}

	ns.ticker = time.NewTicker(ns.batchInterval)
	ns.stopChan = make(chan struct{})
	runCtx, cancel := context.WithCancel(context.Background())
	ns.runCancel = cancel
	ns.runDone = make(chan struct{})
	ns.running = true
	ns.stopping = false
	slog.Debug("Starting notification scheduler", slog.String("component", "scheduler"), slog.String("interval", ns.batchInterval.String()))

	go ns.schedulerLoop(runCtx, ns.ticker, ns.stopChan, ns.runDone)
}

// Stop stops the notification scheduler
func (ns *NotificationScheduler) Stop() {
	ns.mu.Lock()
	if !ns.running {
		ns.mu.Unlock()
		return
	}
	if ns.stopping {
		done := ns.runDone
		ns.mu.Unlock()
		<-done
		return
	}
	ns.stopping = true
	if ns.ticker != nil {
		ns.ticker.Stop()
	}
	if ns.runCancel != nil {
		ns.runCancel()
	}
	close(ns.stopChan)
	done := ns.runDone
	ns.mu.Unlock()

	<-done

	ns.mu.Lock()
	ns.running = false
	ns.stopping = false
	ns.ticker = nil
	ns.runCancel = nil
	ns.runDone = nil
	ns.mu.Unlock()
	slog.Debug("Notification scheduler stopped", slog.String("component", "scheduler"))
}

// schedulerLoop runs the main scheduler loop

func (ns *NotificationScheduler) schedulerLoop(ctx context.Context, ticker *time.Ticker, stopChan <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ticker.C:
			ns.processPendingNotificationsContext(ctx)
		case runComplete := <-ns.runNow:
			ns.processPendingNotificationsContext(ctx)
			close(runComplete)
		case <-stopChan:
			return
		}
	}
}

func (ns *NotificationScheduler) processPendingNotificationsContext(ctx context.Context) {
	start := ns.now()
	var batchesProcessed int
	var runErr error
	defer recordSchedulerRun(ns.runRepo, "notification", start, &batchesProcessed, &runErr, ns.now)

	// Check if SMTP is configured first
	if !ns.smtpSender.IsSMTPConfigured() {
		slog.Debug("SMTP not configured, skipping notification batch processing", slog.String("component", "scheduler"))
		return
	}
	if err := ctx.Err(); err != nil {
		runErr = err
		return
	}

	slog.Debug("Processing notification batches", slog.String("component", "scheduler"))

	ns.mu.RLock()
	replyOutbox := ns.replyOutbox
	mentionOutbox := ns.mentionOutbox
	outboxBatchSize := ns.outboxBatchSize
	ns.mu.RUnlock()
	if replyOutbox != nil {
		delivered, err := replyOutbox.ProcessPendingReplies(ctx, outboxBatchSize)
		batchesProcessed += delivered
		if err != nil {
			slog.Error("Failed to process pending threaded email replies", slog.Any("error", err))
			runErr = err
		}
	}
	// Deliver mention emails before claiming the digest so a successful send
	// marks the matching notification sent and the digest skips it.
	if mentionOutbox != nil {
		delivered, err := mentionOutbox.ProcessPendingMentions(ctx, outboxBatchSize)
		batchesProcessed += delivered
		if err != nil {
			slog.Error("Failed to process pending mention emails", slog.Any("error", err))
			runErr = err
		}
	}

	// Select the recipient batches without claiming them. Each batch is claimed
	// immediately before its SMTP send so a slow tick cannot let an unsent
	// batch's lease expire and be duplicated by another instance (WI-1773).
	userBatches, err := ns.notifSvc.UnreadEmailBatchesAt(maxBatchSize, ns.now())
	if err != nil {
		slog.Error("Failed to get unread notifications", slog.String("component", "scheduler"), slog.Any("error", err))
		runErr = err
		return
	}

	if len(userBatches) == 0 {
		slog.Debug("No unread notifications to process", slog.String("component", "scheduler"))
		return
	}
	batchesProcessed += len(userBatches)

	ns.mu.RLock()
	tickBudget := ns.tickBudget
	ns.mu.RUnlock()
	tickCtx := ctx
	if tickBudget > 0 {
		var cancel context.CancelFunc
		tickCtx, cancel = context.WithTimeout(ctx, tickBudget)
		defer cancel()
	}

	// Send batches for each user. Track failures so the deferred recordSchedulerRun
	// can mark this tick failed when at least one batch couldn't be sent — otherwise
	// the admin Diagnostics page reports "100% success rate" while users miss email.
	failures := 0
	for userEmail, batch := range userBatches {
		if tickCtx.Err() != nil {
			// Nothing is claimed yet, so there is nothing to release.
			continue
		}
		if ns.inCooldown(userEmail) {
			slog.Debug("skipping user in failure cooldown",
				slog.String("component", "scheduler"),
				slog.String("user_email", userEmail),
			)
			continue
		}
		claim, err := ns.notifSvc.ClaimNotificationEmailBatch(batch, ns.claimOwner, ns.now(), ns.claimLease)
		if err != nil {
			slog.Error("failed to claim notification email batch",
				slog.String("component", "scheduler"),
				slog.String("user_email", userEmail),
				slog.Any("error", err),
			)
			failures++
			continue
		}
		if claim == nil {
			// Another instance claimed this batch first.
			continue
		}
		deliverable, err := ns.notifSvc.NotificationEmailClaimDeliverable(claim)
		if err != nil {
			slog.Error("failed to reauthorize claimed notification email batch",
				slog.String("component", "scheduler"),
				slog.String("user_email", userEmail),
				slog.Any("error", err),
			)
			failures++
			continue
		}
		if !deliverable {
			if err := ns.notifSvc.ReleaseNotificationEmailClaim(claim, ns.now()); err != nil {
				slog.Error("failed to release ineligible notification email batch",
					slog.String("component", "scheduler"),
					slog.String("user_email", userEmail),
					slog.Any("error", err),
				)
				failures++
			}
			continue
		}
		if err := ns.sendNotificationBatch(tickCtx, claim.UserNotificationBatch); err != nil {
			if tickCtx.Err() != nil {
				// SMTP acceptance may be ambiguous after cancellation. Keep the
				// claim fenced until expiry so restart cannot overlap it immediately.
				runErr = tickCtx.Err()
				failures++
				continue
			}
			slog.Error("Failed to send notification batch", slog.String("component", "scheduler"), slog.String("user_email", userEmail), slog.Any("error", err))
			if claimErr := ns.notifSvc.FailNotificationEmailClaim(claim, err, ns.now()); claimErr != nil {
				slog.Error("failed to record notification email failure; claim will recover after expiry",
					slog.String("component", "scheduler"),
					slog.String("user_email", userEmail),
					slog.Any("error", claimErr),
				)
			}
			ns.recordFailure(userEmail)
			failures++
			continue
		}
		if err := ns.notifSvc.CompleteNotificationEmailClaim(claim, ns.now()); err != nil {
			// SMTP may already have accepted the message. Leave the fenced claim
			// intact; expiry permits recovery under the documented at-least-once
			// policy without allowing this worker to rewrite newer ownership.
			slog.Error("failed to complete notification email claim after SMTP acceptance",
				slog.String("component", "scheduler"),
				slog.String("user_email", userEmail),
				slog.Any("error", err),
			)
			failures++
			continue
		}
		ns.recordSuccess(userEmail)
	}

	if failures > 0 && runErr == nil {
		runErr = fmt.Errorf("%d of %d notification batches failed", failures, len(userBatches))
	}

	slog.Debug("Processed notification batches", slog.String("component", "scheduler"), slog.Int("batch_count", len(userBatches)))
}

// inCooldown reports whether the user is currently being skipped because of
// prior consecutive SMTP failures.
func (ns *NotificationScheduler) inCooldown(userEmail string) bool {
	ns.failMu.Lock()
	defer ns.failMu.Unlock()
	until, ok := ns.skipUntil[userEmail]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	// Cooldown expired — clear state so the user gets another shot.
	delete(ns.skipUntil, userEmail)
	delete(ns.failCounts, userEmail)
	return false
}

func (ns *NotificationScheduler) recordFailure(userEmail string) {
	ns.failMu.Lock()
	defer ns.failMu.Unlock()
	ns.failCounts[userEmail]++
	if ns.failCounts[userEmail] >= maxConsecutiveFailures {
		ns.skipUntil[userEmail] = time.Now().Add(failureCooldown)
		slog.Warn("tripping notification cooldown for user after repeated SMTP failures",
			slog.String("component", "scheduler"),
			slog.String("user_email", userEmail),
			slog.Duration("cooldown", failureCooldown),
		)
	}
}

func (ns *NotificationScheduler) recordSuccess(userEmail string) {
	ns.failMu.Lock()
	defer ns.failMu.Unlock()
	delete(ns.failCounts, userEmail)
	delete(ns.skipUntil, userEmail)
}

// sendNotificationBatch sends a batch of notifications to a user
func (ns *NotificationScheduler) sendNotificationBatch(ctx context.Context, batch *services.UserNotificationBatch) error {
	if len(batch.Notifications) == 0 {
		return nil
	}

	return ns.smtpSender.SendBatchedNotificationsContext(ctx, batch.UserEmail, batch.UserName, batch.Notifications)
}
