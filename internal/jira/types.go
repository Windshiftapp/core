// Package jira provides a client for the Jira Cloud and Data Center REST APIs
// for importing projects, issues, workflows, and assets into Windshift.
package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DeploymentType represents the Jira deployment type
type DeploymentType string

const (
	DeploymentCloud      DeploymentType = "cloud"
	DeploymentDataCenter DeploymentType = "datacenter"
)

// JiraInstanceInfo contains information about the connected Jira instance
type JiraInstanceInfo struct {
	CloudID     string   `json:"cloud_id"`
	DisplayName string   `json:"display_name"`
	URL         string   `json:"url"`
	Products    []string `json:"products"` // jira-software, jira-servicedesk, etc.
	Timezone    string   `json:"timezone"`
	Locale      string   `json:"locale"`
}

// JiraProject represents a Jira project
type JiraProject struct {
	ID          string            `json:"id"`
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	ProjectType string            `json:"projectTypeKey"` // software, service_desk, business
	AvatarURLs  map[string]string `json:"avatarUrls"`
	Simplified  bool              `json:"simplified"`
	Style       string            `json:"style"` // classic or next-gen
}

// JiraServiceDesk identifies a Jira Service Management portal and its backing
// Jira project.
type JiraServiceDesk struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	ProjectKey  string `json:"projectKey"`
}

// JiraServiceDeskRequestType is a customer-facing request type exposed in a
// Jira Service Management portal.
type JiraServiceDeskRequestType struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	HelpText          string   `json:"helpText"`
	IssueTypeID       string   `json:"issueTypeId"`
	ServiceDeskID     string   `json:"serviceDeskId"`
	PortalID          string   `json:"portalId"`
	GroupIDs          []string `json:"groupIds"`
	RestrictionStatus string   `json:"restrictionStatus"`
}

// JiraServiceDeskOrganization is a customer organization associated with a
// Jira Service Management service desk.
type JiraServiceDeskOrganization struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	UUID        string `json:"uuid"`
	SCIMManaged bool   `json:"scimManaged"`
}

// JiraServiceDeskPage is the common paginated envelope used by the JSM API.
type JiraServiceDeskPage[T any] struct {
	Size       int  `json:"size"`
	Start      int  `json:"start"`
	Limit      int  `json:"limit"`
	IsLastPage bool `json:"isLastPage"`
	Values     []T  `json:"values"`
}

// JiraServiceDeskComment carries the JSM-specific visibility metadata that is
// absent from Jira Platform issue comment payloads.
type JiraServiceDeskComment struct {
	ID     string `json:"id"`
	Public bool   `json:"public"`
}

// JiraIssueType represents a Jira issue type
type JiraIssueType struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	IconURL        string `json:"iconUrl"`
	Subtask        bool   `json:"subtask"`
	HierarchyLevel int    `json:"hierarchyLevel"` // -1=subtask, 0=base, 1=epic
}

// JiraIssueTypeWithStatuses represents a Jira issue type with its available statuses
type JiraIssueTypeWithStatuses struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Subtask  bool         `json:"subtask"`
	Statuses []JiraStatus `json:"statuses"`
}

// JiraCustomField represents a Jira custom field definition
type JiraCustomField struct {
	ID          string           `json:"id"`  // e.g., "customfield_10001"
	Key         string           `json:"key"` // e.g., "com.atlassian.jira.plugin.system.customfieldtypes:textfield"
	Name        string           `json:"name"`
	Description string           `json:"description"`
	FieldType   string           `json:"type"` // Custom field type identifier
	Schema      *JiraFieldSchema `json:"schema"`
	Custom      bool             `json:"custom"`
}

// JiraFieldSchema describes the data type of a field
type JiraFieldSchema struct {
	Type     string `json:"type"`     // string, number, array, option, user, etc.
	Items    string `json:"items"`    // For arrays, the type of items
	System   string `json:"system"`   // System field identifier if applicable
	Custom   string `json:"custom"`   // Custom field type key
	CustomID int    `json:"customId"` // Numeric custom field ID
}

// CustomFieldConfigurationClient is an optional Jira Cloud capability for
// reading the configuration behind a custom field. Jira Data Center does not
// expose an equivalent stable REST contract, and Cloud credentials commonly
// lack the required Jira-admin permission, so callers must treat
// ErrCustomFieldConfigurationNotAvailable as a fidelity finding rather than a
// fatal import error.
type CustomFieldConfigurationClient interface {
	GetCustomFieldConfiguration(
		ctx context.Context,
		fieldID string,
		includeOptions bool,
	) (*JiraCustomFieldConfiguration, error)
}

