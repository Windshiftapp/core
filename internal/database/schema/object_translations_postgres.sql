CREATE TABLE IF NOT EXISTS object_translations (
	object_type TEXT NOT NULL,
	object_id INTEGER NOT NULL,
	field TEXT NOT NULL,
	locale TEXT NOT NULL,
	source TEXT NOT NULL CHECK (source IN ('system', 'instance')),
	value TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (object_type, object_id, field, locale, source)
);

CREATE INDEX IF NOT EXISTS idx_object_translations_lookup
	ON object_translations(object_type, locale, field, object_id);

CREATE OR REPLACE FUNCTION delete_object_translations() RETURNS trigger AS $$
BEGIN
	DELETE FROM object_translations WHERE object_type = TG_ARGV[0] AND object_id = OLD.id;
	RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS delete_configuration_set_translations ON configuration_sets;
DROP TRIGGER IF EXISTS delete_workflow_translations ON workflows;
DROP TRIGGER IF EXISTS delete_screen_translations ON screens;
DROP TRIGGER IF EXISTS delete_notification_setting_translations ON notification_settings;
DROP TRIGGER IF EXISTS delete_item_type_translations ON item_types;
DROP TRIGGER IF EXISTS delete_hierarchy_level_translations ON hierarchy_levels;
DROP TRIGGER IF EXISTS delete_priority_translations ON priorities;
DROP TRIGGER IF EXISTS delete_status_category_translations ON status_categories;
DROP TRIGGER IF EXISTS delete_status_translations ON statuses;
DROP TRIGGER IF EXISTS delete_workspace_role_translations ON workspace_roles;
DROP TRIGGER IF EXISTS delete_link_type_translations ON link_types;
DROP TRIGGER IF EXISTS delete_theme_translations ON themes;

CREATE TRIGGER delete_configuration_set_translations AFTER DELETE ON configuration_sets FOR EACH ROW EXECUTE FUNCTION delete_object_translations('configuration_set');
CREATE TRIGGER delete_workflow_translations AFTER DELETE ON workflows FOR EACH ROW EXECUTE FUNCTION delete_object_translations('workflow');
CREATE TRIGGER delete_screen_translations AFTER DELETE ON screens FOR EACH ROW EXECUTE FUNCTION delete_object_translations('screen');
CREATE TRIGGER delete_notification_setting_translations AFTER DELETE ON notification_settings FOR EACH ROW EXECUTE FUNCTION delete_object_translations('notification_setting');
CREATE TRIGGER delete_item_type_translations AFTER DELETE ON item_types FOR EACH ROW EXECUTE FUNCTION delete_object_translations('item_type');
CREATE TRIGGER delete_hierarchy_level_translations AFTER DELETE ON hierarchy_levels FOR EACH ROW EXECUTE FUNCTION delete_object_translations('hierarchy_level');
CREATE TRIGGER delete_priority_translations AFTER DELETE ON priorities FOR EACH ROW EXECUTE FUNCTION delete_object_translations('priority');
CREATE TRIGGER delete_status_category_translations AFTER DELETE ON status_categories FOR EACH ROW EXECUTE FUNCTION delete_object_translations('status_category');
CREATE TRIGGER delete_status_translations AFTER DELETE ON statuses FOR EACH ROW EXECUTE FUNCTION delete_object_translations('status');
CREATE TRIGGER delete_workspace_role_translations AFTER DELETE ON workspace_roles FOR EACH ROW EXECUTE FUNCTION delete_object_translations('workspace_role');
CREATE TRIGGER delete_link_type_translations AFTER DELETE ON link_types FOR EACH ROW EXECUTE FUNCTION delete_object_translations('link_type');
CREATE TRIGGER delete_theme_translations AFTER DELETE ON themes FOR EACH ROW EXECUTE FUNCTION delete_object_translations('theme');
