package database

import (
	"database/sql"
	"fmt"
	"strings"
)

// Both engines run one shared fresh-install seed sequence so their default
// rows cannot drift. Queries are written with ? placeholders; the engine's
// Tx converts them where needed. The remaining engine differences —
// generated-id retrieval, conflict-clause syntax, and the default-priorities
// statements — live in the seedWriter implementations below.

// seedWriter is the dialect surface the seed sequence needs on top of Tx.
type seedWriter interface {
	Tx
	// insertReturningID runs an INSERT and returns the generated row id.
	insertReturningID(query string, args ...any) (int64, error)
	// insertIgnoringConflicts rewrites an INSERT so duplicate rows are skipped.
	insertIgnoringConflicts(query string) string
	// defaultPrioritiesSQL returns the engine's default-priorities statements.
	defaultPrioritiesSQL() string
}

type sqliteSeedWriter struct{ Tx }

func (s sqliteSeedWriter) insertReturningID(query string, args ...any) (int64, error) {
	result, err := s.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s sqliteSeedWriter) insertIgnoringConflicts(query string) string {
	return strings.Replace(query, "INSERT INTO", "INSERT OR IGNORE INTO", 1)
}

func (s sqliteSeedWriter) defaultPrioritiesSQL() string { return defaultDataSQL }

type postgresSeedWriter struct{ Tx }

func (p postgresSeedWriter) insertReturningID(query string, args ...any) (int64, error) {
	var id int64
	err := p.QueryRow(query+" RETURNING id", args...).Scan(&id)
	return id, err
}

func (p postgresSeedWriter) insertIgnoringConflicts(query string) string {
	return query + " ON CONFLICT DO NOTHING"
}

func (p postgresSeedWriter) defaultPrioritiesSQL() string { return defaultDataPostgresSQL }

// seedFreshInstall seeds default data for a fresh installation unless a seed
// already ran (status categories exist). The whole sequence is one transaction.
func seedFreshInstall(db *sql.DB, newSeedWriter func(*sql.Tx) seedWriter) error {
	var categoryCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM status_categories").Scan(&categoryCount); err != nil {
		return fmt.Errorf("failed to check existing status categories: %w", err)
	}
	if categoryCount > 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := seedDefaultData(newSeedWriter(tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit default data: %w", err)
	}
	return nil
}

