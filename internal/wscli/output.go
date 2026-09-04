package wscli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"
)

// Output handles formatting and printing results
type Output struct {
	format string
}

func NewOutput() *Output {
	return &Output{format: outputFormat}
}

// Print outputs data in the configured format
func (o *Output) Print(data any) {
	switch o.format {
	case "table":
		o.printTable(data)
	case "csv":
		o.printCSV(data)
	default:
		o.printJSON(data)
	}
}

func (o *Output) printJSON(data any) {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(data) //nolint:errcheck // output to stdout
}

func (o *Output) printTable(data any) {
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }() //nolint:errcheck // output to stdout

	switch v := data.(type) {
	case *User:
		o.printUserTable(w, v)
	case []Item:
		o.printItemsTable(w, v)
	case *PaginatedResponse[Item]:
		o.printItemsTable(w, v.Data)
	case *Item:
		o.printItemDetailTable(w, v)
	case []Workspace:
		o.printWorkspacesTable(w, v)
	case *PaginatedResponse[Workspace]:
		o.printWorkspacesTable(w, v.Data)
	case *Workspace:
		o.printWorkspaceDetailTable(w, v)
	case []Status:
		o.printStatusesTable(w, v)
	case *StatusListResult:
		o.printStatusListTable(w, v)
	case []ItemType:
		o.printItemTypesTable(w, v)
	case []TestCase:
		o.printTestCasesTable(w, v)
	case *TestCase:
		o.printTestCaseDetailTable(w, v)
	case []TestRun:
		o.printTestRunsTable(w, v)
	case *TestRun:
		o.printTestRunDetailTable(w, v)
	case []TestResult:
		o.printTestResultsTable(w, v)
	case []TestSet:
		o.printTestSetsTable(w, v)
	case *TestSet:
		o.printTestSetDetailTable(w, v)
	case []Transition:
		o.printTransitionsTable(w, v)
	case []Comment:
		o.printCommentsTable(w, v)
	case *Comment:
		o.printCommentDetailTable(w, v)
	case []Attachment:
		o.printAttachmentsTable(w, v)
	case []Label:
		o.printLabelsTable(w, v)
	case []History:
		o.printHistoryTable(w, v)
	case []Milestone:
		o.printMilestonesTable(w, v)
	case *PaginatedResponse[Milestone]:
		o.printMilestonesTable(w, v.Data)
	case *Milestone:
		o.printMilestoneDetailTable(w, v)
	case *MilestoneProgress:
		o.printMilestoneProgressTable(w, v)
	case []Page:
		o.printPagesTable(w, v)
	case *Page:
		o.printPageDetailTable(w, v)
	case []PageLabel:
		o.printPageLabelsTable(w, v)
	case *PageLabel:
		o.printPageLabelDetailTable(w, v)
	case []PageRevision:
		o.printPageRevisionsTable(w, v)
	case *PageRevision:
		o.printPageRevisionDetailTable(w, v)
	case *PagePermissions:
		o.printPagePermissionsTable(w, v)
	case *PagePermission:
		o.printPagePermissionDetailTable(w, v)
	case []PageDiagram:
		o.printPageDiagramsTable(w, v)
	case *PageDiagram:
		o.printPageDiagramDetailTable(w, v)
	case []LinkType:
		o.printLinkTypesTable(w, v)
	case *ItemLink:
		o.printItemLinkDetailTable(w, v)
	case *LinkListResponse:
		o.printLinkListTable(w, v)
	case []Asset:
		o.printAssetsTable(w, v)
	case *PaginatedResponse[Asset]:
		o.printAssetsTable(w, v.Data)
	case *Asset:
		o.printAssetDetailTable(w, v)
	case []AssetSet:
		o.printAssetSetsTable(w, v)
	case *AssetSet:
		o.printAssetSetDetailTable(w, v)
	case []AssetType:
		o.printAssetTypesTable(w, v)
	case *AssetType:
		o.printAssetTypeDetailTable(w, v)
	case *AssetImportJob:
		o.printAssetImportJobTable(w, v)
	default:
		// Fallback to JSON for unknown types
		o.printJSON(data)
	}
}

func (o *Output) printCSV(data any) {
	w := csv.NewWriter(stdout)
	defer w.Flush()

	switch v := data.(type) {
	case *User:
		o.printUserCSV(w, v)
	case []Item:
		o.printItemsCSV(w, v)
	case *PaginatedResponse[Item]:
		o.printItemsCSV(w, v.Data)
	case *Item:
		o.printItemCSV(w, v)
	case []Workspace:
		o.printWorkspacesCSV(w, v)
	case *PaginatedResponse[Workspace]:
		o.printWorkspacesCSV(w, v.Data)
	case *Workspace:
		o.printWorkspaceCSV(w, v)
	case []Status:
		o.printStatusesCSV(w, v)
	case *StatusListResult:
		o.printStatusListCSV(w, v)
	case []ItemType:
		o.printItemTypesCSV(w, v)
	case []TestCase:
		o.printTestCasesCSV(w, v)
	case *TestCase:
		o.printTestCaseCSV(w, v)
	case []TestRun:
		o.printTestRunsCSV(w, v)
	case *TestRun:
		o.printTestRunCSV(w, v)
	case []TestResult:
		o.printTestResultsCSV(w, v)
	case []TestSet:
		o.printTestSetsCSV(w, v)
	case *TestSet:
		o.printTestSetCSV(w, v)
	case []Transition:
		o.printTransitionsCSV(w, v)
	case []Comment:
		o.printCommentsCSV(w, v)
	case *Comment:
		o.printCommentCSV(w, v)
	case []Attachment:
		o.printAttachmentsCSV(w, v)
	case []Milestone:
		o.printMilestonesCSV(w, v)
	case *PaginatedResponse[Milestone]:
		o.printMilestonesCSV(w, v.Data)
	case *Milestone:
		o.printMilestoneCSV(w, v)
	case *MilestoneProgress:
		o.printMilestoneProgressCSV(w, v)
	case []Page:
		o.printPagesCSV(w, v)
	case *Page:
		o.printPageCSV(w, v)
	case []PageLabel:
		o.printPageLabelsCSV(w, v)
	case *PageLabel:
		o.printPageLabelCSV(w, v)
	case []PageRevision:
		o.printPageRevisionsCSV(w, v)
	case *PageRevision:
		o.printPageRevisionCSV(w, v)
	case *PagePermissions:
		o.printPagePermissionsCSV(w, v)
	case *PagePermission:
		o.printPagePermissionCSV(w, v)
	case []PageDiagram:
		o.printPageDiagramsCSV(w, v)
	case *PageDiagram:
		o.printPageDiagramCSV(w, v)
	case []LinkType:
		o.printLinkTypesCSV(w, v)
	case *ItemLink:
		o.printItemLinkCSV(w, v)
	case *LinkListResponse:
		o.printLinkListCSV(w, v)
	default:
		// Fallback to JSON for unknown types
		o.printJSON(data)
	}
}

func (o *Output) printUserCSV(w *csv.Writer, u *User) {
	_ = w.Write([]string{"ID", "NAME", "EMAIL", "USERNAME"})
	_ = w.Write([]string{fmt.Sprintf("%d", u.ID), u.FullName, u.Email, u.Username})
}

func (o *Output) printItemsCSV(w *csv.Writer, items []Item) {
	_ = w.Write([]string{"KEY", "TITLE", "STATUS", "ASSIGNEE", "TYPE"})
	for i := range items {
		key, status, assignee, itemType := itemDisplayFields(&items[i])
		_ = w.Write([]string{key, items[i].Title, status, assignee, itemType})
	}
}

