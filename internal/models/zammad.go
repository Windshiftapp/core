package models

import "time"

const IntegrationProviderZammad IntegrationProviderType = "zammad"

type ZammadAuthMethod string

const (
	ZammadAuthMethodAPIToken ZammadAuthMethod = "api_token"
	ZammadAuthMethodOAuth    ZammadAuthMethod = "oauth"
)

type ZammadSyncState string

const (
	ZammadSyncPending   ZammadSyncState = "pending"
	ZammadSyncCreating  ZammadSyncState = "creating"
	ZammadSyncLinked    ZammadSyncState = "linked"
	ZammadSyncFailed    ZammadSyncState = "sync_failed"
	ZammadSyncUncertain ZammadSyncState = "creation_uncertain"
)

// ZammadConnection is the provider-specific extension of an
// integration_providers row. The API token stays in action_credentials and is
// never serialized through this model.
type ZammadConnection struct {
	ProviderID                 string           `json:"id"`
	Slug                       string           `json:"slug"`
	Name                       string           `json:"name"`
	Enabled                    bool             `json:"enabled"`
	BaseURL                    string           `json:"base_url"`
	CredentialID               int              `json:"-"`
	AuthMethod                 ZammadAuthMethod `json:"auth_method"`
	OAuthGeneration            int64            `json:"-"`
	ConfigRevision             int64            `json:"-"`
	OAuthAttemptID             string           `json:"-"`
	OAuthClientID              string           `json:"oauth_client_id,omitempty"`
	OAuthClientSecretEncrypted string           `json:"-"`
	HasOAuthClientSecret       bool             `json:"has_oauth_client_secret,omitempty"`
	OAuthConnected             bool             `json:"oauth_connected,omitempty"`
	ReauthorizationRequired    bool             `json:"reauthorization_required,omitempty"`
	DefaultGroupID             int              `json:"default_group_id,omitempty"`
	DefaultGroupName           string           `json:"default_group_name,omitempty"`
	AllowedGroups              []ZammadGroupRef `json:"allowed_groups"`
	DefaultCustomer            string           `json:"default_customer"`
	CorrelationField           string           `json:"correlation_field"`
	ClosedStateIDs             []int            `json:"closed_state_ids"`
	CompletionStatusID         *int             `json:"completion_status_id,omitempty"`
	AppliesToAllWorkspaces     bool             `json:"applies_to_all_workspaces"`
	WorkspaceIDs               []int            `json:"workspace_ids,omitempty"`
	HasAPIToken                bool             `json:"has_api_token"`
	LastTestedAt               *time.Time       `json:"last_tested_at,omitempty"`
	LastTestError              string           `json:"last_test_error,omitempty"`
	CreatedBy                  *int             `json:"created_by,omitempty"`
	CreatedAt                  time.Time        `json:"created_at"`
	UpdatedAt                  time.Time        `json:"updated_at"`
}

type ZammadWorkspaceConnection struct {
	ProviderID              string           `json:"id"`
	Name                    string           `json:"name"`
	AuthMethod              ZammadAuthMethod `json:"auth_method"`
	Ready                   bool             `json:"ready"`
	OAuthConnected          bool             `json:"oauth_connected,omitempty"`
	ReauthorizationRequired bool             `json:"reauthorization_required,omitempty"`
	DefaultGroupID          int              `json:"default_group_id,omitempty"`
	DefaultGroupName        string           `json:"default_group_name,omitempty"`
	AllowedGroups           []ZammadGroupRef `json:"allowed_groups"`
}

type CreateZammadConnectionRequest struct {
	Slug                   string           `json:"slug"`
	Name                   string           `json:"name"`
	Enabled                *bool            `json:"enabled,omitempty"`
	BaseURL                string           `json:"base_url"`
	APIToken               string           `json:"api_token"`
	AuthMethod             ZammadAuthMethod `json:"auth_method,omitempty"`
	OAuthClientID          string           `json:"oauth_client_id,omitempty"`
	OAuthClientSecret      string           `json:"oauth_client_secret,omitempty"`
	DefaultGroupID         int              `json:"default_group_id,omitempty"`
	DefaultGroupName       string           `json:"default_group_name,omitempty"`
	AllowedGroups          []ZammadGroupRef `json:"allowed_groups,omitempty"`
	AllowedGroupIDs        []int            `json:"allowed_group_ids,omitempty"`
	DefaultCustomer        string           `json:"default_customer"`
	CorrelationField       string           `json:"correlation_field,omitempty"`
	ClosedStateIDs         []int            `json:"closed_state_ids,omitempty"`
	CompletionStatusID     *int             `json:"completion_status_id,omitempty"`
	AppliesToAllWorkspaces *bool            `json:"applies_to_all_workspaces,omitempty"`
	WorkspaceIDs           []int            `json:"workspace_ids,omitempty"`
}

