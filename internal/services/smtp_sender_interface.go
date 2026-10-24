package services

import (
	"context"
	"errors"

	"windshift/internal/smtp"
)

// ErrSMTPNotConfigured aliases smtp.ErrSMTPNotConfigured so existing handler
// code (e.g. internal/handlers/auth.go) that switches on
// services.ErrSMTPNotConfigured keeps working without an import change.
// A single equality compare against either symbol resolves to the same var.
var ErrSMTPNotConfigured = smtp.ErrSMTPNotConfigured

// ErrRecipientIsAgent is returned by transactional senders when the
// recipient User row is flagged is_agent (owned agent or admin-provisioned
// service user). Agents do not consume invitation, verification, or other
// human-facing mail; handlers that race against this should treat the
// error as a soft skip, not a delivery failure.
var ErrRecipientIsAgent = errors.New("recipient is an agent or service user; refusing to send")

// TransactionalEmailSender is the surface needed by services that send
// admin-templated transactional emails (magic-link, email-verification,
// invitation). The concrete *smtp.NotificationSMTPSender satisfies this
// interface so production wiring stays unchanged; tests can substitute a
// fake.
type TransactionalEmailSender interface {
	IsSMTPConfigured() bool
	SendTransactional(toEmail, templateName string, data any) error
}

// MentionEmailSender renders and sends the dedicated @mention email. It is a
// narrower surface than ThreadedEmailSender: mentions are fresh, unthreaded
// messages to internal users, so the sender only needs a context-aware
// transactional send of a named template.
type MentionEmailSender interface {
	IsSMTPConfigured() bool
	SendTransactionalContext(ctx context.Context, toEmail, templateName string, data any) error
}

// ThreadedEmailSender abstracts the SMTP sending capability for the threaded
// reply flow. RenderEmail resolves a named template (DB row preferred,
// embedded fallback otherwise) and renders it against the provided data —
// kept separate from TransactionalEmailSender because threaded replies need
// the rendered subject pre-computed by the caller (to keep the "Re: ..."
// header stable across the email-tracking thread).
type ThreadedEmailSender interface {
	IsSMTPConfigured() bool
	SendThreadedEmail(params smtp.ThreadedEmailParams) error
	SendThreadedEmailContext(ctx context.Context, params smtp.ThreadedEmailParams) error
	RenderEmail(templateName string, data any) (subject, htmlBody, textBody string, err error)
}