func (o *Output) printItemCSV(w *csv.Writer, item *Item) {
	key, status, assignee, itemType := itemDisplayFields(item)
	priority := item.PriorityName
	_ = w.Write([]string{"KEY", "TITLE", "STATUS", "TYPE", "PRIORITY", "ASSIGNEE", "DESCRIPTION", "CREATED", "UPDATED"})
	_ = w.Write([]string{key, item.Title, status, itemType, priority, assignee, item.Description, item.CreatedAt.Format(time.RFC3339), item.UpdatedAt.Format(time.RFC3339)})
}

func (o *Output) printWorkspacesCSV(w *csv.Writer, workspaces []Workspace) {
	_ = w.Write([]string{"KEY", "NAME", "ACTIVE", "ID"})
	for _, ws := range workspaces {
		active := "yes"
		if !ws.Active {
			active = "no"
		}
		_ = w.Write([]string{ws.Key, ws.Name, active, fmt.Sprintf("%d", ws.ID)})
	}
}

func (o *Output) printWorkspaceCSV(w *csv.Writer, ws *Workspace) {
	active := "yes"
	if !ws.Active {
		active = "no"
	}
	_ = w.Write([]string{"ID", "KEY", "NAME", "DESCRIPTION", "ACTIVE"})
	_ = w.Write([]string{fmt.Sprintf("%d", ws.ID), ws.Key, ws.Name, ws.Description, active})
}

func (o *Output) printStatusesCSV(w *csv.Writer, statuses []Status) {
	_ = w.Write([]string{"ID", "NAME", "CATEGORY", "DEFAULT", "COMPLETED"})
	for _, s := range statuses {
		isDefault := ""
		if s.IsDefault {
			isDefault = "yes"
		}
		isCompleted := ""
		if s.IsCompleted {
			isCompleted = "yes"
		}
		_ = w.Write([]string{fmt.Sprintf("%d", s.ID), s.Name, s.CategoryName, isDefault, isCompleted})
	}
}

func (o *Output) printStatusListCSV(w *csv.Writer, result *StatusListResult) {
	workspaceKey, workspaceName := "", ""
	if result.Workspace != nil {
		workspaceKey = result.Workspace.Key
		workspaceName = result.Workspace.Name
	}
	_ = w.Write([]string{"SCOPE", "WORKSPACE_KEY", "WORKSPACE_NAME", "ID", "NAME", "CATEGORY", "DEFAULT", "COMPLETED"})
	for _, status := range result.Statuses {
		isDefault := ""
		if status.IsDefault {
			isDefault = "yes"
		}
		isCompleted := ""
		if status.IsCompleted {
			isCompleted = "yes"
		}
		_ = w.Write([]string{
			result.Scope,
			workspaceKey,
			workspaceName,
			fmt.Sprintf("%d", status.ID),
			status.Name,
			status.CategoryName,
			isDefault,
			isCompleted,
		})
	}
}

func (o *Output) printItemTypesCSV(w *csv.Writer, types []ItemType) {
	_ = w.Write([]string{"ID", "NAME", "ICON"})
	for _, t := range types {
		_ = w.Write([]string{fmt.Sprintf("%d", t.ID), t.Name, t.Icon})
	}
}

func (o *Output) printTestCasesCSV(w *csv.Writer, cases []TestCase) {
	_ = w.Write([]string{"ID", "TITLE", "PRIORITY", "STATUS", "FOLDER"})
	for _, tc := range cases {
		folder := tc.FolderName
		if folder == "" {
			folder = "(root)"
		}
		_ = w.Write([]string{fmt.Sprintf("%d", tc.ID), tc.Title, tc.Priority, tc.Status, folder})
	}
}

func (o *Output) printTestCaseCSV(w *csv.Writer, tc *TestCase) {
	folder := tc.FolderName
	if folder == "" {
		folder = "(root)"
	}
	_ = w.Write([]string{"ID", "TITLE", "PRIORITY", "STATUS", "FOLDER", "PRECONDITIONS", "ESTIMATED_DURATION"})
	_ = w.Write([]string{fmt.Sprintf("%d", tc.ID), tc.Title, tc.Priority, tc.Status, folder, tc.Preconditions, fmt.Sprintf("%d", tc.EstimatedDuration)})
}

func (o *Output) printTestRunsCSV(w *csv.Writer, runs []TestRun) {
	_ = w.Write([]string{"ID", "NAME", "ASSIGNEE", "STARTED", "ENDED"})
	for _, run := range runs {
		assignee := run.AssigneeName
		if assignee == "" {
			assignee = ""
		}
		started := run.StartedAt.Format("2006-01-02 15:04")
		ended := ""
		if run.EndedAt != nil {
			ended = run.EndedAt.Format("2006-01-02 15:04")
		}
		_ = w.Write([]string{fmt.Sprintf("%d", run.ID), run.Name, assignee, started, ended})
	}
}

func (o *Output) printTestRunCSV(w *csv.Writer, run *TestRun) {
	assignee := run.AssigneeName
	started := run.StartedAt.Format(time.RFC3339)
	ended := ""
	if run.EndedAt != nil {
		ended = run.EndedAt.Format(time.RFC3339)
	}
	status := "in_progress"
	if run.EndedAt != nil {
		status = "completed"
	}
	_ = w.Write([]string{"ID", "NAME", "SET_ID", "ASSIGNEE", "STARTED", "ENDED", "STATUS"})
	_ = w.Write([]string{fmt.Sprintf("%d", run.ID), run.Name, fmt.Sprintf("%d", run.PlanID), assignee, started, ended, status})
}

func (o *Output) printTestResultsCSV(w *csv.Writer, results []TestResult) {
	_ = w.Write([]string{"CASE_ID", "TITLE", "STATUS", "EXECUTED"})
	for _, r := range results {
		executed := ""
		if r.ExecutedAt != nil {
			executed = r.ExecutedAt.Format("2006-01-02 15:04")
		}
		_ = w.Write([]string{fmt.Sprintf("%d", r.TestCaseID), r.TestCaseTitle, r.Status, executed})
	}
}

func (o *Output) printTestSetsCSV(w *csv.Writer, sets []TestSet) {
	_ = w.Write([]string{"ID", "NAME", "CASES", "RUNS", "LAST_STATUS"})
	for _, s := range sets {
		lastStatus := s.LastRunStatus
		_ = w.Write([]string{fmt.Sprintf("%d", s.ID), s.Name, fmt.Sprintf("%d", s.TestCaseCount), fmt.Sprintf("%d", s.TotalRuns), lastStatus})
	}
}

func (o *Output) printTestSetCSV(w *csv.Writer, set *TestSet) {
	_ = w.Write([]string{"ID", "NAME", "DESCRIPTION", "TEST_CASES", "TOTAL_RUNS", "LAST_RUN_STATUS"})
	_ = w.Write([]string{fmt.Sprintf("%d", set.ID), set.Name, set.Description, fmt.Sprintf("%d", set.TestCaseCount), fmt.Sprintf("%d", set.TotalRuns), set.LastRunStatus})
}

func (o *Output) printTransitionsCSV(w *csv.Writer, transitions []Transition) {
	_ = w.Write([]string{"STATUS_ID", "STATUS_NAME"})
	for _, t := range transitions {
		name := ""
		if t.ToStatus != nil {
			name = t.ToStatus.Name
		}
		_ = w.Write([]string{fmt.Sprintf("%d", t.ToStatusID), name})
	}
}

func (o *Output) printCommentsCSV(w *csv.Writer, comments []Comment) {
	_ = w.Write([]string{"ID", "AUTHOR", "CREATED", "CONTENT"})
	for _, c := range comments {
		author := ""
		if c.Author != nil {
			author = c.Author.FullName
		}
		created := c.CreatedAt.Format("2006-01-02 15:04")
		_ = w.Write([]string{fmt.Sprintf("%d", c.ID), author, created, c.Content})
	}
}

