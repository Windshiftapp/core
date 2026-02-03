package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/utils"
)

type ActiveTimerHandler struct {
	db database.Database
}

func NewActiveTimerHandler(db database.Database) *ActiveTimerHandler {
	return &ActiveTimerHandler{db: db}
}

// StartTimer starts a new active timer
func (h *ActiveTimerHandler) StartTimer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkspaceID int    `json:"workspace_id"`
		ItemID      *int   `json:"item_id,omitempty"`
		ProjectID   int    `json:"project_id"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validation
	if req.WorkspaceID == 0 {
		respondValidationError(w, r, "workspace_id is required")
		return
	}
	if req.ProjectID == 0 {
		respondValidationError(w, r, "project_id is required")
		return
	}
	if req.Description == "" {
		respondValidationError(w, r, "description is required")
		return
	}

	// Validate project exists and is Active
	var projectStatus string
	err := h.db.QueryRow("SELECT status FROM time_projects WHERE id = ?", req.ProjectID).Scan(&projectStatus)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "project")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if projectStatus != "Active" {
		respondValidationError(w, r, "cannot start timer on a project that is not active")
		return
	}

	// Check if there's already an active timer (only one timer allowed at a time)
	var existingID int
	err = h.db.QueryRow("SELECT id FROM active_timers LIMIT 1").Scan(&existingID)
	if err != sql.ErrNoRows {
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		// Timer exists
		respondConflict(w, r, "An active timer is already running. Stop it before starting a new one.")
		return
	}

	// Create new timer
	now := time.Now().UTC().Unix()

	var id int64
	err = h.db.QueryRow(`
		INSERT INTO active_timers (workspace_id, item_id, project_id, description, start_time_utc, created_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, req.WorkspaceID, req.ItemID, req.ProjectID, req.Description, now, now).Scan(&id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get the created timer with joined data
	timer, err := h.getActiveTimerByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timer)
}

// GetActiveTimer gets the currently active timer
func (h *ActiveTimerHandler) GetActiveTimer(w http.ResponseWriter, r *http.Request) {
	var timer *models.ActiveTimer

	query := `
		SELECT 
			at.id, at.workspace_id, at.item_id, at.project_id, at.description, 
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
		LEFT JOIN workspaces ws ON at.workspace_id = ws.id
		LIMIT 1
	`

	row := h.db.QueryRow(query)
	timer = &models.ActiveTimer{}

	// Use sql.NullString for nullable joined fields
	var projectName, customerName, itemTitle, workspaceName, workspaceKey sql.NullString

	err := row.Scan(
		&timer.ID, &timer.WorkspaceID, &timer.ItemID, &timer.ProjectID, &timer.Description,
		&timer.StartTimeUTC, &timer.CreatedAt,
		&projectName, &customerName, &itemTitle, &workspaceName, &workspaceKey,
	)

	// Convert sql.NullString to *string
	timer.ProjectName = utils.NullStringToPtr(projectName)
	timer.CustomerName = utils.NullStringToPtr(customerName)
	timer.ItemTitle = utils.NullStringToPtr(itemTitle)
	timer.WorkspaceName = utils.NullStringToPtr(workspaceName)
	timer.WorkspaceKey = utils.NullStringToPtr(workspaceKey)

	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nil)
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timer)
}

// StopTimer stops the active timer and creates a worklog entry
func (h *ActiveTimerHandler) StopTimer(w http.ResponseWriter, r *http.Request) {
	timerIDStr := r.PathValue("id")
	timerID, err := strconv.Atoi(timerIDStr)
	if err != nil {
		respondInvalidID(w, r, "timer ID")
		return
	}

	// Get the timer data before stopping
	timer, err := h.getActiveTimerByID(timerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "timer")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Calculate duration
	endTimeUTC := time.Now().UTC().Unix()
	durationSeconds := endTimeUTC - timer.StartTimeUTC

	// Get the customer_id from the project
	var customerID int
	err = h.db.QueryRow("SELECT customer_id FROM time_projects WHERE id = ?", timer.ProjectID).Scan(&customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create worklog entry
	worklogQuery := `
		INSERT INTO time_worklogs (project_id, customer_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Convert timestamps to integers for the database
	startTime := time.Unix(timer.StartTimeUTC, 0).UTC()
	dateInt := int(startTime.Truncate(24 * time.Hour).Unix()) // Date as unix timestamp
	durationMinutes := int(durationSeconds / 60)              // Convert seconds to minutes

	nowUnix := time.Now().UTC().Unix()
	_, err = h.db.ExecWrite(worklogQuery,
		timer.ProjectID, customerID, timer.ItemID, timer.Description,
		dateInt, int(timer.StartTimeUTC), int(endTimeUTC),
		durationMinutes, nowUnix, nowUnix)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete the active timer
	_, err = h.db.ExecWrite("DELETE FROM active_timers WHERE id = ?", timerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Helper function to safely get string from pointer
	safeString := func(s *string) string {
		if s != nil {
			return *s
		}
		return ""
	}

	// Return the worklog data
	response := map[string]interface{}{
		"timer_id":         timerID,
		"duration_seconds": durationSeconds,
		"worklog_created":  true,
		"start_time_utc":   timer.StartTimeUTC,
		"end_time_utc":     endTimeUTC,
		"description":      timer.Description,
		"project_name":     safeString(timer.ProjectName),
		"item_title":       safeString(timer.ItemTitle),
		"workspace_name":   safeString(timer.WorkspaceName),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getActiveTimerByID is a helper function to retrieve a timer with joined data
func (h *ActiveTimerHandler) getActiveTimerByID(id int) (*models.ActiveTimer, error) {
	query := `
		SELECT 
			at.id, at.workspace_id, at.item_id, at.project_id, at.description, 
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
		LEFT JOIN workspaces ws ON at.workspace_id = ws.id
		WHERE at.id = ?
	`

	timer := &models.ActiveTimer{}

	// Use sql.NullString for nullable joined fields
	var projectName, customerName, itemTitle, workspaceName, workspaceKey sql.NullString

	err := h.db.QueryRow(query, id).Scan(
		&timer.ID, &timer.WorkspaceID, &timer.ItemID, &timer.ProjectID, &timer.Description,
		&timer.StartTimeUTC, &timer.CreatedAt,
		&projectName, &customerName, &itemTitle, &workspaceName, &workspaceKey,
	)

	if err != nil {
		return timer, err
	}

	// Convert sql.NullString to *string
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

	return timer, err
}
