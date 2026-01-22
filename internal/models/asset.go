package models

import "time"

// AssetManagementSet represents a system-wide asset management container
type AssetManagementSet struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	CreatorName    string `json:"creator_name,omitempty"`
	AssetTypeCount int    `json:"asset_type_count,omitempty"`
	AssetCount     int    `json:"asset_count,omitempty"`
	// User's permission on this set (populated per-request)
	UserPermission string `json:"user_permission,omitempty"` // view, edit, admin, or empty
}

// AssetManagementSetPermission represents user-level permission for an asset set
type AssetManagementSetPermission struct {
	ID              int       `json:"id"`
	SetID           int       `json:"set_id"`
	UserID          int       `json:"user_id"`
	PermissionLevel string    `json:"permission_level"` // view, edit, admin
	GrantedBy       *int      `json:"granted_by,omitempty"`
	GrantedAt       time.Time `json:"granted_at"`
	// Joined fields
	UserName      string `json:"user_name,omitempty"`
	UserEmail     string `json:"user_email,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}

// AssetManagementSetGroupPermission represents group-level permission for an asset set
type AssetManagementSetGroupPermission struct {
	ID              int       `json:"id"`
	SetID           int       `json:"set_id"`
	GroupID         int       `json:"group_id"`
	PermissionLevel string    `json:"permission_level"` // view, edit, admin
	GrantedBy       *int      `json:"granted_by,omitempty"`
	GrantedAt       time.Time `json:"granted_at"`
	// Joined fields
	GroupName     string `json:"group_name,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}

// AssetType defines the structure/attributes of assets
type AssetType struct {
	ID           int       `json:"id"`
	SetID        int       `json:"set_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Icon         string    `json:"icon"`
	Color        string    `json:"color"`
	DisplayOrder int       `json:"display_order"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Joined fields
	SetName    string           `json:"set_name,omitempty"`
	AssetCount int              `json:"asset_count,omitempty"`
	Fields     []AssetTypeField `json:"fields,omitempty"`
}

// AssetTypeField represents a custom field assignment to an asset type
type AssetTypeField struct {
	ID            int       `json:"id"`
	AssetTypeID   int       `json:"asset_type_id"`
	CustomFieldID int       `json:"custom_field_id"`
	IsRequired    bool      `json:"is_required"`
	DisplayOrder  int       `json:"display_order"`
	CreatedAt     time.Time `json:"created_at"`
	// Joined fields from custom_field_definitions
	FieldName        string `json:"field_name,omitempty"`
	FieldType        string `json:"field_type,omitempty"`
	FieldDescription string `json:"field_description,omitempty"`
	Options          string `json:"options,omitempty"`
}

// AssetCategory represents a hierarchical organizational unit for assets
type AssetCategory struct {
	ID               int       `json:"id"`
	SetID            int       `json:"set_id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	ParentID         *int      `json:"parent_id,omitempty"`
	Path             string    `json:"path,omitempty"`
	HasChildren      bool      `json:"has_children"`
	ChildrenCount    int       `json:"children_count"`
	DescendantsCount int       `json:"descendants_count"`
	FracIndex        *string   `json:"frac_index,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Joined/computed fields
	SetName    string          `json:"set_name,omitempty"`
	ParentName string          `json:"parent_name,omitempty"`
	AssetCount int             `json:"asset_count,omitempty"`
	Children   []AssetCategory `json:"children,omitempty"`
}

// AssetStatus represents a configurable status for assets within a set
type AssetStatus struct {
	ID           int       `json:"id"`
	SetID        int       `json:"set_id"`
	Name         string    `json:"name"`
	Color        string    `json:"color"`
	Description  string    `json:"description"`
	IsDefault    bool      `json:"is_default"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Asset represents an individual asset instance
type Asset struct {
	ID                int                    `json:"id"`
	SetID             int                    `json:"set_id"`
	AssetTypeID       int                    `json:"asset_type_id"`
	CategoryID        *int                   `json:"category_id,omitempty"`
	StatusID          *int                   `json:"status_id,omitempty"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	AssetTag          string                 `json:"asset_tag,omitempty"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
	FracIndex         *string                `json:"frac_index,omitempty"`
	CreatedBy         *int                   `json:"created_by,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	// Joined fields
	SetName        string `json:"set_name,omitempty"`
	AssetTypeName  string `json:"asset_type_name,omitempty"`
	AssetTypeIcon  string `json:"asset_type_icon,omitempty"`
	AssetTypeColor string `json:"asset_type_color,omitempty"`
	CategoryName   string `json:"category_name,omitempty"`
	CategoryPath   string `json:"category_path,omitempty"`
	StatusName     string `json:"status_name,omitempty"`
	StatusColor    string `json:"status_color,omitempty"`
	CreatorName    string `json:"creator_name,omitempty"`
	CreatorEmail   string `json:"creator_email,omitempty"`
	// Linked items count
	LinkedItemCount int `json:"linked_item_count,omitempty"`
}

// UserAssetSetPreference stores user's primary asset set preference
type UserAssetSetPreference struct {
	ID           int       `json:"id"`
	UserID       int       `json:"user_id"`
	PrimarySetID *int      `json:"primary_set_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Joined fields
	PrimarySetName string `json:"primary_set_name,omitempty"`
}

// AssetRole represents a role for asset management (Viewer, Editor, Administrator)
type AssetRole struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	IsSystem     bool      `json:"is_system"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Joined fields
	Permissions []AssetPermission `json:"permissions,omitempty"`
}

// AssetPermission represents a specific asset permission (asset.view, asset.edit, etc.)
type AssetPermission struct {
	ID             int       `json:"id"`
	PermissionKey  string    `json:"permission_key"`
	PermissionName string    `json:"permission_name"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
}

// UserAssetSetRole represents a user's role assignment for an asset set
type UserAssetSetRole struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	SetID     int       `json:"set_id"`
	RoleID    int       `json:"role_id"`
	GrantedBy *int      `json:"granted_by,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
	// Joined fields
	UserName      string `json:"user_name,omitempty"`
	UserEmail     string `json:"user_email,omitempty"`
	RoleName      string `json:"role_name,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}

// GroupAssetSetRole represents a group's role assignment for an asset set
type GroupAssetSetRole struct {
	ID        int       `json:"id"`
	GroupID   int       `json:"group_id"`
	SetID     int       `json:"set_id"`
	RoleID    int       `json:"role_id"`
	GrantedBy *int      `json:"granted_by,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
	// Joined fields
	GroupName     string `json:"group_name,omitempty"`
	RoleName      string `json:"role_name,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}

// AssetSetEveryoneRole represents the default role for all authenticated users on a set
type AssetSetEveryoneRole struct {
	SetID     int       `json:"set_id"`
	RoleID    *int      `json:"role_id,omitempty"`
	GrantedBy *int      `json:"granted_by,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
	// Joined fields
	RoleName      string `json:"role_name,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}
