package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"

)

type ConfigurationSetNotificationHandler struct {
	db database.Database
}

func NewConfigurationSetNotificationHandler(db database.Database) *ConfigurationSetNotificationHandler {
	return &ConfigurationSetNotificationHandler{db: db}
}

// GetConfigurationSetNotifications returns all notification settings for a configuration set
func (h *ConfigurationSetNotificationHandler) GetConfigurationSetNotifications(w http.ResponseWriter, r *http.Request) {
	configSetIDStr := r.PathValue("config_set_id")
	configSetID, err := strconv.Atoi(configSetIDStr)
	if err != nil {
		http.Error(w, "Invalid configuration set ID", http.StatusBadRequest)
		return
	}

	query := `
		SELECT 
			csns.id, csns.configuration_set_id, csns.notification_setting_id, csns.created_at,
			cs.name as configuration_set_name,
			ns.name as notification_setting_name, ns.description, ns.is_active
		FROM configuration_set_notification_settings csns
		JOIN configuration_sets cs ON csns.configuration_set_id = cs.id
		JOIN notification_settings ns ON csns.notification_setting_id = ns.id
		WHERE csns.configuration_set_id = ?
		ORDER BY ns.name
	`

	rows, err := h.db.GetDB().Query(query, configSetID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database query error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var assignments []models.ConfigurationSetNotificationSetting
	for rows.Next() {
		var a models.ConfigurationSetNotificationSetting
		var createdAtStr string
		var description *string
		var isActive bool

		err := rows.Scan(
			&a.ID, &a.ConfigurationSetID, &a.NotificationSettingID, &createdAtStr,
			&a.ConfigurationSetName, &a.NotificationSettingName, &description, &isActive,
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error scanning row: %v", err), http.StatusInternalServerError)
			return
		}

		// Parse timestamp
		if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
			a.CreatedAt = createdAt
		}

		assignments = append(assignments, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assignments)
}

// AssignNotificationToConfigurationSet assigns a notification setting to a configuration set
func (h *ConfigurationSetNotificationHandler) AssignNotificationToConfigurationSet(w http.ResponseWriter, r *http.Request) {
	configSetIDStr := r.PathValue("config_set_id")
	configSetID, err := strconv.Atoi(configSetIDStr)
	if err != nil {
		http.Error(w, "Invalid configuration set ID", http.StatusBadRequest)
		return
	}

	var req struct {
		NotificationSettingID int `json:"notification_setting_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.NotificationSettingID == 0 {
		http.Error(w, "notification_setting_id is required", http.StatusBadRequest)
		return
	}

	// Check if configuration set exists
	var csName string
	err = h.db.GetDB().QueryRow("SELECT name FROM configuration_sets WHERE id = ?", configSetID).Scan(&csName)
	if err != nil {
		http.Error(w, "Configuration set not found", http.StatusNotFound)
		return
	}

	// Check if notification setting exists and is active
	var nsName string
	var isActive bool
	err = h.db.GetDB().QueryRow("SELECT name, is_active FROM notification_settings WHERE id = ?", req.NotificationSettingID).Scan(&nsName, &isActive)
	if err != nil {
		http.Error(w, "Notification setting not found", http.StatusNotFound)
		return
	}

	if !isActive {
		http.Error(w, "Cannot assign inactive notification setting", http.StatusBadRequest)
		return
	}

	// Insert assignment (will fail if already exists due to unique constraint)
	var id int64
	err = h.db.GetDB().QueryRow(`
		INSERT INTO configuration_set_notification_settings
		(configuration_set_id, notification_setting_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP) RETURNING id
	`, configSetID, req.NotificationSettingID).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			http.Error(w, "Notification setting is already assigned to this configuration set", http.StatusConflict)
		} else {
			http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Return the assignment
	assignment := models.ConfigurationSetNotificationSetting{
		ID:                      int(id),
		ConfigurationSetID:      configSetID,
		NotificationSettingID:   req.NotificationSettingID,
		CreatedAt:              time.Now(),
		ConfigurationSetName:    csName,
		NotificationSettingName: nsName,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(assignment)
}

// UnassignNotificationFromConfigurationSet removes a notification setting from a configuration set
func (h *ConfigurationSetNotificationHandler) UnassignNotificationFromConfigurationSet(w http.ResponseWriter, r *http.Request) {
	configSetIDStr := r.PathValue("config_set_id")
	assignmentIDStr := r.PathValue("assignment_id")

	configSetID, err := strconv.Atoi(configSetIDStr)
	if err != nil {
		http.Error(w, "Invalid configuration set ID", http.StatusBadRequest)
		return
	}

	assignmentID, err := strconv.Atoi(assignmentIDStr)
	if err != nil {
		http.Error(w, "Invalid assignment ID", http.StatusBadRequest)
		return
	}

	// Delete the assignment
	result, err := h.db.GetDB().Exec(`
		DELETE FROM configuration_set_notification_settings 
		WHERE id = ? AND configuration_set_id = ?
	`, assignmentID, configSetID)
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
		http.Error(w, "Assignment not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAvailableNotificationSettings returns notification settings not yet assigned to a configuration set
func (h *ConfigurationSetNotificationHandler) GetAvailableNotificationSettings(w http.ResponseWriter, r *http.Request) {
	configSetIDStr := r.PathValue("config_set_id")
	configSetID, err := strconv.Atoi(configSetIDStr)
	if err != nil {
		http.Error(w, "Invalid configuration set ID", http.StatusBadRequest)
		return
	}

	query := `
		SELECT
			ns.id, ns.name, ns.description, ns.is_active, ns.created_by, ns.created_at, ns.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name
		FROM notification_settings ns
		LEFT JOIN users u ON ns.created_by = u.id
		WHERE ns.is_active = true
		  AND ns.id NOT IN (
			  SELECT notification_setting_id
			  FROM configuration_set_notification_settings
			  WHERE configuration_set_id = ?
		  )
		ORDER BY ns.name
	`

	rows, err := h.db.GetDB().Query(query, configSetID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database query error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var settings []models.NotificationSetting
	for rows.Next() {
		var s models.NotificationSetting
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&s.ID, &s.Name, &s.Description, &s.IsActive, &s.CreatedBy, &createdAtStr, &updatedAtStr,
			&s.CreatedByName,
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error scanning row: %v", err), http.StatusInternalServerError)
			return
		}

		// Parse timestamps
		if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
			s.CreatedAt = createdAt
		}
		if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
			s.UpdatedAt = updatedAt
		}

		settings = append(settings, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}