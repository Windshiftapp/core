package models

import "time"

// LDAPConfig represents an LDAP directory configuration.
type LDAPConfig struct {
	ID                    int       `json:"id"`
	Name                  string    `json:"name"`
	Enabled               bool      `json:"enabled"`
	Host                  string    `json:"host"`
	Port                  int       `json:"port"`
	UseTLS                bool      `json:"use_tls"`
	UseSSL                bool      `json:"use_ssl"`
	SkipTLSVerify         bool      `json:"skip_tls_verify"`
	BindDN                string    `json:"bind_dn"`
	BindPasswordEncrypted string    `json:"-"` // Never send to client
	BaseDN                string    `json:"base_dn"`
	UserFilter            string    `json:"user_filter"`
	GroupBaseDN           string    `json:"group_base_dn,omitempty"`
	GroupFilter           string    `json:"group_filter,omitempty"`
	AttrUsername          string    `json:"attr_username"`
	AttrEmail             string    `json:"attr_email"`
	AttrFirstName         string    `json:"attr_first_name"`
	AttrLastName          string    `json:"attr_last_name"`
	AttrDisplayName       string    `json:"attr_display_name"`
	AttrGroupMember       string    `json:"attr_group_member"`
	SyncIntervalMinutes   int       `json:"sync_interval_minutes"`
	AutoProvisionUsers    bool      `json:"auto_provision_users"`
	AutoDeactivateUsers   bool      `json:"auto_deactivate_users"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// LDAPSyncStatus represents the status of an LDAP sync operation.
type LDAPSyncStatus struct {
	ID               int        `json:"id"`
	ConfigID         int        `json:"config_id"`
	Status           string     `json:"status"` // pending, running, completed, failed
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	UsersSynced      int        `json:"users_synced"`
	UsersCreated     int        `json:"users_created"`
	UsersUpdated     int        `json:"users_updated"`
	UsersDeactivated int        `json:"users_deactivated"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// LDAPUserMapping tracks which local users are linked to LDAP entries.
type LDAPUserMapping struct {
	ID           int       `json:"id"`
	ConfigID     int       `json:"config_id"`
	UserID       int       `json:"user_id"`
	LDAPDN       string    `json:"ldap_dn"`
	LDAPUID      string    `json:"ldap_uid"`
	LastSyncedAt time.Time `json:"last_synced_at"`
	CreatedAt    time.Time `json:"created_at"`
}
