CREATE TABLE IF NOT EXISTS object_translations (
	object_type TEXT NOT NULL,
	object_id INTEGER NOT NULL,
	field TEXT NOT NULL,
	locale TEXT NOT NULL,
	source TEXT NOT NULL CHECK (source IN ('system', 'instance')),
	value TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (object_type, object_id, field, locale, source)
);

CREATE INDEX IF NOT EXISTS idx_object_translations_lookup
	ON object_translations(object_type, locale, field, object_id);

CREATE TRIGGER IF NOT EXISTS delete_configuration_set_translations AFTER DELETE ON configuration_sets BEGIN
	DELETE FROM object_translations WHERE object_type = 'configuration_set' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_workflow_translations AFTER DELETE ON workflows BEGIN
	DELETE FROM object_translations WHERE object_type = 'workflow' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_screen_translations AFTER DELETE ON screens BEGIN
	DELETE FROM object_translations WHERE object_type = 'screen' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_notification_setting_translations AFTER DELETE ON notification_settings BEGIN
	DELETE FROM object_translations WHERE object_type = 'notification_setting' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_item_type_translations AFTER DELETE ON item_types BEGIN
	DELETE FROM object_translations WHERE object_type = 'item_type' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_hierarchy_level_translations AFTER DELETE ON hierarchy_levels BEGIN
	DELETE FROM object_translations WHERE object_type = 'hierarchy_level' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_priority_translations AFTER DELETE ON priorities BEGIN
	DELETE FROM object_translations WHERE object_type = 'priority' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_status_category_translations AFTER DELETE ON status_categories BEGIN
	DELETE FROM object_translations WHERE object_type = 'status_category' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_status_translations AFTER DELETE ON statuses BEGIN
	DELETE FROM object_translations WHERE object_type = 'status' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_workspace_role_translations AFTER DELETE ON workspace_roles BEGIN
	DELETE FROM object_translations WHERE object_type = 'workspace_role' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_link_type_translations AFTER DELETE ON link_types BEGIN
	DELETE FROM object_translations WHERE object_type = 'link_type' AND object_id = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS delete_theme_translations AFTER DELETE ON themes BEGIN
	DELETE FROM object_translations WHERE object_type = 'theme' AND object_id = OLD.id;
END;
