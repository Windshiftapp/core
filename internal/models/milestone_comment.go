package models

import "time"

// MilestoneComment is one Markdown comment on a milestone. It is
// separate from the item-only Comment so milestone discussion stays out of
// item events, notifications, mentions, and webhooks.
type MilestoneComment struct {
	ID          int       `json:"id"`
	MilestoneID int       `json:"milestone_id"`
	AuthorID    int       `json:"author_id"`
	Content     string    `json:"content"` // Sanitized Markdown source
	ContentHTML string    `json:"content_html,omitempty" db:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	AuthorName   string `json:"author_name,omitempty"`
	AuthorAvatar string `json:"author_avatar,omitempty"`
	IsAgent      bool   `json:"is_agent,omitempty"`
}