func (o *Output) printCommentCSV(w *csv.Writer, c *Comment) {
	author := ""
	if c.Author != nil {
		author = c.Author.FullName
	}
	_ = w.Write([]string{"ID", "ITEM_ID", "AUTHOR", "CREATED", "UPDATED", "CONTENT"})
	_ = w.Write([]string{fmt.Sprintf("%d", c.ID), fmt.Sprintf("%d", c.ItemID), author, c.CreatedAt.Format(time.RFC3339), c.UpdatedAt.Format(time.RFC3339), c.Content})
}

func (o *Output) printUserTable(w *tabwriter.Writer, u *User) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", u.ID)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", u.FullName)
	_, _ = fmt.Fprintf(w, "Email:\t%s\n", u.Email)
	_, _ = fmt.Fprintf(w, "Username:\t%s\n", u.Username)
}

func (o *Output) printItemsTable(w *tabwriter.Writer, items []Item) {
	_, _ = fmt.Fprintln(w, "KEY\tTITLE\tSTATUS\tASSIGNEE\tTYPE")
	_, _ = fmt.Fprintln(w, "---\t-----\t------\t--------\t----")
	for i := range items {
		key, status, assignee, itemType := itemDisplayFields(&items[i])
		// Truncate long titles
		title := items[i].Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", key, title, status, assignee, itemType)
	}
}

func (o *Output) printItemDetailTable(w *tabwriter.Writer, item *Item) {
	key, _, _, _ := itemDisplayFields(item)
	_, _ = fmt.Fprintf(w, "Key:\t%s\n", key)
	_, _ = fmt.Fprintf(w, "Title:\t%s\n", item.Title)
	if item.StatusName != "" {
		_, _ = fmt.Fprintf(w, "Status:\t%s\n", item.StatusName)
	}
	if item.ItemTypeName != "" {
		_, _ = fmt.Fprintf(w, "Type:\t%s\n", item.ItemTypeName)
	}
	if item.ParentID != nil {
		_, _ = fmt.Fprintf(w, "Parent:\t%s\n", parentDisplay(item))
	}
	if len(item.Children) > 0 {
		_, _ = fmt.Fprintf(w, "Children:\t%s\n", childrenSummary(item.Children))
	}
	if item.PriorityName != "" {
		_, _ = fmt.Fprintf(w, "Priority:\t%s\n", item.PriorityName)
	}
	if item.AssigneeName != "" {
		_, _ = fmt.Fprintf(w, "Assignee:\t%s\n", item.AssigneeName)
	}
	if item.CreatorName != "" {
		_, _ = fmt.Fprintf(w, "Creator:\t%s\n", item.CreatorName)
	}
	if item.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:\t%s\n", truncateString(item.Description, 100))
	}
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", item.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", item.UpdatedAt.Format(time.RFC3339))

	printImageAttachmentHint(w, item.Attachments)

	if len(item.Transitions) > 0 {
		_, _ = fmt.Fprintln(w, "\nAvailable Transitions:")
		for _, t := range item.Transitions {
			if t.ToStatus != nil {
				_, _ = fmt.Fprintf(w, "  - %s (ID: %d)\n", t.ToStatus.Name, t.ToStatusID)
			}
		}
	}
}

// printImageAttachmentHint surfaces image attachments and points a coding agent
// at the view_image tool. Discovery is otherwise unreliable: the agent would
// have to infer image-ness from raw attachment JSON. Only image attachments are
// listed here — non-image document extraction is a separate capability and gets
// its own hint elsewhere. The hint is omitted entirely when there are none.
func printImageAttachmentHint(w *tabwriter.Writer, attachments []Attachment) {
	images := make([]Attachment, 0, len(attachments))
	for _, a := range attachments {
		if strings.HasPrefix(a.MimeType, "image/") {
			images = append(images, a)
		}
	}
	if len(images) == 0 {
		return
	}
	noun := "image attachment"
	if len(images) > 1 {
		noun = "image attachments"
	}
	_, _ = fmt.Fprintf(w, "\nThis item has %d %s. If you need to inspect visual content, call view_image with the attachment id; do not download the file or run OCR/identify/strings/xxd first:\n", len(images), noun)
	for _, a := range images {
		name := a.OriginalFilename
		if name == "" {
			name = a.Filename
		}
		_, _ = fmt.Fprintf(w, "  - id %d\t%s (%s)\n", a.ID, name, a.MimeType)
	}
}

func (o *Output) printWorkspacesTable(w *tabwriter.Writer, workspaces []Workspace) {
	_, _ = fmt.Fprintln(w, "KEY\tNAME\tACTIVE\tID")
	_, _ = fmt.Fprintln(w, "---\t----\t------\t--")
	for _, ws := range workspaces {
		active := "yes"
		if !ws.Active {
			active = "no"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", ws.Key, ws.Name, active, ws.ID)
	}
}

func (o *Output) printWorkspaceDetailTable(w *tabwriter.Writer, ws *Workspace) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", ws.ID)
	_, _ = fmt.Fprintf(w, "Key:\t%s\n", ws.Key)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", ws.Name)
	if ws.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:\t%s\n", ws.Description)
	}
	active := "yes"
	if !ws.Active {
		active = "no"
	}
	_, _ = fmt.Fprintf(w, "Active:\t%s\n", active)
}

func (o *Output) printStatusesTable(w *tabwriter.Writer, statuses []Status) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCATEGORY\tDEFAULT\tCOMPLETED")
	_, _ = fmt.Fprintln(w, "--\t----\t--------\t-------\t---------")
	for _, s := range statuses {
		isDefault := ""
		if s.IsDefault {
			isDefault = "yes"
		}
		isCompleted := ""
		if s.IsCompleted {
			isCompleted = "yes"
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", s.ID, s.Name, s.CategoryName, isDefault, isCompleted)
	}
}

func (o *Output) printStatusListTable(w *tabwriter.Writer, result *StatusListResult) {
	_, _ = fmt.Fprintf(w, "Scope:\t%s\n", result.Scope)
	if result.Workspace != nil {
		_, _ = fmt.Fprintf(w, "Workspace:\t%s (%s)\n", result.Workspace.Name, result.Workspace.Key)
	}
	_, _ = fmt.Fprintln(w)
	o.printStatusesTable(w, result.Statuses)
}

func (o *Output) printItemTypesTable(w *tabwriter.Writer, types []ItemType) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tICON")
	_, _ = fmt.Fprintln(w, "--\t----\t----")
	for _, t := range types {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\n", t.ID, t.Name, t.Icon)
	}
}

func (o *Output) printTestCasesTable(w *tabwriter.Writer, cases []TestCase) {
	_, _ = fmt.Fprintln(w, "ID\tTITLE\tPRIORITY\tSTATUS\tFOLDER")
	_, _ = fmt.Fprintln(w, "--\t-----\t--------\t------\t------")
	for _, tc := range cases {
		title := truncateString(tc.Title, 40)
		folder := tc.FolderName
		if folder == "" {
			folder = "(root)"
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", tc.ID, title, tc.Priority, tc.Status, folder)
	}
}

func (o *Output) printTestCaseDetailTable(w *tabwriter.Writer, tc *TestCase) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", tc.ID)
	_, _ = fmt.Fprintf(w, "Title:\t%s\n", tc.Title)
	_, _ = fmt.Fprintf(w, "Priority:\t%s\n", tc.Priority)
	_, _ = fmt.Fprintf(w, "Status:\t%s\n", tc.Status)
	if tc.FolderName != "" {
		_, _ = fmt.Fprintf(w, "Folder:\t%s\n", tc.FolderName)
	}
	if tc.Preconditions != "" {
		_, _ = fmt.Fprintf(w, "Preconditions:\t%s\n", truncateString(tc.Preconditions, 100))
	}
	if tc.EstimatedDuration > 0 {
		_, _ = fmt.Fprintf(w, "Estimated Duration:\t%d min\n", tc.EstimatedDuration)
	}
}

