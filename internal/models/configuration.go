package models

import "time"

// ConfigurationSet represents a configuration set for workspaces
type ConfigurationSet struct {
	ID                      int       `json:"id"`
	WorkspaceID             int       `json:"workspace_id"` // Keep for backward compatibility
	Name                    string    `json:"name"`
	Description             string    `json:"description"`
	IsDefault               bool      `json:"is_default"`
	DifferentiateByItemType bool      `json:"differentiate_by_item_type"`
	WorkflowID              *int      `json:"workflow_id,omitempty"`
	NotificationSettingID   *int      `json:"notification_setting_id,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	// Joined fields for API responses
	WorkspaceName           string `json:"workspace_name,omitempty"`
	WorkflowName            string `json:"workflow_name,omitempty"`
	NotificationSettingName string `json:"notification_setting_name,omitempty"`
	// Many-to-many workspace relationships
	WorkspaceIDs []int    `json:"workspace_ids,omitempty"`
	Workspaces   []string `json:"workspaces,omitempty"` // Workspace names for display
	// Item types associated with this configuration set
	ItemTypes         []string          `json:"item_types,omitempty"`          // Item type names for display (deprecated, use ItemTypesDetailed)
	ItemTypesDetailed []ItemTypeDisplay `json:"item_types_detailed,omitempty"` // Full item type data with icons and colors (deprecated, use ItemTypeConfigs)
	ItemTypeConfigs   []ItemTypeConfig  `json:"item_type_configs,omitempty"`   // Item type configurations with optional workflow and screen overrides
	// Priorities associated with this configuration set
	PriorityIDs        []int             `json:"priority_ids,omitempty"`        // IDs of associated priorities
	PrioritiesDetailed []PriorityDisplay `json:"priorities_detailed,omitempty"` // Full priority data with icons and colors
	// Screen assignments for different contexts
	CreateScreenID   *int   `json:"create_screen_id,omitempty"`
	EditScreenID     *int   `json:"edit_screen_id,omitempty"`
	ViewScreenID     *int   `json:"view_screen_id,omitempty"`
	CreateScreenName string `json:"create_screen_name,omitempty"`
	EditScreenName   string `json:"edit_screen_name,omitempty"`
	ViewScreenName   string `json:"view_screen_name,omitempty"`
	// Default item type for new items (when user has no localStorage preference)
	DefaultItemTypeID   *int   `json:"default_item_type_id,omitempty"`
	DefaultItemTypeName string `json:"default_item_type_name,omitempty"`
}

// ConfigurationSetScreen represents a screen assignment for a configuration set
type ConfigurationSetScreen struct {
	ID                 int       `json:"id"`
	ConfigurationSetID int       `json:"configuration_set_id"`
	ScreenID           int       `json:"screen_id"`
	Context            string    `json:"context"` // create, edit, view
	CreatedAt          time.Time `json:"created_at"`
	// Joined fields for API responses
	ScreenName           string `json:"screen_name,omitempty"`
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`
}

// Screen represents a field layout screen
type Screen struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	Fields       []ScreenField `json:"fields,omitempty"`
	SystemFields []string      `json:"system_fields,omitempty"` // List of system field names to show
}

// ScreenField represents a field on a screen
type ScreenField struct {
	ID              int    `json:"id"`
	ScreenID        int    `json:"screen_id"`
	FieldType       string `json:"field_type"` // 'default' or 'custom'
	FieldIdentifier string `json:"field_identifier"`
	DisplayOrder    int    `json:"display_order"`
	IsRequired      bool   `json:"is_required"`
	FieldWidth      string `json:"field_width"`
	// Joined/computed fields for API responses
	FieldName   string                 `json:"field_name,omitempty"`
	FieldLabel  string                 `json:"field_label,omitempty"`
	FieldConfig map[string]interface{} `json:"field_config,omitempty"`
}

