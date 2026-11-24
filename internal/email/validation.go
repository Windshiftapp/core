package email

import (
	"errors"
	"fmt"
	"strings"

	"windshift/internal/models"
)

// ErrConfigNotReady marks a config that is missing required fields for
// inbound email ingestion. Callers (handlers/ToggleChannel, handlers/UpdateChannelConfig)
// should surface the wrapped message as a 400 so the operator sees the
// concrete missing field instead of a generic enable failure.
var ErrConfigNotReady = errors.New("email channel config is not ready")

// ValidateConfigForEnable verifies that an inbound email channel has the
// minimum fields needed to ingest mail without immediately error-spinning in
// the scheduler. It mirrors the frontend's ChannelEmailConfig.validate plus
// target workspace + item type + mailbox checks. Returns ErrConfigNotReady
// wrapped with the offending field's name.
func ValidateConfigForEnable(channel *models.Channel, config *models.ChannelConfig) error {
	if channel == nil || config == nil {
		return fmt.Errorf("%w: missing channel or config", ErrConfigNotReady)
	}
	if channel.Type != "email" || channel.Direction != "inbound" {
		return nil
	}

	// Routing (workspace/portal/item type/folder) lives on intakes since
	// WI-1644; the channel config only needs a usable connection. Intake
	// presence is validated by the caller, which has DB access.
	if config.EmailRateLimitPerHour != nil && *config.EmailRateLimitPerHour < 0 {
		return fmt.Errorf("%w: email_rate_limit_per_hour must be 0 (unlimited) or a positive number", ErrConfigNotReady)
	}
	if !IsValidEmailDisposition(config.EmailProcessingDisposition) {
		return fmt.Errorf("%w: email_processing_disposition must be leave, mark_read, or delete", ErrConfigNotReady)
	}

	switch strings.ToLower(config.EmailAuthMethod) {
	case "oauth":
		if config.EmailOAuthProviderType != models.EmailProviderTypeMicrosoft && config.EmailOAuthProviderType != models.EmailProviderTypeGoogle {
			return fmt.Errorf("%w: email_oauth_provider_type must be microsoft or google", ErrConfigNotReady)
		}
		if config.EmailOAuthClientID == "" {
			return fmt.Errorf("%w: email_oauth_client_id is required", ErrConfigNotReady)
		}
		if config.EmailOAuthClientSecret == "" {
			return fmt.Errorf("%w: email_oauth_client_secret is required", ErrConfigNotReady)
		}
		// Refresh token presence signals "OAuth completed at least once". A
		// channel enabled without it will just fail on first poll. Require it
		// up front so the operator sees the issue immediately.
		if config.EmailOAuthRefreshToken == "" {
			return fmt.Errorf("%w: OAuth flow not completed (no refresh token)", ErrConfigNotReady)
		}
		// A mailbox change clears the tokens, but guard the invariant directly
		// too: a connection verified for one address must not be enabled for a
		// different one without reconnecting.
		if config.EmailOAuthMailbox != "" && !strings.EqualFold(strings.TrimSpace(config.EmailOAuthMailbox), strings.TrimSpace(config.EmailOAuthEmail)) {
			return fmt.Errorf("%w: the connected mailbox does not match the configured mailbox; reconnect OAuth", ErrConfigNotReady)
		}
	case "basic", "":
		// Empty defaults to basic for back-compat with channels saved before
		// EmailAuthMethod was introduced.
		if config.IMAPHost == "" {
			return fmt.Errorf("%w: imap_host is required", ErrConfigNotReady)
		}
		if config.IMAPUsername == "" {
			return fmt.Errorf("%w: imap_username is required", ErrConfigNotReady)
		}
		if config.IMAPPassword == "" {
			return fmt.Errorf("%w: imap_password is required", ErrConfigNotReady)
		}
		if config.IMAPPort <= 0 || config.IMAPPort > 65535 {
			return fmt.Errorf("%w: imap_port must be between 1 and 65535", ErrConfigNotReady)
		}
		switch strings.ToLower(config.IMAPEncryption) {
		case "ssl", "tls", "starttls":
		default:
			return fmt.Errorf("%w: imap_encryption must be ssl or starttls", ErrConfigNotReady)
		}
	default:
		return fmt.Errorf("%w: unknown email_auth_method %q", ErrConfigNotReady, config.EmailAuthMethod)
	}

	return nil
}

// ResolveEmailDisposition returns the effective post-processing action for a
// channel. The explicit enum wins; an empty value (legacy config) maps the two
// booleans, with delete taking precedence over mark-read.
func ResolveEmailDisposition(config *models.ChannelConfig) string {
	if config == nil {
		return models.EmailDispositionLeave
	}
	if config.EmailProcessingDisposition != "" && IsValidEmailDisposition(config.EmailProcessingDisposition) {
		return config.EmailProcessingDisposition
	}
	if config.EmailDeleteAfterProcess {
		return models.EmailDispositionDelete
	}
	if config.EmailMarkAsRead {
		return models.EmailDispositionMarkRead
	}
	return models.EmailDispositionLeave
}

// IsValidEmailDisposition reports whether value is a recognized disposition or
// the empty legacy sentinel.
func IsValidEmailDisposition(value string) bool {
	switch value {
	case "", models.EmailDispositionLeave, models.EmailDispositionMarkRead, models.EmailDispositionDelete:
		return true
	default:
		return false
	}
}