// JiraCustomFieldConfiguration preserves Jira's context-dependent field
// contract. Windshift custom fields are global, so importers union configured
// options for editability and retain the source contexts/defaults as
// provenance instead of pretending the narrower Jira applicability is
// enforced locally.
type JiraCustomFieldConfiguration struct {
	FieldID                   string                   `json:"field_id"`
	Contexts                  []JiraCustomFieldContext `json:"contexts"`
	DefaultsUnavailableReason string                   `json:"defaults_unavailable_reason,omitempty"`
}

// JiraCustomFieldContext is one Jira field context with its project and issue
// type applicability, configured options, and defaults.
type JiraCustomFieldContext struct {
	ID                       string                         `json:"id"`
	Name                     string                         `json:"name"`
	Description              string                         `json:"description,omitempty"`
	IsGlobal                 bool                           `json:"is_global"`
	IsAnyIssueType           bool                           `json:"is_any_issue_type"`
	ProjectIDs               []string                       `json:"project_ids,omitempty"`
	IssueTypeIDs             []string                       `json:"issue_type_ids,omitempty"`
	Options                  []JiraCustomFieldContextOption `json:"options,omitempty"`
	OptionsUnavailableReason string                         `json:"options_unavailable_reason,omitempty"`
	Defaults                 []JiraCustomFieldDefaultValue  `json:"defaults,omitempty"`
}

// JiraCustomFieldContextOption is a configured option. ParentOptionID is set
// for cascading children; Disabled is retained even though Windshift currently
// has no disabled-choice state.
type JiraCustomFieldContextOption struct {
	ID             string `json:"id"`
	Value          string `json:"value"`
	ParentOptionID string `json:"parent_option_id,omitempty"`
	Disabled       bool   `json:"disabled,omitempty"`
}

// JiraCustomFieldDefaultValue is intentionally polymorphic. Jira emits
// different value objects for text, numbers, users, versions, Forge fields,
// and choices; preserving Value losslessly keeps that provenance useful even
// when Windshift has no global custom-field default model.
type JiraCustomFieldDefaultValue struct {
	IssueTypeID    string `json:"issue_type_id,omitempty"`
	IsAnyIssueType bool   `json:"is_any_issue_type"`
	Value          any    `json:"value"`
}

// JiraStatus represents a Jira status
type JiraStatus struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	IconURL        string              `json:"iconUrl"`
	StatusCategory *JiraStatusCategory `json:"statusCategory"`
}

// JiraStatusCategory represents a Jira status category
type JiraStatusCategory struct {
	ID        int    `json:"id"`
	Key       string `json:"key"`  // new, indeterminate, done
	Name      string `json:"name"` // To Do, In Progress, Done
	ColorName string `json:"colorName"`
}

// JiraWorkflow represents a Jira workflow
type JiraWorkflow struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Statuses    []JiraStatus             `json:"statuses"`
	Transitions []JiraWorkflowTransition `json:"transitions"`
}

// JiraWorkflowTransition represents a transition in a workflow
type JiraWorkflowTransition struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	FromStatus *JiraStatus `json:"from"`
	ToStatus   *JiraStatus `json:"to"`
}

// WorkflowConfigurationClient is an optional Jira client capability. Jira
// Cloud exposes the configured workflow graph through a read-only bulk API;
// older Jira products and limited-permission credentials may only expose
// status membership through Client.GetProjectIssueTypeStatuses.
type WorkflowConfigurationClient interface {
	GetProjectWorkflowConfiguration(
		ctx context.Context,
		projectID string,
		issueTypeIDs []string,
	) (*JiraProjectWorkflowConfiguration, error)
}

// JiraProjectWorkflowConfiguration contains the authoritative workflow chosen
// by Jira for each requested project/issue-type pair.
type JiraProjectWorkflowConfiguration struct {
	IssueTypeWorkflowIDs map[string]string
	Workflows            []JiraWorkflowConfiguration
	// RulesComplete is true only when the source API proves that every
	// configured condition/validator is represented in Transitions. Jira
	// Cloud's current bulk workflow read omits condition trees, so its value is
	// false and rules that are not exposed are omitted from the imported
	// condition sets.
	RulesComplete bool
}

// JiraWorkflowConfiguration is a source workflow definition. StatusIDs and
// transition endpoints contain Jira status IDs, not Windshift database IDs.
type JiraWorkflowConfiguration struct {
	ID          string
	Name        string
	Description string
	StatusIDs   []string
	Transitions []JiraConfiguredWorkflowTransition
}

// JiraConfiguredWorkflowTransitionType is the topology Jira assigns to a
// transition. Global transitions are expanded into explicit directed edges
// during import because Windshift stores directed and initial transitions.
type JiraConfiguredWorkflowTransitionType string

