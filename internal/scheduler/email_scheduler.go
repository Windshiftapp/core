// Package scheduler provides background job scheduling and processing.
package scheduler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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

// processChannel polls every enabled intake on a mailbox. The connection,
// OAuth refresh, and processing lease stay per mailbox (channel); the IMAP
// watermark is per intake so folders advance independently. Returns true when
// every intake polled cleanly.
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

	// Ensure the channel-level health row exists. Watermarks live per intake,
	// but last_checked_at / error_count / last_error stay here.
	if _, err := es.getOrCreateChannelState(ctx, ch.ID); err != nil {
		slog.Error("failed to get channel state", "channel_id", ch.ID, "error", err)
		return false
	}

	provider, decryptedConfig, err := es.providerForChannel(ctx, ch.ID)
	if err != nil {
		slog.Error("failed to get provider for channel", "channel_id", ch.ID, "error", err)
		es.recordError(ctx, ch.ID, err)
		return false
	}

	// OAuth token refresh stays per mailbox. One connection serves every intake.
	if oauthProvider, ok := provider.(email.OAuthProvider); ok {
		if decryptedConfig.EmailAuthMethod == "oauth" {
			newToken, refreshErr := es.credentials.RefreshOAuthTokenIfNeeded(ctx, ch.ID, decryptedConfig, oauthProvider)
			if refreshErr != nil {
				slog.Error("failed to refresh OAuth token", "channel_id", ch.ID, "error", refreshErr)
				es.recordError(ctx, ch.ID, refreshErr)
				return false
			}
			decryptedConfig.EmailOAuthAccessToken = newToken
		}
	}

	client, err := provider.Connect(ctx, decryptedConfig)
	if err != nil {
		slog.Error("failed to connect to IMAP", "channel_id", ch.ID, "error", err)
		es.recordError(ctx, ch.ID, err)
		return false
	}
	defer func() { _ = client.Close() }()

	// A mailbox with any intake row is fully managed by those rows: a disabled
	// intake must not fall back to the legacy routing config. Only a mailbox
	// with no intake rows at all still uses the legacy config.
	allIntakes, err := repository.NewIntakeRepository(es.db).ListByMailbox(ctx, ch.ID)
	if err != nil {
		slog.Error("failed to list intakes", "channel_id", ch.ID, "error", err)
		es.recordError(ctx, ch.ID, err)
		return false
	}
	intakes := make([]models.Intake, 0, len(allIntakes))
	for _, intake := range allIntakes {
		if intake.Status == models.IntakeStatusEnabled {
			intakes = append(intakes, intake)
		}
	}
	if len(allIntakes) == 0 {
		if legacy := legacyIntakeFromConfig(decryptedConfig); legacy != nil {
			intakes = []models.Intake{*legacy}
		}
	}
	if len(intakes) == 0 {
		slog.Debug("email channel has no enabled intake; skipping", "channel_id", ch.ID)
		return true
	}

	failures := 0
	for i := range intakes {
		if pollErr := es.pollIntake(ctx, ch, client, decryptedConfig, &intakes[i]); pollErr != nil {
			failures++
			es.recordError(ctx, ch.ID, pollErr)
		}
	}
	if failures == 0 {
		es.markChannelChecked(ctx, ch.ID)
	}
	es.updateLastActivity(ctx, ch.ID)

	slog.Info("finished processing email channel",
		"channel_id", ch.ID,
		"intakes", len(intakes),
		"failures", failures,
	)
	return failures == 0
}

// legacyIntakeFromConfig derives routing for a channel that predates the intake
// split and has no intake row yet.
func legacyIntakeFromConfig(config *models.ChannelConfig) *models.Intake {
	if config == nil {
		return nil
	}
	folder := config.EmailMailbox
	if folder == "" {
		folder = "INBOX"
	}
	// A channel with no routing still yields an intake so the processor fails
	// loudly on the missing target rather than silently skipping the mailbox.
	return &models.Intake{
		Folder:                folder,
		WorkspaceID:           config.EmailWorkspaceID,
		PortalChannelID:       config.EmailConnectedPortalID,
		ItemTypeID:            config.EmailItemTypeID,
		RateLimitPerHour:      config.EmailRateLimitPerHour,
		ProcessingDisposition: config.EmailProcessingDisposition,
		Status:                models.IntakeStatusEnabled,
	}
}

