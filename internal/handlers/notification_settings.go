package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"

)

type NotificationSettingsHandler struct {
	db database.Database
}

func NewNotificationSettingsHandler(db database.Database) *NotificationSettingsHandler {
	return &NotificationSettingsHandler{db: db}
}

// GetNotificationSettings returns all notification settings with their event rules
func (h *NotificationSettingsHandler) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT 
			ns.id, ns.name, ns.description, ns.is_active, ns.created_by, ns.created_at, ns.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name
		FROM notification_settings ns
		LEFT JOIN users u ON ns.created_by = u.id
		ORDER BY ns.created_at DESC
	`

	rows, err := h.db.GetDB().Query(query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database query error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var settings []models.NotificationSetting
	for rows.Next() {
		var s models.NotificationSetting
		var createdAtStr, updatedAtStr string
		var createdBy sql.NullInt64
		var createdByName sql.NullString

		err := rows.Scan(
			&s.ID, &s.Name, &s.Description, &s.IsActive, &createdBy, &createdAtStr, &updatedAtStr,
			&createdByName,
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error scanning row: %v", err), http.StatusInternalServerError)
			return
		}

		if createdBy.Valid {
			s.CreatedBy = int(createdBy.Int64)
		}
		if createdByName.Valid {
			s.CreatedByName = createdByName.String
		}

		// Parse timestamps
		if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
			s.CreatedAt = createdAt
		}
		if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
			s.UpdatedAt = updatedAt
		}

		// Load event rules for this setting
		eventRules, err := h.getEventRulesForSetting(s.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error loading event rules: %v", err), http.StatusInternalServerError)
			return
		}
		s.EventRules = eventRules

		settings = append(settings, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

// GetNotificationSetting returns a specific notification setting by ID
func (h *NotificationSettingsHandler) GetNotificationSetting(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid notification setting ID", http.StatusBadRequest)
		return
	}

	query := `
		SELECT 
			ns.id, ns.name, ns.description, ns.is_active, ns.created_by, ns.created_at, ns.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name
		FROM notification_settings ns
		LEFT JOIN users u ON ns.created_by = u.id
		WHERE ns.id = ?
	`

	var s models.NotificationSetting
	var createdAtStr, updatedAtStr string
	var createdBy sql.NullInt64
	var createdByName sql.NullString

	err = h.db.GetDB().QueryRow(query, id).Scan(
		&s.ID, &s.Name, &s.Description, &s.IsActive, &createdBy, &createdAtStr, &updatedAtStr,
		&createdByName,
	)
	if err != nil {
		http.Error(w, "Notification setting not found", http.StatusNotFound)
		return
	}

	if createdBy.Valid {
		s.CreatedBy = int(createdBy.Int64)
	}
	if createdByName.Valid {
		s.CreatedByName = createdByName.String
	}

	// Parse timestamps
	if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
		s.CreatedAt = createdAt
	}
	if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
		s.UpdatedAt = updatedAt
	}

	// Load event rules for this setting
	eventRules, err := h.getEventRulesForSetting(s.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading event rules: %v", err), http.StatusInternalServerError)
		return
	}
	s.EventRules = eventRules

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

// CreateNotificationSetting creates a new notification setting
func (h *NotificationSettingsHandler) CreateNotificationSetting(w http.ResponseWriter, r *http.Request) {
	var req models.NotificationSetting
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.CreatedBy == 0 {
		http.Error(w, "CreatedBy is required", http.StatusBadRequest)
		return
	}

	// Insert notification setting
	var id int64
	err := h.db.GetDB().QueryRow(`
		INSERT INTO notification_settings (name, description, is_active, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, req.Name, req.Description, req.IsActive, req.CreatedBy).Scan(&id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Insert event rules if provided
	if len(req.EventRules) > 0 {
		for _, rule := range req.EventRules {
			_, err := h.db.GetDB().Exec(`
				INSERT INTO notification_event_rules 
				(notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator, 
				 notify_watchers, notify_workspace_admins, custom_recipients, message_template, 
				 created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			`, id, rule.EventType, rule.IsEnabled, rule.NotifyAssignee, rule.NotifyCreator,
				rule.NotifyWatchers, rule.NotifyWorkspaceAdmins, rule.CustomRecipients, rule.MessageTemplate)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error inserting event rule: %v", err), http.StatusInternalServerError)
				return
			}
		}
	}

	// Return the created setting
	req.ID = int(id)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(req)
}

