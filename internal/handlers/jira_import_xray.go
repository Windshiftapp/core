package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"windshift/internal/jira"
	"windshift/internal/jiraimport"
	"windshift/internal/models"
	"windshift/internal/sanitize"
	"windshift/internal/xray"
)

type xrayImportPlan struct {
	keysByProject map[string]map[string]struct{}
	cloud         xray.DefinitionClient
	dataCenter    jira.XrayTestStepReader
	total         int
}

func prepareXrayImport(
	ctx context.Context,
	client jira.Client,
	options XrayImportOptions,
	projectKeys []string,
	openOnly bool,
) (*xrayImportPlan, error) {
	if !options.ImportTests {
		return nil, nil
	}
	lister, ok := client.(jira.XrayTestKeyLister)
	if !ok {
		return nil, fmt.Errorf("connected Jira client does not support Xray discovery")
	}

	plan := &xrayImportPlan{keysByProject: make(map[string]map[string]struct{}, len(projectKeys))}
	var issueTypeIDs []string
	if detector, isCloud := client.(jira.XrayTestIssueTypeDetector); isCloud {
		var allIssueTypes []jira.JiraIssueType
		for _, projectKey := range projectKeys {
			issueTypes, err := client.GetProjectIssueTypes(ctx, projectKey)
			if err != nil {
				return nil, fmt.Errorf("load issue types for Xray project %s: %w", projectKey, err)
			}
			allIssueTypes = append(allIssueTypes, issueTypes...)
		}
		detected, err := detector.DetectXrayTestIssueTypes(ctx, allIssueTypes)
		if err != nil {
			return nil, fmt.Errorf("verify Xray Test issue types: %w", err)
		}
		if len(detected) == 0 {
			return nil, fmt.Errorf("the selected projects no longer expose an Xray-owned Test issue type")
		}
		issueTypeIDs = detected

		cloud, err := xray.NewCloudClient(xray.CloudConfig{
			ClientID:     options.ClientID,
			ClientSecret: options.ClientSecret,
			Region:       options.Region,
		})
		if err != nil {
			return nil, err
		}
		if err := cloud.Validate(ctx); err != nil {
			return nil, err
		}
		plan.cloud = cloud
	} else {
		reader, ok := client.(jira.XrayTestStepReader)
		if !ok {
			return nil, fmt.Errorf("connected Jira Data Center client cannot read Xray Test steps")
		}
		plan.dataCenter = reader
	}

	for _, projectKey := range projectKeys {
		keys, err := lister.ListXrayTestKeys(ctx, projectKey, issueTypeIDs, openOnly)
		if err != nil {
			return nil, fmt.Errorf("list Xray Tests for project %s: %w", projectKey, err)
		}
		projectKeysSet := make(map[string]struct{}, len(keys))
		for _, key := range keys {
			if normalized := strings.TrimSpace(key); normalized != "" {
				projectKeysSet[normalized] = struct{}{}
			}
		}
		plan.keysByProject[projectKey] = projectKeysSet
		plan.total += len(projectKeysSet)
	}
	if plan.total == 0 {
		return nil, fmt.Errorf("no Xray Tests remain in the selected project scope")
	}
	return plan, nil
}

func (p *xrayImportPlan) isTest(projectKey, issueKey string) bool {
	if p == nil {
		return false
	}
	_, ok := p.keysByProject[projectKey][issueKey]
	return ok
}

