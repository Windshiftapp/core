package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// EmailTrackingRetentionSweeper trims old email_message_tracking rows on a
// daily cadence so the table doesn't grow unbounded on high-volume support
// mailboxes. Per-channel retention comes from
// ChannelConfig.EmailTrackingRetentionDays; 0 (or unset) means use
// defaultRetentionDays.
//
// Anchor preservation: a row whose message_id is referenced by a more recent
// row's in_reply_to is kept regardless of age, so future replies in a thread
// can still find their parent for comment routing.
type EmailTrackingRetentionSweeper struct {
	db database.Database

	ticker   *time.Ticker
	stopChan chan struct{}
	mu       sync.RWMutex
	running  bool

	interval             time.Duration
	defaultRetentionDays int
}

// NewEmailTrackingRetentionSweeper builds a sweeper with daily ticks and a
// 365-day default retention. Callers wire Start/Stop into the same lifecycle
// as the other in-process schedulers.
func NewEmailTrackingRetentionSweeper(db database.Database) *EmailTrackingRetentionSweeper {
	return &EmailTrackingRetentionSweeper{
		db:                   db,
		interval:             24 * time.Hour,
		defaultRetentionDays: 365,
		stopChan:             make(chan struct{}),
	}
}

// Start begins the daily sweep loop. Safe to call multiple times — second
// call is a no-op.
func (s *EmailTrackingRetentionSweeper) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.ticker = time.NewTicker(s.interval)
	s.stopChan = make(chan struct{})
	s.running = true
	slog.Info("starting email tracking retention sweeper", "interval", s.interval, "default_days", s.defaultRetentionDays)
	go s.loop(s.ticker, s.stopChan)
}

// Stop halts the sweeper. Safe to call multiple times.
func (s *EmailTrackingRetentionSweeper) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	close(s.stopChan)
	slog.Info("email tracking retention sweeper stopped")
}

func (s *EmailTrackingRetentionSweeper) loop(ticker *time.Ticker, stopChan <-chan struct{}) {
	s.tick()
	for {
		select {
		case <-ticker.C:
			s.tick()
		case <-stopChan:
			return
		}
	}
}

func (s *EmailTrackingRetentionSweeper) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := s.SweepNow(ctx); err != nil {
		slog.Error("retention sweeper: failed to list channels", "error", err)
	}
}

// SweepNow runs one sweep pass immediately and returns the number of pruned
// rows. Backs the daily tick and gives admin diagnostics a run-now hook.
func (s *EmailTrackingRetentionSweeper) SweepNow(ctx context.Context) (int64, error) {
	channels, err := s.collectInboundEmailChannels(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, ch := range channels {
		days := ch.RetentionDays
		if days <= 0 {
			days = s.defaultRetentionDays
		}
		deleted, err := s.sweepChannel(ctx, ch.ID, days)
		if err != nil {
			slog.Warn("retention sweeper: channel sweep failed", "channel_id", ch.ID, "error", err)
			continue
		}
		total += deleted
		if deleted > 0 {
			slog.Info("retention sweeper: pruned tracking rows",
				"channel_id", ch.ID, "deleted", deleted, "retention_days", days)
		}
	}
	return total, nil
}

type channelRetention struct {
	ID            int
	RetentionDays int
}

// collectInboundEmailChannels gathers the channels whose tracking rows the
// sweeper trims: inbound email channels (per-channel retention from config)
// plus portal channels, whose rows are system-generated thread anchors and
// outbound notification records (instance-default retention).
func (s *EmailTrackingRetentionSweeper) collectInboundEmailChannels(ctx context.Context) ([]channelRetention, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(config, ''), 'email' FROM channels
		WHERE type = 'email' AND direction = 'inbound'
		UNION ALL
		SELECT id, '', 'portal' FROM channels WHERE type = 'portal'
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []channelRetention
	for rows.Next() {
		var id int
		var configJSON, channelKind string
		if err := rows.Scan(&id, &configJSON, &channelKind); err != nil {
			return nil, err
		}
		days := 0
		if channelKind == "email" && configJSON != "" {
			var cfg models.ChannelConfig
			if jsonErr := json.Unmarshal([]byte(configJSON), &cfg); jsonErr == nil {
				days = cfg.EmailTrackingRetentionDays
			}
		}
		out = append(out, channelRetention{ID: id, RetentionDays: days})
	}
	return out, rows.Err()
}

// sweepChannel deletes tracking rows older than retentionDays except for
// thread anchors: rows whose message_id is referenced by a more recent
// in_reply_to, and synthetic portal anchors (outbound rows with no comment —
// they are the thread root for the ticket's whole lifetime). It also keeps
// completed rows whose item/comment was deleted (completed_at set, refs NULL):
// those are the only thing preventing the message from being recreated if the
// mailbox is refetched. Returns the number of rows deleted.
func (s *EmailTrackingRetentionSweeper) sweepChannel(ctx context.Context, channelID, retentionDays int) (int64, error) {
	const maxDays = int64(1<<63-1) / int64(24*time.Hour)
	if int64(retentionDays) > maxDays || int64(retentionDays) < -maxDays {
		return 0, fmt.Errorf("retention days out of range: %d", retentionDays)
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	// Delete in bounded batches so one sweep cannot hold the SQLite write
	// connection for the whole table. NOT EXISTS lets the planner use
	// idx_email_message_tracking_in_reply_to instead of materializing a NOT IN
	// list over every reply in the channel.
	const batchSize = 1000
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		res, err := s.db.ExecWriteContext(ctx, `
			DELETE FROM email_message_tracking
			WHERE id IN (
				SELECT t.id FROM email_message_tracking t
				WHERE t.channel_id = ?
				  AND t.processed_at < ?
				  AND (t.direction <> 'outbound' OR t.comment_id IS NOT NULL)
				  AND NOT (t.completed_at IS NOT NULL AND t.item_id IS NULL AND t.comment_id IS NULL)
				  AND (t.message_id = '' OR NOT EXISTS (
				      SELECT 1 FROM email_message_tracking r
				      WHERE r.channel_id = ? AND r.in_reply_to = t.message_id
				  ))
				LIMIT ?
			)
		`, channelID, cutoff, channelID, batchSize)
		if err != nil {
			return total, fmt.Errorf("delete: %w", err)
		}
		deleted, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("count deleted rows: %w", err)
		}
		total += deleted
		if deleted < batchSize {
			return total, nil
		}
	}
}