func (o *Output) printTestRunsTable(w *tabwriter.Writer, runs []TestRun) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tASSIGNEE\tSTARTED\tENDED")
	_, _ = fmt.Fprintln(w, "--\t----\t--------\t-------\t-----")
	for _, run := range runs {
		name := truncateString(run.Name, 30)
		assignee := run.AssigneeName
		if assignee == "" {
			assignee = "-"
		}
		started := run.StartedAt.Format("2006-01-02 15:04")
		ended := "-"
		if run.EndedAt != nil {
			ended = run.EndedAt.Format("2006-01-02 15:04")
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", run.ID, name, assignee, started, ended)
	}
}

func (o *Output) printTestRunDetailTable(w *tabwriter.Writer, run *TestRun) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", run.ID)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", run.Name)
	_, _ = fmt.Fprintf(w, "Plan ID:\t%d\n", run.PlanID)
	if run.AssigneeName != "" {
		_, _ = fmt.Fprintf(w, "Assignee:\t%s\n", run.AssigneeName)
	}
	_, _ = fmt.Fprintf(w, "Started:\t%s\n", run.StartedAt.Format(time.RFC3339))
	if run.EndedAt != nil {
		_, _ = fmt.Fprintf(w, "Ended:\t%s\n", run.EndedAt.Format(time.RFC3339))
	} else {
		_, _ = fmt.Fprintf(w, "Status:\tin progress\n")
	}
}

func (o *Output) printTestResultsTable(w *tabwriter.Writer, results []TestResult) {
	_, _ = fmt.Fprintln(w, "CASE_ID\tTITLE\tSTATUS\tEXECUTED")
	_, _ = fmt.Fprintln(w, "-------\t-----\t------\t--------")
	for _, r := range results {
		title := truncateString(r.TestCaseTitle, 40)
		executed := "-"
		if r.ExecutedAt != nil {
			executed = r.ExecutedAt.Format("2006-01-02 15:04")
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.TestCaseID, title, r.Status, executed)
	}
}

func (o *Output) printTestSetsTable(w *tabwriter.Writer, sets []TestSet) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCASES\tRUNS\tLAST_STATUS")
	_, _ = fmt.Fprintln(w, "--\t----\t-----\t----\t-----------")
	for _, s := range sets {
		name := truncateString(s.Name, 30)
		lastStatus := s.LastRunStatus
		if lastStatus == "" {
			lastStatus = "-"
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%s\n", s.ID, name, s.TestCaseCount, s.TotalRuns, lastStatus)
	}
}

func (o *Output) printTestSetDetailTable(w *tabwriter.Writer, set *TestSet) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", set.ID)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", set.Name)
	if set.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:\t%s\n", truncateString(set.Description, 100))
	}
	_, _ = fmt.Fprintf(w, "Test Cases:\t%d\n", set.TestCaseCount)
	_, _ = fmt.Fprintf(w, "Total Runs:\t%d\n", set.TotalRuns)
	if set.LastRunStatus != "" {
		_, _ = fmt.Fprintf(w, "Last Run Status:\t%s\n", set.LastRunStatus)
	}
}

func (o *Output) printTransitionsTable(w *tabwriter.Writer, transitions []Transition) {
	_, _ = fmt.Fprintln(w, "STATUS_ID\tSTATUS_NAME")
	_, _ = fmt.Fprintln(w, "---------\t-----------")
	for _, t := range transitions {
		name := ""
		if t.ToStatus != nil {
			name = t.ToStatus.Name
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\n", t.ToStatusID, name)
	}
}

func (o *Output) printCommentsTable(w *tabwriter.Writer, comments []Comment) {
	_, _ = fmt.Fprintln(w, "ID\tAUTHOR\tCREATED\tCONTENT")
	_, _ = fmt.Fprintln(w, "--\t------\t-------\t-------")
	for _, c := range comments {
		author := ""
		if c.Author != nil {
			author = c.Author.FullName
		}
		created := c.CreatedAt.Format("2006-01-02 15:04")
		content := truncateString(c.Content, 50)
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", c.ID, author, created, content)
	}
}

func (o *Output) printAttachmentsTable(w *tabwriter.Writer, atts []Attachment) {
	_, _ = fmt.Fprintln(w, "ID\tFILENAME\tSIZE\tMIME\tUPLOADER\tCREATED")
	_, _ = fmt.Fprintln(w, "--\t--------\t----\t----\t--------\t-------")
	for _, a := range atts {
		uploader := "-"
		if a.Uploader != nil && a.Uploader.FullName != "" {
			uploader = a.Uploader.FullName
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
			a.ID,
			truncateString(a.OriginalFilename, 50),
			humanFileSize(a.FileSize),
			a.MimeType,
			uploader,
			a.CreatedAt.Format("2006-01-02 15:04"),
		)
	}
	printImageAttachmentHint(w, atts)
}

func (o *Output) printAttachmentsCSV(w *csv.Writer, atts []Attachment) {
	_ = w.Write([]string{"ID", "FILENAME", "SIZE", "MIME", "UPLOADER", "CREATED"})
	for _, a := range atts {
		uploader := ""
		if a.Uploader != nil {
			uploader = a.Uploader.FullName
		}
		_ = w.Write([]string{
			fmt.Sprintf("%d", a.ID),
			a.OriginalFilename,
			fmt.Sprintf("%d", a.FileSize),
			a.MimeType,
			uploader,
			a.CreatedAt.Format(time.RFC3339),
		})
	}
}

// humanFileSize formats a byte count in a compact form (e.g. "12 KB",
// "3.4 MB"). Used only for table output; CSV keeps the raw byte count.
func humanFileSize(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func (o *Output) printCommentDetailTable(w *tabwriter.Writer, c *Comment) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", c.ID)
	_, _ = fmt.Fprintf(w, "Item ID:\t%d\n", c.ItemID)
	if c.Author != nil {
		_, _ = fmt.Fprintf(w, "Author:\t%s\n", c.Author.FullName)
	}
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", c.CreatedAt.Format("2006-01-02 15:04:05"))
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", c.UpdatedAt.Format("2006-01-02 15:04:05"))
	_, _ = fmt.Fprintf(w, "Content:\n%s\n", c.Content)
}

// ============================================
// Item Label / History formatters
// ============================================

func (o *Output) printLabelsTable(w *tabwriter.Writer, labels []Label) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCOLOR")
	_, _ = fmt.Fprintln(w, "--\t----\t-----")
	for _, l := range labels {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\n", l.ID, l.Name, l.Color)
	}
}

// historyValue picks the human-readable value for a history cell: resolved
// value when the server provided one, raw value otherwise, "-" when empty.
func historyValue(raw, resolved *string) string {
	if resolved != nil && *resolved != "" {
		return *resolved
	}
	if raw != nil && *raw != "" {
		return *raw
	}
	return "-"
}

func (o *Output) printHistoryTable(w *tabwriter.Writer, history []History) {
	_, _ = fmt.Fprintln(w, "FIELD\tOLD\tNEW\tACTOR\tTIME")
	_, _ = fmt.Fprintln(w, "-----\t---\t---\t-----\t----")
	for i := range history {
		h := &history[i]
		actor := "-"
		if h.User != nil && h.User.FullName != "" {
			actor = h.User.FullName
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			h.FieldName,
			truncateString(historyValue(h.OldValue, h.ResolvedOldValue), 40),
			truncateString(historyValue(h.NewValue, h.ResolvedNewValue), 40),
			actor,
			h.ChangedAt.Format("2006-01-02 15:04"),
		)
	}
}

