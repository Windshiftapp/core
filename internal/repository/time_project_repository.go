package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TimeProjectRepository owns time-project persistence for both the cookie-auth
// and bearer-token HTTP surfaces.
type TimeProjectRepository struct {
	db database.Database
}

// NewTimeProjectRepository creates a TimeProjectRepository.
func NewTimeProjectRepository(db database.Database) *TimeProjectRepository {
	return &TimeProjectRepository{db: db}
}

// TimeProjectDetail is a time project joined with its customer/category names
// and the project's booked total hours.
type TimeProjectDetail struct {
	ID            int
	CustomerID    *int
	CategoryID    *int
	Name          string
	Description   string
	Status        string
	Color         string
	HourlyRate    float64
	Settings      map[string]any // parsed settings JSON; nil when empty
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CustomerName  string
	CategoryName  string
	CategoryColor string
	TotalHours    *float64
}

const timeProjectDetailSelect = `SELECT tp.id, tp.customer_id, tp.category_id, tp.name, COALESCE(tp.description, ''),
       tp.status, COALESCE(tp.color, ''), tp.hourly_rate, COALESCE(tp.settings, ''),
       tp.created_at, tp.updated_at,
       COALESCE(co.name, ''), COALESCE(tpc.name, ''), COALESCE(tpc.color, ''),
       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = tp.id) as total_hours
FROM time_projects tp
LEFT JOIN customer_organisations co ON tp.customer_id = co.id
LEFT JOIN time_project_categories tpc ON tp.category_id = tpc.id`

func scanTimeProjectDetail(scan func(dest ...any) error) (TimeProjectDetail, error) {
	var p TimeProjectDetail
	var settingsStr sql.NullString
	var totalHours sql.NullFloat64
	err := scan(&p.ID, &p.CustomerID, &p.CategoryID, &p.Name, &p.Description,
		&p.Status, &p.Color, &p.HourlyRate, &settingsStr, &p.CreatedAt, &p.UpdatedAt, &p.CustomerName,
		&p.CategoryName, &p.CategoryColor, &totalHours)
	if err != nil {
		return p, err
	}
	if totalHours.Valid {
		p.TotalHours = &totalHours.Float64
	}
	if settingsStr.Valid && settingsStr.String != "" && settingsStr.String != "{}" {
		var m map[string]any
		_ = json.Unmarshal([]byte(settingsStr.String), &m)
		p.Settings = m
	}
	return p, nil
}

// TimeProjectListFilter narrows joined time-project lists. Nil ID slices mean
// no restriction; an empty non-nil AccessibleIDs slice means no access.
type TimeProjectListFilter struct {
	AccessibleIDs []int
	CategoryIDs   []int
	CustomerID    *int
	Status        string
}

// ListDetails returns project detail rows ordered by name. A nil
// accessibleIDs slice means no access restriction; a non-nil slice limits the
// result to those project IDs. An empty statusFilter disables status
// filtering.
func (r *TimeProjectRepository) ListDetails(accessibleIDs []int, statusFilter string) ([]TimeProjectDetail, error) {
	return r.ListDetailsFiltered(TimeProjectListFilter{AccessibleIDs: accessibleIDs, Status: statusFilter})
}

