// Package smtp provides SMTP email sending functionality for notifications,
// including support for batched notification delivery and various encryption methods.
package smtp

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/emailutil"
	"windshift/internal/models"
)

// smtpDialTimeout caps how long we'll wait to establish a TCP/TLS connection
// to an SMTP server. Without this the scheduler can hang its 5-minute tick
// on a single wedged MX, stalling notifications for every other user.
const smtpDialTimeout = 15 * time.Second

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

// NotificationSMTPSender handles sending batched notifications via email
type NotificationSMTPSender struct {
	db database.Database
}

// NewNotificationSMTPSender creates a new SMTP notification sender
func NewNotificationSMTPSender(db database.Database) *NotificationSMTPSender {
	return &NotificationSMTPSender{
		db: db,
	}
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
		SELECT config FROM channels
		WHERE type = 'smtp' AND direction = 'outbound' AND status = 'enabled'
		ORDER BY updated_at DESC, is_default DESC
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

	return &config, nil
}

// SendBatchedNotifications sends a batch of notifications to a user via email
func (s *NotificationSMTPSender) SendBatchedNotifications(userEmail, userName string, notifications []models.Notification) error {
	if len(notifications) == 0 {
		return nil
	}

	// Get SMTP configuration
	config, err := s.getSMTPConfig()
	if err != nil {
		return fmt.Errorf("failed to get SMTP config: %w", err)
	}

	// Generate email content
	subject := s.generateSubject(len(notifications))
	htmlBody, textBody, err := s.generateEmailBody(userName, notifications)
	if err != nil {
		return fmt.Errorf("failed to generate email body: %w", err)
	}

	// Send email
	return s.sendEmail(config, userEmail, subject, htmlBody, textBody)
}

// generateSubject creates the email subject based on notification count
func (s *NotificationSMTPSender) generateSubject(count int) string {
	if count == 1 {
		return "Windshift - You have 1 new notification"
	}
	return fmt.Sprintf("Windshift - You have %d new notifications", count)
}

