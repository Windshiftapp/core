package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// PortalAuthRepository contains persistence helpers used by the portal auth
// cookie-auth handler. Keeping these lookups here lets the handler stay free
// of direct database access.
type PortalAuthRepository struct {
	db          database.Database
	channelRepo *ChannelRepository
}

func NewPortalAuthRepository(db database.Database) *PortalAuthRepository {
	return &PortalAuthRepository{db: db, channelRepo: NewChannelRepository(db)}
}

// PortalCustomerSessionInfo contains customer metadata needed by /portal/*/auth/me.
type PortalCustomerSessionInfo struct {
	PasskeyCount             int
	DismissedPasskeyPromptAt *time.Time
}

// FindPortalBySlug resolves an enabled portal channel by its public slug.
func (r *PortalAuthRepository) FindPortalBySlug(ctx context.Context, slug string) (*models.Channel, *models.ChannelConfig, error) {
	candidate, err := r.channelRepo.FindEnabledByPublicSlug(ctx, "portal", slug)
	if err != nil {
		return nil, nil, err
	}
	if candidate.Config.PortalSlug != slug {
		return nil, nil, ErrNotFound
	}
	return &candidate.Channel, &candidate.Config, nil
}

// CustomerEmailHasChannelAccess reports whether an existing portal customer
// with the supplied email has been granted access to the channel.
func (r *PortalAuthRepository) CustomerEmailHasChannelAccess(ctx context.Context, email string, channelID int) (bool, error) {
	var hasAccess bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM portal_customer_channels pcc
			JOIN portal_customers pc ON pc.id = pcc.portal_customer_id
			WHERE LOWER(pc.email) = LOWER(?) AND pcc.channel_id = ?
		)
	`, email, channelID).Scan(&hasAccess)
	if err != nil {
		return false, fmt.Errorf("check portal customer access: %w", err)
	}
	return hasAccess, nil
}

// GetCustomerSessionInfo loads passkey metadata for the authenticated portal customer.
func (r *PortalAuthRepository) GetCustomerSessionInfo(ctx context.Context, customerID int) (*PortalCustomerSessionInfo, error) {
	var out PortalCustomerSessionInfo
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM portal_webauthn_credentials WHERE portal_customer_id = ?
	`, customerID).Scan(&out.PasskeyCount); err != nil {
		return nil, fmt.Errorf("count portal customer passkeys: %w", err)
	}

	var dismissedAt sql.NullTime
	if err := r.db.QueryRowContext(ctx, `
		SELECT dismissed_passkey_prompt_at FROM portal_customers WHERE id = ?
	`, customerID).Scan(&dismissedAt); err != nil {
		return nil, fmt.Errorf("load dismissed passkey prompt: %w", err)
	}
	if dismissedAt.Valid {
		out.DismissedPasskeyPromptAt = &dismissedAt.Time
	}
	return &out, nil
}
