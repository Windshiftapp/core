package models

import "time"

// Milestone Activity entry types. Milestone events describe the
// milestone itself; item events describe its member items.
const (
	MilestoneActivityMilestoneCommentAdded     = "milestone_comment_added"
	MilestoneActivityMilestoneDescriptionEdit  = "milestone_description_changed"
	MilestoneActivityMilestoneStatusChanged    = "milestone_status_changed"
	MilestoneActivityMilestoneTargetDateChange = "milestone_target_date_changed"
	MilestoneActivityMilestonePageLinked       = "milestone_page_linked"
	MilestoneActivityMilestonePageUnlinked     = "milestone_page_unlinked"
	MilestoneActivityItemCommentAdded          = "item_comment_added"
	MilestoneActivityItemStatusChanged         = "item_status_changed"
	MilestoneActivityItemAdded                 = "item_added"
	MilestoneActivityItemRemoved               = "item_removed"
)

// Milestone Activity actor kinds.
const (
	MilestoneActivityActorUser           = "user"
	MilestoneActivityActorPortalCustomer = "portal_customer"
	MilestoneActivityActorSystem         = "system"
)

// MilestoneActivity is one entry in a milestone's Activity feed:
// who did what, when, and to which item or page. OldValue and NewValue carry
// the change for status and target-date entries (item statuses resolved to
// names); description edits and comments carry no values.
type MilestoneActivity struct {
	ID         string                 `json:"id"` // stable key: "<source>:<row id>"
	Type       string                 `json:"type"`
	OccurredAt time.Time              `json:"occurred_at"`
	ActorKind  string                 `json:"actor_kind"`
	ActorID    *int                   `json:"actor_id,omitempty"` // user ID when actor_kind is "user"
	ActorName  string                 `json:"actor_name,omitempty"`
	Item       *MilestoneActivityItem `json:"item,omitempty"`
	Page       *MilestoneActivityPage `json:"page,omitempty"`
	CommentID  *int                   `json:"comment_id,omitempty"`
	OldValue   *string                `json:"old_value,omitempty"`
	NewValue   *string                `json:"new_value,omitempty"`
}

// MilestoneActivityItem references the work item an Activity entry is about.
type MilestoneActivityItem struct {
	ID          int    `json:"id"`
	Key         string `json:"key"`
	Title       string `json:"title"`
	WorkspaceID int    `json:"workspace_id"`
}

// MilestoneActivityPage references the page a "page linked" or "page
// unlinked" entry is about.
type MilestoneActivityPage struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	WorkspaceID int    `json:"workspace_id"`
}
