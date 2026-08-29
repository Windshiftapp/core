package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// HierarchyLevelGenericSubtask is the sentinel used by item types that may be
// attached below any regular hierarchy level. Generic subtasks are terminal
// leaves: they require a parent and cannot themselves parent another item.
const HierarchyLevelGenericSubtask = -1

// ConfigurationSet represents a configuration set for workspaces
type ConfigurationSet struct {
	ID                      int       `json:"id"`
	BuiltinKey              string    `json:"builtin_key,omitempty"`
	WorkspaceID             int       `json:"workspace_id"` // Keep for backward compatibility
	Name                    string    `json:"name"`
	Description             string    `json:"description"`
	IsDefault               bool      `json:"is_default"`
	DifferentiateByItemType bool      `json:"differentiate_by_item_type"`
	WorkflowID              *int      `json:"workflow_id,omitempty"`
	NotificationSettingID   *int      `json:"notification_setting_id,omitempty"`
	ConditionSetID          *int      `json:"condition_set_id,omitempty"`
	ApprovalSetID           *int      `json:"approval_set_id,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	// Joined fields for API responses
	WorkspaceName                 string `json:"workspace_name,omitempty"`
	WorkflowName                  string `json:"workflow_name,omitempty"`
	WorkflowBuiltinKey            string `json:"workflow_builtin_key,omitempty"`
	NotificationSettingName       string `json:"notification_setting_name,omitempty"`
	NotificationSettingBuiltinKey string `json:"notification_setting_builtin_key,omitempty"`
	ConditionSetName              string `json:"condition_set_name,omitempty"`
	ApprovalSetName               string `json:"approval_set_name,omitempty"`
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
	CreateScreenID         *int   `json:"create_screen_id,omitempty"`
	EditScreenID           *int   `json:"edit_screen_id,omitempty"`
	ViewScreenID           *int   `json:"view_screen_id,omitempty"`
	CreateScreenName       string `json:"create_screen_name,omitempty"`
	CreateScreenBuiltinKey string `json:"create_screen_builtin_key,omitempty"`
	EditScreenName         string `json:"edit_screen_name,omitempty"`
	EditScreenBuiltinKey   string `json:"edit_screen_builtin_key,omitempty"`
	ViewScreenName         string `json:"view_screen_name,omitempty"`
	ViewScreenBuiltinKey   string `json:"view_screen_builtin_key,omitempty"`
	// Default item type for new items (when user has no localStorage preference)
	DefaultItemTypeID         *int   `json:"default_item_type_id,omitempty"`
	DefaultItemTypeName       string `json:"default_item_type_name,omitempty"`
	DefaultItemTypeBuiltinKey string `json:"default_item_type_builtin_key,omitempty"`
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
	BuiltinKey  string    `json:"builtin_key,omitempty"`
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
	FieldName   string         `json:"field_name,omitempty"`
	FieldLabel  string         `json:"field_label,omitempty"`
	FieldConfig map[string]any `json:"field_config,omitempty"`
}

// ItemType represents a type of work item
type ItemType struct {
	ID                 int       `json:"id"`
	BuiltinKey         string    `json:"builtin_key,omitempty"`
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
	ConfigurationSetIDs         []int    `json:"configuration_set_ids,omitempty"`          // IDs of associated configuration sets
	ConfigurationSetNames       []string `json:"configuration_set_names,omitempty"`        // Names for display
	ConfigurationSetBuiltinKeys []string `json:"configuration_set_builtin_keys,omitempty"` // Stable keys for built-in configuration sets
	// Deprecated joined fields (kept for backward compatibility)
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`
	WorkspaceName        string `json:"workspace_name,omitempty"`
}

// ItemTypeDisplay holds minimal item type data for displaying in configuration sets
type ItemTypeDisplay struct {
	BuiltinKey     string `json:"builtin_key,omitempty"`
	Name           string `json:"name"`
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	HierarchyLevel int    `json:"hierarchy_level"`
}

