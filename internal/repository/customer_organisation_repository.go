package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// CustomerOrganisationRepository handles persistence for the customer_organisations
// table that backs the time-tracking customers admin endpoint. (Spelled
// "organisation" to match the existing column / table name.)
//
//nolint:misspell // British spelling matches the table name.
type CustomerOrganisationRepository struct {
	db database.Database
}

// NewCustomerOrganisationRepository creates a CustomerOrganisationRepository.
func NewCustomerOrganisationRepository(db database.Database) *CustomerOrganisationRepository {
	return &CustomerOrganisationRepository{db: db}
}

const customerOrgColumns = "id, name, email, description, active, avatar_url, custom_field_values, created_at, updated_at"

// List returns every customer_organisation, name-ordered. Rows whose
// custom_field_values JSON fails to parse are returned with the field empty
// rather than dropped, so the admin UI sees the row and can repair it.
func (r *CustomerOrganisationRepository) List() ([]models.CustomerOrganisation, error) {
	//nolint:misspell // British spelling matches the table name.
	rows, err := r.db.Query("SELECT " + customerOrgColumns + " FROM customer_organisations ORDER BY name ASC")
	if err != nil {
		return nil, fmt.Errorf("list customer_organisations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var customers []models.CustomerOrganisation
	for rows.Next() {
		c, scanErr := scanCustomerOrganisation(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan customer_organisation: %w", scanErr)
		}
		customers = append(customers, c)
	}
	return customers, nil
}

// GetByID returns one customer or ErrNotFound. JSON parse errors on
// custom_field_values are propagated since a single-record fetch shouldn't
// silently lose data.
func (r *CustomerOrganisationRepository) GetByID(id int) (*models.CustomerOrganisation, error) {
	//nolint:misspell // British spelling matches the table name.
	row := r.db.QueryRow("SELECT "+customerOrgColumns+" FROM customer_organisations WHERE id = ?", id)
	c, err := scanCustomerOrganisation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get customer_organisation %d: %w", id, err)
	}
	return &c, nil
}

// Create inserts a customer_organisation and returns the generated id and
// the timestamp it stamped on created_at/updated_at. Sanitization is the
// caller's responsibility.
func (r *CustomerOrganisationRepository) Create(c *models.CustomerOrganisation) (int, time.Time, error) {
	cfv, err := encodeCustomFieldValues(c.CustomFieldValues)
	if err != nil {
		return 0, time.Time{}, err
	}
	now := time.Now()
	var id int64
	//nolint:misspell // British spelling matches the table name.
	err = r.db.QueryRow(`
		INSERT INTO customer_organisations (name, email, description, active, avatar_url, custom_field_values, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, c.Name, c.Email, c.Description, c.Active, c.AvatarURL, cfv, now, now).Scan(&id)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("create customer_organisation: %w", err)
	}
	return int(id), now, nil
}

// Update overwrites an existing customer_organisation's editable fields and
// returns the updated_at timestamp. Returns ErrNotFound when no row matches
// the given id so handlers can render a 404 instead of a misleading success.
func (r *CustomerOrganisationRepository) Update(id int, c *models.CustomerOrganisation) (time.Time, error) {
	cfv, err := encodeCustomFieldValues(c.CustomFieldValues)
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now()
	//nolint:misspell // British spelling matches the table name.
	result, err := r.db.ExecWrite(`
		UPDATE customer_organisations
		SET name = ?, email = ?, description = ?, active = ?, avatar_url = ?, custom_field_values = ?, updated_at = ?
		WHERE id = ?
	`, c.Name, c.Email, c.Description, c.Active, c.AvatarURL, cfv, now, id)
	if err != nil {
		return time.Time{}, fmt.Errorf("update customer_organisation %d: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return time.Time{}, fmt.Errorf("update customer_organisation %d rows affected: %w", id, err)
	}
	if rowsAffected == 0 {
		return time.Time{}, ErrNotFound
	}
	return now, nil
}

// Delete removes a customer_organisation row. Returns ErrNotFound when no row
// matches the given id.
func (r *CustomerOrganisationRepository) Delete(id int) error {
	//nolint:misspell // British spelling matches the table name.
	result, err := r.db.ExecWrite("DELETE FROM customer_organisations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete customer_organisation %d: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete customer_organisation %d rows affected: %w", id, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountTimeProjects returns the number of time_projects pointing at the given
// customer; the handler uses this to refuse a delete that would orphan
// projects.
func (r *CustomerOrganisationRepository) CountTimeProjects(customerID int) (int, error) {
	var n int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM time_projects WHERE customer_id = ?", customerID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count time_projects for customer %d: %w", customerID, err)
	}
	return n, nil
}

func scanCustomerOrganisation(scanner interface {
	Scan(dest ...any) error
}) (models.CustomerOrganisation, error) {
	var c models.CustomerOrganisation
	var avatarURL, cfvStr sql.NullString
	if err := scanner.Scan(
		&c.ID, &c.Name, &c.Email, &c.Description, &c.Active,
		&avatarURL, &cfvStr, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return c, err
	}
	if avatarURL.Valid {
		c.AvatarURL = avatarURL.String
	}
	if cfvStr.Valid && cfvStr.String != "" {
		// Best-effort decode: leave CustomFieldValues nil on parse failure so
		// the row still surfaces in admin lists for repair.
		_ = json.Unmarshal([]byte(cfvStr.String), &c.CustomFieldValues)
	}
	return c, nil
}

// encodeCustomFieldValues returns the JSON string form for SQL params, or
// nil when the map is empty (column is nullable).
func encodeCustomFieldValues(values map[string]interface{}) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode custom_field_values: %w", err)
	}
	return string(b), nil
}
