// Package scheduler provides background job scheduling and processing.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// EmailScheduler handles periodic IMAP polling for inbound email channels
type EmailScheduler struct {
	db              database.Database
	credentials     *email.CredentialManager
	processor       *email.Processor
	parser          *email.Parser
	runRepo         *repository.SchedulerRunRepository
	ticker          *time.Ticker
	stopChan        chan struct{}
	mu              sync.RWMutex
	running         bool
	defaultInterval time.Duration
	attachmentPath  string
}

// NewEmailScheduler creates a new email scheduler
func NewEmailScheduler(db database.Database, credentials *email.CredentialManager, attachmentPath string) *EmailScheduler {
	return &EmailScheduler{
		db:              db,
		credentials:     credentials,
		processor:       email.NewProcessor(db, attachmentPath),
		parser:          email.NewParser(),
		runRepo:         repository.NewSchedulerRunRepository(db),
		ticker:          time.NewTicker(5 * time.Minute),
		stopChan:        make(chan struct{}),
		running:         false,
		defaultInterval: 5 * time.Minute,
		attachmentPath:  attachmentPath,
	}
}

// SetCommentService passes the CommentService through to the email processor
// for unified comment creation from inbound email replies.
func (es *EmailScheduler) SetCommentService(cs *services.CommentService) {
	es.processor.SetCommentService(cs)
}

// Start begins the email polling scheduler
func (es *EmailScheduler) Start() {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.running {
		return
	}

	es.running = true
	slog.Info("starting email scheduler (IMAP polling)")

	go es.schedulerLoop()
}

// Stop stops the email scheduler
func (es *EmailScheduler) Stop() {
	es.mu.Lock()
	defer es.mu.Unlock()

	if !es.running {
		return
	}

	es.running = false
	es.ticker.Stop()
	close(es.stopChan)
	slog.Info("email scheduler stopped")
}

// schedulerLoop runs the main scheduler loop
func (es *EmailScheduler) schedulerLoop() {
	// Run immediately on start
	es.processEmailChannels()

	for {
		select {
		case <-es.ticker.C:
			es.processEmailChannels()
		case <-es.stopChan:
			return
		}
	}
}

// processEmailChannels processes all active email channels
func (es *EmailScheduler) processEmailChannels() {
	start := time.Now()
	var channelsProcessed int
	var runErr error
	defer recordSchedulerRun(es.runRepo, "email", start, &channelsProcessed, &runErr)

	ctx := context.Background()

	// Get all enabled email channels
	channels, err := es.getActiveEmailChannels(ctx)
	if err != nil {
		slog.Error("failed to get email channels", "error", err)
		runErr = err
		return
	}

	if len(channels) == 0 {
		return
	}

	slog.Debug("processing email channels", "count", len(channels))

	// Count per-channel failures so the deferred recordSchedulerRun reflects them.
	// Without this, channelsProcessed grows on every tick and success stays true even
	// when every IMAP connect / parse / process step fails — admin Diagnostics then
	// shows a green "100% success rate" while real mail is silently dropped.
	failures := 0
	for _, channel := range channels {
		if !es.processChannel(ctx, channel) {
			failures++
		}
		channelsProcessed++
	}

	if failures > 0 {
		runErr = fmt.Errorf("%d of %d email channels failed", failures, len(channels))
	}
}

// channelInfo holds channel data for processing
type channelInfo struct {
	ID     int
	Name   string
	Config string
}

