// Package smtp provides SMTP email sending functionality for notifications,
// including support for batched notification delivery and various encryption methods.
package smtp

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/emailutil"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

// EncryptionModeAllowed reports whether the sender can dispatch with the
// configured mode. Unknown and empty modes are rejected so a typo can never
// silently downgrade a connection to plaintext.
func EncryptionModeAllowed(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "tls", "starttls", "ssl", "none":
		return true
	default:
		return false
	}
}

// ValidateTransport rejects unsupported modes and authentication without TLS.
// Plaintext SMTP is intended for trusted relays that authorize by network;
// credentials must never be exposed on the connection.
func ValidateTransport(config *models.ChannelConfig) error {
	if config == nil {
		return fmt.Errorf("SMTP config is required")
	}
	mode := strings.ToLower(strings.TrimSpace(config.SMTPEncryption))
	if !EncryptionModeAllowed(mode) {
		return fmt.Errorf("SMTP encryption %q not allowed; use \"tls\", \"starttls\", \"ssl\", or \"none\"", config.SMTPEncryption)
	}
	if mode == "none" && (strings.TrimSpace(config.SMTPUsername) != "" || config.SMTPPassword != "") {
		return ErrSMTPAuthenticationRequiresTLS
	}
	return nil
}

// ErrSMTPNotConfigured is returned when a transactional send is attempted but
// SMTP isn't configured. Re-exported by `internal/services` so existing
// callers (e.g. internal/handlers/auth.go) keep working.
var ErrSMTPNotConfigured = errors.New("SMTP is not configured")

// ErrSMTPAuthenticationRequiresTLS is returned when a plaintext channel
// contains credentials that would otherwise be exposed on the connection.
var ErrSMTPAuthenticationRequiresTLS = errors.New("SMTP authentication requires TLS")

// Encryptor mirrors email.Encryptor — duplicated here to avoid an
// smtp→email→services→smtp import cycle. *sso.SecretEncryption (the same
// concrete type passed to email.NewCredentialManager) satisfies both
// interfaces, so production wiring uses one instance.
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

const encryptedSecretPrefix = "enc:v1:"

// decryptOrLegacy mirrors email/credentials.go::DecryptOrLegacy. The legacy-
// plaintext fallback (return value verbatim if it's not a valid AES-GCM
// ciphertext) lets pre-encryption rows keep working through a rolling
// migration of SMTPPassword from plaintext to AES-GCM. AES-GCM minimum is
// 12-byte nonce + 16-byte tag = 28 raw bytes, ~40 base64 chars.
func decryptOrLegacy(enc Encryptor, value string) (string, error) {
	if value == "" || enc == nil {
		return value, nil
	}
	if strings.HasPrefix(value, encryptedSecretPrefix) {
		plain, err := enc.Decrypt(strings.TrimPrefix(value, encryptedSecretPrefix))
		if err != nil {
			return "", fmt.Errorf("decrypt SMTP secret: %w", err)
		}
		return plain, nil
	}
	const minCipherBytes = 28
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(raw) < minCipherBytes {
		return value, nil //nolint:nilerr // legacy-plaintext fallback is intentional
	}
	plain, err := enc.Decrypt(value)
	if err != nil {
		return value, nil //nolint:nilerr // legacy plaintext may itself be long base64
	}
	return plain, nil
}

// smtpDialTimeout caps how long we'll wait to establish a TCP/TLS connection
// to an SMTP server. Without this the scheduler can hang its 5-minute tick
// on a single wedged MX, stalling notifications for every other user.
const smtpDialTimeout = 15 * time.Second
const smtpOperationTimeout = 30 * time.Second

