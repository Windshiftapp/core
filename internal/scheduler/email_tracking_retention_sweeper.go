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
	s.running = true
	s.ticker = time.NewTicker(s.interval)
	slog.Info("starting email tracking retention sweeper", "interval", s.interval, "default_days", s.defaultRetentionDays)
	go s.loop()
}

// Stop halts the sweeper. Safe to call multiple times.
func (s *EmailTrackingRetentionSweeper) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	s.ticker.Stop()
	close(s.stopChan)
	slog.Info("email tracking retention sweeper stopped")
}

func (s *EmailTrackingRetentionSweeper) loop() {
	s.tick()
	for {
		select {
		case <-s.ticker.C:
			s.tick()
		case <-s.stopChan:
			return
		}
	}
}

func (s *EmailTrackingRetentionSweeper) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	channels, err := s.collectInboundEmailChannels(ctx)
	if err != nil {
		slog.Error("retention sweeper: failed to list channels", "error", err)
		return
	}
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
		if deleted > 0 {
			slog.Info("retention sweeper: pruned tracking rows",
				"channel_id", ch.ID, "deleted", deleted, "retention_days", days)
		}
	}
}

type channelRetention struct {
	ID            int
	RetentionDays int
}

func (s *EmailTrackingRetentionSweeper) collectInboundEmailChannels(ctx context.Context) ([]channelRetention, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(config, '')
		FROM channels
		WHERE type = 'email' AND direction = 'inbound'
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []channelRetention
	for rows.Next() {
		var id int
		var configJSON string
		if err := rows.Scan(&id, &configJSON); err != nil {
			return nil, err
		}
		var days int
		if configJSON != "" {
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
// anchors (rows whose message_id is referenced by a more recent in_reply_to).
// Returns the number of rows deleted.
func (s *EmailTrackingRetentionSweeper) sweepChannel(ctx context.Context, channelID, retentionDays int) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM email_message_tracking
		WHERE channel_id = ?
		  AND processed_at < ?
		  AND (message_id = '' OR message_id NOT IN (
		      SELECT in_reply_to FROM email_message_tracking
		      WHERE channel_id = ? AND in_reply_to IS NOT NULL AND in_reply_to <> ''
		  ))
	`, channelID, cutoff, channelID)
	if err != nil {
		return 0, fmt.Errorf("delete: %w", err)
	}
	return res.RowsAffected()
}
