package jira

import (
	"context"
	"net/url"
	"sort"
)

// groups_roles.go reads Jira groups and project roles, which the importer maps
// onto Windshift groups and workspace roles so workflow guards that reference
// them can be recreated faithfully.

// JiraGroup is a Jira group and its members.
type JiraGroup struct {
	Name    string
	Members []JiraGroupMember
}

// JiraGroupMember identifies a group member. Data Center uses name/key; Cloud
// uses accountId. Callers match on whichever the user map is keyed by.
type JiraGroupMember struct {
	AccountID   string
	Name        string
	Email       string
	DisplayName string
}

// JiraProjectRole is a project role and its actors.
type JiraProjectRole struct {
	Name   string
	Actors []JiraRoleActor
}

// JiraRoleActor is a user or group granted a project role.
type JiraRoleActor struct {
	Type        string // "atlassian-user-role-actor" or "atlassian-group-role-actor"
	Name        string
	AccountID   string
	DisplayName string
}

// GroupRoleClient is an optional Jira client capability. Data Center exposes
// groups and project roles through the standard REST API; callers use this when
// available and skip the feature otherwise.
type GroupRoleClient interface {
	ListGroups(ctx context.Context) ([]JiraGroup, error)
	GetProjectRoles(ctx context.Context, projectKey string) ([]JiraProjectRole, error)
}

// GroupMemberClient is an optional capability for expanding a group's members.
// Service-desk customer classification needs it to resolve group-granted
// project roles without listing every group.
type GroupMemberClient interface {
	GetGroupMembers(ctx context.Context, groupName string) ([]JiraGroupMember, error)
}

func (c *dataCenterClient) GetGroupMembers(ctx context.Context, groupName string) ([]JiraGroupMember, error) {
	return c.groupMembers(ctx, groupName)
}

func (c *dataCenterClient) ListGroups(ctx context.Context) ([]JiraGroup, error) {
	var picker struct {
		Groups []struct {
			Name string `json:"name"`
		} `json:"groups"`
	}
	if err := jiraGetJSON(ctx, c, c.baseURL+"/groups/picker?maxResults=1000", &picker); err != nil {
		return nil, err
	}

	groups := make([]JiraGroup, 0, len(picker.Groups))
	for _, group := range picker.Groups {
		members, err := c.groupMembers(ctx, group.Name)
		if err != nil {
			return nil, err
		}
		groups = append(groups, JiraGroup{Name: group.Name, Members: members})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups, nil
}

func (c *dataCenterClient) groupMembers(ctx context.Context, groupName string) ([]JiraGroupMember, error) {
	pageURL := c.baseURL + "/group/member?groupname=" + url.QueryEscape(groupName) + "&maxResults=1000"
	var page struct {
		Values []struct {
			Name         string `json:"name"`
			Key          string `json:"key"`
			EmailAddress string `json:"emailAddress"`
			DisplayName  string `json:"displayName"`
		} `json:"values"`
	}
	if err := jiraGetJSON(ctx, c, pageURL, &page); err != nil {
		return nil, err
	}

	members := make([]JiraGroupMember, 0, len(page.Values))
	for _, user := range page.Values {
		identifier := user.Name
		if identifier == "" {
			identifier = user.Key
		}
		members = append(members, JiraGroupMember{
			AccountID:   identifier,
			Name:        identifier,
			Email:       user.EmailAddress,
			DisplayName: user.DisplayName,
		})
	}
	return members, nil
}

func (c *dataCenterClient) GetProjectRoles(ctx context.Context, projectKey string) ([]JiraProjectRole, error) {
	var rolesByName map[string]string
	if err := jiraGetJSON(ctx, c, c.baseURL+"/project/"+url.PathEscape(projectKey)+"/role", &rolesByName); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(rolesByName))
	for name := range rolesByName {
		names = append(names, name)
	}
	sort.Strings(names)

	roles := make([]JiraProjectRole, 0, len(names))
	for _, name := range names {
		var detail struct {
			Name   string `json:"name"`
			Actors []struct {
				Type        string `json:"type"`
				Name        string `json:"name"`
				AccountID   string `json:"accountId"`
				DisplayName string `json:"displayName"`
			} `json:"actors"`
		}
		if err := jiraGetJSON(ctx, c, rolesByName[name], &detail); err != nil {
			return nil, err
		}
		role := JiraProjectRole{Name: name}
		for _, actor := range detail.Actors {
			role.Actors = append(role.Actors, JiraRoleActor{
				Type:        actor.Type,
				Name:        actor.Name,
				AccountID:   actor.AccountID,
				DisplayName: actor.DisplayName,
			})
		}
		roles = append(roles, role)
	}
	return roles, nil
}