// hostFromAddr returns just the host portion of an SMTP address, handling
// IPv6 bracketed notation safely. The old `strings.Split(addr, ":")[0]` path
// returned "[" for `[::1]:465`, which makes TLS SNI verification impossible.
func hostFromAddr(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// sanitizeHeader strips CR/LF from a string intended for an SMTP header value.
// Values that land here include admin-configured from-names and fully
// user-controlled content (e.g., inbound Subject or Message-ID re-emitted on
// a reply), so a single `\r\n` in the input lets an attacker inject arbitrary
// Bcc/Cc/Reply-To headers downstream.
func sanitizeHeader(s string) string {
	if s == "" {
		return s
	}
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// encodeHeaderWord returns a value safe to use as an unstructured header
// (Subject, From display name, To display name). It strips CR/LF and
// RFC 2047-encodes anything non-ASCII so foreign-language subjects don't
// wire-break the message.
func encodeHeaderWord(s string) string {
	s = sanitizeHeader(s)
	if s == "" {
		return s
	}
	return mime.QEncoding.Encode("utf-8", s)
}

// encodeMailbox formats a "Name <addr>" mailbox header value safely. The
// address is sanitized but not QEncoded (it has to stay RFC 5322 addr-spec).
func encodeMailbox(name, addr string) string {
	addr = sanitizeHeader(addr)
	if name == "" {
		return addr
	}
	return fmt.Sprintf("%s <%s>", encodeHeaderWord(name), addr)
}

// normalizeEnvelopeAddress accepts only a bare addr-spec for SMTP MAIL/RCPT
// commands. Header sanitization alone is insufficient at this layer: display
// names, address lists, or control characters must never reach net/smtp's
// command formatter.
func normalizeEnvelopeAddress(raw string) (string, error) {
	address := strings.TrimSpace(raw)
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address == "" || parsed.Address != address {
		return "", fmt.Errorf("invalid bare email address")
	}
	return address, nil
}

// NotificationSMTPSender handles sending batched notifications via email.
// Encryptor is optional: when non-nil, getSMTPConfig decrypts SMTPPassword
// before handing the config to dispatch(). If unset (e.g. in tests), the
// password is passed through verbatim — matching the legacy-plaintext
// fallback in decryptOrLegacy.
type NotificationSMTPSender struct {
	db         database.Database
	templates  *repository.EmailTemplateRepository
	encryption Encryptor
}

// NewNotificationSMTPSender creates a new SMTP notification sender
func NewNotificationSMTPSender(db database.Database) *NotificationSMTPSender {
	return &NotificationSMTPSender{
		db:        db,
		templates: repository.NewEmailTemplateRepository(db),
	}
}

// SetEncryption wires the at-rest encryption service used to decrypt
// SMTPPassword before SMTP AUTH. Called from server startup with the same
// *sso.SecretEncryption instance that the email/SCM/integration handlers use.
func (s *NotificationSMTPSender) SetEncryption(enc Encryptor) {
	s.encryption = enc
}

// RenderEmail resolves the named email template (admin-edited DB row preferred,
// embedded fallback otherwise) and renders subject/html/text against `data`.
// The rendered Subject is also exposed to the HTML/text body templates as
// `{{.Subject}}` (used by the shared shell's <title> tag), so each call site
// only has to pass its native fields without remembering to mirror the
// subject in the struct.
func (s *NotificationSMTPSender) RenderEmail(templateName string, data any) (subject, htmlBody, textBody string, err error) {
	subjectSrc, htmlSrc, textSrc := s.resolveTemplate(templateName)

	_, subject, err = emailutil.RenderTemplates(subjectSrc, subjectSrc, data)
	if err != nil {
		return "", "", "", err
	}

	enriched := emailutil.EnrichWithSubject(data, subject)
	htmlBody, textBody, err = emailutil.RenderTemplates(htmlSrc, textSrc, enriched)
	if err != nil {
		return "", "", "", err
	}
	return subject, htmlBody, textBody, nil
}

// SendTransactional renders a named template against `data` and sends it via
// SMTP to `toEmail`. Returns ErrSMTPNotConfigured if SMTP isn't set up. This
// is the shared one-call surface used by the magic-link / verification /
// invitation services so each one stays a thin URL-builder + caller.
func (s *NotificationSMTPSender) SendTransactional(toEmail, templateName string, data any) error {
	if !s.IsSMTPConfigured() {
		return ErrSMTPNotConfigured
	}
	subject, htmlBody, textBody, err := s.RenderEmail(templateName, data)
	if err != nil {
		return fmt.Errorf("render %s: %w", templateName, err)
	}
	return s.SendCustomEmail(toEmail, subject, htmlBody, textBody)
}

func (s *NotificationSMTPSender) resolveTemplate(name string) (subject, html, text string) {
	if s.templates != nil {
		if t, err := s.templates.GetByName(name); err == nil && t != nil {
			return t.Subject, t.HTMLBody, t.TextBody
		}
	}
	for _, t := range emailutil.DefaultTemplates() {
		if t.Name == name {
			return t.Subject, t.HTMLBody, t.TextBody
		}
	}
	return "", "", ""
}

// IsSMTPConfigured checks if SMTP is properly configured
func (s *NotificationSMTPSender) IsSMTPConfigured() bool {
	config, err := s.getSMTPConfig()
	if err != nil {
		return false
	}

	// Check that essential SMTP fields are configured
	return config.SMTPHost != "" &&
		config.SMTPPort > 0 &&
		config.SMTPFromEmail != ""
}

// getSMTPConfig retrieves the active SMTP configuration
func (s *NotificationSMTPSender) getSMTPConfig() (*models.ChannelConfig, error) {
	query := `
		SELECT COALESCE(config, '{}') FROM channels
		WHERE type = 'smtp' AND direction = 'outbound'
		  AND status = 'enabled' AND is_default = true
		ORDER BY updated_at DESC
		LIMIT 1
	`

	var configJSON string
	err := s.db.QueryRow(query).Scan(&configJSON)
	if err != nil {
		return nil, fmt.Errorf("no active SMTP configuration found: %w", err)
	}

	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("failed to parse SMTP configuration: %w", err)
	}

	// SMTPPassword stays encrypted in the returned config — dispatch decrypts
	// it just before AUTH PLAIN. Keeping the encrypted value in the struct
	// avoids handing plaintext to any code paths that don't strictly need it.
	return &config, nil
}

// SendBatchedNotifications sends a batch of notifications to a user via email
func (s *NotificationSMTPSender) SendBatchedNotifications(userEmail, userName string, notifications []models.Notification) error {
	return s.SendBatchedNotificationsContext(context.Background(), userEmail, userName, notifications)
}

// SendBatchedNotificationsContext sends a batch with cancellation propagated
// through SMTP dialing, handshake, commands, and DATA completion.
func (s *NotificationSMTPSender) SendBatchedNotificationsContext(ctx context.Context, userEmail, userName string, notifications []models.Notification) error {
	if len(notifications) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	config, err := s.getSMTPConfig()
	if err != nil {
		return fmt.Errorf("failed to get SMTP config: %w", err)
	}

	subject, htmlBody, textBody, err := s.RenderEmail(emailutil.TemplateNotificationBatch, buildNotificationBatchData(userName, notifications))
	if err != nil {
		return fmt.Errorf("failed to render notification email: %w", err)
	}

	return s.sendEmailContext(ctx, config, userEmail, subject, htmlBody, textBody)
}

// notificationBatchEntry mirrors the per-row data exposed to the
// notification_batch template. AccentColor is a hex code used for the
// left-border accent, derived from the notification Type.
type notificationBatchEntry struct {
	Title         string
	Message       string
	Type          string
	AccentColor   string
	FormattedTime string
}

func buildNotificationBatchData(userName string, notifications []models.Notification) any {
	data := struct {
		UserName          string
		NotificationCount int
		Notifications     []notificationBatchEntry
	}{
		UserName:          userName,
		NotificationCount: len(notifications),
	}
	for _, n := range notifications {
		data.Notifications = append(data.Notifications, notificationBatchEntry{
			Title:         n.Title,
			Message:       n.Message,
			Type:          n.Type,
			AccentColor:   notificationAccentColor(n.Type),
			FormattedTime: n.Timestamp.Format("January 2, 2006 at 3:04 PM"),
		})
	}
	return data
}

// notificationAccentColor maps notification types to a brand-aligned accent
// color used as the left border of each card in the email template.
func notificationAccentColor(t string) string {
	switch t {
	case "success":
		return "#10b981"
	case "warning":
		return "#f59e0b"
	case "error":
		return "#ef4444"
	case "assignment":
		return "#8b5cf6"
	case "comment":
		return "#06b6d4"
	case "mention":
		return "#a855f7"
	case "status_change":
		return "#f97316"
	case "reminder":
		return "#84cc16"
	case "milestone":
		return "#ec4899"
	default:
		return "#2874bb"
	}
}

// mimeOptions captures everything that can vary between transactional and
// threaded MIME bodies. Threading-related fields (MessageID, InReplyTo,
// References) and ToName are optional — when empty they are simply omitted
// from the rendered headers, which keeps transactional sends terse and
// threaded sends RFC 5322-correct without two parallel builders.
type mimeOptions struct {
	FromEmail, FromName  string
	ToEmail, ToName      string
	Subject              string
	HTMLBody, TextBody   string
	MessageID, InReplyTo string
	References           []string
}

// buildMime renders a multipart/alternative message. All header values that
// originate from inbound parsing or admin-configured display names go through
// sanitizeHeader/encodeHeaderWord/encodeMailbox so a stray `\r\n` cannot
// inject Bcc/Cc/Reply-To downstream.
func buildMime(opts mimeOptions) string {
	boundary := "----=_NextPart_" + fmt.Sprintf("%d", time.Now().UnixNano())

	from := encodeMailbox(opts.FromName, opts.FromEmail)
	to := sanitizeHeader(opts.ToEmail)
	if opts.ToName != "" {
		to = encodeMailbox(opts.ToName, opts.ToEmail)
	}

	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n",
		from, to, encodeHeaderWord(opts.Subject), boundary)

	if messageID := formatMessageIDHeader(opts.MessageID); messageID != "" {
		headers += fmt.Sprintf("Message-ID: %s\r\n", messageID)
	}
	if inReplyTo := formatMessageIDHeader(opts.InReplyTo); inReplyTo != "" {
		headers += fmt.Sprintf("In-Reply-To: %s\r\n", inReplyTo)
	}
	if len(opts.References) > 0 {
		clean := make([]string, 0, len(opts.References))
		for _, ref := range opts.References {
			if formatted := formatMessageIDHeader(ref); formatted != "" {
				clean = append(clean, formatted)
			}
		}
		if len(clean) > 0 {
			headers += fmt.Sprintf("References: %s\r\n", strings.Join(clean, " "))
		}
	}

	headers += "\r\n"

	textPart := fmt.Sprintf("--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n\r\n",
		boundary, opts.TextBody)
	htmlPart := fmt.Sprintf("--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n\r\n",
		boundary, opts.HTMLBody)
	ending := fmt.Sprintf("--%s--\r\n", boundary)

	return headers + textPart + htmlPart + ending
}

