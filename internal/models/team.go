package models

import "time"

// Team represents a functional unit for ticket handling and scheduling
type Team struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	CreatedByName       string             `json:"created_by_name,omitempty"`
	DirectMemberCount   int                `json:"direct_member_count"`
	GroupCount          int                `json:"group_count"`
	ResolvedMemberCount int                `json:"resolved_member_count,omitempty"`
	DirectMembers       []TeamMember       `json:"direct_members,omitempty"`
	MappedGroups        []TeamGroupMapping `json:"mapped_groups,omitempty"`
}

// TeamCreateRequest represents the API request for creating a team
type TeamCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// TeamUpdateRequest represents the API request for updating a team
type TeamUpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
}

// TeamMember represents a direct member of a team
type TeamMember struct {
	ID        int       `json:"id"`
	TeamID    int       `json:"team_id"`
	UserID    int       `json:"user_id"`
	Role      string    `json:"role"` // "member" or "admin"
	AddedBy   *int      `json:"added_by,omitempty"`
	AddedAt   time.Time `json:"added_at"`
	CreatedAt time.Time `json:"created_at"`
	// Joined user fields
	UserName      string `json:"user_name,omitempty"`
	UserEmail     string `json:"user_email,omitempty"`
	UserUsername  string `json:"user_username,omitempty"`
	UserAvatarURL string `json:"user_avatar_url,omitempty"`
	AddedByName   string `json:"added_by_name,omitempty"`
}

// TeamMemberRequest represents the API request for adding/removing members
type TeamMemberRequest struct {
	UserIDs []int  `json:"user_ids"`
	Role    string `json:"role,omitempty"` // defaults to "member"
}

// TeamMemberRoleRequest represents the API request for updating a member's role
type TeamMemberRoleRequest struct {
	Role string `json:"role"`
}

// TeamGroupMapping represents a group mapped to a team
type TeamGroupMapping struct {
	ID      int       `json:"id"`
	TeamID  int       `json:"team_id"`
	GroupID int       `json:"group_id"`
	AddedBy *int      `json:"added_by,omitempty"`
	AddedAt time.Time `json:"added_at"`
	// Joined group fields
	GroupName   string `json:"group_name,omitempty"`
	MemberCount int    `json:"member_count"`
	AddedByName string `json:"added_by_name,omitempty"`
}

// TeamGroupRequest represents the API request for adding/removing group mappings
type TeamGroupRequest struct {
	GroupIDs []int `json:"group_ids"`
}

// ResolvedTeamMember represents a union view member (from direct membership or group)
type ResolvedTeamMember struct {
	UserID         int    `json:"user_id"`
	UserName       string `json:"user_name"`
	UserEmail      string `json:"user_email"`
	UserUsername   string `json:"user_username"`
	UserAvatarURL  string `json:"user_avatar_url,omitempty"`
	Source         string `json:"source"` // "direct" or "group"
	SourceName     string `json:"source_name"`
	IsOnLeave      bool   `json:"is_on_leave"`
	SubstituteID   *int   `json:"substitute_id,omitempty"`
	SubstituteName string `json:"substitute_name,omitempty"`
}

// UserLeavePeriod represents a leave period for a user
type UserLeavePeriod struct {
	ID               int       `json:"id"`
	UserID           int       `json:"user_id"`
	SubstituteUserID *int      `json:"substitute_user_id,omitempty"`
	StartDate        string    `json:"start_date"` // YYYY-MM-DD
	EndDate          string    `json:"end_date"`   // YYYY-MM-DD
	Reason           string    `json:"reason"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Joined fields
	SubstituteName string `json:"substitute_name,omitempty"`
	UserName       string `json:"user_name,omitempty"`
}

// UserLeavePeriodRequest represents the API request for creating/updating a leave period
type UserLeavePeriodRequest struct {
	SubstituteUserID *int   `json:"substitute_user_id,omitempty"`
	StartDate        string `json:"start_date"`
	EndDate          string `json:"end_date"`
	Reason           string `json:"reason"`
}

// RoundRobinState represents the current state of round-robin assignment for a team+node
type RoundRobinState struct {
	ID                 int        `json:"id"`
	ActionNodeID       int        `json:"action_node_id"`
	TeamID             int        `json:"team_id"`
	LastAssignedUserID *int       `json:"last_assigned_user_id,omitempty"`
	LastAssignedAt     *time.Time `json:"last_assigned_at,omitempty"`
	AssignmentCount    int        `json:"assignment_count"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// RoundRobinAssignNodeConfig represents the configuration for a round_robin_assign action node
type RoundRobinAssignNodeConfig struct {
	TeamID              int  `json:"team_id"`
	SkipOnLeaveMembers  bool `json:"skip_on_leave_members"`
	UseLeaveSubstitutes bool `json:"use_leave_substitutes"`
}
