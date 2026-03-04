package main

import (
	"encoding/csv"
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
	case "csv":
		o.printCSV(data)
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
	case []Milestone:
		o.printMilestonesTable(w, v)
	case *Milestone:
		o.printMilestoneDetailTable(w, v)
	case *MilestoneProgress:
		o.printMilestoneProgressTable(w, v)
	default:
		// Fallback to JSON for unknown types
		o.printJSON(data)
	}
}

func (o *Output) printCSV(data interface{}) {
	w := csv.NewWriter(os.Stdout)
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
	case []Milestone:
		o.printMilestonesCSV(w, v)
	case *Milestone:
		o.printMilestoneCSV(w, v)
	case *MilestoneProgress:
		o.printMilestoneProgressCSV(w, v)
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
		_ = w.Write([]string{key, item.Title, status, assignee, itemType})
	}
}

func (o *Output) printItemCSV(w *csv.Writer, item *Item) {
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
	priority := ""
	if item.Priority != nil {
		priority = item.Priority.Name
	}
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
	_ = w.Write([]string{fmt.Sprintf("%d", run.ID), run.Name, fmt.Sprintf("%d", run.SetID), assignee, started, ended, status})
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
			author = c.Author.Name
		}
		created := c.CreatedAt.Format("2006-01-02 15:04")
		_ = w.Write([]string{fmt.Sprintf("%d", c.ID), author, created, c.Content})
	}
}

func (o *Output) printCommentCSV(w *csv.Writer, c *Comment) {
	author := ""
	if c.Author != nil {
		author = c.Author.Name
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
	// First print milestone details
	o.printMilestoneDetailTable(w, &p.Milestone)

	// Then print progress
	_, _ = fmt.Fprintln(w, "\nProgress:")
	_, _ = fmt.Fprintf(w, "  Total Items:\t%d\n", p.TotalItems)
	if len(p.ItemsByStatus) > 0 {
		_, _ = fmt.Fprintln(w, "  By Status:")
		for status, count := range p.ItemsByStatus {
			_, _ = fmt.Fprintf(w, "    %s:\t%d\n", status, count)
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
	// Print milestone info with progress
	target := ""
	if p.Milestone.TargetDate != nil {
		target = *p.Milestone.TargetDate
	}

	// Flatten items by status into a string
	statusParts := make([]string, 0, len(p.ItemsByStatus))
	for status, count := range p.ItemsByStatus {
		statusParts = append(statusParts, fmt.Sprintf("%s:%d", status, count))
	}
	statusBreakdown := strings.Join(statusParts, ";")

	_ = w.Write([]string{"ID", "NAME", "STATUS", "TARGET_DATE", "TOTAL_ITEMS", "ITEMS_BY_STATUS"})
	_ = w.Write([]string{fmt.Sprintf("%d", p.Milestone.ID), p.Milestone.Name, p.Milestone.Status, target, fmt.Sprintf("%d", p.TotalItems), statusBreakdown})
}
