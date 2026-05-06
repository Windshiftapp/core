package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TimeWorklogHandler struct {
	db                    database.Database
	permissionService     *services.PermissionService
	timePermissionService *services.TimePermissionService
}

func NewTimeWorklogHandler(db database.Database, permissionService *services.PermissionService, timePermissionService *services.TimePermissionService) *TimeWorklogHandler {
	return &TimeWorklogHandler{
		db:                    db,
		permissionService:     permissionService,
		timePermissionService: timePermissionService,
	}
}

//nolint:misspell // database table name uses British spelling (customer_organisations)
const worklogBaseQuery = `SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number,
       p.settings as project_settings,
       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = w.project_id) as project_total_hours
FROM time_worklogs w
JOIN customer_organisations c ON w.customer_id = c.id
JOIN time_projects p ON w.project_id = p.id
LEFT JOIN items i ON w.item_id = i.id
LEFT JOIN workspaces ws ON i.workspace_id = ws.id`

// worklogWithUserQuery extends worklogBaseQuery with user_id and user_name columns and the users join.
const worklogWithUserQuery = `SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number,
       p.settings as project_settings,
       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = w.project_id) as project_total_hours,
       w.user_id, COALESCE(u.first_name || ' ' || u.last_name, '') as user_name
FROM time_worklogs w
JOIN customer_organisations c ON w.customer_id = c.id
JOIN time_projects p ON w.project_id = p.id
LEFT JOIN items i ON w.item_id = i.id
LEFT JOIN workspaces ws ON i.workspace_id = ws.id
LEFT JOIN users u ON w.user_id = u.id`

// worklogScanner is an interface satisfied by both *sql.Row and *sql.Rows.
type worklogScanner interface {
	Scan(dest ...interface{}) error
}

// populateWorklogFields assigns nullable columns to worklog fields and parses project settings.
func populateWorklogFields(wl *models.Worklog, itemTitle, workspaceKey, projectSettings sql.NullString, workspaceID, workspaceItemNumber sql.NullInt64, projectTotalHours sql.NullFloat64) {
	wl.ItemTitle = itemTitle.String
	wl.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	wl.WorkspaceKey = workspaceKey.String
	wl.WorkspaceItemNumber = int(workspaceItemNumber.Int64)
	if projectTotalHours.Valid {
		wl.ProjectTotalHours = &projectTotalHours.Float64
	}
	if projectSettings.Valid && projectSettings.String != "" {
		var settings map[string]interface{}
		if err := json.Unmarshal([]byte(projectSettings.String), &settings); err == nil {
			if maxHours, ok := settings["max_hours"].(float64); ok && maxHours > 0 {
				wl.ProjectMaxHours = &maxHours
			}
		}
	}
}

// scanWorklog scans the 19 base worklog columns from a row and returns a populated Worklog.
func scanWorklog(s worklogScanner) (models.Worklog, error) {
	var wl models.Worklog
	var itemTitle, workspaceKey, projectSettings sql.NullString
	var workspaceID, workspaceItemNumber sql.NullInt64
	var projectTotalHours sql.NullFloat64

	err := s.Scan(&wl.ID, &wl.ProjectID, &wl.CustomerID, &wl.ItemID, &wl.Description, &wl.Date, &wl.StartTime,
		&wl.EndTime, &wl.DurationMins, &wl.CreatedAt, &wl.UpdatedAt, &wl.CustomerName, &wl.ProjectName, &itemTitle,
		&workspaceID, &workspaceKey, &workspaceItemNumber, &projectSettings, &projectTotalHours)
	if err != nil {
		return wl, err
	}

	populateWorklogFields(&wl, itemTitle, workspaceKey, projectSettings, workspaceID, workspaceItemNumber, projectTotalHours)
	return wl, nil
}

// scanWorklogWithUser scans the 19 base columns plus user_id and user_name (21 total).
func scanWorklogWithUser(s worklogScanner) (models.Worklog, error) {
	var wl models.Worklog
	var itemTitle, workspaceKey, projectSettings, userName sql.NullString
	var workspaceID, workspaceItemNumber sql.NullInt64
	var projectTotalHours sql.NullFloat64

	err := s.Scan(&wl.ID, &wl.ProjectID, &wl.CustomerID, &wl.ItemID, &wl.Description, &wl.Date, &wl.StartTime,
		&wl.EndTime, &wl.DurationMins, &wl.CreatedAt, &wl.UpdatedAt, &wl.CustomerName, &wl.ProjectName, &itemTitle,
		&workspaceID, &workspaceKey, &workspaceItemNumber, &projectSettings, &projectTotalHours,
		&wl.UserID, &userName)
	if err != nil {
		return wl, err
	}

	populateWorklogFields(&wl, itemTitle, workspaceKey, projectSettings, workspaceID, workspaceItemNumber, projectTotalHours)
	wl.UserName = userName.String
	return wl, nil
}

