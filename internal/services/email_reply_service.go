package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/emailutil"
	"windshift/internal/i18n"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/smtp"
)

// EmailReplyService sends threaded SMTP replies to portal customers
// when internal users add comments to email-originated items.
type EmailReplyService struct {
	db         database.Database
	smtpSender ThreadedEmailSender
	idResolver *IDResolverService
	outboxMu   sync.Mutex
	canonical  bool
	// baseURL is the browser-visible origin used to build portal request links
	// in customer emails. Empty disables the link.
	baseURL string
	// leaseOwner identifies this process's delivery claims so a manual retry
	// can tell a live claim from retry backoff (WI-1572).
	leaseOwner string
}

// NewEmailReplyService creates a new EmailReplyService.
func NewEmailReplyService(db database.Database, smtpSender ThreadedEmailSender) *EmailReplyService {
	hostname, _ := os.Hostname()
	return &EmailReplyService{
		db:         db,
		smtpSender: smtpSender,
		idResolver: NewIDResolverService(db),
		leaseOwner: fmt.Sprintf("%s-%d", hostname, os.Getpid()),
	}
}

// SetBaseURL sets the browser-visible origin used to build portal request
// links in customer emails. Called from server startup.
func (s *EmailReplyService) SetBaseURL(baseURL string) {
	s.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// HandleCommentCreated checks if an outbound email should be sent for a new comment.
// It sends a threaded email to the portal customer if:
// - The comment is not private
// - The comment is from an internal user (not from a portal customer)
// - The item was created via an email channel by a portal customer
func (s *EmailReplyService) HandleCommentCreated(params HandleCommentParams) error {
	if s.canonical {
		return nil
	}
	return s.handleCommentCreated(params, true)
}

func (s *EmailReplyService) handleCommentCreated(params HandleCommentParams, deliverImmediately bool) error {
	// Guard: skip private comments
	if params.IsPrivate {
		return nil
	}

	// Guard: imported comments are historical. Emailing them would send the
	// whole archive to every customer on the ticket as if it were written now.
	if itemevents.IsImportOrigin(params.ActorKind, params.SourceKind) {
		return nil
	}

	// Guard: skip if comment is FROM a portal customer (don't echo back)
	if params.PortalCustomerID != nil {
		return nil
	}

	// Guard: skip if no identified author
	if params.AuthorID == 0 {
		return nil
	}

	// Query item: channel, portal customer creator, workspace key, item number, title
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(params.ItemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("failed to query item for email reply: %w", err)
	}

	// Skip if item has no channel or no addressable requester.
	if item.ChannelID == nil {
		return nil
	}

	// Any existing channel works as long as the conversation is threadable:
	// email-originated tickets thread from the customer's real messages,
	// portal/form tickets thread from their synthetic anchor.
	var channelType string
	err = s.db.QueryRow("SELECT type FROM channels WHERE id = ?", *item.ChannelID).Scan(&channelType)
	if err != nil {
		// Channel gone — nothing to thread from, skip.
		return nil
	}

	// Portal/form-originated tickets thread from the anchor minted at
	// submission; the ensure covers pre-upgrade tickets (created before the
	// anchor existed) and repairs a failed best-effort mint. It is a no-op for
	// email channels and for items that already have their anchor. An
	// unverified email contact has no anchor: the inbound message is the root.
	maybeRecordPortalThreadAnchor(s.db, int64(params.ItemID), item.ChannelID, item.CreatorPortalCustomerID, item.Title)

	// Recipients: the verified creator, or the item's unverified contact until a
	// reply key proves the address, plus every external request participant
	// (WI-1136). Each gets its own outbox row so replies attribute per
	// recipient, and the intake participant guard accepts them as repliers.
	customerEmail, customerName, ok := s.requesterRecipient(item)
	if !ok {
		slog.Debug("no addressable requester for item, skipping reply",
			"component", "email_reply_service",
			slog.Int("item_id", params.ItemID),
		)
		return nil
	}
	recipients := []emailRecipient{{Email: customerEmail, Name: customerName}}
	participants, err := repository.NewItemParticipantRepository(s.db).ListByItem(params.ItemID)
	if err != nil {
		return fmt.Errorf("load item participants for email reply: %w", err)
	}
	for _, participant := range participants {
		if participant.CustomerEmail == "" || strings.EqualFold(participant.CustomerEmail, customerEmail) {
			continue
		}
		recipients = append(recipients, emailRecipient{Email: participant.CustomerEmail, Name: participant.CustomerName})
	}

	// Load the item's recorded thread. Threading headers are built per
	// recipient below: another recipient's reply key is never copied into
	// References or In-Reply-To (WI-2365).
	records, err := s.loadItemThread(params.ItemID)
	if err != nil {
		return fmt.Errorf("failed to query email tracking: %w", err)
	}
	if len(records) == 0 {
		// No email tracking records — can't thread, skip
		slog.Debug("no email tracking records for item, skipping reply",
			slog.String("component", "email_reply_service"),
			slog.Int("item_id", params.ItemID),
		)
		return nil
	}

	// Subject: Re: {original subject} from first tracking record
	originalSubject := item.Title
	if records[0].Subject.Valid && records[0].Subject.String != "" {
		originalSubject = records[0].Subject.String
	}
	subject := originalSubject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}

	// Get SMTP domain for Message-ID generation
	smtpDomain := s.getSMTPDomain()

	// Get author name for email template
	authorName := s.idResolver.ResolveUserName(params.AuthorID)
	if authorName == "" {
		authorName = "Team member"
	}

	// Build email body via the shared template pipeline. We pre-compute the
	// threaded subject above (Re: …) and pass it through as OriginalSubject;
	// the rendered subject from RenderEmail is discarded so the threading
	// stays correct.
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
	portalURL := s.portalRequestURL(params.ItemID)

	// Route replies back to the monitored intake mailbox, not the SMTP sender.
	// Persisted per row so a retry is stable if the channel config changes.
	replyToEmail, replyToName := s.resolveReplyToAddress(params.ItemID, *item.ChannelID)
	outboxIDs := make([]int, 0, len(recipients))
	for _, recipient := range recipients {
		// Render per recipient so each gets the body in their own language.
		_, htmlBody, textBody, err := s.smtpSender.RenderEmailLocale(s.portalCustomerLocale(recipient.Email), emailutil.TemplatePortalReply, struct {
			AuthorName      string
			ItemKey         string
			ItemTitle       string
			Content         string
			OriginalSubject string
			PortalURL       string
		}{
			AuthorName:      authorName,
			ItemKey:         itemKey,
			ItemTitle:       item.Title,
			Content:         params.Content,
			OriginalSubject: subject,
			PortalURL:       portalURL,
		})
		if err != nil {
			return fmt.Errorf("failed to render portal reply email: %w", err)
		}
		// Recipient-specific threading keeps the reply key scoped to this
		// recipient: another participant's outbound Message-ID is never copied
		// into References or In-Reply-To. An item whose original mail had no
		// Message-ID still gets its reply: the minted Message-ID below is the
		// reply key, and empty threading headers must not silently drop the
		// customer's reply.
		inReplyTo, references := threadHeadersForRecipient(records, recipient.Email)
		referencesJSON, err := json.Marshal(references)
		if err != nil {
			return fmt.Errorf("encode email references: %w", err)
		}
		// A cryptographically random, per-recipient Message-ID is the reply key.
		// It is persisted on the outbox row so a retry reuses the same ID.
		messageID, err := newReplyMessageID("ws-comment", params.CommentID, smtpDomain)
		if err != nil {
			return fmt.Errorf("mint reply Message-ID: %w", err)
		}
		var outboxID int
		err = s.db.QueryRow(`
			INSERT INTO email_reply_outbox (
				comment_id, channel_id, item_id, to_email, to_name, subject,
				html_body, text_body, message_id, key_generation, in_reply_to, references_json,
				from_email, from_name, reply_to_email, reply_to_name,
				next_attempt_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT(comment_id, to_email) WHERE comment_id IS NOT NULL DO NOTHING
			RETURNING id
		`, params.CommentID, *item.ChannelID, params.ItemID, recipient.Email, recipient.Name,
			subject, htmlBody, textBody, messageID, inReplyTo, string(referencesJSON),
			s.getSMTPFromEmail(), authorName, replyToEmail, replyToName).Scan(&outboxID)
		if errors.Is(err, sql.ErrNoRows) {
			continue // already enqueued for this recipient
		}
		if err != nil {
			return fmt.Errorf("enqueue threaded email reply: %w", err)
		}
		outboxIDs = append(outboxIDs, outboxID)
	}
	if len(outboxIDs) > 0 {
		// The key is persisted, so the ticket must never fall back to legacy
		// (generation 0) authorization again, even after this row is pruned.
		s.markReplyKeyIssued(params.ItemID)
	}

	// Sending immediately keeps the current low-latency behavior. The durable
	// rows remain pending on failure and are retried by NotificationScheduler.
	if !deliverImmediately || !s.smtpSender.IsSMTPConfigured() {
		return nil
	}
	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()
	for _, outboxID := range outboxIDs {
		if _, err := s.deliverPendingReply(context.Background(), outboxID); err != nil {
			return fmt.Errorf("threaded email queued for retry: %w", err)
		}
	}
	return nil
}