// seedDefaultData inserts the built-in defaults in dependency order: status
// system, workflow, screen, configuration set, link types, system settings,
// item types, notification channel, themes, priorities, and notification
// rules. The step order is contractual — later inserts reference earlier ids.
func seedDefaultData(w seedWriter) error {
	// 1. Default status categories.
	categoryIDs := make(map[string]int64)
	for _, cat := range defaultStatusCategories {
		id, err := w.insertReturningID(
			"INSERT INTO status_categories (builtin_key, name, color, description, is_default, is_completed) VALUES (?, ?, ?, ?, ?, ?)",
			cat.builtinKey, cat.name, cat.color, cat.description, cat.isDefault, cat.isCompleted,
		)
		if err != nil {
			return fmt.Errorf("failed to create status category %s: %w", cat.name, err)
		}
		categoryIDs[cat.name] = id
	}

	// 2. Default statuses.
	statusIDs := make(map[string]int64)
	for _, status := range defaultStatuses {
		id, err := w.insertReturningID(
			"INSERT INTO statuses (builtin_key, name, description, category_id, is_default) VALUES (?, ?, ?, ?, ?)",
			status.builtinKey, status.name, status.description, categoryIDs[status.category], status.isDefault,
		)
		if err != nil {
			return fmt.Errorf("failed to create status %s: %w", status.name, err)
		}
		statusIDs[status.name] = id
	}

	// 3. Default workflow.
	workflowID, err := w.insertReturningID(
		"INSERT INTO workflows (builtin_key, name, description, is_default) VALUES (?, ?, ?, ?)",
		"default", "Default Workflow", "Basic workflow for getting work done", true,
	)
	if err != nil {
		return fmt.Errorf("failed to create default workflow: %w", err)
	}

	// 4. Workflow transitions.
	for i, transition := range defaultTransitions {
		var fromStatusID *int64
		if transition.from != "" {
			id := statusIDs[transition.from]
			fromStatusID = &id
		}
		if _, err := w.Exec(
			"INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order) VALUES (?, ?, ?, ?)",
			workflowID, fromStatusID, statusIDs[transition.to], i,
		); err != nil {
			return fmt.Errorf("failed to create transition from %s to %s: %w", transition.from, transition.to, err)
		}
	}

	// 5. Default screen.
	screenID, err := w.insertReturningID(
		"INSERT INTO screens (builtin_key, name, description) VALUES (?, ?, ?)",
		"default", "Default Screen", "Default screen with essential work item fields",
	)
	if err != nil {
		return fmt.Errorf("failed to create default screen: %w", err)
	}

	// 6. Screen fields.
	for _, field := range defaultScreenFields {
		if _, err := w.Exec(
			"INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width) VALUES (?, ?, ?, ?, ?, ?)",
			screenID, field.fieldType, field.fieldIdentifier, field.displayOrder, field.isRequired, field.fieldWidth,
		); err != nil {
			return fmt.Errorf("failed to add field %s to default screen: %w", field.fieldIdentifier, err)
		}
	}

	// 7. Default configuration set.
	configSetID, err := w.insertReturningID(
		"INSERT INTO configuration_sets (builtin_key, name, description, workflow_id, is_default) VALUES (?, ?, ?, ?, ?)",
		"default", "Default Configuration", "Default configuration set with basic workflow and screen", workflowID, true,
	)
	if err != nil {
		return fmt.Errorf("failed to create default configuration set: %w", err)
	}

	// 8. Default screen for every context.
	for _, context := range defaultScreenContexts {
		if _, err := w.Exec(
			"INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context) VALUES (?, ?, ?)",
			configSetID, screenID, context,
		); err != nil {
			return fmt.Errorf("failed to assign screen to configuration set for %s context: %w", context, err)
		}
	}

	// 9. Default link types.
	for _, linkType := range defaultLinkTypes {
		if _, err := w.Exec(
			"INSERT INTO link_types (builtin_key, name, description, forward_label, reverse_label, color, is_system, allowed_entity_types) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			linkType.builtinKey, linkType.name, linkType.description, linkType.forwardLabel, linkType.reverseLabel, linkType.color, linkType.isSystem, linkType.allowedEntityTypes,
		); err != nil {
			return fmt.Errorf("failed to create link type %s: %w", linkType.name, err)
		}
	}

	// 10. System settings.
	for _, setting := range defaultSystemSettings {
		if _, err := w.Exec(
			"INSERT INTO system_settings (key, value, value_type, description, category) VALUES (?, ?, ?, ?, ?)",
			setting.key, setting.value, setting.valueType, setting.description, setting.category,
		); err != nil {
			return fmt.Errorf("failed to create system setting %s: %w", setting.key, err)
		}
	}

	// 11. Hierarchy levels.
	for _, hl := range defaultHierarchyLevels {
		if _, err := w.Exec(
			"INSERT INTO hierarchy_levels (builtin_key, level, name, description) VALUES (?, ?, ?, ?)",
			hl.builtinKey, hl.level, hl.name, hl.description,
		); err != nil {
			return fmt.Errorf("failed to create hierarchy level %s: %w", hl.name, err)
		}
	}

	// 12. Item types.
	for _, itemType := range defaultItemTypes {
		if _, err := w.Exec(
			"INSERT INTO item_types (builtin_key, configuration_set_id, name, description, icon, color, hierarchy_level, sort_order, is_default) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			itemType.builtinKey, configSetID, itemType.name, itemType.description, itemType.icon, itemType.color, itemType.hierarchyLevel, itemType.sortOrder, true,
		); err != nil {
			return fmt.Errorf("failed to create default item type %s: %w", itemType.name, err)
		}
	}

	// 13. Bind the selected item types to the default configuration set
	// (excluding Initiative for the simplified setup).
	for _, typeName := range defaultItemTypeBindings {
		var itemTypeID int64
		if err := w.QueryRow("SELECT id FROM item_types WHERE name = ?", typeName).Scan(&itemTypeID); err != nil {
			return fmt.Errorf("failed to get item type ID for %s: %w", typeName, err)
		}
		if _, err := w.Exec(
			"INSERT INTO configuration_set_item_types (configuration_set_id, item_type_id) VALUES (?, ?)",
			configSetID, itemTypeID,
		); err != nil {
			return fmt.Errorf("failed to bind item type %s to default configuration set: %w", typeName, err)
		}
	}

	// 14. Default notification mail channel. (Built-in email templates are
	// seeded separately by emailutil.SeedTemplates during server bootstrap —
	// keeps the database layer free of email-domain imports.)
	if _, err := w.Exec(
		"INSERT INTO channels (name, type, direction, description, status, is_default, config) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"Notification Mail", "smtp", "outbound", "Default SMTP channel for sending notification emails", "pending", true, defaultNotificationChannelConfig,
	); err != nil {
		return fmt.Errorf("failed to create default notification mail channel: %w", err)
	}

	// 15. Default themes with dual light/dark nav colors.
	for _, theme := range defaultThemes {
		if _, err := w.Exec(
			"INSERT INTO themes (builtin_key, name, description, is_default, is_active, nav_background_color_light, nav_text_color_light, nav_background_color_dark, nav_text_color_dark) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			theme.builtinKey, theme.name, theme.description, theme.isDefault, theme.isActive, theme.navBackgroundColorLight, theme.navTextColorLight, theme.navBackgroundColorDark, theme.navTextColorDark,
		); err != nil {
			return fmt.Errorf("failed to create theme %s: %w", theme.name, err)
		}
	}

	// 16. Default priorities, if the schema shipped none.
	var priorityCount int
	if err := w.QueryRow("SELECT COUNT(*) FROM priorities").Scan(&priorityCount); err != nil {
		return fmt.Errorf("failed to check existing priorities: %w", err)
	}
	if priorityCount == 0 {
		if _, err := w.Exec(w.defaultPrioritiesSQL()); err != nil {
			return fmt.Errorf("failed to create default priorities: %w", err)
		}
	}

	// 17. Link every priority to the default configuration set. Priority ids
	// are collected first: a transaction cannot run a new statement while
	// rows from the same transaction are still open (PostgreSQL).
	priorityRows, err := w.Query("SELECT id FROM priorities")
	if err != nil {
		return fmt.Errorf("failed to query priorities: %w", err)
	}
	var priorityIDs []int64
	for priorityRows.Next() {
		var priorityID int64
		if err := priorityRows.Scan(&priorityID); err != nil {
			_ = priorityRows.Close()
			return fmt.Errorf("failed to scan priority: %w", err)
		}
		priorityIDs = append(priorityIDs, priorityID)
	}
	if err := priorityRows.Err(); err != nil {
		_ = priorityRows.Close()
		return fmt.Errorf("failed to iterate priorities: %w", err)
	}
	if err := priorityRows.Close(); err != nil {
		return fmt.Errorf("failed to close priority rows: %w", err)
	}

	for _, priorityID := range priorityIDs {
		if _, err := w.Exec(
			w.insertIgnoringConflicts("INSERT INTO configuration_set_priorities (configuration_set_id, priority_id) VALUES (?, ?)"),
			configSetID, priorityID,
		); err != nil {
			return fmt.Errorf("failed to link priority to default config set: %w", err)
		}
	}

	// 18. Default notification setting and event rules.
	// mention.created has no rule by design: mentions always notify the
	// mentioned user (subject to workspace visibility), enforced in
	// mention_service.go outside the configurable rules system.
	notificationSettingID, err := w.insertReturningID(
		"INSERT INTO notification_settings (builtin_key, name, description, is_active, created_by) VALUES (?, ?, ?, ?, ?)",
		"default", "Default Notifications", "Standard notification rules for work item updates", true, nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create default notification setting: %w", err)
	}

	for _, rule := range defaultNotificationEventRules {
		if _, err := w.Exec(
			`INSERT INTO notification_event_rules
			 (notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator,
			  notify_watchers, notify_workspace_admins)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			notificationSettingID, rule.eventType, true, rule.notifyAssignee,
			rule.notifyCreator, rule.notifyWatchers, rule.notifyWorkspaceAdmins,
		); err != nil {
			return fmt.Errorf("failed to create notification rule for %s: %w", rule.eventType, err)
		}
	}

	// 19. Link the notification setting to the default configuration set.
	if _, err := w.Exec(
		"INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id) VALUES (?, ?)",
		configSetID, notificationSettingID,
	); err != nil {
		return fmt.Errorf("failed to link notification setting to configuration set: %w", err)
	}

	return nil
}
