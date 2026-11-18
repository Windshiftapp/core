package jiraimport

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/jira"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func (s *Service) EnsureIterationType(name, color, description string) (int, error) {
	enum := services.NewEnumService(s.db, services.NewIterationTypeConfig())
	types, err := enum.GetAll()
	if err != nil {
		return 0, err
	}
	for _, iterationType := range types {
		if iterationType.GetName() == name {
			return iterationType.GetID(), nil
		}
	}
	created, err := enum.Create(&models.IterationType{
		Name:        name,
		Color:       color,
		Description: description,
	}, nil)
	if err != nil {
		return 0, err
	}
	return created.GetID(), nil
}

func (s *Service) EnsureSprintIteration(workspaceID, typeID int, sprint jira.JiraSprint) (iterationID int, created, ok bool) {
	start, end, ok := sprintDates(sprint)
	if !ok {
		return 0, false, false
	}
	status := "planned"
	switch strings.ToLower(sprint.State) {
	case "active":
		status = "active"
	case "closed":
		status = "completed"
	}
	name := strings.TrimSpace(sprint.Name)
	if name == "" {
		name = fmt.Sprintf("Jira Sprint %d", sprint.ID)
	}
	existing, err := s.planning.FindIterationByName(workspaceID, name)
	if err == nil {
		return existing.ID, false, true
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return 0, false, false
	}
	description := strings.TrimSpace(sprint.Goal)
	if description != "" {
		description = "Jira sprint goal: " + description
	}
	iteration, err := s.planning.CreateIteration(services.CreateIterationParams{
		Name:        name,
		Description: description,
		StartDate:   start,
		EndDate:     end,
		Status:      status,
		TypeID:      &typeID,
		IsGlobal:    false,
		WorkspaceID: &workspaceID,
	})
	if err != nil {
		return 0, false, false
	}
	return iteration.ID, true, true
}

func sprintDates(sprint jira.JiraSprint) (start, end string, ok bool) {
	start = jiraDateOnly(sprint.StartDate)
	end = jiraDateOnly(sprint.EndDate)
	if start == "" {
		start = jiraDateOnly(sprint.CompleteDate)
	}
	if end == "" {
		end = jiraDateOnly(sprint.CompleteDate)
	}
	if start == "" && end == "" {
		return "", "", false
	}
	if start == "" {
		start = end
	}
	if end == "" {
		end = start
	}
	return start, end, true
}

func jiraDateOnly(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed := jira.ParseJiraTimestamp(value); parsed != nil {
		return parsed.UTC().Format("2006-01-02")
	}
	if len(value) >= len("2006-01-02") {
		candidate := value[:len("2006-01-02")]
		if _, err := time.Parse("2006-01-02", candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func (s *Service) EnsureTimeProject(jobID string, workspaceID int, projectKey, projectName string) (*int, error) {
	workspace, err := s.workspaces.FindByID(workspaceID)
	if err != nil {
		return nil, err
	}
	if workspace.TimeProjectID != nil {
		id := *workspace.TimeProjectID
		err := s.RecordMapping(jobID, "time_project", "project:"+projectKey+":worklogs", projectKey, id, map[string]any{
			"workspace_id": workspaceID, "action": "reuse_workspace_default", "was_created": false,
		})
		return &id, err
	}
	customerID, _, err := s.EnsureCustomerOrganisation(
		"Jira Imports",
		"Synthetic customer used for imported Jira worklogs",
	)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = projectKey
	}
	if name == "" {
		name = "Jira Import"
	}
	projectName = fmt.Sprintf("%s Jira Worklogs", name)
	wasCreated := false
	projectID, err := s.timeProjects.FindIDByNameAndCustomer(projectName, customerID)
	if errors.Is(err, repository.ErrNotFound) {
		description := fmt.Sprintf("Imported Jira worklogs for project %s", projectKey)
		project := &models.TimeProject{
			CustomerID:  &customerID,
			Name:        projectName,
			Description: description,
			Status:      "Active",
			Color:       "#3b82f6",
			Settings:    map[string]any{},
		}
		err = s.timeProjects.Create(project)
		projectID = project.ID
		wasCreated = err == nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.workspaces.AssignTimeProjectIfUnset(workspaceID, projectID); err != nil {
		return nil, err
	}
	if err := s.RecordMapping(jobID, "time_project", "project:"+projectKey+":worklogs", projectKey, projectID, map[string]any{
		"workspace_id": workspaceID, "was_created": wasCreated,
	}); err != nil {
		return nil, err
	}
	return &projectID, nil
}