// ItemType represents a type of work item
type ItemType struct {
	ID                 int       `json:"id"`
	ConfigurationSetID int       `json:"configuration_set_id,omitempty"` // Deprecated: kept for backward compatibility
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	IsDefault          bool      `json:"is_default"`
	Icon               string    `json:"icon"`            // Lucide icon name
	Color              string    `json:"color"`           // Hex color for background
	HierarchyLevel     int       `json:"hierarchy_level"` // 0=top level, 1=level 1, etc.
	SortOrder          int       `json:"sort_order"`      // For ordering within same level
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Many-to-many configuration set relationships
	ConfigurationSetIDs   []int    `json:"configuration_set_ids,omitempty"`   // IDs of associated configuration sets
	ConfigurationSetNames []string `json:"configuration_set_names,omitempty"` // Names for display
	// Deprecated joined fields (kept for backward compatibility)
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`
	WorkspaceName        string `json:"workspace_name,omitempty"`
}

// ItemTypeDisplay holds minimal item type data for displaying in configuration sets
type ItemTypeDisplay struct {
	Name           string `json:"name"`
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	HierarchyLevel int    `json:"hierarchy_level"`
}

// ItemTypeConfig represents item type configuration with optional workflow and screen overrides
type ItemTypeConfig struct {
	ItemTypeID     int    `json:"item_type_id"`
	ItemTypeName   string `json:"item_type_name"`
	ItemTypeIcon   string `json:"item_type_icon"`
	ItemTypeColor  string `json:"item_type_color"`
	HierarchyLevel int    `json:"hierarchy_level"`
	// Override workflow (NULL = use configuration set default)
	WorkflowID   *int   `json:"workflow_id,omitempty"`
	WorkflowName string `json:"workflow_name,omitempty"` // "Default" or workflow name
	// Override screens (NULL = use configuration set defaults)
	CreateScreenID   *int   `json:"create_screen_id,omitempty"`
	CreateScreenName string `json:"create_screen_name,omitempty"`
	EditScreenID     *int   `json:"edit_screen_id,omitempty"`
	EditScreenName   string `json:"edit_screen_name,omitempty"`
	ViewScreenID     *int   `json:"view_screen_id,omitempty"`
	ViewScreenName   string `json:"view_screen_name,omitempty"`
}

// Priority represents a priority level
type Priority struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	Icon        string    `json:"icon"`       // Lucide icon name
	Color       string    `json:"color"`      // Hex color for background
	SortOrder   int       `json:"sort_order"` // For ordering priorities
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Many-to-many configuration set relationships
	ConfigurationSetIDs   []int    `json:"configuration_set_ids,omitempty"`   // IDs of associated configuration sets
	ConfigurationSetNames []string `json:"configuration_set_names,omitempty"` // Names for display
}

// PriorityDisplay holds minimal priority data for displaying in configuration sets
type PriorityDisplay struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

// HierarchyLevel represents a hierarchy level definition
type HierarchyLevel struct {
	ID          int       `json:"id"`
	Level       int       `json:"level"` // 0, 1, 2, 3...
	Name        string    `json:"name"`  // e.g., "Initiative", "Epic", "Task", "Sub-task"
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StatusCategory represents a category for statuses
type StatusCategory struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // Hex color code (e.g., "#3b82f6")
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	IsCompleted bool      `json:"is_completed"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Status represents a workflow status
type Status struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CategoryID  int       `json:"category_id"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	IsCompleted   bool   `json:"is_completed,omitempty"`
}

// Workflow represents a workflow definition
type Workflow struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	Transitions []WorkflowTransition `json:"transitions,omitempty"`
}

// WorkflowTransition represents a transition between statuses in a workflow
type WorkflowTransition struct {
	ID           int       `json:"id"`
	WorkflowID   int       `json:"workflow_id"`
	FromStatusID *int      `json:"from_status_id"` // NULL means it's an initial status
	ToStatusID   int       `json:"to_status_id"`
	DisplayOrder int       `json:"display_order"`
	SourceHandle string    `json:"source_handle,omitempty"` // Connection point on source status (top, right, bottom, left)
	TargetHandle string    `json:"target_handle,omitempty"` // Connection point on target status (top, right, bottom, left)
	CreatedAt    time.Time `json:"created_at"`
	// Joined fields for API responses
	FromStatusName string `json:"from_status_name,omitempty"`
	ToStatusName   string `json:"to_status_name,omitempty"`
	WorkflowName   string `json:"workflow_name,omitempty"`
}

// CustomFieldIndexInfo represents which tables have indexes for a custom field
type CustomFieldIndexInfo struct {
	Items  bool `json:"items"`
	Assets bool `json:"assets"`
}

// CustomFieldDefinition represents a custom field definition
type CustomFieldDefinition struct {
	ID                             int       `json:"id"`
	Name                           string    `json:"name"`
	FieldType                      string    `json:"field_type"`
	Description                    string    `json:"description,omitempty"`
	Required                       bool      `json:"required"`
	Options                        string    `json:"options,omitempty"` // JSON string for select options
	DisplayOrder                   int       `json:"display_order"`
	SystemDefault                  bool      `json:"system_default"` // Cannot be deleted by users
	AppliesToPortalCustomers       bool      `json:"applies_to_portal_customers"`
	AppliesToCustomerOrganisations bool      `json:"applies_to_customer_organisations"` //nolint:misspell // matches database column name
	CreatedAt                      time.Time `json:"created_at"`
	UpdatedAt                      time.Time `json:"updated_at"`
}

// ProjectFieldRequirement represents a field requirement for a project
type ProjectFieldRequirement struct {
	ID            int  `json:"id"`
	ProjectID     int  `json:"project_id"`
	CustomFieldID int  `json:"custom_field_id"`
	IsRequired    bool `json:"is_required"`
	// Joined fields for API responses
	FieldName   string `json:"field_name,omitempty"`
	FieldType   string `json:"field_type,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
}

