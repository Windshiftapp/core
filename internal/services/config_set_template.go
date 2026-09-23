package services

import "time"

// Versioned `windshift.configuration-set` export/import types. References use
// names or emails, never IDs: shared entities are reused or created, bundle
// entities are created fresh, and missing identities return ErrUnresolvedReferences.

const (
	ConfigSetTemplateSchemaVersion = 1
	ConfigSetTemplateKind          = "windshift.configuration-set"
)

// ConfigSetTemplate is the top-level envelope.
type ConfigSetTemplate struct {
	SchemaVersion int                 `json:"schema_version"`
	Kind          string              `json:"kind"`
	ExportedAt    time.Time           `json:"exported_at"`
	ExportedBy    *ConfigSetExportBy  `json:"exported_by,omitempty"`
	Payload       ConfigSetTplPayload `json:"payload"`
}

// ConfigSetExportBy carries non-sensitive provenance metadata.
type ConfigSetExportBy struct {
	Username string `json:"username,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// ConfigSetTplPayload order matches importer creation dependencies.
type ConfigSetTplPayload struct {
	ConfigurationSet ConfigSetTplConfigSet      `json:"configuration_set"`
	StatusCategories []ConfigSetTplStatusCat    `json:"status_categories,omitempty"`
	CustomFields     []ConfigSetTplCustomField  `json:"custom_fields,omitempty"`
	Statuses         []ConfigSetTplStatus       `json:"statuses,omitempty"`
	ItemTypes        []ConfigSetTplItemType     `json:"item_types,omitempty"`
	Priorities       []ConfigSetTplPriority     `json:"priorities,omitempty"`
	LinkTypes        []ConfigSetTplLinkType     `json:"link_types,omitempty"`
	Screens          []ConfigSetTplScreen       `json:"screens,omitempty"`
	Workflows        []ConfigSetTplWorkflow     `json:"workflows,omitempty"`
	ConditionSets    []ConfigSetTplConditionSet `json:"condition_sets,omitempty"`
	ApprovalSets     []ConfigSetTplApprovalSet  `json:"approval_sets,omitempty"`
	Links            ConfigSetTplLinks          `json:"configuration_set_links"`
}

type ConfigSetTplConfigSet struct {
	Name                    string `json:"name"`
	Description             string `json:"description,omitempty"`
	DifferentiateByItemType bool   `json:"differentiate_by_item_type,omitempty"`
	DefaultItemTypeName     string `json:"default_item_type_name,omitempty"`
}

// ConfigSetTplStatusCat names a required system-seeded status category.
type ConfigSetTplStatusCat struct {
	Name string `json:"name"`
}

// ConfigSetTplCustomField is matched by name and imported non-system-default.
type ConfigSetTplCustomField struct {
	Name                           string `json:"name"`
	FieldType                      string `json:"field_type"`
	Description                    string `json:"description,omitempty"`
	Required                       bool   `json:"required"`
	Options                        string `json:"options,omitempty"`
	DisplayOrder                   int    `json:"display_order"`
	AppliesToPortalCustomers       bool   `json:"applies_to_portal_customers,omitempty"`
	AppliesToCustomerOrganisations bool   `json:"applies_to_customer_organisations,omitempty"` //nolint:misspell // matches DB column
}

type ConfigSetTplStatus struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	CategoryName string `json:"category_name"`
}

type ConfigSetTplItemType struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Icon           string `json:"icon,omitempty"`
	Color          string `json:"color,omitempty"`
	HierarchyLevel int    `json:"hierarchy_level"`
	SortOrder      int    `json:"sort_order"`
}

type ConfigSetTplPriority struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
	SortOrder   int    `json:"sort_order"`
}

// ConfigSetTplLinkType carries one link type's semantic definition. The
// active flag and system marker are instance-local state, not part of the
// portable definition: import always creates active non-system rows, and
// verification that required types are present and active belongs to the
// conformance check, not the template.
type ConfigSetTplLinkType struct {
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	ForwardLabel       string   `json:"forward_label"`
	ReverseLabel       string   `json:"reverse_label"`
	Color              string   `json:"color,omitempty"`
	AllowedEntityTypes []string `json:"allowed_entity_types,omitempty"`
}

type ConfigSetTplScreen struct {
	Name         string                    `json:"name"`
	Description  string                    `json:"description,omitempty"`
	Fields       []ConfigSetTplScreenField `json:"fields"`
	SystemFields []string                  `json:"system_fields,omitempty"`
}

// ConfigSetTplScreenField represents one field on a screen. FieldKind matches
// the DB's screen_fields.field_type column ("system", "default", "custom").
// For "custom", the bundle stores CustomFieldName; the importer rewrites it
// to the resolved custom_field_definitions.id at apply time.
type ConfigSetTplScreenField struct {
	FieldKind       string `json:"field_kind"`
	FieldIdentifier string `json:"field_identifier,omitempty"`
	CustomFieldName string `json:"custom_field_name,omitempty"`
	DisplayOrder    int    `json:"display_order"`
	IsRequired      bool   `json:"is_required,omitempty"`
	FieldWidth      string `json:"field_width,omitempty"`
}

type ConfigSetTplWorkflow struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description,omitempty"`
	Transitions []ConfigSetTplWorkflowTransition `json:"transitions,omitempty"`
}

