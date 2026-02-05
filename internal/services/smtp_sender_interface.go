package services

import "windshift/internal/smtp"

// ThreadedEmailSender abstracts the SMTP sending capability for testing.
type ThreadedEmailSender interface {
	IsSMTPConfigured() bool
	SendThreadedEmail(params smtp.ThreadedEmailParams) error
}