// PaginatedConfigurationSetsResponse represents a paginated list of configuration sets
type PaginatedConfigurationSetsResponse struct {
	ConfigurationSets []ConfigurationSet `json:"configuration_sets"`
	Pagination        PaginationMeta     `json:"pagination"`
}

// Workflow Migration Models

// StatusMigrationInfo describes status migration information
type StatusMigrationInfo struct {
	CurrentStatus       string `json:"current_status"`
	CurrentStatusID     *int   `json:"current_status_id"`
	ItemTypeID          *int   `json:"item_type_id,omitempty"`
	ItemTypeName        string `json:"item_type_name,omitempty"`
	RequiresMigration   bool   `json:"requires_migration"`
	SuggestedStatusID   *int   `json:"suggested_status_id"`
	SuggestedStatusName string `json:"suggested_status_name"`
	ItemCount           int    `json:"item_count"`
}

// WorkflowMigrationAnalysis represents workflow migration analysis
type WorkflowMigrationAnalysis struct {
	OldWorkflowID      *int                  `json:"old_workflow_id"`
	OldWorkflowName    string                `json:"old_workflow_name"`
	NewWorkflowID      *int                  `json:"new_workflow_id"`
	NewWorkflowName    string                `json:"new_workflow_name"`
	AffectedWorkspaces []int                 `json:"affected_workspaces"`
	StatusMigrations   []StatusMigrationInfo `json:"status_migrations"`
	RequiresMigration  bool                  `json:"requires_migration"`
	TotalAffectedItems int                   `json:"total_affected_items"`
}

// StatusMigrationMapping represents a status migration mapping
type StatusMigrationMapping struct {
	FromStatus   string `json:"from_status"`
	FromStatusID int    `json:"from_status_id"`
	ToStatusID   int    `json:"to_status_id"`
	ItemTypeID   *int   `json:"item_type_id,omitempty"`
	ItemCount    int    `json:"item_count"`
}

// WorkflowMigrationRequest represents a workflow migration request
type WorkflowMigrationRequest struct {
	ConfigurationSetID int                      `json:"configuration_set_id"`
	WorkspaceIDs       []int                    `json:"workspace_ids"`
	StatusMappings     []StatusMigrationMapping `json:"status_mappings"`
}

// Comprehensive Configuration Set Migration Models

// ItemTypeMigrationInfo describes an item type that needs migration
type ItemTypeMigrationInfo struct {
	CurrentItemTypeID     *int             `json:"current_item_type_id"`
	CurrentItemTypeName   string           `json:"current_item_type_name"`
	ItemCount             int              `json:"item_count"`
	RequiresMigration     bool             `json:"requires_migration"`
	SuggestedItemTypeID   *int             `json:"suggested_item_type_id,omitempty"`
	SuggestedItemTypeName string           `json:"suggested_item_type_name,omitempty"`
	AvailableTargets      []ItemTypeTarget `json:"available_targets,omitempty"`
}

// ItemTypeTarget represents an available target item type for migration
type ItemTypeTarget struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	HierarchyLevel int    `json:"hierarchy_level"`
}

