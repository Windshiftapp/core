package repository

import (
	"database/sql"
	"errors"

	"windshift/internal/database"
	"windshift/internal/models"
)

// NotificationSettingsRepository owns the SQL for the notification_settings
// and notification_event_rules tables.
type NotificationSettingsRepository struct {
	db database.Database
}

// NewNotificationSettingsRepository creates a new repository.
func NewNotificationSettingsRepository(db database.Database) *NotificationSettingsRepository {
	return &NotificationSettingsRepository{db: db}
}

// ListAll returns every notification setting with its event rules loaded.
func (r *NotificationSettingsRepository) ListAll() ([]models.NotificationSetting, error) {
	rows, err := r.db.Query(`
		SELECT
			ns.id, ns.name, ns.description, ns.is_active, ns.created_by, ns.created_at, ns.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name
		FROM notification_settings ns
		LEFT JOIN users u ON ns.created_by = u.id
		ORDER BY ns.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var settings []models.NotificationSetting
	for rows.Next() {
		s, err := scanNotificationSettingRow(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.attachEventRules(settings); err != nil {
		return nil, err
	}
	return settings, nil
}

// FindByID returns a single notification setting with its event rules loaded.
// Returns ErrNotFound if no row matches.
func (r *NotificationSettingsRepository) FindByID(id int) (*models.NotificationSetting, error) {
	row := r.db.QueryRow(`
		SELECT
			ns.id, ns.name, ns.description, ns.is_active, ns.created_by, ns.created_at, ns.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name
		FROM notification_settings ns
		LEFT JOIN users u ON ns.created_by = u.id
		WHERE ns.id = ?
	`, id)
	s, err := scanNotificationSettingRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	settings := []models.NotificationSetting{s}
	if err := r.attachEventRules(settings); err != nil {
		return nil, err
	}
	return &settings[0], nil
}

// CreateWithRules inserts a setting plus its event rules in a single transaction.
// Returns the new setting ID.
func (r *NotificationSettingsRepository) CreateWithRules(setting *models.NotificationSetting) (int, error) {
	var id int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		var insertedID int64
		if err := tx.QueryRow(`
			INSERT INTO notification_settings (name, description, is_active, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
		`, setting.Name, setting.Description, setting.IsActive, setting.CreatedBy).Scan(&insertedID); err != nil {
			return err
		}
		id = int(insertedID)
		return insertNotificationEventRules(tx, id, setting.EventRules)
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateWithRules updates the setting and replaces its event rules atomically.
// Returns ErrNotFound if no row matches the id.
func (r *NotificationSettingsRepository) UpdateWithRules(id int, setting *models.NotificationSetting) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		result, err := tx.Exec(`
			UPDATE notification_settings
			SET name = ?, description = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, setting.Name, setting.Description, setting.IsActive, id)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(`DELETE FROM notification_event_rules WHERE notification_setting_id = ?`, id); err != nil {
			return err
		}
		return insertNotificationEventRules(tx, id, setting.EventRules)
	})
}

