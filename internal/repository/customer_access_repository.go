package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

var ErrCustomerAccessDenied = errors.New("customer access denied")
var ErrCustomerAccessPolicyInvalid = errors.New("invalid customer access policy")

type CustomerAccessRepository struct{ db database.Database }

func NewCustomerAccessRepository(db database.Database) *CustomerAccessRepository {
	return &CustomerAccessRepository{db: db}
}

func (r *CustomerAccessRepository) Resolve(ctx context.Context, kind, slug string) (*models.CustomerSurface, error) {
	surface := &models.CustomerSurface{Kind: kind, Slug: slug}
	switch kind {
	case "portal":
		channel, err := NewChannelRepository(r.db).FindEnabledByPublicSlug(ctx, "portal", slug)
		if err != nil {
			return nil, err
		}
		if channel.Config.PortalSlug != slug {
			return nil, ErrNotFound
		}
		surface.ID = channel.Channel.ID
	case "board":
		err := r.db.QueryRowContext(ctx, `SELECT id FROM collections WHERE public_slug = ? AND is_public = true AND is_private = false`, slug).Scan(&surface.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrNotFound
	}
	return surface, nil
}

func (r *CustomerAccessRepository) Surface(ctx context.Context, kind string, id int) (*models.CustomerSurface, error) {
	var slug string
	switch kind {
	case "portal":
		channel, err := NewChannelRepository(r.db).FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		var config models.ChannelConfig
		if channel.Type != "portal" {
			return nil, ErrNotFound
		}
		if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
			return nil, err
		}
		slug = config.PortalSlug
	case "board":
		if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(public_slug, '') FROM collections WHERE id = ?`, id).Scan(&slug); err != nil {
			return nil, err
		}
	default:
		return nil, ErrNotFound
	}
	return &models.CustomerSurface{Kind: kind, ID: id, Slug: slug}, nil
}

func (r *CustomerAccessRepository) Policy(ctx context.Context, kind string, id int) (models.CustomerAccessPolicy, error) {
	policy := models.CustomerAccessPolicy{Mode: "public", OrganisationIDs: []int{}}
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT mode, organisation_ids FROM customer_resource_access WHERE resource_kind = ? AND resource_id = ?`, kind, id).Scan(&policy.Mode, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return policy, nil
	}
	if err != nil {
		return policy, err
	}
	if policy.Mode != "public" && policy.Mode != "organisations" {
		return policy, ErrCustomerAccessDenied
	}
	if err := json.Unmarshal([]byte(raw), &policy.OrganisationIDs); err != nil {
		return policy, err
	}
	for _, orgID := range policy.OrganisationIDs {
		if orgID <= 0 {
			return policy, ErrCustomerAccessDenied
		}
	}
	return policy, nil
}

