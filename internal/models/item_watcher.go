package models

import "time"

// ItemWatcher is an internal user with an active watch on a work item. Watchers
// receive notifications for the item and see it in their watched feed.
type ItemWatcher struct {
	UserID      int       `json:"user_id"`
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	Email       string    `json:"email"`
	Username    string    `json:"username"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
	WatchReason *string   `json:"watch_reason,omitempty"`
	WatchedAt   time.Time `json:"watched_at"`
}
