package models

import "time"

// SchedulerRun records one tick of an in-process scheduler — start/end, what it
// processed, and whether it succeeded. Surfaced on the admin Diagnostics page.
type SchedulerRun struct {
	ID             int        `json:"id"`
	SchedulerName  string     `json:"scheduler_name"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	DurationMs     *int       `json:"duration_ms,omitempty"`
	ItemsProcessed *int       `json:"items_processed,omitempty"`
	Success        bool       `json:"success"`
	ErrorMessage   string     `json:"error_message,omitempty"`
}
