package handlers

import (
	"strings"

	"windshift/internal/jira"
	"windshift/internal/jiraimport"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// jira_workflow_guards.go maps the workflow condition/validator descriptors the
// DC companion plugin returns onto Windshift's condition model.
//
// Only guards whose principal already exists in Windshift can be recreated
// faithfully. Jira built-ins with no Windshift equivalent (workflow
// permissions, sub-task blocking) are ignored: an operator has nothing to map
// them onto. Everything else that cannot be mapped (unknown classes, or a
// group/role that was not imported) keeps the transition review-locked, so an
// import can never silently broaden who may perform a transition.

// jiraGuardPrincipals resolves Jira group and project-role names to Windshift
// IDs. Both are looked up by name because the importer does not (yet) import
// Jira group/project-role membership.
type jiraGuardPrincipals struct {
	groupsByName map[string]int
	rolesByName  map[string]int
}

func (h *JiraImportHandler) loadJiraGuardPrincipals() jiraGuardPrincipals {
	principals := jiraGuardPrincipals{
		groupsByName: map[string]int{},
		rolesByName:  map[string]int{},
	}
	if groups, err := repository.NewGroupRepository(h.db).ListAll(); err == nil {
		for _, group := range groups {
			principals.groupsByName[strings.ToLower(group.Name)] = group.ID
		}
	}
	if roles, err := repository.NewWorkspaceRoleRepository(h.db).List(); err == nil {
		for _, role := range roles {
			principals.rolesByName[strings.ToLower(role.Name)] = role.ID
		}
	}
	return principals
}

// jiraGuardIgnorable reports whether a normalized companion guard kind is a
// Jira built-in with no Windshift equivalent. Jira attaches some of these
// (notably the workflow permission guard) to transitions automatically, so they
// are reported for fidelity but neither recreated nor review-locked.
func jiraGuardIgnorable(kind string) bool {
	switch kind {
	case "permission", "subtask_blocking":
		return true
	default:
		return false
	}
}

// hasNonIgnorableJiraGuard reports whether any source guard needs import work.
func hasNonIgnorableJiraGuard(guards []jira.JiraWorkflowGuard) bool {
	for _, guard := range guards {
		if !jiraGuardIgnorable(guard.Kind) {
			return true
		}
	}
	return false
}

// mapJiraWorkflowGuards turns source guards into Windshift guards. Unmappable
// guards are returned with an empty ConditionType; guards with no Windshift
// equivalent are flagged Ignored.
func mapJiraWorkflowGuards(guards []jira.JiraWorkflowGuard, principals jiraGuardPrincipals) []jiraimport.WorkflowGuard {
	mapped := make([]jiraimport.WorkflowGuard, 0, len(guards))
	for _, guard := range guards {
		workflowGuard := jiraimport.WorkflowGuard{Kind: guard.Kind, Mode: guard.Mode}
		if jiraGuardIgnorable(guard.Kind) {
			workflowGuard.Ignored = true
			mapped = append(mapped, workflowGuard)
			continue
		}
		switch guard.Kind {
		case "user_in_group":
			groupName := jiraGuardArgString(guard.Args, "groupName")
			if groupName == "" {
				groupName = jiraGuardArgString(guard.Args, "group")
			}
			if id, ok := principals.groupsByName[strings.ToLower(groupName)]; ok {
				workflowGuard.ConditionType = models.ConditionTypeUserInGroup
				workflowGuard.Config = map[string]any{"source": "current_user", "group_id": id}
			}
		case "user_in_role":
			roleName := strings.ToLower(jiraGuardArgString(guard.Args, "projectRole"))
			id, ok := principals.rolesByName[roleName]
			if !ok {
				if alias, exists := jiraRoleAliases[roleName]; exists {
					id, ok = principals.rolesByName[alias]
				}
			}
			if ok {
				workflowGuard.ConditionType = models.ConditionTypeUserInRole
				workflowGuard.Config = map[string]any{"source": "current_user", "role_id": id}
			}
		case "only_assignee":
			// Jira's "only the assignee can execute" maps onto a Windshift
			// user_is condition comparing the current user to the assignee.
			workflowGuard.ConditionType = models.ConditionTypeUserIs
			workflowGuard.Config = map[string]any{"source": "assignee"}
		}
		mapped = append(mapped, workflowGuard)
	}
	return mapped
}

func jiraGuardArgString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}

// jiraGuardMetadata summarizes guards for the workflow mapping record.
func jiraGuardMetadata(edges []jiraimport.WorkflowEdge) map[string]any {
	kinds := map[string]int{}
	mappedCount := 0
	lockedCount := 0
	ignoredCount := 0
	for _, edge := range edges {
		for _, guard := range edge.Guards {
			kinds[guard.Kind]++
			switch {
			case guard.Ignored:
				ignoredCount++
			case guard.Mappable():
				mappedCount++
			default:
				lockedCount++
			}
		}
	}
	if len(kinds) == 0 {
		return nil
	}
	metadata := map[string]any{
		"guard_kinds":        kinds,
		"mapped_guard_count": mappedCount,
		"locked_guard_count": lockedCount,
	}
	if ignoredCount > 0 {
		metadata["ignored_guard_count"] = ignoredCount
	}
	return metadata
}
