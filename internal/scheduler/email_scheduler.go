// Package scheduler provides background job scheduling and processing.
package scheduler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	// providerForChannel resolves the IMAP provider + decrypted config for a
	// channel. Defaults to CredentialManager.GetProviderForChannel; tests
	// override it to inject a fake provider (the real path dials through an
	// SSRF-safe, TLS-only client that can't reach an in-process mock server).
	providerForChannel func(ctx context.Context, channelID int) (email.Provider, *models.ChannelConfig, error)
}

// NewEmailScheduler creates a new email scheduler
func NewEmailScheduler(db database.Database, credentials *email.CredentialManager, attachmentPath string) *EmailScheduler {
	return &EmailScheduler{
		db:                 db,
		credentials:        credentials,
		processor:          email.NewProcessor(db, attachmentPath),
		parser:             email.NewParser(),
		runRepo:            repository.NewSchedulerRunRepository(db),
		stopChan:           make(chan struct{}),
		running:            false,
		defaultInterval:    5 * time.Minute,
		attachmentPath:     attachmentPath,
		providerForChannel: credentials.GetProviderForChannel,
	}
}

// SetCommentService passes the CommentService through to the email processor
// for unified comment creation from inbound email replies.
func (es *EmailScheduler) SetCommentService(cs *services.CommentService) {
	es.processor.SetCommentService(cs)
}

// SetEventCoordinator forwards the event coordinator wiring to the processor
// so email-created items emit the same side effects as REST-created ones
// (notifications, webhooks, action triggers, activity tracking).
func (es *EmailScheduler) SetEventCoordinator(ec *services.EventCoordinator) {
	es.processor.SetEventCoordinator(ec)
}

// Start begins the email polling scheduler
func (es *EmailScheduler) Start() {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.running {
		return
	}

	es.ticker = time.NewTicker(es.defaultInterval)
	es.stopChan = make(chan struct{})
	es.running = true
	slog.Info("starting email scheduler (IMAP polling)")

	go es.schedulerLoop(es.ticker, es.stopChan)
}

// Stop stops the email scheduler
func (es *EmailScheduler) Stop() {
	es.mu.Lock()
	defer es.mu.Unlock()

	if !es.running {
		return
	}

	es.running = false
	if es.ticker != nil {
		es.ticker.Stop()
		es.ticker = nil
	}
	close(es.stopChan)
	slog.Info("email scheduler stopped")
}

