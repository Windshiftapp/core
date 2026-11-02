-- Milestone history: one row per change to a milestone's
-- description, status, or target date, with the acting user and the old and
-- new values. The milestone Activity tab reads it. Rows cascade away with
-- their milestone; like item_history, they keep their user (users are
-- offboarded, not deleted). A row without a user is a system change.

CREATE TABLE IF NOT EXISTS milestone_history (
    id SERIAL PRIMARY KEY,
    milestone_id INTEGER NOT NULL,
    user_id INTEGER,
    field_name TEXT NOT NULL,
    old_value TEXT,
    new_value TEXT,
    changed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (milestone_id) REFERENCES milestones(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_milestone_history_milestone ON milestone_history(milestone_id, changed_at, id);
CREATE INDEX IF NOT EXISTS idx_milestone_history_user ON milestone_history(user_id);