func truncateString(s string, maxLen int) string {
	// Remove newlines for table display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

// ============================================
// Milestone Formatters
// ============================================

func (o *Output) printMilestonesTable(w *tabwriter.Writer, milestones []Milestone) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tSTATUS\tTARGET\tWORKSPACE")
	_, _ = fmt.Fprintln(w, "--\t----\t------\t------\t---------")
	for _, m := range milestones {
		name := truncateString(m.Name, 30)
		target := "-"
		if m.TargetDate != nil {
			target = *m.TargetDate
		}
		workspace := "(global)"
		if m.WorkspaceName != "" {
			workspace = m.WorkspaceName
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", m.ID, name, m.Status, target, workspace)
	}
}

func (o *Output) printMilestoneDetailTable(w *tabwriter.Writer, m *Milestone) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", m.ID)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", m.Name)
	_, _ = fmt.Fprintf(w, "Status:\t%s\n", m.Status)
	if m.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:\t%s\n", truncateString(m.Description, 100))
	}
	if m.TargetDate != nil {
		_, _ = fmt.Fprintf(w, "Target Date:\t%s\n", *m.TargetDate)
	}
	if m.IsGlobal {
		_, _ = fmt.Fprintf(w, "Scope:\tGlobal\n")
	} else if m.WorkspaceName != "" {
		_, _ = fmt.Fprintf(w, "Workspace:\t%s\n", m.WorkspaceName)
	}
	if m.CategoryName != "" {
		_, _ = fmt.Fprintf(w, "Category:\t%s\n", m.CategoryName)
	}
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", m.CreatedAt)
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", m.UpdatedAt)
}

func (o *Output) printMilestoneProgressTable(w *tabwriter.Writer, p *MilestoneProgress) {
	_, _ = fmt.Fprintf(w, "Milestone:\t%s (#%d)\n", p.MilestoneName, p.MilestoneID)
	if p.Status != "" {
		_, _ = fmt.Fprintf(w, "Status:\t%s\n", p.Status)
	}
	if p.TargetDate != nil && *p.TargetDate != "" {
		_, _ = fmt.Fprintf(w, "Target Date:\t%s\n", *p.TargetDate)
	}
	if p.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:\t%s\n", p.Description)
	}

	_, _ = fmt.Fprintln(w, "\nProgress:")
	_, _ = fmt.Fprintf(w, "  Total Items:\t%d\n", p.TotalItems)
	_, _ = fmt.Fprintf(w, "  Completed:\t%d (%.1f%%)\n", p.CompletedItems, p.PercentComplete)
	if len(p.StatusBreakdown) > 0 {
		_, _ = fmt.Fprintln(w, "  By Status Category:")
		for _, sb := range p.StatusBreakdown {
			_, _ = fmt.Fprintf(w, "    %s:\t%d\n", sb.CategoryName, sb.ItemCount)
		}
	}
}

func (o *Output) printMilestonesCSV(w *csv.Writer, milestones []Milestone) {
	_ = w.Write([]string{"ID", "NAME", "STATUS", "TARGET_DATE", "WORKSPACE", "IS_GLOBAL"})
	for _, m := range milestones {
		target := ""
		if m.TargetDate != nil {
			target = *m.TargetDate
		}
		workspace := ""
		if m.WorkspaceName != "" {
			workspace = m.WorkspaceName
		}
		isGlobal := "no"
		if m.IsGlobal {
			isGlobal = "yes"
		}
		_ = w.Write([]string{fmt.Sprintf("%d", m.ID), m.Name, m.Status, target, workspace, isGlobal})
	}
}

func (o *Output) printMilestoneCSV(w *csv.Writer, m *Milestone) {
	target := ""
	if m.TargetDate != nil {
		target = *m.TargetDate
	}
	workspace := ""
	if m.WorkspaceName != "" {
		workspace = m.WorkspaceName
	}
	isGlobal := "no"
	if m.IsGlobal {
		isGlobal = "yes"
	}
	_ = w.Write([]string{"ID", "NAME", "STATUS", "TARGET_DATE", "DESCRIPTION", "WORKSPACE", "IS_GLOBAL", "CREATED", "UPDATED"})
	_ = w.Write([]string{fmt.Sprintf("%d", m.ID), m.Name, m.Status, target, m.Description, workspace, isGlobal, m.CreatedAt, m.UpdatedAt})
}

func (o *Output) printMilestoneProgressCSV(w *csv.Writer, p *MilestoneProgress) {
	target := ""
	if p.TargetDate != nil {
		target = *p.TargetDate
	}

	statusParts := make([]string, 0, len(p.StatusBreakdown))
	for _, sb := range p.StatusBreakdown {
		statusParts = append(statusParts, fmt.Sprintf("%s:%d", sb.CategoryName, sb.ItemCount))
	}
	breakdown := strings.Join(statusParts, ";")

	_ = w.Write([]string{"ID", "NAME", "STATUS", "TARGET_DATE", "TOTAL_ITEMS", "COMPLETED_ITEMS", "PERCENT_COMPLETE", "STATUS_BREAKDOWN"})
	_ = w.Write([]string{
		fmt.Sprintf("%d", p.MilestoneID),
		p.MilestoneName,
		p.Status,
		target,
		fmt.Sprintf("%d", p.TotalItems),
		fmt.Sprintf("%d", p.CompletedItems),
		fmt.Sprintf("%.1f", p.PercentComplete),
		breakdown,
	})
}

// ============================================
// Page / PageLabel / PageRevision formatters
// ============================================

// pageRowTitle indents the title by depth so the flat list reads like a
// tree — matches the `ws page list` help promise (id / depth-indented
// title / slug / updated).
func pageRowTitle(p *Page) string {
	indent := strings.Repeat("  ", p.Depth)
	return indent + p.Title
}

func (o *Output) printPagesTable(w *tabwriter.Writer, pages []Page) {
	_, _ = fmt.Fprintln(w, "ID\tTITLE\tSLUG\tUPDATED")
	_, _ = fmt.Fprintln(w, "--\t-----\t----\t-------")
	for i := range pages {
		p := &pages[i]
		title := truncateString(pageRowTitle(p), 60)
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", p.ID, title, p.Slug, p.UpdatedAt.Format("2006-01-02 15:04"))
	}
}

func (o *Output) printPageDetailTable(w *tabwriter.Writer, p *Page) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", p.ID)
	_, _ = fmt.Fprintf(w, "Title:\t%s\n", p.Title)
	_, _ = fmt.Fprintf(w, "Slug:\t%s\n", p.Slug)
	if p.ParentID != nil {
		_, _ = fmt.Fprintf(w, "Parent:\t%d\n", *p.ParentID)
	}
	_, _ = fmt.Fprintf(w, "Depth:\t%d\n", p.Depth)
	if p.Path != "" {
		_, _ = fmt.Fprintf(w, "Path:\t%s\n", p.Path)
	}
	if p.IsHome {
		_, _ = fmt.Fprintf(w, "Home:\tyes\n")
	}
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", p.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", p.UpdatedAt.Format(time.RFC3339))
	if p.Excerpt != "" {
		_, _ = fmt.Fprintf(w, "Excerpt:\t%s\n", truncateString(p.Excerpt, 100))
	}
	if len(p.Labels) > 0 {
		names := make([]string, 0, len(p.Labels))
		for _, l := range p.Labels {
			names = append(names, l.Name)
		}
		_, _ = fmt.Fprintf(w, "Labels:\t%s\n", strings.Join(names, ", "))
	}
}