const (
	JiraWorkflowTransitionInitial  JiraConfiguredWorkflowTransitionType = "INITIAL"
	JiraWorkflowTransitionDirected JiraConfiguredWorkflowTransitionType = "DIRECTED"
	JiraWorkflowTransitionGlobal   JiraConfiguredWorkflowTransitionType = "GLOBAL"
)

// JiraConfiguredWorkflowTransition describes a transition returned by Jira's
// workflow configuration API. Rule counts are retained so unsupported guarded
// transitions can be reported and handled conservatively by the importer.
type JiraConfiguredWorkflowTransition struct {
	ID             string
	Name           string
	Description    string
	Type           JiraConfiguredWorkflowTransitionType
	FromStatusIDs  []string
	ToStatusID     string
	ValidatorCount int
	ActionCount    int
	TriggerCount   int
	ConditionCount int
}

// ScreenConfigurationClient is an optional Jira client capability for
// company-managed issue-type screen schemes and their configured fields.
type ScreenConfigurationClient interface {
	GetProjectScreenConfiguration(
		ctx context.Context,
		projectID string,
		projectKey string,
		issueTypeIDs []string,
	) (*JiraProjectScreenConfiguration, error)
}

// JiraProjectScreenConfiguration contains effective create/edit/view screens
// per Jira issue type plus the deduplicated source screen definitions.
type JiraProjectScreenConfiguration struct {
	IssueTypeScreens map[string]JiraIssueTypeScreens
	Screens          []JiraScreenConfiguration
}

// JiraIssueTypeScreens contains effective Jira screen IDs after applying both
// the issue-type default and the operation-level default screen.
type JiraIssueTypeScreens struct {
	CreateScreenID string
	EditScreenID   string
	ViewScreenID   string
}

// JiraScreenConfiguration is a Jira screen with tabs flattened in source
// order. Windshift does not have a tab model, so TabCount is retained as
// fidelity metadata.
type JiraScreenConfiguration struct {
	ID          string
	Name        string
	Description string
	TabCount    int
	Fields      []JiraScreenField
}

// JiraScreenField is one ordered field in a Jira screen.
type JiraScreenField struct {
	ID   string
	Name string
}

// JiraIssue represents a Jira issue
type JiraIssue struct {
	ID             string          `json:"id"`
	Key            string          `json:"key"`
	Self           string          `json:"self"`
	Fields         JiraIssueFields `json:"fields"`
	Changelog      *JiraChangelog  `json:"changelog,omitempty"`
	Renderedfields map[string]any  `json:"renderedFields,omitempty"`
}

// JiraIssueFields contains the fields of a Jira issue
type JiraIssueFields struct {
	Summary      string                  `json:"summary"`
	Description  any                     `json:"description"` // Can be string or ADF
	IssueType    *JiraIssueType          `json:"issuetype"`
	Project      *JiraProject            `json:"project"`
	Status       *JiraStatus             `json:"status"`
	Priority     *JiraPriority           `json:"priority"`
	Assignee     *JiraUser               `json:"assignee"`
	Reporter     *JiraUser               `json:"reporter"`
	Creator      *JiraUser               `json:"creator"`
	Created      string                  `json:"created"`
	Updated      string                  `json:"updated"`
	Resolved     string                  `json:"resolutiondate"`
	DueDate      string                  `json:"duedate"`
	Labels       []string                `json:"labels"`
	Components   []JiraComponent         `json:"components"`
	FixVersions  []JiraVersion           `json:"fixVersions"`
	Versions     []JiraVersion           `json:"versions"` // Affects versions
	Parent       *JiraIssue              `json:"parent"`
	Subtasks     []JiraIssue             `json:"subtasks"`
	IssueLinks   []JiraIssueLink         `json:"issuelinks"`
	Attachment   []JiraAttachment        `json:"attachment"`
	Comment      *JiraCommentContainer   `json:"comment"`
	Worklog      *JiraWorklogContainer   `json:"worklog"`
	TimeTracking *JiraTimeTracking       `json:"timetracking"`
	Watches      *JiraWatchSummary       `json:"watches"`
	Votes        *JiraVoteSummary        `json:"votes"`
	Security     *JiraIssueSecurityLevel `json:"security"`
	Sprint       any                     `json:"sprint"` // Can be object or customfield
	Epic         *JiraIssue              `json:"epic"`   // Epic link for stories
	CustomFields map[string]any          `json:"-"`      // Populated separately
	// Watchers is populated from the paged issue-watchers endpoint. The issue
	// payload contains only Watches.Count and never the identities needed for
	// first-class Windshift item_watches rows.
	Watchers                   []JiraUser `json:"-"`
	WatcherIdentitiesAvailable bool       `json:"-"`
	WatcherFetchError          string     `json:"-"`
}

