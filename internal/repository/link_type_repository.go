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
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find link_type name: %w", err)
	}
	return name, nil
}

const linkTypeColumns = "id, name, description, forward_label, reverse_label, color, is_system, active, allowed_entity_types, created_at, updated_at"

// scanLinkType scans a *sql.Rows or *sql.Row into a models.LinkType, decoding
// the allowed_entity_types JSON column.
func scanLinkType(scanner interface {
	Scan(dest ...any) error
}) (models.LinkType, error) {
	var lt models.LinkType
	var aetRaw sql.NullString
	if err := scanner.Scan(
		&lt.ID, &lt.Name, &lt.Description, &lt.ForwardLabel, &lt.ReverseLabel,
		&lt.Color, &lt.IsSystem, &lt.Active, &aetRaw, &lt.CreatedAt, &lt.UpdatedAt,
	); err != nil {
		return lt, err
	}
	lt.AllowedEntityTypes = decodeAllowedEntityTypes(aetRaw)
	return lt, nil
}

// decodeAllowedEntityTypes parses the allowed_entity_types JSON column.
// A nil/invalid value means "all entity types allowed".
func decodeAllowedEntityTypes(raw sql.NullString) []string {
	if !raw.Valid || raw.String == "" {
		return nil
	}
	var types []string
	if err := json.Unmarshal([]byte(raw.String), &types); err != nil {
		return nil
	}
	return types
}

// encodeAllowedEntityTypes returns the JSON-encoded form for SQL parameters,
// or nil when the slice is empty (column is nullable).
func encodeAllowedEntityTypes(types []string) any {
	if len(types) == 0 {
		return nil
	}
	b, err := json.Marshal(types)
	if err != nil {
		return nil
	}
	return string(b)
}

// List returns link types ordered by system-flag then name. When
// includeInactive is false, only active rows are returned (the default for
// non-admin callers).
func (r *LinkTypeRepository) List(includeInactive bool) ([]models.LinkType, error) {
	query := "SELECT " + linkTypeColumns + " FROM link_types"
	if !includeInactive {
		query += " WHERE active = true"
	}
	query += " ORDER BY is_system DESC, name ASC"

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list link_types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var linkTypes []models.LinkType
	for rows.Next() {
		lt, err := scanLinkType(rows)
		if err != nil {
			return nil, fmt.Errorf("scan link_type: %w", err)
		}
		linkTypes = append(linkTypes, lt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate link_types: %w", err)
	}
	return linkTypes, nil
}

// GetByID returns the full link_type record. Returns ErrNotFound if no row exists.
func (r *LinkTypeRepository) GetByID(id int) (*models.LinkType, error) {
	row := r.db.QueryRow(
		"SELECT "+linkTypeColumns+" FROM link_types WHERE id = ?",
		id,
	)
	lt, err := scanLinkType(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get link_type %d: %w", id, err)
	}
	return &lt, nil
}

// Create inserts a new (non-system, active) link_type. The caller is
// responsible for sanitizing string fields; this method is purely SQL.
// Returns the new row's ID and the created/updated timestamp it stamped.
func (r *LinkTypeRepository) Create(lt *models.LinkType) (int, time.Time, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO link_types (name, description, forward_label, reverse_label, color, is_system, active, allowed_entity_types, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, lt.Name, lt.Description, lt.ForwardLabel, lt.ReverseLabel, lt.Color,
		false, true, encodeAllowedEntityTypes(lt.AllowedEntityTypes), now, now,
	).Scan(&id)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("create link_type: %w", err)
	}
	return int(id), now, nil
}

// Update replaces the editable fields of an existing link_type. is_system is
// not modifiable through this path. Returns the updated_at timestamp.
func (r *LinkTypeRepository) Update(id int, lt *models.LinkType) (time.Time, error) {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE link_types
		SET name = ?, description = ?, forward_label = ?, reverse_label = ?, color = ?, active = ?, allowed_entity_types = ?, updated_at = ?
		WHERE id = ?
	`, lt.Name, lt.Description, lt.ForwardLabel, lt.ReverseLabel, lt.Color,
		lt.Active, encodeAllowedEntityTypes(lt.AllowedEntityTypes), now, id,
	)
	if err != nil {
		return time.Time{}, fmt.Errorf("update link_type %d: %w", id, err)
	}
	return now, nil
}

// Delete removes a link_type row. Callers must enforce the is_system check
// before calling — system link types are not deletable.
func (r *LinkTypeRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM link_types WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete link_type %d: %w", id, err)
	}
	return nil
}