// scanWorklogWithUserRows iterates rows calling scanWorklogWithUser and returns the collected slice.
// On scan error it writes an internal-error response and returns nil, false.
func scanWorklogWithUserRows(w http.ResponseWriter, r *http.Request, rows *sql.Rows) ([]models.Worklog, bool) {
	var worklogs []models.Worklog
	for rows.Next() {
		worklog, err := scanWorklogWithUser(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return nil, false
		}
		worklogs = append(worklogs, worklog)
	}
	return worklogs, true
}

// ParseDuration is re-exported from internal/utils for backward compat.
// New callers should import utils.ParseDuration directly.
func ParseDuration(input string) (time.Duration, error) {
	return utils.ParseDuration(input)
}

type WorklogRequest struct {
	ProjectID     int    `json:"project_id"`
	ItemID        *int   `json:"item_id,omitempty"` // Optional link to work item
	Description   string `json:"description"`
	Date          string `json:"date"`       // YYYY-MM-DD format
	StartTime     string `json:"start_time"` // HH:MM format or empty
	EndTime       string `json:"end_time"`   // HH:MM format or empty
	DurationInput string `json:"duration"`   // "1h", "30m", "2h15m" etc
}

// requireWorklogEditAccess extracts the worklog ID, authenticates the user, and verifies
// edit permission (own worklog or manager). Returns the worklog ID, user, and ok bool.
func (h *TimeWorklogHandler) requireWorklogEditAccess(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return 0, false
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return 0, false
	}

	if h.timePermissionService != nil {
		canEdit, err := h.timePermissionService.CanEditWorklog(user.ID, id)
		if err != nil {
			respondInternalError(w, r, err)
			return 0, false
		}
		if !canEdit {
			respondForbidden(w, r)
			return 0, false
		}
	}

	return id, true
}

// filterWorklogsByPermission checks permissions and hides item info if user doesn't have access
func (h *TimeWorklogHandler) filterWorklogsByPermission(worklogs []models.Worklog, userID int) []models.Worklog {
	if h.permissionService == nil {
		slog.Error("permission service unavailable, hiding all item info from worklogs", slog.String("component", "time_tracking"))
		// Fail closed: strip item-related fields from all worklogs
		for i := range worklogs {
			worklogs[i].ItemID = nil
			worklogs[i].ItemTitle = ""
			worklogs[i].WorkspaceID = nil
			worklogs[i].WorkspaceKey = ""
			worklogs[i].WorkspaceItemNumber = 0
		}
		return worklogs
	}

	// Check if user is system admin first
	isAdmin, err := h.permissionService.IsSystemAdmin(userID)
	if err != nil {
		slog.Warn("error checking system admin status", slog.String("component", "time_tracking"), slog.Any("error", err))
		// On error, fall through to per-item checking
	} else if isAdmin {
		// System admin can see everything
		return worklogs
	}

	// Filter each worklog based on item permissions
	for i := range worklogs {
		worklog := &worklogs[i]

		// Only check permission if worklog has an associated item
		if worklog.ItemID == nil || worklog.WorkspaceID == nil {
			continue
		}

		// Check if user has permission to view this workspace
		hasPermission, err := h.permissionService.HasWorkspacePermission(userID, *worklog.WorkspaceID, models.PermissionItemView)
		if err != nil {
			slog.Warn("error checking workspace permission", slog.String("component", "time_tracking"), slog.Int("user_id", userID), slog.Int("workspace_id", *worklog.WorkspaceID), slog.Any("error", err))
			// On error, hide item info to be safe
			hasPermission = false
		}

		// If no permission, clear item-related fields
		if !hasPermission {
			worklog.ItemID = nil
			worklog.ItemTitle = ""
			worklog.WorkspaceID = nil
			worklog.WorkspaceKey = ""
			worklog.WorkspaceItemNumber = 0
		}
	}

	return worklogs
}

func (h *TimeWorklogHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Support filtering by date range, customer, project
	query := worklogWithUserQuery + ` WHERE 1=1`

	args := []interface{}{}

	// Add filters based on query parameters
	if customerID := r.URL.Query().Get("customer_id"); customerID != "" {
		query += " AND w.customer_id = ?"
		args = append(args, customerID)
	}

	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		query += " AND w.project_id = ?"
		args = append(args, projectID)
	}

	addDateRangeFilter(r, &query, &args, "w.date")

	query += " ORDER BY w.date DESC, w.start_time DESC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	worklogs, ok := scanWorklogWithUserRows(w, r, rows)
	if !ok {
		return
	}

	worklogs = h.filterWorklogsByPermission(worklogs, user.ID)

	respondJSONOK(w, worklogs)
}

