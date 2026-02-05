package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/models"
	"windshift/internal/services"
)

// EmailScheduler handles periodic IMAP polling for inbound email channels
type EmailScheduler struct {
	db              database.Database
	credentials     *email.CredentialManager
	processor       *email.Processor
	parser          *email.Parser
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
	ctx := context.Background()

	// Get all enabled email channels
	channels, err := es.getActiveEmailChannels(ctx)
	if err != nil {
		slog.Error("failed to get email channels", "error", err)
		return
	}

	if len(channels) == 0 {
		return
	}

	slog.Debug("processing email channels", "count", len(channels))

	for _, channel := range channels {
		es.processChannel(ctx, channel)
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
	rows, err := es.db.Query(`
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

// processChannel processes a single email channel
func (es *EmailScheduler) processChannel(ctx context.Context, ch channelInfo) {
	slog.Debug("processing email channel", "channel_id", ch.ID, "name", ch.Name)

	// Parse channel config
	var config models.ChannelConfig
	if ch.Config != "" {
		if err := json.Unmarshal([]byte(ch.Config), &config); err != nil {
			slog.Error("failed to parse channel config", "channel_id", ch.ID, "error", err)
			es.recordError(ctx, ch.ID, err)
			return
		}
	}

	// Get or create channel state
	state, err := es.getOrCreateChannelState(ctx, ch.ID)
	if err != nil {
		slog.Error("failed to get channel state", "channel_id", ch.ID, "error", err)
		return
	}

	// Get provider and connect
	provider, decryptedConfig, err := es.credentials.GetProviderForChannel(ctx, ch.ID)
	if err != nil {
		slog.Error("failed to get provider for channel", "channel_id", ch.ID, "error", err)
		es.recordError(ctx, ch.ID, err)
		return
	}

	// Refresh OAuth token if needed (for OAuth providers)
	if oauthProvider, ok := provider.(email.OAuthProvider); ok {
		if decryptedConfig.EmailAuthMethod == "oauth" {
			newToken, err := es.credentials.RefreshOAuthTokenIfNeeded(ctx, ch.ID, decryptedConfig, oauthProvider)
			if err != nil {
				slog.Error("failed to refresh OAuth token", "channel_id", ch.ID, "error", err)
				es.recordError(ctx, ch.ID, err)
				return
			}
			decryptedConfig.EmailOAuthAccessToken = newToken
		}
	}

	// Connect to IMAP
	client, err := provider.Connect(ctx, decryptedConfig)
	if err != nil {
		slog.Error("failed to connect to IMAP", "channel_id", ch.ID, "error", err)
		es.recordError(ctx, ch.ID, err)
		return
	}
	defer client.Close()

	// Determine mailbox
	mailbox := decryptedConfig.EmailMailbox
	if mailbox == "" {
		mailbox = "INBOX"
	}

	// Fetch new messages
	batchSize := 50
	messages, err := client.FetchMessages(mailbox, uint32(state.LastUID), batchSize)
	if err != nil {
		slog.Error("failed to fetch messages", "channel_id", ch.ID, "error", err)
		es.recordError(ctx, ch.ID, err)
		return
	}

	if len(messages) == 0 {
		es.updateLastChecked(ctx, ch.ID)
		return
	}

	slog.Info("fetched new emails", "channel_id", ch.ID, "count", len(messages))

	// Process each message
	var maxUID uint32 = uint32(state.LastUID)
	processedCount := 0
	errorCount := 0

	for _, msg := range messages {
		// Parse the message
		parsed, err := es.parser.Parse(msg)
		if err != nil {
			slog.Error("failed to parse email", "channel_id", ch.ID, "uid", msg.UID, "error", err)
			errorCount++
			continue
		}

		// Process the email
		result, err := es.processor.ProcessEmail(ctx, parsed, ch.ID, decryptedConfig)
		if err != nil {
			slog.Error("failed to process email",
				"channel_id", ch.ID,
				"message_id", parsed.MessageID,
				"error", err,
			)
			errorCount++
			continue
		}

		slog.Info("processed email",
			"channel_id", ch.ID,
			"message_id", parsed.MessageID,
			"action", result.Action,
			"item_id", result.ItemID,
			"comment_id", result.CommentID,
		)

		// Handle post-processing
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

		// Track max UID
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

	// Update channel state
	es.updateChannelState(ctx, ch.ID, int(maxUID), errorCount)

	// Update channel last_activity
	es.updateLastActivity(ctx, ch.ID)

	slog.Info("finished processing email channel",
		"channel_id", ch.ID,
		"processed", processedCount,
		"errors", errorCount,
	)
}

// getOrCreateChannelState gets or creates the channel state record
func (es *EmailScheduler) getOrCreateChannelState(ctx context.Context, channelID int) (*models.EmailChannelState, error) {
	var state models.EmailChannelState
	var lastCheckedAt sql.NullTime
	var lastError sql.NullString

	err := es.db.QueryRow(`
		SELECT id, channel_id, last_uid, last_checked_at, error_count, last_error
		FROM email_channel_state
		WHERE channel_id = ?
	`, channelID).Scan(
		&state.ID, &state.ChannelID, &state.LastUID,
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
	_, err = es.db.Exec(`
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
func (es *EmailScheduler) updateChannelState(ctx context.Context, channelID, lastUID, errorCount int) {
	_, err := es.db.Exec(`
		UPDATE email_channel_state
		SET last_uid = ?, last_checked_at = CURRENT_TIMESTAMP, error_count = ?, last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, lastUID, errorCount, channelID)
	if err != nil {
		slog.Error("failed to update channel state", "error", err)
	}
}

// updateLastChecked updates the last checked timestamp
func (es *EmailScheduler) updateLastChecked(ctx context.Context, channelID int) {
	es.db.Exec(`
		UPDATE email_channel_state
		SET last_checked_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, channelID)
}

// recordError records an error for the channel
func (es *EmailScheduler) recordError(ctx context.Context, channelID int, err error) {
	es.db.Exec(`
		UPDATE email_channel_state
		SET error_count = error_count + 1, last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, err.Error(), channelID)
}

// updateLastActivity updates the channel's last_activity timestamp
func (es *EmailScheduler) updateLastActivity(ctx context.Context, channelID int) {
	es.db.Exec(`
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

	es.processChannel(ctx, ch)
	return nil
}