// resolveReplyToAddress returns the monitored inbound address that customer
// replies should route back to for an item on the given channel. Empty when no
// monitored mailbox applies; the transport then omits Reply-To and falls back
// to From.
//
// The item's own intake is authoritative: the mailbox that fed it owns the
// reply address, so a portal with several intakes never answers from an
// unrelated one. A portal web submission has no feeding intake, so it falls
// back to the oldest enabled intake exposing requests to that portal.
func (s *EmailReplyService) resolveReplyToAddress(itemID, channelID int) (address, name string) {
	var intakeMailboxID sql.NullInt64
	if err := s.db.QueryRow(`
		SELECT i.mailbox_id
		FROM items it
		JOIN intakes i ON i.id = it.intake_id
		WHERE it.id = ?
	`, itemID).Scan(&intakeMailboxID); err == nil && intakeMailboxID.Valid {
		if address, name = s.mailboxAddress(int(intakeMailboxID.Int64)); address != "" {
			return address, name
		}
	}
	// Email-originated items also carry the mailbox as their own channel.
	if address, name = s.mailboxAddress(channelID); address != "" {
		return address, name
	}
	// Portal-originated items carry a portal channel. Route replies back to the
	// mailbox whose enabled intake feeds that portal, preferring the oldest one
	// so the choice is stable across sends. Disabled mailboxes are skipped, and
	// a mailbox with no readable address falls through to the next candidate.
	rows, err := s.db.Query(`
		SELECT i.mailbox_id
		FROM intakes i
		JOIN channels c ON c.id = i.mailbox_id
		WHERE i.portal_channel_id = ?
		  AND i.status = ? AND c.status = 'enabled'
		ORDER BY i.id
	`, channelID, models.IntakeStatusEnabled)
	if err != nil {
		return "", ""
	}
	var mailboxIDs []int
	for rows.Next() {
		var mailboxID int
		if err := rows.Scan(&mailboxID); err != nil {
			_ = rows.Close()
			return "", ""
		}
		mailboxIDs = append(mailboxIDs, mailboxID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", ""
	}
	// Release the cursor before resolving addresses: holding it while the
	// mailbox query runs deadlocks a one-connection read pool.
	if err := rows.Close(); err != nil {
		return "", ""
	}
	for _, mailboxID := range mailboxIDs {
		if address, name = s.mailboxAddress(mailboxID); address != "" {
			return address, name
		}
	}
	return "", ""
}

// mailboxAddress resolves the monitored address of an email/imap channel.
func (s *EmailReplyService) mailboxAddress(channelID int) (address, name string) {
	var configJSON string
	if err := s.db.QueryRow(
		`SELECT COALESCE(config, '{}') FROM channels WHERE id = ? AND type IN ('email', 'imap')`,
		channelID,
	).Scan(&configJSON); err != nil {
		return "", ""
	}
	var cfg models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", ""
	}
	address = strings.TrimSpace(cfg.EmailOAuthEmail)
	if address == "" {
		address = strings.TrimSpace(cfg.IMAPUsername)
	}
	if address == "" {
		return "", ""
	}
	_ = s.db.QueryRow(`SELECT name FROM channels WHERE id = ?`, channelID).Scan(&name)
	return address, name
}

