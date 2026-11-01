-- Global item-label catalog

CREATE TABLE IF NOT EXISTS labels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    color TEXT DEFAULT '#3B82F6',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_labels_name_ci ON labels(LOWER(name));

-- Junction table for item-label many-to-many relationship

CREATE TABLE IF NOT EXISTS item_labels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id INTEGER NOT NULL,
    label_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
    FOREIGN KEY (label_id) REFERENCES labels(id) ON DELETE CASCADE,
    UNIQUE(item_id, label_id)
);

CREATE INDEX IF NOT EXISTS idx_item_labels_item_id ON item_labels(item_id);
CREATE INDEX IF NOT EXISTS idx_item_labels_label_id ON item_labels(label_id);
