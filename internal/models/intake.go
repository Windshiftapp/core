package models

import "time"

// Intake statuses. An intake is processed only while enabled; needs_attention
// marks one that a configuration change invalidated (for example the portal
// stopped serving its workspace) but that keeps its routing config for repair.
const (
	IntakeStatusEnabled        = "enabled"
	IntakeStatusDisabled       = "disabled"
	IntakeStatusNeedsAttention = "needs_attention"
)

// Intake is the routing half of an email mailbox (WI-1644). The mailbox (a
// type='email' channel) owns the connection, credentials, and monitored
// address; an intake owns one folder, the workspace and item type its mail
// becomes, and an optional portal that exposes the requests to customers.
type Intake struct {
	ID        int    `json:"id"`
	MailboxID int    `json:"mailbox_id"`
	Folder    string `json:"folder"`
	// WorkspaceID is the routing target. When PortalChannelID is set it must be
	// a workspace the portal serves.
	WorkspaceID int `json:"workspace_id"`
	// PortalChannelID is the portal whose customers see requests this intake
	// creates. NULL means an internal workspace feed.
	PortalChannelID *int `json:"portal_channel_id,omitempty"`
	// RequestTypeID is reserved for a cached pointer to the portal's system
	// Email request type; the processor resolves it lazily from the portal.
	RequestTypeID *int `json:"-"`
	// ItemTypeID is the item type for created items (workspace-validated).
	ItemTypeID            *int   `json:"item_type_id,omitempty"`
	RateLimitPerHour      *int   `json:"rate_limit_per_hour,omitempty"`
	ProcessingDisposition string `json:"processing_disposition,omitempty"`
	Status                string `json:"status"`
	// StatusReason explains a needs_attention status for the admin UI.
	StatusReason string    `json:"status_reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Joined fields for API responses.
	MailboxName    string `json:"mailbox_name,omitempty"`
	MailboxAddress string `json:"mailbox_address,omitempty"`
	// Per-folder health. LastUID/UIDValidity come from email_intake_state;
	// LastPolledAt is that row's updated_at. RateLimitedCount counts tracking
	// rows this intake declined and an operator can requeue.
	LastUID          int        `json:"last_uid,omitempty"`
	UIDValidity      uint32     `json:"uid_validity,omitempty"`
	LastPolledAt     *time.Time `json:"last_polled_at,omitempty"`
	RateLimitedCount int        `json:"rate_limited_count,omitempty"`
}
