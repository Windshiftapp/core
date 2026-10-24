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

// GetProjectCustomerID returns the optional customer_id linked to a time
// project. A nil ID is distinct from a missing project and lets TimerService
// reject new timers while safely recovering legacy running timers.
func (r *ActiveTimerRepository) GetProjectCustomerID(projectID int) (*int, error) {
	var customerID sql.NullInt64
	err := r.db.QueryRow("SELECT customer_id FROM time_projects WHERE id = ?", projectID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !customerID.Valid {
		return nil, nil
	}
	id := int(customerID.Int64)
	return &id, nil
}

// GetUserTimezone returns the user's configured timezone. Empty values keep
// the application's UTC default.
func (r *ActiveTimerRepository) GetUserTimezone(userID int) (string, error) {
	var timezone sql.NullString
	err := r.db.QueryRow("SELECT timezone FROM users WHERE id = ?", userID).Scan(&timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if !timezone.Valid || timezone.String == "" {
		return "UTC", nil
	}
	return timezone.String, nil
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

// CreateTimer inserts a new active timer and returns its id. A unique-constraint
// violation on user_id (the user already has a running timer — the DB backstop
// for the "only one running" invariant) is mapped to ErrDuplicateEntry so the
// service can resolve a concurrent start/start race as ErrTimerAlreadyRunning.
func (r *ActiveTimerRepository) CreateTimer(in CreateTimerInput) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO active_timers (workspace_id, item_id, project_id, user_id, description, start_time_utc, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, in.WorkspaceID, in.ItemID, in.ProjectID, in.UserID, in.Description, in.StartTimeUTC, in.StartTimeUTC).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, err
	}
	return int(id), nil
}

// DeleteTimer removes an active timer by id.
func (r *ActiveTimerRepository) DeleteTimer(id int) error {
	_, err := r.db.ExecWrite("DELETE FROM active_timers WHERE id = ?", id)
	return err
}

// FinalizeTimer atomically materializes a timer as a worklog and removes the
// active timer. Keeping both writes in one transaction prevents a failed timer
// delete from leaving behind a worklog that would be duplicated on retry.
func (r *ActiveTimerRepository) FinalizeTimer(timerID int, in CreateWorklogInput) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		if _, err := tx.ExecWrite(createWorklogQuery,
			in.ProjectID, in.CustomerID, in.UserID, in.ItemID, in.Description,
			in.DateUnix, in.StartTimeUnix, in.EndTimeUnix,
			in.DurationMinutes, in.NowUnix, in.NowUnix,
		); err != nil {
			return err
		}

		result, err := tx.ExecWrite("DELETE FROM active_timers WHERE id = ?", timerID)
		if err != nil {
			return err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
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
		COALESCE(tc.builtin_key, '') as customer_builtin_key,
		i.title as item_title,
		ws.name as workspace_name,
		ws.key as workspace_key,
		i.workspace_item_number
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
	var projectName, customerName, customerBuiltinKey, itemTitle, workspaceName, workspaceKey sql.NullString
	var workspaceItemNumber sql.NullInt64
	err := row.Scan(
		&timer.ID, &timer.WorkspaceID, &timer.ItemID, &timer.ProjectID, &timer.UserID, &timer.Description,
		&timer.StartTimeUTC, &timer.CreatedAt,
		&projectName, &customerName, &customerBuiltinKey, &itemTitle, &workspaceName, &workspaceKey, &workspaceItemNumber,
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
	if customerBuiltinKey.Valid && customerBuiltinKey.String != "" {
		timer.CustomerBuiltinKey = &customerBuiltinKey.String
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
	if workspaceItemNumber.Valid {
		itemNumber := int(workspaceItemNumber.Int64)
		timer.WorkspaceItemNumber = &itemNumber
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

const createWorklogQuery = `
		INSERT INTO time_worklogs (project_id, customer_id, user_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

// CreateWorklog inserts a standalone worklog row. Timer stops use
// FinalizeTimer so worklog creation and timer deletion remain atomic.
func (r *ActiveTimerRepository) CreateWorklog(in CreateWorklogInput) error {
	_, err := r.db.ExecWrite(createWorklogQuery,
		in.ProjectID, in.CustomerID, in.UserID, in.ItemID, in.Description,
		in.DateUnix, in.StartTimeUnix, in.EndTimeUnix,
		in.DurationMinutes, in.NowUnix, in.NowUnix)
	return err
}