// schedulerLoop runs the main scheduler loop
func (es *EmailScheduler) schedulerLoop(ticker *time.Ticker, stopChan <-chan struct{}) {
	// Run immediately on start
	es.processEmailChannels()

	for {
		select {
		case <-ticker.C:
			es.processEmailChannels()
		case <-stopChan:
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
		channelCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		ok := es.processChannel(channelCtx, channel)
		cancel()
		if !ok {
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
		SELECT id, name, COALESCE(config, '{}')
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return channels, nil
}

// maxDeliveryAttempts is how many consecutive polls may fail on the same
// message before the scheduler gives up on it. Until this is reached the UID
// watermark is held back so the message is retried (transient failures recover
// on their own); once reached, the message is treated as poison and skipped so
// one un-parseable email can't wedge the whole channel forever.
const maxDeliveryAttempts = 5

const emailProcessingLeaseDuration = 3 * time.Minute

// acquireProcessingLease makes mailbox polling single-writer across both
// scheduler instances and the manual process-now endpoint. It is deliberately
// non-blocking: another worker already owns the useful work, so a duplicate
// scheduler tick should skip instead of waiting only to re-fetch the same UIDs.
// The lease outlives the two-minute per-channel context by one minute so a
// canceled worker cannot overlap its replacement while unwinding.
func (es *EmailScheduler) acquireProcessingLease(ctx context.Context, channelID int) (owner string, acquired bool, err error) {
	ownerBytes := make([]byte, 16)
	if _, err := rand.Read(ownerBytes); err != nil {
		return "", false, fmt.Errorf("generate email processing lease token: %w", err)
	}
	owner = hex.EncodeToString(ownerBytes)
	result, err := es.db.ExecWriteContext(ctx, `
		INSERT INTO email_processing_leases(channel_id, owner_token, expires_at)
		VALUES (?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			owner_token = excluded.owner_token,
			expires_at = excluded.expires_at
		WHERE email_processing_leases.expires_at <= CURRENT_TIMESTAMP
	`, channelID, owner, time.Now().Add(emailProcessingLeaseDuration))
	if err != nil {
		return "", false, fmt.Errorf("claim email processing lease: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return "", false, fmt.Errorf("count claimed email processing leases: %w", err)
	}
	return owner, rows > 0, nil
}

func (es *EmailScheduler) releaseProcessingLease(ctx context.Context, channelID int, owner string) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := es.db.ExecWriteContext(releaseCtx, `
		DELETE FROM email_processing_leases
		WHERE channel_id = ? AND owner_token = ?
	`, channelID, owner); err != nil {
		slog.Error("failed to release email processing lease", "channel_id", channelID, "error", err)
	}
}

// processChannel processes a single email channel. Returns true on success
// (including the no-new-messages case) and false when any step failed; the caller
// counts failures so the scheduler_run record reflects partial outages.
func (es *EmailScheduler) processChannel(ctx context.Context, ch channelInfo) bool {
	slog.Debug("processing email channel", "channel_id", ch.ID, "name", ch.Name)
	owner, acquired, err := es.acquireProcessingLease(ctx, ch.ID)
	if err != nil {
		slog.Error("failed to acquire email processing lease", "channel_id", ch.ID, "error", err)
		return false
	}
	if !acquired {
		slog.Debug("email channel is already being processed; skipping duplicate poll", "channel_id", ch.ID)
		return true
	}
	defer es.releaseProcessingLease(ctx, ch.ID, owner)

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
	provider, decryptedConfig, err := es.providerForChannel(ctx, ch.ID)
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
		// Persist the observed UIDVALIDITY even when the mailbox is empty. If the
		// server changed epochs, retaining the old validity/LastUID would make
		// every subsequent poll restart from UID 0 until a message arrived.
		es.updateLastChecked(ctx, ch.ID, currentValidity)
		return true
	}

	slog.Info("fetched new emails", "channel_id", ch.ID, "count", len(messages))

	// Process in UID order and stop at failures so watermark advancement cannot
	// lose mail. Retry each UID/UIDVALIDITY until the poison-message limit; seed
	// maxUID from sinceUID so UIDVALIDITY resets persist.
	maxUID := sinceUID
	processedCount := 0
	errorCount := 0
	var lastBatchError string
	var offenderUID uint32
	deferredClaim := false

	for _, msg := range messages {
		if msg.FetchError != nil {
			slog.Error("failed to fetch bounded email body, stopping batch to avoid skipping the UID",
				"channel_id", ch.ID,
				"uid", msg.UID,
				"error", msg.FetchError,
			)
			errorCount++
			offenderUID = msg.UID
			lastBatchError = fmt.Sprintf("fetch UID %d: %s", msg.UID, msg.FetchError.Error())
			break
		}
		parsed := es.parser.Parse(msg)

		result, err := es.processor.ProcessEmail(ctx, parsed, ch.ID, currentValidity, decryptedConfig)
		if err != nil {
			slog.Error("failed to process email, stopping batch to avoid skipping the UID",
				"channel_id", ch.ID,
				"uid", msg.UID,
				"message_id", parsed.MessageID,
				"error", err,
			)
			errorCount++
			offenderUID = msg.UID
			lastBatchError = fmt.Sprintf("process UID %d: %s", msg.UID, err.Error())
			break
		}
		if result.Action == email.ActionDeferred {
			slog.Info("deferring email behind unfinished tracking claim",
				"channel_id", ch.ID,
				"uid", msg.UID,
				"message_id", parsed.MessageID,
			)
			deferredClaim = true
			break
		}

		slog.Info("processed email",
			"channel_id", ch.ID,
			"message_id", parsed.MessageID,
			"action", result.Action,
			"item_id", result.ItemID,
			"comment_id", result.CommentID,
		)

		// Post-processing failures do not block UID advancement. Do not alter
		// re-fetched ActionAlreadyExists mail after resets or restores.
		if result.Action != email.ActionAlreadyExists {
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

	// Track poison retries per UID/UIDVALIDITY separately from channel health.
	// After the limit, advance past the offender and surface the drop to admins.
	healthErrorCount := 0
	failedMessageUID := 0
	failedMessageUIDValidity := uint32(0)
	failedMessageCount := 0
	if deferredClaim {
		healthErrorCount = state.ErrorCount
		lastBatchError = state.LastError
		failedMessageUID = state.FailedMessageUID
		failedMessageUIDValidity = state.FailedMessageUIDValidity
		failedMessageCount = state.FailedMessageCount
	}
	if errorCount > 0 {
		healthErrorCount = state.ErrorCount + 1
		failedMessageUID, failedMessageUIDValidity, failedMessageCount = nextFailedMessageAttempt(state, offenderUID, currentValidity)
		if failedMessageCount >= maxDeliveryAttempts {
			slog.Error("dropping poison email after repeated failures; advancing past it",
				"channel_id", ch.ID,
				"uid", offenderUID,
				"attempts", failedMessageCount,
				"error", lastBatchError,
			)
			if offenderUID > maxUID {
				maxUID = offenderUID
			}
			lastBatchError = fmt.Sprintf("dropped poison message uid=%d after %d failed attempts: %s",
				offenderUID, failedMessageCount, lastBatchError)
			// Restart tracking for the next UID; last_error remains until a clean poll.
			healthErrorCount = 0
			failedMessageUID = 0
			failedMessageUIDValidity = 0
			failedMessageCount = 0
		}
	}

	// Update channel state (including the observed UIDVALIDITY so a future
	// server-side reset is detected on the next tick).
	es.updateChannelState(ctx, ch.ID, int(maxUID), currentValidity, healthErrorCount, lastBatchError,
		failedMessageUID, failedMessageUIDValidity, failedMessageCount)

	// Update channel last_activity
	es.updateLastActivity(ctx, ch.ID)

	slog.Info("finished processing email channel",
		"channel_id", ch.ID,
		"processed", processedCount,
		"errors", errorCount,
	)

	// errorCount > 0 means we hit a parse/process failure mid-batch (the loop above
	// breaks on the first such failure). Whether we retried or dropped a poison
	// message, the tick records as failed so admins see it on the diagnostics
	// surface (scheduler_runs) and via the channel's last_error.
	return errorCount == 0
}

// nextFailedMessageAttempt advances the poison counter only when the same UID
// failed in the same UIDVALIDITY epoch. Connectivity failures and a different
// message must never inherit attempts from an older blocker.
func nextFailedMessageAttempt(state *models.EmailChannelState, uid, uidValidity uint32) (failedUID int, failedUIDValidity uint32, count int) {
	count = 1
	if state != nil && state.FailedMessageUID == int(uid) && state.FailedMessageUIDValidity == uidValidity {
		count = state.FailedMessageCount + 1
	}
	return int(uid), uidValidity, count
}

// getOrCreateChannelState gets or creates the channel state record
func (es *EmailScheduler) getOrCreateChannelState(ctx context.Context, channelID int) (*models.EmailChannelState, error) {
	var state models.EmailChannelState
	var lastCheckedAt sql.NullTime
	var lastError sql.NullString

	err := es.db.QueryRowContext(ctx, `
		SELECT id, channel_id, last_uid, uid_validity, last_checked_at, error_count, last_error,
		       failed_message_uid, failed_message_uid_validity, failed_message_count
		FROM email_channel_state
		WHERE channel_id = ?
	`, channelID).Scan(
		&state.ID, &state.ChannelID, &state.LastUID, &state.UIDValidity,
		&lastCheckedAt, &state.ErrorCount, &lastError,
		&state.FailedMessageUID, &state.FailedMessageUIDValidity, &state.FailedMessageCount,
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

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Create new state
	_, err = es.db.ExecWriteContext(ctx, `
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

// updateChannelState updates the channel state after processing. last_error
// is preserved when the batch had partial failures (errorCount > 0) so the
// operator keeps the concrete message instead of just an error counter. It's
// cleared only on a clean run so a previously broken channel can be seen to
// recover.
func (es *EmailScheduler) updateChannelState(
	ctx context.Context,
	channelID, lastUID int,
	uidValidity uint32,
	errorCount int,
	lastBatchError string,
	failedMessageUID int,
	failedMessageUIDValidity uint32,
	failedMessageCount int,
) {
	// last_error is keyed off the message, not the count: a dropped poison
	// message resets error_count to 0 but still records why, so the channel
	// stays flagged unhealthy until a clean poll passes lastBatchError == "".
	var lastError sql.NullString
	if lastBatchError != "" {
		lastError = sql.NullString{String: lastBatchError, Valid: true}
	}
	_, err := es.db.ExecWriteContext(ctx, `
		UPDATE email_channel_state
		SET last_uid = ?, uid_validity = ?, last_checked_at = CURRENT_TIMESTAMP,
		    error_count = ?, last_error = ?, failed_message_uid = ?,
		    failed_message_uid_validity = ?, failed_message_count = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, lastUID, uidValidity, errorCount, lastError, failedMessageUID,
		failedMessageUIDValidity, failedMessageCount, channelID)
	if err != nil {
		slog.Error("failed to update channel state", "error", err)
	}
}

// updateLastChecked records a clean empty poll. The current UIDVALIDITY must be
// persisted here as well as after non-empty batches; otherwise a changed epoch
// is rediscovered on every empty poll. Match processChannel's legacy behavior
// by preserving LastUID when the stored validity is 0 (unknown), but reset it
// when a known epoch changes.
func (es *EmailScheduler) updateLastChecked(ctx context.Context, channelID int, uidValidity uint32) {
	_, _ = es.db.ExecWriteContext(ctx, `
		UPDATE email_channel_state
		SET last_uid = CASE
		        WHEN uid_validity <> 0 AND uid_validity <> ? THEN 0
		        ELSE last_uid
		    END,
		    uid_validity = ?, last_checked_at = CURRENT_TIMESTAMP,
		    error_count = 0, last_error = NULL,
		    failed_message_uid = 0, failed_message_uid_validity = 0,
		    failed_message_count = 0, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, uidValidity, uidValidity, channelID)
}

// recordError records an error for the channel
func (es *EmailScheduler) recordError(ctx context.Context, channelID int, err error) {
	_, _ = es.db.ExecWriteContext(ctx, `
		UPDATE email_channel_state
		SET error_count = error_count + 1, last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, err.Error(), channelID)
}

// updateLastActivity updates the channel's last_activity timestamp
func (es *EmailScheduler) updateLastActivity(ctx context.Context, channelID int) {
	_, _ = es.db.ExecWriteContext(ctx, `
		UPDATE channels SET last_activity = CURRENT_TIMESTAMP WHERE id = ?
	`, channelID)
}

// ProcessChannelNow triggers immediate processing of a specific channel.
// This is primarily used for testing to avoid waiting for the scheduler interval.
// The caller's deadline is honored by every DB and IMAP operation.
func (es *EmailScheduler) ProcessChannelNow(ctx context.Context, channelID int) error {
	// Get channel info
	var ch channelInfo
	err := es.db.QueryRowContext(ctx, `
		SELECT id, name, COALESCE(config, '{}') FROM channels
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
