package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TimeProjectCategoryRepository provides data access for the
// time_project_categories table.
type TimeProjectCategoryRepository struct {
	db database.Database
}

// NewTimeProjectCategoryRepository creates a new TimeProjectCategoryRepository.
func NewTimeProjectCategoryRepository(db database.Database) *TimeProjectCategoryRepository {
	return &TimeProjectCategoryRepository{db: db}
}

// List returns all time project categories ordered by display_order then name.
func (r *TimeProjectCategoryRepository) List() ([]models.TimeProjectCategory, error) {
	rows, err := r.db.Query(`
		SELECT id, name, description, color, display_order, created_at, updated_at
		FROM time_project_categories
		ORDER BY display_order ASC, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list time project categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	categories := []models.TimeProjectCategory{}
	for rows.Next() {
		c, err := scanTimeProjectCategory(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}

// FindByID loads a single category. Returns ErrNotFound when the id is unknown.
func (r *TimeProjectCategoryRepository) FindByID(id int) (*models.TimeProjectCategory, error) {
	var c models.TimeProjectCategory
	var description, color sql.NullString

	err := r.db.QueryRow(`
		SELECT id, name, description, color, display_order, created_at, updated_at
		FROM time_project_categories
		WHERE id = ?
	`, id).Scan(
		&c.ID, &c.Name, &description, &color,
		&c.DisplayOrder, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find time project category: %w", err)
	}
	if description.Valid {
		c.Description = description.String
	}
	if color.Valid {
		c.Color = color.String
	}
	return &c, nil
}

// Exists reports whether a category with the given id exists.
func (r *TimeProjectCategoryRepository) Exists(id int) (bool, error) {
	var exists bool
	err := r.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM time_project_categories WHERE id = ?)",
		id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check time project category exists: %w", err)
	}
	return exists, nil
}

// NextDisplayOrder returns the next display_order to use for a new category
// (max(existing)+1, or 0 when the table is empty).
func (r *TimeProjectCategoryRepository) NextDisplayOrder() (int, error) {
	var maxOrder sql.NullInt64
	err := r.db.QueryRow("SELECT MAX(display_order) FROM time_project_categories").Scan(&maxOrder)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("next display order: %w", err)
	}
	if maxOrder.Valid {
		return int(maxOrder.Int64) + 1, nil
	}
	return 0, nil
}

// Create inserts a new category. On success, c.ID, c.DisplayOrder,
// c.CreatedAt, and c.UpdatedAt are populated from the write.
func (r *TimeProjectCategoryRepository) Create(c *models.TimeProjectCategory) error {
	displayOrder, err := r.NextDisplayOrder()
	if err != nil {
		return err
	}

	now := time.Now()
	var id int64
	err = r.db.QueryRow(`
		INSERT INTO time_project_categories (name, description, color, display_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, c.Name, c.Description, c.Color, displayOrder, now, now).Scan(&id)
	if err != nil {
		return fmt.Errorf("create time project category: %w", err)
	}

	c.ID = int(id)
	c.DisplayOrder = displayOrder
	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

// Update overwrites the category with the given id. The caller's c.UpdatedAt
// is set to the write time on success.
func (r *TimeProjectCategoryRepository) Update(id int, c *models.TimeProjectCategory) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE time_project_categories
		SET name = ?, description = ?, color = ?, display_order = ?, updated_at = ?
		WHERE id = ?
	`, c.Name, c.Description, c.Color, c.DisplayOrder, now, id)
	if err != nil {
		return fmt.Errorf("update time project category: %w", err)
	}
	c.ID = id
	c.UpdatedAt = now
	return nil
}

// UpdateDisplayOrder sets display_order for a single category.
func (r *TimeProjectCategoryRepository) UpdateDisplayOrder(id, displayOrder int, now time.Time) error {
	_, err := r.db.Exec(`
		UPDATE time_project_categories
		SET display_order = ?, updated_at = ?
		WHERE id = ?
	`, displayOrder, now, id)
	if err != nil {
		return fmt.Errorf("update category display order: %w", err)
	}
	return nil
}

// Delete removes a category by id and returns the number of rows affected.
func (r *TimeProjectCategoryRepository) Delete(id int) (int64, error) {
	result, err := r.db.Exec("DELETE FROM time_project_categories WHERE id = ?", id)
	if err != nil {
		return 0, fmt.Errorf("delete time project category: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete time project category rows affected: %w", err)
	}
	return rowsAffected, nil
}

// CountProjectsUsing returns the number of time_projects that reference the
// given category id.
func (r *TimeProjectCategoryRepository) CountProjectsUsing(categoryID int) (int, error) {
	var count int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM time_projects WHERE category_id = ?",
		categoryID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count time projects using category: %w", err)
	}
	return count, nil
}

// scanTimeProjectCategory scans a single row into a TimeProjectCategory.
func scanTimeProjectCategory(rows *sql.Rows) (models.TimeProjectCategory, error) {
	var c models.TimeProjectCategory
	var description, color sql.NullString
	if err := rows.Scan(
		&c.ID, &c.Name, &description, &color,
		&c.DisplayOrder, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return c, fmt.Errorf("scan time project category: %w", err)
	}
	if description.Valid {
		c.Description = description.String
	}
	if color.Valid {
		c.Color = color.String
	}
	return c, nil
}
