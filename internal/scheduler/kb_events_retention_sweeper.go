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
	"windshift/internal/repository"
)

// DefaultKBEventsRetentionDays is the instance default for kb_events
// retention. Portal channels can override it per channel via
// ChannelConfig.KBEventsRetentionDays; the instance default itself is
// overridable via the system_settings key kb_events_retention_days.
const (
	DefaultKBEventsRetentionDays       = 365
	systemSettingKBEventsRetentionDays = "kb_events_retention_days"
)

// KBEventsRetentionSweeper trims old kb_events rows on a daily cadence. The
// rows tie search/view/deflection analytics to portal customers, so
// unbounded retention keeps behavioral profiles alive for no operational
// gain (WI-1552). Mirrors the email tracking retention sweeper: per-channel
// retention from ChannelConfig.KBEventsRetentionDays, 0 (or unset) means the
// instance default.
type KBEventsRetentionSweeper struct {
	db      database.Database
	runRepo *repository.SchedulerRunRepository

	ticker   *time.Ticker
	stopChan chan struct{}
	mu       sync.RWMutex
	running  bool

	interval             time.Duration
	defaultRetentionDays int
}

// NewKBEventsRetentionSweeper builds a sweeper with daily ticks. Callers wire
// Start/Stop into the same lifecycle as the other in-process schedulers.
func NewKBEventsRetentionSweeper(db database.Database) *KBEventsRetentionSweeper {
	return &KBEventsRetentionSweeper{
		db:                   db,
		runRepo:              repository.NewSchedulerRunRepository(db),
		interval:             24 * time.Hour,
		defaultRetentionDays: DefaultKBEventsRetentionDays,
		stopChan:             make(chan struct{}),
	}
}

// Start begins the daily sweep loop. Safe to call multiple times — second
// call is a no-op.
func (s *KBEventsRetentionSweeper) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.ticker = time.NewTicker(s.interval)
	s.stopChan = make(chan struct{})
	s.running = true
	slog.Info("starting kb events retention sweeper", "interval", s.interval, "default_days", s.defaultRetentionDays)
	go s.loop(s.ticker, s.stopChan)
}

// Stop halts the sweeper. Safe to call multiple times.
func (s *KBEventsRetentionSweeper) Stop() {
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
	slog.Info("kb events retention sweeper stopped")
}

func (s *KBEventsRetentionSweeper) loop(ticker *time.Ticker, stopChan <-chan struct{}) {
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

func (s *KBEventsRetentionSweeper) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()
	var runErr error
	items := 0
	defer recordSchedulerRun(s.runRepo, "kb_events_retention", start, &items, &runErr, time.Now)

	defaultDays := s.defaultRetentionDays
	if configured := readRetentionDaysSetting(ctx, s.db, systemSettingKBEventsRetentionDays, defaultDays); configured != DefaultKBEventsRetentionDays {
		// An explicit instance default replaces the built-in one; per-channel
		// zeros still fall back to it.
		defaultDays = configured
	}

	channels, err := s.collectPortalChannels(ctx)
	if err != nil {
		runErr = fmt.Errorf("list portal channels: %w", err)
		slog.Error("kb events retention sweeper: failed to list channels", "error", runErr)
		return
	}
	for _, ch := range channels {
		days := ch.RetentionDays
		if days <= 0 {
			days = defaultDays
		}
		deleted, err := s.sweepChannel(ctx, ch.ID, days)
		if err != nil {
			slog.Warn("kb events retention sweeper: channel sweep failed", "channel_id", ch.ID, "error", err)
			continue
		}
		items += int(deleted)
		if deleted > 0 {
			slog.Info("kb events retention sweeper: pruned analytics rows",
				"channel_id", ch.ID, "deleted", deleted, "retention_days", days)
		}
	}
}

type kbChannelRetention struct {
	ID            int
	RetentionDays int
}

func (s *KBEventsRetentionSweeper) collectPortalChannels(ctx context.Context) ([]kbChannelRetention, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(config, '')
		FROM channels
		WHERE type = 'portal'
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []kbChannelRetention
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
				days = cfg.KBEventsRetentionDays
			}
		}
		out = append(out, kbChannelRetention{ID: id, RetentionDays: days})
	}
	return out, rows.Err()
}

// sweepChannel deletes the channel's kb_events older than retentionDays.
// Returns the number of rows deleted.
func (s *KBEventsRetentionSweeper) sweepChannel(ctx context.Context, channelID, retentionDays int) (int64, error) {
	const maxDays = int64(1<<63-1) / int64(24*time.Hour)
	if int64(retentionDays) > maxDays || int64(retentionDays) < -maxDays {
		return 0, fmt.Errorf("retention days out of range: %d", retentionDays)
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	res, err := s.db.ExecWriteContext(ctx, `
		DELETE FROM kb_events
		WHERE channel_id = ? AND created_at < ?
	`, channelID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete: %w", err)
	}
	return res.RowsAffected()
}