func (r *CustomerAccessRepository) OrganisationExists(ctx context.Context, id int) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM customer_organisations WHERE id = ?)`, id).Scan(&exists)
	return exists, err
}

func (r *CustomerAccessRepository) SavePolicy(ctx context.Context, surface models.CustomerSurface, policy models.CustomerAccessPolicy) error {
	if policy.Mode != "public" && policy.Mode != "organisations" {
		return fmt.Errorf("%w: access mode must be public or organisations", ErrCustomerAccessPolicyInvalid)
	}
	if len(policy.OrganisationIDs) > 100 {
		return fmt.Errorf("%w: select at most 100 customer organisations", ErrCustomerAccessPolicyInvalid)
	}
	for _, id := range policy.OrganisationIDs {
		exists, err := r.OrganisationExists(ctx, id)
		if err != nil {
			return err
		}
		if id <= 0 || !exists {
			return fmt.Errorf("%w: customer organisation %d does not exist", ErrCustomerAccessPolicyInvalid, id)
		}
	}
	ids := slices.Clone(policy.OrganisationIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if ids == nil {
		ids = []int{}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, err = r.db.ExecWriteContext(ctx, `INSERT INTO customer_resource_access (resource_kind,resource_id,mode,organisation_ids)
	 VALUES (?,?,?,?) ON CONFLICT(resource_kind,resource_id) DO UPDATE SET mode = excluded.mode, organisation_ids = excluded.organisation_ids`, surface.Kind, surface.ID, policy.Mode, string(raw))
	return err
}

func (r *CustomerAccessRepository) Allowed(ctx context.Context, surface models.CustomerSurface, customerID int) (bool, error) {
	policy, err := r.Policy(ctx, surface.Kind, surface.ID)
	if err != nil {
		return false, err
	}
	var orgID sql.NullInt64
	var deactivated sql.NullTime
	err = r.db.QueryRowContext(ctx, `SELECT customer_organisation_id, deactivated_at FROM portal_customers WHERE id = ?`, customerID).Scan(&orgID, &deactivated)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if deactivated.Valid {
		return false, nil
	}
	return policy.Mode == "public" || (orgID.Valid && slices.Contains(policy.OrganisationIDs, int(orgID.Int64))), nil
}

func (r *CustomerAccessRepository) PortalLoginAllowed(ctx context.Context, customerID, channelID int) error {
	allowed, err := r.Allowed(ctx, models.CustomerSurface{Kind: "portal", ID: channelID}, customerID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrCustomerAccessDenied
	}
	return nil
}

// GitHubCustomer links only customer identities and never reassigns an existing organisation.
func (r *CustomerAccessRepository) GitHubCustomer(ctx context.Context, providerID int, externalID, email, name string, orgID int, signup bool) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var id int
	err = tx.QueryRow(`SELECT customer_id FROM customer_external_accounts WHERE provider_id = ? AND external_id = ?`, providerID, externalID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRow(`SELECT id FROM portal_customers WHERE LOWER(email) = ?`, strings.ToLower(strings.TrimSpace(email))).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			if !signup {
				return 0, ErrCustomerAccessDenied
			}
			err = tx.QueryRow(`INSERT INTO portal_customers (name,email,customer_organisation_id,created_via)
			 VALUES (?,?,?,?) ON CONFLICT DO NOTHING RETURNING id`, name, strings.ToLower(strings.TrimSpace(email)), orgID, models.CustomerCreatedViaGitHub).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				err = tx.QueryRow(`SELECT id FROM portal_customers WHERE LOWER(email) = ?`, strings.ToLower(strings.TrimSpace(email))).Scan(&id)
			}
		}
		if err != nil {
			return 0, err
		}
		var existingOrg sql.NullInt64
		var deactivated sql.NullTime
		if err := tx.QueryRow(`SELECT customer_organisation_id,deactivated_at FROM portal_customers WHERE id = ?`, id).Scan(&existingOrg, &deactivated); err != nil {
			return 0, err
		}
		if !existingOrg.Valid || int(existingOrg.Int64) != orgID || deactivated.Valid {
			return 0, ErrCustomerAccessDenied
		}
		if _, err := tx.Exec(`INSERT INTO customer_external_accounts (provider_id,external_id,customer_id) VALUES (?,?,?) ON CONFLICT DO NOTHING`, providerID, externalID, id); err != nil {
			return 0, err
		}
		var linkedID int
		if err := tx.QueryRow(`SELECT customer_id FROM customer_external_accounts WHERE provider_id = ? AND external_id = ?`, providerID, externalID).Scan(&linkedID); err != nil {
			return 0, err
		}
		if linkedID != id {
			return 0, ErrCustomerAccessDenied
		}
	} else if err != nil {
		return 0, err
	}
	var valid bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM portal_customers WHERE id = ? AND customer_organisation_id = ? AND deactivated_at IS NULL)`, id, orgID).Scan(&valid); err != nil {
		return 0, err
	}
	if !valid {
		return 0, ErrCustomerAccessDenied
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func customerTokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func (r *CustomerAccessRepository) StoreSession(ctx context.Context, token string, surface models.CustomerSurface, customerID int, expires time.Time) error {
	_, err := r.db.ExecWriteContext(ctx, `INSERT INTO customer_resource_sessions (token,resource_kind,resource_id,customer_id,expires_at) VALUES (?,?,?,?,?)`, customerTokenHash(token), surface.Kind, surface.ID, customerID, expires)
	return err
}

func (r *CustomerAccessRepository) SessionCustomer(ctx context.Context, token string, surface models.CustomerSurface, now time.Time) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT customer_id FROM customer_resource_sessions WHERE token = ? AND resource_kind = ? AND resource_id = ? AND expires_at > ?`, customerTokenHash(token), surface.Kind, surface.ID, now).Scan(&id)
	return id, err
}

func (r *CustomerAccessRepository) DeleteSession(ctx context.Context, token string) error {
	_, err := r.db.ExecWriteContext(ctx, `DELETE FROM customer_resource_sessions WHERE token = ?`, customerTokenHash(token))
	return err
}

func (r *CustomerAccessRepository) RevokeCustomerCredentials(ctx context.Context, customerID int) error {
	if _, err := r.db.ExecWriteContext(ctx, `DELETE FROM customer_resource_sessions WHERE customer_id = ?`, customerID); err != nil {
		return err
	}
	_, err := r.db.ExecWriteContext(ctx, `DELETE FROM customer_resource_tokens WHERE customer_id = ?`, customerID)
	return err
}

func (r *CustomerAccessRepository) CleanupExpired(ctx context.Context, now time.Time) error {
	if _, err := r.db.ExecWriteContext(ctx, `DELETE FROM customer_resource_sessions WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	_, err := r.db.ExecWriteContext(ctx, `DELETE FROM customer_resource_tokens WHERE expires_at <= ?`, now)
	return err
}

