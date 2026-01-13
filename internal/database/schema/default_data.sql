-- Default data for initial setup (SQLite)
-- This file contains INSERT statements for default priorities

-- Default priorities
INSERT INTO priorities (name, description, icon, color, sort_order, is_default)
VALUES
    ('Critical', 'Urgent items requiring immediate attention', 'AlertCircle', '#dc2626', 1, 0),
    ('High', 'High priority items', 'ArrowUp', '#ea580c', 2, 0),
    ('Medium', 'Normal priority items', 'Minus', '#ca8a04', 3, 1),
    ('Low', 'Low priority items', 'ArrowDown', '#16a34a', 4, 0);
