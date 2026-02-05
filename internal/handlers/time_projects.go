package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
)

type TimeProjectHandler struct {
	db                    database.Database
	timePermissionService *services.TimePermissionService
}

func NewTimeProjectHandler(db database.Database, timePermissionService *services.TimePermissionService) *TimeProjectHandler {
	return &TimeProjectHandler{
		db:                    db,
		timePermissionService: timePermissionService,
	}
}

func (h *TimeProjectHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
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
	//nolint:misspell // database table name uses British spelling (customer_organisations)
	query := `
		SELECT p.id, p.customer_id, p.category_id, p.name, p.description, p.status, p.color,
		       p.hourly_rate, p.settings, p.created_at, p.updated_at,
		       c.name as customer_name, cat.name as category_name, cat.color as category_color,
		       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = p.id) as total_hours
		FROM time_projects p
		LEFT JOIN customer_organisations c ON p.customer_id = c.id
		LEFT JOIN time_project_categories cat ON p.category_id = cat.id`

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

	var projects []models.TimeProject
	for rows.Next() {
		var p models.TimeProject
		var customerName, categoryName, categoryColor, status, color, settingsStr sql.NullString
		var totalHours sql.NullFloat64

		err := rows.Scan(&p.ID, &p.CustomerID, &p.CategoryID, &p.Name, &p.Description, &status, &color,
			&p.HourlyRate, &settingsStr, &p.CreatedAt, &p.UpdatedAt, &customerName, &categoryName, &categoryColor, &totalHours)
		if err != nil {
			respondInternalError(w, r, err)
			return
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

		projects = append(projects, p)
	}

	respondJSONOK(w, projects)
}

func (h *TimeProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
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

	var p models.TimeProject
	var status, color, settingsStr sql.NullString
	var totalHours sql.NullFloat64

	err := h.db.QueryRow(`
		SELECT id, customer_id, category_id, name, description, status, color,
		       hourly_rate, settings, created_at, updated_at,
		       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = time_projects.id) as total_hours
		FROM time_projects
		WHERE id = ?
	`, id).Scan(&p.ID, &p.CustomerID, &p.CategoryID, &p.Name, &p.Description, &status, &color,
		&p.HourlyRate, &settingsStr, &p.CreatedAt, &p.UpdatedAt, &totalHours)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "project")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	p.Status = status.String
	p.Color = color.String
	if totalHours.Valid {
		p.TotalHours = &totalHours.Float64
	}
	if settingsStr.Valid && settingsStr.String != "" {
		if err := json.Unmarshal([]byte(settingsStr.String), &p.Settings); err != nil {
			slog.Warn("failed to parse project settings", slog.Int("project_id", p.ID), slog.Any("error", err))
		}
	}

	respondJSONOK(w, p)
}