func (r *CustomerAccessRepository) StoreToken(ctx context.Context, token, purpose string, surface models.CustomerSurface, customerID *int, expires time.Time) error {
	_, err := r.db.ExecWriteContext(ctx, `INSERT INTO customer_resource_tokens (token,purpose,resource_kind,resource_id,customer_id,expires_at) VALUES (?,?,?,?,?,?)`, customerTokenHash(token), purpose, surface.Kind, surface.ID, customerID, expires)
	return err
}

func (r *CustomerAccessRepository) ConsumeToken(ctx context.Context, token, purpose string, surface models.CustomerSurface, now time.Time) (int, error) {
	var id sql.NullInt64
	err := r.db.QueryRowContext(ctx, `DELETE FROM customer_resource_tokens WHERE token = ? AND purpose = ? AND resource_kind = ? AND resource_id = ? AND expires_at > ? RETURNING customer_id`, customerTokenHash(token), purpose, surface.Kind, surface.ID, now).Scan(&id)
	return int(id.Int64), err
}

// StoreChallenge keeps board challenges outside the portal WebAuthn session store.
func (r *CustomerAccessRepository) StoreChallenge(ctx context.Context, token string, surface models.CustomerSurface, payload string, expires time.Time) error {
	_, err := r.db.ExecWriteContext(ctx, `INSERT INTO customer_resource_tokens (token,purpose,resource_kind,resource_id,payload,expires_at) VALUES (?,'passkey',?,?,?,?)`, customerTokenHash(token), surface.Kind, surface.ID, payload, expires)
	return err
}

func (r *CustomerAccessRepository) ConsumeChallenge(ctx context.Context, token string, surface models.CustomerSurface, now time.Time) (string, error) {
	var payload string
	err := r.db.QueryRowContext(ctx, `DELETE FROM customer_resource_tokens WHERE token = ? AND purpose = 'passkey' AND resource_kind = ? AND resource_id = ? AND expires_at > ? RETURNING payload`, customerTokenHash(token), surface.Kind, surface.ID, now).Scan(&payload)
	return payload, err
}
