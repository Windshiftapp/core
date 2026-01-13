package services

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/database"
)

// TokenTrackerConfig represents configuration for the token tracker
type TokenTrackerConfig struct {
	FlushInterval time.Duration `json:"flush_interval"` // Default: 5min
}

// DefaultTokenTrackerConfig returns default configuration
func DefaultTokenTrackerConfig() TokenTrackerConfig {
	return TokenTrackerConfig{
		FlushInterval: 5 * time.Minute,
	}
}

// TokenTracker handles batched updates of API token last_used_at timestamps
// This prevents database write contention by buffering updates and flushing periodically
type TokenTracker struct {
	db     database.Database
	config TokenTrackerConfig

	// Pending token updates (buffered for batch write)
	pendingTokens map[int]time.Time // tokenID -> last_used_at
	pendingMu     sync.RWMutex

	// Statistics
	updates int64
	flushes int64
	errors  int64

	// Flush ticker
	flushTicker *time.Ticker
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewTokenTracker creates a new token tracker service
func NewTokenTracker(db database.Database, config TokenTrackerConfig) *TokenTracker {
	tracker := &TokenTracker{
		db:            db,
		config:        config,
		pendingTokens: make(map[int]time.Time),
		flushTicker:   time.NewTicker(config.FlushInterval),
		stopChan:      make(chan struct{}),
	}

	// Start periodic flush goroutine
	tracker.wg.Add(1)
	go tracker.periodicFlush()

	slog.Debug("TokenTracker initialized", slog.String("component", "tokens"), slog.Duration("flush_interval", config.FlushInterval))

	return tracker
}

// RecordTokenUse marks a token as used (buffers for batch write)
// This method is safe to call concurrently from multiple goroutines
func (tt *TokenTracker) RecordTokenUse(tokenID int) {
	tt.pendingMu.Lock()
	tt.pendingTokens[tokenID] = time.Now()
	tt.pendingMu.Unlock()

	atomic.AddInt64(&tt.updates, 1)
}

// periodicFlush runs in background goroutine
func (tt *TokenTracker) periodicFlush() {
	defer tt.wg.Done()

	for {
		select {
		case <-tt.flushTicker.C:
			if err := tt.FlushPendingUpdates(); err != nil {
				slog.Error("Error flushing pending token updates", slog.String("component", "tokens"), slog.Any("error", err))
			}
		case <-tt.stopChan:
			slog.Debug("Stopping token tracker periodic flush", slog.String("component", "tokens"))
			return
		}
	}
}

// FlushPendingUpdates writes all buffered updates to database
func (tt *TokenTracker) FlushPendingUpdates() error {
	tt.pendingMu.Lock()

	// Copy and clear (prevents lock contention during database writes)
	tokens := tt.pendingTokens
	tt.pendingTokens = make(map[int]time.Time)

	tt.pendingMu.Unlock()

	if len(tokens) == 0 {
		return nil
	}

	slog.Debug("Flushing token updates to database", slog.String("component", "tokens"), slog.Int("count", len(tokens)))

	// Flush each token update
	for tokenID, lastUsedAt := range tokens {
		if err := tt.flushTokenToDB(tokenID, lastUsedAt); err != nil {
			slog.Error("Error flushing token update", slog.String("component", "tokens"), slog.Int("token_id", tokenID), slog.Any("error", err))
			atomic.AddInt64(&tt.errors, 1)
		}
	}

	atomic.AddInt64(&tt.flushes, 1)
	return nil
}

// flushTokenToDB writes a single token update to the database
func (tt *TokenTracker) flushTokenToDB(tokenID int, lastUsedAt time.Time) error {
	_, err := tt.db.Exec(`
		UPDATE api_tokens
		SET last_used_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, lastUsedAt, tokenID)

	if err != nil {
		return fmt.Errorf("failed to update token last_used_at: %w", err)
	}

	return nil
}

// Close gracefully shuts down the tracker with final flush
func (tt *TokenTracker) Close() error {
	slog.Debug("Closing TokenTracker", slog.String("component", "tokens"))

	// Stop periodic flush
	close(tt.stopChan)
	tt.flushTicker.Stop()
	tt.wg.Wait()

	// Final flush of pending updates
	if err := tt.FlushPendingUpdates(); err != nil {
		slog.Error("Error during final token tracker flush", slog.String("component", "tokens"), slog.Any("error", err))
		return err
	}

	slog.Debug("TokenTracker closed successfully", slog.String("component", "tokens"))
	return nil
}

// GetStats returns tracker statistics
func (tt *TokenTracker) GetStats() map[string]int64 {
	return map[string]int64{
		"updates": atomic.LoadInt64(&tt.updates),
		"flushes": atomic.LoadInt64(&tt.flushes),
		"errors":  atomic.LoadInt64(&tt.errors),
	}
}
