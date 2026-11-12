package handlers

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/jira"
	"windshift/internal/repository"
)

// jira_import_groups_roles.go imports Jira groups and project roles so that
// workflow guards referencing them resolve to real Windshift principals.
//
// Groups are resolved by name first. An existing group is reused as-is and its
// membership is never rewritten: it may be provisioned by LDAP or SCIM, where
// that membership is authoritative. Only groups created by this import get
// members. Project roles resolve to existing workspace roles by name (or a
// small alias map) and otherwise become custom roles.

// jiraRoleAliases maps common Jira project role names onto Windshift's built-in
// workspace roles when no exact name match exists.
var jiraRoleAliases = map[string]string{
	"administrators": "administrator",
	"developers":     "editor",
	"users":          "viewer",
}

func (h *JiraImportHandler) importJiraGroupsAndRoles(ctx context.Context, im *jiraImportContext, projectKey string) {
	capable, ok := im.client.(jira.GroupRoleClient)
	if !ok {
		return
	}

	groups, err := capable.ListGroups(ctx)
	if err != nil {
		slog.Warn("Failed to list Jira groups", slog.String("component", "jira"), slog.Any("error", err))
		groups = nil
	}
	roles, err := capable.GetProjectRoles(ctx, projectKey)
	if err != nil {
		slog.Warn("Failed to list Jira project roles", slog.String("component", "jira"), slog.Any("error", err))
		roles = nil
	}
	if len(groups) == 0 && len(roles) == 0 {
		return
	}

	// Ensure the referenced users first so memberships and role actors resolve.
	h.ensureGroupRoleUsers(ctx, im, groups, roles)
	groupMap := h.ensureJiraGroups(groups, im.userMap)
	h.ensureJiraProjectRoles(projectKey, im.workspaceID, roles, im.userMap, groupMap)
}

func (h *JiraImportHandler) ensureGroupRoleUsers(ctx context.Context, im *jiraImportContext, groups []jira.JiraGroup, roles []jira.JiraProjectRole) {
	users := make([]JiraUserSummary, 0)
	seen := make(map[string]bool)
	add := func(accountID, name, email, displayName string) {
		identifier := accountID
		if identifier == "" {
			identifier = name
		}
		if identifier == "" || seen[identifier] {
			return
		}
		seen[identifier] = true
		users = append(users, JiraUserSummary{AccountID: identifier, Email: email, DisplayName: displayName})
	}
	for _, group := range groups {
		for _, member := range group.Members {
			add(member.AccountID, member.Name, member.Email, member.DisplayName)
		}
	}
	for _, role := range roles {
		for _, actor := range role.Actors {
			if actor.Type == "atlassian-user-role-actor" {
				add(actor.AccountID, actor.Name, "", actor.DisplayName)
			}
		}
	}
	if len(users) == 0 {
		return
	}
	// Group members and role actors include service-desk customers. Route them
	// through the portal-customer path so they keep portal ownership instead of
	// becoming inactive internal users.
	internalUsers := users
	var portalCustomers []JiraUserSummary
	if len(im.customerIdentities) > 0 {
		internalUsers, portalCustomers = splitJiraImportUsers(users, im.customerIdentities)
	}
	if len(internalUsers) > 0 {
		newMappings, newUsernames, err := h.ensureUsers(ctx, im.jobID, internalUsers, im.client)
		if err != nil {
			slog.Warn("Failed to ensure users referenced by Jira groups/roles", slog.String("component", "jira"), slog.Any("error", err))
		}
		for key, value := range newMappings {
			im.userMap[key] = value
		}
		for key, value := range newUsernames {
			im.usernameMap[key] = value
		}
	}
	// Portal customers can only be created when this project owns a portal;
	// otherwise the service-desk project creates them and this project just
	// must not turn them into internal users.
	if len(portalCustomers) > 0 && im.jsmImport != nil {
		newPortalMappings, portalErr := h.ensurePortalCustomers(
			im.jobID, im.jsmImport.ChannelID, portalCustomers, im.jsmImport.CustomerOrganizations,
		)
		if portalErr != nil {
			slog.Warn("Failed to ensure Jira portal customers referenced by groups/roles", slog.String("component", "jira"), slog.Any("error", portalErr))
		}
		for key, value := range newPortalMappings {
			im.portalCustomerMap[key] = value
		}
	}
}