// UpdateNotificationSetting updates an existing notification setting
func (h *NotificationSettingsHandler) UpdateNotificationSetting(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid notification setting ID", http.StatusBadRequest)
		return
	}

	var req models.NotificationSetting
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Start transaction for updating setting and its rules
	tx, err := h.db.GetDB().Begin()
	if err != nil {
		http.Error(w, fmt.Sprintf("Transaction error: %v", err), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Update notification setting
	_, err = tx.Exec(`
		UPDATE notification_settings 
		SET name = ?, description = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, req.Name, req.Description, req.IsActive, id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Delete existing event rules
	_, err = tx.Exec(`DELETE FROM notification_event_rules WHERE notification_setting_id = ?`, id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error deleting existing rules: %v", err), http.StatusInternalServerError)
		return
	}

	// Insert new event rules
	if len(req.EventRules) > 0 {
		for _, rule := range req.EventRules {
			_, err := tx.Exec(`
				INSERT INTO notification_event_rules 
				(notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator, 
				 notify_watchers, notify_workspace_admins, custom_recipients, message_template, 
				 created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			`, id, rule.EventType, rule.IsEnabled, rule.NotifyAssignee, rule.NotifyCreator,
				rule.NotifyWatchers, rule.NotifyWorkspaceAdmins, rule.CustomRecipients, rule.MessageTemplate)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error inserting event rule: %v", err), http.StatusInternalServerError)
				return
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		http.Error(w, fmt.Sprintf("Transaction commit error: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the updated setting
	req.ID = id
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}

// DeleteNotificationSetting deletes a notification setting
func (h *NotificationSettingsHandler) DeleteNotificationSetting(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid notification setting ID", http.StatusBadRequest)
		return
	}

	// Check if this setting is assigned to any configuration sets
	var count int
	err = h.db.GetDB().QueryRow(`
		SELECT COUNT(*) FROM configuration_set_notification_settings 
		WHERE notification_setting_id = ?
	`, id).Scan(&count)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	if count > 0 {
		http.Error(w, "Cannot delete notification setting: it is assigned to one or more configuration sets", http.StatusConflict)
		return
	}

	// Delete the notification setting (event rules will be cascade deleted)
	result, err := h.db.GetDB().Exec(`DELETE FROM notification_settings WHERE id = ?`, id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error checking result: %v", err), http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, "Notification setting not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAvailableEvents returns all available notification event types
func (h *NotificationSettingsHandler) GetAvailableEvents(w http.ResponseWriter, r *http.Request) {
	events := models.GetAvailableNotificationEvents()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// Helper function to get event rules for a notification setting
func (h *NotificationSettingsHandler) getEventRulesForSetting(settingID int) ([]models.NotificationEventRule, error) {
	query := `
		SELECT id, notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator,
			   notify_watchers, notify_workspace_admins, custom_recipients, message_template, 
			   created_at, updated_at
		FROM notification_event_rules
		WHERE notification_setting_id = ?
		ORDER BY event_type
	`

	rows, err := h.db.GetDB().Query(query, settingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.NotificationEventRule
	for rows.Next() {
		var rule models.NotificationEventRule
		var createdAtStr, updatedAtStr string
		var customRecipients, messageTemplate *string

		err := rows.Scan(
			&rule.ID, &rule.NotificationSettingID, &rule.EventType, &rule.IsEnabled,
			&rule.NotifyAssignee, &rule.NotifyCreator, &rule.NotifyWatchers, &rule.NotifyWorkspaceAdmins,
			&customRecipients, &messageTemplate, &createdAtStr, &updatedAtStr,
		)
		if err != nil {
			return nil, err
		}

		if customRecipients != nil {
			rule.CustomRecipients = *customRecipients
		}
		if messageTemplate != nil {
			rule.MessageTemplate = *messageTemplate
		}

		// Parse timestamps
		if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
			rule.CreatedAt = createdAt
		}
		if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
			rule.UpdatedAt = updatedAt
		}

		rules = append(rules, rule)
	}

	return rules, nil
}