func (o *Output) printPageLabelsTable(w *tabwriter.Writer, labels []PageLabel) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCOLOR")
	_, _ = fmt.Fprintln(w, "--\t----\t-----")
	for _, l := range labels {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\n", l.ID, l.Name, l.Color)
	}
}

func (o *Output) printPageLabelDetailTable(w *tabwriter.Writer, l *PageLabel) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", l.ID)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", l.Name)
	_, _ = fmt.Fprintf(w, "Color:\t%s\n", l.Color)
	_, _ = fmt.Fprintf(w, "Workspace:\t%d\n", l.WorkspaceID)
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", l.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", l.UpdatedAt.Format(time.RFC3339))
}

func (o *Output) printPageRevisionsTable(w *tabwriter.Writer, revs []PageRevision) {
	_, _ = fmt.Fprintln(w, "ID\tREVISION\tCHANGE_TYPE\tAUTHOR\tCREATED")
	_, _ = fmt.Fprintln(w, "--\t--------\t-----------\t------\t-------")
	for _, r := range revs {
		_, _ = fmt.Fprintf(w, "%d\t%d\t%s\t%d\t%s\n", r.ID, r.RevisionNumber, r.ChangeType, r.CreatedBy, r.CreatedAt.Format("2006-01-02 15:04"))
	}
}

func (o *Output) printPageRevisionDetailTable(w *tabwriter.Writer, r *PageRevision) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", r.ID)
	_, _ = fmt.Fprintf(w, "Page:\t%d\n", r.PageID)
	_, _ = fmt.Fprintf(w, "Revision:\t%d\n", r.RevisionNumber)
	_, _ = fmt.Fprintf(w, "Change:\t%s\n", r.ChangeType)
	_, _ = fmt.Fprintf(w, "Author:\t%d\n", r.CreatedBy)
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", r.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "Title:\t%s\n", r.Title)
	if r.ChangeSummary != "" {
		_, _ = fmt.Fprintf(w, "Summary:\t%s\n", r.ChangeSummary)
	}
	if r.Excerpt != "" {
		_, _ = fmt.Fprintf(w, "Excerpt:\t%s\n", truncateString(r.Excerpt, 100))
	}
}

func (o *Output) printPagePermissionsTable(w *tabwriter.Writer, p *PagePermissions) {
	_, _ = fmt.Fprintf(w, "Page:\t%d\n", p.PageID)
	_, _ = fmt.Fprintf(w, "Inherit permissions:\t%t\n", p.InheritPermissions)
	_, _ = fmt.Fprintf(w, "Effective level:\t%s\n\n", p.EffectiveLevel)
	_, _ = fmt.Fprintln(w, "ID\tPRINCIPAL\tLEVEL\tGRANTED_BY")
	_, _ = fmt.Fprintln(w, "--\t---------\t-----\t----------")
	for _, row := range p.ACL {
		grantedBy := ""
		if row.GrantedBy != nil {
			grantedBy = fmt.Sprintf("%d", *row.GrantedBy)
		}
		_, _ = fmt.Fprintf(w, "%d\t%s:%d\t%s\t%s\n", row.ID, row.PrincipalType, row.PrincipalID, row.PermissionLevel, grantedBy)
	}
}

func (o *Output) printPagePermissionDetailTable(w *tabwriter.Writer, p *PagePermission) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", p.ID)
	_, _ = fmt.Fprintf(w, "Page:\t%d\n", p.PageID)
	_, _ = fmt.Fprintf(w, "Principal:\t%s:%d\n", p.PrincipalType, p.PrincipalID)
	_, _ = fmt.Fprintf(w, "Level:\t%s\n", p.PermissionLevel)
	if p.GrantedBy != nil {
		_, _ = fmt.Fprintf(w, "Granted by:\t%d\n", *p.GrantedBy)
	}
}

func (o *Output) printPageDiagramsTable(w *tabwriter.Writer, diagrams []PageDiagram) {
	_, _ = fmt.Fprintln(w, "ATTACHMENT\tPAGE\tNAME\tKIND\tCONTENT HASH")
	_, _ = fmt.Fprintln(w, "----------\t----\t----\t----\t------------")
	for i := range diagrams {
		d := &diagrams[i]
		_, _ = fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\n", d.AttachmentID, d.PageID, d.Name, d.Kind, d.ContentHash)
	}
}

func (o *Output) printPageDiagramDetailTable(w *tabwriter.Writer, d *PageDiagram) {
	_, _ = fmt.Fprintf(w, "Attachment:\t%d\n", d.AttachmentID)
	_, _ = fmt.Fprintf(w, "Page:\t%d\n", d.PageID)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", d.Name)
	_, _ = fmt.Fprintf(w, "Kind:\t%s\n", d.Kind)
	_, _ = fmt.Fprintf(w, "Content hash:\t%s\n", d.ContentHash)
	if d.RevisionNumber > 0 {
		_, _ = fmt.Fprintf(w, "Revision:\t%d\n", d.RevisionNumber)
	}
}

func (o *Output) printPagesCSV(w *csv.Writer, pages []Page) {
	_ = w.Write([]string{"ID", "TITLE", "SLUG", "DEPTH", "PARENT_ID", "UPDATED"})
	for i := range pages {
		p := &pages[i]
		parent := ""
		if p.ParentID != nil {
			parent = fmt.Sprintf("%d", *p.ParentID)
		}
		_ = w.Write([]string{
			fmt.Sprintf("%d", p.ID),
			p.Title,
			p.Slug,
			fmt.Sprintf("%d", p.Depth),
			parent,
			p.UpdatedAt.Format(time.RFC3339),
		})
	}
}

func (o *Output) printPageCSV(w *csv.Writer, p *Page) {
	parent := ""
	if p.ParentID != nil {
		parent = fmt.Sprintf("%d", *p.ParentID)
	}
	_ = w.Write([]string{"ID", "TITLE", "SLUG", "PARENT_ID", "DEPTH", "PATH", "EXCERPT", "CREATED", "UPDATED"})
	_ = w.Write([]string{
		fmt.Sprintf("%d", p.ID),
		p.Title,
		p.Slug,
		parent,
		fmt.Sprintf("%d", p.Depth),
		p.Path,
		p.Excerpt,
		p.CreatedAt.Format(time.RFC3339),
		p.UpdatedAt.Format(time.RFC3339),
	})
}

func (o *Output) printPageLabelsCSV(w *csv.Writer, labels []PageLabel) {
	_ = w.Write([]string{"ID", "NAME", "COLOR", "WORKSPACE_ID"})
	for _, l := range labels {
		_ = w.Write([]string{fmt.Sprintf("%d", l.ID), l.Name, l.Color, fmt.Sprintf("%d", l.WorkspaceID)})
	}
}

func (o *Output) printPageLabelCSV(w *csv.Writer, l *PageLabel) {
	_ = w.Write([]string{"ID", "NAME", "COLOR", "WORKSPACE_ID", "CREATED", "UPDATED"})
	_ = w.Write([]string{
		fmt.Sprintf("%d", l.ID),
		l.Name,
		l.Color,
		fmt.Sprintf("%d", l.WorkspaceID),
		l.CreatedAt.Format(time.RFC3339),
		l.UpdatedAt.Format(time.RFC3339),
	})
}

func (o *Output) printPageRevisionsCSV(w *csv.Writer, revs []PageRevision) {
	_ = w.Write([]string{"REVISION", "PAGE_ID", "CHANGE_TYPE", "AUTHOR_ID", "TITLE", "CREATED"})
	for _, r := range revs {
		o.writePageRevisionCSVRow(w, &r)
	}
}