// formatMessageIDHeader tolerates historical tracking rows that stored the
// go-imap ENVELOPE form without angle brackets and always emits RFC 5322's
// bracketed form on the wire.
func formatMessageIDHeader(value string) string {
	value = strings.TrimSpace(sanitizeHeader(value))
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">"))
	if value == "" || strings.ContainsAny(value, "<> \t") {
		return ""
	}
	return "<" + value + ">"
}

// dispatch picks the configured transport and sends the assembled MIME
// message. Shared by sendEmail and SendThreadedEmail so the encryption switch
// lives in exactly one place. Plaintext SMTP is allowed only without
// authentication; empty or unknown modes are errors rather than downgrades.
//
// dispatch is a method (rather than a free function) so it can decrypt the
// at-rest SMTPPassword before passing it to AUTH PLAIN — every caller goes
// through the encryption-aware sender, even the channel-test path that loads
// raw config from the DB on its own.
func (s *NotificationSMTPSender) dispatch(config *models.ChannelConfig, toEmail, message string) error {
	return s.dispatchContext(context.Background(), config, toEmail, message)
}

func (s *NotificationSMTPSender) dispatchContext(ctx context.Context, config *models.ChannelConfig, toEmail, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if config == nil {
		return fmt.Errorf("SMTP config is required")
	}
	fromEmail, err := normalizeEnvelopeAddress(config.SMTPFromEmail)
	if err != nil {
		return fmt.Errorf("invalid SMTP from address: %w", err)
	}
	toEmail, err = normalizeEnvelopeAddress(toEmail)
	if err != nil {
		return fmt.Errorf("invalid SMTP recipient address: %w", err)
	}
	if strings.TrimSpace(config.SMTPHost) == "" || config.SMTPPort <= 0 || config.SMTPPort > 65535 {
		return fmt.Errorf("invalid SMTP host or port")
	}
	if err := ValidateTransport(config); err != nil {
		return err
	}
	password, err := decryptOrLegacy(s.encryption, config.SMTPPassword)
	if err != nil {
		return err
	}

	var auth smtp.Auth
	if config.SMTPUsername != "" && password != "" {
		auth = smtp.PlainAuth("", config.SMTPUsername, password, config.SMTPHost)
	}

	addr := fmt.Sprintf("%s:%d", config.SMTPHost, config.SMTPPort)

	switch strings.ToLower(strings.TrimSpace(config.SMTPEncryption)) {
	case "tls", "starttls":
		return sendWithStartTLSContext(ctx, addr, auth, fromEmail, toEmail, message, config.SMTPSkipTLSVerify)
	case "ssl":
		return sendWithSSLContext(ctx, addr, auth, fromEmail, toEmail, message, config.SMTPSkipTLSVerify)
	case "none":
		return sendPlaintextContext(ctx, addr, fromEmail, toEmail, message)
	default:
		return fmt.Errorf("SMTP encryption %q not allowed", config.SMTPEncryption)
	}
}

