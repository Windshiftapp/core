package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TimeWorklogHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewTimeWorklogHandler(db database.Database, permissionService *services.PermissionService) *TimeWorklogHandler {
	return &TimeWorklogHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// ParseDuration parses time duration strings like "1h", "30m", "2h15m", "1d"
func ParseDuration(input string) (time.Duration, error) {
	input = strings.TrimSpace(strings.ToLower(input))

	// Handle "1d" as 8 hours
	if strings.HasSuffix(input, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(input, "d"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid day format: %s", input)
		}
		return time.Duration(days * 8 * float64(time.Hour)), nil
	}

	// Parse complex formats like "2h30m"
	re := regexp.MustCompile(`(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m)?`)
	matches := re.FindStringSubmatch(input)

	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid duration format: %s", input)
	}

	var total time.Duration

	// Hours
	if matches[1] != "" {
		hours, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hour format: %s", matches[1])
		}
		total += time.Duration(hours * float64(time.Hour))
	}

	// Minutes
	if matches[2] != "" {
		minutes, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minute format: %s", matches[2])
		}
		total += time.Duration(minutes * float64(time.Minute))
	}

	if total == 0 {
		return 0, fmt.Errorf("no time duration found in: %s", input)
	}

	return total, nil
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

// filterWorklogsByPermission checks permissions and hides item info if user doesn't have access
func (h *TimeWorklogHandler) filterWorklogsByPermission(worklogs []models.Worklog, userID int) []models.Worklog {
	if h.permissionService == nil {
		// No permission service configured - return all worklogs as-is
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
	// Support filtering by date range, customer, project
	query := `
		SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
		       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
		       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number
		FROM time_worklogs w
		JOIN customer_organisations c ON w.customer_id = c.id
		JOIN time_projects p ON w.project_id = p.id
		LEFT JOIN items i ON w.item_id = i.id
		LEFT JOIN workspaces ws ON i.workspace_id = ws.id
		WHERE 1=1`

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

	if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
		if fromDate, err := time.Parse("2006-01-02", dateFrom); err == nil {
			// Convert to start of day unix timestamp
			fromUnix := time.Date(fromDate.Year(), fromDate.Month(), fromDate.Day(), 0, 0, 0, 0, time.Local).Unix()
			query += " AND w.date >= ?"
			args = append(args, fromUnix)
		}
	}

	if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
		if toDate, err := time.Parse("2006-01-02", dateTo); err == nil {
			// Convert to end of day unix timestamp
			toUnix := time.Date(toDate.Year(), toDate.Month(), toDate.Day(), 23, 59, 59, 0, time.Local).Unix()
			query += " AND w.date <= ?"
			args = append(args, toUnix)
		}
	}

	query += " ORDER BY w.date DESC, w.start_time DESC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var worklogs []models.Worklog
	for rows.Next() {
		var worklog models.Worklog
		var itemTitle, workspaceKey sql.NullString
		var workspaceID, workspaceItemNumber sql.NullInt64
		err := rows.Scan(&worklog.ID, &worklog.ProjectID, &worklog.CustomerID, &worklog.ItemID, &worklog.Description, &worklog.Date, &worklog.StartTime,
			&worklog.EndTime, &worklog.DurationMins, &worklog.CreatedAt, &worklog.UpdatedAt, &worklog.CustomerName, &worklog.ProjectName, &itemTitle,
			&workspaceID, &workspaceKey, &workspaceItemNumber)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		worklog.ItemTitle = itemTitle.String
		worklog.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
		worklog.WorkspaceKey = workspaceKey.String
		worklog.WorkspaceItemNumber = int(workspaceItemNumber.Int64)
		worklogs = append(worklogs, worklog)
	}

	respondJSONOK(w, worklogs)
}

