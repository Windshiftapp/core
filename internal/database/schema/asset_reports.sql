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
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
    FOREIGN KEY (asset_set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE
);

-- Index for efficient querying by channel
CREATE INDEX IF NOT EXISTS idx_asset_reports_channel_id ON asset_reports(channel_id);

-- Index for efficient querying by asset set
CREATE INDEX IF NOT EXISTS idx_asset_reports_asset_set_id ON asset_reports(asset_set_id);
