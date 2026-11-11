package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type PortalCustomerRepository struct {
	db database.Database
}

func NewPortalCustomerRepository(db database.Database) *PortalCustomerRepository {
	return &PortalCustomerRepository{db: db}
}

// UserID returns the linked application user, if any.
func (r *PortalCustomerRepository) UserID(ctx context.Context, customerID int) (*int, error) {
	var id sql.NullInt64
	err := r.db.QueryRowContext(ctx, "SELECT user_id FROM portal_customers WHERE id = ?", customerID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get portal customer %d user: %w", customerID, err)
	}
	if !id.Valid {
		return nil, nil
	}
	value := int(id.Int64)
	return &value, nil
}

// GetIdentity returns a customer's display name and email by id.
func (r *PortalCustomerRepository) GetIdentity(ctx context.Context, customerID int) (name, email string, err error) {
	err = r.db.QueryRowContext(ctx, "SELECT name, email FROM portal_customers WHERE id = ?", customerID).Scan(&name, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("get portal customer %d identity: %w", customerID, err)
	}
	return name, email, nil
}

func (r *PortalCustomerRepository) FindIDByEmail(email string) (int, error) {
	var id int
	err := r.db.QueryRow(
		"SELECT id FROM portal_customers WHERE LOWER(email) = LOWER(?)",
		email,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("find portal customer by email: %w", err)
	}
	return id, nil
}

// FindOrCreateByEmail returns the id of the portal customer whose email
// matches case-insensitively (both sides trimmed), creating the customer when
// missing. createdVia records the creation provenance on the new row
// (WI-1553); existing rows are returned untouched. The insert uses ON
// CONFLICT DO NOTHING so a concurrent writer with the same address makes this
// call fall through and re-read the winning row instead of failing; created
// reports whether this call inserted the row.
func (r *PortalCustomerRepository) FindOrCreateByEmail(ctx context.Context, name, email, createdVia string) (id int, created bool, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if createdVia == "" {
		createdVia = models.CustomerCreatedViaUnknown
	}

	err = r.db.QueryRowContext(ctx,
		"SELECT id FROM portal_customers WHERE LOWER(email) = ?", email,
	).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("failed to query portal customer: %w", err)
	}

	var insertedID int64
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO portal_customers (name, email, created_via, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, name, email, createdVia).Scan(&insertedID)
	if errors.Is(err, sql.ErrNoRows) {
		if err := r.db.QueryRowContext(ctx,
			"SELECT id FROM portal_customers WHERE LOWER(email) = ?", email,
		).Scan(&id); err != nil {
			return 0, false, fmt.Errorf("failed to re-select portal customer after conflict: %w", err)
		}
		return id, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to create portal customer: %w", err)
	}
	return int(insertedID), true, nil
}

// CreateWithProvenance inserts a portal customer with an explicit creation
// provenance (WI-1553). Every production creation path must record where the
// row came from — never introduce an untagged INSERT.
func (r *PortalCustomerRepository) CreateWithProvenance(name, email string, organisationID int, createdVia string) (int, error) {
	if createdVia == "" {
		createdVia = models.CustomerCreatedViaUnknown
	}
	var id int
	err := r.db.QueryRow(`
		INSERT INTO portal_customers
			(name, email, customer_organisation_id, created_via, created_at, updated_at)
		VALUES (?, ?, NULLIF(?, 0), ?, ?, ?) RETURNING id
	`, name, email, organisationID, createdVia, time.Now(), time.Now()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create portal customer: %w", err)
	}
	return id, nil
}

func (r *PortalCustomerRepository) OrganisationID(customerID int) (*int, error) {
	var id sql.NullInt64
	err := r.db.QueryRow(
		"SELECT customer_organisation_id FROM portal_customers WHERE id = ?",
		customerID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get portal customer organisation: %w", err)
	}
	if !id.Valid {
		return nil, nil
	}
	value := int(id.Int64)
	return &value, nil
}

