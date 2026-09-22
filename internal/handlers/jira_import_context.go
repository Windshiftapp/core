package handlers

import (
	"time"

	"windshift/internal/jira"
)

// jiraImportContext carries everything one issue import needs: the job- and
// project-scoped identity maps the orchestration stages resolve before the
// batch loop, plus the shared client and progress tracker. It replaces the
// former 20-parameter importIssue signature.
//
// Field scopes:
//   - job: jobID, forceReimport, progress, client, the global model maps
//     (statuses, item types, custom fields), and affectsVersionField
//   - project: workspaceID, timeProjectID, jsmImport, versionMap, iterationMap
//   - batch: userMap, usernameMap, portalCustomerMap (shared across the
//     project's issues so identities resolved once are reused)
type jiraImportContext struct {
	jobID         string
	forceReimport bool
	progress      *ImportProgress
	client        jira.Client

	workspaceID   int
	timeProjectID *int

	statusMap           map[string]int
	itemTypeMap         map[string]int
	customFieldIDMap    map[string]int
	choiceOptionIDs     map[string]map[string]int
	customFieldMappings []CustomFieldMapping
	affectsVersionField *jiraAffectsVersionCustomField

	userMap           map[string]int
	usernameMap       map[string]string
	portalCustomerMap map[string]int
	versionMap        map[string]int
	iterationMap      map[string]int

	jsmImport *jiraServiceManagementImport
}

// mentionResolver maps Jira accountIDs to Windshift usernames so ADF
// conversion can render @mentions as `@<username>` and MentionService picks
// them up through its standard regex.
func (im *jiraImportContext) mentionResolver() jira.MentionResolver {
	return jira.MentionResolver(func(accountID string) string {
		return im.usernameMap[accountID]
	})
}

// jiraGlobalModel holds the job-wide reference data resolved once before any
// project is imported: status/item-type/custom-field maps and the per-project
// preflight results.
type jiraGlobalModel struct {
	statusMap                 map[string]int
	itemTypeMap               map[string]int
	customFieldIDMap          map[string]int
	choiceOptionIDs           map[string]map[string]int
	affectsVersionField       *jiraAffectsVersionCustomField
	issueKeysByProject        map[string][]string
	applicableFieldsByProject map[string]map[string]bool
}

// jiraIssueReferences holds the Windshift identities resolved from one Jira
// issue's direct fields, ready to be spread into item creation parameters.
type jiraIssueReferences struct {
	statusID                *int
	itemTypeID              *int
	assigneeID              *int
	reporterID              *int
	creatorID               *int
	creatorPortalCustomerID *int
	channelID               *int
	requestTypeID           *int
	milestoneIDs            []int
	priorityName            string
	dueDate                 *time.Time
	createdAt               *time.Time
	updatedAt               *time.Time
	iterationID             *int
	storyPoints             *float64
	estimateMinutes         *int
	jiraRequestType         string
}