// sendEmail sends a transactional (non-threaded) email using the SMTP config.
func (s *NotificationSMTPSender) sendEmail(config *models.ChannelConfig, toEmail, subject, htmlBody, textBody string) error {
	return s.sendEmailContext(context.Background(), config, toEmail, subject, htmlBody, textBody)
}

func (s *NotificationSMTPSender) sendEmailContext(ctx context.Context, config *models.ChannelConfig, toEmail, subject, htmlBody, textBody string) error {
	return s.dispatchContext(ctx, config, toEmail, buildMime(mimeOptions{
		FromEmail: config.SMTPFromEmail,
		FromName:  config.SMTPFromName,
		ToEmail:   toEmail,
		Subject:   subject,
		HTMLBody:  htmlBody,
		TextBody:  textBody,
	}))
}

// sendWithStartTLS sends email using STARTTLS encryption. The dial goes
// through utils.SafeNetDialer so a maliciously-configured SMTPHost cannot
// reach loopback / private-IP / link-local / CGNAT targets.
func sendWithStartTLSContext(ctx context.Context, addr string, auth smtp.Auth, from, to, message string, skipTLSVerify bool) error {
	conn, err := utils.SafeNetDialer(smtpDialTimeout).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	if err := setSMTPDeadline(ctx, conn); err != nil {
		_ = conn.Close()
		return err
	}
	stopCancellation := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellation()
	client, err := smtp.NewClient(conn, hostFromAddr(addr))
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = client.Close() }()

	tlsConfig := smtpTLSConfig(addr, skipTLSVerify)

	if err = client.StartTLS(tlsConfig); err != nil { //nolint:gocritic
		return err
	}

	return sendWithClientContext(ctx, client, auth, from, to, message)
}