func (h *TimeWorklogHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	wl, err := scanWorklog(h.db.QueryRow(worklogBaseQuery+` WHERE w.id = ?`, id))

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "worklog")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Filter item info by permission
	filtered := h.filterWorklogsByPermission([]models.Worklog{wl}, user.ID)
	respondJSONOK(w, filtered[0])
}

// validateAndParseWorklog validates a WorklogRequest and returns parsed values
//
//nolint:gocritic // tooManyResultsChecker: returns are semantically grouped
func (h *TimeWorklogHandler) validateAndParseWorklog(req WorklogRequest) (customerID int, date, startTime, endTime time.Time, durationMins int, err error) {
	// Validate project exists, get customer_id, and check status
	var projectStatus string
	var customerIDNull sql.NullInt64
	err = h.db.QueryRow("SELECT customer_id, status FROM time_projects WHERE id = ?", req.ProjectID).Scan(&customerIDNull, &projectStatus)
	if err == sql.ErrNoRows {
		err = fmt.Errorf("project not found")
		return
	}
	if err != nil {
		return
	}
	if !customerIDNull.Valid {
		err = fmt.Errorf("project has no customer assigned")
		return
	}
	customerID = int(customerIDNull.Int64)

	// Only allow time logging on Active projects
	if projectStatus != "Active" {
		err = fmt.Errorf("cannot log time on a project that is not active (status: %s)", projectStatus)
		return
	}

	// Parse date
	date, err = time.Parse("2006-01-02", req.Date)
	if err != nil {
		err = fmt.Errorf("invalid date format, use YYYY-MM-DD")
		return
	}

	// Handle time parsing - either explicit times or duration shorthand
	if req.StartTime != "" && req.EndTime != "" {
		// Explicit start and end times
		start, parseErr := time.Parse("15:04", req.StartTime)
		if parseErr != nil {
			err = fmt.Errorf("invalid start time format, use HH:MM")
			return
		}
		end, parseErr := time.Parse("15:04", req.EndTime)
		if parseErr != nil {
			err = fmt.Errorf("invalid end time format, use HH:MM")
			return
		}

		startTime = time.Date(date.Year(), date.Month(), date.Day(), start.Hour(), start.Minute(), 0, 0, time.Local)
		endTime = time.Date(date.Year(), date.Month(), date.Day(), end.Hour(), end.Minute(), 0, 0, time.Local)
		durationMins = int(endTime.Sub(startTime).Minutes())

		if durationMins <= 0 {
			err = fmt.Errorf("end time must be after start time")
			return
		}
	} else if req.DurationInput != "" {
		// Duration shorthand like "1h", "30m", "2h15m"
		duration, parseErr := ParseDuration(req.DurationInput)
		if parseErr != nil {
			err = fmt.Errorf("invalid duration: %v", parseErr)
			return
		}

		durationMins = int(duration.Minutes())

		// Default to ending "now" and calculating start time backwards
		if req.EndTime != "" {
			end, parseErr := time.Parse("15:04", req.EndTime)
			if parseErr != nil {
				err = fmt.Errorf("invalid end time format, use HH:MM")
				return
			}
			endTime = time.Date(date.Year(), date.Month(), date.Day(), end.Hour(), end.Minute(), 0, 0, time.Local)
		} else {
			endTime = time.Now()
			if !date.Equal(time.Now().Truncate(24 * time.Hour)) {
				// If not today, default end time to 17:00
				endTime = time.Date(date.Year(), date.Month(), date.Day(), 17, 0, 0, 0, time.Local)
			}
		}

		startTime = endTime.Add(-duration)
	} else {
		err = fmt.Errorf("either provide start_time+end_time or duration")
		return
	}

	return
}