func (o *Output) printPageRevisionCSV(w *csv.Writer, r *PageRevision) {
	_ = w.Write([]string{"REVISION", "PAGE_ID", "CHANGE_TYPE", "AUTHOR_ID", "TITLE", "CREATED"})
	o.writePageRevisionCSVRow(w, r)
}

func (o *Output) writePageRevisionCSVRow(w *csv.Writer, r *PageRevision) {
	_ = w.Write([]string{
		fmt.Sprintf("%d", r.RevisionNumber),
		fmt.Sprintf("%d", r.PageID),
		r.ChangeType,
		fmt.Sprintf("%d", r.CreatedBy),
		r.Title,
		r.CreatedAt.Format(time.RFC3339),
	})
}

func (o *Output) printPagePermissionsCSV(w *csv.Writer, p *PagePermissions) {
	_ = w.Write([]string{"ID", "PAGE_ID", "PRINCIPAL_TYPE", "PRINCIPAL_ID", "PERMISSION_LEVEL", "GRANTED_BY"})
	for _, row := range p.ACL {
		o.writePagePermissionCSVRow(w, &row)
	}
}

func (o *Output) printPagePermissionCSV(w *csv.Writer, p *PagePermission) {
	_ = w.Write([]string{"ID", "PAGE_ID", "PRINCIPAL_TYPE", "PRINCIPAL_ID", "PERMISSION_LEVEL", "GRANTED_BY"})
	o.writePagePermissionCSVRow(w, p)
}

func (o *Output) writePagePermissionCSVRow(w *csv.Writer, p *PagePermission) {
	grantedBy := ""
	if p.GrantedBy != nil {
		grantedBy = fmt.Sprintf("%d", *p.GrantedBy)
	}
	_ = w.Write([]string{fmt.Sprintf("%d", p.ID), fmt.Sprintf("%d", p.PageID), p.PrincipalType, fmt.Sprintf("%d", p.PrincipalID), p.PermissionLevel, grantedBy})
}

func (o *Output) printPageDiagramsCSV(w *csv.Writer, diagrams []PageDiagram) {
	_ = w.Write([]string{"ATTACHMENT_ID", "PAGE_ID", "NAME", "KIND", "CONTENT_HASH"})
	for i := range diagrams {
		o.writePageDiagramCSVRow(w, &diagrams[i])
	}
}

func (o *Output) printPageDiagramCSV(w *csv.Writer, d *PageDiagram) {
	_ = w.Write([]string{"ATTACHMENT_ID", "PAGE_ID", "NAME", "KIND", "CONTENT_HASH"})
	o.writePageDiagramCSVRow(w, d)
}

func (o *Output) writePageDiagramCSVRow(w *csv.Writer, d *PageDiagram) {
	_ = w.Write([]string{
		fmt.Sprintf("%d", d.AttachmentID),
		fmt.Sprintf("%d", d.PageID),
		d.Name,
		d.Kind,
		d.ContentHash,
	})
}

// ============================================
// Link / LinkType formatters
// ============================================

func (o *Output) printLinkTypesTable(w *tabwriter.Writer, types []LinkType) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tALLOWED\tFORWARD\tREVERSE")
	_, _ = fmt.Fprintln(w, "--\t----\t-------\t-------\t-------")
	for i := range types {
		t := &types[i]
		allowed := "any"
		if len(t.AllowedEntityTypes) > 0 {
			allowed = strings.Join(t.AllowedEntityTypes, ",")
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", t.ID, t.Name, allowed, t.ForwardLabel, t.ReverseLabel)
	}
}

func (o *Output) printLinkTypesCSV(w *csv.Writer, types []LinkType) {
	_ = w.Write([]string{"ID", "NAME", "ALLOWED", "FORWARD", "REVERSE", "ACTIVE", "IS_SYSTEM"})
	for _, t := range types {
		allowed := ""
		if len(t.AllowedEntityTypes) > 0 {
			allowed = strings.Join(t.AllowedEntityTypes, ",")
		}
		_ = w.Write([]string{
			fmt.Sprintf("%d", t.ID),
			t.Name,
			allowed,
			t.ForwardLabel,
			t.ReverseLabel,
			fmt.Sprintf("%t", t.Active),
			fmt.Sprintf("%t", t.IsSystem),
		})
	}
}

// linkEndpointRef renders one side of a link as "TYPE:ID Title" for the
// table output. Items get the workspace-key form when available so the
// caller sees a usable handle (WI-7) rather than an opaque numeric id.
func linkEndpointRef(entityType string, id int, workspaceKey, title string) string {
	handle := fmt.Sprintf("%s:%d", entityType, id)
	if entityType == "item" && workspaceKey != "" {
		handle = fmt.Sprintf("%s-%d", workspaceKey, id)
	}
	if title == "" {
		return handle
	}
	return fmt.Sprintf("%s  %s", handle, truncateString(title, 50))
}

func (o *Output) printItemLinkDetailTable(w *tabwriter.Writer, l *ItemLink) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", l.ID)
	_, _ = fmt.Fprintf(w, "Type:\t%s (%s)\n", l.LinkTypeName, l.LinkTypeForwardLabel)
	_, _ = fmt.Fprintf(w, "Source:\t%s\n", linkEndpointRef(l.SourceType, l.SourceID, l.SourceWorkspaceKey, l.SourceTitle))
	_, _ = fmt.Fprintf(w, "Target:\t%s\n", linkEndpointRef(l.TargetType, l.TargetID, l.TargetWorkspaceKey, l.TargetTitle))
}

func (o *Output) printItemLinkCSV(w *csv.Writer, l *ItemLink) {
	_ = w.Write([]string{"ID", "TYPE", "SOURCE", "TARGET"})
	_ = w.Write([]string{
		fmt.Sprintf("%d", l.ID),
		l.LinkTypeName,
		linkEndpointRef(l.SourceType, l.SourceID, l.SourceWorkspaceKey, l.SourceTitle),
		linkEndpointRef(l.TargetType, l.TargetID, l.TargetWorkspaceKey, l.TargetTitle),
	})
}

// printLinkListTable renders outgoing + incoming links as two sections.
// Each row shows the link id, type label (forward for outgoing, reverse
// for incoming) and the *other* endpoint relative to the queried entity.
func (o *Output) printLinkListTable(w *tabwriter.Writer, resp *LinkListResponse) {
	writeRows := func(header string, links []ItemLink, otherSide func(ItemLink) (string, int, string, string), label func(ItemLink) string) {
		_, _ = fmt.Fprintf(w, "%s\n", header)
		_, _ = fmt.Fprintln(w, "ID\tTYPE\tLABEL\tENTITY")
		_, _ = fmt.Fprintln(w, "--\t----\t-----\t------")
		for _, l := range links {
			eType, eID, eWS, eTitle := otherSide(l)
			_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", l.ID, l.LinkTypeName, label(l), linkEndpointRef(eType, eID, eWS, eTitle))
		}
		if len(links) == 0 {
			_, _ = fmt.Fprintln(w, "(none)\t\t\t")
		}
		_, _ = fmt.Fprintln(w, "\t\t\t")
	}
	writeRows(
		"Outgoing",
		resp.Outgoing,
		func(l ItemLink) (string, int, string, string) {
			return l.TargetType, l.TargetID, l.TargetWorkspaceKey, l.TargetTitle
		},
		func(l ItemLink) string { return l.LinkTypeForwardLabel },
	)
	writeRows(
		"Incoming",
		resp.Incoming,
		func(l ItemLink) (string, int, string, string) {
			return l.SourceType, l.SourceID, l.SourceWorkspaceKey, l.SourceTitle
		},
		func(l ItemLink) string { return l.LinkTypeReverseLabel },
	)
}