type UpdateZammadConnectionRequest struct {
	Slug                   *string           `json:"slug,omitempty"`
	Name                   *string           `json:"name,omitempty"`
	Enabled                *bool             `json:"enabled,omitempty"`
	BaseURL                *string           `json:"base_url,omitempty"`
	APIToken               *string           `json:"api_token,omitempty"`
	AuthMethod             *ZammadAuthMethod `json:"auth_method,omitempty"`
	OAuthClientID          *string           `json:"oauth_client_id,omitempty"`
	OAuthClientSecret      *string           `json:"oauth_client_secret,omitempty"`
	DefaultGroupID         *int              `json:"default_group_id,omitempty"`
	DefaultGroupName       *string           `json:"default_group_name,omitempty"`
	AllowedGroups          *[]ZammadGroupRef `json:"allowed_groups,omitempty"`
	AllowedGroupIDs        *[]int            `json:"allowed_group_ids,omitempty"`
	DefaultCustomer        *string           `json:"default_customer,omitempty"`
	CorrelationField       *string           `json:"correlation_field,omitempty"`
	ClosedStateIDs         *[]int            `json:"closed_state_ids,omitempty"`
	CompletionStatusID     *int              `json:"completion_status_id,omitempty"`
	ClearCompletionStatus  bool              `json:"clear_completion_status,omitempty"`
	AppliesToAllWorkspaces *bool             `json:"applies_to_all_workspaces,omitempty"`
	WorkspaceIDs           *[]int            `json:"workspace_ids,omitempty"`
}

type ZammadGroup struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ZammadGroupRef is the persisted group policy used at runtime. Keeping both
// the stable ID and display name avoids requiring Zammad's admin-only groups
// endpoint after connection setup.
type ZammadGroupRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ZammadState struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	StateTypeID int    `json:"state_type_id"`
	Active      bool   `json:"active"`
}

type ZammadConnectionMetadata struct {
	Groups                   []ZammadGroup `json:"groups"`
	States                   []ZammadState `json:"states"`
	GroupCatalogVerified     bool          `json:"group_catalog_verified"`
	CorrelationFieldVerified bool          `json:"correlation_field_verified"`
}

type CreateZammadTicketRequest struct {
	ConnectionID string `json:"connection_id"`
	GroupID      int    `json:"group_id,omitempty"`
}

type LinkZammadTicketRequest struct {
	ConnectionID string `json:"connection_id"`
	TicketNumber string `json:"ticket_number"`
}

type UpdateZammadTicketLinkRequest struct {
	StateID *int `json:"state_id,omitempty"`
	GroupID *int `json:"group_id,omitempty"`
	OwnerID *int `json:"owner_id,omitempty"`
}

type ZammadOwner struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ZammadTicketLink struct {
	ID                    string          `json:"id"`
	ItemID                int             `json:"item_id"`
	ProviderID            string          `json:"connection_id"`
	ProviderName          string          `json:"connection_name,omitempty"`
	ItemIntegrationLinkID string          `json:"item_integration_link_id,omitempty"`
	TicketID              int             `json:"ticket_id,omitempty"`
	TicketNumber          string          `json:"ticket_number,omitempty"`
	TicketURL             string          `json:"ticket_url,omitempty"`
	GroupID               int             `json:"group_id,omitempty"`
	GroupName             string          `json:"group_name,omitempty"`
	OwnerID               int             `json:"owner_id,omitempty"`
	OwnerName             string          `json:"owner_name,omitempty"`
	CorrelationKey        string          `json:"correlation_key"`
	SyncState             ZammadSyncState `json:"sync_state"`
	LastStatusID          int             `json:"last_status_id,omitempty"`
	LastStatusName        string          `json:"last_status_name,omitempty"`
	LastSyncedAt          *time.Time      `json:"last_synced_at,omitempty"`
	LastAttemptAt         *time.Time      `json:"-"`
	NextAttemptAt         *time.Time      `json:"-"`
	LastError             string          `json:"last_error,omitempty"`
	CompletionApplied     bool            `json:"-"`
	CreatedBy             *int            `json:"created_by,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

type ZammadTicketLinkResponse struct {
	ID             string          `json:"id"`
	ProviderID     string          `json:"connection_id"`
	ProviderName   string          `json:"connection_name,omitempty"`
	TicketID       int             `json:"ticket_id,omitempty"`
	TicketNumber   string          `json:"ticket_number,omitempty"`
	TicketURL      string          `json:"ticket_url,omitempty"`
	GroupID        int             `json:"group_id,omitempty"`
	GroupName      string          `json:"group_name,omitempty"`
	OwnerID        int             `json:"owner_id,omitempty"`
	OwnerName      string          `json:"owner_name,omitempty"`
	SyncState      ZammadSyncState `json:"sync_state"`
	LastStatusID   int             `json:"last_status_id,omitempty"`
	LastStatusName string          `json:"last_status_name,omitempty"`
	LastSyncedAt   *time.Time      `json:"last_synced_at,omitempty"`
	LastError      string          `json:"last_error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func (link *ZammadTicketLink) Response() ZammadTicketLinkResponse {
	return ZammadTicketLinkResponse{
		ID: link.ID, ProviderID: link.ProviderID, ProviderName: link.ProviderName,
		TicketID: link.TicketID, TicketNumber: link.TicketNumber, TicketURL: link.TicketURL,
		GroupID: link.GroupID, GroupName: link.GroupName, SyncState: link.SyncState,
		OwnerID: link.OwnerID, OwnerName: link.OwnerName,
		LastStatusID: link.LastStatusID, LastStatusName: link.LastStatusName,
		LastSyncedAt: link.LastSyncedAt, LastError: link.LastError,
		CreatedAt: link.CreatedAt, UpdatedAt: link.UpdatedAt,
	}
}