// IssueWatchersClient is the optional per-issue watcher capability shared by
// Jira Cloud and Data Center. Jira can withhold watcher identities even when
// it exposes a count; callers preserve that distinction in import metadata.
type IssueWatchersClient interface {
	GetIssueWatchers(ctx context.Context, issueKey string) (*JiraIssueWatchers, error)
}

type JiraWatchSummary struct {
	Self       string `json:"self"`
	WatchCount int    `json:"watchCount"`
	IsWatching bool   `json:"isWatching"`
}

type JiraIssueWatchers struct {
	Self       string     `json:"self"`
	WatchCount int        `json:"watchCount"`
	IsWatching bool       `json:"isWatching"`
	Watchers   []JiraUser `json:"watchers"`
}

type JiraVoteSummary struct {
	Self     string     `json:"self"`
	Votes    int        `json:"votes"`
	HasVoted bool       `json:"hasVoted"`
	Voters   []JiraUser `json:"voters,omitempty"`
}

type JiraIssueSecurityLevel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Self        string `json:"self"`
}

// UnmarshalJSON implements custom unmarshalling for JiraIssueFields.
// Standard fields are decoded normally. Any key starting with "customfield_"
// is captured into the CustomFields map, which the default json:"-" tag
// would otherwise leave empty.
func (f *JiraIssueFields) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type Alias JiraIssueFields
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(f),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Now decode the raw JSON again to pick up custom fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	f.CustomFields = make(map[string]any)
	for key, val := range raw {
		if strings.HasPrefix(key, "customfield_") {
			var v any
			if err := json.Unmarshal(val, &v); err == nil {
				f.CustomFields[key] = v
			}
		}
	}

	return nil
}

// JiraPriority represents a Jira priority
type JiraPriority struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IconURL string `json:"iconUrl"`
}

// JiraUser represents a Jira user
// Cloud uses AccountID as the unique identifier
// Data Center uses Name or Key as the unique identifier
type JiraUser struct {
	AccountID    string            `json:"accountId"` // Cloud identifier
	AccountType  string            `json:"accountType"`
	Name         string            `json:"name"` // Data Center identifier (username)
	Key          string            `json:"key"`  // Data Center identifier (user key)
	EmailAddress string            `json:"emailAddress"`
	DisplayName  string            `json:"displayName"`
	Active       bool              `json:"active"`
	TimeZone     string            `json:"timeZone"`
	AvatarURLs   map[string]string `json:"avatarUrls"`
}

// GetIdentifier returns the appropriate unique identifier for the user
// based on what's available (Cloud uses AccountID, Data Center uses Name or Key)
func (u *JiraUser) GetIdentifier() string {
	if u.AccountID != "" {
		return u.AccountID
	}
	if u.Name != "" {
		return u.Name
	}
	return u.Key
}

// JiraComponent represents a Jira project component
type JiraComponent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// JiraVersion represents a Jira version/release
type JiraVersion struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Archived    bool   `json:"archived"`
	Released    bool   `json:"released"`
	ReleaseDate string `json:"releaseDate"`
	StartDate   string `json:"startDate"`
}

// JiraSprint represents a Jira sprint (from Agile API)
type JiraSprint struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	State         string `json:"state"` // future, active, closed
	StartDate     string `json:"startDate"`
	EndDate       string `json:"endDate"`
	CompleteDate  string `json:"completeDate"`
	OriginBoardID int    `json:"originBoardId"`
	Goal          string `json:"goal"`
}

// JiraIssueLink represents a link between two issues
type JiraIssueLink struct {
	ID           string        `json:"id"`
	Type         *JiraLinkType `json:"type"`
	InwardIssue  *JiraIssue    `json:"inwardIssue"`
	OutwardIssue *JiraIssue    `json:"outwardIssue"`
}

// JiraLinkType represents a link type between issues
type JiraLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

// JiraAttachment represents a file attachment
type JiraAttachment struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	Author    *JiraUser `json:"author"`
	Created   string    `json:"created"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mimeType"`
	Content   string    `json:"content"` // URL to download
	Thumbnail string    `json:"thumbnail"`
}

// JiraCommentContainer holds comments with pagination info
type JiraCommentContainer struct {
	Comments   []JiraComment `json:"comments"`
	MaxResults int           `json:"maxResults"`
	Total      int           `json:"total"`
	StartAt    int           `json:"startAt"`
}