// markReplyKeyIssued records the durable per-item marker that this ticket has
// issued a recipient-bound reply key (WI-2365). It is what lets the legacy
// fallback stay closed after the outbox row carrying the key is pruned, so it
// must be written whenever a generation-1 outbox row is persisted. Set-once; a
// failure is logged because the outbox row still blocks the legacy path until
// it is pruned.
func (s *EmailReplyService) markReplyKeyIssued(itemID int) {
	if _, err := s.db.ExecWrite(`
		UPDATE items SET reply_key_issued_at = CURRENT_TIMESTAMP
		WHERE id = ? AND reply_key_issued_at IS NULL
	`, itemID); err != nil {
		slog.Warn("failed to record reply-key marker; legacy replies stay blocked by the outbox row only",
			"component", "email_reply_service",
			slog.Int("item_id", itemID),
			slog.Any("error", err),
		)
	}
}

// requesterRecipient resolves the customer-facing recipient of an item: the
// verified portal owner when there is one, otherwise the unverified email
// contact recorded by intake. Notices, receipts and replies all use it so an
// email-only request reaches its sender before ownership is verified
// (WI-2365). ok is false when the item has no addressable requester.
func (s *EmailReplyService) requesterRecipient(item *models.Item) (email, name string, ok bool) {
	if item.CreatorPortalCustomerID != nil {
		return s.noticeCustomer(*item.CreatorPortalCustomerID)
	}
	if item.ContactEmail == "" {
		return "", "", false
	}
	return item.ContactEmail, item.ContactName, true
}

// emailRecipient is one outbound reply target.
type emailRecipient struct {
	Email string
	Name  string
}

// newReplyMessageID mints a cryptographically random Message-ID for one
// outbound delivery. It is generated once and persisted with the outbox row,
// so a retry reuses the same ID rather than minting a second reply key. The
// prefix and scope id keep the value recognizable in logs; the random suffix
// makes it unguessable.
func newReplyMessageID(prefix string, scopeID int, domain string) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate reply Message-ID entropy: %w", err)
	}
	return fmt.Sprintf("<%s-%d-%s@%s>", prefix, scopeID, hex.EncodeToString(entropy[:]), domain), nil
}

// emailThreadRecord is one message recorded on an item's conversation.
// ToEmail is the intended recipient of an outbound row and empty for inbound
// rows and synthetic thread anchors.
type emailThreadRecord struct {
	MessageID string
	Subject   sql.NullString
	Direction string
	ToEmail   string
	At        time.Time
}

// loadItemThread loads the item's conversation for building recipient-specific
// threading headers. Inbound messages and synthetic anchors come from
// email_message_tracking; delivered reply keys come from email_reply_outbox,
// which alone records the intended recipient (WI-2365). The two sources are
// merged chronologically so the parent chosen below is the most recent message,
// not whichever source was appended last.
func (s *EmailReplyService) loadItemThread(itemID int) ([]emailThreadRecord, error) {
	var records []emailThreadRecord
	rows, err := s.db.Query(`
		SELECT message_id, subject, direction, '', processed_at FROM email_message_tracking
		WHERE item_id = ?
		  AND (direction = 'inbound' OR (direction = 'outbound' AND comment_id IS NULL AND from_email = ''))
		ORDER BY processed_at ASC, id ASC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("query email tracking: %w", err)
	}
	for rows.Next() {
		var rec emailThreadRecord
		if err := rows.Scan(&rec.MessageID, &rec.Subject, &rec.Direction, &rec.ToEmail, &rec.At); err != nil {
			continue
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate email tracking: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close email tracking rows: %w", err)
	}

	// Only delivered, non-discarded, recipient-bound keys belong in the thread:
	// a pending or discarded row was never sent, and a generation-0 row is a
	// pre-upgrade ID that must not be exposed.
	keyRows, err := s.db.Query(`
		SELECT message_id, subject, to_email, delivered_at FROM email_reply_outbox
		WHERE item_id = ? AND message_id <> '' AND to_email <> ''
		  AND delivered_at IS NOT NULL AND discarded_at IS NULL AND key_generation >= 1
		ORDER BY delivered_at ASC, id ASC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("query email reply keys: %w", err)
	}
	defer keyRows.Close()
	for keyRows.Next() {
		var rec emailThreadRecord
		if err := keyRows.Scan(&rec.MessageID, &rec.Subject, &rec.ToEmail, &rec.At); err != nil {
			continue
		}
		rec.Direction = "outbound"
		records = append(records, rec)
	}
	if err := keyRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email reply keys: %w", err)
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].At.Before(records[j].At) })
	return records, nil
}