// sendWithSSL sends email using SSL/TLS encryption. SafeNetDialer enforces
// the SSRF reject list before the TLS handshake.
func sendWithSSLContext(ctx context.Context, addr string, auth smtp.Auth, from, to, message string, skipTLSVerify bool) error {
	tlsConfig := smtpTLSConfig(addr, skipTLSVerify)
	rawConn, err := utils.SafeNetDialer(smtpDialTimeout).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	if err := setSMTPDeadline(ctx, rawConn); err != nil {
		_ = rawConn.Close()
		return err
	}
	stopCancellation := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stopCancellation()
	conn := tls.Client(rawConn, tlsConfig)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, hostFromAddr(addr))
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	return sendWithClientContext(ctx, client, auth, from, to, message)
}

func smtpTLSConfig(addr string, skipTLSVerify bool) *tls.Config {
	config := utils.OutboundTLSConfig(hostFromAddr(addr))
	config.InsecureSkipVerify = config.InsecureSkipVerify || skipTLSVerify //nolint:gosec // Explicit SMTP channel or process-wide opt-in.
	return config
}

// sendPlaintext sends unauthenticated email over an unencrypted connection.
// SafeNetDialer keeps the process-wide local-connection policy effective for
// this transport just as it is for TLS SMTP.
func sendPlaintextContext(ctx context.Context, addr, from, to, message string) error {
	conn, err := utils.SafeNetDialer(smtpDialTimeout).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	if err := setSMTPDeadline(ctx, conn); err != nil {
		_ = conn.Close()
		return err
	}
	stopCancellation := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellation()
	client, err := smtp.NewClient(conn, hostFromAddr(addr))
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = client.Close() }()

	return sendWithClientContext(ctx, client, nil, from, to, message)
}

