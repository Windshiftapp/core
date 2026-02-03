package models

import "time"

// Permission represents a permission definition
type Permission struct {
	ID            int       `json:"id" db:"id"`
	PermissionKey string    `json:"permission_key" db:"permission_key"`
	PermissionName string   `json:"permission_name" db:"permission_name"`
	Description   string    `json:"description" db:"description"`
	Scope         string    `json:"scope" db:"scope"` // 'global' or 'workspace'
	IsSystem      bool      `json:"is_system" db:"is_system"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// UserGlobalPermission represents a user's global permission assignment
type UserGlobalPermission struct {
	ID           int        `json:"id" db:"id"`
	UserID       int        `json:"user_id" db:"user_id"`
	PermissionID int        `json:"permission_id" db:"permission_id"`
	GrantedBy    *int       `json:"granted_by" db:"granted_by"`
	GrantedAt    time.Time  `json:"granted_at" db:"granted_at"`
	
	// Joined fields
	Permission   *Permission `json:"permission,omitempty"`
	User         *User       `json:"user,omitempty"`
	GrantedByUser *User      `json:"granted_by_user,omitempty"`
}

// UserWorkspacePermission represents a user's workspace-specific permission assignment
type UserWorkspacePermission struct {
	ID           int        `json:"id" db:"id"`
	UserID       int        `json:"user_id" db:"user_id"`
	WorkspaceID  int        `json:"workspace_id" db:"workspace_id"`
	PermissionID int        `json:"permission_id" db:"permission_id"`
	GrantedBy    *int       `json:"granted_by" db:"granted_by"`
	GrantedAt    time.Time  `json:"granted_at" db:"granted_at"`
	
	// Joined fields
	Permission    *Permission `json:"permission,omitempty"`
	User          *User       `json:"user,omitempty"`
	Workspace     *Workspace  `json:"workspace,omitempty"`
	GrantedByUser *User       `json:"granted_by_user,omitempty"`
}

// PermissionScope constants
const (
	PermissionScopeGlobal    = "global"
	PermissionScopeWorkspace = "workspace"
)

// System permission keys
const (
	// Global permissions
	PermissionSystemAdmin     = "system.admin"
	PermissionWorkspaceCreate = "workspace.create"
	PermissionMilestoneCreate = "milestone.create"
	PermissionIterationManage = "iteration.manage"
	PermissionUserList        = "user.list"
	PermissionProjectManage   = "project.manage"
	PermissionCustomersManage = "customers.manage"

	// Workspace permissions
	PermissionWorkspaceAdmin = "workspace.admin" // Manage workspace (administer workspace)

	// Item permissions
	PermissionItemView    = "item.view"    // View workspace & items
	PermissionItemEdit    = "item.edit"    // Edit items
	PermissionItemDelete  = "item.delete"  // Delete items
	PermissionItemComment = "item.comment" // Add comment & edit own comment

	// Comment permissions
	PermissionCommentEditOthers = "comment.edit_others" // Edit other comments

	// Project permissions
	PermissionProjectView   = "project.view"   // View projects
	PermissionProjectCreate = "project.create" // Create projects
	PermissionProjectEdit   = "project.edit"   // Edit projects
	PermissionProjectDelete = "project.delete" // Delete projects

	// Item creation permission
	PermissionItemCreate = "item.create" // Create work items

	// Test management permissions
	PermissionTestView    = "test.view"    // View test cases, runs, and results
	PermissionTestExecute = "test.execute" // Execute test runs and record results
	PermissionTestManage  = "test.manage"  // Create, edit, delete test cases, sets, and folders

	// Action management permissions
	PermissionActionManage = "action.manage" // Create, edit, delete, and execute workspace actions
)

// UserPermissionSummary provides a complete overview of a user's permissions
type UserPermissionSummary struct {
	UserID              int                        `json:"user_id"`
	User                *User                      `json:"user,omitempty"`
	GlobalPermissions   []UserGlobalPermission     `json:"global_permissions"`
	WorkspacePermissions []UserWorkspacePermission `json:"workspace_permissions"`
	HasSystemAdmin      bool                       `json:"has_system_admin"`
}

// PermissionRequest for granting/revoking permissions
type PermissionRequest struct {
	UserID       int  `json:"user_id" binding:"required"`
	PermissionID int  `json:"permission_id" binding:"required"`
	WorkspaceID  *int `json:"workspace_id,omitempty"` // Only for workspace permissions
}

// Group definitions for future use
type Group struct {
	ID          int        `json:"id" db:"id"`
	GroupName   string     `json:"group_name" db:"group_name"`
	Description string     `json:"description" db:"description"`
	WorkspaceID *int       `json:"workspace_id" db:"workspace_id"` // NULL for global groups
	CreatedBy   int        `json:"created_by" db:"created_by"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	
	// Joined fields
	Workspace   *Workspace `json:"workspace,omitempty"`
	Creator     *User      `json:"creator,omitempty"`
}

