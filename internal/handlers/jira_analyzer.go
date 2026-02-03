package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"windshift/internal/jira"
)

// GetProjects handles GET /api/jira-import/projects?connection_id={id}&open_issues_only=true
func (h *JiraImportHandler) GetProjects(w http.ResponseWriter, r *http.Request) {
	connectionID := r.URL.Query().Get("connection_id")
	if connectionID == "" {
		respondValidationError(w, r, "connection_id is required")
		return
	}

	// Check if we should only count open issues
	openIssuesOnly := r.URL.Query().Get("open_issues_only") == "true"

	client, err := h.getClientForConnection(r.Context(), connectionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	projects, err := client.ListProjects(r.Context())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get issue counts for each project (in parallel would be better, but keeping it simple)
	projectInfos := make([]JiraProjectInfo, len(projects))
	for i, p := range projects {
		count, err := client.GetIssueCount(r.Context(), p.Key, openIssuesOnly)
		if err != nil {
			slog.Warn("Failed to get issue count for project", slog.String("component", "jira"), slog.String("project", p.Key), slog.Any("error", err))
			// Continue with 0 count on error
		}

		avatarURL := ""
		if p.AvatarURLs != nil {
			if url, ok := p.AvatarURLs["48x48"]; ok {
				avatarURL = url
			}
		}

		projectInfos[i] = JiraProjectInfo{
			Key:           p.Key,
			ID:            p.ID,
			Name:          p.Name,
			Description:   p.Description,
			ProjectType:   p.ProjectType,
			IssueCount:    count,
			AvatarURL:     avatarURL,
			IsTeamManaged: p.Simplified || p.Style == "next-gen",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projectInfos)
}

// Analyze handles POST /api/jira-import/analyze
func (h *JiraImportHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	var req JiraAnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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

	ctx := r.Context()
	result := JiraAnalysisResult{
		Projects:       make([]JiraProjectAnalysis, 0),
		IssueTypes:     make([]JiraIssueTypeInfo, 0),
		Statuses:       make([]JiraStatusInfo, 0),
		CustomFields:   make([]jira.FieldMappingSuggestion, 0),
		AssetSchemas:   make([]JiraAssetSchemaInfo, 0),
		OpenIssuesOnly: req.OpenIssuesOnly,
	}

	// Track unique issue types and statuses across all projects
	issueTypeMap := make(map[string]JiraIssueTypeInfo)
	statusMap := make(map[string]JiraStatusInfo)

	// Collect project IDs for the custom fields API
	projectIDs := make([]string, 0, len(req.ProjectKeys))

	// Analyze each project
	for _, projectKey := range req.ProjectKeys {
		projectAnalysis := JiraProjectAnalysis{
			Key:        projectKey,
			IssueTypes: make([]string, 0),
		}

		// Get project details
		project, err := client.GetProject(ctx, projectKey)
		if err != nil {
			slog.Warn("Failed to get project", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			continue
		}
		projectAnalysis.Name = project.Name
		// Only include company-managed projects for field search (team-managed projects don't support this API)
		if !project.Simplified && project.Style != "next-gen" {
			projectIDs = append(projectIDs, project.ID)
		}

		// Get issue count (respecting open_issues_only filter)
		count, err := client.GetIssueCount(ctx, projectKey, req.OpenIssuesOnly)
		if err != nil {
			slog.Warn("Failed to get issue count for project", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
		}
		projectAnalysis.IssueCount = count
		result.TotalIssues += count

		// Get project issue types and statuses
		issueTypes, err := client.GetProjectIssueTypes(ctx, projectKey)
		if err == nil {
			for _, it := range issueTypes {
				projectAnalysis.IssueTypes = append(projectAnalysis.IssueTypes, it.Name)
				if _, exists := issueTypeMap[it.ID]; !exists {
					issueTypeMap[it.ID] = JiraIssueTypeInfo{
						ID:             it.ID,
						Name:           it.Name,
						Description:    it.Description,
						Subtask:        it.Subtask,
						HierarchyLevel: it.HierarchyLevel,
					}
				}
			}
		}

		// Get workflow/statuses for this project
		workflow, err := client.GetProjectWorkflowScheme(ctx, projectKey)
		if err == nil && workflow != nil {
			for _, s := range workflow.Statuses {
				if _, exists := statusMap[s.ID]; !exists {
					info := JiraStatusInfo{
						ID:   s.ID,
						Name: s.Name,
					}
					if s.StatusCategory != nil {
						info.CategoryID = s.StatusCategory.ID
						info.CategoryName = s.StatusCategory.Name
						info.CategoryKey = s.StatusCategory.Key
						if color, ok := jira.StatusCategoryColorMap[s.StatusCategory.ColorName]; ok {
							info.Color = color
						}
					}
					statusMap[s.ID] = info
				}
			}
		}

		// Check for versions and collect them
		versions, err := client.GetProjectVersions(ctx, projectKey)
		if err == nil && len(versions) > 0 {
			projectAnalysis.HasVersions = true
			projectAnalysis.VersionCount = len(versions)
			for _, v := range versions {
				result.Versions = append(result.Versions, JiraVersionInfo{
					ID:          v.ID,
					Name:        v.Name,
					Description: v.Description,
					Archived:    v.Archived,
					Released:    v.Released,
					ReleaseDate: v.ReleaseDate,
					ProjectKey:  projectKey,
				})
			}
		}

		// Check for sprints (via boards)
		boards, err := client.ListBoards(ctx, projectKey)
		if err == nil && boards != nil && len(boards.Values) > 0 {
			projectAnalysis.HasSprints = true
		}

		result.Projects = append(result.Projects, projectAnalysis)
	}

	// Convert maps to slices
	for _, it := range issueTypeMap {
		result.IssueTypes = append(result.IssueTypes, it)
	}
	for _, s := range statusMap {
		result.Statuses = append(result.Statuses, s)
	}

	// Get custom fields - try project-specific endpoint first, then fall back to all fields
	customFields, err := client.GetProjectFields(ctx, projectIDs)
	if err != nil {
		// Fallback to all fields if API fails
		slog.Debug("GetProjectFields failed, falling back to ListCustomFields", slog.String("component", "jira"), slog.Any("projectIDs", projectIDs), slog.Any("error", err))
		customFields, err = client.ListCustomFields(ctx)
		if err == nil {
			slog.Debug("ListCustomFields returned custom fields", slog.String("component", "jira"), slog.Int("count", len(customFields)))
		}
	} else {
		slog.Debug("GetProjectFields returned custom fields", slog.String("component", "jira"), slog.Int("count", len(customFields)), slog.Any("projectIDs", projectIDs))
	}
	if err == nil {
		result.CustomFields = jira.SuggestFieldMappings(customFields)
	}

	// Collect users from a sample of issues
	userMap := make(map[string]JiraUserSummary)
	for _, projectKey := range req.ProjectKeys {
		// Fetch a sample of issues to discover users (limit to 100 per project for performance)
		jql := fmt.Sprintf("project = %s ORDER BY created DESC", projectKey)
		if req.OpenIssuesOnly {
			jql = fmt.Sprintf("project = %s AND statusCategory != Done ORDER BY created DESC", projectKey)
		}

		searchResult, err := client.SearchIssues(ctx, jira.SearchOptions{
			JQL:        jql,
			MaxResults: 100,
			StartAt:    0,
		})
		if err != nil {
			slog.Debug("Failed to fetch sample issues for user collection", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
			continue
		}

		for _, issue := range searchResult.Issues {
			// Collect assignee
			if issue.Fields.Assignee != nil && issue.Fields.Assignee.AccountID != "" {
				if _, exists := userMap[issue.Fields.Assignee.AccountID]; !exists {
					avatarURL := ""
					if issue.Fields.Assignee.AvatarURLs != nil {
						avatarURL = issue.Fields.Assignee.AvatarURLs["48x48"]
					}
					userMap[issue.Fields.Assignee.AccountID] = JiraUserSummary{
						AccountID:   issue.Fields.Assignee.AccountID,
						Email:       issue.Fields.Assignee.EmailAddress,
						DisplayName: issue.Fields.Assignee.DisplayName,
						AvatarURL:   avatarURL,
					}
				}
			}
			// Collect reporter
			if issue.Fields.Reporter != nil && issue.Fields.Reporter.AccountID != "" {
				if _, exists := userMap[issue.Fields.Reporter.AccountID]; !exists {
					avatarURL := ""
					if issue.Fields.Reporter.AvatarURLs != nil {
						avatarURL = issue.Fields.Reporter.AvatarURLs["48x48"]
					}
					userMap[issue.Fields.Reporter.AccountID] = JiraUserSummary{
						AccountID:   issue.Fields.Reporter.AccountID,
						Email:       issue.Fields.Reporter.EmailAddress,
						DisplayName: issue.Fields.Reporter.DisplayName,
						AvatarURL:   avatarURL,
					}
				}
			}
			// Collect creator
			if issue.Fields.Creator != nil && issue.Fields.Creator.AccountID != "" {
				if _, exists := userMap[issue.Fields.Creator.AccountID]; !exists {
					avatarURL := ""
					if issue.Fields.Creator.AvatarURLs != nil {
						avatarURL = issue.Fields.Creator.AvatarURLs["48x48"]
					}
					userMap[issue.Fields.Creator.AccountID] = JiraUserSummary{
						AccountID:   issue.Fields.Creator.AccountID,
						Email:       issue.Fields.Creator.EmailAddress,
						DisplayName: issue.Fields.Creator.DisplayName,
						AvatarURL:   avatarURL,
					}
				}
			}
		}
	}

	// Convert user map to slice and try to match with existing Windshift users
	for _, user := range userMap {
		if user.Email != "" {
			// Try to find matching Windshift user by email
			var userID int
			err := h.db.QueryRow(`SELECT id FROM users WHERE email = ?`, user.Email).Scan(&userID)
			if err == nil {
				user.MatchedUserID = &userID
			}
		}
		result.Users = append(result.Users, user)
	}

	// Try to get asset schemas (may not be available)
	assetSchemas, err := client.ListObjectSchemas(ctx)
	if err == nil {
		for _, schema := range assetSchemas {
			result.AssetSchemas = append(result.AssetSchemas, JiraAssetSchemaInfo{
				ID:          schema.ID,
				Name:        schema.Name,
				Description: schema.Description,
				ObjectCount: schema.ObjectCount,
				TypeCount:   schema.ObjectTypeCount,
			})
			result.TotalAssets += schema.ObjectCount
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetAssetSchemas handles GET /api/jira-import/assets?connection_id={id}
func (h *JiraImportHandler) GetAssetSchemas(w http.ResponseWriter, r *http.Request) {
	connectionID := r.URL.Query().Get("connection_id")
	if connectionID == "" {
		respondValidationError(w, r, "connection_id is required")
		return
	}

	client, err := h.getClientForConnection(r.Context(), connectionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	schemas, err := client.ListObjectSchemas(r.Context())
	if err != nil {
		if errors.Is(err, jira.ErrAssetsNotAvailable) {
			// Assets API not available, return empty list
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]JiraAssetSchemaInfo{})
			return
		}
		respondInternalError(w, r, err)
		return
	}

	schemaInfos := make([]JiraAssetSchemaInfo, len(schemas))
	for i, s := range schemas {
		schemaInfos[i] = JiraAssetSchemaInfo{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			ObjectCount: s.ObjectCount,
			TypeCount:   s.ObjectTypeCount,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schemaInfos)
}

// GetAssetTypes handles GET /api/jira-import/assets/{schemaId}/types?connection_id={id}
func (h *JiraImportHandler) GetAssetTypes(w http.ResponseWriter, r *http.Request) {
	schemaID := r.PathValue("schemaId")
	connectionID := r.URL.Query().Get("connection_id")

	if connectionID == "" || schemaID == "" {
		respondValidationError(w, r, "connection_id and schemaId are required")
		return
	}

	client, err := h.getClientForConnection(r.Context(), connectionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	types, err := client.ListObjectTypes(r.Context(), schemaID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types)
}
