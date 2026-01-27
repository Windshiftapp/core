// Package jira provides a client for the Jira Cloud and Data Center REST APIs
// for importing projects, issues, workflows, and assets into Windshift.
package jira

import (
	"encoding/json"
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

// JiraIssueType represents a Jira issue type
type JiraIssueType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"iconUrl"`
	Subtask     bool   `json:"subtask"`
	HierarchyLevel int `json:"hierarchyLevel"` // -1=subtask, 0=base, 1=epic
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
	ID          string              `json:"id"`   // e.g., "customfield_10001"
	Key         string              `json:"key"`  // e.g., "com.atlassian.jira.plugin.system.customfieldtypes:textfield"
	Name        string              `json:"name"`
	Description string              `json:"description"`
	FieldType   string              `json:"type"` // Custom field type identifier
	Schema      *JiraFieldSchema    `json:"schema"`
	Custom      bool                `json:"custom"`
}

// JiraFieldSchema describes the data type of a field
type JiraFieldSchema struct {
	Type     string `json:"type"`     // string, number, array, option, user, etc.
	Items    string `json:"items"`    // For arrays, the type of items
	System   string `json:"system"`   // System field identifier if applicable
	Custom   string `json:"custom"`   // Custom field type key
	CustomID int    `json:"customId"` // Numeric custom field ID
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
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Statuses    []JiraStatus          `json:"statuses"`
	Transitions []JiraWorkflowTransition `json:"transitions"`
}

// JiraWorkflowTransition represents a transition in a workflow
type JiraWorkflowTransition struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	FromStatus *JiraStatus `json:"from"`
	ToStatus  *JiraStatus `json:"to"`
}

// JiraIssue represents a Jira issue
type JiraIssue struct {
	ID         string                 `json:"id"`
	Key        string                 `json:"key"`
	Self       string                 `json:"self"`
	Fields     JiraIssueFields        `json:"fields"`
	Changelog  *JiraChangelog         `json:"changelog,omitempty"`
	Renderedfields map[string]interface{} `json:"renderedFields,omitempty"`
}

// JiraIssueFields contains the fields of a Jira issue
type JiraIssueFields struct {
	Summary       string                 `json:"summary"`
	Description   interface{}            `json:"description"` // Can be string or ADF
	IssueType     *JiraIssueType         `json:"issuetype"`
	Project       *JiraProject           `json:"project"`
	Status        *JiraStatus            `json:"status"`
	Priority      *JiraPriority          `json:"priority"`
	Assignee      *JiraUser              `json:"assignee"`
	Reporter      *JiraUser              `json:"reporter"`
	Creator       *JiraUser              `json:"creator"`
	Created       string                 `json:"created"`
	Updated       string                 `json:"updated"`
	Resolved      string                 `json:"resolutiondate"`
	DueDate       string                 `json:"duedate"`
	Labels        []string               `json:"labels"`
	Components    []JiraComponent        `json:"components"`
	FixVersions   []JiraVersion          `json:"fixVersions"`
	Versions      []JiraVersion          `json:"versions"` // Affects versions
	Parent        *JiraIssue             `json:"parent"`
	Subtasks      []JiraIssue            `json:"subtasks"`
	IssueLinks    []JiraIssueLink        `json:"issuelinks"`
	Attachment    []JiraAttachment       `json:"attachment"`
	Comment       *JiraCommentContainer  `json:"comment"`
	Worklog       *JiraWorklogContainer  `json:"worklog"`
	TimeTracking  *JiraTimeTracking      `json:"timetracking"`
	Sprint        interface{}            `json:"sprint"`  // Can be object or customfield
	Epic          *JiraIssue             `json:"epic"`    // Epic link for stories
	CustomFields  map[string]interface{} `json:"-"`       // Populated separately
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

	f.CustomFields = make(map[string]interface{})
	for key, val := range raw {
		if strings.HasPrefix(key, "customfield_") {
			var v interface{}
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
	AccountID    string            `json:"accountId"`    // Cloud identifier
	Name         string            `json:"name"`         // Data Center identifier (username)
	Key          string            `json:"key"`          // Data Center identifier (user key)
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
	ID           string          `json:"id"`
	Type         *JiraLinkType   `json:"type"`
	InwardIssue  *JiraIssue      `json:"inwardIssue"`
	OutwardIssue *JiraIssue      `json:"outwardIssue"`
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
	ID           string      `json:"id"`
	Author       *JiraUser   `json:"author"`
	Body         interface{} `json:"body"` // Can be string or ADF
	Created      string      `json:"created"`
	Updated      string      `json:"updated"`
	UpdateAuthor *JiraUser   `json:"updateAuthor"`
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
	ID               string      `json:"id"`
	Author           *JiraUser   `json:"author"`
	Comment          interface{} `json:"comment"` // Can be string or ADF
	Created          string      `json:"created"`
	Updated          string      `json:"updated"`
	Started          string      `json:"started"`
	TimeSpent        string      `json:"timeSpent"`
	TimeSpentSeconds int         `json:"timeSpentSeconds"`
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
	Expand     string       `json:"expand"`
	StartAt    int          `json:"startAt"`
	MaxResults int          `json:"maxResults"`
	Total      int          `json:"total"`
	Issues     []JiraIssue  `json:"issues"`
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

// AssetObjectSchema represents a Jira Assets object schema
type AssetObjectSchema struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	ObjectSchemaKey    string    `json:"objectSchemaKey"`
	Description        string    `json:"description"`
	Created            time.Time `json:"created"`
	Updated            time.Time `json:"updated"`
	ObjectCount        int       `json:"objectCount"`
	ObjectTypeCount    int       `json:"objectTypeCount"`
}

// AssetObjectType represents an object type within a schema
type AssetObjectType struct {
	ID                  string                  `json:"id"`
	Name                string                  `json:"name"`
	Description         string                  `json:"description"`
	Icon                *AssetIcon              `json:"icon"`
	Position            int                     `json:"position"`
	Created             time.Time               `json:"created"`
	Updated             time.Time               `json:"updated"`
	ObjectCount         int                     `json:"objectCount"`
	ObjectSchemaID      string                  `json:"objectSchemaId"`
	Inherited           bool                    `json:"inherited"`
	AbstractObjectType  bool                    `json:"abstractObjectType"`
	ParentObjectTypeID  string                  `json:"parentObjectTypeId,omitempty"`
	Attributes          []AssetObjectAttribute  `json:"attributes,omitempty"`
}

// AssetIcon represents an icon for an object type
type AssetIcon struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL16 string `json:"url16"`
	URL48 string `json:"url48"`
}

// AssetObjectAttribute represents an attribute definition for an object type
type AssetObjectAttribute struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Label                bool   `json:"label"`
	Type                 int    `json:"type"` // 0=Default, 1=ObjectRef, 2=User, 3=Confluence, etc.
	TypeValue            string `json:"typeValue,omitempty"`
	DefaultTypeID        int    `json:"defaultTypeId,omitempty"` // For type=0: 0=Text, 1=Integer, 2=Boolean, etc.
	Description          string `json:"description"`
	Editable             bool   `json:"editable"`
	Hidden               bool   `json:"hidden"`
	IncludeChildObjects  bool   `json:"includeChildObjectTypes"`
	UniqueAttribute      bool   `json:"uniqueAttribute"`
	MinimumCardinality   int    `json:"minimumCardinality"`
	MaximumCardinality   int    `json:"maximumCardinality"`
	Removable            bool   `json:"removable"`
	Position             int    `json:"position"`
}

// AssetObject represents an object instance in Assets
type AssetObject struct {
	ID                string                     `json:"id"`
	Label             string                     `json:"label"`
	ObjectKey         string                     `json:"objectKey"`
	ObjectType        *AssetObjectType           `json:"objectType"`
	Created           time.Time                  `json:"created"`
	Updated           time.Time                  `json:"updated"`
	HasAvatar         bool                       `json:"hasAvatar"`
	Timestamp         int64                      `json:"timestamp"`
	Attributes        []AssetObjectAttributeValue `json:"attributes"`
	ExtendedInfo      *AssetExtendedInfo         `json:"extendedInfo,omitempty"`
	Links             *AssetObjectLinks          `json:"links,omitempty"`
}

// AssetObjectAttributeValue represents an attribute value on an object
type AssetObjectAttributeValue struct {
	ID                    string                 `json:"id"`
	ObjectTypeAttributeID string                 `json:"objectTypeAttributeId"`
	ObjectAttributeValues []AssetAttributeValue  `json:"objectAttributeValues"`
}

// AssetAttributeValue represents a single value for an attribute
type AssetAttributeValue struct {
	Value          interface{} `json:"value"`
	DisplayValue   string      `json:"displayValue"`
	SearchValue    string      `json:"searchValue"`
	ReferencedType int         `json:"referencedType,omitempty"`
	User           *JiraUser   `json:"user,omitempty"`
	Status         *AssetStatus `json:"status,omitempty"`
}

// AssetStatus represents a status in Assets
type AssetStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    int    `json:"category"` // 0=Inactive, 1=Active, 2=Pending
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
	Page           int    `json:"page"`
	PageSize       int    `json:"pageSize"`
	IncludeAttributes bool `json:"includeAttributes"`
}

// ObjectSearchResult represents the result of an object search
type ObjectSearchResult struct {
	ObjectEntries  []AssetObject `json:"objectEntries"`
	ObjectTypeAttributes []AssetObjectAttribute `json:"objectTypeAttributes,omitempty"`
	PageNumber     int           `json:"pageNumber"`
	PageSize       int           `json:"pageSize"`
	TotalFilterCount int         `json:"totalFilterCount"`
	StartIndex     int           `json:"startIndex"`
	ToIndex        int           `json:"toIndex"`
	IsLast         bool          `json:"isLast"`
}

// ================================================================
// Jira Agile Types (Boards, Sprints)
// ================================================================

// JiraBoard represents a Jira Agile board
type JiraBoard struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // scrum, kanban
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

// BulkFetchResponse is the response from POST /rest/api/3/issue/bulkfetch
type BulkFetchResponse struct {
	Issues []JiraIssue       `json:"issues"`
	Errors []BulkFetchError  `json:"errors,omitempty"`
}

// BulkFetchError represents an error when fetching a specific issue
type BulkFetchError struct {
	IssueIdOrKey string `json:"issueIdOrKey"`
	ErrorMessage string `json:"errorMessage"`
}