// ListDetailsFiltered returns joined projects matching filter.
func (r *TimeProjectRepository) ListDetailsFiltered(filter TimeProjectListFilter) ([]TimeProjectDetail, error) {
	if filter.AccessibleIDs != nil && len(filter.AccessibleIDs) == 0 {
		return []TimeProjectDetail{}, nil
	}

	query := timeProjectDetailSelect + "\nWHERE 1=1"
	var qa []any

	if filter.AccessibleIDs != nil {
		ph := make([]string, len(filter.AccessibleIDs))
		for i, id := range filter.AccessibleIDs {
			ph[i] = "?"
			qa = append(qa, id)
		}
		query += " AND tp.id IN (" + strings.Join(ph, ",") + ")"
	}
	if filter.CustomerID != nil {
		query += " AND tp.customer_id = ?"
		qa = append(qa, *filter.CustomerID)
	}
	if len(filter.CategoryIDs) > 0 {
		ph := make([]string, len(filter.CategoryIDs))
		for i, id := range filter.CategoryIDs {
			ph[i] = "?"
			qa = append(qa, id)
		}
		query += " AND tp.category_id IN (" + strings.Join(ph, ",") + ")"
	}
	if filter.Status != "" {
		query += " AND tp.status = ?"
		qa = append(qa, filter.Status)
	}
	query += " ORDER BY tp.name"

	rows, err := r.db.Query(query, qa...)
	if err != nil {
		return nil, fmt.Errorf("list time projects: %w", err)
	}
	defer rows.Close()

	out := make([]TimeProjectDetail, 0)
	for rows.Next() {
		p, err := scanTimeProjectDetail(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan time project: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list time projects: %w", err)
	}
	return out, nil
}

// Create inserts a project and populates its generated ID and timestamps.
func (r *TimeProjectRepository) Create(project *models.TimeProject) error {
	now := time.Now()
	id, err := insertTimeProject(r.db.QueryRow, project, now)
	if err != nil {
		return err
	}
	project.ID, project.CreatedAt, project.UpdatedAt = id, now, now
	return nil
}

// CreateWithManager atomically creates a project and assigns its creator.
func (r *TimeProjectRepository) CreateWithManager(project *models.TimeProject, creatorID int) error {
	now := time.Now()
	var id int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		var err error
		id, err = insertTimeProject(tx.QueryRow, project, now)
		if err != nil {
			return err
		}
		_, err = tx.ExecWrite(`INSERT INTO time_project_managers (project_id, manager_type, manager_id, granted_by)
   VALUES (?, 'user', ?, ?)`, id, creatorID, creatorID)
		if err != nil {
			return fmt.Errorf("assign project creator: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	project.ID, project.CreatedAt, project.UpdatedAt = id, now, now
	return nil
}

func insertTimeProject(queryRow func(string, ...any) *sql.Row, project *models.TimeProject, now time.Time) (int, error) {
	settings, err := encodeTimeProjectSettings(project.Settings)
	if err != nil {
		return 0, err
	}
	var id int
	err = queryRow(`INSERT INTO time_projects
  (customer_id, category_id, name, description, status, color, hourly_rate, settings, created_at, updated_at)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`, project.CustomerID, project.CategoryID,
		project.Name, project.Description, project.Status, project.Color, project.HourlyRate, settings, now, now).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create time project: %w", err)
	}
	return id, nil
}

// FindIDByNameAndCustomer returns a project with the exact import identity.
func (r *TimeProjectRepository) FindIDByNameAndCustomer(name string, customerID int) (int, error) {
	var id int
	err := r.db.QueryRow(
		"SELECT id FROM time_projects WHERE name = ? AND customer_id = ?",
		name,
		customerID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("find time project by name and customer: %w", err)
	}
	return id, nil
}

// Update replaces a project's editable fields and stamps UpdatedAt.
func (r *TimeProjectRepository) Update(id int, project *models.TimeProject) error {
	settings, err := encodeTimeProjectSettings(project.Settings)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = r.db.ExecWrite(`
		UPDATE time_projects
		SET customer_id = ?, category_id = ?, name = ?, description = ?, status = ?, color = ?,
		    hourly_rate = ?, settings = ?, updated_at = ?
		WHERE id = ?
	`, project.CustomerID, project.CategoryID, project.Name, project.Description, project.Status,
		project.Color, project.HourlyRate, settings, now, id)
	if err != nil {
		return fmt.Errorf("update time project %d: %w", id, err)
	}
	project.ID = id
	project.UpdatedAt = now
	return nil
}

// Delete removes a time project.
func (r *TimeProjectRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM time_projects WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete time project %d: %w", id, err)
	}
	return nil
}

func encodeTimeProjectSettings(settings map[string]any) (any, error) {
	if len(settings) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("encode time project settings: %w", err)
	}
	return string(encoded), nil
}

// GetDetail returns a single project detail row. Returns ErrNotFound when the
// project does not exist.
func (r *TimeProjectRepository) GetDetail(projectID int) (*TimeProjectDetail, error) {
	row := r.db.QueryRow(timeProjectDetailSelect+"\nWHERE tp.id = ?", projectID)
	p, err := scanTimeProjectDetail(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get time project: %w", err)
	}
	return &p, nil
}

// TimeProjectBookingInfo carries the fields needed to validate logging time
// on a project.
type TimeProjectBookingInfo struct {
	Name       string
	Status     string
	CustomerID *int64 // nil when the project has no customer assigned
}

// GetBookingInfo returns the name, status, and customer of a project.
// Returns ErrNotFound when the project does not exist.
func (r *TimeProjectRepository) GetBookingInfo(projectID int) (*TimeProjectBookingInfo, error) {
	var info TimeProjectBookingInfo
	var customerID sql.NullInt64
	err := r.db.QueryRow("SELECT name, status, customer_id FROM time_projects WHERE id = ?", projectID).
		Scan(&info.Name, &info.Status, &customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get time project booking info: %w", err)
	}
	if customerID.Valid {
		info.CustomerID = &customerID.Int64
	}
	return &info, nil
}
