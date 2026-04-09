package models

import "time"

// OnCallSchedule represents a team's on-call schedule
type OnCallSchedule struct {
	ID          int       `json:"id"`
	TeamID      int       `json:"team_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Timezone    string    `json:"timezone"`
	IsActive    bool      `json:"is_active"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined/computed fields
	CreatedByName string                `json:"created_by_name,omitempty"`
	TeamName      string                `json:"team_name,omitempty"`
	Layers        []OnCallScheduleLayer `json:"layers,omitempty"`
}

// OnCallScheduleRequest represents the API request for creating/updating a schedule
type OnCallScheduleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Timezone    string `json:"timezone"`
	IsActive    *bool  `json:"is_active,omitempty"`
}

// OnCallScheduleLayer represents a rotation layer within a schedule
type OnCallScheduleLayer struct {
	ID                   int       `json:"id"`
	ScheduleID           int       `json:"schedule_id"`
	Name                 string    `json:"name"`
	Priority             int       `json:"priority"`      // 1 = primary
	RotationType         string    `json:"rotation_type"` // daily, weekly, custom
	RotationIntervalDays int       `json:"rotation_interval_days"`
	HandoffTime          string    `json:"handoff_time"` // HH:MM
	StartDate            string    `json:"start_date"`
	EndDate              *string   `json:"end_date,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	// Computed
	Members []OnCallScheduleLayerMember `json:"members,omitempty"`
}

// OnCallScheduleLayerRequest represents the API request for creating/updating a layer
type OnCallScheduleLayerRequest struct {
	Name                 string  `json:"name"`
	Priority             int     `json:"priority"`
	RotationType         string  `json:"rotation_type"`
	RotationIntervalDays int     `json:"rotation_interval_days"`
	HandoffTime          string  `json:"handoff_time"`
	StartDate            string  `json:"start_date"`
	EndDate              *string `json:"end_date,omitempty"`
}

// OnCallScheduleLayerMember represents a member in a rotation layer
type OnCallScheduleLayerMember struct {
	ID        int       `json:"id"`
	LayerID   int       `json:"layer_id"`
	UserID    int       `json:"user_id"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	// Joined
	UserName      string `json:"user_name,omitempty"`
	UserEmail     string `json:"user_email,omitempty"`
	UserAvatarURL string `json:"user_avatar_url,omitempty"`
}

// SetLayerMembersRequest represents the API request for setting layer members
type SetLayerMembersRequest struct {
	UserIDs []int `json:"user_ids"` // Ordered list
}