// UserGroup represents group membership
type UserGroup struct {
	ID      int       `json:"id" db:"id"`
	UserID  int       `json:"user_id" db:"user_id"`
	GroupID int       `json:"group_id" db:"group_id"`
	AddedBy *int      `json:"added_by" db:"added_by"`
	AddedAt time.Time `json:"added_at" db:"added_at"`

	// Joined fields
	User        *User  `json:"user,omitempty"`
	Group       *Group `json:"group,omitempty"`
	AddedByUser *User  `json:"added_by_user,omitempty"`
}

// PermissionSet represents a bundled set of permissions
type PermissionSet struct {
	ID          int       `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	IsSystem    bool      `json:"is_system" db:"is_system"`
	CreatedBy   *int      `json:"created_by" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`

	// Joined fields
	CreatedByUser *User        `json:"created_by_user,omitempty"`
	Permissions   []Permission `json:"permissions,omitempty"`
}

// PermissionSetPermission links permissions to permission sets
type PermissionSetPermission struct {
	ID              int       `json:"id" db:"id"`
	PermissionSetID int       `json:"permission_set_id" db:"permission_set_id"`
	PermissionID    int       `json:"permission_id" db:"permission_id"`
	GrantedBy       *int      `json:"granted_by" db:"granted_by"`
	GrantedAt       time.Time `json:"granted_at" db:"granted_at"`

	// Joined fields
	Permission    *Permission    `json:"permission,omitempty"`
	PermissionSet *PermissionSet `json:"permission_set,omitempty"`
}

// WorkspaceRole represents a predefined role (Viewer, Editor, Administrator)
type WorkspaceRole struct {
	ID           int       `json:"id" db:"id"`
	Name         string    `json:"name" db:"name"`
	Description  string    `json:"description" db:"description"`
	IsSystem     bool      `json:"is_system" db:"is_system"`
	DisplayOrder int       `json:"display_order" db:"display_order"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`

	// Joined fields
	Permissions []Permission `json:"permissions,omitempty"`
}

// RolePermission links permissions to roles
type RolePermission struct {
	ID           int `json:"id" db:"id"`
	RoleID       int `json:"role_id" db:"role_id"`
	PermissionID int `json:"permission_id" db:"permission_id"`

	// Joined fields
	Role       *WorkspaceRole `json:"role,omitempty"`
	Permission *Permission    `json:"permission,omitempty"`
}

// UserWorkspaceRole assigns a role to a user in a specific workspace
type UserWorkspaceRole struct {
	ID          int       `json:"id" db:"id"`
	UserID      int       `json:"user_id" db:"user_id"`
	WorkspaceID int       `json:"workspace_id" db:"workspace_id"`
	RoleID      int       `json:"role_id" db:"role_id"`
	GrantedBy   *int      `json:"granted_by" db:"granted_by"`
	GrantedAt   time.Time `json:"granted_at" db:"granted_at"`

	// Joined fields
	User          *User          `json:"user,omitempty"`
	Workspace     *Workspace     `json:"workspace,omitempty"`
	Role          *WorkspaceRole `json:"role,omitempty"`
	GrantedByUser *User          `json:"granted_by_user,omitempty"`
}

// UserWorkspaceDirectPermission assigns a permission directly to a user
type UserWorkspaceDirectPermission struct {
	ID          int       `json:"id" db:"id"`
	UserID      int       `json:"user_id" db:"user_id"`
	WorkspaceID int       `json:"workspace_id" db:"workspace_id"`
	PermissionID int      `json:"permission_id" db:"permission_id"`
	GrantedBy   *int      `json:"granted_by" db:"granted_by"`
	GrantedAt   time.Time `json:"granted_at" db:"granted_at"`

	// Joined fields
	User          *User       `json:"user,omitempty"`
	Workspace     *Workspace  `json:"workspace,omitempty"`
	Permission    *Permission `json:"permission,omitempty"`
	GrantedByUser *User       `json:"granted_by_user,omitempty"`
}

// GroupWorkspaceRole assigns a role to a group in a specific workspace
type GroupWorkspaceRole struct {
	ID          int       `json:"id" db:"id"`
	GroupID     int       `json:"group_id" db:"group_id"`
	WorkspaceID int       `json:"workspace_id" db:"workspace_id"`
	RoleID      int       `json:"role_id" db:"role_id"`
	GrantedBy   *int      `json:"granted_by" db:"granted_by"`
	GrantedAt   time.Time `json:"granted_at" db:"granted_at"`

	// Joined fields
	Group         *Group         `json:"group,omitempty"`
	Workspace     *Workspace     `json:"workspace,omitempty"`
	Role          *WorkspaceRole `json:"role,omitempty"`
	GrantedByUser *User          `json:"granted_by_user,omitempty"`
}

// PermissionSetCreateRequest represents the payload for creating a permission set
type PermissionSetCreateRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	PermissionIDs []int  `json:"permission_ids"`
}

// PermissionSetUpdateRequest represents the payload for updating a permission set
type PermissionSetUpdateRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	PermissionIDs []int  `json:"permission_ids"`
}

// UserRoleAssignmentRequest represents the payload for assigning a role to a user
type UserRoleAssignmentRequest struct {
	UserID      int `json:"user_id"`
	WorkspaceID int `json:"workspace_id"`
	RoleID      int `json:"role_id"`
}

// UserDirectPermissionRequest represents the payload for granting a direct permission
type UserDirectPermissionRequest struct {
	UserID       int `json:"user_id"`
	WorkspaceID  int `json:"workspace_id"`
	PermissionID int `json:"permission_id"`
}

// PermissionSetRoleAssignment assigns a role to a permission within a permission set
type PermissionSetRoleAssignment struct {
	ID              int       `json:"id" db:"id"`
	PermissionSetID int       `json:"permission_set_id" db:"permission_set_id"`
	PermissionID    int       `json:"permission_id" db:"permission_id"`
	RoleID          int       `json:"role_id" db:"role_id"`
	CreatedBy       *int      `json:"created_by" db:"created_by"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`

	// Joined fields
	Permission    *Permission    `json:"permission,omitempty"`
	Role          *WorkspaceRole `json:"role,omitempty"`
	PermissionSet *PermissionSet `json:"permission_set,omitempty"`
	CreatedByUser *User          `json:"created_by_user,omitempty"`
}

// PermissionSetGroupAssignment assigns a group to a permission within a permission set
type PermissionSetGroupAssignment struct {
	ID              int       `json:"id" db:"id"`
	PermissionSetID int       `json:"permission_set_id" db:"permission_set_id"`
	PermissionID    int       `json:"permission_id" db:"permission_id"`
	GroupID         int       `json:"group_id" db:"group_id"`
	CreatedBy       *int      `json:"created_by" db:"created_by"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`

	// Joined fields
	Permission    *Permission    `json:"permission,omitempty"`
	Group         *Group         `json:"group,omitempty"`
	PermissionSet *PermissionSet `json:"permission_set,omitempty"`
	CreatedByUser *User          `json:"created_by_user,omitempty"`
}

// PermissionSetUserAssignment assigns a user to a permission within a permission set
type PermissionSetUserAssignment struct {
	ID              int       `json:"id" db:"id"`
	PermissionSetID int       `json:"permission_set_id" db:"permission_set_id"`
	PermissionID    int       `json:"permission_id" db:"permission_id"`
	UserID          int       `json:"user_id" db:"user_id"`
	CreatedBy       *int      `json:"created_by" db:"created_by"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`

	// Joined fields
	Permission    *Permission    `json:"permission,omitempty"`
	User          *User          `json:"user,omitempty"`
	PermissionSet *PermissionSet `json:"permission_set,omitempty"`
	CreatedByUser *User          `json:"created_by_user,omitempty"`
}

// PermissionSetAssignmentRequest represents the payload for assigning roles/groups/users to permissions
type PermissionSetAssignmentRequest struct {
	PermissionID int `json:"permission_id"`
	RoleID       *int `json:"role_id,omitempty"`
	GroupID      *int `json:"group_id,omitempty"`
	UserID       *int `json:"user_id,omitempty"`
}