func (h *TimeWorklogHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var req WorklogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("JSON decode error", slog.String("component", "time_tracking"), slog.Any("error", err))
		respondBadRequest(w, r, fmt.Sprintf("JSON decode error: %v", err))
		return
	}

	// Check booking permission on project
	if h.timePermissionService != nil {
		canBook, err := h.timePermissionService.CanBookTimeOnProject(user.ID, req.ProjectID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canBook {
			respondForbidden(w, r)
			return
		}
	}

	// Debug: Log the received request
	slog.Debug("received worklog request", slog.String("component", "time_tracking"), slog.Int("project_id", req.ProjectID), slog.String("description", req.Description))

	req.Description = utils.SanitizeCommentContent(req.Description)

	customerID, date, startTime, endTime, durationMins, err := h.validateAndParseWorklog(req)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Convert times to unix timestamps for database operations
	dateUnix := date.Unix()
	startTimeUnix := startTime.Unix()
	endTimeUnix := endTime.Unix()

	// No overlap validation - users should be free to log time as needed

	now := time.Now()
	nowUnix := now.Unix()

	// Debug: Log the data being inserted
	slog.Debug("inserting worklog", slog.String("component", "time_tracking"), slog.Int("project_id", req.ProjectID), slog.Int("customer_id", customerID), slog.Any("item_id", req.ItemID), slog.String("description", req.Description), slog.Int64("date", dateUnix), slog.Int64("start_time", startTimeUnix), slog.Int64("end_time", endTimeUnix), slog.Int("duration_minutes", durationMins))

	var id int64
	err = h.db.QueryRow(`
		INSERT INTO time_worklogs (project_id, customer_id, user_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, req.ProjectID, customerID, user.ID, req.ItemID, req.Description, dateUnix, startTimeUnix, endTimeUnix, durationMins, nowUnix, nowUnix).Scan(&id)

	if err != nil {
		slog.Error("database insert error", slog.String("component", "time_tracking"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Return the created worklog with joined data
	wl, err := scanWorklog(h.db.QueryRow(worklogBaseQuery+` WHERE w.id = ?`, id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, wl)
}

func (h *TimeWorklogHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireWorklogEditAccess(w, r)
	if !ok {
		return
	}

	var req WorklogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("JSON decode error", slog.String("component", "time_tracking"), slog.Any("error", err))
		respondBadRequest(w, r, fmt.Sprintf("JSON decode error: %v", err))
		return
	}

	slog.Debug("received worklog update request", slog.String("component", "time_tracking"), slog.Int("id", id), slog.Int("project_id", req.ProjectID))

	req.Description = utils.SanitizeCommentContent(req.Description)

	customerID, date, startTime, endTime, durationMins, err := h.validateAndParseWorklog(req)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Convert times to unix timestamps for database operations
	dateUnix := date.Unix()
	startTimeUnix := startTime.Unix()
	endTimeUnix := endTime.Unix()
	nowUnix := time.Now().Unix()

	_, err = h.db.ExecWrite(`
		UPDATE time_worklogs 
		SET project_id = ?, customer_id = ?, item_id = ?, description = ?, date = ?, 
		    start_time = ?, end_time = ?, duration_minutes = ?, updated_at = ?
		WHERE id = ?
	`, req.ProjectID, customerID, req.ItemID, req.Description, dateUnix, startTimeUnix, endTimeUnix, durationMins, nowUnix, id)

	if err != nil {
		slog.Error("database update error", slog.String("component", "time_tracking"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Return the updated worklog with joined data
	wl, err := scanWorklog(h.db.QueryRow(worklogBaseQuery+` WHERE w.id = ?`, id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, wl)
}

func (h *TimeWorklogHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireWorklogEditAccess(w, r)
	if !ok {
		return
	}

	_, err := h.db.ExecWrite("DELETE FROM time_worklogs WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TimeWorklogHandler) GetByProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Only project managers can view project-level worklogs (includes all users)
	if h.timePermissionService != nil {
		isManager, err := h.timePermissionService.IsTimeProjectManager(user.ID, projectID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !isManager {
			respondForbidden(w, r)
			return
		}
	}

	query := worklogWithUserQuery + ` WHERE w.project_id = ?`

	args := []interface{}{projectID}

	addDateRangeFilter(r, &query, &args, "w.date")

	query += " ORDER BY w.date DESC, w.start_time DESC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	worklogs, ok := scanWorklogWithUserRows(w, r, rows)
	if !ok {
		return
	}

	if worklogs == nil {
		worklogs = []models.Worklog{}
	}

	respondJSONOK(w, worklogs)
}

func (h *TimeWorklogHandler) GetByItem(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, repository.NewItemRepository(h.db), h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	rows, err := h.db.Query(worklogWithUserQuery+` WHERE w.item_id = ? ORDER BY w.date DESC, w.start_time DESC`, itemID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	worklogs, ok := scanWorklogWithUserRows(w, r, rows)
	if !ok {
		return
	}

	if worklogs == nil {
		worklogs = []models.Worklog{}
	}

	respondJSONOK(w, worklogs)
}
