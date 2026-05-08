package portalwebauthn

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/auth"
	"windshift/internal/models"
)

// PortalLookupStore exposes the read/write paths the portal-passkey handler
// needs that fall outside credential and session storage: portal customer
// lookup, channel-by-slug resolution, channel access checks, credential
// ownership lookup, and the passkey-banner dismissal write. Keeping these
// here lets the handler stay out of internal/database directly.
type PortalLookupStore struct {
	db Database
}

// NewPortalLookupStore wires a lookup helper bound to the given DB handle.
func NewPortalLookupStore(db Database) *PortalLookupStore {
	return &PortalLookupStore{db: db}
}

// ErrPortalCustomerNotFound is returned when the customer row is missing.
var ErrPortalCustomerNotFound = errors.New("portal customer not found")

// ErrPortalChannelNotFound is returned when no enabled portal channel matches
// the slug.
var ErrPortalChannelNotFound = errors.New("portal channel not found")

// ErrCredentialNotFound is returned when the credential ID has no owner row.
var ErrCredentialNotFound = errors.New("portal credential not found")

// GetCustomer loads a portal customer by ID. Wraps sql.ErrNoRows in
// ErrPortalCustomerNotFound so callers can switch on the sentinel without
// importing database/sql.
func (s *PortalLookupStore) GetCustomer(id int) (*auth.PortalCustomer, error) {
	c := &auth.PortalCustomer{}
	var phone sql.NullString
	var orgID sql.NullInt64
	err := s.db.QueryRow(`
		SELECT id, name, email, phone, customer_organisation_id, created_at, updated_at
		FROM portal_customers WHERE id = ?
	`, id).Scan(&c.ID, &c.Name, &c.Email, &phone, &orgID, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPortalCustomerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load portal customer: %w", err)
	}
	if phone.Valid {
		c.Phone = phone.String
	}
	if orgID.Valid {
		v := int(orgID.Int64)
		c.CustomerOrganisationID = &v
	}
	return c, nil
}

// FindEnabledPortalChannelBySlug locates the channel backing a portal slug.
// Mirrors the lookup in portal_auth.go so this store is independent.
func (s *PortalLookupStore) FindEnabledPortalChannelBySlug(slug string) (*models.Channel, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, config, status
		FROM channels WHERE type = 'portal' ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query portal channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var ch models.Channel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Config, &ch.Status); err != nil {
			continue
		}
		var cfg models.ChannelConfig
		if ch.Config != "" {
			if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
				continue
			}
		}
		if cfg.PortalSlug == slug && ch.Status == "enabled" {
			return &ch, nil
		}
	}
	return nil, ErrPortalChannelNotFound
}

// CustomerHasChannelAccess reports whether the portal customer is allowed to
// sign in to the given channel.
func (s *PortalLookupStore) CustomerHasChannelAccess(customerID, channelID int) (bool, error) {
	var ok bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM portal_customer_channels
			WHERE portal_customer_id = ? AND channel_id = ?
		)
	`, customerID, channelID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check portal customer channel access: %w", err)
	}
	return ok, nil
}

// GetCredentialOwner returns the portal customer ID that owns a given
// credential ID. Used to enforce ownership before deletion.
func (s *PortalLookupStore) GetCredentialOwner(credentialID string) (int, error) {
	var ownerID int
	err := s.db.QueryRow(`
		SELECT portal_customer_id FROM portal_webauthn_credentials WHERE id = ?
	`, credentialID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrCredentialNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("load credential owner: %w", err)
	}
	return ownerID, nil
}

// DismissPasskeyPrompt records the moment the customer dismissed the
// "set up a passkey" banner so it doesn't reappear next session.
func (s *PortalLookupStore) DismissPasskeyPrompt(customerID int, now time.Time) error {
	if _, err := s.db.Exec(`
		UPDATE portal_customers SET dismissed_passkey_prompt_at = ?, updated_at = ? WHERE id = ?
	`, now, now, customerID); err != nil {
		return fmt.Errorf("dismiss passkey prompt: %w", err)
	}
	return nil
}
