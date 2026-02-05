package main

import (
	"encoding/json"
	"fmt"
	"os"
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
func (o *Output) Print(data interface{}) {
	switch o.format {
	case "table":
		o.printTable(data)
	default:
		o.printJSON(data)
	}
}

// PrintError outputs an error in the configured format
func (o *Output) PrintError(err error) {
	if o.format == "json" {
		output := map[string]string{"error": err.Error()}
		var jsonBytes []byte
		jsonBytes, err = json.Marshal(output)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
			return
		}
		_, _ = fmt.Fprintln(os.Stderr, string(jsonBytes))
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
	}
}

func (o *Output) printJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(data) //nolint:errcheck // output to stdout
}

func (o *Output) printTable(data interface{}) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
	default:
		// Fallback to JSON for unknown types
		o.printJSON(data)
	}
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
	for _, item := range items {
		key := item.Key
		if key == "" {
			key = fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
		}
		status := ""
		if item.Status != nil {
			status = item.Status.Name
		}
		assignee := ""
		if item.Assignee != nil {
			assignee = item.Assignee.Name
		}
		itemType := ""
		if item.ItemType != nil {
			itemType = item.ItemType.Name
		}
		// Truncate long titles
		title := item.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", key, title, status, assignee, itemType)
	}
}

func (o *Output) printItemDetailTable(w *tabwriter.Writer, item *Item) {
	key := item.Key
	if key == "" {
		key = fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
	}
	_, _ = fmt.Fprintf(w, "Key:\t%s\n", key)
	_, _ = fmt.Fprintf(w, "Title:\t%s\n", item.Title)
	if item.Status != nil {
		_, _ = fmt.Fprintf(w, "Status:\t%s\n", item.Status.Name)
	}
	if item.ItemType != nil {
		_, _ = fmt.Fprintf(w, "Type:\t%s\n", item.ItemType.Name)
	}
	if item.Priority != nil {
		_, _ = fmt.Fprintf(w, "Priority:\t%s\n", item.Priority.Name)
	}
	if item.Assignee != nil {
		_, _ = fmt.Fprintf(w, "Assignee:\t%s\n", item.Assignee.Name)
	}
	if item.Creator != nil {
		_, _ = fmt.Fprintf(w, "Creator:\t%s\n", item.Creator.Name)
	}
	if item.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:\t%s\n", truncateString(item.Description, 100))
	}
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", item.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", item.UpdatedAt.Format(time.RFC3339))

	if len(item.Transitions) > 0 {
		_, _ = fmt.Fprintln(w, "\nAvailable Transitions:")
		for _, t := range item.Transitions {
			if t.ToStatus != nil {
				_, _ = fmt.Fprintf(w, "  - %s (ID: %d)\n", t.ToStatus.Name, t.ToStatusID)
			}
		}
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
	_, _ = fmt.Fprintf(w, "Set ID:\t%d\n", run.SetID)
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
			author = c.Author.Name
		}
		created := c.CreatedAt.Format("2006-01-02 15:04")
		content := truncateString(c.Content, 50)
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", c.ID, author, created, content)
	}
}

func (o *Output) printCommentDetailTable(w *tabwriter.Writer, c *Comment) {
	_, _ = fmt.Fprintf(w, "ID:\t%d\n", c.ID)
	_, _ = fmt.Fprintf(w, "Item ID:\t%d\n", c.ItemID)
	if c.Author != nil {
		_, _ = fmt.Fprintf(w, "Author:\t%s\n", c.Author.Name)
	}
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", c.CreatedAt.Format("2006-01-02 15:04:05"))
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", c.UpdatedAt.Format("2006-01-02 15:04:05"))
	_, _ = fmt.Fprintf(w, "Content:\n%s\n", c.Content)
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
