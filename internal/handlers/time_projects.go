package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TimeProjectHandler struct {
	db                    database.Database
	timePermissionService *services.TimePermissionService
	keyCache              *WorkspaceKeyCache
}

func NewTimeProjectHandler(db database.Database, timePermissionService *services.TimePermissionService, keyCache *WorkspaceKeyCache) *TimeProjectHandler {
	return &TimeProjectHandler{
		db:                    db,
		timePermissionService: timePermissionService,
		keyCache:              keyCache,
	}
}

// scanTimeProjectWithJoins scans a row from a query that joins time_projects with
// customer_organisations, time_project_categories, and the total_hours subquery.
func scanTimeProjectWithJoins(scanner interface {
	Scan(dest ...interface{}) error
}) (models.TimeProject, error) {
	var p models.TimeProject
	var customerName, categoryName, categoryColor, status, color, settingsStr sql.NullString
	var totalHours sql.NullFloat64

	err := scanner.Scan(&p.ID, &p.CustomerID, &p.CategoryID, &p.Name, &p.Description, &status, &color,
		&p.HourlyRate, &settingsStr, &p.CreatedAt, &p.UpdatedAt, &customerName, &categoryName, &categoryColor, &totalHours)
	if err != nil {
		return p, err
	}

	p.Status = status.String
	p.Color = color.String
	p.CustomerName = customerName.String
	p.CategoryName = categoryName.String
	p.CategoryColor = categoryColor.String
	if totalHours.Valid {
		p.TotalHours = &totalHours.Float64
	}
	if settingsStr.Valid && settingsStr.String != "" {
		if err := json.Unmarshal([]byte(settingsStr.String), &p.Settings); err != nil {
			slog.Warn("failed to parse project settings", slog.Int("project_id", p.ID), slog.Any("error", err))
		}
	}

	return p, nil
}

// scanTimeProjectRows scans all rows from a query using scanTimeProjectWithJoins and returns them
// as a slice. On scan error it writes the appropriate HTTP error response and returns nil, false.
func scanTimeProjectRows(w http.ResponseWriter, r *http.Request, rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}) ([]models.TimeProject, bool) {
	var projects []models.TimeProject
	for rows.Next() {
		p, err := scanTimeProjectWithJoins(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return nil, false
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return projects, true
}

// marshalTimeProjectSettings serializes the project's Settings map to a JSON
// string pointer suitable for database storage. Returns nil when Settings is empty.
func marshalTimeProjectSettings(settings map[string]interface{}) *string {
	if len(settings) == 0 {
		return nil
	}
	b, err := json.Marshal(settings)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// validateTimeProjectReferences checks that the referenced customer and category exist.
// Returns true if validation passes, false if a response has already been written.
func (h *TimeProjectHandler) validateTimeProjectReferences(w http.ResponseWriter, r *http.Request, customerID, categoryID *int) bool {
	// Customer is required
	if customerID == nil {
		respondValidationError(w, r, "Customer is required")
		return false
	}

	// Validate customer exists
	{
		var customerExists bool
		//nolint:misspell // database table name uses British spelling (customer_organisations)
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM customer_organisations WHERE id = ?)", *customerID).Scan(&customerExists)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !customerExists {
			respondValidationError(w, r, "Customer not found")
			return false
		}
	}

	// Validate category exists (if provided)
	if categoryID != nil {
		var categoryExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM time_project_categories WHERE id = ?)", *categoryID).Scan(&categoryExists)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !categoryExists {
			respondValidationError(w, r, "Category not found")
			return false
		}
	}

	return true
}

// timeProjectSelectPrefix is the shared SELECT column list + FROM/JOIN for every
// time_projects list query in this handler. Consumers append WHERE/ORDER BY.
//
//nolint:misspell // database table name uses British spelling (customer_organisations)
const timeProjectSelectPrefix = `
	SELECT p.id, p.customer_id, p.category_id, p.name, p.description, p.status, p.color,
	       p.hourly_rate, p.settings, p.created_at, p.updated_at,
	       c.name as customer_name, cat.name as category_name, cat.color as category_color,
	       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = p.id) as total_hours
	FROM time_projects p
	LEFT JOIN customer_organisations c ON p.customer_id = c.id
	LEFT JOIN time_project_categories cat ON p.category_id = cat.id`

// respondTimeProjects runs query, scans time-project rows, and writes the result
// (or an empty array) as JSON, handling the usual 500 cascades.
func (h *TimeProjectHandler) respondTimeProjects(w http.ResponseWriter, r *http.Request, query string, args ...interface{}) {
	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	projects, ok := scanTimeProjectRows(w, r, rows)
	if !ok {
		return
	}
	if projects == nil {
		projects = []models.TimeProject{}
	}
	respondJSONOK(w, projects)
}

