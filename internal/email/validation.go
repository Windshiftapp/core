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

	if config.EmailWorkspaceID == 0 {
		return fmt.Errorf("%w: email_workspace_id is required", ErrConfigNotReady)
	}
	if config.EmailItemTypeID == nil || *config.EmailItemTypeID == 0 {
		return fmt.Errorf("%w: email_item_type_id is required", ErrConfigNotReady)
	}
	if strings.TrimSpace(config.EmailMailbox) == "" {
		return fmt.Errorf("%w: email_mailbox is required (default \"INBOX\")", ErrConfigNotReady)
	}

	switch strings.ToLower(config.EmailAuthMethod) {
	case "oauth":
		if config.EmailOAuthProviderType == "" {
			return fmt.Errorf("%w: email_oauth_provider_type is required", ErrConfigNotReady)
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
	default:
		return fmt.Errorf("%w: unknown email_auth_method %q", ErrConfigNotReady, config.EmailAuthMethod)
	}

	return nil
}