// CustomFieldMigrationInfo describes a custom field migration need
type CustomFieldMigrationInfo struct {
	FieldID         int    `json:"field_id"`
	FieldName       string `json:"field_name"`
	FieldType       string `json:"field_type"`
	ItemCount       int    `json:"item_count"`       // items with non-null value for this field
	Action          string `json:"action"`           // keep, orphan, add_default
	RequiresDefault bool   `json:"requires_default"` // new required field needs default value
}

// PriorityMigrationInfo describes a priority that needs migration
type PriorityMigrationInfo struct {
	CurrentPriorityID     *int   `json:"current_priority_id"`
	CurrentPriorityName   string `json:"current_priority_name"`
	ItemCount             int    `json:"item_count"`
	RequiresMigration     bool   `json:"requires_migration"`
	SuggestedPriorityID   *int   `json:"suggested_priority_id,omitempty"`
	SuggestedPriorityName string `json:"suggested_priority_name,omitempty"`
}

// PriorityTarget represents an available target priority for migration
type PriorityTarget struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

// ComprehensiveMigrationAnalysis is the full analysis response for config set migration
type ComprehensiveMigrationAnalysis struct {
	// Existing status migration fields (backward compatible)
	StatusMigrations []StatusMigrationInfo `json:"status_migrations"`
	NewWorkflowID    *int                  `json:"new_workflow_id"`
	NewWorkflowName  string                `json:"new_workflow_name"`

	// New dimensions
	ItemTypeMigrations    []ItemTypeMigrationInfo    `json:"item_type_migrations"`
	CustomFieldMigrations []CustomFieldMigrationInfo `json:"custom_field_migrations"`
	PriorityMigrations    []PriorityMigrationInfo    `json:"priority_migrations"`

	// Available targets for UI dropdowns
	AvailableItemTypes  []ItemTypeTarget `json:"available_item_types"`
	AvailablePriorities []PriorityTarget `json:"available_priorities"`

	// Context
	OldConfigSetID     int    `json:"old_config_set_id"`
	OldConfigSetName   string `json:"old_config_set_name"`
	NewConfigSetID     int    `json:"new_config_set_id"`
	NewConfigSetName   string `json:"new_config_set_name"`
	AffectedWorkspaces []int  `json:"affected_workspaces"`
	TotalAffectedItems int    `json:"total_affected_items"`

	// Flags
	RequiresMigration         bool `json:"requires_migration"`
	RequiresItemTypeMigration bool `json:"requires_item_type_migration"`
	RequiresFieldMigration    bool `json:"requires_field_migration"`
	RequiresStatusMigration   bool `json:"requires_status_migration"`
	RequiresPriorityMigration bool `json:"requires_priority_migration"`
}

// ItemTypeMigrationMapping maps old item type to new
type ItemTypeMigrationMapping struct {
	FromItemTypeID *int `json:"from_item_type_id"` // nil = items with no type
	ToItemTypeID   int  `json:"to_item_type_id"`
}

// CustomFieldMigrationMapping specifies how to handle a custom field
type CustomFieldMigrationMapping struct {
	FieldID      int         `json:"field_id"`
	Action       string      `json:"action"`                  // keep, orphan, add_default
	DefaultValue interface{} `json:"default_value,omitempty"` // for new required fields
}

// PriorityMigrationMapping maps old priority to new
type PriorityMigrationMapping struct {
	FromPriorityID *int `json:"from_priority_id"` // nil = items with no priority
	ToPriorityID   int  `json:"to_priority_id"`
}

// ComprehensiveMigrationRequest is the full migration execution request
type ComprehensiveMigrationRequest struct {
	OldConfigurationSetID int   `json:"old_configuration_set_id"`
	NewConfigurationSetID int   `json:"new_configuration_set_id"`
	WorkspaceIDs          []int `json:"workspace_ids"`

	StatusMappings      []StatusMigrationMapping      `json:"status_mappings"`
	ItemTypeMappings    []ItemTypeMigrationMapping    `json:"item_type_mappings"`
	CustomFieldMappings []CustomFieldMigrationMapping `json:"custom_field_mappings"`
	PriorityMappings    []PriorityMigrationMapping    `json:"priority_mappings"`
}
