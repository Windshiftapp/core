package services

import "windshift/internal/smtp"

// ErrSMTPNotConfigured aliases smtp.ErrSMTPNotConfigured so existing handler
// code (e.g. internal/handlers/auth.go) that switches on
// services.ErrSMTPNotConfigured keeps working without an import change.
// A single equality compare against either symbol resolves to the same var.
var ErrSMTPNotConfigured = smtp.ErrSMTPNotConfigured

// TransactionalEmailSender is the surface needed by services that send
// admin-templated transactional emails (magic-link, email-verification,
// invitation). The concrete *smtp.NotificationSMTPSender satisfies this
// interface so production wiring stays unchanged; tests can substitute a
// fake.
type TransactionalEmailSender interface {
	IsSMTPConfigured() bool
	SendTransactional(toEmail, templateName string, data interface{}) error
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
	RenderEmail(templateName string, data interface{}) (subject, htmlBody, textBody string, err error)
}