func (p *xrayImportPlan) definitions(
	ctx context.Context,
	projectKey string,
	issues []jira.JiraIssue,
) (definitions map[string]xray.Test, failures map[string]error) {
	definitions = make(map[string]xray.Test)
	failures = make(map[string]error)
	if p == nil {
		return definitions, failures
	}

	var tests []jira.JiraIssue
	for _, issue := range issues {
		if p.isTest(projectKey, issue.Key) {
			tests = append(tests, issue)
		}
	}
	if len(tests) == 0 {
		return definitions, failures
	}

	if p.cloud != nil {
		issueIDs := make([]string, 0, len(tests))
		for _, issue := range tests {
			issueIDs = append(issueIDs, issue.ID)
		}
		loaded, err := p.cloud.GetTests(ctx, issueIDs)
		if err != nil {
			for _, issue := range tests {
				failures[issue.Key] = err
			}
			return definitions, failures
		}
		for _, definition := range loaded {
			definitions[definition.IssueID] = definition
		}
		for _, issue := range tests {
			if _, exists := definitions[issue.ID]; !exists {
				failures[issue.Key] = fmt.Errorf("xray Cloud returned no definition for Jira issue %s", issue.Key)
			}
		}
		return definitions, failures
	}

	for _, issue := range tests {
		steps, err := p.dataCenter.GetXrayTestSteps(ctx, issue.Key)
		if err != nil {
			failures[issue.Key] = err
			continue
		}
		definitions[issue.ID] = xray.Test{
			IssueID:      issue.ID,
			TestTypeName: "Manual",
			Steps:        steps,
		}
	}
	return definitions, failures
}

// importXrayTestCase writes a Test and all of its manual steps atomically. A
// Jira issue routed here must not also be passed to importIssue.
func (h *JiraImportHandler) importXrayTestCase(
	jobID string,
	workspaceID int,
	issue *jira.JiraIssue,
	definition xray.Test,
) (int, error) {
	if issue == nil || strings.TrimSpace(issue.ID) == "" {
		return 0, fmt.Errorf("xray Test is missing its Jira issue ID")
	}
	title := sanitize.PlainTextField.Sanitize(issue.Fields.Summary)
	if title == "" {
		title = sanitize.PlainTextField.Sanitize(issue.Key)
	}
	if title == "" {
		return 0, fmt.Errorf("xray Test %s has no title", issue.ID)
	}

	createdAt := parseJiraImportTime(issue.Fields.Created)
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := parseJiraImportTime(issue.Fields.Updated)
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	steps := make([]models.TestStep, 0, len(definition.Steps))
	for index, source := range definition.Steps {
		steps = append(steps, models.TestStep{
			StepNumber: index + 1,
			Action:     sanitize.Comment.Sanitize(source.Action),
			Data:       sanitize.Comment.Sanitize(source.Data),
			Expected:   sanitize.Comment.Sanitize(source.Expected),
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
		})
	}
	labels := make([]models.TestLabel, 0, len(issue.Fields.Labels))
	seen := make(map[string]struct{}, len(issue.Fields.Labels))
	for _, source := range issue.Fields.Labels {
		name := sanitize.PlainTextField.Sanitize(source)
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		labels = append(labels, models.TestLabel{
			WorkspaceID: workspaceID,
			Name:        name,
			Color:       "#3B82F6",
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
		})
	}
	return h.imports.CreateXrayTestCase(jiraimport.XrayTestCaseInput{
		JobID:   jobID,
		JiraID:  issue.ID,
		JiraKey: issue.Key,
		TestCase: models.TestCase{
			WorkspaceID: workspaceID,
			Title:       title,
			Priority:    jiraTestCasePriority(issue.Fields.Priority),
			Status:      "active",
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		},
		Steps:        steps,
		Labels:       labels,
		XrayTestType: definition.TestTypeName,
	})
}

func jiraTestCasePriority(priority *jira.JiraPriority) string {
	if priority == nil {
		return "medium"
	}
	switch strings.ToLower(strings.TrimSpace(priority.Name)) {
	case "highest", "blocker", "critical":
		return "critical"
	case "high", "major":
		return "high"
	case "low", "lowest", "minor", "trivial":
		return "low"
	default:
		return "medium"
	}
}

func parseJiraImportTime(value string) time.Time {
	if strings.TrimSpace(value) != "" {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000-0700"} {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}