// OnCallScheduleOverride represents a manual override for a schedule
type OnCallScheduleOverride struct {
	ID             int       `json:"id"`
	ScheduleID     int       `json:"schedule_id"`
	UserID         int       `json:"user_id"`          // Person being replaced
	OverrideUserID int       `json:"override_user_id"` // Person taking over
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Reason         string    `json:"reason"`
	CreatedBy      *int      `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	// Joined
	UserName         string `json:"user_name,omitempty"`
	OverrideUserName string `json:"override_user_name,omitempty"`
	CreatedByName    string `json:"created_by_name,omitempty"`
}

// OnCallOverrideRequest represents the API request for creating an override
type OnCallOverrideRequest struct {
	UserID         int    `json:"user_id"`
	OverrideUserID int    `json:"override_user_id"`
	StartTime      string `json:"start_time"` // ISO 8601
	EndTime        string `json:"end_time"`   // ISO 8601
	Reason         string `json:"reason"`
}

// OnCallEscalationPolicy represents an escalation policy for a team
type OnCallEscalationPolicy struct {
	ID          int       `json:"id"`
	TeamID      int       `json:"team_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	RepeatCount int       `json:"repeat_count"`
	IsActive    bool      `json:"is_active"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined/computed
	CreatedByName string                 `json:"created_by_name,omitempty"`
	TeamName      string                 `json:"team_name,omitempty"`
	Rules         []OnCallEscalationRule `json:"rules,omitempty"`
}

// OnCallEscalationPolicyRequest represents the API request for creating/updating an escalation policy
type OnCallEscalationPolicyRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	RepeatCount int    `json:"repeat_count"`
	IsActive    *bool  `json:"is_active,omitempty"`
}

// OnCallEscalationRule represents a step in an escalation chain
type OnCallEscalationRule struct {
	ID                     int       `json:"id"`
	PolicyID               int       `json:"policy_id"`
	StepOrder              int       `json:"step_order"`
	EscalationDelayMinutes int       `json:"escalation_delay_minutes"`
	TargetType             string    `json:"target_type"` // on_call_schedule, user, team
	TargetID               int       `json:"target_id"`
	CreatedAt              time.Time `json:"created_at"`
	// Joined
	TargetName        string                   `json:"target_name,omitempty"`
	NotificationRules []OnCallNotificationRule `json:"notification_rules,omitempty"`
}

// SetEscalationRulesRequest represents the API request for setting escalation rules
type SetEscalationRulesRequest struct {
	Rules []EscalationRuleInput `json:"rules"`
}

// EscalationRuleInput represents a single rule in the set request
type EscalationRuleInput struct {
	StepOrder              int                     `json:"step_order"`
	EscalationDelayMinutes int                     `json:"escalation_delay_minutes"`
	TargetType             string                  `json:"target_type"`
	TargetID               int                     `json:"target_id"`
	NotificationRules      []NotificationRuleInput `json:"notification_rules,omitempty"`
}

// OnCallNotificationRule represents how to notify at each escalation step
type OnCallNotificationRule struct {
	ID                    int       `json:"id"`
	EscalationRuleID      int       `json:"escalation_rule_id"`
	NotificationType      string    `json:"notification_type"` // email, webhook, in_app
	DelayMinutes          int       `json:"delay_minutes"`
	RepeatIntervalMinutes *int      `json:"repeat_interval_minutes,omitempty"`
	RepeatCount           int       `json:"repeat_count"`
	CreatedAt             time.Time `json:"created_at"`
}

// NotificationRuleInput represents a notification rule in the set request
type NotificationRuleInput struct {
	NotificationType      string `json:"notification_type"`
	DelayMinutes          int    `json:"delay_minutes"`
	RepeatIntervalMinutes *int   `json:"repeat_interval_minutes,omitempty"`
	RepeatCount           int    `json:"repeat_count"`
}

// OnCallSwapRequest represents a shift swap request between members
type OnCallSwapRequest struct {
	ID              int        `json:"id"`
	ScheduleID      int        `json:"schedule_id"`
	RequesterUserID int        `json:"requester_user_id"`
	TargetUserID    int        `json:"target_user_id"`
	SwapStart       time.Time  `json:"swap_start"`
	SwapEnd         time.Time  `json:"swap_end"`
	Status          string     `json:"status"` // pending, approved, rejected, canceled
	RespondedAt     *time.Time `json:"responded_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	// Joined
	RequesterName string `json:"requester_name,omitempty"`
	TargetName    string `json:"target_name,omitempty"`
}

// OnCallSwapRequestCreate represents the API request for creating a swap request
type OnCallSwapRequestCreate struct {
	TargetUserID int    `json:"target_user_id"`
	SwapStart    string `json:"swap_start"` // ISO 8601
	SwapEnd      string `json:"swap_end"`   // ISO 8601
}

// OnCallSwapRequestResponse represents the API request for responding to a swap
type OnCallSwapRequestResponse struct {
	Status string `json:"status"` // approved, rejected
}

// OnCallIncident represents an incident routed through on-call
type OnCallIncident struct {
	ID                    int        `json:"id"`
	EscalationPolicyID    int        `json:"escalation_policy_id"`
	ItemID                *int       `json:"item_id,omitempty"`
	Status                string     `json:"status"` // triggered, acknowledged, resolved
	TriggeredAt           time.Time  `json:"triggered_at"`
	AcknowledgedAt        *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy        *int       `json:"acknowledged_by,omitempty"`
	ResolvedAt            *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy            *int       `json:"resolved_by,omitempty"`
	CurrentEscalationStep int        `json:"current_escalation_step"`
	EscalationRepeatCount int        `json:"escalation_repeat_count"`
	CreatedAt             time.Time  `json:"created_at"`
	// Joined
	PolicyName         string `json:"policy_name,omitempty"`
	ItemTitle          string `json:"item_title,omitempty"`
	AcknowledgedByName string `json:"acknowledged_by_name,omitempty"`
	ResolvedByName     string `json:"resolved_by_name,omitempty"`
}

// CurrentOnCallResponse represents who is currently on call
type CurrentOnCallResponse struct {
	ScheduleID int               `json:"schedule_id"`
	OnCall     []OnCallUserEntry `json:"on_call"`
}

// OnCallUserEntry represents a user currently on call
type OnCallUserEntry struct {
	UserID     int    `json:"user_id"`
	UserName   string `json:"user_name"`
	UserEmail  string `json:"user_email"`
	LayerName  string `json:"layer_name,omitempty"`
	IsOverride bool   `json:"is_override"`
}

// CoverageGap represents a period with no on-call coverage
type CoverageGap struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
