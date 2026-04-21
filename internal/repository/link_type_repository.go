package repository

import (
	"database/sql"
	"fmt"

	"windshift/internal/database"
)

// LinkTypeRepository provides data access methods for the link_types table.
type LinkTypeRepository struct {
	db database.Database
}

// NewLinkTypeRepository creates a new LinkTypeRepository.
func NewLinkTypeRepository(db database.Database) *LinkTypeRepository {
	return &LinkTypeRepository{db: db}
}

// FindActiveIDByName returns the ID of the active link_type matching the given name.
// Returns ErrNotFound if no active link_type with that name exists.
func (r *LinkTypeRepository) FindActiveIDByName(name string) (int, error) {
	var id int
	err := r.db.QueryRow(
		"SELECT id FROM link_types WHERE name = ? AND active = true",
		name,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("find link_type by name: %w", err)
	}
	return id, nil
}

// LinkTypeBasic carries the subset of link_type fields needed by validators.
// AllowedEntityTypes is the raw JSON string as stored in the column; callers
// parse it as needed.
type LinkTypeBasic struct {
	Active             bool
	AllowedEntityTypes string
}

// FindBasicByID returns Active and AllowedEntityTypes for a given link_type id.
// Returns ErrNotFound if the id doesn't exist.
func (r *LinkTypeRepository) FindBasicByID(id int) (*LinkTypeBasic, error) {
	var basic LinkTypeBasic
	var aet sql.NullString
	err := r.db.QueryRow(
		"SELECT active, allowed_entity_types FROM link_types WHERE id = ?",
		id,
	).Scan(&basic.Active, &aet)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find link_type basic: %w", err)
	}
	if aet.Valid {
		basic.AllowedEntityTypes = aet.String
	}
	return &basic, nil
}

// FindNameByID returns the name of the link_type with the given id, or an
// empty string if not found. Used for user-facing error messages where a
// missing row shouldn't be fatal.
func (r *LinkTypeRepository) FindNameByID(id int) (string, error) {
	var name string
	err := r.db.QueryRow("SELECT name FROM link_types WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find link_type name: %w", err)
	}
	return name, nil
}