// JiraComment represents a comment on an issue
type JiraComment struct {
	ID           string    `json:"id"`
	Author       *JiraUser `json:"author"`
	Body         any       `json:"body"` // Can be string or ADF
	Created      string    `json:"created"`
	Updated      string    `json:"updated"`
	UpdateAuthor *JiraUser `json:"updateAuthor"`
	// ServiceDeskPublic is populated from the JSM request comment endpoint.
	// A false value means an agent-only internal note.
	ServiceDeskPublic *bool `json:"-"`
	// Visibility scopes a comment to a role/group. Jira Cloud emits
	// {"type":"group","value":"..."} (or "role"); Data Center uses the same
	// shape. Windshift only models a private/internal toggle, so any
	// restricted comment is imported as private and the original scope is
	// preserved in the comment mapping metadata.
	Visibility *JiraCommentVisibility `json:"visibility"`
}

// JiraCommentVisibility carries the restriction on a Jira comment.
type JiraCommentVisibility struct {
	Type  string `json:"type"`  // "group" or "role"
	Value string `json:"value"` // role or group name
}

// MediaAttachment is the minimal reference to an imported Windshift
// attachment that the ADF media resolver needs: the Windshift attachment id,
// its MIME type, and the original filename. Jira's ADF `media` nodes carry the
// Jira attachment id as `attrs.id`, so a map keyed by that id lets the
// converter link the node to the imported attachment.
type MediaAttachment struct {
	ID               int
	MimeType         string
	OriginalFilename string
}

// JiraWorklogContainer holds worklogs with pagination info
type JiraWorklogContainer struct {
	Worklogs   []JiraWorklog `json:"worklogs"`
	MaxResults int           `json:"maxResults"`
	Total      int           `json:"total"`
	StartAt    int           `json:"startAt"`
}

// JiraWorklog represents a worklog entry
type JiraWorklog struct {
	ID               string    `json:"id"`
	Author           *JiraUser `json:"author"`
	Comment          any       `json:"comment"` // Can be string or ADF
	Created          string    `json:"created"`
	Updated          string    `json:"updated"`
	Started          string    `json:"started"`
	TimeSpent        string    `json:"timeSpent"`
	TimeSpentSeconds int       `json:"timeSpentSeconds"`
}

// JiraTimeTracking represents time tracking info
type JiraTimeTracking struct {
	OriginalEstimate         string `json:"originalEstimate"`
	RemainingEstimate        string `json:"remainingEstimate"`
	TimeSpent                string `json:"timeSpent"`
	OriginalEstimateSeconds  int    `json:"originalEstimateSeconds"`
	RemainingEstimateSeconds int    `json:"remainingEstimateSeconds"`
	TimeSpentSeconds         int    `json:"timeSpentSeconds"`
}

// JiraChangelog contains issue change history
type JiraChangelog struct {
	Histories  []JiraChangeHistory `json:"histories"`
	MaxResults int                 `json:"maxResults"`
	Total      int                 `json:"total"`
	StartAt    int                 `json:"startAt"`
}

// JiraChangeHistory represents a change in issue history
type JiraChangeHistory struct {
	ID      string           `json:"id"`
	Author  *JiraUser        `json:"author"`
	Created string           `json:"created"`
	Items   []JiraChangeItem `json:"items"`
}

// JiraChangeItem represents a single field change
type JiraChangeItem struct {
	Field      string `json:"field"`
	FieldType  string `json:"fieldtype"`
	FieldID    string `json:"fieldId"`
	From       string `json:"from"`
	FromString string `json:"fromString"`
	To         string `json:"to"`
	ToString   string `json:"toString"`
}

// SearchResult represents the result of a JQL search
type SearchResult struct {
	Expand     string      `json:"expand"`
	StartAt    int         `json:"startAt"`
	MaxResults int         `json:"maxResults"`
	Total      int         `json:"total"`
	Issues     []JiraIssue `json:"issues"`
}

// SearchOptions contains options for searching issues
type SearchOptions struct {
	JQL        string   `json:"jql"`
	StartAt    int      `json:"startAt"`
	MaxResults int      `json:"maxResults"`
	Fields     []string `json:"fields"`
	Expand     []string `json:"expand"`
}

// ================================================================
// Jira Assets (Insight) Types
// ================================================================

// AssetTime decodes Assets timestamps, which arrive as epoch-millisecond
// numbers on Cloud and as ISO date-time strings (or null) on Data Center.
type AssetTime time.Time

func (t *AssetTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*t = AssetTime(time.Time{})
		return nil
	}
	var millis int64
	if err := json.Unmarshal(data, &millis); err == nil {
		*t = AssetTime(time.UnixMilli(millis))
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("asset timestamp is neither epoch millis nor a string: %w", err)
	}
	// Cloud sends RFC 3339; Data Center uses Java-style offsets (+0000).
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05-0700"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			*t = AssetTime(parsed)
			return nil
		}
	}
	return fmt.Errorf("unrecognized asset timestamp format: %q", text)
}