// generateEmailBody generates the HTML and text email body
func (s *NotificationSMTPSender) generateEmailBody(userName string, notifications []models.Notification) (htmlBody, textBody string, err error) {
	// HTML template
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Windshift Notifications</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 0; background-color: #f5f5f5; }
		.container { max-width: 600px; margin: 0 auto; background-color: white; }
		.header { background-color: #2563eb; color: white; padding: 24px; text-align: center; }
		.header h1 { margin: 0; font-size: 24px; font-weight: 600; }
		.content { padding: 24px; }
		.greeting { font-size: 16px; color: #374151; margin-bottom: 24px; }
		.notification { border-left: 4px solid #e5e7eb; padding: 16px; margin-bottom: 16px; background-color: #f9fafb; }
		.notification:last-child { margin-bottom: 0; }
		.notification.info { border-left-color: #3b82f6; }
		.notification.success { border-left-color: #10b981; }
		.notification.warning { border-left-color: #f59e0b; }
		.notification.error { border-left-color: #ef4444; }
		.notification.assignment { border-left-color: #8b5cf6; }
		.notification.comment { border-left-color: #06b6d4; }
		.notification.mention { border-left-color: #a855f7; }
		.notification.status_change { border-left-color: #f97316; }
		.notification.reminder { border-left-color: #84cc16; }
		.notification.milestone { border-left-color: #ec4899; }
		.notification-title { font-weight: 600; font-size: 14px; color: #111827; margin-bottom: 8px; }
		.notification-message { font-size: 14px; color: #4b5563; line-height: 1.5; }
		.notification-time { font-size: 12px; color: #9ca3af; margin-top: 8px; }
		.footer { background-color: #f9fafb; padding: 24px; text-align: center; font-size: 14px; color: #6b7280; border-top: 1px solid #e5e7eb; }
		.footer a { color: #2563eb; text-decoration: none; }
		.footer a:hover { text-decoration: underline; }
		.unsubscribe { font-size: 12px; color: #9ca3af; margin-top: 16px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Windshift - Work Management</h1>
		</div>
		<div class="content">
			<div class="greeting">
				Hello {{.UserName}},
			</div>
			<div class="greeting">
				You have {{.NotificationCount}} new notification{{if ne .NotificationCount 1}}s{{end}} from Windshift:
			</div>
			{{range .Notifications}}
			<div class="notification {{.Type}}">
				<div class="notification-title">{{.Title}}</div>
				<div class="notification-message">{{.Message}}</div>
				<div class="notification-time">{{.FormattedTime}}</div>
			</div>
			{{end}}
		</div>
		<div class="footer">
			<p>
				This is an automated notification from <strong>Windshift - Work Management</strong>.<br>
				<a href="#">View all notifications in Windshift</a>
			</p>
			<div class="unsubscribe">
				To manage your notification preferences, please contact your administrator.
			</div>
		</div>
	</div>
</body>
</html>`

	// Text template
	textTemplate := `Windshift - Work Management

Hello {{.UserName}},

You have {{.NotificationCount}} new notification{{if ne .NotificationCount 1}}s{{end}} from Windshift:

{{range .Notifications}}
• {{.Title}}
  {{.Message}}
  {{.FormattedTime}}

{{end}}
---
This is an automated notification from Windshift - Work Management.
To manage your notification preferences, please contact your administrator.`

	// Prepare template data
	templateData := struct {
		UserName          string
		NotificationCount int
		Notifications     []struct {
			Title         string
			Message       string
			Type          string
			FormattedTime string
		}
	}{
		UserName:          userName,
		NotificationCount: len(notifications),
	}

	// Process notifications for template
	for _, n := range notifications {
		templateData.Notifications = append(templateData.Notifications, struct {
			Title         string
			Message       string
			Type          string
			FormattedTime string
		}{
			Title:         n.Title,
			Message:       n.Message,
			Type:          n.Type,
			FormattedTime: n.Timestamp.Format("January 2, 2006 at 3:04 PM"),
		})
	}

	return emailutil.RenderTemplates(htmlTemplate, textTemplate, templateData)
}

// sendEmail sends an email using the SMTP configuration
func (s *NotificationSMTPSender) sendEmail(config *models.ChannelConfig, toEmail, subject, htmlBody, textBody string) error {
	// Build message
	message := s.buildMimeMessage(config.SMTPFromEmail, config.SMTPFromName, toEmail, subject, htmlBody, textBody)

	// Set up authentication
	var auth smtp.Auth
	if config.SMTPUsername != "" && config.SMTPPassword != "" {
		auth = smtp.PlainAuth("", config.SMTPUsername, config.SMTPPassword, config.SMTPHost)
	}

	// Determine server address
	addr := fmt.Sprintf("%s:%d", config.SMTPHost, config.SMTPPort)

	// Handle different encryption types
	switch strings.ToLower(config.SMTPEncryption) {
	case "tls":
		return s.sendWithStartTLS(addr, auth, config.SMTPFromEmail, toEmail, message)
	case "ssl":
		return s.sendWithSSL(addr, auth, config.SMTPFromEmail, toEmail, message)
	default: // "none" or empty
		return smtp.SendMail(addr, auth, config.SMTPFromEmail, []string{toEmail}, []byte(message))
	}
}

// buildMimeMessage builds a MIME multipart message with both HTML and text parts
func (s *NotificationSMTPSender) buildMimeMessage(fromEmail, fromName, toEmail, subject, htmlBody, textBody string) string {
	boundary := "----=_NextPart_" + fmt.Sprintf("%d", time.Now().UnixNano())

	from := encodeMailbox(fromName, fromEmail)
	to := sanitizeHeader(toEmail)

	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n",
		from, to, encodeHeaderWord(subject), boundary)

	textPart := fmt.Sprintf("--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n\r\n",
		boundary, textBody)

	htmlPart := fmt.Sprintf("--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n\r\n",
		boundary, htmlBody)

	ending := fmt.Sprintf("--%s--\r\n", boundary)

	return headers + textPart + htmlPart + ending
}

// sendWithStartTLS sends email using STARTTLS encryption
func (s *NotificationSMTPSender) sendWithStartTLS(addr string, auth smtp.Auth, from, to, message string) error {
	conn, err := net.DialTimeout("tcp", addr, smtpDialTimeout)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, hostFromAddr(addr))
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = client.Close() }()

	tlsConfig := &tls.Config{
		ServerName: hostFromAddr(addr),
		MinVersion: tls.VersionTLS12,
	}

	if err = client.StartTLS(tlsConfig); err != nil { //nolint:gocritic
		return err
	}

	return sendWithClient(client, auth, from, to, message)
}

// sendWithSSL sends email using SSL/TLS encryption
func (s *NotificationSMTPSender) sendWithSSL(addr string, auth smtp.Auth, from, to, message string) error {
	tlsConfig := &tls.Config{
		ServerName: hostFromAddr(addr),
		MinVersion: tls.VersionTLS12,
	}

	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, hostFromAddr(addr))
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	return sendWithClient(client, auth, from, to, message)
}

// sendWithClient performs authentication, addressing, and message delivery on an established SMTP client.
func sendWithClient(client *smtp.Client, auth smtp.Auth, from, to, message string) error {
	if auth != nil {
		if err := client.Auth(auth); err != nil { //nolint:gocritic
			return err
		}
	}

	if err := client.Mail(from); err != nil { //nolint:gocritic
		return err
	}

	if err := client.Rcpt(to); err != nil { //nolint:gocritic
		return err
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	defer func() { _ = writer.Close() }()

	_, err = writer.Write([]byte(message))
	return err
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

// SendThreadedEmail sends an email with RFC 5322 threading headers (Message-ID, In-Reply-To, References).
func (s *NotificationSMTPSender) SendThreadedEmail(params ThreadedEmailParams) error {
	config, err := s.getSMTPConfig()
	if err != nil {
		return fmt.Errorf("failed to get SMTP config: %w", err)
	}

	message := s.buildThreadedMimeMessage(config.SMTPFromEmail, config.SMTPFromName, params)

	var auth smtp.Auth
	if config.SMTPUsername != "" && config.SMTPPassword != "" {
		auth = smtp.PlainAuth("", config.SMTPUsername, config.SMTPPassword, config.SMTPHost)
	}

	addr := fmt.Sprintf("%s:%d", config.SMTPHost, config.SMTPPort)

	switch strings.ToLower(config.SMTPEncryption) {
	case "tls":
		return s.sendWithStartTLS(addr, auth, config.SMTPFromEmail, params.ToEmail, message)
	case "ssl":
		return s.sendWithSSL(addr, auth, config.SMTPFromEmail, params.ToEmail, message)
	default:
		return smtp.SendMail(addr, auth, config.SMTPFromEmail, []string{params.ToEmail}, []byte(message))
	}
}

// buildThreadedMimeMessage builds a MIME multipart message with threading headers.
// Every header value flowing in here is sanitized against CRLF injection because
// MessageID / InReplyTo / References originate from inbound email parsing and
// ToName is derived from the sender's From display name.
func (s *NotificationSMTPSender) buildThreadedMimeMessage(fromEmail, fromName string, params ThreadedEmailParams) string {
	boundary := "----=_NextPart_" + fmt.Sprintf("%d", time.Now().UnixNano())

	from := encodeMailbox(fromName, fromEmail)
	to := encodeMailbox(params.ToName, params.ToEmail)

	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n",
		from, to, encodeHeaderWord(params.Subject), boundary)

	if params.MessageID != "" {
		headers += fmt.Sprintf("Message-ID: %s\r\n", sanitizeHeader(params.MessageID))
	}
	if params.InReplyTo != "" {
		headers += fmt.Sprintf("In-Reply-To: %s\r\n", sanitizeHeader(params.InReplyTo))
	}
	if len(params.References) > 0 {
		cleanRefs := make([]string, 0, len(params.References))
		for _, ref := range params.References {
			cleanRefs = append(cleanRefs, sanitizeHeader(ref))
		}
		headers += fmt.Sprintf("References: %s\r\n", strings.Join(cleanRefs, " "))
	}

	headers += "\r\n"

	textPart := fmt.Sprintf("--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n\r\n",
		boundary, params.TextBody)

	htmlPart := fmt.Sprintf("--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n\r\n",
		boundary, params.HTMLBody)

	ending := fmt.Sprintf("--%s--\r\n", boundary)

	return headers + textPart + htmlPart + ending
}