// pollIntake selects one folder and processes new mail with the intake's
// routing. The watermark advances independently per intake.
func (es *EmailScheduler) pollIntake(
	ctx context.Context,
	ch channelInfo,
	client email.IMAPClient,
	base *models.ChannelConfig,
	intake *models.Intake,
) error {
	state, err := es.getOrCreatePollState(ctx, ch, intake)
	if err != nil {
		slog.Error("failed to get intake state", "channel_id", ch.ID, "intake_id", intake.ID, "error", err)
		return err
	}

	// The mailbox config carries the connection; the intake carries routing.
	effective := *base
	effective.EmailMailbox = intake.Folder
	effective.EmailWorkspaceID = intake.WorkspaceID
	effective.EmailItemTypeID = intake.ItemTypeID
	effective.EmailConnectedPortalID = intake.PortalChannelID
	effective.EmailRateLimitPerHour = intake.RateLimitPerHour
	// An empty intake disposition means "inherit the mailbox default", so keep
	// whatever the mailbox config carries (enum or legacy booleans).
	if intake.ProcessingDisposition != "" {
		effective.EmailProcessingDisposition = intake.ProcessingDisposition
	}

	mailbox := intake.Folder
	if mailbox == "" {
		mailbox = "INBOX"
	}

	selectData, err := client.SelectMailbox(mailbox)
	if err != nil {
		slog.Error("failed to select mailbox",
			"channel_id", ch.ID, "intake_id", intake.ID, "mailbox", mailbox, "error", err)
		return err
	}
	currentValidity := selectData.UIDValidity
	sinceUID := uint32(state.LastUID) //nolint:gosec // bounded by IMAP UID constraints
	if state.UIDValidity != 0 && state.UIDValidity != currentValidity {
		slog.Warn("UIDVALIDITY changed, resetting LastUID to refetch the mailbox",
			"channel_id", ch.ID,
			"intake_id", intake.ID,
			"old_validity", state.UIDValidity,
			"new_validity", currentValidity,
		)
		sinceUID = 0
	}

	messages, err := client.FetchMessages(sinceUID, 50)
	if err != nil {
		slog.Error("failed to fetch messages", "channel_id", ch.ID, "intake_id", intake.ID, "error", err)
		return err
	}
	if len(messages) == 0 {
		es.updatePollLastChecked(ctx, ch, intake, currentValidity)
		return nil
	}

	slog.Info("fetched new emails", "channel_id", ch.ID, "intake_id", intake.ID, "count", len(messages))

	maxUID := sinceUID
	processedCount := 0
	rateLimitedCount := 0
	errorCount := 0
	var lastBatchError string
	var offenderUID uint32
	deferredClaim := false
	disposition := email.ResolveEmailDisposition(&effective)
	// A synthetic legacy intake (ID 0) is channel-scoped: leave the tracking
	// row's intake_id NULL so channel-level rate-limit requeue still finds it.
	var intakeID *int
	if intake.ID > 0 {
		id := intake.ID
		intakeID = &id
	}

	for _, msg := range messages {
		if msg.FetchError != nil {
			slog.Error("failed to fetch bounded email body, stopping batch to avoid skipping the UID",
				"channel_id", ch.ID, "intake_id", intake.ID, "uid", msg.UID, "error", msg.FetchError)
			errorCount++
			offenderUID = msg.UID
			lastBatchError = fmt.Sprintf("fetch UID %d: %s", msg.UID, msg.FetchError.Error())
			break
		}
		parsed := es.parser.Parse(msg)
		result, processErr := es.processor.ProcessEmailWithIntake(ctx, parsed, ch.ID, currentValidity, &effective, intakeID)
		if processErr != nil {
			slog.Error("failed to process email, stopping batch to avoid skipping the UID",
				"channel_id", ch.ID, "intake_id", intake.ID, "uid", msg.UID,
				"message_id", parsed.MessageID, "error", processErr)
			errorCount++
			offenderUID = msg.UID
			lastBatchError = fmt.Sprintf("process UID %d: %s", msg.UID, processErr.Error())
			break
		}
		if result.Action == email.ActionDeferred {
			slog.Info("deferring email behind unfinished tracking claim",
				"channel_id", ch.ID, "intake_id", intake.ID, "uid", msg.UID, "message_id", parsed.MessageID)
			deferredClaim = true
			break
		}
		slog.Info("processed email",
			"channel_id", ch.ID,
			"intake_id", intake.ID,
			"message_id", parsed.MessageID,
			"action", result.Action,
			"item_id", result.ItemID,
			"comment_id", result.CommentID,
		)

		// Rate-limited mail is left untouched in the mailbox (unread, not
		// deleted) so an operator can requeue it; the watermark still advances
		// past it so one flooding sender cannot wedge the intake.
		rateLimited := result.Action == email.ActionRateLimited
		if !rateLimited && result.Action != email.ActionAlreadyExists {
			switch disposition {
			case models.EmailDispositionMarkRead:
				if markErr := client.MarkAsRead(msg.UID); markErr != nil {
					slog.Warn("failed to mark email as read", "uid", msg.UID, "error", markErr)
				}
			case models.EmailDispositionDelete:
				if delErr := client.DeleteMessage(msg.UID); delErr != nil {
					slog.Warn("failed to delete email", "uid", msg.UID, "error", delErr)
				}
			}
		}

		if msg.UID > maxUID {
			maxUID = msg.UID
		}
		if rateLimited {
			rateLimitedCount++
		} else {
			processedCount++
		}
	}

	if disposition == models.EmailDispositionDelete && processedCount > 0 {
		if expErr := client.Expunge(); expErr != nil {
			slog.Warn("failed to expunge deleted messages", "error", expErr)
		}
	}

	if deferredClaim {
		// Leave the cursor where it is so the claimed message is retried next tick.
		return nil
	}

	failedUID, failedValidity, failedCount := 0, uint32(0), 0
	if errorCount > 0 {
		failedUID, failedValidity, failedCount = nextFailedMessageAttempt(
			state.FailedMessageUID, state.FailedMessageUIDValidity, state.FailedMessageCount, offenderUID, currentValidity)
		if failedCount >= maxDeliveryAttempts {
			slog.Error("dropping poison email after repeated failures; advancing past it",
				"channel_id", ch.ID, "intake_id", intake.ID, "uid", offenderUID,
				"attempts", failedCount, "error", lastBatchError)
			if offenderUID > maxUID {
				maxUID = offenderUID
			}
			lastBatchError = fmt.Sprintf("dropped poison message uid=%d after %d failed attempts: %s",
				offenderUID, failedCount, lastBatchError)
			failedUID, failedValidity, failedCount = 0, 0, 0
		}
	}

	es.updatePollState(ctx, ch, intake, int(maxUID), currentValidity, failedUID, failedValidity, failedCount)

	slog.Info("finished processing intake",
		"channel_id", ch.ID,
		"intake_id", intake.ID,
		"processed", processedCount,
		"rate_limited", rateLimitedCount,
		"errors", errorCount,
	)

	if errorCount > 0 {
		return fmt.Errorf("%s", lastBatchError)
	}
	return nil
}

