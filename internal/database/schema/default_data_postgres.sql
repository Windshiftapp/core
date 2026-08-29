-- Default data for initial setup (PostgreSQL)
-- This file contains INSERT statements for default priorities

-- Default priorities
INSERT INTO priorities (builtin_key, name, description, icon, color, sort_order, is_default)
VALUES
    ('critical', 'Critical', 'Urgent items requiring immediate attention', 'AlertCircle', '#dc2626', 1, false),
    ('high', 'High', 'High priority items', 'ArrowUp', '#ea580c', 2, false),
    ('medium', 'Medium', 'Normal priority items', 'Minus', '#ca8a04', 3, true),
    ('low', 'Low', 'Low priority items', 'ArrowDown', '#16a34a', 4, false);