// AssetObjectSchema represents a Jira Assets object schema
type AssetObjectSchema struct {
	ID              cloudFlexibleID `json:"id"`
	Name            string          `json:"name"`
	ObjectSchemaKey string          `json:"objectSchemaKey"`
	Description     string          `json:"description"`
	Created         AssetTime       `json:"created"`
	Updated         AssetTime       `json:"updated"`
	ObjectCount     int             `json:"objectCount"`
	ObjectTypeCount int             `json:"objectTypeCount"`
}

// AssetObjectType represents an object type within a schema
type AssetObjectType struct {
	ID                 cloudFlexibleID        `json:"id"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	Icon               *AssetIcon             `json:"icon"`
	Position           int                    `json:"position"`
	Created            AssetTime              `json:"created"`
	Updated            AssetTime              `json:"updated"`
	ObjectCount        int                    `json:"objectCount"`
	ObjectSchemaID     cloudFlexibleID        `json:"objectSchemaId"`
	Inherited          bool                   `json:"inherited"`
	AbstractObjectType bool                   `json:"abstractObjectType"`
	ParentObjectTypeID cloudFlexibleID        `json:"parentObjectTypeId,omitempty"`
	Attributes         []AssetObjectAttribute `json:"attributes,omitempty"`
}

// AssetIcon represents an icon for an object type
type AssetIcon struct {
	ID    cloudFlexibleID `json:"id"`
	Name  string          `json:"name"`
	URL16 string          `json:"url16"`
	URL48 string          `json:"url48"`
}

// AssetObjectAttribute represents an attribute definition for an object type
type AssetObjectAttribute struct {
	ID                  cloudFlexibleID   `json:"id"`
	Name                string            `json:"name"`
	Label               bool              `json:"label"`
	Type                int               `json:"type"` // 0=Default, 1=ObjectRef, 2=User, 3=Confluence, etc.
	TypeValue           cloudFlexibleID   `json:"typeValue,omitempty"`
	DefaultTypeID       int               `json:"defaultTypeId,omitempty"` // For type=0: 0=Text, 1=Integer, 2=Boolean, etc.
	DefaultType         *AssetDefaultType `json:"defaultType,omitempty"`
	Description         string            `json:"description"`
	Editable            bool              `json:"editable"`
	Hidden              bool              `json:"hidden"`
	IncludeChildObjects bool              `json:"includeChildObjectTypes"`
	UniqueAttribute     bool              `json:"uniqueAttribute"`
	MinimumCardinality  int               `json:"minimumCardinality"`
	MaximumCardinality  int               `json:"maximumCardinality"`
	Removable           bool              `json:"removable"`
	Position            int               `json:"position"`
}

type AssetDefaultType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// AssetObject represents an object instance in Assets
type AssetObject struct {
	ID           cloudFlexibleID             `json:"id"`
	Label        string                      `json:"label"`
	ObjectKey    string                      `json:"objectKey"`
	ObjectType   *AssetObjectType            `json:"objectType"`
	Created      AssetTime                   `json:"created"`
	Updated      AssetTime                   `json:"updated"`
	HasAvatar    bool                        `json:"hasAvatar"`
	Timestamp    int64                       `json:"timestamp"`
	Attributes   []AssetObjectAttributeValue `json:"attributes"`
	ExtendedInfo *AssetExtendedInfo          `json:"extendedInfo,omitempty"`
	Links        *AssetObjectLinks           `json:"links,omitempty"`
}

// AssetObjectAttributeValue represents an attribute value on an object
type AssetObjectAttributeValue struct {
	ID                    cloudFlexibleID       `json:"id"`
	ObjectTypeAttributeID cloudFlexibleID       `json:"objectTypeAttributeId"`
	ObjectAttributeValues []AssetAttributeValue `json:"objectAttributeValues"`
}

// AssetAttributeValue represents a single value for an attribute
type AssetAttributeValue struct {
	Value        any    `json:"value"`
	DisplayValue string `json:"displayValue"`
	SearchValue  string `json:"searchValue"`
	// Jira Cloud currently returns a boolean here, while older Assets
	// responses used a numeric reference type. The importer does not depend on
	// this metadata, so retain either representation without rejecting the
	// containing asset.
	ReferencedType any          `json:"referencedType,omitempty"`
	User           *JiraUser    `json:"user,omitempty"`
	Status         *AssetStatus `json:"status,omitempty"`
}

// AssetStatus represents a status in Assets
type AssetStatus struct {
	ID          cloudFlexibleID `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    int             `json:"category"` // 0=Inactive, 1=Active, 2=Pending
}