// ConfigSetTplWorkflowTransition carries from/to status names. FromStatusName
// is a pointer-string: nil means "initial transition" (from_status_id IS
// NULL) unless FromAllStatuses marks a transition from every other status.
type ConfigSetTplWorkflowTransition struct {
	FromStatusName  *string `json:"from_status_name"`
	ToStatusName    string  `json:"to_status_name"`
	FromAllStatuses bool    `json:"from_all_statuses,omitempty"`
	DisplayOrder    int     `json:"display_order,omitempty"`
	SourceHandle    string  `json:"source_handle,omitempty"`
	TargetHandle    string  `json:"target_handle,omitempty"`
}

type ConfigSetTplConditionSet struct {
	Name                 string                            `json:"name"`
	Description          string                            `json:"description,omitempty"`
	WorkflowName         string                            `json:"workflow_name"`
	TransitionConditions []ConfigSetTplTransitionCondition `json:"transition_conditions,omitempty"`
}

type ConfigSetTplTransitionCondition struct {
	FromStatusName  *string                 `json:"from_status_name"`
	ToStatusName    string                  `json:"to_status_name"`
	FromAllStatuses bool                    `json:"from_all_statuses,omitempty"`
	LogicMode       string                  `json:"logic_mode"`
	Conditions      []ConfigSetTplCondition `json:"conditions,omitempty"`
}

// ConfigSetTplCondition keeps the rule type and config as a free-form object.
// On export, the service rewrites known integer references inside Config:
//   - role_id   → role_name
//   - group_id  → group_name
//   - field_id  → custom_field_name (when source=='custom_field')
//
// Importer reverses these substitutions.
type ConfigSetTplCondition struct {
	Type         string         `json:"type"`
	Mode         string         `json:"mode"`
	ErrorMessage string         `json:"error_message,omitempty"`
	DisplayOrder int            `json:"display_order,omitempty"`
	Config       map[string]any `json:"config"`
}

type ConfigSetTplApprovalSet struct {
	Name         string                          `json:"name"`
	Description  string                          `json:"description,omitempty"`
	WorkflowName string                          `json:"workflow_name"`
	SetStatuses  []ConfigSetTplApprovalSetStatus `json:"set_statuses,omitempty"`
}

type ConfigSetTplApprovalSetStatus struct {
	StatusName        string                     `json:"status_name"`
	StepMode          string                     `json:"step_mode"`
	ApproveTransition ConfigSetTplTransitionRef  `json:"approve_transition"`
	DenyTransition    ConfigSetTplTransitionRef  `json:"deny_transition"`
	Steps             []ConfigSetTplApprovalStep `json:"steps,omitempty"`
}

type ConfigSetTplTransitionRef struct {
	FromStatusName  *string `json:"from_status_name"`
	ToStatusName    string  `json:"to_status_name"`
	FromAllStatuses bool    `json:"from_all_statuses,omitempty"`
}

// ConfigSetTplApprovalStep mirrors models.ApprovalStep but swaps every
// integer ID-by-source for a name (or email, for users).
type ConfigSetTplApprovalStep struct {
	DisplayOrder    int    `json:"display_order"`
	Name            string `json:"name,omitempty"`
	QuorumMode      string `json:"quorum_mode"`
	QuorumCount     *int   `json:"quorum_count,omitempty"`
	QuorumPercent   *int   `json:"quorum_percent,omitempty"`
	RejectionPolicy string `json:"rejection_policy"`

	ApproverSource          string `json:"approver_source"`
	ApproverFieldIdentifier string `json:"approver_field_identifier,omitempty"`
	ApproverCustomFieldName string `json:"approver_custom_field_name,omitempty"`
	ApproverRoleName        string `json:"approver_role_name,omitempty"`
	ApproverGroupName       string `json:"approver_group_name,omitempty"`
	ApproverUserEmail       string `json:"approver_user_email,omitempty"`
	AllowSelfApproval       bool   `json:"allow_self_approval,omitempty"`

	OnLeaveStrategy string `json:"on_leave_strategy"`

	EscalationAfterHours            *int   `json:"escalation_after_hours,omitempty"`
	EscalationAction                string `json:"escalation_action,omitempty"`
	EscalationTargetSource          string `json:"escalation_target_source,omitempty"`
	EscalationTargetFieldIdentifier string `json:"escalation_target_field_identifier,omitempty"`
	EscalationTargetCustomFieldName string `json:"escalation_target_custom_field_name,omitempty"`
	EscalationTargetRoleName        string `json:"escalation_target_role_name,omitempty"`
	EscalationTargetGroupName       string `json:"escalation_target_group_name,omitempty"`
	EscalationTargetUserEmail       string `json:"escalation_target_user_email,omitempty"`
	MaxEscalations                  *int   `json:"max_escalations,omitempty"`
}