// getActiveEmailChannels retrieves all enabled inbound email channels
func (es *EmailScheduler) getActiveEmailChannels(ctx context.Context) ([]channelInfo, error) {
	rows, err := es.db.QueryContext(ctx, `
		SELECT id, name, config
		FROM channels
		WHERE type = 'email' AND direction = 'inbound' AND status = 'enabled'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []channelInfo
	for rows.Next() {
		var ch channelInfo
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Config); err != nil {
			continue
		}
		channels = append(channels, ch)
	}

	return channels, nil
}

// processChannel processes a single email channel. Returns true on success
// (including the no-new-messages case) and false when any step failed; the caller
// counts failures so the scheduler_run record reflects partial outages.
func (es *EmailScheduler) processChannel(ctx context.Context, ch channelInfo) bool {
	slog.Debug("processing email channel", "channel_id", ch.ID, "name", ch.Name)

	// Parse channel config
	var config models.ChannelConfig
	if ch.Config != "" {
		if err := json.Unmarshal([]byte(ch.Config), &config); err != nil {
			slog.Error("failed to parse channel config", "channel_id", ch.ID, "error", err)
			es.recordError(ctx, ch.ID, err)
			return false
		}
	}

	// Get or create channel state
	state, err := es.getOrCreateChannelState(ctx, ch.ID)
	if err != nil {
		slog.Error("failed to get channel state", "channel_id", ch.ID, "error", err)
		return false
	}

	// Get provider and connect
	provider, decryptedConfig, err := es.credentials.GetProviderForChannel(ctx, ch.ID)
	if err != nil {
		slog.Error("failed to get provider for channel", "channel_id", ch.ID, "error", err)
		es.recordError(ctx, ch.ID, err)
		return false
	}

	// Refresh OAuth token if needed (for OAuth providers)
	if oauthProvider, ok := provider.(email.OAuthProvider); ok {
		if decryptedConfig.EmailAuthMethod == "oauth" {
			var newToken string
			newToken, err = es.credentials.RefreshOAuthTokenIfNeeded(ctx, ch.ID, decryptedConfig, oauthProvider)
			if err != nil {
				slog.Error("failed to refresh OAuth token", "channel_id", ch.ID, "error", err)
				es.recordError(ctx, ch.ID, err)
				return false
			}
			decryptedConfig.EmailOAuthAccessToken = newToken
		}
	}

	// Connect to IMAP
	client, err := provider.Connect(ctx, decryptedConfig)
	if err != nil {
		slog.Error("failed to connect to IMAP", "channel_id", ch.ID, "error", err)
		es.recordError(ctx, ch.ID, err)
		return false
	}
	defer func() { _ = client.Close() }()

	// Determine mailbox
	mailbox := decryptedConfig.EmailMailbox
	if mailbox == "" {
		mailbox = "INBOX"
	}

	// Select the mailbox and check UIDVALIDITY. Per RFC 3501, UIDs are only
	// meaningful within a given UIDVALIDITY epoch — if the server bumps it
	// (mailbox restore, quota reset, folder migration) then our cached LastUID
	// is pointing into a different universe and we must start over, or we'll
	// either skip unread messages (their new UIDs are below the stale LastUID)
	// or spam dedup with reprocessed old messages.
	selectData, err := client.SelectMailbox(mailbox)
	if err != nil {
		slog.Error("failed to select mailbox", "channel_id", ch.ID, "mailbox", mailbox, "error", err)
		es.recordError(ctx, ch.ID, err)
		return false
	}
	currentValidity := selectData.UIDValidity
	sinceUID := uint32(state.LastUID) //nolint:gosec // G115: value is bounded by IMAP UID constraints
	if state.UIDValidity != 0 && state.UIDValidity != currentValidity {
		slog.Warn("UIDVALIDITY changed, resetting LastUID to refetch the mailbox",
			"channel_id", ch.ID,
			"old_validity", state.UIDValidity,
			"new_validity", currentValidity,
		)
		sinceUID = 0
	}

	// Fetch new messages
	batchSize := 50
	messages, err := client.FetchMessages(sinceUID, batchSize)
	if err != nil {
		slog.Error("failed to fetch messages", "channel_id", ch.ID, "error", err)
		es.recordError(ctx, ch.ID, err)
		return false
	}

	if len(messages) == 0 {
		es.updateLastChecked(ctx, ch.ID)
		return true
	}

	slog.Info("fetched new emails", "channel_id", ch.ID, "count", len(messages))

	// Process messages in UID order and bail at the first failure so we never
	// advance past a message we haven't successfully handled. Advancing over a
	// gap would permanently lose the failed UID (next poll searches UID > last,
	// which skips it). A stuck message will keep failing and hold up the queue,
	// which is the correct behavior — surface it via errorCount/last_error.
	// Seed maxUID from sinceUID (not state.LastUID) so a UIDVALIDITY reset
	// persists: after a reset sinceUID==0 and we want LastUID to reflect the
	// new UID space even if processing stops at the first message.
	maxUID := sinceUID
	processedCount := 0
	errorCount := 0

	for _, msg := range messages {
		parsed, err := es.parser.Parse(msg)
		if err != nil {
			slog.Error("failed to parse email, stopping batch to avoid skipping the UID",
				"channel_id", ch.ID, "uid", msg.UID, "error", err)
			errorCount++
			break
		}

		result, err := es.processor.ProcessEmail(ctx, parsed, ch.ID, decryptedConfig)
		if err != nil {
			slog.Error("failed to process email, stopping batch to avoid skipping the UID",
				"channel_id", ch.ID,
				"uid", msg.UID,
				"message_id", parsed.MessageID,
				"error", err,
			)
			errorCount++
			break
		}

		slog.Info("processed email",
			"channel_id", ch.ID,
			"message_id", parsed.MessageID,
			"action", result.Action,
			"item_id", result.ItemID,
			"comment_id", result.CommentID,
		)

		// Post-processing. Failures here are logged but don't block UID advancement:
		// dedup is by UID tracking (not \Seen flag), so a failed MarkAsRead just
		// leaves the message visually unread, not double-processed.
		if decryptedConfig.EmailMarkAsRead {
			if err := client.MarkAsRead(msg.UID); err != nil {
				slog.Warn("failed to mark email as read", "uid", msg.UID, "error", err)
			}
		}
		if decryptedConfig.EmailDeleteAfterProcess {
			if err := client.DeleteMessage(msg.UID); err != nil {
				slog.Warn("failed to delete email", "uid", msg.UID, "error", err)
			}
		}

		if msg.UID > maxUID {
			maxUID = msg.UID
		}
		processedCount++
	}

	// Expunge if we deleted messages
	if decryptedConfig.EmailDeleteAfterProcess && processedCount > 0 {
		if err := client.Expunge(); err != nil {
			slog.Warn("failed to expunge deleted messages", "error", err)
		}
	}

	// Update channel state (including the observed UIDVALIDITY so a future
	// server-side reset is detected on the next tick).
	es.updateChannelState(ctx, ch.ID, int(maxUID), currentValidity, errorCount)

	// Update channel last_activity
	es.updateLastActivity(ctx, ch.ID)

	slog.Info("finished processing email channel",
		"channel_id", ch.ID,
		"processed", processedCount,
		"errors", errorCount,
	)

	// errorCount > 0 means we hit a parse/process failure mid-batch (the loop above
	// breaks on the first such failure). The channel's UID watermark didn't advance
	// past the offender, so the next tick retries — but for diagnostics the tick
	// should still record as failed so admins see the partial outage.
	return errorCount == 0
}

// getOrCreateChannelState gets or creates the channel state record
func (es *EmailScheduler) getOrCreateChannelState(ctx context.Context, channelID int) (*models.EmailChannelState, error) {
	var state models.EmailChannelState
	var lastCheckedAt sql.NullTime
	var lastError sql.NullString

	err := es.db.QueryRowContext(ctx, `
		SELECT id, channel_id, last_uid, uid_validity, last_checked_at, error_count, last_error
		FROM email_channel_state
		WHERE channel_id = ?
	`, channelID).Scan(
		&state.ID, &state.ChannelID, &state.LastUID, &state.UIDValidity,
		&lastCheckedAt, &state.ErrorCount, &lastError,
	)

	if err == nil {
		if lastCheckedAt.Valid {
			state.LastCheckedAt = &lastCheckedAt.Time
		}
		if lastError.Valid {
			state.LastError = lastError.String
		}
		return &state, nil
	}

	if err != sql.ErrNoRows {
		return nil, err
	}

	// Create new state
	_, err = es.db.ExecContext(ctx, `
		INSERT INTO email_channel_state (channel_id, last_uid, error_count, created_at, updated_at)
		VALUES (?, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, channelID)
	if err != nil {
		return nil, err
	}

	return &models.EmailChannelState{
		ChannelID:  channelID,
		LastUID:    0,
		ErrorCount: 0,
	}, nil
}

// updateChannelState updates the channel state after processing
func (es *EmailScheduler) updateChannelState(ctx context.Context, channelID, lastUID int, uidValidity uint32, errorCount int) {
	_, err := es.db.ExecContext(ctx, `
		UPDATE email_channel_state
		SET last_uid = ?, uid_validity = ?, last_checked_at = CURRENT_TIMESTAMP, error_count = ?, last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, lastUID, uidValidity, errorCount, channelID)
	if err != nil {
		slog.Error("failed to update channel state", "error", err)
	}
}

// updateLastChecked updates the last checked timestamp
func (es *EmailScheduler) updateLastChecked(ctx context.Context, channelID int) {
	_, _ = es.db.ExecContext(ctx, `
		UPDATE email_channel_state
		SET last_checked_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, channelID)
}

// recordError records an error for the channel
func (es *EmailScheduler) recordError(ctx context.Context, channelID int, err error) {
	_, _ = es.db.ExecContext(ctx, `
		UPDATE email_channel_state
		SET error_count = error_count + 1, last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, err.Error(), channelID)
}

// updateLastActivity updates the channel's last_activity timestamp
func (es *EmailScheduler) updateLastActivity(ctx context.Context, channelID int) {
	_, _ = es.db.ExecContext(ctx, `
		UPDATE channels SET last_activity = CURRENT_TIMESTAMP WHERE id = ?
	`, channelID)
}

// ProcessChannelNow triggers immediate processing of a specific channel.
// This is primarily used for testing to avoid waiting for the scheduler interval.
func (es *EmailScheduler) ProcessChannelNow(channelID int) error {
	ctx := context.Background()

	// Get channel info
	var ch channelInfo
	err := es.db.QueryRow(`
		SELECT id, name, config FROM channels
		WHERE id = ? AND type = 'email' AND direction = 'inbound'
	`, channelID).Scan(&ch.ID, &ch.Name, &ch.Config)
	if err != nil {
		slog.Error("failed to get channel for on-demand processing", "channel_id", channelID, "error", err)
		return err
	}

	if !es.processChannel(ctx, ch) {
		return fmt.Errorf("processing email channel %d failed; see scheduler logs for details", channelID)
	}
	return nil
}
