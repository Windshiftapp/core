package models

import "time"

// TimeProjectManager represents a user or group assigned as manager of a time project
type TimeProjectManager struct {
	ID          int       `json:"id" db:"id"`
	ProjectID   int       `json:"project_id" db:"project_id"`
	ManagerType string    `json:"manager_type" db:"manager_type"` // 'user' or 'group'
	ManagerID   int       `json:"manager_id" db:"manager_id"`
	GrantedBy   *int      `json:"granted_by" db:"granted_by"`
	GrantedAt   time.Time `json:"granted_at" db:"granted_at"`

	// Joined fields for API responses
	ManagerName  string `json:"manager_name,omitempty"`
	ManagerEmail string `json:"manager_email,omitempty"`
}

// TimeProjectMember represents a user or group assigned as member of a time project
type TimeProjectMember struct {
	ID         int       `json:"id" db:"id"`
	ProjectID  int       `json:"project_id" db:"project_id"`
	MemberType string    `json:"member_type" db:"member_type"` // 'user' or 'group'
	MemberID   int       `json:"member_id" db:"member_id"`
	GrantedBy  *int      `json:"granted_by" db:"granted_by"`
	GrantedAt  time.Time `json:"granted_at" db:"granted_at"`

	// Joined fields for API responses
	MemberName  string `json:"member_name,omitempty"`
	MemberEmail string `json:"member_email,omitempty"`
}

// TimeProjectManagerRequest represents the API request for adding a project manager
type TimeProjectManagerRequest struct {
	ManagerType string `json:"manager_type"` // 'user' or 'group'
	ManagerID   int    `json:"manager_id"`
}

// TimeProjectMemberRequest represents the API request for adding a project member
type TimeProjectMemberRequest struct {
	MemberType string `json:"member_type"` // 'user' or 'group'
	MemberID   int    `json:"member_id"`
}
