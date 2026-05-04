package models

import "time"

// WebhookDelivery is one outbound webhook send attempt — recorded for both
// successes and failures, both HTTP and plugin transports. Surfaced on the
// admin Diagnostics page.
type WebhookDelivery struct {
	ID                 int       `json:"id"`
	ChannelID          int       `json:"channel_id"`
	ItemID             *int      `json:"item_id,omitempty"`
	EventType          string    `json:"event_type"`
	AttemptType        string    `json:"attempt_type"` // "automatic", "manual"
	Transport          string    `json:"transport"`    // "http" or "plugin"
	RequestURL         string    `json:"request_url,omitempty"`
	RequestedAt        time.Time `json:"requested_at"`
	ResponseStatusCode *int      `json:"response_status_code,omitempty"`
	ResponseTimeMs     *int      `json:"response_time_ms,omitempty"`
	Success            bool      `json:"success"`
	ErrorMessage       string    `json:"error_message,omitempty"`

	// Joined fields for API responses
	ChannelName string `json:"channel_name,omitempty"`
}
