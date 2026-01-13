-- Default data for initial setup (PostgreSQL)
-- This file contains INSERT statements for default priorities

-- Default priorities
INSERT INTO priorities (name, description, icon, color, sort_order, is_default)
VALUES
    ('Critical', 'Urgent items requiring immediate attention', 'AlertCircle', '#dc2626', 1, false),
    ('High', 'High priority items', 'ArrowUp', '#ea580c', 2, false),
    ('Medium', 'Normal priority items', 'Minus', '#ca8a04', 3, true),
    ('Low', 'Low priority items', 'ArrowDown', '#16a34a', 4, false);