// threadHeadersForRecipient builds the In-Reply-To and References for one
// recipient. The conversation root, inbound messages, and the recipient's own
// prior reply keys form the chain; another recipient's reply key is never
// copied into the headers (WI-2365). Returns empty headers when the item has
// no recorded thread.
func threadHeadersForRecipient(records []emailThreadRecord, recipient string) (inReplyTo string, references []string) {
	for _, rec := range records {
		if rec.MessageID == "" {
			continue
		}
		// An outbound row with a recipient is a reply key owned by that
		// recipient. Never expose it to anyone else.
		if rec.Direction == "outbound" && rec.ToEmail != "" && !strings.EqualFold(rec.ToEmail, recipient) {
			continue
		}
		references = append(references, rec.MessageID)
		inReplyTo = rec.MessageID
	}
	return inReplyTo, references
}

type emailReplyOutboxRow struct {
	ID             int
	Kind           string
	CommentID      sql.NullInt64
	ChannelID      int
	ItemID         int
	ToEmail        string
	ToName         string
	Subject        string
	HTMLBody       string
	TextBody       string
	MessageID      string
	InReplyTo      string
	ReferencesJSON string
	FromEmail      string
	FromName       string
	ReplyToEmail   string
	ReplyToName    string
	AttemptCount   int
}

// SendAutomationNotice implements CustomerNotifier: it emails the portal
// customer who created the item through the same threaded transport as reply
// notifications. Automation notices send directly rather than through the
// comment-bound reply outbox; action run history is the durable record.
// Skips (no customer, no thread, SMTP unset) are reported, not errors.
func (s *EmailReplyService) SendAutomationNotice(itemID, actorID int, subject, message string) (delivered bool, skipReason string, err error) {
	item, skipReason, err := s.loadNoticeItem(itemID)
	if err != nil || item == nil {
		return false, skipReason, err
	}
	// Same requester resolution as staff replies and receipts: an email-only
	// request has no portal owner yet, but its contact is still the person an
	// automation `notify_customer` rule is meant to reach (WI-2365).
	customerEmail, customerName, ok := s.requesterRecipient(item)
	if !ok {
		return false, "no_customer", nil
	}
	return s.sendItemNotice(item, customerEmail, customerName, subject, message, actorID)
}

// noticeCustomer resolves a portal customer's email and name for a notice.
// ok is false when the row is missing; that is an expected skip, not an error.
func (s *EmailReplyService) noticeCustomer(customerID int) (email, name string, ok bool) {
	if err := s.db.QueryRow("SELECT email, name FROM portal_customers WHERE id = ?", customerID).Scan(&email, &name); err != nil {
		return "", "", false
	}
	return email, name, true
}

// SendParticipantAddedNotice emails a newly added external request participant
// through the item's threaded transport (WI-1136). Participants receive
// customer-facing content only. Expected skips (no SMTP, no channel, no
// thread, no email) are reported, not errors.
func (s *EmailReplyService) SendParticipantAddedNotice(itemID, customerID, actorID int) (delivered bool, skipReason string, err error) {
	item, skipReason, err := s.loadNoticeItem(itemID)
	if err != nil || item == nil {
		return false, skipReason, err
	}
	customerEmail, customerName, ok := s.noticeCustomer(customerID)
	if !ok {
		return false, "no_customer", nil
	}
	message := fmt.Sprintf("You have been added as a participant to %s.", item.Title)
	return s.sendItemNotice(item, customerEmail, customerName, "", message, actorID)
}

// loadNoticeItem loads an item for a customer-facing notice and ensures its
// portal thread anchor exists. Returns (nil, reason, nil) for expected skips.
func (s *EmailReplyService) loadNoticeItem(itemID int) (*models.Item, string, error) {
	if !s.smtpSender.IsSMTPConfigured() {
		return nil, "smtp_not_configured", nil
	}
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, "item_missing", nil
		}
		return nil, "", fmt.Errorf("load item for customer notice: %w", err)
	}
	if item.ChannelID == nil {
		return nil, "no_channel", nil
	}
	// Same anchor ensure as the reply path: notices must thread for
	// portal-originated tickets (and survive pre-upgrade tickets) too.
	maybeRecordPortalThreadAnchor(s.db, int64(itemID), item.ChannelID, item.CreatorPortalCustomerID, item.Title)
	return item, "", nil
}