func (o *Output) printLinkListCSV(w *csv.Writer, resp *LinkListResponse) {
	_ = w.Write([]string{"DIRECTION", "ID", "TYPE", "LABEL", "ENTITY"})
	for _, l := range resp.Outgoing {
		_ = w.Write([]string{
			"outgoing",
			fmt.Sprintf("%d", l.ID),
			l.LinkTypeName,
			l.LinkTypeForwardLabel,
			linkEndpointRef(l.TargetType, l.TargetID, l.TargetWorkspaceKey, l.TargetTitle),
		})
	}
	for _, l := range resp.Incoming {
		_ = w.Write([]string{
			"incoming",
			fmt.Sprintf("%d", l.ID),
			l.LinkTypeName,
			l.LinkTypeReverseLabel,
			linkEndpointRef(l.SourceType, l.SourceID, l.SourceWorkspaceKey, l.SourceTitle),
		})
	}
}

// ----------------------------------------------------------------------
// Asset table printers
// ----------------------------------------------------------------------

func (o *Output) printAssetsTable(w *tabwriter.Writer, assets []Asset) {
	_, _ = fmt.Fprintln(w, "ID\tTITLE\tTYPE\tSTATUS\tCATEGORY\tUPDATED")
	_, _ = fmt.Fprintln(w, "--\t-----\t----\t------\t--------\t-------")
	for _, a := range assets {
		title := truncateString(a.Title, 30)
		typeName := "-"
		if a.AssetType != nil {
			typeName = a.AssetType.Name
		}
		statusName := "-"
		if a.Status != nil {
			statusName = a.Status.Name
		}
		catName := "-"
		if a.Category != nil {
			catName = a.Category.Name
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", a.ID, title, typeName, statusName, catName, a.UpdatedAt)
	}
}

func (o *Output) printAssetDetailTable(w *tabwriter.Writer, a *Asset) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", a.ID)
	_, _ = fmt.Fprintf(w, "Title:\t%s\n", a.Title)
	if a.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:\t%s\n", truncateString(a.Description, 100))
	}
	if a.AssetTag != "" {
		_, _ = fmt.Fprintf(w, "Tag:\t%s\n", a.AssetTag)
	}
	if a.Set != nil {
		_, _ = fmt.Fprintf(w, "Set:\t%s (#%d)\n", a.Set.Name, a.Set.ID)
	} else {
		_, _ = fmt.Fprintf(w, "Set ID:\t%d\n", a.SetID)
	}
	if a.AssetType != nil {
		_, _ = fmt.Fprintf(w, "Type:\t%s (#%d)\n", a.AssetType.Name, a.AssetType.ID)
	} else {
		_, _ = fmt.Fprintf(w, "Type ID:\t%d\n", a.AssetTypeID)
	}
	if a.Status != nil {
		_, _ = fmt.Fprintf(w, "Status:\t%s\n", a.Status.Name)
	}
	if a.Category != nil {
		_, _ = fmt.Fprintf(w, "Category:\t%s\n", a.Category.Name)
	}
	if a.Creator != nil {
		// The asset surface does not expose creator.email under assets:read.
		name := a.Creator.FullName
		if name == "" {
			name = fmt.Sprintf("#%d", a.Creator.ID)
		}
		_, _ = fmt.Fprintf(w, "Creator:\t%s\n", name)
	}
	if a.LinkedItemCount > 0 {
		_, _ = fmt.Fprintf(w, "Linked items:\t%d\n", a.LinkedItemCount)
	}
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", a.CreatedAt)
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", a.UpdatedAt)
	for _, warn := range a.Warnings {
		_, _ = fmt.Fprintf(w, "Warning:\t%s\n", warn)
	}
}

func (o *Output) printAssetSetsTable(w *tabwriter.Writer, sets []AssetSet) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tASSETS\tTYPES\tDEFAULT\tROLE")
	_, _ = fmt.Fprintln(w, "--\t----\t------\t-----\t-------\t----")
	for _, s := range sets {
		def := ""
		if s.IsDefault {
			def = "yes"
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%s\t%s\n", s.ID, truncateString(s.Name, 30), s.AssetCount, s.AssetTypeCount, def, s.UserPermission)
	}
}

func (o *Output) printAssetSetDetailTable(w *tabwriter.Writer, s *AssetSet) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", s.ID)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", s.Name)
	if s.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:\t%s\n", truncateString(s.Description, 100))
	}
	if s.IsDefault {
		_, _ = fmt.Fprintf(w, "Default:\tyes\n")
	}
	_, _ = fmt.Fprintf(w, "Asset count:\t%d\n", s.AssetCount)
	_, _ = fmt.Fprintf(w, "Type count:\t%d\n", s.AssetTypeCount)
	if s.UserPermission != "" {
		_, _ = fmt.Fprintf(w, "Your role:\t%s\n", s.UserPermission)
	}
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", s.CreatedAt)
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", s.UpdatedAt)
}

func (o *Output) printAssetTypesTable(w *tabwriter.Writer, types []AssetType) {
	_, _ = fmt.Fprintln(w, "ID\tNAME\tFIELDS\tASSETS\tACTIVE")
	_, _ = fmt.Fprintln(w, "--\t----\t------\t------\t------")
	for _, t := range types {
		active := "no"
		if t.IsActive {
			active = "yes"
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%s\n", t.ID, truncateString(t.Name, 30), len(t.Fields), t.AssetCount, active)
	}
}

func (o *Output) printAssetTypeDetailTable(w *tabwriter.Writer, t *AssetType) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", t.ID)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", t.Name)
	if t.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:\t%s\n", truncateString(t.Description, 100))
	}
	_, _ = fmt.Fprintf(w, "Set ID:\t%d\n", t.SetID)
	_, _ = fmt.Fprintf(w, "Asset count:\t%d\n", t.AssetCount)
	active := "no"
	if t.IsActive {
		active = "yes"
	}
	_, _ = fmt.Fprintf(w, "Active:\t%s\n", active)
	if len(t.Fields) > 0 {
		_, _ = fmt.Fprintf(w, "Fields:\t%d declared\n", len(t.Fields))
		for _, f := range t.Fields {
			req := ""
			if f.IsRequired {
				req = " (required)"
			}
			_, _ = fmt.Fprintf(w, "  - %s\t%s%s\n", f.FieldName, f.FieldType, req)
		}
	}
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", t.CreatedAt)
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", t.UpdatedAt)
}

func (o *Output) printAssetImportJobTable(w *tabwriter.Writer, j *AssetImportJob) {
	_, _ = fmt.Fprintf(w, "Job ID:\t%s\n", j.JobID)
	_, _ = fmt.Fprintf(w, "Status:\t%s\n", j.Status)
	if j.Progress != nil {
		_, _ = fmt.Fprintf(w, "Rows:\t%d total, %d imported, %d errors\n", j.Progress.TotalRows, j.Progress.ImportedCount, j.Progress.FailedCount)
	}
	if j.ErrorMessage != "" {
		_, _ = fmt.Fprintf(w, "Error:\t%s\n", j.ErrorMessage)
	}
	if j.CreatedAt != nil {
		_, _ = fmt.Fprintf(w, "Created:\t%s\n", j.CreatedAt.Format(time.RFC3339))
	}
	if j.StartedAt != nil {
		_, _ = fmt.Fprintf(w, "Started:\t%s\n", j.StartedAt.Format(time.RFC3339))
	}
	if j.CompletedAt != nil {
		_, _ = fmt.Fprintf(w, "Completed:\t%s\n", j.CompletedAt.Format(time.RFC3339))
	}
}
