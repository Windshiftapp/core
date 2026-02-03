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

type NotificationTemplateHandler struct {
	*BaseHandler
}

func NewNotificationTemplateHandlerWithPool(db database.Database) *NotificationTemplateHandler {
	return &NotificationTemplateHandler{
		BaseHandler: NewBaseHandler(db),
	}
}

// GetAllTemplates handles GET /api/notification-templates
func (h *NotificationTemplateHandler) GetAllTemplates(w http.ResponseWriter, r *http.Request) {
	templateType := r.URL.Query().Get("type") // Optional filter by type

	query := `
		SELECT id, name, template_type, subject, content, description, is_active, created_at, updated_at
		FROM notification_templates
		WHERE 1=1
	`
	args := []interface{}{}

	if templateType != "" {
		query += " AND template_type = ?"
		args = append(args, templateType)
	}

	query += " ORDER BY template_type, name"

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	var templates []models.NotificationTemplate
	for rows.Next() {
		var template models.NotificationTemplate
		var subject sql.NullString

		err := rows.Scan(
			&template.ID,
			&template.Name,
			&template.TemplateType,
			&subject,
			&template.Content,
			&template.Description,
			&template.IsActive,
			&template.CreatedAt,
			&template.UpdatedAt,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if subject.Valid {
			template.Subject = subject.String
		}

		templates = append(templates, template)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templates)
}

// GetTemplate handles GET /api/notification-templates/{id}
func (h *NotificationTemplateHandler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	var template models.NotificationTemplate
	var subject sql.NullString

	err = db.QueryRow(`
		SELECT id, name, template_type, subject, content, description, is_active, created_at, updated_at
		FROM notification_templates
		WHERE id = ?
	`, id).Scan(
		&template.ID,
		&template.Name,
		&template.TemplateType,
		&subject,
		&template.Content,
		&template.Description,
		&template.IsActive,
		&template.CreatedAt,
		&template.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "template")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if subject.Valid {
		template.Subject = subject.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(template)
}

// CreateTemplate handles POST /api/notification-templates
func (h *NotificationTemplateHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var template models.NotificationTemplate
	if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	// Validate required fields
	if template.Name == "" || template.TemplateType == "" || template.Content == "" {
		respondValidationError(w, r, "Name, template_type, and content are required")
		return
	}

	// Validate template type
	if template.TemplateType != "header" && template.TemplateType != "footer" && template.TemplateType != "notification_type" {
		respondValidationError(w, r, "Invalid template_type. Must be 'header', 'footer', or 'notification_type'")
		return
	}

	// Sanitize user input to prevent XSS
	template.Name = utils.StripHTMLTags(template.Name)
	template.Subject = utils.StripHTMLTags(template.Subject)
	template.Content = utils.StripHTMLTags(template.Content)
	template.Description = utils.StripHTMLTags(template.Description)

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	now := time.Now()
	var id int64
	err := db.QueryRow(`
		INSERT INTO notification_templates (name, template_type, subject, content, description, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, template.Name, template.TemplateType, nullableString(template.Subject), template.Content, template.Description, template.IsActive, now, now).Scan(&id)

	if err != nil {
		if err.Error() == "UNIQUE constraint failed: notification_templates.name" {
			respondConflict(w, r, "Template name already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	template.ID = int(id)
	template.CreatedAt = now
	template.UpdatedAt = now

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(template)
}

// UpdateTemplate handles PUT /api/notification-templates/{id}
func (h *NotificationTemplateHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var template models.NotificationTemplate
	if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	// Validate required fields
	if template.Name == "" || template.TemplateType == "" || template.Content == "" {
		respondValidationError(w, r, "Name, template_type, and content are required")
		return
	}

	// Validate template type
	if template.TemplateType != "header" && template.TemplateType != "footer" && template.TemplateType != "notification_type" {
		respondValidationError(w, r, "Invalid template_type. Must be 'header', 'footer', or 'notification_type'")
		return
	}

	// Sanitize user input to prevent XSS
	template.Name = utils.StripHTMLTags(template.Name)
	template.Subject = utils.StripHTMLTags(template.Subject)
	template.Content = utils.StripHTMLTags(template.Content)
	template.Description = utils.StripHTMLTags(template.Description)

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	now := time.Now()
	result, err := db.Exec(`
		UPDATE notification_templates
		SET name = ?, template_type = ?, subject = ?, content = ?, description = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, template.Name, template.TemplateType, nullableString(template.Subject), template.Content, template.Description, template.IsActive, now, id)

	if err != nil {
		if err.Error() == "UNIQUE constraint failed: notification_templates.name" {
			respondConflict(w, r, "Template name already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "template")
		return
	}

	// Return updated template
	template.ID = id
	template.UpdatedAt = now

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(template)
}

// DeleteTemplate handles DELETE /api/notification-templates/{id}
func (h *NotificationTemplateHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	result, err := db.Exec(`DELETE FROM notification_templates WHERE id = ?`, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "template")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}