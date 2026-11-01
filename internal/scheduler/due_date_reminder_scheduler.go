package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// DueDateReminderSchedulerConfig configures the background sweep that turns
// upcoming due dates into reminder notifications for users who opted in.
type DueDateReminderSchedulerConfig struct {
	// TickInterval is how often the sweep runs; default 1 hour.
	TickInterval time.Duration
	// BatchSize caps how many due items one sweep handles; default 500.
	BatchSize int
}

// DefaultDueDateReminderSchedulerConfig returns the built-in defaults.
func DefaultDueDateReminderSchedulerConfig() DueDateReminderSchedulerConfig {
	return DueDateReminderSchedulerConfig{
		TickInterval: time.Hour,
		BatchSize:    500,
	}
}

// DueDateReminderScheduler periodically creates a "reminder" notification for
// opted-in users about their open items whose due date is approaching. The
// reminder is never automatic: only users who enabled due-date reminders in
// their profile are notified, using their configured lead time. Assignees are
// reminded for regular items; personal-workspace items without an assignee
// remind their owner. Notification creation goes through the shared direct
// notification path, which re-checks recipient eligibility and workspace
// permission and dedupes on the delivery key; a per-(item, user, due date)
// ledger that outlives the notification row keeps a dismissed reminder from
// being recreated. Email rides the existing notification email batching.
type DueDateReminderScheduler struct {
	items         *repository.ItemRepository
	notifications *services.NotificationService
	config        DueDateReminderSchedulerConfig
	stopChan      chan struct{}
	wg            sync.WaitGroup
	startOnce     sync.Once
	stopOnce      sync.Once
	// now is overridable for tests; production uses wall-clock time.
	now func() time.Time
}

// NewDueDateReminderScheduler constructs the scheduler. Call Start() to begin.
func NewDueDateReminderScheduler(db database.Database, notifications *services.NotificationService, config DueDateReminderSchedulerConfig) *DueDateReminderScheduler {
	defaults := DefaultDueDateReminderSchedulerConfig()
	if config.TickInterval <= 0 {
		config.TickInterval = defaults.TickInterval
	}
	if config.BatchSize <= 0 {
		config.BatchSize = defaults.BatchSize
	}
	return &DueDateReminderScheduler{
		items:         repository.NewItemRepository(db),
		notifications: notifications,
		config:        config,
		stopChan:      make(chan struct{}),
		now:           time.Now,
	}
}

// Start launches the background sweep. It is safe to call more than once: only
// the first call starts a worker.
func (s *DueDateReminderScheduler) Start() {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.run()
		slog.Info("due-date reminder scheduler started",
			slog.String("component", "reminders"),
			slog.Duration("tick_interval", s.config.TickInterval),
		)
	})
}

// Stop signals shutdown and waits for the worker to drain. It is safe to call
// more than once; only the first call closes the stop channel.
func (s *DueDateReminderScheduler) Stop() {
	s.stopOnce.Do(func() { close(s.stopChan) })
	s.wg.Wait()
}

func (s *DueDateReminderScheduler) run() {
	defer s.wg.Done()
	// Sweep once at startup so a restart catches reminders that came due while
	// the process was down; the ledger makes the extra pass idempotent.
	s.RunOnce(context.Background(), s.now())

	t := time.NewTicker(s.config.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-t.C:
			s.RunOnce(context.Background(), s.now())
		}
	}
}

// RunOnce scans for opted-in recipients' due-soon items and creates their
// reminder notifications. Exported so tests can drive a deterministic pass
// with an injected clock.
func (s *DueDateReminderScheduler) RunOnce(ctx context.Context, now time.Time) {
	if s.notifications == nil {
		return
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	// The query applies each recipient's own lead time and skips items already
	// reminded, so every returned row is a candidate and the batch advances.
	horizon := today.AddDate(0, 0, services.MaxReminderDays)

	items, err := s.items.ListDueSoonItemsForReminderUsers(today, horizon, s.config.BatchSize)
	if err != nil {
		slog.Warn("due-date reminders: failed to list due items",
			slog.String("component", "reminders"), slog.Any("error", err))
		return
	}
	for i := range items {
		if err := ctx.Err(); err != nil {
			return
		}
		item := items[i]
		if err := s.notify(item, now); err != nil {
			slog.Warn("due-date reminders: failed to create reminder",
				slog.String("component", "reminders"),
				slog.Int("item_id", item.ItemID),
				slog.Any("error", err))
		}
	}
}

func (s *DueDateReminderScheduler) notify(item repository.DueSoonItem, now time.Time) error {
	if err := s.notifications.NotifyDueDateReminder(services.DueDateReminder{
		RecipientID: item.RecipientID,
		WorkspaceID: item.WorkspaceID,
		ItemID:      item.ItemID,
		ItemKey:     fmt.Sprintf("%s-%d", item.WorkspaceKey, item.ItemNumber),
		Title:       item.Title,
		DueDate:     item.DueDate,
	}, now); err != nil {
		return err
	}
	// Record the ledger even when the recipient was filtered out (inactive,
	// agent, or no workspace access) so the candidate cannot consume a batch
	// slot on every sweep. A failed insert returns before this point, so the
	// reminder is retried; the delivery key keeps the retry from duplicating it.
	return s.items.RecordDueDateReminder(item.ItemID, item.RecipientID, item.DueDate.Format("2006-01-02"))
}