func (r *PortalCustomerRepository) AssignOrganisationIfUnset(customerID, organisationID int) (bool, error) {
	result, err := r.db.ExecWrite(`
		UPDATE portal_customers
		SET customer_organisation_id = ?, updated_at = ?
		WHERE id = ? AND customer_organisation_id IS NULL
	`, organisationID, time.Now(), customerID)
	if err != nil {
		return false, fmt.Errorf("assign portal customer organisation: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// SetLanguageIfUnset stores the language only when the customer has never
// chosen or had one detected. It reports whether the value was written, so the
// caller can treat detection as a one-time event and never override a choice.
func (r *PortalCustomerRepository) SetLanguageIfUnset(customerID int, language string) (bool, error) {
	if language == "" {
		return false, nil
	}
	result, err := r.db.ExecWrite(`
		UPDATE portal_customers
		SET language = ?, updated_at = ?
		WHERE id = ? AND (language IS NULL OR language = '')
	`, language, time.Now(), customerID)
	if err != nil {
		return false, fmt.Errorf("set portal customer language: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// UpdateRegionalSettings stores the customer's chosen language and timezone.
// Empty values clear the preference (NULL), which re-enables the instance
// default. Callers validate the values before writing.
func (r *PortalCustomerRepository) UpdateRegionalSettings(customerID int, language, timezone string) error {
	result, err := r.db.ExecWrite(`
		UPDATE portal_customers
		SET language = NULLIF(?, ''), timezone = NULLIF(?, ''), updated_at = ?
		WHERE id = ?
	`, language, timezone, time.Now(), customerID)
	if err != nil {
		return fmt.Errorf("update portal customer regional settings: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// Language returns the customer's stored language, or "" when unset.
func (r *PortalCustomerRepository) Language(ctx context.Context, customerID int) (string, error) {
	var language string
	err := r.db.QueryRowContext(ctx,
		"SELECT COALESCE(language, '') FROM portal_customers WHERE id = ?", customerID,
	).Scan(&language)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get portal customer %d language: %w", customerID, err)
	}
	return language, nil
}

// RegionalSettings returns the customer's stored language and timezone, or ""
// when either is unset. Callers use it to keep an omitted field unchanged.
func (r *PortalCustomerRepository) RegionalSettings(ctx context.Context, customerID int) (language, timezone string, err error) {
	err = r.db.QueryRowContext(ctx,
		"SELECT COALESCE(language, ''), COALESCE(timezone, '') FROM portal_customers WHERE id = ?", customerID,
	).Scan(&language, &timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("get portal customer %d regional settings: %w", customerID, err)
	}
	return language, timezone, nil
}

// LanguageByEmail returns the language of the customer with the given email,
// or "" when no customer row exists. The reply paths only know the recipient
// address, so a missing row is not an error.
func (r *PortalCustomerRepository) LanguageByEmail(ctx context.Context, email string) (string, error) {
	var language string
	err := r.db.QueryRowContext(ctx,
		"SELECT COALESCE(language, '') FROM portal_customers WHERE LOWER(email) = LOWER(?)", strings.TrimSpace(email),
	).Scan(&language)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get portal customer language by email: %w", err)
	}
	return language, nil
}

func (r *PortalCustomerRepository) EnsureChannelAccess(customerID, channelID int) (bool, error) {
	var existed bool
	if err := r.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM portal_customer_channels
			WHERE portal_customer_id = ? AND channel_id = ?
		)
	`, customerID, channelID).Scan(&existed); err != nil {
		return false, fmt.Errorf("check portal customer channel access: %w", err)
	}
	if _, err := r.db.ExecWrite(`
		INSERT INTO portal_customer_channels (portal_customer_id, channel_id, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(portal_customer_id, channel_id) DO NOTHING
	`, customerID, channelID, time.Now()); err != nil {
		return false, fmt.Errorf("grant portal customer channel access: %w", err)
	}
	return !existed, nil
}

func (r *PortalCustomerRepository) EnsureRole(customerID int, roleName string) (roleID int, created bool, err error) {
	if err = r.db.QueryRow(
		"SELECT id FROM contact_roles WHERE name = ?",
		roleName,
	).Scan(&roleID); err != nil {
		err = fmt.Errorf("find portal customer role: %w", err)
		return
	}
	var existed bool
	if err = r.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM portal_customer_roles
			WHERE portal_customer_id = ? AND contact_role_id = ?
		)
	`, customerID, roleID).Scan(&existed); err != nil {
		err = fmt.Errorf("check portal customer role: %w", err)
		return
	}
	if _, err = r.db.ExecWrite(`
		INSERT INTO portal_customer_roles (portal_customer_id, contact_role_id, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(portal_customer_id, contact_role_id) DO NOTHING
	`, customerID, roleID, time.Now()); err != nil {
		err = fmt.Errorf("assign portal customer role: %w", err)
		return
	}
	return roleID, !existed, nil
}