func (h *TimeProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Get user from context
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
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

	var p models.TimeProject
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	// Set default status if not provided
	if p.Status == "" {
		p.Status = "Active"
	}

	// Validate customer exists (if provided)
	if p.CustomerID != nil {
		var customerExists bool
		//nolint:misspell // database table name uses British spelling (customer_organisations)
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM customer_organisations WHERE id = ?)", *p.CustomerID).Scan(&customerExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !customerExists {
			respondValidationError(w, r, "Customer not found")
			return
		}
	}

	// Validate category exists (if provided)
	if p.CategoryID != nil {
		var categoryExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM time_project_categories WHERE id = ?)", *p.CategoryID).Scan(&categoryExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !categoryExists {
			respondValidationError(w, r, "Category not found")
			return
		}
	}

	// Serialize settings to JSON
	var settingsJSON *string
	if len(p.Settings) > 0 {
		b, err := json.Marshal(p.Settings)
		if err == nil {
			s := string(b)
			settingsJSON = &s
		}
	}

	now := time.Now()
	result, err := h.db.Exec(`
		INSERT INTO time_projects (customer_id, category_id, name, description, status, color, hourly_rate, settings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.CustomerID, p.CategoryID, p.Name, p.Description, p.Status, p.Color, p.HourlyRate, settingsJSON, now, now)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	p.ID = int(id)
	p.CreatedAt = now
	p.UpdatedAt = now

	respondJSONCreated(w, p)
}

func (h *TimeProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
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

	var p models.TimeProject
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	// Validate customer exists (if provided)
	if p.CustomerID != nil {
		var customerExists bool
		//nolint:misspell // database table name uses British spelling (customer_organisations)
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM customer_organisations WHERE id = ?)", *p.CustomerID).Scan(&customerExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !customerExists {
			respondValidationError(w, r, "Customer not found")
			return
		}
	}

	// Validate category exists (if provided)
	if p.CategoryID != nil {
		var categoryExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM time_project_categories WHERE id = ?)", *p.CategoryID).Scan(&categoryExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !categoryExists {
			respondValidationError(w, r, "Category not found")
			return
		}
	}

	// Serialize settings to JSON
	var settingsJSON *string
	if len(p.Settings) > 0 {
		b, err := json.Marshal(p.Settings)
		if err == nil {
			s := string(b)
			settingsJSON = &s
		}
	}

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

	respondJSONOK(w, p)
}

func (h *TimeProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
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

	w.WriteHeader(http.StatusNoContent)
}

func (h *TimeProjectHandler) GetByCustomer(w http.ResponseWriter, r *http.Request) {
	customerID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	//nolint:misspell // database table name uses British spelling (customer_organisations)
	rows, err := h.db.Query(`
		SELECT p.id, p.customer_id, p.category_id, p.name, p.description, p.status, p.color,
		       p.hourly_rate, p.settings, p.created_at, p.updated_at,
		       c.name as customer_name, cat.name as category_name, cat.color as category_color,
		       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = p.id) as total_hours
		FROM time_projects p
		LEFT JOIN customer_organisations c ON p.customer_id = c.id
		LEFT JOIN time_project_categories cat ON p.category_id = cat.id
		WHERE p.customer_id = ?
		ORDER BY p.name ASC
	`, customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var projects []models.TimeProject
	for rows.Next() {
		var p models.TimeProject
		var customerName, categoryName, categoryColor, status, color, settingsStr sql.NullString
		var totalHours sql.NullFloat64

		err := rows.Scan(&p.ID, &p.CustomerID, &p.CategoryID, &p.Name, &p.Description, &status, &color,
			&p.HourlyRate, &settingsStr, &p.CreatedAt, &p.UpdatedAt, &customerName, &categoryName, &categoryColor, &totalHours)
		if err != nil {
			respondInternalError(w, r, err)
			return
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

		projects = append(projects, p)
	}

	if projects == nil {
		projects = []models.TimeProject{}
	}

	respondJSONOK(w, projects)
}

func (h *TimeProjectHandler) GetByWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
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

		//nolint:misspell // database table name uses British spelling (customer_organisations)
		query = `
			SELECT p.id, p.customer_id, p.category_id, p.name, p.description, p.status, p.color,
			       p.hourly_rate, p.settings, p.created_at, p.updated_at,
			       c.name as customer_name, cat.name as category_name, cat.color as category_color,
			       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = p.id) as total_hours
			FROM time_projects p
			LEFT JOIN customer_organisations c ON p.customer_id = c.id
			LEFT JOIN time_project_categories cat ON p.category_id = cat.id
			WHERE p.category_id IN (` + strings.Join(placeholders, ",") + `)
			ORDER BY p.name ASC
		`
	} else {
		// No category restrictions - return all projects
		//nolint:misspell // database table name uses British spelling (customer_organisations)
		query = `
			SELECT p.id, p.customer_id, p.category_id, p.name, p.description, p.status, p.color,
			       p.hourly_rate, p.settings, p.created_at, p.updated_at,
			       c.name as customer_name, cat.name as category_name, cat.color as category_color,
			       (SELECT COALESCE(SUM(duration_minutes), 0) / 60.0 FROM time_worklogs WHERE project_id = p.id) as total_hours
			FROM time_projects p
			LEFT JOIN customer_organisations c ON p.customer_id = c.id
			LEFT JOIN time_project_categories cat ON p.category_id = cat.id
			ORDER BY p.name ASC
		`
	}

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var projects []models.TimeProject
	for rows.Next() {
		var p models.TimeProject
		var customerName, categoryName, categoryColor, status, color, settingsStr sql.NullString
		var totalHours sql.NullFloat64

		err := rows.Scan(&p.ID, &p.CustomerID, &p.CategoryID, &p.Name, &p.Description, &status, &color,
			&p.HourlyRate, &settingsStr, &p.CreatedAt, &p.UpdatedAt, &customerName, &categoryName, &categoryColor, &totalHours)
		if err != nil {
			respondInternalError(w, r, err)
			return
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

		projects = append(projects, p)
	}

	if projects == nil {
		projects = []models.TimeProject{}
	}

	respondJSONOK(w, projects)
}