// CountConfigurationSetAssignments counts how many configuration sets the
// given notification setting is currently assigned to.
func (r *NotificationSettingsRepository) CountConfigurationSetAssignments(settingID int) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM configuration_set_notification_settings
		WHERE notification_setting_id = ?
	`, settingID).Scan(&count)
	return count, err
}

// EnsureDefault creates a "Default Notifications" setting (with the standard
// item.assigned / comment.created / status.changed event rules) and links it
// to the default configuration set, if no notification settings exist yet.
// Idempotent: a no-op when any setting already exists or when no default
// configuration set is present.
func (r *NotificationSettingsRepository) EnsureDefault() error {
	var settingCount int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM notification_settings").Scan(&settingCount); err != nil {
		return err
	}
	if settingCount > 0 {
		return nil
	}

	var configSetID int
	err := r.db.QueryRow("SELECT id FROM configuration_sets WHERE is_default = true LIMIT 1").Scan(&configSetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	return database.WithTx(r.db, func(tx database.Tx) error {
		var notificationSettingID int64
		if err := tx.QueryRow(
			`INSERT INTO notification_settings (name, description, is_active, created_by)
			 VALUES (?, ?, ?, ?) RETURNING id`,
			"Default Notifications", "Standard notification rules for work item updates", true, nil,
		).Scan(&notificationSettingID); err != nil {
			return err
		}

		eventRules := []struct {
			eventType      string
			notifyAssignee bool
			notifyCreator  bool
		}{
			{"item.assigned", true, false},
			{"comment.created", true, true},
			{"status.changed", true, true},
		}
		for _, rule := range eventRules {
			if _, err := tx.Exec(
				`INSERT INTO notification_event_rules
				 (notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator,
				  notify_watchers, notify_workspace_admins)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				notificationSettingID, rule.eventType, true, rule.notifyAssignee, rule.notifyCreator, false, false,
			); err != nil {
				return err
			}
		}

		_, err := tx.Exec(
			`INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id)
			 VALUES (?, ?)`,
			configSetID, notificationSettingID,
		)
		return err
	})
}

// Delete removes a notification setting. Returns ErrNotFound if no row is
// affected. Event rules are cascade-deleted by the database.
func (r *NotificationSettingsRepository) Delete(id int) error {
	result, err := r.db.Exec(`DELETE FROM notification_settings WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func scanNotificationSettingRow(s rowScanner) (models.NotificationSetting, error) {
	var setting models.NotificationSetting
	var createdBy sql.NullInt64
	var createdByName sql.NullString
	if err := s.Scan(
		&setting.ID, &setting.Name, &setting.Description, &setting.IsActive,
		&createdBy, &setting.CreatedAt, &setting.UpdatedAt, &createdByName,
	); err != nil {
		return models.NotificationSetting{}, err
	}
	if createdBy.Valid {
		setting.CreatedBy = int(createdBy.Int64)
	}
	if createdByName.Valid {
		setting.CreatedByName = createdByName.String
	}
	return setting, nil
}

func (r *NotificationSettingsRepository) attachEventRules(settings []models.NotificationSetting) error {
	for i := range settings {
		rules, err := r.eventRulesFor(settings[i].ID)
		if err != nil {
			return err
		}
		settings[i].EventRules = rules
	}
	return nil
}

func (r *NotificationSettingsRepository) eventRulesFor(settingID int) ([]models.NotificationEventRule, error) {
	rows, err := r.db.Query(`
		SELECT id, notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator,
		       notify_watchers, notify_workspace_admins, custom_recipients, message_template,
		       created_at, updated_at
		FROM notification_event_rules
		WHERE notification_setting_id = ?
		ORDER BY event_type
	`, settingID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var rules []models.NotificationEventRule
	for rows.Next() {
		var rule models.NotificationEventRule
		var customRecipients, messageTemplate *string
		if err := rows.Scan(
			&rule.ID, &rule.NotificationSettingID, &rule.EventType, &rule.IsEnabled,
			&rule.NotifyAssignee, &rule.NotifyCreator, &rule.NotifyWatchers, &rule.NotifyWorkspaceAdmins,
			&customRecipients, &messageTemplate, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if customRecipients != nil {
			rule.CustomRecipients = *customRecipients
		}
		if messageTemplate != nil {
			rule.MessageTemplate = *messageTemplate
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func insertNotificationEventRules(tx database.Tx, settingID int, rules []models.NotificationEventRule) error {
	for _, rule := range rules {
		if _, err := tx.Exec(`
			INSERT INTO notification_event_rules
			(notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator,
			 notify_watchers, notify_workspace_admins, custom_recipients, message_template,
			 created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, settingID, rule.EventType, rule.IsEnabled, rule.NotifyAssignee, rule.NotifyCreator,
			rule.NotifyWatchers, rule.NotifyWorkspaceAdmins, rule.CustomRecipients, rule.MessageTemplate); err != nil {
			return err
		}
	}
	return nil
}
