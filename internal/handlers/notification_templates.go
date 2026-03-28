package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// validateNotificationTemplate checks required fields, template type, and sanitizes inputs.
// Returns true if valid; writes an error response and returns false otherwise.
func validateNotificationTemplate(w http.ResponseWriter, r *http.Request, t *models.NotificationTemplate) bool {
	if t.Name == "" || t.TemplateType == "" || t.Content == "" {
		respondValidationError(w, r, "Name, template_type, and content are required")
		return false
	}
	if t.TemplateType != "header" && t.TemplateType != "footer" && t.TemplateType != "notification_type" {
		respondValidationError(w, r, "Invalid template_type. Must be 'header', 'footer', or 'notification_type'")
		return false
	}
	t.Name = utils.SanitizeName(t.Name)
	t.Subject = utils.SanitizeCommentContent(t.Subject)
	t.Content = utils.SanitizeCommentContent(t.Content)
	t.Description = utils.SanitizeCommentContent(t.Description)
	return true
}

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
	defer func() { _ = rows.Close() }()

	var templates []models.NotificationTemplate
	for rows.Next() {
		var template models.NotificationTemplate
		var subject sql.NullString

		err = rows.Scan(
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

	respondJSONOK(w, templates)
}

// GetTemplate handles GET /api/notification-templates/{id}
func (h *NotificationTemplateHandler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

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

	respondJSONOK(w, template)
}

// CreateTemplate handles POST /api/notification-templates
func (h *NotificationTemplateHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	template, ok := decodeJSON[models.NotificationTemplate](w, r)
	if !ok {
		return
	}

	if !validateNotificationTemplate(w, r, &template) {
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	// Check uniqueness before insert
	var nameExists bool
	_ = db.QueryRow("SELECT EXISTS(SELECT 1 FROM notification_templates WHERE name = ?)", template.Name).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Template name already exists")
		return
	}

	now := time.Now()
	var id int64
	err := db.QueryRow(`
		INSERT INTO notification_templates (name, template_type, subject, content, description, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, template.Name, template.TemplateType, nullableString(template.Subject), template.Content, template.Description, template.IsActive, now, now).Scan(&id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Template name already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	template.ID = int(id)
	template.CreatedAt = now
	template.UpdatedAt = now

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		intID := int(id)
		logAudit(h.db, r, currentUser, logger.ActionNotificationTemplateCreate, logger.ResourceNotificationTemplate, &intID, template.Name)
	}

	respondJSONCreated(w, template)
}

// UpdateTemplate handles PUT /api/notification-templates/{id}
func (h *NotificationTemplateHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	template, ok := decodeJSON[models.NotificationTemplate](w, r)
	if !ok {
		return
	}

	if !validateNotificationTemplate(w, r, &template) {
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	// Check uniqueness before update
	var nameExists bool
	_ = db.QueryRow("SELECT EXISTS(SELECT 1 FROM notification_templates WHERE name = ? AND id != ?)", template.Name, id).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Template name already exists")
		return
	}

	now := time.Now()
	result, err := db.Exec(`
		UPDATE notification_templates
		SET name = ?, template_type = ?, subject = ?, content = ?, description = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, template.Name, template.TemplateType, nullableString(template.Subject), template.Content, template.Description, template.IsActive, now, id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
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

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionNotificationTemplateUpdate, logger.ResourceNotificationTemplate, &id, template.Name)
	}

	respondJSONOK(w, template)
}

// DeleteTemplate handles DELETE /api/notification-templates/{id}
func (h *NotificationTemplateHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
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

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionNotificationTemplateDelete, logger.ResourceNotificationTemplate, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}