// sendItemNotice renders and sends one threaded customer-facing notice to a
// specific recipient on the item's conversation.
func (s *EmailReplyService) sendItemNotice(item *models.Item, toEmail, toName, subject, message string, actorID int) (delivered bool, skipReason string, err error) {
	if toEmail == "" {
		return false, "no_email", nil
	}

	// Thread against the item's recorded messages, scoped to this recipient:
	// another participant's reply key is never copied into References or
	// In-Reply-To. Without a thread there is nothing to reply into, so the
	// notice would start an orphaned conversation.
	records, err := s.loadItemThread(item.ID)
	if err != nil {
		return false, "", fmt.Errorf("query email tracking for customer notice: %w", err)
	}
	if len(records) == 0 {
		return false, "no_thread", nil
	}
	inReplyTo, references := threadHeadersForRecipient(records, toEmail)
	var originalSubject sql.NullString
	for _, rec := range records {
		if rec.Subject.Valid && rec.Subject.String != "" {
			originalSubject = rec.Subject
			break
		}
	}

	emailSubject := item.Title
	if originalSubject.Valid && originalSubject.String != "" {
		emailSubject = originalSubject.String
	}
	if subject != "" {
		emailSubject = subject
	}
	if !strings.HasPrefix(strings.ToLower(emailSubject), "re:") {
		emailSubject = "Re: " + emailSubject
	}

	authorName := s.idResolver.ResolveUserName(actorID)
	if authorName == "" {
		authorName = "Support Team"
	}
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
	portalURL := s.portalRequestURL(item.ID)
	_, htmlBody, textBody, err := s.smtpSender.RenderEmailLocale(s.portalCustomerLocale(toEmail), emailutil.TemplatePortalReply, struct {
		AuthorName      string
		ItemKey         string
		ItemTitle       string
		Content         string
		OriginalSubject string
		PortalURL       string
	}{
		AuthorName:      authorName,
		ItemKey:         itemKey,
		ItemTitle:       item.Title,
		Content:         message,
		OriginalSubject: emailSubject,
		PortalURL:       portalURL,
	})
	if err != nil {
		return false, "", fmt.Errorf("render customer notice email: %w", err)
	}

	smtpDomain := s.getSMTPDomain()
	messageID, err := newReplyMessageID("ws-notice", item.ID, smtpDomain)
	if err != nil {
		return false, "", fmt.Errorf("mint notice Message-ID: %w", err)
	}
	replyToEmail, replyToName := s.resolveReplyToAddress(item.ID, *item.ChannelID)
	referencesJSON, err := json.Marshal(references)
	if err != nil {
		referencesJSON = []byte("[]")
	}

	// Persist the reply key before crossing the SMTP boundary. If the send
	// succeeds but the process dies before recording the key, the customer
	// holds a Message-ID Windshift never learned. A persistence failure must
	// therefore fail the notice instead of sending an unroutable key.
	//
	// Delivery is recorded only after SMTP accepts the message: an undelivered
	// row is not a delivered reply key, and must never be picked as a thread
	// parent, or a later email would name a message the customer never received
	// as its parent (WI-2365). Holding outboxMu keeps the scheduler from
	// claiming this pending row between the insert and the send.
	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()
	var outboxID int
	if err := s.db.QueryRow(`
		INSERT INTO email_reply_outbox (
			kind, channel_id, item_id, to_email, to_name, subject,
			html_body, text_body, message_id, key_generation, in_reply_to, references_json,
			from_email, from_name, reply_to_email, reply_to_name,
			next_attempt_at, created_at, updated_at
		) VALUES ('notice', ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?,
		          CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, *item.ChannelID, item.ID, toEmail, toName, emailSubject,
		htmlBody, textBody, messageID, inReplyTo, string(referencesJSON),
		s.getSMTPFromEmail(), authorName, replyToEmail, replyToName).Scan(&outboxID); err != nil {
		return false, "", fmt.Errorf("persist customer notice reply key: %w", err)
	}
	// The key exists on the ticket from now on, so the legacy fallback is closed
	// for it even if this notice is never delivered.
	s.markReplyKeyIssued(item.ID)

	if err := s.smtpSender.SendThreadedEmail(smtp.ThreadedEmailParams{
		ToEmail:      toEmail,
		ToName:       toName,
		ReplyToEmail: replyToEmail,
		ReplyToName:  replyToName,
		Subject:      emailSubject,
		HTMLBody:     htmlBody,
		TextBody:     textBody,
		MessageID:    messageID,
		InReplyTo:    inReplyTo,
		References:   references,
	}); err != nil {
		// The customer never received this key, and the action run records the
		// failure, so the row must not be retried as a delayed notice. Drop it;
		// the reply-key marker above stays set conservatively.
		if _, delErr := s.db.ExecWrite(`DELETE FROM email_reply_outbox WHERE id = ? AND delivered_at IS NULL`, outboxID); delErr != nil {
			slog.Warn("failed to drop undelivered customer notice key",
				"component", "email_reply_service",
				slog.Int("item_id", item.ID),
				slog.Int("outbox_id", outboxID),
				slog.Any("error", delErr),
			)
		}
		return false, "", fmt.Errorf("send customer notice: %w", err)
	}

	if _, err := s.db.ExecWrite(`
		UPDATE email_reply_outbox
		SET delivered_at = CURRENT_TIMESTAMP, last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL
	`, outboxID); err != nil {
		return false, "", fmt.Errorf("mark customer notice delivered: %w", err)
	}

	// Audit record after a successful send: the notice's Message-ID and thread
	// placement for the channel email log. Best-effort.
	if _, err := s.db.ExecWrite(`
		INSERT INTO email_message_tracking (
			channel_id, message_id, dedup_key, in_reply_to, from_email, from_name, subject,
			item_id, comment_id, key_generation, direction, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, 1, 'outbound', CURRENT_TIMESTAMP)
		ON CONFLICT(channel_id, dedup_key) DO NOTHING
	`, *item.ChannelID, messageID, messageID, inReplyTo, s.getSMTPFromEmail(), authorName, emailSubject, item.ID); err != nil {
		slog.Warn("failed to record customer notice in email tracking; replies may not thread",
			slog.String("component", "email_reply_service"),
			slog.Int("item_id", item.ID),
			slog.String("message_id", messageID),
			slog.Any("error", err),
		)
	}
	return true, "", nil
}

// EnqueueIntakeConfirmation queues the sender's receipt for a request an
// email intake just created (WI-1712, WI-1769). The receipt goes through the
// durable reply outbox instead of sending inline, so a slow or failing SMTP
// server never stalls mailbox polling and the receipt is retried with backoff.
// The receipt starts a fresh thread and records its own Message-ID on
// delivery, so a reply to it lands on the ticket. Expected skips (missing
// customer/item, no SMTP) are non-errors.
func (s *EmailReplyService) EnqueueIntakeConfirmation(itemID, customerID int) error {
	if !s.smtpSender.IsSMTPConfigured() {
		return nil
	}
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("load item for intake confirmation: %w", err)
	}
	if item.ChannelID == nil {
		return nil
	}
	// A receipt goes to the verified portal customer when there is one, or to
	// the item's unverified contact address until a reply key verifies it.
	var customerEmail, customerName string
	if customerID > 0 {
		if err := s.db.QueryRow(`SELECT email, COALESCE(name, '') FROM portal_customers WHERE id = ?`, customerID).Scan(&customerEmail, &customerName); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("load portal customer for intake confirmation: %w", err)
		}
	} else {
		customerEmail, customerName = item.ContactEmail, item.ContactName
	}
	if customerEmail == "" {
		return nil
	}
	if customerName == "" {
		customerName = "there"
	}
	itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
	portalURL := s.portalRequestURL(itemID)
	subject, htmlBody, textBody, err := s.smtpSender.RenderEmailLocale(s.portalCustomerLocale(customerEmail), emailutil.TemplateIntakeConfirmation, struct {
		FirstName string
		ItemKey   string
		ItemTitle string
		PortalURL string
	}{
		FirstName: customerName,
		ItemKey:   itemKey,
		ItemTitle: item.Title,
		PortalURL: portalURL,
	})
	if err != nil {
		return fmt.Errorf("render intake confirmation email: %w", err)
	}

	messageID, err := newReplyMessageID("ws-confirm", itemID, s.getSMTPDomain())
	if err != nil {
		return fmt.Errorf("mint intake confirmation Message-ID: %w", err)
	}
	replyToEmail, replyToName := s.resolveReplyToAddress(itemID, *item.ChannelID)
	// The per-item confirmation unique index makes re-processing the same email
	// a no-op, so a duplicate receipt is never queued.
	if _, err := s.db.ExecWrite(`
		INSERT INTO email_reply_outbox (
			kind, channel_id, item_id, to_email, to_name, subject,
			html_body, text_body, message_id, key_generation, in_reply_to, references_json,
			from_email, from_name, reply_to_email, reply_to_name,
			next_attempt_at, created_at, updated_at
		) VALUES ('intake_confirmation', ?, ?, ?, ?, ?, ?, ?, ?, 1, '', '[]', ?, ?, ?, ?,
		          CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
	`, *item.ChannelID, itemID, customerEmail, customerName, subject,
		htmlBody, textBody, messageID, s.getSMTPFromEmail(), customerName, replyToEmail, replyToName); err != nil {
		return fmt.Errorf("enqueue intake confirmation: %w", err)
	}
	// A duplicate receipt (ON CONFLICT DO NOTHING) means an earlier enqueue already
	// carried the key, so the marker is safe to set either way.
	s.markReplyKeyIssued(itemID)
	return nil
}

// portalRequestURL resolves the customer-facing link to an item's request in
// the portal that exposes it. The item's intake portal wins (email intake with
// a portal link); otherwise the item's own channel is used (portal/form
// submissions). Returns empty when the item is not exposed to a portal or the
// base URL is unset.
func (s *EmailReplyService) portalRequestURL(itemID int) string {
	if s.baseURL == "" {
		return ""
	}
	var channelID, intakeID sql.NullInt64
	if err := s.db.QueryRow(`SELECT channel_id, intake_id FROM items WHERE id = ?`, itemID).Scan(&channelID, &intakeID); err != nil {
		return ""
	}
	if intakeID.Valid {
		var portalChannelID sql.NullInt64
		if err := s.db.QueryRow(`SELECT portal_channel_id FROM intakes WHERE id = ?`, intakeID.Int64).Scan(&portalChannelID); err == nil && portalChannelID.Valid {
			if slug := s.portalSlug(int(portalChannelID.Int64)); slug != "" {
				return fmt.Sprintf("%s/portal/%s?view=requests&id=%d", s.baseURL, slug, itemID)
			}
		}
	}
	if channelID.Valid {
		if slug := s.portalSlug(int(channelID.Int64)); slug != "" {
			return fmt.Sprintf("%s/portal/%s?view=requests&id=%d", s.baseURL, slug, itemID)
		}
	}
	return ""
}

// portalSlug returns the slug of a portal channel, or an empty string when the
// channel is not a portal or has no slug configured.
func (s *EmailReplyService) portalSlug(channelID int) string {
	var channelType, configJSON string
	if err := s.db.QueryRow(`SELECT type, COALESCE(config, '{}') FROM channels WHERE id = ?`, channelID).Scan(&channelType, &configJSON); err != nil {
		return ""
	}
	if channelType != "portal" {
		return ""
	}
	var cfg models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.PortalSlug)
}

// portalCustomerLocale resolves the recipient portal customer's stored
// language by email, falling back to the default when unset or unknown.
func (s *EmailReplyService) portalCustomerLocale(email string) string {
	language, err := repository.NewPortalCustomerRepository(s.db).LanguageByEmail(context.Background(), email)
	if err != nil || language == "" {
		return i18n.Default
	}
	return i18n.Normalize(language)
}

// ProcessPendingReplies retries a bounded batch from the durable reply outbox.
// It is called by NotificationScheduler on the existing SMTP cadence.
func (s *EmailReplyService) ProcessPendingReplies(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 50
	}
	if !s.smtpSender.IsSMTPConfigured() {
		return 0, nil
	}

	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()

	rows, err := s.db.Query(`
		SELECT id
		FROM email_reply_outbox
		WHERE delivered_at IS NULL AND discarded_at IS NULL AND next_attempt_at <= CURRENT_TIMESTAMP
		ORDER BY created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("query email reply outbox: %w", err)
	}
	var outboxIDs []int
	for rows.Next() {
		var outboxID int
		if err := rows.Scan(&outboxID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan email reply outbox: %w", err)
		}
		outboxIDs = append(outboxIDs, outboxID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate email reply outbox: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close email reply outbox rows: %w", err)
	}

	delivered := 0
	var lastErr error
	for _, outboxID := range outboxIDs {
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}
		sent, err := s.deliverPendingReply(ctx, outboxID)
		if err != nil {
			lastErr = err
			continue
		}
		if sent {
			delivered++
		}
	}
	// Delivered rows stay as reply keys for 90 days even though a key is only
	// honored for 30 (see email.ReplyKeyValidity): the extra window keeps the
	// row available for audit and diagnostics before it is pruned.
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	if _, err := s.db.ExecWrite(`DELETE FROM email_reply_outbox WHERE delivered_at IS NOT NULL AND delivered_at < ?`, cutoff); err != nil {
		slog.Warn("failed to prune delivered email reply outbox rows", "error", err)
	}
	return delivered, lastErr
}

func (s *EmailReplyService) deliverPendingReply(ctx context.Context, outboxID int) (bool, error) {
	var row emailReplyOutboxRow
	// Atomically lease the row before crossing the SMTP boundary. The process
	// mutex prevents duplicates within one server; this conditional UPDATE also
	// prevents two application instances from selecting and sending the same
	// pending reply. A crashed worker releases itself when the lease expires.
	leaseUntil := time.Now().Add(5 * time.Minute)
	err := s.db.QueryRow(`
		UPDATE email_reply_outbox
		SET next_attempt_at = ?, lease_owner = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL AND discarded_at IS NULL
		  AND next_attempt_at <= CURRENT_TIMESTAMP
		RETURNING id, kind, comment_id, channel_id, item_id, to_email, to_name, subject,
		       html_body, text_body, message_id, in_reply_to, references_json,
		       from_email, from_name, reply_to_email, reply_to_name, attempt_count
	`, leaseUntil, s.leaseOwner, outboxID).Scan(
		&row.ID, &row.Kind, &row.CommentID, &row.ChannelID, &row.ItemID, &row.ToEmail, &row.ToName,
		&row.Subject, &row.HTMLBody, &row.TextBody, &row.MessageID, &row.InReplyTo,
		&row.ReferencesJSON, &row.FromEmail, &row.FromName, &row.ReplyToEmail, &row.ReplyToName,
		&row.AttemptCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim pending email reply: %w", err)
	}

	var references []string
	if err := json.Unmarshal([]byte(row.ReferencesJSON), &references); err != nil {
		s.recordReplyFailure(row.ID, row.AttemptCount, err)
		return false, fmt.Errorf("decode pending email references: %w", err)
	}
	err = s.smtpSender.SendThreadedEmailContext(ctx, smtp.ThreadedEmailParams{
		ToEmail:      row.ToEmail,
		ToName:       row.ToName,
		ReplyToEmail: row.ReplyToEmail,
		ReplyToName:  row.ReplyToName,
		Subject:      row.Subject,
		HTMLBody:     row.HTMLBody,
		TextBody:     row.TextBody,
		MessageID:    row.MessageID,
		InReplyTo:    row.InReplyTo,
		References:   references,
	})
	if err != nil {
		s.recordReplyFailure(row.ID, row.AttemptCount, err)
		return false, fmt.Errorf("send threaded email: %w", err)
	}

	if _, err := s.db.ExecWrite(`
		UPDATE email_reply_outbox
		SET delivered_at = CURRENT_TIMESTAMP, last_error = NULL, lease_owner = NULL,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL
	`, row.ID); err != nil {
		return false, fmt.Errorf("mark threaded email delivered: %w", err)
	}

	// Tracking is best-effort after delivery. Do not retry SMTP merely because
	// this audit/threading insert failed; that would duplicate customer mail.
	// key_generation marks the row as a recipient-bound reply key so it survives
	// outbox pruning without ever being mistaken for a pre-upgrade ID.
	if _, err := s.db.ExecWrite(`
		INSERT INTO email_message_tracking (
			channel_id, message_id, dedup_key, in_reply_to, from_email, from_name, subject,
			item_id, comment_id, key_generation, direction, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'outbound', CURRENT_TIMESTAMP)
		ON CONFLICT(channel_id, dedup_key) DO NOTHING
	`, row.ChannelID, row.MessageID, row.MessageID, row.InReplyTo, row.FromEmail,
		row.FromName, row.Subject, row.ItemID, row.CommentID); err != nil {
		slog.Warn("failed to record delivered outbound email in tracking",
			"comment_id", row.CommentID, "error", err)
	}

	slog.Info("sent threaded email reply to customer",
		"component", "email_reply_service",
		"kind", row.Kind,
		"comment_id", row.CommentID,
		"item_id", row.ItemID,
		"to", row.ToEmail,
	)
	return true, nil
}

func (s *EmailReplyService) recordReplyFailure(outboxID, previousAttempts int, sendErr error) {
	shift := min(previousAttempts, 6)
	nextAttempt := time.Now().Add(time.Minute * time.Duration(1<<shift))
	if _, err := s.db.ExecWrite(`
		UPDATE email_reply_outbox
		SET attempt_count = attempt_count + 1, next_attempt_at = ?,
		    last_error = ?, lease_owner = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND delivered_at IS NULL
	`, nextAttempt, sendErr.Error(), outboxID); err != nil {
		slog.Error("failed to record email reply delivery failure", "outbox_id", outboxID, "error", err)
	}
}

// getSMTPDomain extracts the domain from the SMTP from email.
func (s *EmailReplyService) getSMTPDomain() string {
	return smtpDomain(s.db)
}

// getSMTPFromEmail gets the configured SMTP from email address.
func (s *EmailReplyService) getSMTPFromEmail() string {
	return smtpFromEmail(s.db)
}

// fallbackSMTPFromEmail is used when no default outbound SMTP channel is
// configured or its config can't be read.
const fallbackSMTPFromEmail = "noreply@windshift.local"

// smtpDomain extracts the domain from the default outbound SMTP from email.
// Shared by the reply service and the portal thread-anchor minter.
func smtpDomain(db database.Database) string {
	fromEmail := smtpFromEmail(db)
	if idx := strings.LastIndex(fromEmail, "@"); idx >= 0 {
		return fromEmail[idx+1:]
	}
	return "windshift.local"
}

// smtpFromEmail reads the configured from email of the default outbound SMTP
// channel, falling back when none is configured or its config can't be read.
func smtpFromEmail(db database.Database) string {
	var configJSON string
	err := db.QueryRow(`
		SELECT COALESCE(config, '{}') FROM channels
		WHERE type = 'smtp' AND direction = 'outbound'
		  AND status = 'enabled' AND is_default = true
		ORDER BY updated_at DESC
		LIMIT 1
	`).Scan(&configJSON)
	if err != nil {
		return fallbackSMTPFromEmail
	}

	var cfg models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil || cfg.SMTPFromEmail == "" {
		return fallbackSMTPFromEmail
	}
	return cfg.SMTPFromEmail
}

// ErrEmailReplyNotRetryable marks an outbox row an operator cannot re-send:
// it is already delivered, explicitly discarded, or belongs to another
// channel. Handlers surface it as a conflict; repository.ErrNotFound stays
// the missing-row signal.
var ErrEmailReplyNotRetryable = errors.New("email reply is not retryable")

// ErrEmailReplyInFlight marks an outbox row whose delivery lease another
// worker still holds; the operator retry must not steal it (WI-1572).
var ErrEmailReplyInFlight = errors.New("email reply is being delivered by another worker")

// RetryPendingReply attempts immediate delivery of one pending outbound
// reply on behalf of an operator. The row's backoff lease is cleared first so
// a stuck schedule (or an earlier failure's next_attempt_at) cannot block the
// explicit retry. A failed send is recorded like any scheduler attempt:
// attempt_count increments, last_error is stored, and the row returns to the
// backoff schedule.
func (s *EmailReplyService) RetryPendingReply(channelID, commentID int) (delivered bool, err error) {
	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()

	// One comment can have one outbox row per recipient (creator +
	// participants, WI-1136). Retry every retryable row and never steal a
	// live delivery lease.
	type outboxState struct {
		id        int
		delivered sql.NullTime
		discarded sql.NullTime
		lease     sql.NullString
		next      time.Time
	}
	rows, err := s.db.Query(`
		SELECT id, delivered_at, discarded_at, lease_owner, next_attempt_at
		FROM email_reply_outbox
		WHERE channel_id = ? AND comment_id = ?
	`, channelID, commentID)
	if err != nil {
		return false, fmt.Errorf("load email reply for retry: %w", err)
	}
	var states []outboxState
	for rows.Next() {
		var state outboxState
		if scanErr := rows.Scan(&state.id, &state.delivered, &state.discarded, &state.lease, &state.next); scanErr != nil {
			_ = rows.Close()
			return false, fmt.Errorf("scan email reply for retry: %w", scanErr)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, fmt.Errorf("iterate email replies for retry: %w", err)
	}
	_ = rows.Close()

	if len(states) == 0 {
		return false, repository.ErrNotFound
	}

	now := time.Now()
	inFlight := false
	retryable := make([]int, 0, len(states))
	for _, state := range states {
		if state.delivered.Valid || state.discarded.Valid {
			continue
		}
		if state.lease.Valid && state.next.After(now) {
			inFlight = true
			continue
		}
		retryable = append(retryable, state.id)
	}
	if len(retryable) == 0 {
		if inFlight {
			return false, ErrEmailReplyInFlight
		}
		return false, ErrEmailReplyNotRetryable
	}

	if !s.smtpSender.IsSMTPConfigured() {
		return false, ErrSMTPNotConfigured
	}

	// Retry scheduling never steals a live claim: the reset only applies
	// while no worker holds the delivery lease. A future next_attempt_at
	// with a NULL owner is retry backoff and retries right now; a future
	// next_attempt_at with an owner is another instance's in-flight send.
	for _, id := range retryable {
		if _, err := s.db.ExecWrite(`
			UPDATE email_reply_outbox
			SET next_attempt_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND (lease_owner IS NULL OR next_attempt_at <= CURRENT_TIMESTAMP)
		`, id); err != nil {
			return false, fmt.Errorf("reset email reply backoff for retry: %w", err)
		}
	}

	for _, id := range retryable {
		sent, err := s.deliverPendingReply(context.Background(), id)
		if err != nil {
			return delivered, err
		}
		if sent {
			delivered = true
		}
	}
	return delivered, nil
}
