package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sort"

	"windshift/internal/jira"
)

const (
	defaultReadinessSample = 200
	maxReadinessSample     = 500
	readinessPageSize      = 100 // enhanced JQL search page size
)

// Readiness handles POST /api/admin/jira-import/readiness. It deep-scans a
// sample of each selected project's issues and returns a migration-readiness
// report: every Jira concept classified clean / lossy / blocked, with a
// usage-weighted 0–100 score per project and overall. Unlike Analyze (which
// returns raw mapping *suggestions* for the wizard), this answers "how cleanly
// will my instance actually migrate?" before the user commits.
func (h *JiraImportHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[JiraReadinessRequest](w, r)
	if !ok {
		return
	}
	if req.ConnectionID == "" || len(req.ProjectKeys) == 0 {
		respondValidationError(w, r, "connection_id and project_keys are required")
		return
	}

	client, err := h.getClientForConnection(r.Context(), req.ConnectionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	report := h.analyzeReadiness(r.Context(), client, req)
	respondJSONOK(w, report)
}

// analyzeReadiness is the I/O-driving core, split out so tests can inject a
// fake jira.Client (the same seam executeImportWithClientContext uses).
func (h *JiraImportHandler) analyzeReadiness(ctx context.Context, client jira.Client, req JiraReadinessRequest) JiraReadinessReport {
	sampleSize := req.SampleSize
	if sampleSize <= 0 {
		sampleSize = defaultReadinessSample
	}
	if sampleSize > maxReadinessSample {
		sampleSize = maxReadinessSample
	}

	report := JiraReadinessReport{
		Projects:       make([]JiraProjectReadiness, 0, len(req.ProjectKeys)),
		OpenIssuesOnly: req.OpenIssuesOnly,
	}

	// Custom-field definitions are instance-wide; fetch suggestions once and
	// index them by field ID ("customfield_10001") for per-issue usage lookup.
	fieldSuggestions := h.fieldSuggestionIndex(ctx, client, req.ProjectKeys)

	var allFindings []jira.Finding
	for _, projectKey := range req.ProjectKeys {
		pr, attachmentBytes := h.scanProject(ctx, client, projectKey, req.OpenIssuesOnly, sampleSize, fieldSuggestions)
		report.Projects = append(report.Projects, pr)
		report.TotalIssues += pr.TotalIssues
		report.SampledIssues += pr.SampledIssues
		report.AttachmentBytes += attachmentBytes
		if pr.SampledIssues < pr.TotalIssues {
			report.Extrapolated = true
		}
		allFindings = append(allFindings, pr.Findings...)
	}

	report.OverallScore, report.FindingsBySev = jira.ScoreFindings(allFindings)
	return report
}

// fieldSuggestionIndex fetches custom-field definitions (company-managed
// projects first, falling back to the all-fields endpoint) and returns them
// keyed by Jira field ID. Mirrors the field-fetch path in Analyze.
func (h *JiraImportHandler) fieldSuggestionIndex(ctx context.Context, client jira.Client, projectKeys []string) map[string]jira.FieldMappingSuggestion {
	projectIDs := make([]string, 0, len(projectKeys))
	for _, key := range projectKeys {
		project, err := client.GetProject(ctx, key)
		if err != nil || project == nil {
			continue
		}
		// Team-managed/next-gen projects don't support the field-screen API.
		if !project.Simplified && project.Style != "next-gen" {
			projectIDs = append(projectIDs, project.ID)
		}
	}

	fields, err := client.GetProjectFields(ctx, projectIDs)
	if err != nil {
		slog.Debug("readiness: GetProjectFields failed, falling back to ListCustomFields",
			slog.String("component", "jira"), slog.Any("error", err))
		fields, err = client.ListCustomFields(ctx)
		if err != nil {
			slog.Warn("readiness: could not list custom fields",
				slog.String("component", "jira"), slog.Any("error", err))
			return map[string]jira.FieldMappingSuggestion{}
		}
	}

	index := make(map[string]jira.FieldMappingSuggestion)
	for _, s := range jira.SuggestFieldMappings(fields) {
		index[s.JiraFieldID] = s
	}
	return index
}

// projectScanTally accumulates per-project counts while walking sampled issues.
type projectScanTally struct {
	sampled           int
	comments          int
	attachments       int
	attachmentBytes   int64
	labeledIssues     int
	components        int
	affectsVersions   int
	worklogs          int
	changelogs        int
	sprintIssues      int
	unsupportedADF    map[string]int  // node type -> occurrences
	customFieldUsage  map[string]int  // field ID -> issues using it
	unmappedFieldUse  int             // values seen for fields with no known mapping
	linkTypeUsage     map[string]int  // link type name -> link count
	usersMissingEmail map[string]bool // accountIDs lacking an email in the sample
}

func newProjectScanTally() *projectScanTally {
	return &projectScanTally{
		unsupportedADF:    make(map[string]int),
		customFieldUsage:  make(map[string]int),
		linkTypeUsage:     make(map[string]int),
		usersMissingEmail: make(map[string]bool),
	}
}

// scanProject samples one project's issues, tallies fidelity signals, and
// turns them into findings + a score.
func (h *JiraImportHandler) scanProject(ctx context.Context, client jira.Client, projectKey string, openOnly bool, sampleSize int, fields map[string]jira.FieldMappingSuggestion) (readiness JiraProjectReadiness, attachmentBytes int64) {
	pr := JiraProjectReadiness{Key: projectKey, Name: projectKey}

	var project *jira.JiraProject
	if fetched, err := client.GetProject(ctx, projectKey); err == nil && fetched != nil {
		project = fetched
		pr.Name = project.Name
	}

	if total, err := client.GetIssueCount(ctx, projectKey, openOnly); err == nil {
		pr.TotalIssues = total
	}

	hasSprints := false
	if boards, err := client.ListBoards(ctx, projectKey); err == nil && boards != nil && len(boards.Values) > 0 {
		hasSprints = true
	}

	issues := h.sampleIssues(ctx, client, projectKey, openOnly, sampleSize)
	tally := newProjectScanTally()
	for i := range issues {
		h.tallyIssue(&issues[i], fields, tally)
	}
	pr.SampledIssues = tally.sampled

	pr.Findings = buildFindings(tally, fields, hasSprints)
	pr.Findings = append(pr.Findings, h.jiraConfigurationReadinessFindings(
		ctx,
		client,
		project,
		projectKey,
		fields,
	)...)
	pr.Score, _ = jira.ScoreFindings(pr.Findings)
	return pr, tally.attachmentBytes
}

func (h *JiraImportHandler) jiraConfigurationReadinessFindings(
	ctx context.Context,
	client jira.Client,
	project *jira.JiraProject,
	projectKey string,
	fields map[string]jira.FieldMappingSuggestion,
) []jira.Finding {
	if project == nil {
		return []jira.Finding{{
			Entity:   "Workflow and screen configuration",
			Category: "configuration",
			Severity: jira.SeverityLossy,
			Reason:   "The Jira project metadata could not be read, so workflow and screen configuration fidelity could not be assessed.",
		}}
	}
	issueTypes, err := client.GetProjectIssueTypes(ctx, projectKey)
	if err != nil {
		return []jira.Finding{{
			Entity:   "Workflow and screen configuration",
			Category: "configuration",
			Severity: jira.SeverityLossy,
			Reason:   "The project's Jira issue types could not be read, so workflow and screen configuration fidelity could not be assessed.",
		}}
	}
	issueTypeIDs := make([]string, 0, len(issueTypes))
	for _, issueType := range issueTypes {
		issueTypeIDs = append(issueTypeIDs, issueType.ID)
	}
	sort.Strings(issueTypeIDs)

	findings := make([]jira.Finding, 0, 6)
	workflowClient, workflowCapable := client.(jira.WorkflowConfigurationClient)
	if !workflowCapable {
		findings = append(findings, jiraWorkflowUnavailableFinding())
	} else if config, configErr := workflowClient.GetProjectWorkflowConfiguration(
		ctx,
		project.ID,
		issueTypeIDs,
	); configErr != nil || config == nil {
		findings = append(findings, jiraWorkflowUnavailableFinding())
	} else {
		transitionCount := 0
		nonInitialTransitionCount := 0
		guardedCount := 0
		actionCount := 0
		triggerCount := 0
		for _, workflow := range config.Workflows {
			transitionCount += len(workflow.Transitions)
			for _, transition := range workflow.Transitions {
				if transition.Type != jira.JiraWorkflowTransitionInitial {
					nonInitialTransitionCount++
				}
				if transition.ValidatorCount > 0 || transition.ConditionCount > 0 {
					guardedCount++
				}
				actionCount += transition.ActionCount
				triggerCount += transition.TriggerCount
			}
		}
		findings = append(findings, jira.Finding{
			Entity:     "Workflow graph and issue-type assignments",
			Category:   "workflow",
			Severity:   jira.SeverityClean,
			Reason:     "Jira's configured workflow identities, status graph, initial/global/directed topology, and per-issue-type assignments are available for import.",
			UsageCount: max(transitionCount, 1),
		})
		if !config.RulesComplete && nonInitialTransitionCount > 0 {
			findings = append(findings, jira.Finding{
				Entity:     "Workflow rule visibility",
				Category:   "workflow_rule",
				Severity:   jira.SeverityLossy,
				Reason:     "Jira's current workflow read API does not expose complete condition trees. The graph imports, but rules that cannot be identified are not recreated; only transitions with exposed conditions or validators receive a generated review lock.",
				UsageCount: nonInitialTransitionCount,
			})
		}
		if guardedCount > 0 {
			findings = append(findings, jira.Finding{
				Entity:     "Workflow conditions and validators",
				Category:   "workflow_rule",
				Severity:   jira.SeverityBlocked,
				Reason:     "Jira transitions with exposed conditions or validators retain their topology but stay behind a generated review lock because Jira rule semantics do not safely map to the Windshift condition model.",
				UsageCount: guardedCount,
			})
		}
		if actionCount+triggerCount > 0 {
			findings = append(findings, jira.Finding{
				Entity:     "Workflow post-functions and triggers",
				Category:   "workflow_rule",
				Severity:   jira.SeverityLossy,
				Reason:     "Transition topology imports, but Jira post-functions/actions and triggers have no portable Windshift equivalent and are recorded only as import fidelity metadata.",
				UsageCount: actionCount + triggerCount,
			})
		}
	}

	screenClient, screenCapable := client.(jira.ScreenConfigurationClient)
	if !screenCapable {
		findings = append(findings, jiraScreenUnavailableFinding(project))
		return findings
	}
	screenConfig, screenErr := screenClient.GetProjectScreenConfiguration(
		ctx,
		project.ID,
		projectKey,
		issueTypeIDs,
	)
	if screenErr != nil || screenConfig == nil {
		findings = append(findings, jiraScreenUnavailableFinding(project))
		return findings
	}

	fieldCount := 0
	unsupportedFieldCount := 0
	flattenedTabCount := 0
	for _, screen := range screenConfig.Screens {
		fieldCount += len(screen.Fields)
		if screen.TabCount > 1 {
			flattenedTabCount += screen.TabCount
		}
		for _, field := range screen.Fields {
			if !jiraScreenFieldIsImportable(field.ID, fields) {
				unsupportedFieldCount++
			}
		}
	}
	findings = append(findings,
		jira.Finding{
			Entity:     "Create/edit/view screens",
			Category:   "screen",
			Severity:   jira.SeverityClean,
			Reason:     "Jira issue-type screen schemes and operation-specific create/edit/view field order are available for Windshift screen assignments.",
			UsageCount: max(fieldCount, 1),
		},
		jira.Finding{
			Entity:     "Jira field configuration",
			Category:   "screen",
			Severity:   jira.SeverityLossy,
			Reason:     "Screen membership and field order import, but Jira hidden/required field-configuration rules are not exposed by the screen APIs and are not inferred; only Windshift's required title invariant is applied.",
			UsageCount: max(len(screenConfig.Screens), 1),
		},
	)
	if flattenedTabCount > 0 {
		findings = append(findings, jira.Finding{
			Entity:     "Screen tabs",
			Category:   "screen",
			Severity:   jira.SeverityLossy,
			Reason:     "Windshift has no screen-tab model; fields from multiple Jira tabs are flattened in tab and field order.",
			UsageCount: flattenedTabCount,
		})
	}
	if unsupportedFieldCount > 0 {
		findings = append(findings, jira.Finding{
			Entity:     "Screen fields without an imported field",
			Category:   "screen",
			Severity:   jira.SeverityLossy,
			Reason:     "Jira screen entries with no Windshift system-field mapping or imported custom-field definition are omitted from the imported screen.",
			UsageCount: unsupportedFieldCount,
		})
	}
	return findings
}

func jiraWorkflowUnavailableFinding() jira.Finding {
	return jira.Finding{
		Entity:   "Configured workflow graph",
		Category: "workflow",
		Severity: jira.SeverityLossy,
		Reason:   "The configured Jira workflow graph is unavailable with this deployment or credential. The importer creates a deterministic initial status but does not invent directed transitions.",
	}
}

func jiraScreenUnavailableFinding(project *jira.JiraProject) jira.Finding {
	reason := "The Jira issue-type screen configuration is unavailable with this deployment or credential, so Windshift screens cannot be reconstructed."
	if project != nil && (project.Simplified || project.Style == "next-gen") {
		reason = "This is a team-managed Jira project; Jira's company-managed screen scheme APIs do not expose its layout, so Windshift screens cannot be reconstructed."
	}
	return jira.Finding{
		Entity:   "Create/edit/view screens",
		Category: "screen",
		Severity: jira.SeverityLossy,
		Reason:   reason,
	}
}

func jiraScreenFieldIsImportable(
	jiraFieldID string,
	fields map[string]jira.FieldMappingSuggestion,
) bool {
	switch jiraFieldID {
	case "summary", "description", "status", "priority", "assignee", "duedate", "fixVersions", "labels":
		return true
	}
	suggestion, ok := fields[jiraFieldID]
	return ok && suggestion.WindshiftFieldType != jira.FieldTypeUnmapped
}

// sampleIssues pages through a project's issues (newest first) up to limit,
// requesting all fields plus the changelog so the tally can see history,
// comment bodies, and custom-field values.
func (h *JiraImportHandler) sampleIssues(ctx context.Context, client jira.Client, projectKey string, openOnly bool, limit int) []jira.JiraIssue {
	jql := `project = "` + escapeHandlerJQLString(projectKey) + `" ORDER BY created DESC`
	if openOnly {
		jql = `project = "` + escapeHandlerJQLString(projectKey) + `" AND statusCategory != Done ORDER BY created DESC`
	}

	var out []jira.JiraIssue
	nextPageToken := ""
	for len(out) < limit {
		res, err := client.SearchIssuesJQL(ctx, jira.JQLSearchRequest{
			JQL:           jql,
			MaxResults:    readinessPageSize,
			Fields:        []string{"*all"},
			Expand:        []string{"changelog"},
			NextPageToken: nextPageToken,
		})
		if err != nil {
			slog.Warn("readiness: issue sample failed",
				slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			break
		}
		if res == nil || len(res.Issues) == 0 {
			break
		}
		out = append(out, res.Issues...)
		if res.NextPageToken == "" {
			break // last page
		}
		nextPageToken = res.NextPageToken
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// tallyIssue folds one sampled issue's fidelity signals into the running tally.
func (h *JiraImportHandler) tallyIssue(issue *jira.JiraIssue, fields map[string]jira.FieldMappingSuggestion, t *projectScanTally) {
	t.sampled++
	f := &issue.Fields

	// Rich formatting in the description.
	for node, n := range jira.UnsupportedADFNodes(jira.ScanADF(f.Description)) {
		t.unsupportedADF[node] += n
	}

	// Comments (clean) + their formatting.
	if f.Comment != nil {
		for ci := range f.Comment.Comments {
			t.comments++
			for node, n := range jira.UnsupportedADFNodes(jira.ScanADF(f.Comment.Comments[ci].Body)) {
				t.unsupportedADF[node] += n
			}
		}
	}

	t.attachments += len(f.Attachment)
	for _, att := range f.Attachment {
		t.attachmentBytes += att.Size
	}

	if len(f.Labels) > 0 {
		t.labeledIssues++
	}
	if len(f.Components) > 0 {
		t.components++
	}
	if len(f.Versions) > 0 {
		t.affectsVersions++
	}
	if (f.Worklog != nil && f.Worklog.Total > 0) || (f.TimeTracking != nil && f.TimeTracking.TimeSpentSeconds > 0) {
		t.worklogs++
	}
	if issue.Changelog != nil && issue.Changelog.Total > 0 {
		t.changelogs++
	}
	if f.Sprint != nil {
		t.sprintIssues++
	}

	for _, link := range f.IssueLinks {
		if link.Type != nil {
			t.linkTypeUsage[link.Type.Name]++
		}
	}

	for _, u := range []*jira.JiraUser{f.Assignee, f.Reporter, f.Creator} {
		if u != nil && u.EmailAddress == "" {
			if id := u.GetIdentifier(); id != "" {
				t.usersMissingEmail[id] = true
			}
		}
	}

	for fieldID, val := range f.CustomFields {
		if val == nil {
			continue
		}
		if _, known := fields[fieldID]; known {
			t.customFieldUsage[fieldID]++
		} else {
			t.unmappedFieldUse++
		}
	}
}

// buildFindings converts a completed tally into the per-project finding list.
func buildFindings(t *projectScanTally, fields map[string]jira.FieldMappingSuggestion, hasSprints bool) []jira.Finding {
	findings := make([]jira.Finding, 0, 16)

	// Clean core: the bulk of every issue (summary, description text, status,
	// type, priority, dates, assignee/reporter/creator) imports 1:1. Weighting
	// this by the sample size anchors the score against the lossy/blocked tail.
	if t.sampled > 0 {
		findings = append(findings, jira.Finding{
			Entity:     "Core issue fields",
			Category:   "core",
			Severity:   jira.SeverityClean,
			Reason:     "Summary, description text, status (by category), issue type, priority, due/created/updated dates, assignee/reporter/creator, and the searchable original Jira key import 1:1.",
			UsageCount: t.sampled,
		})
	}
	if t.comments > 0 {
		findings = append(findings, jira.Finding{
			Entity: "Comments", Category: "comments", Severity: jira.SeverityClean,
			Reason: "Comment bodies import with @mention resolution and original timestamps.", UsageCount: t.comments,
		})
	}
	if t.attachments > 0 {
		findings = append(findings, jira.Finding{
			Entity: "Attachments", Category: "attachments", Severity: jira.SeverityClean,
			Reason: "Files download and re-attach when attachment storage is configured on the Windshift side.", UsageCount: t.attachments,
		})
	}
	if t.labeledIssues > 0 {
		findings = append(findings, jira.Finding{
			Entity: "Labels", Category: "labels", Severity: jira.SeverityClean,
			Reason: "Labels import into the global item-label catalog.", UsageCount: t.labeledIssues,
		})
	}
	for name, n := range t.linkTypeUsage {
		findings = append(findings, jira.Finding{
			Entity: "Issue links: " + name, Category: "links", Severity: jira.SeverityClean,
			Reason: "Link type and direction are preserved between imported issues; links to Jira issues outside the selected projects become durable item-facing integration links.", UsageCount: n,
		})
	}

	// Custom fields actually used in the sample.
	for fieldID, n := range t.customFieldUsage {
		findings = append(findings, jira.ClassifyField(fields[fieldID], n))
	}
	if t.unmappedFieldUse > 0 {
		findings = append(findings, jira.Finding{
			Entity: "Unmapped custom fields", Category: "custom_field", Severity: jira.SeverityBlocked,
			Reason: "Values belong to custom fields with no known Windshift mapping (typically third-party/app fields) and are not imported.", UsageCount: t.unmappedFieldUse,
		})
	}

	// Lossy: rich formatting flattened to text.
	for node, n := range t.unsupportedADF {
		findings = append(findings, jira.Finding{
			Entity: "Rich formatting: " + node, Category: "formatting", Severity: jira.SeverityLossy,
			Reason: "This ADF node type has no Markdown equivalent in the importer and is flattened to its text content.", UsageCount: n,
		})
	}
	if hasSprints {
		weight := t.sprintIssues
		if weight == 0 {
			weight = 1
		}
		findings = append(findings, jira.Finding{
			Entity: "Sprints / iterations", Category: "iteration", Severity: jira.SeverityClean,
			Reason: "Boards/sprints import as Windshift iterations (name, start/end dates, state) and each issue's sprint membership is assigned to the imported item.", UsageCount: weight,
		})
	}
	if len(t.usersMissingEmail) > 0 {
		findings = append(findings, jira.Finding{
			Entity: "Users without a visible email", Category: "users", Severity: jira.SeverityLossy,
			Reason: "Some assignees/reporters expose no email (e.g. Cloud GDPR settings); the importer creates an inactive user with a deterministic synthetic address, so the identity is preserved but the real email is not.", UsageCount: len(t.usersMissingEmail),
		})
	}

	// Lossy: imported, but as metadata or under conditions rather than as a
	// first-class Windshift concept.
	if t.components > 0 {
		findings = append(findings, jira.Finding{
			Entity: "Components", Category: "components", Severity: jira.SeverityLossy,
			Reason: "Jira components have no first-class Windshift equivalent; they are preserved as read-only metadata on the item, not as editable components.", UsageCount: t.components,
		})
	}
	if t.affectsVersions > 0 {
		findings = append(findings, jira.Finding{
			Entity: "Affects versions", Category: "version", Severity: jira.SeverityLossy,
			Reason: "Only fixVersions map to milestones; affects-versions are preserved as item metadata (and an optional 'Jira Affects Version/s' custom field) rather than as milestones.", UsageCount: t.affectsVersions,
		})
	}
	if t.worklogs > 0 {
		findings = append(findings, jira.Finding{
			Entity: "Worklogs / time tracking", Category: "worklog", Severity: jira.SeverityLossy,
			Reason: "Worklog entries import into Windshift time tracking when the import maps a time project; without one they are skipped, and only the worklogs returned in the issue payload are imported, so very long histories may be truncated. Estimates are kept as item metadata.", UsageCount: t.worklogs,
		})
	}

	// Blocked: data with no Windshift home today.
	if t.changelogs > 0 {
		findings = append(findings, jira.Finding{
			Entity: "Issue history / changelog", Category: "changelog", Severity: jira.SeverityBlocked,
			Reason: "Field-change history is not imported; items start with a fresh Windshift history.", UsageCount: t.changelogs,
		})
	}

	return findings
}
