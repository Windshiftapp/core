package models

import "time"

// MilestonePageLink is a link from a workspace (local) milestone to a page in
// the same workspace. It is stored as an item_links row with
// source_type "milestone", target_type "page", and the built-in Page link
// type; ID is that row's ID.
type MilestonePageLink struct {
	ID          int       `json:"id"`
	MilestoneID int       `json:"milestone_id"`
	PageID      int       `json:"page_id"`
	PageTitle   string    `json:"page_title"`
	WorkspaceID int       `json:"workspace_id"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	// Joined for API responses
	CreatedByName string `json:"created_by_name,omitempty"`
}

// MilestonePageLinkEvent is one recorded page link or unlink on a milestone,
// read from its milestone_history page_link row. ID is that row's
// ID. PageTitle and WorkspaceID are the page's current values; events for
// deleted pages are not returned. UserID nil means no recorded actor.
type MilestonePageLinkEvent struct {
	ID          int
	MilestoneID int
	PageID      int
	PageTitle   string
	WorkspaceID int
	Unlinked    bool
	UserID      *int
	UserName    string
	OccurredAt  time.Time
}
