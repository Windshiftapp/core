-- Asset Reports table for portal asset reports
CREATE TABLE IF NOT EXISTS asset_reports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL,
    asset_set_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    cql_query TEXT DEFAULT '',
    icon TEXT DEFAULT 'Table2',
    color TEXT DEFAULT '#6b7280',
    display_order INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT 1,
    column_config TEXT DEFAULT NULL,  -- JSON array: ["title", "status", "cf_serial"]
    visibility_group_ids TEXT DEFAULT NULL,
    visibility_org_ids TEXT DEFAULT NULL,
    run_mode TEXT NOT NULL DEFAULT 'direct',  -- 'direct' (inline table) or 'form' (launch-as-request)
    item_type_id INTEGER DEFAULT NULL,  -- Used in form mode to resolve available custom fields
    workspace_id INTEGER DEFAULT NULL,  -- Used in form mode to resolve custom fields
    config TEXT DEFAULT NULL,  -- JSON config (submit button label, success message, etc.)
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
    FOREIGN KEY (asset_set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE,
    FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE SET NULL,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE SET NULL
);

-- Index for efficient querying by channel
CREATE INDEX IF NOT EXISTS idx_asset_reports_channel_id ON asset_reports(channel_id);

-- Index for efficient querying by asset set
CREATE INDEX IF NOT EXISTS idx_asset_reports_asset_set_id ON asset_reports(asset_set_id);

-- Asset Report Fields - parallels request_type_fields, used only for run_mode = 'form'
CREATE TABLE IF NOT EXISTS asset_report_fields (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    asset_report_id INTEGER NOT NULL,
    field_identifier TEXT NOT NULL,
    field_type TEXT NOT NULL,  -- 'default', 'custom', or 'virtual'
    is_required BOOLEAN DEFAULT false,
    display_order INTEGER DEFAULT 0,
    options TEXT,
    display_name TEXT,
    description TEXT,
    step_number INTEGER DEFAULT 1,
    virtual_field_type TEXT,
    virtual_field_options TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (asset_report_id) REFERENCES asset_reports(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_asset_report_fields_asset_report_id ON asset_report_fields(asset_report_id);
