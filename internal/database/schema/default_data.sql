-- Default data for initial setup (SQLite)
-- This file contains INSERT statements for default priorities

-- Default priorities
INSERT INTO priorities (builtin_key, name, description, icon, color, sort_order, is_default)
VALUES
    ('critical', 'Critical', 'Urgent items requiring immediate attention', 'AlertCircle', '#dc2626', 1, FALSE),
    ('high', 'High', 'High priority items', 'ArrowUp', '#ea580c', 2, FALSE),
    ('medium', 'Medium', 'Normal priority items', 'Minus', '#ca8a04', 3, TRUE),
    ('low', 'Low', 'Low priority items', 'ArrowDown', '#16a34a', 4, FALSE);

-- migration: 0000_baseline
