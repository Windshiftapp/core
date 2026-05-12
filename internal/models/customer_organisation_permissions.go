package models

import "time"

// CustomerOrganisationManager represents a user or group assigned as manager of a customer organisation
type CustomerOrganisationManager struct {
	ID                     int       `json:"id" db:"id"`
	CustomerOrganisationID int       `json:"customer_organisation_id" db:"customer_organisation_id"`
	ManagerType            string    `json:"manager_type" db:"manager_type"`
	ManagerID              int       `json:"manager_id" db:"manager_id"`
	GrantedBy              *int      `json:"granted_by" db:"granted_by"`
	GrantedAt              time.Time `json:"granted_at" db:"granted_at"`

	ManagerName  string `json:"manager_name,omitempty"`
	ManagerEmail string `json:"manager_email,omitempty"`
}

// CustomerOrganisationMember represents a user or group assigned as member of a customer organisation
type CustomerOrganisationMember struct {
	ID                     int       `json:"id" db:"id"`
	CustomerOrganisationID int       `json:"customer_organisation_id" db:"customer_organisation_id"`
	MemberType             string    `json:"member_type" db:"member_type"`
	MemberID               int       `json:"member_id" db:"member_id"`
	GrantedBy              *int      `json:"granted_by" db:"granted_by"`
	GrantedAt              time.Time `json:"granted_at" db:"granted_at"`

	MemberName  string `json:"member_name,omitempty"`
	MemberEmail string `json:"member_email,omitempty"`
}

// CustomerOrganisationManagerRequest is the request payload for adding a manager
type CustomerOrganisationManagerRequest struct {
	ManagerType string `json:"manager_type"`
	ManagerID   int    `json:"manager_id"`
}

// CustomerOrganisationMemberRequest is the request payload for adding a member
type CustomerOrganisationMemberRequest struct {
	MemberType string `json:"member_type"`
	MemberID   int    `json:"member_id"`
}