func (h *TimeWorklogHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var wl models.Worklog
	var itemTitle, workspaceKey sql.NullString
	var workspaceID, workspaceItemNumber sql.NullInt64
	err := h.db.QueryRow(`
		SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
		       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
		       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number
		FROM time_worklogs w
		JOIN customer_organisations c ON w.customer_id = c.id
		JOIN time_projects p ON w.project_id = p.id
		LEFT JOIN items i ON w.item_id = i.id
		LEFT JOIN workspaces ws ON i.workspace_id = ws.id
		WHERE w.id = ?
	`, id).Scan(&wl.ID, &wl.ProjectID, &wl.CustomerID, &wl.ItemID, &wl.Description, &wl.Date, &wl.StartTime,
		&wl.EndTime, &wl.DurationMins, &wl.CreatedAt, &wl.UpdatedAt, &wl.CustomerName, &wl.ProjectName, &itemTitle,
		&workspaceID, &workspaceKey, &workspaceItemNumber)

	wl.ItemTitle = itemTitle.String
	wl.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	wl.WorkspaceKey = workspaceKey.String
	wl.WorkspaceItemNumber = int(workspaceItemNumber.Int64)

	if err == sql.ErrNoRows {
		http.Error(w, "Worklog not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, wl)
}

// validateAndParseWorklog validates a WorklogRequest and returns parsed values
func (h *TimeWorklogHandler) validateAndParseWorklog(req WorklogRequest) (customerID int, date time.Time, startTime, endTime time.Time, durationMins int, err error) {
	// Validate project exists and get customer_id
	err = h.db.QueryRow("SELECT customer_id FROM time_projects WHERE id = ?", req.ProjectID).Scan(&customerID)
	if err == sql.ErrNoRows {
		err = fmt.Errorf("project not found")
		return
	}
	if err != nil {
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
	var req WorklogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("JSON decode error", slog.String("component", "time_tracking"), slog.Any("error", err))
		http.Error(w, fmt.Sprintf("JSON decode error: %v", err), http.StatusBadRequest)
		return
	}

	// Debug: Log the received request
	slog.Debug("received worklog request", slog.String("component", "time_tracking"), slog.Int("project_id", req.ProjectID), slog.String("description", req.Description))

	customerID, date, startTime, endTime, durationMins, err := h.validateAndParseWorklog(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		INSERT INTO time_worklogs (project_id, customer_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, req.ProjectID, customerID, req.ItemID, req.Description, dateUnix, startTimeUnix, endTimeUnix, durationMins, nowUnix, nowUnix).Scan(&id)

	if err != nil {
		slog.Error("database insert error", slog.String("component", "time_tracking"), slog.Any("error", err))
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the created worklog with joined data
	var wl models.Worklog
	var itemTitle, workspaceKey sql.NullString
	var workspaceID, workspaceItemNumber sql.NullInt64
	err = h.db.QueryRow(`
		SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
		       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
		       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number
		FROM time_worklogs w
		JOIN customer_organisations c ON w.customer_id = c.id
		JOIN time_projects p ON w.project_id = p.id
		LEFT JOIN items i ON w.item_id = i.id
		LEFT JOIN workspaces ws ON i.workspace_id = ws.id
		WHERE w.id = ?
	`, id).Scan(&wl.ID, &wl.ProjectID, &wl.CustomerID, &wl.ItemID, &wl.Description, &wl.Date, &wl.StartTime,
		&wl.EndTime, &wl.DurationMins, &wl.CreatedAt, &wl.UpdatedAt, &wl.CustomerName, &wl.ProjectName, &itemTitle,
		&workspaceID, &workspaceKey, &workspaceItemNumber)

	wl.ItemTitle = itemTitle.String
	wl.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	wl.WorkspaceKey = workspaceKey.String
	wl.WorkspaceItemNumber = int(workspaceItemNumber.Int64)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONCreated(w, wl)
}

func (h *TimeWorklogHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var req WorklogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("JSON decode error", slog.String("component", "time_tracking"), slog.Any("error", err))
		http.Error(w, fmt.Sprintf("JSON decode error: %v", err), http.StatusBadRequest)
		return
	}

	slog.Debug("received worklog update request", slog.String("component", "time_tracking"), slog.Int("id", id), slog.Int("project_id", req.ProjectID))

	customerID, date, startTime, endTime, durationMins, err := h.validateAndParseWorklog(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the updated worklog with joined data
	var wl models.Worklog
	var itemTitle, workspaceKey sql.NullString
	var workspaceID, workspaceItemNumber sql.NullInt64
	err = h.db.QueryRow(`
		SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
		       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
		       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number
		FROM time_worklogs w
		JOIN customer_organisations c ON w.customer_id = c.id
		JOIN time_projects p ON w.project_id = p.id
		LEFT JOIN items i ON w.item_id = i.id
		LEFT JOIN workspaces ws ON i.workspace_id = ws.id
		WHERE w.id = ?
	`, id).Scan(&wl.ID, &wl.ProjectID, &wl.CustomerID, &wl.ItemID, &wl.Description, &wl.Date, &wl.StartTime,
		&wl.EndTime, &wl.DurationMins, &wl.CreatedAt, &wl.UpdatedAt, &wl.CustomerName, &wl.ProjectName, &itemTitle,
		&workspaceID, &workspaceKey, &workspaceItemNumber)

	wl.ItemTitle = itemTitle.String
	wl.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
	wl.WorkspaceKey = workspaceKey.String
	wl.WorkspaceItemNumber = int(workspaceItemNumber.Int64)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, wl)
}

func (h *TimeWorklogHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	_, err := h.db.ExecWrite("DELETE FROM time_worklogs WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TimeWorklogHandler) GetByProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
		       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
		       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number
		FROM time_worklogs w
		JOIN customer_organisations c ON w.customer_id = c.id
		JOIN time_projects p ON w.project_id = p.id
		LEFT JOIN items i ON w.item_id = i.id
		LEFT JOIN workspaces ws ON i.workspace_id = ws.id
		WHERE w.project_id = ?
		ORDER BY w.date DESC, w.start_time DESC
	`, projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var worklogs []models.Worklog
	for rows.Next() {
		var worklog models.Worklog
		var itemTitle, workspaceKey sql.NullString
		var workspaceID, workspaceItemNumber sql.NullInt64
		err := rows.Scan(&worklog.ID, &worklog.ProjectID, &worklog.CustomerID, &worklog.ItemID, &worklog.Description,
			&worklog.Date, &worklog.StartTime, &worklog.EndTime, &worklog.DurationMins,
			&worklog.CreatedAt, &worklog.UpdatedAt, &worklog.CustomerName, &worklog.ProjectName, &itemTitle,
			&workspaceID, &workspaceKey, &workspaceItemNumber)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		worklog.ItemTitle = itemTitle.String
		worklog.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
		worklog.WorkspaceKey = workspaceKey.String
		worklog.WorkspaceItemNumber = int(workspaceItemNumber.Int64)
		worklogs = append(worklogs, worklog)
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

	rows, err := h.db.Query(`
		SELECT w.id, w.project_id, w.customer_id, w.item_id, w.description, w.date, w.start_time,
		       w.end_time, w.duration_minutes, w.created_at, w.updated_at,
		       c.name, p.name, i.title, ws.id, ws.key, i.workspace_item_number
		FROM time_worklogs w
		JOIN customer_organisations c ON w.customer_id = c.id
		JOIN time_projects p ON w.project_id = p.id
		LEFT JOIN items i ON w.item_id = i.id
		LEFT JOIN workspaces ws ON i.workspace_id = ws.id
		WHERE w.item_id = ?
		ORDER BY w.date DESC, w.start_time DESC
	`, itemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var worklogs []models.Worklog
	for rows.Next() {
		var worklog models.Worklog
		var itemTitle, workspaceKey sql.NullString
		var workspaceID, workspaceItemNumber sql.NullInt64
		err := rows.Scan(&worklog.ID, &worklog.ProjectID, &worklog.CustomerID, &worklog.ItemID, &worklog.Description,
			&worklog.Date, &worklog.StartTime, &worklog.EndTime, &worklog.DurationMins,
			&worklog.CreatedAt, &worklog.UpdatedAt, &worklog.CustomerName, &worklog.ProjectName, &itemTitle,
			&workspaceID, &workspaceKey, &workspaceItemNumber)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		worklog.ItemTitle = itemTitle.String
		worklog.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
		worklog.WorkspaceKey = workspaceKey.String
		worklog.WorkspaceItemNumber = int(workspaceItemNumber.Int64)
		worklogs = append(worklogs, worklog)
	}

	if worklogs == nil {
		worklogs = []models.Worklog{}
	}

	respondJSONOK(w, worklogs)
}
