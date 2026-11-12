package jira

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"
)

// workflow_datacenter.go reads the configured workflow graph for Jira Data
// Center.
//
// Data Center's public REST API only returns workflow name/description/steps,
// but the admin workflow designer loads the full graph from
// `/rest/workflowDesigner/latest/workflows`. That payload carries the statuses
// (with initial flags), the transitions (source, target, global/initial), and
// per-transition counts for Conditions, Validators, Post Functions, and
// Triggers. Guard *definitions* are not exposed, so guarded transitions are
// imported review-locked rather than recreated.

type jiraWorkflowDesignerResponse struct {
	IsDraft bool `json:"isDraft"`
	Layout  struct {
		Statuses []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Initial  bool   `json:"initial"`
			StatusID string `json:"statusId"`
		} `json:"statuses"`
		Transitions []struct {
			ID                string `json:"id"`
			Name              string `json:"name"`
			Description       string `json:"description"`
			SourceID          string `json:"sourceId"`
			TargetID          string `json:"targetId"`
			Initial           bool   `json:"initial"`
			GlobalTransition  bool   `json:"globalTransition"`
			LoopedTransition  bool   `json:"loopedTransition"`
			TransitionOptions []struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			} `json:"transitionOptions"`
		} `json:"transitions"`
	} `json:"layout"`
}

type jiraProjectWorkflowScheme struct {
	ID                int               `json:"id"`
	DefaultWorkflow   string            `json:"defaultWorkflow"`
	IssueTypeMappings map[string]string `json:"issueTypeMappings"`
}

// GetProjectWorkflowConfiguration resolves the workflow each requested issue
// type uses and reads each workflow's graph from the designer API. It returns
// ErrWorkflowConfigurationNotAvailable when the source cannot provide it, so
// the importer keeps its status-membership fallback.
func (c *dataCenterClient) GetProjectWorkflowConfiguration(
	ctx context.Context,
	projectID string,
	issueTypeIDs []string,
) (*JiraProjectWorkflowConfiguration, error) {
	// Prefer the companion plugin when installed: it exposes the condition and
	// validator descriptors the designer API only counts. Any failure falls
	// through to the designer reader.
	if caps, ok := c.companionCapabilities(ctx); ok && companionSupports(caps, "workflows") {
		if config, err := c.companionWorkflowConfiguration(ctx, projectID, issueTypeIDs); err == nil && config != nil {
			return config, nil
		}
	}

	var scheme jiraProjectWorkflowScheme
	schemeURL := c.baseURL + "/project/" + url.PathEscape(projectID) + "/workflowscheme"
	if err := jiraGetJSON(ctx, c, schemeURL, &scheme); err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) {
			return nil, ErrWorkflowConfigurationNotAvailable
		}
		return nil, err
	}
	if scheme.DefaultWorkflow == "" && len(scheme.IssueTypeMappings) == 0 {
		return nil, ErrWorkflowConfigurationNotAvailable
	}

	issueTypeWorkflowIDs := make(map[string]string, len(issueTypeIDs))
	workflowNames := make(map[string]struct{}, len(issueTypeIDs))
	for _, issueTypeID := range issueTypeIDs {
		name := scheme.IssueTypeMappings[issueTypeID]
		if name == "" {
			name = scheme.DefaultWorkflow
		}
		if name == "" {
			return nil, ErrWorkflowConfigurationNotAvailable
		}
		issueTypeWorkflowIDs[issueTypeID] = name
		workflowNames[name] = struct{}{}
	}

	names := make([]string, 0, len(workflowNames))
	for name := range workflowNames {
		names = append(names, name)
	}
	sort.Strings(names)

	config := &JiraProjectWorkflowConfiguration{
		IssueTypeWorkflowIDs: issueTypeWorkflowIDs,
		Workflows:            make([]JiraWorkflowConfiguration, 0, len(names)),
		// The designer API exposes guard counts, not definitions, so rules are
		// never claimed complete; guarded transitions stay review-locked.
		RulesComplete: false,
	}
	for _, name := range names {
		layout, err := c.getWorkflowDesignerLayout(ctx, name)
		if err != nil {
			return nil, err
		}
		config.Workflows = append(config.Workflows, jiraWorkflowConfigurationFromDesignerLayout(name, layout))
	}
	return config, nil
}

func (c *dataCenterClient) getWorkflowDesignerLayout(ctx context.Context, workflowName string) (*jiraWorkflowDesignerResponse, error) {
	designerURL := jiraSiteURLFromBase(c.baseURL) + "/rest/workflowDesigner/latest/workflows?" + url.Values{
		"name":  {workflowName},
		"draft": {"false"},
	}.Encode()

	var layout jiraWorkflowDesignerResponse
	if err := jiraGetJSON(ctx, c, designerURL, &layout); err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) {
			return nil, ErrWorkflowConfigurationNotAvailable
		}
		return nil, err
	}
	return &layout, nil
}

// jiraWorkflowConfigurationFromDesignerLayout maps a designer layout to the
// importer's workflow configuration. Unknown targets (for example the initial
// pseudo-status) are skipped rather than guessed.
func jiraWorkflowConfigurationFromDesignerLayout(name string, layout *jiraWorkflowDesignerResponse) JiraWorkflowConfiguration {
	statusIDByHandle := make(map[string]string, len(layout.Layout.Statuses))
	workflow := JiraWorkflowConfiguration{ID: name, Name: name}
	for _, status := range layout.Layout.Statuses {
		if status.StatusID == "" {
			// The initial pseudo-status (e.g. "I<1>") has no Jira status id.
			continue
		}
		statusIDByHandle[status.ID] = status.StatusID
		workflow.StatusIDs = append(workflow.StatusIDs, status.StatusID)
	}
	sort.Strings(workflow.StatusIDs)

	for _, transition := range layout.Layout.Transitions {
		toStatusID, ok := statusIDByHandle[transition.TargetID]
		if !ok {
			continue
		}
		counts := make(map[string]int, len(transition.TransitionOptions))
		for _, option := range transition.TransitionOptions {
			counts[option.Name] = option.Count
		}
		configured := JiraConfiguredWorkflowTransition{
			ID:             transition.ID,
			Name:           transition.Name,
			Description:    transition.Description,
			ToStatusID:     toStatusID,
			ConditionCount: counts["Conditions"],
			ValidatorCount: counts["Validators"],
			ActionCount:    counts["Post Functions"],
			TriggerCount:   counts["Triggers"],
		}
		switch {
		case transition.GlobalTransition:
			configured.Type = JiraWorkflowTransitionGlobal
		case transition.Initial || strings.HasPrefix(transition.SourceID, "I<"):
			configured.Type = JiraWorkflowTransitionInitial
		default:
			fromStatusID, ok := statusIDByHandle[transition.SourceID]
			if !ok {
				continue
			}
			configured.Type = JiraWorkflowTransitionDirected
			configured.FromStatusIDs = []string{fromStatusID}
		}
		workflow.Transitions = append(workflow.Transitions, configured)
	}
	return workflow
}