// sendWithClient performs authentication, addressing, and message delivery on an established SMTP client.
func sendWithClientContext(ctx context.Context, client *smtp.Client, auth smtp.Auth, from, to, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil { //nolint:gocritic
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil { //nolint:gocritic
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil { //nolint:gocritic
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return err
	}
	// DATA is not accepted until the writer is closed; servers report final
	// policy/recipient failures (for example 550) here. Discarding this error
	// falsely reported rejected mail as delivered.
	return writer.Close()
}

func setSMTPDeadline(ctx context.Context, conn net.Conn) error {
	deadline := time.Now().Add(smtpOperationTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	return conn.SetDeadline(deadline)
}

// SendCustomEmail sends a custom email with the provided subject and body
// This is used for transactional emails like email verification
func (s *NotificationSMTPSender) SendCustomEmail(toEmail, subject, htmlBody, textBody string) error {
	// Get SMTP configuration
	config, err := s.getSMTPConfig()
	if err != nil {
		return fmt.Errorf("failed to get SMTP config: %w", err)
	}

	// Send email
	return s.sendEmail(config, toEmail, subject, htmlBody, textBody)
}

// SendEmailWithConfig sends an email using a provided config (for testing channels)
// This allows channel test emails to use the same sending logic as production emails
func (s *NotificationSMTPSender) SendEmailWithConfig(config *models.ChannelConfig, toEmail, subject, htmlBody, textBody string) error {
	return s.sendEmail(config, toEmail, subject, htmlBody, textBody)
}

// ThreadedEmailParams contains the parameters for sending a threaded email reply.
type ThreadedEmailParams struct {
	ToEmail    string
	ToName     string
	Subject    string
	HTMLBody   string
	TextBody   string
	MessageID  string
	InReplyTo  string
	References []string
}

// SendThreadedEmail sends an email with RFC 5322 threading headers
// (Message-ID, In-Reply-To, References) so reply-tracking email clients
// keep the conversation grouped.
func (s *NotificationSMTPSender) SendThreadedEmail(params ThreadedEmailParams) error {
	config, err := s.getSMTPConfig()
	if err != nil {
		return fmt.Errorf("failed to get SMTP config: %w", err)
	}
	return s.dispatch(config, params.ToEmail, buildMime(mimeOptions{
		FromEmail:  config.SMTPFromEmail,
		FromName:   config.SMTPFromName,
		ToEmail:    params.ToEmail,
		ToName:     params.ToName,
		Subject:    params.Subject,
		HTMLBody:   params.HTMLBody,
		TextBody:   params.TextBody,
		MessageID:  params.MessageID,
		InReplyTo:  params.InReplyTo,
		References: params.References,
	}))
}