func (h *TimeProjectHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get accessible project IDs (nil means all accessible)
	var accessibleIDs []int
	if h.timePermissionService != nil {
		var err error
		accessibleIDs, err = h.timePermissionService.GetAccessibleProjects(user.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Build query based on accessible projects
	query := timeProjectSelectPrefix

	var args []interface{}
	if len(accessibleIDs) > 0 {
		// Filter to only accessible projects
		placeholders := make([]string, len(accessibleIDs))
		for i, id := range accessibleIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " WHERE p.id IN (" + strings.Join(placeholders, ",") + ")"
	} else if accessibleIDs != nil {
		// User has no access to any projects (slice is non-nil but empty)
		respondJSONOK(w, []models.TimeProject{})
		return
	}
	// If accessibleIDs is nil, user has full access - no WHERE clause needed

	query += " ORDER BY p.name ASC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	projects, ok := scanTimeProjectRows(w, r, rows)
	if !ok {
		return
	}

	// Set IsManager flag for each project
	if h.timePermissionService != nil {
		for i := range projects {
			isManager, err := h.timePermissionService.IsTimeProjectManager(user.ID, projects[i].ID)
			if err != nil {
				slog.Warn("failed to check manager status", slog.Int("project_id", projects[i].ID), slog.Any("error", err))
				continue
			}
			projects[i].IsManager = isManager
		}
	}

	respondJSONOK(w, projects)
}

func (h *TimeProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check view permission
	if h.timePermissionService != nil {
		canView, err := h.timePermissionService.CanViewProject(user.ID, id)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canView {
			respondForbidden(w, r)
			return
		}
	}

	//nolint:misspell // database table name uses British spelling (customer_organisations)
	p, err := scanTimeProjectWithJoins(h.db.QueryRow(`
		SELECT p.id, p.customer_id, p.category_id, p.name, p.description, p.status, p.color,
		       p.hourly_rate, p.settings, p.created_at, p.updated_at,
		       c.name as customer_name, cat.name as category_name, cat.color as category_color,
		       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = p.id) as total_hours
		FROM time_projects p
		LEFT JOIN customer_organisations c ON p.customer_id = c.id
		LEFT JOIN time_project_categories cat ON p.category_id = cat.id
		WHERE p.id = ?
	`, id))

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "project")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, p)
}

func (h *TimeProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check project.manage permission (required to create new projects)
	if h.timePermissionService != nil {
		hasPermission, err := h.timePermissionService.HasProjectManagePermission(user.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !hasPermission {
			respondForbidden(w, r)
			return
		}
	}

	p, ok := decodeJSON[models.TimeProject](w, r)
	if !ok {
		return
	}

	// Set default status if not provided
	if p.Status == "" {
		p.Status = "Active"
	}

	// Validate customer and category references
	if !h.validateTimeProjectReferences(w, r, p.CustomerID, p.CategoryID) {
		return
	}

	settingsJSON := marshalTimeProjectSettings(p.Settings)

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO time_projects (customer_id, category_id, name, description, status, color, hourly_rate, settings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, p.CustomerID, p.CategoryID, p.Name, p.Description, p.Status, p.Color, p.HourlyRate, settingsJSON, now, now).Scan(&id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	p.ID = int(id)
	p.CreatedAt = now
	p.UpdatedAt = now

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		projectID := p.ID
		logAudit(h.db, r, currentUser, logger.ActionTimeProjectCreate, logger.ResourceTimeProject, &projectID, p.Name)
	}

	respondJSONCreated(w, p)
}

func (h *TimeProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check manager permission (required to update project)
	if h.timePermissionService != nil {
		isManager, err := h.timePermissionService.IsTimeProjectManager(user.ID, id)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !isManager {
			respondForbidden(w, r)
			return
		}
	}

	p, ok := decodeJSON[models.TimeProject](w, r)
	if !ok {
		return
	}

	// Validate customer and category references
	if !h.validateTimeProjectReferences(w, r, p.CustomerID, p.CategoryID) {
		return
	}

	settingsJSON := marshalTimeProjectSettings(p.Settings)

	now := time.Now()
	_, err := h.db.Exec(`
		UPDATE time_projects
		SET customer_id = ?, category_id = ?, name = ?, description = ?, status = ?, color = ?,
		    hourly_rate = ?, settings = ?, updated_at = ?
		WHERE id = ?
	`, p.CustomerID, p.CategoryID, p.Name, p.Description, p.Status, p.Color, p.HourlyRate, settingsJSON, now, id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	p.ID = id
	p.UpdatedAt = now

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionTimeProjectUpdate, logger.ResourceTimeProject, &id, p.Name)
	}

	respondJSONOK(w, p)
}

func (h *TimeProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check project.manage permission (only global permission can delete projects)
	if h.timePermissionService != nil {
		hasPermission, err := h.timePermissionService.HasProjectManagePermission(user.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !hasPermission {
			respondForbidden(w, r)
			return
		}
	}

	_, err := h.db.Exec("DELETE FROM time_projects WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionTimeProjectDelete, logger.ResourceTimeProject, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TimeProjectHandler) GetByCustomer(w http.ResponseWriter, r *http.Request) {
	customerID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	h.respondTimeProjects(w, r, timeProjectSelectPrefix+`
		WHERE p.customer_id = ?
		ORDER BY p.name ASC`, customerID)
}

func (h *TimeProjectHandler) GetByWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "id")
	if !ok {
		return
	}

	// Check if workspace has category restrictions
	categoryRows, err := h.db.Query(`
		SELECT time_project_category_id
		FROM workspace_time_project_categories
		WHERE workspace_id = ?
	`, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var allowedCategories []int
	for categoryRows.Next() {
		var categoryID int
		if err = categoryRows.Scan(&categoryID); err != nil {
			_ = categoryRows.Close()
			respondInternalError(w, r, err)
			return
		}
		allowedCategories = append(allowedCategories, categoryID)
	}
	_ = categoryRows.Close()

	// Build query based on whether there are category restrictions
	var query string
	var args []interface{}

	if len(allowedCategories) > 0 {
		// Workspace has category restrictions - filter projects
		placeholders := make([]string, len(allowedCategories))
		for i, categoryID := range allowedCategories {
			placeholders[i] = "?"
			args = append(args, categoryID)
		}
		query = timeProjectSelectPrefix + `
			WHERE p.category_id IN (` + strings.Join(placeholders, ",") + `)
			ORDER BY p.name ASC`
	} else {
		query = timeProjectSelectPrefix + `
			ORDER BY p.name ASC`
	}

	h.respondTimeProjects(w, r, query, args...)
}
