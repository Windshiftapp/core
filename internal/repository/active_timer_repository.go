package repository

import (
	"database/sql"
	"errors"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ActiveTimerRepository owns SQL for the active_timers table plus the small
// surface of time_projects / time_worklogs reads and writes that the active
// timer handler needs (looking up project status / customer when starting and
// stopping a timer). When a dedicated TimeWorklogRepository lands the worklog
// write should move there.
type ActiveTimerRepository struct {
	db database.Database
}

// NewActiveTimerRepository creates a new repository.
func NewActiveTimerRepository(db database.Database) *ActiveTimerRepository {
	return &ActiveTimerRepository{db: db}
}

// GetProjectStatus returns the status string for a time project, or
// ErrNotFound if it does not exist.
func (r *ActiveTimerRepository) GetProjectStatus(projectID int) (string, error) {
	var status string
	err := r.db.QueryRow("SELECT status FROM time_projects WHERE id = ?", projectID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return status, err
}

// GetProjectCustomerID returns the customer_id linked to a time project.
func (r *ActiveTimerRepository) GetProjectCustomerID(projectID int) (int, error) {
	var customerID int
	err := r.db.QueryRow("SELECT customer_id FROM time_projects WHERE id = ?", projectID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return customerID, err
}

// HasActiveTimerForUser reports whether the user already has a running timer.
func (r *ActiveTimerRepository) HasActiveTimerForUser(userID int) (bool, error) {
	var existingID int
	err := r.db.QueryRow("SELECT id FROM active_timers WHERE user_id = ? LIMIT 1", userID).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CreateTimerInput captures the fields needed to insert a row in active_timers.
type CreateTimerInput struct {
	WorkspaceID  int
	ItemID       *int
	ProjectID    int
	UserID       int
	Description  string
	StartTimeUTC int64
}

// CreateTimer inserts a new active timer and returns its id.
func (r *ActiveTimerRepository) CreateTimer(in CreateTimerInput) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO active_timers (workspace_id, item_id, project_id, user_id, description, start_time_utc, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, in.WorkspaceID, in.ItemID, in.ProjectID, in.UserID, in.Description, in.StartTimeUTC, in.StartTimeUTC).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// DeleteTimer removes an active timer by id.
func (r *ActiveTimerRepository) DeleteTimer(id int) error {
	_, err := r.db.ExecWrite("DELETE FROM active_timers WHERE id = ?", id)
	return err
}

// activeTimerJoinedQuery is the joined SELECT used to fetch a timer with its
// project / customer / item / workspace context.
//
//nolint:misspell // customer_organisations uses British spelling in the schema
const activeTimerJoinedQuery = `
	SELECT
		at.id, at.workspace_id, at.item_id, at.project_id, at.user_id, at.description,
		at.start_time_utc, at.created_at,
		tp.name as project_name,
		tc.name as customer_name,
		i.title as item_title,
		ws.name as workspace_name,
		ws.key as workspace_key
	FROM active_timers at
	LEFT JOIN time_projects tp ON at.project_id = tp.id
	LEFT JOIN customer_organisations tc ON tp.customer_id = tc.id
	LEFT JOIN items i ON at.item_id = i.id
	LEFT JOIN workspaces ws ON at.workspace_id = ws.id`

// GetTimerByID returns the timer joined with project / customer / item /
// workspace context. Returns ErrNotFound if no timer matches.
func (r *ActiveTimerRepository) GetTimerByID(id int) (*models.ActiveTimer, error) {
	return scanJoinedActiveTimer(r.db.QueryRow(activeTimerJoinedQuery+` WHERE at.id = ?`, id))
}

// GetTimerForUser returns the user's running timer joined with project /
// customer / item / workspace context. Returns ErrNotFound if the user has no
// active timer.
func (r *ActiveTimerRepository) GetTimerForUser(userID int) (*models.ActiveTimer, error) {
	return scanJoinedActiveTimer(r.db.QueryRow(activeTimerJoinedQuery+` WHERE at.user_id = ? LIMIT 1`, userID))
}

func scanJoinedActiveTimer(row *sql.Row) (*models.ActiveTimer, error) {
	timer := &models.ActiveTimer{}
	var projectName, customerName, itemTitle, workspaceName, workspaceKey sql.NullString
	err := row.Scan(
		&timer.ID, &timer.WorkspaceID, &timer.ItemID, &timer.ProjectID, &timer.UserID, &timer.Description,
		&timer.StartTimeUTC, &timer.CreatedAt,
		&projectName, &customerName, &itemTitle, &workspaceName, &workspaceKey,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if projectName.Valid {
		timer.ProjectName = &projectName.String
	}
	if customerName.Valid {
		timer.CustomerName = &customerName.String
	}
	if itemTitle.Valid {
		timer.ItemTitle = &itemTitle.String
	}
	if workspaceName.Valid {
		timer.WorkspaceName = &workspaceName.String
	}
	if workspaceKey.Valid {
		timer.WorkspaceKey = &workspaceKey.String
	}
	return timer, nil
}

// CreateWorklogInput captures the fields needed to materialize a worklog row
// when a timer stops.
type CreateWorklogInput struct {
	ProjectID       int
	CustomerID      int
	UserID          int
	ItemID          *int
	Description     string
	DateUnix        int
	StartTimeUnix   int
	EndTimeUnix     int
	DurationMinutes int
	NowUnix         int64
}

// CreateWorklog inserts a worklog row. Lives here for now because the active
// timer is the only caller; move into a TimeWorklogRepository when we migrate
// the standalone worklog handler.
func (r *ActiveTimerRepository) CreateWorklog(in CreateWorklogInput) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO time_worklogs (project_id, customer_id, user_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, in.ProjectID, in.CustomerID, in.UserID, in.ItemID, in.Description,
		in.DateUnix, in.StartTimeUnix, in.EndTimeUnix,
		in.DurationMinutes, in.NowUnix, in.NowUnix)
	return err
}
