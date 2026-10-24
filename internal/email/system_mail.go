package email

import (
	"net/mail"
	"strings"
)

// SystemMailCategory identifies machine-generated inbound mail that must not
// become a ticket or comment. Values are persisted verbatim in
// email_message_tracking.skipped_reason and rendered by the admin diagnostics
// view, so they are a stable contract.
type SystemMailCategory string

const (
	// SystemMailDeliveryFailure is a bounce / delivery status notification
	// (DSN): mail returned by a mail system because a message could not be
	// delivered.
	SystemMailDeliveryFailure SystemMailCategory = "delivery_failure"
	// SystemMailAutoReply is an RFC 3834 auto-reply, such as an out-of-office
	// or vacation responder.
	SystemMailAutoReply SystemMailCategory = "auto_reply"
)

// SystemMailInfo describes a detected machine-generated message.
type SystemMailInfo struct {
	Category SystemMailCategory
	// Detail is a short, operator-facing explanation of what matched.
	Detail string
}

// deliveryFailureSubjectPrefixes are the subject stems used by the common
// bounce generators (Exchange/Outlook, Postfix, Sendmail, Google). Matching is
// case-insensitive and anchored at the start of the subject.
var deliveryFailureSubjectPrefixes = []string{
	"undeliverable:",
	"undelivered mail returned to sender",
	"delivery status notification",
	"delivery has failed",
	"delivery failure",
	"mail delivery failed",
	"mail delivery failure",
	"message not delivered",
	"failed to deliver message",
	"returned mail:",
	"return to sender",
	"failure notice",
	"could not be delivered",
	"undeliverable mail",
}

// deliveryFailureBodyMarkers are phrases emitted by the standard bounce
// templates. They are matched against the plain-text body, lowercased.
var deliveryFailureBodyMarkers = []string{
	"diagnostic information for administrators",
	"delivery has failed to these recipients",
	"your message could not be delivered",
	"your message wasn't delivered",
	"your message was not delivered",
	"this is the mail system at host",
	"a message that you sent could not be delivered",
	"the following address(es) failed",
	"this is an automatically generated delivery status notification",
}

// autoReplySubjectPrefixes are subject stems used by out-of-office and
// vacation responders.
var autoReplySubjectPrefixes = []string{
	"automatic reply:",
	"automatic response:",
	"auto reply:",
	"auto response:",
	"autoreply:",
	"autoresponse:",
	"out of office",
	"out-of-office",
	"abwesenheit",
	"abwesenheitsnotiz",
}

// mailerDaemonLocalParts are the local parts mail systems use for automated
// bounce senders.
var mailerDaemonLocalParts = map[string]bool{
	"mailer-daemon": true,
	"mail-daemon":   true,
	"postmaster":    true,
}

// ClassifySystemMail reports whether an inbound message is machine-generated
// and should be recorded as a diagnostic instead of a ticket. Detection is
// intentionally conservative: only delivery status notifications (bounces)
// and RFC 3834 auto-replies are matched, because those can never carry a
// customer request and would otherwise create junk tickets or comments. Other
// automated senders (monitoring alerts, CI notifications, mailing lists) are
// left alone.
func ClassifySystemMail(email *ParsedEmail) (SystemMailInfo, bool) {
	if email == nil {
		return SystemMailInfo{}, false
	}
	if detail, ok := detectDeliveryFailure(email); ok {
		return SystemMailInfo{Category: SystemMailDeliveryFailure, Detail: detail}, true
	}
	if detail, ok := detectAutoReply(email); ok {
		return SystemMailInfo{Category: SystemMailAutoReply, Detail: detail}, true
	}
	return SystemMailInfo{}, false
}

// detectDeliveryFailure matches DSNs by MIME report type, sender, subject, and
// body template. Any single signal is enough: bounce templates vary widely and
// a false positive only moves mail to the diagnostics view rather than a
// ticket, which is the intended outcome for machine-generated mail.
func detectDeliveryFailure(email *ParsedEmail) (string, bool) {
	if contentType := headerFirst(email, "Content-Type"); contentType != "" {
		lower := strings.ToLower(contentType)
		if strings.Contains(lower, "report-type=delivery-status") ||
			(strings.Contains(lower, "multipart/report") && strings.Contains(lower, "delivery-status")) {
			return "Content-Type " + contentType, true
		}
	}

	if local, ok := localPart(email.From.Address); ok && mailerDaemonLocalParts[local] {
		return "sender " + email.From.Address, true
	}

	subject := strings.ToLower(strings.TrimSpace(email.Subject))
	for _, prefix := range deliveryFailureSubjectPrefixes {
		if strings.HasPrefix(subject, prefix) {
			return "subject " + strings.TrimSpace(email.Subject), true
		}
	}

	body := strings.ToLower(email.GetBodyText())
	for _, marker := range deliveryFailureBodyMarkers {
		if strings.Contains(body, marker) {
			return "body marker: " + marker, true
		}
	}

	return "", false
}

// detectAutoReply matches RFC 3834 auto-replies and out-of-office subjects.
// Auto-Submitted: auto-generated is deliberately excluded: it is also used by
// legitimate notification senders, and only auto-replied/auto-notified
// unambiguously mark a reply to a human message.
func detectAutoReply(email *ParsedEmail) (string, bool) {
	if value := strings.TrimSpace(headerFirst(email, "Auto-Submitted")); value != "" {
		switch strings.ToLower(value) {
		case "auto-replied", "auto-notified":
			return "Auto-Submitted: " + value, true
		}
	}

	subject := strings.ToLower(strings.TrimSpace(email.Subject))
	for _, prefix := range autoReplySubjectPrefixes {
		if strings.HasPrefix(subject, prefix) {
			return "subject " + strings.TrimSpace(email.Subject), true
		}
	}

	return "", false
}

// headerFirst returns the first value of a header, looked up case-insensitively
// because RawHeaders may come from either IMAP or raw RFC 5322 parsing.
func headerFirst(email *ParsedEmail, name string) string {
	if email == nil {
		return ""
	}
	for key, values := range email.RawHeaders {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// localPart returns the lowercased local part of an email address.
func localPart(address string) (string, bool) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", false
	}
	if parsed, err := mail.ParseAddress(address); err == nil {
		address = parsed.Address
	}
	at := strings.LastIndex(address, "@")
	if at <= 0 {
		return "", false
	}
	return strings.ToLower(address[:at]), true
}
