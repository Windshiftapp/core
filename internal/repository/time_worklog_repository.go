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

// TimeWorklogRepository persists rows in the time_worklogs table for both the
// cookie-auth and bearer-token HTTP surfaces. The timer-stop path performs its
// insert in ActiveTimerRepository because it must commit atomically with timer
// deletion.
type TimeWorklogRepository struct {
	db database.Database
}

type BriefingWorklog struct {
	Description     string
	DurationMinutes int
	ProjectName     string
}

type CaptureWorklog struct {
	ID              int
	AuthorUsername  string
	DurationMinutes int
	StartedUnix     int64
}

// ListCaptureWorklogs returns mapped worklogs that still belong to the item.
func (r *TimeWorklogRepository) ListCaptureWorklogs(itemID int, worklogIDs []int) ([]CaptureWorklog, error) {
	if len(worklogIDs) == 0 {
		return []CaptureWorklog{}, nil
	}
	placeholders, idArgs := inPlaceholders(worklogIDs)
	args := append([]any{itemID}, idArgs...)
	rows, err := r.db.Query(`
		SELECT w.id, COALESCE(u.username, ''), COALESCE(w.duration_minutes, 0), COALESCE(w.start_time, 0)
		FROM time_worklogs w
		LEFT JOIN users u ON u.id = w.user_id
		WHERE w.item_id = ? AND w.id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list capture worklogs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []CaptureWorklog{}
	for rows.Next() {
		var worklog CaptureWorklog
		if err := rows.Scan(&worklog.ID, &worklog.AuthorUsername, &worklog.DurationMinutes, &worklog.StartedUnix); err != nil {
			return nil, fmt.Errorf("scan capture worklog: %w", err)
		}
		out = append(out, worklog)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate capture worklogs: %w", err)
	}
	return out, nil
}

// ListBriefingWorklogs returns a user's worklogs in a half-open time window.
func (r *TimeWorklogRepository) ListBriefingWorklogs(userID int, start, end time.Time) ([]BriefingWorklog, error) {
	rows, err := r.db.Query(`
		SELECT tw.description, tw.duration_minutes, tp.name
		FROM time_worklogs tw
		JOIN time_projects tp ON tw.project_id = tp.id
		WHERE tw.user_id = ? AND tw.date >= ? AND tw.date < ?
		ORDER BY tw.date DESC
	`, userID, start.Unix(), end.Unix())
	if err != nil {
		return nil, fmt.Errorf("list briefing worklogs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]BriefingWorklog, 0)
	for rows.Next() {
		var worklog BriefingWorklog
		if err := rows.Scan(&worklog.Description, &worklog.DurationMinutes, &worklog.ProjectName); err != nil {
			return nil, fmt.Errorf("scan briefing worklog: %w", err)
		}
		out = append(out, worklog)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate briefing worklogs: %w", err)
	}
	return out, nil
}

// NewTimeWorklogRepository creates a TimeWorklogRepository.
func NewTimeWorklogRepository(db database.Database) *TimeWorklogRepository {
	return &TimeWorklogRepository{db: db}
}

const worklogDetailSelect = `SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number,
       p.settings,
       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = w.project_id),
       w.user_id, COALESCE(u.first_name || ' ' || u.last_name, '')
FROM time_worklogs w
JOIN customer_organisations c ON w.customer_id = c.id
JOIN time_projects p ON w.project_id = p.id
LEFT JOIN items i ON w.item_id = i.id
LEFT JOIN workspaces ws ON i.workspace_id = ws.id
LEFT JOIN users u ON w.user_id = u.id`

type worklogDetailScanner interface {
	Scan(dest ...any) error
}

func scanWorklogDetail(scanner worklogDetailScanner) (models.Worklog, error) {
	var worklog models.Worklog
	var itemTitle, workspaceKey, projectSettings, userName sql.NullString
	var workspaceID, workspaceItemNumber, userID sql.NullInt64
	var projectTotalHours sql.NullFloat64

	err := scanner.Scan(
		&worklog.ID, &worklog.ProjectID, &worklog.CustomerID, &worklog.ItemID, &worklog.Description,
		&worklog.Date, &worklog.StartTime, &worklog.EndTime, &worklog.DurationMins,
		&worklog.CreatedAt, &worklog.UpdatedAt, &worklog.CustomerName, &worklog.ProjectName, &itemTitle,
		&workspaceID, &workspaceKey, &workspaceItemNumber, &projectSettings, &projectTotalHours,
		&userID, &userName,
	)
	if err != nil {
		return models.Worklog{}, err
	}

	worklog.ItemTitle = itemTitle.String
	if workspaceID.Valid {
		id := int(workspaceID.Int64)
		worklog.WorkspaceID = &id
	}
	worklog.WorkspaceKey = workspaceKey.String
	worklog.WorkspaceItemNumber = int(workspaceItemNumber.Int64)
	if projectTotalHours.Valid {
		worklog.ProjectTotalHours = &projectTotalHours.Float64
	}
	if userID.Valid {
		id := int(userID.Int64)
		worklog.UserID = &id
	}
	worklog.UserName = userName.String
	if projectSettings.Valid && projectSettings.String != "" {
		var settings map[string]any
		if err := json.Unmarshal([]byte(projectSettings.String), &settings); err == nil {
			if maxHours, ok := settings["max_hours"].(float64); ok && maxHours > 0 {
				worklog.ProjectMaxHours = &maxHours
			}
		}
	}
	return worklog, nil
}

// WorklogDetailFilter narrows cookie-auth worklog list endpoints. A nil
// AccessibleProjectIDs slice means unrestricted access; an empty non-nil
// slice returns no rows.
type WorklogDetailFilter struct {
	AccessibleProjectIDs []int
	CustomerID           *int
	ProjectID            *int
	ItemID               *int
	DateFromUnix         *int64
	DateToExclusiveUnix  *int64
	Limit                int
	Offset               int
}

// ListDetails returns joined worklogs ordered newest-first.
func (r *TimeWorklogRepository) ListDetails(filter WorklogDetailFilter) ([]models.Worklog, error) {
	if filter.AccessibleProjectIDs != nil && len(filter.AccessibleProjectIDs) == 0 {
		return []models.Worklog{}, nil
	}
	where, args := worklogDetailWhere(filter)
	query := worklogDetailSelect + "\n" + where
	query += " ORDER BY w.date DESC, w.start_time DESC, w.id DESC"
	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit, max(filter.Offset, 0))
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list worklog details: %w", err)
	}
	defer func() { _ = rows.Close() }()

	worklogs := make([]models.Worklog, 0)
	for rows.Next() {
		worklog, err := scanWorklogDetail(rows)
		if err != nil {
			return nil, fmt.Errorf("scan worklog details: %w", err)
		}
		worklogs = append(worklogs, worklog)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read worklog details: %w", err)
	}
	return worklogs, nil
}

// ListDetailsPage returns a repository-bounded page and its matching total.
func (r *TimeWorklogRepository) ListDetailsPage(filter WorklogDetailFilter) ([]models.Worklog, int, error) {
	if filter.Limit <= 0 {
		return nil, 0, errors.New("worklog page limit must be positive")
	}
	if filter.AccessibleProjectIDs != nil && len(filter.AccessibleProjectIDs) == 0 {
		return []models.Worklog{}, 0, nil
	}
	where, args := worklogDetailWhere(filter)
	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM time_worklogs w "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count worklog details: %w", err)
	}
	items, err := r.ListDetails(filter)
	return items, total, err
}

func worklogDetailWhere(filter WorklogDetailFilter) (query string, args []any) {
	query = "WHERE 1=1"
	if filter.AccessibleProjectIDs != nil {
		placeholders := make([]string, len(filter.AccessibleProjectIDs))
		for i, id := range filter.AccessibleProjectIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND w.project_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	if filter.CustomerID != nil {
		query += " AND w.customer_id = ?"
		args = append(args, *filter.CustomerID)
	}
	if filter.ProjectID != nil {
		query += " AND w.project_id = ?"
		args = append(args, *filter.ProjectID)
	}
	if filter.ItemID != nil {
		query += " AND w.item_id = ?"
		args = append(args, *filter.ItemID)
	}
	if filter.DateFromUnix != nil {
		query += " AND w.date >= ?"
		args = append(args, *filter.DateFromUnix)
	}
	if filter.DateToExclusiveUnix != nil {
		query += " AND w.date < ?"
		args = append(args, *filter.DateToExclusiveUnix)
	}
	return query, args
}

// GetDetail returns a joined worklog or ErrNotFound when it does not exist.
func (r *TimeWorklogRepository) GetDetail(worklogID int) (*models.Worklog, error) {
	worklog, err := scanWorklogDetail(r.db.QueryRow(worklogDetailSelect+"\nWHERE w.id = ?", worklogID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get worklog details: %w", err)
	}
	return &worklog, nil
}

// NewWorklog captures the fields needed to insert a worklog row. Date and
// start/end times are unix seconds, matching the table's storage format.
type NewWorklog struct {
	ProjectID       int
	CustomerID      int64
	UserID          int
	ItemID          *int // nil when the worklog isn't linked to a work item
	Description     string
	DateUnix        int64
	StartTimeUnix   int64
	EndTimeUnix     int64
	DurationMinutes int
}

// ImportedWorklog preserves source timestamps and permits an unresolved user.
type ImportedWorklog struct {
	ProjectID       int
	CustomerID      int64
	UserID          *int
	ItemID          int
	Description     string
	DateUnix        int64
	StartTimeUnix   int64
	EndTimeUnix     int64
	DurationMinutes int
	CreatedAtUnix   int64
	UpdatedAtUnix   int64
}

// CreateImported inserts a worklog with source-system timestamps.
func (r *TimeWorklogRepository) CreateImported(in ImportedWorklog) (int64, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO time_worklogs
			(project_id, customer_id, user_id, item_id, description, date,
			 start_time, end_time, duration_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, in.ProjectID, in.CustomerID, in.UserID, in.ItemID, in.Description,
		in.DateUnix, in.StartTimeUnix, in.EndTimeUnix, in.DurationMinutes,
		in.CreatedAtUnix, in.UpdatedAtUnix).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create imported worklog: %w", err)
	}
	return id, nil
}

// UpdateWorklog captures the mutable fields of an existing worklog.
type UpdateWorklog struct {
	ID              int
	ProjectID       int
	CustomerID      int
	ItemID          *int
	Description     string
	DateUnix        int64
	StartTimeUnix   int64
	EndTimeUnix     int64
	DurationMinutes int
}

// Update replaces a worklog's editable fields and stamps updated_at.
func (r *TimeWorklogRepository) Update(in UpdateWorklog) error {
	_, err := r.db.ExecWrite(`
		UPDATE time_worklogs
		SET project_id = ?, customer_id = ?, item_id = ?, description = ?, date = ?,
		    start_time = ?, end_time = ?, duration_minutes = ?, updated_at = ?
		WHERE id = ?
	`, in.ProjectID, in.CustomerID, in.ItemID, in.Description, in.DateUnix,
		in.StartTimeUnix, in.EndTimeUnix, in.DurationMinutes, time.Now().Unix(), in.ID)
	if err != nil {
		return fmt.Errorf("update worklog: %w", err)
	}
	return nil
}

// Create inserts a worklog row, stamping created_at/updated_at, and returns
// the new row's id.
func (r *TimeWorklogRepository) Create(in NewWorklog) (int64, error) {
	now := time.Now().Unix()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO time_worklogs (project_id, customer_id, user_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		in.ProjectID, in.CustomerID, in.UserID, in.ItemID, in.Description,
		in.DateUnix, in.StartTimeUnix, in.EndTimeUnix, in.DurationMinutes, now, now,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create worklog: %w", err)
	}
	return id, nil
}

// WorklogListFilter narrows ListForUser results. Nil pointer fields disable
// the corresponding filter; the upper date bound is exclusive.
type WorklogListFilter struct {
	UserID              int
	DateFromUnix        *int64
	DateToExclusiveUnix *int64
	ProjectID           *int
	Limit               int
	Offset              int
}

// ListForUser returns a page of the user's worklogs, newest first, with the
// joined display fields (customer/project/item names, workspace reference,
// project budget figures) populated. The second return value is the total
// match count before pagination.
func (r *TimeWorklogRepository) ListForUser(f WorklogListFilter) ([]models.Worklog, int, error) {
	query := `SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
	       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
	       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number,
	       p.settings as project_settings,
	       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = w.project_id) as project_total_hours
	FROM time_worklogs w
	JOIN customer_organisations c ON w.customer_id = c.id
	JOIN time_projects p ON w.project_id = p.id
	LEFT JOIN items i ON w.item_id = i.id
	LEFT JOIN workspaces ws ON i.workspace_id = ws.id
	WHERE w.user_id = ?`
	qa := []any{f.UserID}

	if f.DateFromUnix != nil {
		query += " AND w.date >= ?"
		qa = append(qa, *f.DateFromUnix)
	}
	if f.DateToExclusiveUnix != nil {
		query += " AND w.date < ?"
		qa = append(qa, *f.DateToExclusiveUnix)
	}
	if f.ProjectID != nil {
		query += " AND w.project_id = ?"
		qa = append(qa, *f.ProjectID)
	}
	query += " ORDER BY w.date DESC"

	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM ("+query+") AS filtered_worklogs", qa...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count worklogs: %w", err)
	}

	query += " LIMIT ? OFFSET ?"
	qa = append(qa, f.Limit, f.Offset)

	rows, err := r.db.Query(query, qa...)
	if err != nil {
		return nil, 0, fmt.Errorf("list worklogs: %w", err)
	}
	defer rows.Close()

	out := make([]models.Worklog, 0)
	for rows.Next() {
		var wl models.Worklog
		var itemTitle, workspaceKey, projectSettings sql.NullString
		var workspaceID, workspaceItemNumber sql.NullInt64
		var projectTotalHours sql.NullFloat64
		if err := rows.Scan(&wl.ID, &wl.ProjectID, &wl.CustomerID, &wl.ItemID, &wl.Description,
			&wl.Date, &wl.StartTime, &wl.EndTime, &wl.DurationMins,
			&wl.CreatedAt, &wl.UpdatedAt, &wl.CustomerName, &wl.ProjectName, &itemTitle,
			&workspaceID, &workspaceKey, &workspaceItemNumber, &projectSettings, &projectTotalHours); err != nil {
			continue
		}
		wl.ItemTitle = itemTitle.String
		if workspaceID.Valid {
			id := int(workspaceID.Int64)
			wl.WorkspaceID = &id
		}
		wl.WorkspaceKey = workspaceKey.String
		wl.WorkspaceItemNumber = int(workspaceItemNumber.Int64)
		if projectTotalHours.Valid {
			wl.ProjectTotalHours = &projectTotalHours.Float64
		}
		if projectSettings.Valid && projectSettings.String != "" {
			var settings map[string]any
			if err := json.Unmarshal([]byte(projectSettings.String), &settings); err == nil {
				if maxHours, ok := settings["max_hours"].(float64); ok && maxHours > 0 {
					wl.ProjectMaxHours = &maxHours
				}
			}
		}
		out = append(out, wl)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list worklogs: %w", err)
	}
	return out, total, nil
}

// GetOwnerID returns the user_id that owns a worklog. Returns ErrNotFound
// when the worklog does not exist.
func (r *TimeWorklogRepository) GetOwnerID(worklogID int) (int, error) {
	var ownerID int
	err := r.db.QueryRow("SELECT user_id FROM time_worklogs WHERE id = ?", worklogID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get worklog owner: %w", err)
	}
	return ownerID, nil
}

// UpdateDescription replaces a worklog's description, stamping updated_at.
func (r *TimeWorklogRepository) UpdateDescription(worklogID int, description string) error {
	_, err := r.db.ExecWrite("UPDATE time_worklogs SET description = ?, updated_at = ? WHERE id = ?",
		description, time.Now().Unix(), worklogID)
	if err != nil {
		return fmt.Errorf("update worklog description: %w", err)
	}
	return nil
}

// Delete removes a worklog row.
func (r *TimeWorklogRepository) Delete(worklogID int) error {
	if _, err := r.db.ExecWrite("DELETE FROM time_worklogs WHERE id = ?", worklogID); err != nil {
		return fmt.Errorf("delete worklog: %w", err)
	}
	return nil
}
