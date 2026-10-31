-- Milestone comments: Markdown discussion on a milestone's detail
-- page. Kept apart from the item-only comments table so milestone comments
-- never reach item events, notifications, mentions, or webhooks. Who may
-- comment is decided by MilestoneCommentService.
-- Rows cascade away with their milestone or author.

CREATE TABLE IF NOT EXISTS milestone_comments (
    id SERIAL PRIMARY KEY,
    milestone_id INTEGER NOT NULL,
    author_id INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (milestone_id) REFERENCES milestones(id) ON DELETE CASCADE,
    FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_milestone_comments_milestone ON milestone_comments(milestone_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_milestone_comments_author ON milestone_comments(author_id);