// AssetExtendedInfo contains additional object info
type AssetExtendedInfo struct {
	OpenIssuesExists  bool `json:"openIssuesExists"`
	AttachmentsExists bool `json:"attachmentsExists"`
}

// AssetObjectLinks contains links related to the object
type AssetObjectLinks struct {
	Self string `json:"self"`
}

// ObjectSearchOptions contains options for searching assets
type ObjectSearchOptions struct {
	ObjectSchemaID string `json:"objectSchemaId"`
	ObjectTypeID   string `json:"objectTypeId,omitempty"`
	IQL            string `json:"iql,omitempty"` // Insight Query Language
	// StartAt is an explicit row offset. It takes precedence over Page so
	// callers can advance by the rows a server-capped page actually returned
	// instead of by the requested page size.
	StartAt           int  `json:"startAt,omitempty"`
	Page              int  `json:"page"`
	PageSize          int  `json:"pageSize"`
	IncludeAttributes bool `json:"includeAttributes"`
}

// ObjectSearchResult represents the result of an object search
type ObjectSearchResult struct {
	ObjectEntries        []AssetObject          `json:"objectEntries"`
	ObjectTypeAttributes []AssetObjectAttribute `json:"objectTypeAttributes,omitempty"`
	PageNumber           int                    `json:"pageNumber"`
	PageSize             int                    `json:"pageSize"`
	TotalFilterCount     int                    `json:"totalFilterCount"`
	StartIndex           int                    `json:"startIndex"`
	ToIndex              int                    `json:"toIndex"`
	IsLast               bool                   `json:"isLast"`
}

// ================================================================
// Jira Filter Types
// ================================================================

// JiraFilter represents a saved Jira filter.
type JiraFilter struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	JQL         string    `json:"jql"`
	ViewURL     string    `json:"viewUrl"`
	Owner       *JiraUser `json:"owner,omitempty"`
	Self        string    `json:"self"`
}

// FilterSearchResult represents paged Jira saved filter search results.
type FilterSearchResult struct {
	MaxResults int          `json:"maxResults"`
	StartAt    int          `json:"startAt"`
	Total      int          `json:"total"`
	IsLast     bool         `json:"isLast"`
	Values     []JiraFilter `json:"values"`
	// Partial marks an enumeration that cannot list every saved filter, such as
	// Data Center where only favorites are discoverable.
	Partial       bool   `json:"partial,omitempty"`
	PartialReason string `json:"partial_reason,omitempty"`
}

// ================================================================
// Jira Agile Types (Boards, Sprints)
// ================================================================

// JiraBoard represents a Jira Agile board
type JiraBoard struct {
	ID       int                `json:"id"`
	Name     string             `json:"name"`
	Type     string             `json:"type"` // scrum, kanban
	Location *JiraBoardLocation `json:"location"`
}

// JiraBoardLocation represents the project location of a board
type JiraBoardLocation struct {
	ProjectID   int    `json:"projectId"`
	DisplayName string `json:"displayName"`
	ProjectName string `json:"projectName"`
	ProjectKey  string `json:"projectKey"`
}

// BoardListResult represents paginated board results
type BoardListResult struct {
	MaxResults int         `json:"maxResults"`
	StartAt    int         `json:"startAt"`
	Total      int         `json:"total"`
	IsLast     bool        `json:"isLast"`
	Values     []JiraBoard `json:"values"`
}

// SprintListResult represents paginated sprint results
type SprintListResult struct {
	MaxResults int          `json:"maxResults"`
	StartAt    int          `json:"startAt"`
	Total      int          `json:"total"`
	IsLast     bool         `json:"isLast"`
	Values     []JiraSprint `json:"values"`
}

