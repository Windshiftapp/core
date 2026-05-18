package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"windshift/internal/jira"
	"windshift/internal/repository"
)

// projectCountConcurrency caps parallel Jira count requests. Picked to stay
// well under the Jira client's default 10 req/s ceiling while still being
// markedly faster than the previous fully-serial loop.
const projectCountConcurrency = 6

// GetProjects handles GET /api/admin/jira-import/projects?connection_id={id}&open_issues_only=true
// GetProjects returns the metadata for every project on the connection. Issue
// counts are intentionally NOT fetched by default — they used to cost one
// serial Jira request per project, which dominated wizard latency on large
// instances (500 projects ≈ 50s at the 10 req/s client cap). Callers that
// want counts can pass ?include_counts=true (kept for parity with old
// behavior) or, preferred, call POST /admin/jira-import/projects/counts to
// batch counts in parallel for just the visible/selected keys.
func (h *JiraImportHandler) GetProjects(w http.ResponseWriter, r *http.Request) {
	connectionID := r.URL.Query().Get("connection_id")
	if connectionID == "" {
		respondValidationError(w, r, "connection_id is required")
		return
	}
	includeCounts := r.URL.Query().Get("include_counts") == "true"
	openIssuesOnly := r.URL.Query().Get("open_issues_only") == "true"

	client, err := h.getClientForConnection(r.Context(), connectionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	projects, err := client.ListProjects(r.Context())
	if err != nil {
		respondJiraUpstreamError(w, r, err)
		return
	}

	projectInfos := make([]JiraProjectInfo, len(projects))
	for i, p := range projects {
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
			AvatarURL:     avatarURL,
			IsTeamManaged: p.Simplified || p.Style == "next-gen",
		}
	}

	if includeCounts {
		keys := make([]string, len(projects))
		for i, p := range projects {
			keys[i] = p.Key
		}
		counts, upstreamErr := fetchProjectCounts(r.Context(), client, keys, openIssuesOnly)
		if upstreamErr != nil {
			respondJiraUpstreamError(w, r, upstreamErr)
			return
		}
		for i := range projectInfos {
			if c, ok := counts[projectInfos[i].Key]; ok {
				v := c
				projectInfos[i].IssueCount = &v
			}
		}
	}

	respondJSONOK(w, projectInfos)
}

// GetProjectCountsRequest is the body for POST /admin/jira-import/projects/counts.
type GetProjectCountsRequest struct {
	ConnectionID   string   `json:"connection_id"`
	Keys           []string `json:"keys"`
	OpenIssuesOnly bool     `json:"open_issues_only"`
}

// GetProjectCounts handles POST /admin/jira-import/projects/counts and
// returns {key: count} for the requested project keys, fetched with bounded
// concurrency. Errors per project are logged and silently omitted from the
// response — the frontend treats a missing key as "unknown" and the UI can
// retry visible projects.
func (h *JiraImportHandler) GetProjectCounts(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[GetProjectCountsRequest](w, r)
	if !ok {
		return
	}
	if req.ConnectionID == "" {
		respondValidationError(w, r, "connection_id is required")
		return
	}
	keys := dedupeNonEmpty(req.Keys)
	if len(keys) == 0 {
		respondJSONOK(w, map[string]int{})
		return
	}

	client, err := h.getClientForConnection(r.Context(), req.ConnectionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	counts, upstreamErr := fetchProjectCounts(r.Context(), client, keys, req.OpenIssuesOnly)
	if upstreamErr != nil {
		respondJiraUpstreamError(w, r, upstreamErr)
		return
	}
	respondJSONOK(w, counts)
}

// fetchProjectCounts fans out GetIssueCount calls with bounded concurrency
// and returns a {key: count} map plus the first credential/upstream error if
// any goroutine saw one. Other (per-project) failures stay logged-and-omitted
// because they're typically permission-scoped; a credential failure on the
// other hand means every count is going to fail and the caller should surface
// it rather than return a quietly empty map.
func fetchProjectCounts(ctx context.Context, client jira.Client, keys []string, openIssuesOnly bool) (map[string]int, error) {
	counts := make(map[string]int, len(keys))
	if len(keys) == 0 {
		return counts, nil
	}
	var (
		mu           sync.Mutex
		wg           sync.WaitGroup
		upstreamErr  error
		upstreamOnce sync.Once
	)
	sem := make(chan struct{}, projectCountConcurrency)

	for _, k := range keys {
		k := k
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			count, err := client.GetIssueCount(ctx, k, openIssuesOnly)
			if err != nil {
				slog.Warn("Failed to get issue count for project",
					slog.String("component", "jira"), slog.String("project", k), slog.Any("error", err))
				if isJiraUpstreamError(err) {
					upstreamOnce.Do(func() { upstreamErr = err })
				}
				return
			}
			mu.Lock()
			counts[k] = count
			mu.Unlock()
		}()
	}
	wg.Wait()
	return counts, upstreamErr
}

// isJiraUpstreamError reports whether the error is one the wizard should
// surface to the user rather than silently swallow.
func isJiraUpstreamError(err error) bool {
	return errors.Is(err, jira.ErrInvalidCredentials) ||
		errors.Is(err, jira.ErrForbidden) ||
		errors.Is(err, jira.ErrRateLimited)
}

func dedupeNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Analyze handles POST /api/admin/jira-import/analyze
func (h *JiraImportHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[JiraAnalyzeRequest](w, r)
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
		var project *jira.JiraProject
		project, err = client.GetProject(ctx, projectKey)
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
		var count int
		count, err = client.GetIssueCount(ctx, projectKey, req.OpenIssuesOnly)
		if err != nil {
			slog.Warn("Failed to get issue count for project", slog.String("component", "jira"), slog.String("project", projectKey), slog.Any("error", err))
		}
		projectAnalysis.IssueCount = count
		result.TotalIssues += count

		// Get project issue types and statuses
		var issueTypes []jira.JiraIssueType
		issueTypes, err = client.GetProjectIssueTypes(ctx, projectKey)
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
		var workflow *jira.JiraWorkflow
		workflow, err = client.GetProjectWorkflowScheme(ctx, projectKey)
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
		var versions []jira.JiraVersion
		versions, err = client.GetProjectVersions(ctx, projectKey)
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
		var boards *jira.BoardListResult
		boards, err = client.ListBoards(ctx, projectKey)
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

		var searchResult *jira.SearchResult
		searchResult, err = client.SearchIssues(ctx, jira.SearchOptions{
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
	userRepo := repository.NewUserRepository(h.db)
	for _, user := range userMap {
		if user.Email != "" {
			// Try to find matching Windshift user by email
			userID, err := userRepo.GetIDByEmail(user.Email)
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

	respondJSONOK(w, result)
}

// GetAssetSchemas handles GET /api/admin/jira-import/assets?connection_id={id}
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
			respondJSONOK(w, []JiraAssetSchemaInfo{})
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

	respondJSONOK(w, schemaInfos)
}

// GetAssetTypes handles GET /api/admin/jira-import/assets/{schemaId}/types?connection_id={id}
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

	respondJSONOK(w, types)
}
