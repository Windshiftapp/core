package jira

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// companion.go reads the Windshift DC import companion plugin's API. The plugin
// exposes the workflow graph with condition/validator descriptors, which the
// public Data Center REST API and the workflow designer API do not. Everything
// here is optional: a missing plugin falls back to the designer reader.

const companionBasePath = "/rest/windshift-import/1.0"

type companionCapabilities struct {
	PluginVersion  string   `json:"pluginVersion"`
	DeploymentType string   `json:"deploymentType"`
	Capabilities   []string `json:"capabilities"`
}

type companionWorkflowsResponse struct {
	DefaultWorkflow      string              `json:"defaultWorkflow"`
	IssueTypeWorkflowIDs map[string]string   `json:"issueTypeWorkflowIds"`
	Workflows            []companionWorkflow `json:"workflows"`
}

type companionWorkflow struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Statuses    []companionStatus     `json:"statuses"`
	Transitions []companionTransition `json:"transitions"`
}

type companionStatus struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type companionTransition struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Type          string           `json:"type"`
	FromStatusIDs []string         `json:"fromStatusIds"`
	ToStatusID    string           `json:"toStatusId"`
	Conditions    []companionGuard `json:"conditions"`
	Validators    []companionGuard `json:"validators"`
}

type companionGuard struct {
	Type  string         `json:"type"`
	Class string         `json:"class"`
	Kind  string         `json:"kind"`
	Args  map[string]any `json:"args"`
}

func (c *dataCenterClient) companionCapabilities(ctx context.Context) (*companionCapabilities, bool) {
	var caps companionCapabilities
	endpoint := jiraSiteURLFromBase(c.baseURL) + companionBasePath + "/capabilities"
	if err := jiraGetJSON(ctx, c, endpoint, &caps); err != nil {
		return nil, false
	}
	return &caps, true
}

func companionSupports(caps *companionCapabilities, capability string) bool {
	for _, c := range caps.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

// companionWorkflowConfiguration reads the project's workflows from the
// companion plugin and maps them to the importer's configuration shape,
// including the condition/validator descriptors.
func (c *dataCenterClient) companionWorkflowConfiguration(
	ctx context.Context,
	projectID string,
	issueTypeIDs []string,
) (*JiraProjectWorkflowConfiguration, error) {
	project, err := jiraGetProject(ctx, c, c.baseURL, projectID)
	if err != nil {
		return nil, err
	}
	if project == nil || project.Key == "" {
		return nil, fmt.Errorf("companion workflows: project %s has no key", projectID)
	}

	endpoint := jiraSiteURLFromBase(c.baseURL) + companionBasePath + "/project/" + project.Key + "/workflows"
	var payload companionWorkflowsResponse
	if err := jiraGetJSON(ctx, c, endpoint, &payload); err != nil {
		return nil, err
	}

	issueTypeWorkflowIDs := make(map[string]string, len(issueTypeIDs))
	for _, issueTypeID := range issueTypeIDs {
		name := payload.IssueTypeWorkflowIDs[issueTypeID]
		if name == "" {
			name = payload.DefaultWorkflow
		}
		if name == "" {
			return nil, ErrWorkflowConfigurationNotAvailable
		}
		issueTypeWorkflowIDs[issueTypeID] = name
	}

	config := &JiraProjectWorkflowConfiguration{
		IssueTypeWorkflowIDs: issueTypeWorkflowIDs,
		Workflows:            make([]JiraWorkflowConfiguration, 0, len(payload.Workflows)),
		RulesComplete:        false,
	}
	for _, workflow := range payload.Workflows {
		config.Workflows = append(config.Workflows, companionWorkflowToConfiguration(workflow))
	}
	return config, nil
}

func companionWorkflowToConfiguration(workflow companionWorkflow) JiraWorkflowConfiguration {
	statusIDs := make([]string, 0, len(workflow.Statuses))
	for _, status := range workflow.Statuses {
		if status.ID != "" {
			statusIDs = append(statusIDs, status.ID)
		}
	}
	sort.Strings(statusIDs)

	transitions := make([]JiraConfiguredWorkflowTransition, 0, len(workflow.Transitions))
	for _, transition := range workflow.Transitions {
		guards := make([]JiraWorkflowGuard, 0, len(transition.Conditions)+len(transition.Validators))
		for _, condition := range transition.Conditions {
			guards = append(guards, companionGuardToGuard(condition, "condition"))
		}
		for _, validator := range transition.Validators {
			guards = append(guards, companionGuardToGuard(validator, "validator"))
		}

		configured := JiraConfiguredWorkflowTransition{
			ID:             transition.ID,
			Name:           transition.Name,
			Type:           JiraConfiguredWorkflowTransitionType(strings.ToUpper(transition.Type)),
			FromStatusIDs:  transition.FromStatusIDs,
			ToStatusID:     transition.ToStatusID,
			Guards:         guards,
			ConditionCount: len(transition.Conditions),
			ValidatorCount: len(transition.Validators),
		}
		transitions = append(transitions, configured)
	}

	return JiraWorkflowConfiguration{
		ID:          workflow.ID,
		Name:        workflow.Name,
		StatusIDs:   statusIDs,
		Transitions: transitions,
	}
}

func companionGuardToGuard(guard companionGuard, mode string) JiraWorkflowGuard {
	kind := guard.Kind
	if kind == "" {
		kind = "unknown"
	}
	return JiraWorkflowGuard{
		Kind:  kind,
		Mode:  mode,
		Class: guard.Class,
		Args:  guard.Args,
	}
}

type companionScreensResponse struct {
	IssueTypeScreens map[string]companionIssueTypeScreens `json:"issueTypeScreens"`
	Screens          []companionScreen                    `json:"screens"`
}

type companionIssueTypeScreens struct {
	CreateScreenID string `json:"createScreenId"`
	EditScreenID   string `json:"editScreenId"`
	ViewScreenID   string `json:"viewScreenId"`
}

type companionScreen struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	TabCount    int                    `json:"tabCount"`
	Fields      []companionScreenField `json:"fields"`
}

type companionScreenField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// companionScreenConfiguration reads a project's create/edit/view screens and
// their ordered fields from the companion plugin.
func (c *dataCenterClient) companionScreenConfiguration(ctx context.Context, projectKey string) (*JiraProjectScreenConfiguration, error) {
	endpoint := jiraSiteURLFromBase(c.baseURL) + companionBasePath + "/project/" + url.PathEscape(projectKey) + "/screens"
	var payload companionScreensResponse
	if err := jiraGetJSON(ctx, c, endpoint, &payload); err != nil {
		return nil, err
	}

	config := &JiraProjectScreenConfiguration{
		IssueTypeScreens: make(map[string]JiraIssueTypeScreens, len(payload.IssueTypeScreens)),
		Screens:          make([]JiraScreenConfiguration, 0, len(payload.Screens)),
	}
	for issueTypeID, screens := range payload.IssueTypeScreens {
		config.IssueTypeScreens[issueTypeID] = JiraIssueTypeScreens(screens)
	}
	for _, screen := range payload.Screens {
		fields := make([]JiraScreenField, 0, len(screen.Fields))
		for _, field := range screen.Fields {
			fields = append(fields, JiraScreenField(field))
		}
		config.Screens = append(config.Screens, JiraScreenConfiguration{
			ID:          screen.ID,
			Name:        screen.Name,
			Description: screen.Description,
			TabCount:    screen.TabCount,
			Fields:      fields,
		})
	}
	return config, nil
}
