package handlers

import (
	"encoding/json"
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
		respondInvalidID(w, r, "config_set_id")
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
		respondInternalError(w, r, err)
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
			respondInternalError(w, r, err)
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
		respondInvalidID(w, r, "config_set_id")
		return
	}

	var req struct {
		NotificationSettingID int `json:"notification_setting_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	if req.NotificationSettingID == 0 {
		respondValidationError(w, r, "notification_setting_id is required")
		return
	}

	// Check if configuration set exists
	var csName string
	err = h.db.GetDB().QueryRow("SELECT name FROM configuration_sets WHERE id = ?", configSetID).Scan(&csName)
	if err != nil {
		respondNotFound(w, r, "Configuration set")
		return
	}

	// Check if notification setting exists and is active
	var nsName string
	var isActive bool
	err = h.db.GetDB().QueryRow("SELECT name, is_active FROM notification_settings WHERE id = ?", req.NotificationSettingID).Scan(&nsName, &isActive)
	if err != nil {
		respondNotFound(w, r, "Notification setting")
		return
	}

	if !isActive {
		respondBadRequest(w, r, "Cannot assign inactive notification setting")
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
			respondConflict(w, r, "Notification setting is already assigned to this configuration set")
		} else {
			respondInternalError(w, r, err)
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
		respondInvalidID(w, r, "config_set_id")
		return
	}

	assignmentID, err := strconv.Atoi(assignmentIDStr)
	if err != nil {
		respondInvalidID(w, r, "assignment_id")
		return
	}

	// Delete the assignment
	result, err := h.db.GetDB().Exec(`
		DELETE FROM configuration_set_notification_settings
		WHERE id = ? AND configuration_set_id = ?
	`, assignmentID, configSetID)
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
		respondNotFound(w, r, "Assignment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAvailableNotificationSettings returns notification settings not yet assigned to a configuration set
func (h *ConfigurationSetNotificationHandler) GetAvailableNotificationSettings(w http.ResponseWriter, r *http.Request) {
	configSetIDStr := r.PathValue("config_set_id")
	configSetID, err := strconv.Atoi(configSetIDStr)
	if err != nil {
		respondInvalidID(w, r, "config_set_id")
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
		respondInternalError(w, r, err)
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
			respondInternalError(w, r, err)
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