// ConfigSetTplLinks captures how the configuration set glues everything
// together: which workflow/condition-set/approval-set it points at, which
// priorities and item types it lists, and any per-item-type overrides.
type ConfigSetTplLinks struct {
	WorkflowName     string                       `json:"workflow_name,omitempty"`
	ConditionSetName string                       `json:"condition_set_name,omitempty"`
	ApprovalSetName  string                       `json:"approval_set_name,omitempty"`
	CreateScreenName string                       `json:"create_screen_name,omitempty"`
	EditScreenName   string                       `json:"edit_screen_name,omitempty"`
	ViewScreenName   string                       `json:"view_screen_name,omitempty"`
	PriorityNames    []string                     `json:"priority_names,omitempty"`
	ItemTypeConfigs  []ConfigSetTplItemTypeConfig `json:"item_type_configs,omitempty"`
}

// ConfigSetTplItemTypeConfig captures per-item-type overrides. Empty strings
// mean "use the configuration set default" (i.e. NULL on the DB row).
type ConfigSetTplItemTypeConfig struct {
	ItemTypeName     string `json:"item_type_name"`
	WorkflowName     string `json:"workflow_name,omitempty"`
	ConditionSetName string `json:"condition_set_name,omitempty"`
	ApprovalSetName  string `json:"approval_set_name,omitempty"`
	CreateScreenName string `json:"create_screen_name,omitempty"`
	EditScreenName   string `json:"edit_screen_name,omitempty"`
	ViewScreenName   string `json:"view_screen_name,omitempty"`
}

// UnresolvedRefKind enumerates identity references that the importer refuses
// to auto-create. Templates may reference them, but the target instance must
// already have them by name (or by email, for users).
type UnresolvedRefKind string

const (
	UnresolvedKindStatusCategory UnresolvedRefKind = "status_category"
	UnresolvedKindRole           UnresolvedRefKind = "role"
	UnresolvedKindGroup          UnresolvedRefKind = "group"
	UnresolvedKindUser           UnresolvedRefKind = "user"
)

// UnresolvedRef is a single missing reference. Path is a coarse human-readable
// breadcrumb so an admin reviewing a 422 response can locate the offending
// row in the source bundle.
type UnresolvedRef struct {
	Kind  UnresolvedRefKind `json:"kind"`
	Name  string            `json:"name,omitempty"`
	Email string            `json:"email,omitempty"`
	Path  string            `json:"at"`
}

// ErrUnresolvedReferences is returned by ImportConfigSet when one or more
// required identity references (roles, groups, users, status categories) are
// not present on the target instance. Import has not written anything when
// this is returned.
type ErrUnresolvedReferences struct {
	Items []UnresolvedRef
}

func (e *ErrUnresolvedReferences) Error() string {
	if len(e.Items) == 1 {
		return "configuration set import: 1 unresolved reference"
	}
	return "configuration set import: unresolved references"
}

// DefaultConflictKind enumerates which entity types the importer refuses to
// shadow with a duplicate-named, non-default copy. Statuses / item types /
// priorities / custom fields are excluded — those are reused-by-name and
// cannot be duplicated by import.
type DefaultConflictKind string

const (
	DefaultConflictConfigurationSet DefaultConflictKind = "configuration_set"
	DefaultConflictWorkflow         DefaultConflictKind = "workflow"
)

// DefaultConflict points at one entity that already exists on the target as
// a default-flagged row, blocking import because the bundle would otherwise
// create a confusing same-named non-default sibling.
type DefaultConflict struct {
	Kind DefaultConflictKind `json:"kind"`
	Name string              `json:"name"`
}

// ErrDefaultEntityConflict is returned by ImportConfigSet when the bundle
// would create a non-default entity whose name collides with a default. No
// writes have happened.
type ErrDefaultEntityConflict struct {
	Conflicts []DefaultConflict
}

func (e *ErrDefaultEntityConflict) Error() string {
	return "configuration set import: bundle conflicts with default-flagged entities on the target"
}

// LinkTypeConflict records one template link type whose name collides with
// an existing row in the target's global registry whose definition differs.
// Both definitions are echoed so the admin can decide which side to rename.
type LinkTypeConflict struct {
	Name     string               `json:"name"`
	Template ConfigSetTplLinkType `json:"template"`
	Existing ConfigSetTplLinkType `json:"existing"`
}

// ErrLinkTypeDefinitionConflict is returned by ImportConfigSet before any
// write when a template link type collides with a same-named registry row
// carrying a different definition. Import never overwrites existing link
// types and never duplicates identical ones.
type ErrLinkTypeDefinitionConflict struct {
	Conflicts []LinkTypeConflict
}

func (e *ErrLinkTypeDefinitionConflict) Error() string {
	if len(e.Conflicts) == 1 {
		return "configuration set import: 1 link type definition conflict"
	}
	return "configuration set import: link type definition conflicts"
}

// ErrCannotExportDefault is returned by ConfigSetExportService.Export when
// the target configuration set has is_default=true. The default config set
// is intentionally not portable; users are expected to clone it first.
var ErrCannotExportDefault = &cannotExportDefaultError{}

type cannotExportDefaultError struct{}

func (e *cannotExportDefaultError) Error() string {
	return "configuration set export: the default configuration set cannot be exported"
}