// ensureJiraGroups resolves each Jira group to a Windshift group, creating the
// group (and its members) only when no group with that name exists. Existing
// groups are reused untouched.
func (h *JiraImportHandler) ensureJiraGroups(groups []jira.JiraGroup, userMap map[string]int) map[string]int {
	if len(groups) == 0 {
		return nil
	}
	repo := repository.NewGroupRepository(h.db)
	existing, err := repo.ListAll()
	if err != nil {
		slog.Warn("Failed to list Windshift groups", slog.String("component", "jira"), slog.Any("error", err))
		return nil
	}
	byName := make(map[string]int, len(existing))
	for _, group := range existing {
		byName[strings.ToLower(group.Name)] = group.ID
	}

	result := make(map[string]int, len(groups))
	now := time.Now()
	for _, group := range groups {
		key := strings.ToLower(group.Name)
		if id, ok := byName[key]; ok {
			result[group.Name] = id
			continue
		}
		created, err := repo.Create(group.Name, "Imported from Jira", nil, now)
		if err != nil {
			slog.Warn("Failed to create imported Jira group",
				slog.String("component", "jira"), slog.String("group", group.Name), slog.Any("error", err))
			continue
		}
		id := int(created)
		for _, member := range group.Members {
			userID, ok := userMap[member.AccountID]
			if !ok {
				continue
			}
			if _, err := repo.AddMember(id, userID, nil, now); err != nil {
				slog.Debug("Failed to add imported group member",
					slog.String("component", "jira"), slog.String("group", group.Name), slog.Any("error", err))
			}
		}
		byName[key] = id
		result[group.Name] = id
	}
	return result
}

// ensureJiraProjectRoles resolves each Jira project role to a Windshift
// workspace role and assigns its actors in the workspace.
func (h *JiraImportHandler) ensureJiraProjectRoles(
	projectKey string,
	workspaceID int,
	roles []jira.JiraProjectRole,
	userMap map[string]int,
	groupMap map[string]int,
) map[string]int {
	if len(roles) == 0 {
		return nil
	}
	repo := repository.NewWorkspaceRoleRepository(h.db)
	existing, err := repo.List()
	if err != nil {
		slog.Warn("Failed to list Windshift workspace roles", slog.String("component", "jira"), slog.Any("error", err))
		return nil
	}
	byName := make(map[string]int, len(existing))
	for _, role := range existing {
		byName[strings.ToLower(role.Name)] = role.ID
	}

	result := make(map[string]int, len(roles))
	now := time.Now()
	for _, role := range roles {
		key := strings.ToLower(role.Name)
		id, ok := byName[key]
		if !ok {
			if alias, exists := jiraRoleAliases[key]; exists {
				id, ok = byName[alias]
			}
		}
		if !ok {
			created, err := repo.CreateCustomRole(role.Name, "Imported from Jira", now)
			if err != nil {
				slog.Warn("Failed to create imported Jira project role",
					slog.String("component", "jira"), slog.String("project", projectKey), slog.String("role", role.Name), slog.Any("error", err))
				continue
			}
			id = created
			byName[key] = id
		}
		result[role.Name] = id

		for _, actor := range role.Actors {
			switch actor.Type {
			case "atlassian-group-role-actor":
				if groupID, ok := groupMap[actor.Name]; ok {
					if err := repo.AssignToGroup(groupID, workspaceID, id, 0); err != nil {
						slog.Debug("Failed to assign imported role to group",
							slog.String("component", "jira"), slog.String("role", role.Name), slog.Any("error", err))
					}
				}
			case "atlassian-user-role-actor":
				if userID, ok := userMap[actor.AccountID]; ok {
					if err := repo.AssignToUser(userID, workspaceID, id, 0); err != nil {
						slog.Debug("Failed to assign imported role to user",
							slog.String("component", "jira"), slog.String("role", role.Name), slog.Any("error", err))
					}
				}
			}
		}
	}
	return result
}