// JiraBoardConfiguration represents Agile board configuration details.
type JiraBoardConfiguration struct {
	ID           int                    `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	Filter       *JiraBoardFilter       `json:"filter,omitempty"`
	Location     *JiraBoardLocation     `json:"location,omitempty"`
	ColumnConfig *JiraBoardColumnConfig `json:"columnConfig,omitempty"`
	SubQuery     *JiraBoardSubQuery     `json:"subQuery,omitempty"`
	Ranking      map[string]any         `json:"ranking,omitempty"`
}

// JiraBoardFilter is the saved filter backing a board. The agile API
// serializes the filter id as a number on some deployments (WI-1156), so the
// ID decodes flexibly like the other Jira ids.
type JiraBoardFilter struct {
	ID   cloudFlexibleID `json:"id"`
	Self string          `json:"self"`
	Name string          `json:"name,omitempty"`
	JQL  string          `json:"jql,omitempty"`
}

// JiraBoardSubQuery preserves the board sub-query, if Jira exposes it.
type JiraBoardSubQuery struct {
	Query string `json:"query"`
}

// JiraBoardColumnConfig contains board columns and their status mapping.
type JiraBoardColumnConfig struct {
	Columns        []JiraBoardConfigColumn `json:"columns"`
	ConstraintType string                  `json:"constraintType,omitempty"`
}

// JiraBoardConfigColumn represents a Jira board column.
type JiraBoardConfigColumn struct {
	Name     string                  `json:"name"`
	Statuses []JiraBoardColumnStatus `json:"statuses"`
	Min      *int                    `json:"min,omitempty"`
	Max      *int                    `json:"max,omitempty"`
}

// JiraBoardColumnStatus is a Jira status assigned to a board column.
type JiraBoardColumnStatus struct {
	ID   string `json:"id"`
	Self string `json:"self"`
}

// ================================================================
// Enhanced JQL Search Types (POST /rest/api/3/search/jql)
// ================================================================

// JQLSearchRequest is the request body for POST /rest/api/3/search/jql
type JQLSearchRequest struct {
	JQL           string   `json:"jql"`
	MaxResults    int      `json:"maxResults,omitempty"`
	Fields        []string `json:"fields,omitempty"`
	Expand        []string `json:"expand,omitempty"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
}

// MarshalJSON serializes expand as the comma-delimited string
// SearchAndReconcileRequestBean expects. Bulk fetch keeps its array shape.
func (r JQLSearchRequest) MarshalJSON() ([]byte, error) {
	type alias JQLSearchRequest
	return json.Marshal(struct {
		alias
		Expand string `json:"expand,omitempty"`
	}{
		alias:  alias(r),
		Expand: strings.Join(r.Expand, ","),
	})
}

// JQLSearchResponse is the response from POST /rest/api/3/search/jql
type JQLSearchResponse struct {
	Issues        []JiraIssue `json:"issues"`
	NextPageToken string      `json:"nextPageToken,omitempty"`
	Total         int         `json:"total,omitempty"` // May not be returned in new API
}

// UserEmailResponse is the response from GET /rest/api/3/user/email
// Used to fetch user emails separately since Cloud omits them from issue responses
type UserEmailResponse struct {
	AccountID string `json:"accountId"`
	Email     string `json:"email"`
}

// ================================================================
// Issue Bulk Fetch Types (POST /rest/api/3/issue/bulkfetch)
// ================================================================

// BulkFetchRequest is the request body for POST /rest/api/3/issue/bulkfetch
type BulkFetchRequest struct {
	IssueIdsOrKeys []string `json:"issueIdsOrKeys"`
	Fields         []string `json:"fields,omitempty"`
	Expand         []string `json:"expand,omitempty"`
	Properties     []string `json:"properties,omitempty"`
}

// BulkFetchResponse is the response from POST /rest/api/3/issue/bulkfetch.
// Jira answers HTTP 200 with partial success: resolved issues land in Issues
// and per-issue failures in IssueErrors.
type BulkFetchResponse struct {
	Issues      []JiraIssue      `json:"issues"`
	IssueErrors []BulkFetchError `json:"issueErrors,omitempty"`
}

// BulkFetchError represents a per-issue failure in a bulk-fetch response. The
// published Cloud IssueError shape is {id, errorMessage}; the id is the numeric
// issue id, not the issue key. The legacy issueIdOrKey/errorMessages/errors
// fields are decoded defensively for older deployments.
type BulkFetchError struct {
	ID            string            `json:"id,omitempty"`
	ErrorMessage  string            `json:"errorMessage,omitempty"`
	IssueIDOrKey  string            `json:"issueIdOrKey,omitempty"`
	ErrorMessages []string          `json:"errorMessages,omitempty"`
	Errors        map[string]string `json:"errors,omitempty"`
}

// Message renders the per-issue failure for logs and error accounting.
func (e BulkFetchError) Message() string {
	if message := strings.TrimSpace(e.ErrorMessage); message != "" {
		return message
	}
	if len(e.ErrorMessages) > 0 {
		return strings.Join(e.ErrorMessages, "; ")
	}
	if len(e.Errors) > 0 {
		parts := make([]string, 0, len(e.Errors))
		for field, message := range e.Errors {
			parts = append(parts, field+": "+message)
		}
		sort.Strings(parts)
		return strings.Join(parts, "; ")
	}
	return "unknown Jira bulk fetch error"
}

// JiraIssueRef is a stable Jira issue identity: the numeric id plus the
// human-readable key. Discovery returns both so bulk-fetch IssueError ids can
// be correlated to requested keys without guessing.
type JiraIssueRef struct {
	ID  string
	Key string
}
