package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// StatusRepository serves the legacy /api/statuses CRUD surface and the
// shared "non-done status IDs" lookup. Production traffic uses the v1
// status handler; this repo backs the overlay-only legacy handler so
// behavior keeps round-tripping through the layering rules.
type StatusRepository struct {
	db database.Database
}

// NewStatusRepository creates a StatusRepository.
func NewStatusRepository(db database.Database) *StatusRepository {
	return &StatusRepository{db: db}
}

const statusJoinedSelect = `
	SELECT s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
	       sc.name as category_name, sc.color as category_color
	FROM statuses s
	JOIN status_categories sc ON s.category_id = sc.id`

// List returns all statuses with their joined category data, ordered the
// same way the handler did (defaults first, then by category, then by name).
func (r *StatusRepository) List() ([]models.Status, error) {
	rows, err := r.db.Query(statusJoinedSelect + ` ORDER BY s.is_default DESC, sc.name ASC, s.name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var statuses []models.Status
	for rows.Next() {
		s, scanErr := scanStatusJoined(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan status: %w", scanErr)
		}
		statuses = append(statuses, s)
	}
	if statuses == nil {
		statuses = []models.Status{}
	}
	return statuses, nil
}

// GetByID returns a single status with category fields. ErrNotFound when missing.
func (r *StatusRepository) GetByID(id int) (*models.Status, error) {
	row := r.db.QueryRow(statusJoinedSelect+" WHERE s.id = ?", id)
	s, err := scanStatusJoined(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get status %d: %w", id, err)
	}
	return &s, nil
}

// Create inserts a status row and returns the new id and timestamp it stamped.
// Sanitization is the caller's responsibility.
func (r *StatusRepository) Create(s *models.Status) (int64, time.Time, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, s.Name, s.Description, s.CategoryID, s.IsDefault, now, now).Scan(&id)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("create status: %w", err)
	}
	return id, now, nil
}

// Update replaces the editable fields of an existing status.
func (r *StatusRepository) Update(id int, s *models.Status) error {
	_, err := r.db.ExecWrite(`
		UPDATE statuses
		SET name = ?, description = ?, category_id = ?, is_default = ?, updated_at = ?
		WHERE id = ?
	`, s.Name, s.Description, s.CategoryID, s.IsDefault, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update status %d: %w", id, err)
	}
	return nil
}

// Delete removes a status row. Caller must enforce the system-critical and
// referential checks (CategoryExists, NameExists, CountTransitionsUsing,
// item count) before calling.
func (r *StatusRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM statuses WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete status %d: %w", id, err)
	}
	return nil
}

// CategoryExists reports whether a status_category with the given id exists.
// Used by the handler's pre-write validation.
func (r *StatusRepository) CategoryExists(categoryID int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE id = ?)", categoryID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check status_category %d: %w", categoryID, err)
	}
	return ok, nil
}

// NameExists reports whether another status row already uses the given name.
// excludeID > 0 excludes that row from the check (so an Update doesn't
// collide with itself).
func (r *StatusRepository) NameExists(name string, excludeID int) (bool, error) {
	var ok bool
	var err error
	if excludeID > 0 {
		err = r.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ? AND id != ?)",
			name, excludeID,
		).Scan(&ok)
	} else {
		err = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ?)", name).Scan(&ok)
	}
	if err != nil {
		return false, fmt.Errorf("check status name %q: %w", name, err)
	}
	return ok, nil
}

// CountTransitionsUsing returns the number of workflow_transitions whose
// from_status_id or to_status_id references the given status. The handler
// uses this to refuse a delete that would orphan transitions.
func (r *StatusRepository) CountTransitionsUsing(statusID int) (int, error) {
	var n int
	if err := r.db.QueryRow(
		"SELECT COUNT(*) FROM workflow_transitions WHERE from_status_id = ? OR to_status_id = ?",
		statusID, statusID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("count transitions using status %d: %w", statusID, err)
	}
	return n, nil
}

// ListNonDoneIDs returns the IDs of statuses whose category is NOT marked
// completed. Backs the GET /api/statuses/non-done-ids endpoint.
func (r *StatusRepository) ListNonDoneIDs() ([]int, error) {
	rows, err := r.db.Query(`
		SELECT s.id
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE COALESCE(sc.is_completed, FALSE) = FALSE
		ORDER BY s.id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list non-done status ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan status id: %w", err)
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []int{}
	}
	return ids, nil
}

func scanStatusJoined(scanner interface {
	Scan(dest ...any) error
}) (models.Status, error) {
	var s models.Status
	if err := scanner.Scan(
		&s.ID, &s.Name, &s.Description, &s.CategoryID, &s.IsDefault,
		&s.CreatedAt, &s.UpdatedAt,
		&s.CategoryName, &s.CategoryColor,
	); err != nil {
		return s, err
	}
	return s, nil
}
