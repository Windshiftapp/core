package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// Default retention windows for the portal auth retention sweeper. Operators
// can override both via system_settings keys portal_session_retention_days
// and portal_magic_link_retention_days (integer day counts; unparsable or
// non-positive values fall back to these defaults).
const (
	DefaultPortalSessionRetentionDays   = 30
	DefaultPortalMagicLinkRetentionDays = 30

	systemSettingSessionRetentionDays   = "portal_session_retention_days"
	systemSettingMagicLinkRetentionDays = "portal_magic_link_retention_days"
)

// PortalAuthRetentionSweeper purges expired portal customer sessions and
// consumed/expired magic links once they have aged past their retention
// window. Both tables carry personal data (session ip_address/user_agent,
// magic-link tokens) that serves no purpose once the credential is dead, and
// for live customers nothing else removes old rows (WI-1552; erasure only
// covers erased customers).
//
// Grace semantics: rows are purged strictly after expires_at/used_at has
// passed the retention window, so live sessions and recent activity are never
// touched.
type PortalAuthRetentionSweeper struct {
	db      database.Database
	runRepo *repository.SchedulerRunRepository

	ticker   *time.Ticker
	stopChan chan struct{}
	mu       sync.RWMutex
	running  bool

	interval time.Duration
}

// NewPortalAuthRetentionSweeper builds a sweeper with daily ticks. Callers
// wire Start/Stop into the same lifecycle as the other in-process schedulers.
func NewPortalAuthRetentionSweeper(db database.Database) *PortalAuthRetentionSweeper {
	return &PortalAuthRetentionSweeper{
		db:       db,
		runRepo:  repository.NewSchedulerRunRepository(db),
		interval: 24 * time.Hour,
		stopChan: make(chan struct{}),
	}
}

// Start begins the daily sweep loop. Safe to call multiple times — second
// call is a no-op.
func (s *PortalAuthRetentionSweeper) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.ticker = time.NewTicker(s.interval)
	s.stopChan = make(chan struct{})
	s.running = true
	slog.Info("starting portal auth retention sweeper", "interval", s.interval)
	go s.loop(s.ticker, s.stopChan)
}

// Stop halts the sweeper. Safe to call multiple times.
func (s *PortalAuthRetentionSweeper) Stop() {
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
	slog.Info("portal auth retention sweeper stopped")
}

func (s *PortalAuthRetentionSweeper) loop(ticker *time.Ticker, stopChan <-chan struct{}) {
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

func (s *PortalAuthRetentionSweeper) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()
	var runErr error
	items := 0
	defer recordSchedulerRun(s.runRepo, "portal_auth_retention", start, &items, &runErr, time.Now)

	sessionDays := readRetentionDaysSetting(ctx, s.db, systemSettingSessionRetentionDays, DefaultPortalSessionRetentionDays)
	linkDays := readRetentionDaysSetting(ctx, s.db, systemSettingMagicLinkRetentionDays, DefaultPortalMagicLinkRetentionDays)

	sessionCutoff := time.Now().Add(-time.Duration(sessionDays) * 24 * time.Hour)
	res, err := s.db.ExecWriteContext(ctx, `
		DELETE FROM portal_customer_sessions WHERE expires_at < ?
	`, sessionCutoff)
	if err != nil {
		runErr = fmt.Errorf("purge sessions: %w", err)
		slog.Error("portal auth retention sweeper: session purge failed", "error", runErr)
		return
	}
	deletedSessions, _ := res.RowsAffected()

	linkCutoff := time.Now().Add(-time.Duration(linkDays) * 24 * time.Hour)
	res, err = s.db.ExecWriteContext(ctx, `
		DELETE FROM portal_customer_magic_links
		WHERE (used_at IS NOT NULL AND used_at < ?) OR expires_at < ?
	`, linkCutoff, linkCutoff)
	if err != nil {
		runErr = fmt.Errorf("purge magic links: %w", err)
		slog.Error("portal auth retention sweeper: magic link purge failed", "error", runErr)
		items = int(deletedSessions)
		return
	}
	deletedLinks, _ := res.RowsAffected()

	items = int(deletedSessions + deletedLinks)
	if items > 0 {
		slog.Info("portal auth retention sweeper: pruned credential rows",
			"sessions", deletedSessions, "magic_links", deletedLinks,
			"session_retention_days", sessionDays, "magic_link_retention_days", linkDays)
	}
}

// readRetentionDaysSetting reads an integer day-count from system_settings,
// falling back to def when the key is absent, unparsable, or non-positive.
func readRetentionDaysSetting(ctx context.Context, db database.Database, key string, def int) int {
	var value string
	if err := db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = ?`, key).Scan(&value); err != nil {
		return def
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return def
	}
	return parsed
}