// nextFailedMessageAttempt advances the poison counter only when the same UID
// failed in the same UIDVALIDITY epoch. Connectivity failures and a different
// message must never inherit attempts from an older blocker.
func nextFailedMessageAttempt(failedUID int, failedUIDValidity uint32, failedCount int, uid, uidValidity uint32) (nextUID int, nextValidity uint32, count int) {
	count = 1
	if failedUID == int(uid) && failedUIDValidity == uidValidity {
		count = failedCount + 1
	}
	return int(uid), uidValidity, count
}

// getOrCreateIntakeState gets or creates the per-folder watermark record.
func (es *EmailScheduler) getOrCreateIntakeState(ctx context.Context, intakeID int) (*models.EmailIntakeState, error) {
	read := func() (*models.EmailIntakeState, error) {
		var state models.EmailIntakeState
		err := es.db.QueryRowContext(ctx, `
			SELECT intake_id, last_uid, uid_validity,
			       failed_message_uid, failed_message_uid_validity, failed_message_count
			FROM email_intake_state WHERE intake_id = ?
		`, intakeID).Scan(&state.IntakeID, &state.LastUID, &state.UIDValidity,
			&state.FailedMessageUID, &state.FailedMessageUIDValidity, &state.FailedMessageCount)
		if err != nil {
			return nil, err
		}
		return &state, nil
	}

	state, err := read()
	if err == nil {
		return state, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err := es.db.ExecWriteContext(ctx, `
		INSERT INTO email_intake_state (intake_id, last_uid, uid_validity, created_at, updated_at)
		VALUES (?, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(intake_id) DO NOTHING
	`, intakeID); err != nil {
		return nil, err
	}
	// Re-read so a row created by a concurrent caller is not reported as zero.
	return read()
}

// updateIntakeState persists the per-folder cursor and poison tracker.
func (es *EmailScheduler) updateIntakeState(
	ctx context.Context,
	intakeID, lastUID int,
	uidValidity uint32,
	failedMessageUID int,
	failedMessageUIDValidity uint32,
	failedMessageCount int,
) {
	if _, err := es.db.ExecWriteContext(ctx, `
		UPDATE email_intake_state
		SET last_uid = ?, uid_validity = ?, failed_message_uid = ?,
		    failed_message_uid_validity = ?, failed_message_count = ?, updated_at = CURRENT_TIMESTAMP
		WHERE intake_id = ?
	`, lastUID, uidValidity, failedMessageUID, failedMessageUIDValidity, failedMessageCount, intakeID); err != nil {
		slog.Error("failed to update intake state", "intake_id", intakeID, "error", err)
	}
}

// updateIntakeLastChecked records a clean empty poll and clears poison state.
func (es *EmailScheduler) updateIntakeLastChecked(ctx context.Context, intakeID int, uidValidity uint32) {
	_, _ = es.db.ExecWriteContext(ctx, `
		UPDATE email_intake_state
		SET last_uid = CASE
		        WHEN uid_validity <> 0 AND uid_validity <> ? THEN 0
		        ELSE last_uid
		    END,
		    uid_validity = ?, failed_message_uid = 0, failed_message_uid_validity = 0,
		    failed_message_count = 0, updated_at = CURRENT_TIMESTAMP
		WHERE intake_id = ?
	`, uidValidity, uidValidity, intakeID)
}

// getOrCreatePollState returns the watermark for an intake. A synthetic legacy
// intake (ID 0, derived from a channel with no intake row) reuses the
// channel-level watermark so it still persists across ticks.
func (es *EmailScheduler) getOrCreatePollState(ctx context.Context, ch channelInfo, intake *models.Intake) (*models.EmailIntakeState, error) {
	if intake.ID > 0 {
		return es.getOrCreateIntakeState(ctx, intake.ID)
	}
	channelState, err := es.getOrCreateChannelState(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	return &models.EmailIntakeState{
		LastUID:                  channelState.LastUID,
		UIDValidity:              channelState.UIDValidity,
		FailedMessageUID:         channelState.FailedMessageUID,
		FailedMessageUIDValidity: channelState.FailedMessageUIDValidity,
		FailedMessageCount:       channelState.FailedMessageCount,
	}, nil
}

// updatePollState persists the watermark for an intake (or the channel for a
// synthetic legacy intake).
func (es *EmailScheduler) updatePollState(ctx context.Context, ch channelInfo, intake *models.Intake, lastUID int, uidValidity uint32, failedUID int, failedUIDValidity uint32, failedCount int) {
	if intake.ID > 0 {
		es.updateIntakeState(ctx, intake.ID, lastUID, uidValidity, failedUID, failedUIDValidity, failedCount)
		return
	}
	es.updateChannelState(ctx, ch.ID, lastUID, uidValidity, 0, "", failedUID, failedUIDValidity, failedCount)
}

// updatePollLastChecked records a clean empty poll for an intake (or channel).
func (es *EmailScheduler) updatePollLastChecked(ctx context.Context, ch channelInfo, intake *models.Intake, uidValidity uint32) {
	if intake.ID > 0 {
		es.updateIntakeLastChecked(ctx, intake.ID, uidValidity)
		return
	}
	es.updateLastChecked(ctx, ch.ID, uidValidity)
}

// markChannelChecked clears channel-level health after a clean poll. The
// per-folder cursors live on email_intake_state.
func (es *EmailScheduler) markChannelChecked(ctx context.Context, channelID int) {
	_, _ = es.db.ExecWriteContext(ctx, `
		UPDATE email_channel_state
		SET last_checked_at = CURRENT_TIMESTAMP, error_count = 0, last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, channelID)
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