// ItemTypeConfig represents item type configuration with optional workflow and screen overrides
type ItemTypeConfig struct {
	ItemTypeID         int    `json:"item_type_id"`
	ItemTypeBuiltinKey string `json:"item_type_builtin_key,omitempty"`
	ItemTypeName       string `json:"item_type_name"`
	ItemTypeIcon       string `json:"item_type_icon"`
	ItemTypeColor      string `json:"item_type_color"`
	HierarchyLevel     int    `json:"hierarchy_level"`
	// Override workflow (NULL = use configuration set default)
	WorkflowID         *int   `json:"workflow_id,omitempty"`
	WorkflowName       string `json:"workflow_name,omitempty"` // "Default" or workflow name
	WorkflowBuiltinKey string `json:"workflow_builtin_key,omitempty"`
	// Override condition set (NULL = use configuration set default)
	ConditionSetID   *int   `json:"condition_set_id,omitempty"`
	ConditionSetName string `json:"condition_set_name,omitempty"`
	// Override approval set (NULL = use configuration set default)
	ApprovalSetID   *int   `json:"approval_set_id,omitempty"`
	ApprovalSetName string `json:"approval_set_name,omitempty"`
	// Override screens (NULL = use configuration set defaults)
	CreateScreenID         *int   `json:"create_screen_id,omitempty"`
	CreateScreenName       string `json:"create_screen_name,omitempty"`
	CreateScreenBuiltinKey string `json:"create_screen_builtin_key,omitempty"`
	EditScreenID           *int   `json:"edit_screen_id,omitempty"`
	EditScreenName         string `json:"edit_screen_name,omitempty"`
	EditScreenBuiltinKey   string `json:"edit_screen_builtin_key,omitempty"`
	ViewScreenID           *int   `json:"view_screen_id,omitempty"`
	ViewScreenName         string `json:"view_screen_name,omitempty"`
	ViewScreenBuiltinKey   string `json:"view_screen_builtin_key,omitempty"`
}

// Priority represents a priority level
type Priority struct {
	ID          int       `json:"id"`
	BuiltinKey  string    `json:"builtin_key,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	Icon        string    `json:"icon"`       // Lucide icon name
	Color       string    `json:"color"`      // Hex color for background
	SortOrder   int       `json:"sort_order"` // For ordering priorities
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Many-to-many configuration set relationships
	ConfigurationSetIDs         []int    `json:"configuration_set_ids,omitempty"`          // IDs of associated configuration sets
	ConfigurationSetNames       []string `json:"configuration_set_names,omitempty"`        // Names for display
	ConfigurationSetBuiltinKeys []string `json:"configuration_set_builtin_keys,omitempty"` // Stable keys for built-in configuration sets
}

// PriorityDisplay holds minimal priority data for displaying in configuration sets
type PriorityDisplay struct {
	ID         int    `json:"id"`
	BuiltinKey string `json:"builtin_key,omitempty"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
	SortOrder  int    `json:"sort_order"`
}

// HierarchyLevel represents a hierarchy level definition
type HierarchyLevel struct {
	ID          int       `json:"id"`
	BuiltinKey  string    `json:"builtin_key,omitempty"`
	Level       int       `json:"level"` // 0, 1, 2, 3...
	Name        string    `json:"name"`  // e.g., "Initiative", "Epic", "Task", "Sub-task"
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StatusCategory represents a category for statuses
type StatusCategory struct {
	ID          int       `json:"id"`
	BuiltinKey  string    `json:"builtin_key,omitempty"`
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
	BuiltinKey  string    `json:"builtin_key,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CategoryID  int       `json:"category_id"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	CategoryName       string `json:"category_name,omitempty"`
	CategoryBuiltinKey string `json:"category_builtin_key,omitempty"`
	CategoryColor      string `json:"category_color,omitempty"`
	IsCompleted        bool   `json:"is_completed,omitempty"`
}

// Workflow represents a workflow definition
type Workflow struct {
	ID          int       `json:"id"`
	BuiltinKey  string    `json:"builtin_key,omitempty"`
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
	ID              int       `json:"id"`
	WorkflowID      int       `json:"workflow_id"`
	FromStatusID    *int      `json:"from_status_id"` // NULL means it's an initial status
	ToStatusID      int       `json:"to_status_id"`
	FromAllStatuses bool      `json:"from_all_statuses"` // TRUE = allowed from every other status; from_status_id stays NULL
	DisplayOrder    int       `json:"display_order"`
	SourceHandle    string    `json:"source_handle,omitempty"` // Connection point on source status (top, right, bottom, left)
	TargetHandle    string    `json:"target_handle,omitempty"` // Connection point on target status (top, right, bottom, left)
	CreatedAt       time.Time `json:"created_at"`
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

const (
	// CustomFieldTypeBoolean is the canonical field type used by the asset
	// subsystem and every surface that consumes global custom fields.
	CustomFieldTypeBoolean = "boolean"
	// CustomFieldTypeCheckbox is retained as a compatibility alias for
	// definitions written by older clients and prerelease WI-891 builds.
	CustomFieldTypeCheckbox = "checkbox"
)

// IsBooleanCustomFieldType reports whether a definition uses either the
// canonical asset type or its legacy UI-oriented alias.
func IsBooleanCustomFieldType(fieldType string) bool {
	return fieldType == CustomFieldTypeBoolean || fieldType == CustomFieldTypeCheckbox
}

// CanonicalCustomFieldType maps compatibility aliases to the asset subsystem's
// canonical type before definitions are persisted or compared for reuse.
func CanonicalCustomFieldType(fieldType string) string {
	if IsBooleanCustomFieldType(fieldType) {
		return CustomFieldTypeBoolean
	}
	return fieldType
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

// StatusMigrationMapping represents a status migration mapping.
// FromStatusID is a pointer because an item with no status (status_id IS NULL)
// must be addressable distinctly from status_id == 0 — older clients that send
// 0 are treated as nil (NULL) for backwards compatibility.
type StatusMigrationMapping struct {
	FromStatus   string `json:"from_status"`
	FromStatusID *int   `json:"from_status_id"`
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
	AvailableTargets      []ItemTypeTarget `json:"available_targets"`
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
	FieldID      int    `json:"field_id"`
	Action       string `json:"action"`                  // keep, orphan, add_default
	DefaultValue any    `json:"default_value,omitempty"` // for new required fields
}

// PriorityMigrationMapping maps old priority to new
type PriorityMigrationMapping struct {
	FromPriorityID *int `json:"from_priority_id"` // nil = items with no priority
	ToPriorityID   int  `json:"to_priority_id"`
}

// ComprehensiveMigrationRequest is the full migration execution request.
// When AttachAfterMigration is true, the server performs the workspace-to-
// configuration-set assignment swap inside the same transaction as the data
// migration. This closes the TOCTOU window between "migrate items" and
// "swap config set" — without it, items created concurrently can be left
// referencing the old workflow's statuses.
type ComprehensiveMigrationRequest struct {
	OldConfigurationSetID int   `json:"old_configuration_set_id"`
	NewConfigurationSetID int   `json:"new_configuration_set_id"`
	WorkspaceIDs          []int `json:"workspace_ids"`

	StatusMappings      []StatusMigrationMapping      `json:"status_mappings"`
	ItemTypeMappings    []ItemTypeMigrationMapping    `json:"item_type_mappings"`
	CustomFieldMappings []CustomFieldMigrationMapping `json:"custom_field_mappings"`
	PriorityMappings    []PriorityMigrationMapping    `json:"priority_mappings"`

	// AttachAfterMigration, when true, also performs the workspace_configuration_sets
	// swap (compare-and-swap from OldConfigurationSetID to NewConfigurationSetID)
	// for every WorkspaceID in the same transaction.
	AttachAfterMigration bool `json:"attach_after_migration,omitempty"`

	// ApplyWorkflowToConfigSet, when non-nil, atomically updates
	// configuration_sets.workflow_id = ? WHERE id = NewConfigurationSetID
	// inside the migration transaction. Used for intra-set workflow changes:
	// the PUT-update endpoint detects the change and returns 409, the FE runs
	// the migration assistant, and the assistant calls execute with this field
	// set so that "migrate items to the new workflow's statuses" and "switch
	// the config set to that workflow" happen as a single atomic step.
	// When set, status mappings are validated against this workflow rather
	// than the workflow currently persisted on the config set.
	ApplyWorkflowToConfigSet *int `json:"apply_workflow_to_config_set,omitempty"`

	// ApplyItemTypeConfigsToConfigSet, when non-nil, replaces the target
	// configuration set's item-type allow-list in the same transaction as the
	// item migrations. This is the intra-set item-type-removal path: it avoids
	// a gap in which newly-created items could continue using a removed type
	// between migration execution and the follow-up configuration-set update.
	ApplyItemTypeConfigsToConfigSet *[]ItemTypeConfig `json:"apply_item_type_configs_to_config_set,omitempty"`
}

// SelectOption represents a single option in a select/multiselect custom field
type SelectOption struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

// SelectFieldOptions represents the ID-based options format for select/multiselect fields
type SelectFieldOptions struct {
	NextID int            `json:"next_id"`
	Items  []SelectOption `json:"items"`
}

// ParseSelectOptions parses the options JSON string for select/multiselect fields.
// It handles both the new format {"next_id": N, "items": [...]} and the legacy format ["A", "B", "C"].
// Legacy format is converted to the new format with sequential IDs starting at 1.
func ParseSelectOptions(optionsJSON string) (*SelectFieldOptions, error) {
	if optionsJSON == "" {
		return &SelectFieldOptions{NextID: 1, Items: []SelectOption{}}, nil
	}

	optionsJSON = strings.TrimSpace(optionsJSON)

	// Try new format first
	if strings.HasPrefix(optionsJSON, "{") {
		var opts SelectFieldOptions
		if err := json.Unmarshal([]byte(optionsJSON), &opts); err == nil && opts.NextID > 0 {
			return &opts, nil
		}
	}

	// Try legacy string array format
	if strings.HasPrefix(optionsJSON, "[") {
		var legacy []string
		if err := json.Unmarshal([]byte(optionsJSON), &legacy); err != nil {
			return nil, fmt.Errorf("invalid options format: %w", err)
		}
		items := make([]SelectOption, len(legacy))
		for i, label := range legacy {
			items[i] = SelectOption{ID: i + 1, Label: label}
		}
		return &SelectFieldOptions{
			NextID: len(legacy) + 1,
			Items:  items,
		}, nil
	}

	return nil, fmt.Errorf("unrecognized options format")
}

// SerializeSelectOptions serializes SelectFieldOptions to a JSON string
func SerializeSelectOptions(opts *SelectFieldOptions) (string, error) {
	b, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ConditionSet represents a named bundle of conditions for workflow transitions
type ConditionSet struct {
	ID                   int                   `json:"id"`
	Name                 string                `json:"name"`
	Description          string                `json:"description"`
	WorkflowID           int                   `json:"workflow_id"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
	WorkflowName         string                `json:"workflow_name,omitempty"`
	TransitionConditions []TransitionCondition `json:"transition_conditions,omitempty"`
	// GatedTransitions is a lightweight per-transition summary populated by the
	// list endpoints so the condition-sets manager UI can render From → To
	// chips without a per-row detail fetch.
	GatedTransitions []ConditionSetTransitionSummary `json:"gated_transitions,omitempty"`
}

// ConditionSetTransitionSummary is a minimal transition descriptor for list
// responses: just enough to render a "From → To" lozenge in the manager UI.
// FromStatusName is empty for initial transitions (from_status_id IS NULL).
type ConditionSetTransitionSummary struct {
	TransitionID   int    `json:"transition_id"`
	FromStatusName string `json:"from_status_name,omitempty"`
	ToStatusName   string `json:"to_status_name"`
}

// TransitionCondition links a condition set to a specific transition with conditions
type TransitionCondition struct {
	ID             int         `json:"id"`
	ConditionSetID int         `json:"condition_set_id"`
	TransitionID   int         `json:"transition_id"`
	LogicMode      string      `json:"logic_mode"` // "and" | "or"
	Conditions     []Condition `json:"conditions,omitempty"`
	FromStatusName string      `json:"from_status_name,omitempty"`
	ToStatusName   string      `json:"to_status_name,omitempty"`
}

// Condition represents a single condition within a transition condition
type Condition struct {
	ID                       int             `json:"id"`
	ConditionSetTransitionID int             `json:"condition_set_transition_id"`
	ConditionType            string          `json:"condition_type"` // user_in_role, user_in_group, field_value, script
	Config                   json.RawMessage `json:"config"`
	DisplayOrder             int             `json:"display_order"`
	Mode                     string          `json:"mode"`                    // "condition" or "validator"
	ErrorMessage             string          `json:"error_message,omitempty"` // shown to user when condition fails (validator mode)
}

// Condition type constants
const (
	ConditionTypeUserInRole  = "user_in_role"
	ConditionTypeUserInGroup = "user_in_group"
	ConditionTypeFieldValue  = "field_value"
	ConditionTypeScript      = "script"
)

// Condition mode constants
const (
	ConditionModeCondition = "condition" // hides transition if rule fails
	ConditionModeValidator = "validator" // blocks transition with error message
)

// ConditionUserInRoleConfig is the config for user_in_role conditions.
// Source vocabulary is shared with approvals via the embedded FieldRef:
// 'current_user' | 'creator' | 'assignee' | 'regular_field' | 'custom_field'.
// Older rows used 'field' to mean custom_field; a one-shot bootstrap migration
// rewrites those to the new vocabulary.
type ConditionUserInRoleConfig struct {
	FieldRef
	RoleID int `json:"role_id"`
}

// ConditionUserInGroupConfig mirrors ConditionUserInRoleConfig for group membership.
type ConditionUserInGroupConfig struct {
	FieldRef
	GroupID int `json:"group_id"`
}

// ConditionFieldValueConfig is the config for field_value conditions
type ConditionFieldValueConfig struct {
	FieldIdentifier string `json:"field_identifier"`
	Pattern         string `json:"pattern"` // regex
}

// ConditionScriptConfig is the config for script conditions
type ConditionScriptConfig struct {
	Script    string `json:"script"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// Approvals open asynchronous requests on configured statuses. ApprovalService
// alone drives approve/deny transitions; leaving cancels and re-entry reopens.

// FieldRef is a generic reference to either a regular item field or a custom field.
// Used by approvals today and by conditions/validators after the migration in slice 10.
type FieldRef struct {
	Source          string `json:"source"`                     // 'creator'|'assignee'|'regular_field'|'custom_field'|'role'|'group'|'user'|'current_user'
	FieldIdentifier string `json:"field_identifier,omitempty"` // whitelisted column name when Source='regular_field'
	FieldID         *int   `json:"field_id,omitempty"`         // custom field id when Source='custom_field'
}

// ApprovalSet — a named bundle of approvals owned by a workflow. Mirrors ConditionSet.
type ApprovalSet struct {
	ID           int                 `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	WorkflowID   int                 `json:"workflow_id"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	WorkflowName string              `json:"workflow_name,omitempty"`
	SetStatuses  []ApprovalSetStatus `json:"set_statuses,omitempty"`
	// GatedStatuses is a lightweight summary of the active status gates on
	// this set, populated by the list endpoints so the manager UI can render
	// status chips without a per-row detail fetch.
	GatedStatuses []ApprovalSetStatusSummary `json:"gated_statuses,omitempty"`
}

// ApprovalSetStatusSummary is a minimal status descriptor for list responses:
// just enough to render a colored lozenge in the approval-sets manager.
type ApprovalSetStatusSummary struct {
	StatusID      int    `json:"status_id"`
	StatusName    string `json:"status_name"`
	CategoryColor string `json:"category_color,omitempty"`
}

// ApprovalSetStatus links an approval set to a specific status, plus the two
// transitions the approval engine drives (approve / deny).
type ApprovalSetStatus struct {
	ID                  int            `json:"id"`
	ApprovalSetID       int            `json:"approval_set_id"`
	StatusID            int            `json:"status_id"`
	ApproveTransitionID int            `json:"approve_transition_id"`
	DenyTransitionID    int            `json:"deny_transition_id"`
	StepMode            string         `json:"step_mode"` // 'sequential' | 'parallel'
	CreatedAt           time.Time      `json:"created_at"`
	StatusName          string         `json:"status_name,omitempty"`
	Steps               []ApprovalStep `json:"steps,omitempty"`
}

// ApprovalStep is a single step within an approval-set-status. The approver pool
// is resolved from approver_source (regular field, custom field, role, group, or user)
// at request time and snapshotted into approval_step_approvers; LeaveRepository is
// consulted unconditionally so on-leave substitutes are honored automatically.
type ApprovalStep struct {
	ID                  int    `json:"id"`
	ApprovalSetStatusID int    `json:"approval_set_status_id"`
	DisplayOrder        int    `json:"display_order"`
	Name                string `json:"name"`

	// Quorum
	QuorumMode      string `json:"quorum_mode"` // 'any'|'all'|'count'|'percent'
	QuorumCount     *int   `json:"quorum_count,omitempty"`
	QuorumPercent   *int   `json:"quorum_percent,omitempty"`
	RejectionPolicy string `json:"rejection_policy"` // 'any_rejection_fails'|'requires_quorum_to_fail'

	// Approver source (mirrors FieldRef vocabulary)
	ApproverSource          string `json:"approver_source"`
	ApproverFieldIdentifier string `json:"approver_field_identifier,omitempty"`
	ApproverFieldID         *int   `json:"approver_field_id,omitempty"`
	ApproverRoleID          *int   `json:"approver_role_id,omitempty"`
	ApproverGroupID         *int   `json:"approver_group_id,omitempty"`
	ApproverUserID          *int   `json:"approver_user_id,omitempty"`
	AllowSelfApproval       bool   `json:"allow_self_approval"`

	// On-leave override
	OnLeaveStrategy string `json:"on_leave_strategy"` // 'use_substitute'|'skip'|'keep'

	// Time-based escalation
	EscalationAfterHours            *int   `json:"escalation_after_hours,omitempty"`
	EscalationAction                string `json:"escalation_action,omitempty"` // 'reassign'|'skip_step'|'auto_reject'
	EscalationTargetSource          string `json:"escalation_target_source,omitempty"`
	EscalationTargetFieldIdentifier string `json:"escalation_target_field_identifier,omitempty"`
	EscalationTargetFieldID         *int   `json:"escalation_target_field_id,omitempty"`
	EscalationTargetRoleID          *int   `json:"escalation_target_role_id,omitempty"`
	EscalationTargetGroupID         *int   `json:"escalation_target_group_id,omitempty"`
	EscalationTargetUserID          *int   `json:"escalation_target_user_id,omitempty"`
	MaxEscalations                  *int   `json:"max_escalations,omitempty"` // NULL = unlimited

	CreatedAt time.Time `json:"created_at"`
}

// ApprovalRequest is the runtime instance — created when an item enters an
// approval-bound status. At most one open request per item (DB-enforced).
type ApprovalRequest struct {
	ID                  int                    `json:"id"`
	ItemID              int                    `json:"item_id"`
	WorkspaceID         int                    `json:"workspace_id"`
	ApprovalSetStatusID int                    `json:"approval_set_status_id"`
	StatusID            int                    `json:"status_id"`
	FromStatusID        *int                   `json:"from_status_id,omitempty"` // snapshot of prior status; revert target on cancel
	TriggeredByUserID   int                    `json:"triggered_by_user_id"`
	Status              string                 `json:"status"` // 'pending'|'approved'|'rejected'|'cancelled'
	CreatedAt           time.Time              `json:"created_at"`
	CompletedAt         *time.Time             `json:"completed_at,omitempty"`
	StepInstances       []ApprovalStepInstance `json:"step_instances,omitempty"`
	Decisions           []ApprovalDecision     `json:"decisions,omitempty"`
}

// ApprovalStepInstance is a per-step runtime row.
type ApprovalStepInstance struct {
	ID                int                    `json:"id"`
	ApprovalRequestID int                    `json:"approval_request_id"`
	ApprovalStepID    int                    `json:"approval_step_id"`
	DisplayOrder      int                    `json:"display_order"`
	Status            string                 `json:"status"` // 'pending'|'approved'|'rejected'|'skipped'|'escalated'
	EscalationDueAt   *time.Time             `json:"escalation_due_at,omitempty"`
	EscalationCount   int                    `json:"escalation_count"`
	LastEscalatedAt   *time.Time             `json:"last_escalated_at,omitempty"`
	StartedAt         *time.Time             `json:"started_at,omitempty"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	Approvers         []ApprovalStepApprover `json:"approvers,omitempty"`
}

// ApprovalStepApprover is one resolved approver in a step's snapshot. Exactly
// one of UserID / PortalCustomerID is set per row — the schema enforces this
// invariant via a CHECK constraint on fresh installs and at the application
// layer on existing installs (SQLite can't add CHECK to existing tables).
type ApprovalStepApprover struct {
	ID                     int       `json:"id"`
	ApprovalStepInstanceID int       `json:"approval_step_instance_id"`
	UserID                 *int      `json:"user_id,omitempty"`
	PortalCustomerID       *int      `json:"portal_customer_id,omitempty"`
	SourceRoleID           *int      `json:"source_role_id,omitempty"`
	SourceGroupID          *int      `json:"source_group_id,omitempty"`
	SubstitutedForUserID   *int      `json:"substituted_for_user_id,omitempty"`
	IsActive               bool      `json:"is_active"`
	CreatedAt              time.Time `json:"created_at"`
}

// ApprovalDecision is the auditable, append-only log row for an approval event.
// At most one of ActorUserID / ActorPortalCustomerID is set; both null means
// the actor is the system (e.g. sweeper-driven escalation).
type ApprovalDecision struct {
	ID                     int             `json:"id"`
	ApprovalRequestID      int             `json:"approval_request_id"`
	ApprovalStepInstanceID *int            `json:"approval_step_instance_id,omitempty"`
	ActorUserID            *int            `json:"actor_user_id,omitempty"`
	ActorPortalCustomerID  *int            `json:"actor_portal_customer_id,omitempty"`
	Decision               string          `json:"decision"`
	Comment                string          `json:"comment,omitempty"`
	DelegatedToUserID      *int            `json:"delegated_to_user_id,omitempty"`
	Metadata               json.RawMessage `json:"metadata,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
}

// Approval enum constants.
const (
	// Step modes
	ApprovalStepModeSequential = "sequential"
	ApprovalStepModeParallel   = "parallel"

	// Quorum modes
	ApprovalQuorumModeAny     = "any"
	ApprovalQuorumModeAll     = "all"
	ApprovalQuorumModeCount   = "count"
	ApprovalQuorumModePercent = "percent"

	// Rejection policies
	ApprovalRejectionPolicyAnyFails       = "any_rejection_fails"
	ApprovalRejectionPolicyQuorumRequired = "requires_quorum_to_fail"

	// On-leave strategies
	ApprovalOnLeaveUseSubstitute = "use_substitute"
	ApprovalOnLeaveSkip          = "skip"
	ApprovalOnLeaveKeep          = "keep"

	// Escalation actions
	ApprovalEscalationActionReassign   = "reassign"
	ApprovalEscalationActionSkipStep   = "skip_step"
	ApprovalEscalationActionAutoReject = "auto_reject"

	// Approver / target sources (shared with FieldRef.Source)
	ApprovalSourceCreator      = "creator"
	ApprovalSourceAssignee     = "assignee"
	ApprovalSourceRegularField = "regular_field"
	ApprovalSourceCustomField  = "custom_field"
	ApprovalSourceRole         = "role"
	ApprovalSourceGroup        = "group"
	ApprovalSourceUser         = "user"
	ApprovalSourceCurrentUser  = "current_user"

	// Request statuses
	ApprovalRequestStatusPending   = "pending"
	ApprovalRequestStatusApproved  = "approved"
	ApprovalRequestStatusRejected  = "rejected"
	ApprovalRequestStatusCancelled = "cancelled" //nolint:misspell // British spelling used in database

	// Step instance statuses
	ApprovalStepStatusPending   = "pending"
	ApprovalStepStatusApproved  = "approved"
	ApprovalStepStatusRejected  = "rejected"
	ApprovalStepStatusSkipped   = "skipped"
	ApprovalStepStatusEscalated = "escalated"

	// Decision types (audit log)
	ApprovalDecisionApprove    = "approve"
	ApprovalDecisionReject     = "reject"
	ApprovalDecisionComment    = "comment"
	ApprovalDecisionDelegate   = "delegate"
	ApprovalDecisionReassign   = "reassign"
	ApprovalDecisionCancel     = "cancel"
	ApprovalDecisionEscalate   = "escalate"
	ApprovalDecisionSubstitute = "substitute"
	ApprovalDecisionRequested  = "requested"
	ApprovalDecisionCompleted  = "completed"
)

// AllowedRegularApproverFields is the server-side whitelist of regular-field
// identifiers that can be used as approver_source='regular_field'. Extending
// this list is an explicit code change — never accept arbitrary client input.
var AllowedRegularApproverFields = map[string]struct{}{
	"assignee_id": {},
	"creator_id":  {},
	"reporter_id": {},
}
