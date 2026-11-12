package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"windshift/internal/jira"
	"windshift/internal/models"
)

// importJiraBoardsAndFilters imports Jira saved filters as Windshift collections
// and Jira Agile boards as collection-backed board configurations. The Jira API
// does not expose every JQL/board concept in a Windshift-compatible shape; when
// translation is partial, the importer records unsupported clauses in mapping
// metadata and the collection description instead of silently dropping them.
func (h *JiraImportHandler) importJiraBoardsAndFilters(ctx context.Context, jobID, projectKey string, workspaceID int, workspaceProjectKeys []string, statusMap map[string]int, client jira.Client, createdByUserID int) {
	h.importJiraSavedFilters(ctx, jobID, projectKey, workspaceID, workspaceProjectKeys, client, createdByUserID)
	h.importJiraBoards(ctx, jobID, projectKey, workspaceID, workspaceProjectKeys, statusMap, client, createdByUserID)
}

// jiraWorkspaceProjectKeys lists the Jira project keys that share workspaceID.
// A workspace created for a single project has one key; a reused workspace may
// hold several, which changes how a saved filter's project predicate can be
// represented.
func jiraWorkspaceProjectKeys(mappings []WorkspaceMapping, currentProjectKey string, workspaceID int) []string {
	keys := []string{currentProjectKey}
	seen := map[string]struct{}{currentProjectKey: {}}
	for _, mapping := range mappings {
		if mapping.WindshiftID == nil || *mapping.WindshiftID != workspaceID {
			continue
		}
		if _, ok := seen[mapping.JiraKey]; ok {
			continue
		}
		seen[mapping.JiraKey] = struct{}{}
		keys = append(keys, mapping.JiraKey)
	}
	sort.Strings(keys)
	return keys
}

// resolveJiraFilterQL translates saved-filter or board JQL for one workspace,
// honoring the project predicate. It returns the selector, the unsupported
// clauses, whether the selector preserves membership, and the scope decision.
// A JQLScopeUnrelated result means the filter targets only other projects.
func resolveJiraFilterQL(jql string, workspaceID int, workspaceProjectKeys []string) (ql string, unsupported []string, membershipComplete bool, scope jira.JQLProjectScope, decision jira.JQLProjectScopeDecision) {
	scope = jira.AnalyzeJQLProjectScope(jql)
	decision = jira.DecideJQLProjectScope(scope, workspaceProjectKeys)
	if decision == jira.JQLScopeUnrelated {
		return "", nil, false, scope, decision
	}
	translation := jira.TranslateJQL(jql, workspaceID)
	membershipComplete = translation.MembershipComplete
	if decision == jira.JQLScopeNarrowed || decision == jira.JQLScopeIndeterminate {
		// A project predicate that workspace-scoped QL cannot express would
		// otherwise widen the collection to the whole workspace.
		membershipComplete = false
	}
	ql = translation.QL
	if !membershipComplete {
		ql = jira.UnsupportedJQLSelector(workspaceID)
	}
	return ql, translation.Unsupported, membershipComplete, scope, decision
}

func (h *JiraImportHandler) importJiraSavedFilters(ctx context.Context, jobID, projectKey string, workspaceID int, workspaceProjectKeys []string, client jira.Client, createdByUserID int) {
	filters, err := client.ListFilters(ctx, projectKey)
	if err != nil {
		// Distinguish an unavailable/permission-denied discovery from a partial
		// one, so a missing saved-filter import is not read as "no filters".
		_ = h.recordMapping(jobID, "fidelity_finding", "filter-discovery:"+projectKey, "Saved filters could not be listed: "+err.Error(), 0, map[string]any{
			"code":        "jira_saved_filters_unavailable",
			"severity":    "warning",
			"disposition": "failed",
		})
		slog.Warn("Failed to list Jira saved filters",
			slog.String("component", "jira"),
			slog.String("project", projectKey),
			slog.Any("error", err))
		return
	}
	if filters == nil {
		return
	}
	if filters.Partial {
		// The source cannot enumerate every saved filter (Data Center only
		// exposes favorites). Record the boundary so the result is not read as a
		// complete saved-filter import.
		_ = h.recordMapping(jobID, "fidelity_finding", "filter-discovery:"+projectKey, filters.PartialReason, 0, map[string]any{
			"code":           "jira_saved_filters_partial",
			"severity":       "warning",
			"disposition":    "unavailable",
			"source_count":   len(filters.Values),
			"resolved_count": len(filters.Values),
		})
	}
	for _, filter := range filters.Values {
		if strings.TrimSpace(filter.ID) == "" {
			continue
		}
		// The saved-filter list is not project-scoped (Data Center returns every
		// favorite), so the project predicate decides whether this filter belongs
		// to the current workspace at all.
		ql, unsupported, membershipComplete, scope, decision := resolveJiraFilterQL(filter.JQL, workspaceID, workspaceProjectKeys)
		if decision == jira.JQLScopeUnrelated {
			slog.Debug("Skipping Jira saved filter scoped to other projects",
				slog.String("component", "jira"),
				slog.String("project", projectKey),
				slog.String("filterID", filter.ID),
				slog.Any("filter_projects", scope.Include))
			continue
		}
		description := jiraFilterCollectionDescription(filter, unsupported, membershipComplete)
		metadata := map[string]any{
			"jira_entity": "filter",
			"jira_jql":    filter.JQL,
		}
		if filter.ViewURL != "" {
			metadata["view_url"] = filter.ViewURL
		}
		if len(unsupported) > 0 {
			metadata["unsupported_jql"] = unsupported
		}
		if scope.HasClause {
			metadata["jira_project_predicate"] = append(append([]string{}, scope.Include...), scope.Exclude...)
		}
		if !membershipComplete {
			metadata["needs_review"] = true
			metadata["unsupported_membership"] = true
		}
		if decision == jira.JQLScopeNarrowed || decision == jira.JQLScopeIndeterminate {
			metadata["jira_project_scope"] = jiraProjectScopeLabel(decision)
			h.recordJiraFilterScopeFinding(jobID, filter.ID, workspaceID, scope, decision)
		}
		// Identity includes the workspace: the same Jira filter can legitimately
		// back one collection per workspace it targets.
		collectionID, ok := h.ensureJiraCollection(jobID, jiraFilterCollectionID(filter.ID, workspaceID), filter.ID, jiraCollectionName("Jira Filter", filter.Name, filter.ID), description, ql, workspaceID, createdByUserID, metadata)
		if ok {
			slog.Debug("Imported Jira saved filter", slog.String("component", "jira"), slog.String("filterID", filter.ID), slog.Int("collectionID", collectionID))
		}
	}
}

func jiraFilterCollectionID(filterID string, workspaceID int) string {
	return fmt.Sprintf("filter:%s:ws%d", filterID, workspaceID)
}

func jiraProjectScopeLabel(decision jira.JQLProjectScopeDecision) string {
	if decision == jira.JQLScopeIndeterminate {
		return "indeterminate"
	}
	return "narrowed"
}

// recordJiraFilterScopeFinding makes the project-scope limitation explicit so
// the non-executable collection is not read as a faithfully scoped import.
func (h *JiraImportHandler) recordJiraFilterScopeFinding(jobID, filterID string, workspaceID int, scope jira.JQLProjectScope, decision jira.JQLProjectScopeDecision) {
	code := "jira_filter_project_scope_narrowed"
	reason := "The saved filter's project predicate restricts membership to a subset of this workspace's projects, which workspace-scoped selectors cannot express."
	if decision == jira.JQLScopeIndeterminate {
		code = "jira_filter_project_scope_indeterminate"
		reason = "The saved filter's project predicate could not be analyzed, so the collection stays non-executable for review."
	}
	_ = h.recordMapping(jobID, "fidelity_finding", fmt.Sprintf("filter-scope:%s:ws%d", filterID, workspaceID), reason, 0, map[string]any{
		"code":              code,
		"severity":          "warning",
		"disposition":       "unavailable",
		"jira_filter_id":    filterID,
		"workspace_id":      workspaceID,
		"included_projects": scope.Include,
		"excluded_projects": scope.Exclude,
	})
}

func (h *JiraImportHandler) importJiraBoards(ctx context.Context, jobID, projectKey string, workspaceID int, workspaceProjectKeys []string, statusMap map[string]int, client jira.Client, createdByUserID int) {
	boards, err := client.ListBoards(ctx, projectKey)
	if err != nil {
		slog.Warn("Failed to list Jira boards for board import",
			slog.String("component", "jira"),
			slog.String("project", projectKey),
			slog.Any("error", err))
		return
	}
	if boards == nil {
		return
	}
	for _, board := range boards.Values {
		config, err := client.GetBoardConfiguration(ctx, board.ID)
		if err != nil {
			slog.Warn("Failed to load Jira board configuration; importing minimal board collection",
				slog.String("component", "jira"),
				slog.String("project", projectKey),
				slog.Int("boardID", board.ID),
				slog.Any("error", err))
		}

		jql := ""
		filterMeta := map[string]any{}
		if config != nil && config.Filter != nil && strings.TrimSpace(string(config.Filter.ID)) != "" {
			filter, filterErr := client.GetFilter(ctx, string(config.Filter.ID))
			if filterErr != nil {
				slog.Warn("Failed to load Jira board filter JQL",
					slog.String("component", "jira"),
					slog.Int("boardID", board.ID),
					slog.String("filterID", string(config.Filter.ID)),
					slog.Any("error", filterErr))
			} else if filter != nil {
				jql = filter.JQL
				filterMeta["jira_filter_id"] = filter.ID
				filterMeta["jira_filter_name"] = filter.Name
				filterMeta["jira_filter_jql"] = filter.JQL
			}
		}
		if strings.TrimSpace(jql) == "" {
			jql = fmt.Sprintf("project = %s", projectKey)
		}

		ql, unsupported, membershipComplete, _, decision := resolveJiraFilterQL(jql, workspaceID, workspaceProjectKeys)
		metadata := map[string]any{
			"jira_entity":   "board",
			"jira_board_id": board.ID,
			"jira_type":     board.Type,
			"jira_jql":      jql,
		}
		for k, v := range filterMeta {
			metadata[k] = v
		}
		if len(unsupported) > 0 {
			metadata["unsupported_jql"] = unsupported
		}
		if !membershipComplete {
			metadata["needs_review"] = true
			metadata["unsupported_membership"] = true
		}

		// A board's saved filter may already be imported as a standalone
		// collection; reuse it instead of creating a duplicate.
		collectionID := 0
		if filterID, ok := filterMeta["jira_filter_id"].(string); ok && strings.TrimSpace(filterID) != "" {
			collectionID = h.existingMappedEntity(jobID, "collection", jiraFilterCollectionID(filterID, workspaceID))
		}
		if collectionID == 0 {
			if decision == jira.JQLScopeUnrelated {
				// The board's filter targets other projects, so a collection
				// scoped to this workspace would have the wrong membership.
				slog.Debug("Skipping Jira board whose filter is scoped to other projects",
					slog.String("component", "jira"),
					slog.String("project", projectKey),
					slog.Int("boardID", board.ID))
				continue
			}
			var ok bool
			collectionID, ok = h.ensureJiraCollection(jobID, fmt.Sprintf("board:%d", board.ID), strconv.Itoa(board.ID), jiraCollectionName("Jira Board", board.Name, strconv.Itoa(board.ID)), jiraBoardCollectionDescription(board, config, jql, unsupported, membershipComplete), ql, workspaceID, createdByUserID, metadata)
			if !ok {
				continue
			}
		}

		columns, backlogStatusIDs, columnUnsupported := h.jiraBoardColumns(config, statusMap)
		if len(columns) == 0 {
			columns = h.defaultBoardColumnsFromMappedStatuses(statusMap)
		}
		if len(columnUnsupported) > 0 {
			metadata["unsupported_status_ids"] = columnUnsupported
		}
		boardConfigID, ok := h.ensureJiraBoardConfiguration(jobID, board, collectionID, columns, backlogStatusIDs, metadata)
		if ok {
			slog.Debug("Imported Jira board configuration", slog.String("component", "jira"), slog.Int("boardID", board.ID), slog.Int("boardConfigID", boardConfigID))
		}
	}
}

func jiraCollectionName(prefix, name, fallbackID string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallbackID
	}
	return fmt.Sprintf("%s: %s", prefix, name)
}

func jiraFilterCollectionDescription(filter jira.JiraFilter, unsupported []string, membershipComplete bool) string {
	parts := []string{"Imported from Jira saved filter."}
	if strings.TrimSpace(filter.Description) != "" {
		parts = append(parts, strings.TrimSpace(filter.Description))
	}
	if strings.TrimSpace(filter.JQL) != "" {
		parts = append(parts, "Original JQL:\n```jql\n"+strings.TrimSpace(filter.JQL)+"\n```")
	}
	parts = appendJiraUnsupportedJQLNote(parts, unsupported, membershipComplete)
	return strings.Join(parts, "\n\n")
}

func jiraBoardCollectionDescription(board jira.JiraBoard, config *jira.JiraBoardConfiguration, jql string, unsupported []string, membershipComplete bool) string {
	parts := []string{fmt.Sprintf("Imported from Jira %s board %d.", strings.TrimSpace(board.Type), board.ID)}
	if config != nil && config.SubQuery != nil && strings.TrimSpace(config.SubQuery.Query) != "" {
		parts = append(parts, "Board sub-query preserved for reference:\n```jql\n"+strings.TrimSpace(config.SubQuery.Query)+"\n```")
	}
	if strings.TrimSpace(jql) != "" {
		parts = append(parts, "Original board/filter JQL:\n```jql\n"+strings.TrimSpace(jql)+"\n```")
	}
	parts = appendJiraUnsupportedJQLNote(parts, unsupported, membershipComplete)
	return strings.Join(parts, "\n\n")
}

// appendJiraUnsupportedJQLNote records unsupported clauses and, when the
// predicate tree is incomplete, that the imported collection is intentionally
// non-executable until an administrator reviews it.
func appendJiraUnsupportedJQLNote(parts, unsupported []string, membershipComplete bool) []string {
	if len(unsupported) > 0 {
		parts = append(parts, "Unsupported JQL clauses not translated into Windshift QL:\n- "+strings.Join(unsupported, "\n- "))
	}
	if !membershipComplete {
		parts = append(parts, "This collection is intentionally empty and needs review: its Jira predicate tree could not be translated in full, so matching every workspace issue would be incorrect.")
	}
	return parts
}

func (h *JiraImportHandler) ensureJiraCollection(jobID, jiraID, jiraKey, name, description, ql string, workspaceID, createdByUserID int, metadata map[string]any) (int, bool) {
	return h.imports.EnsureCollection(jobID, jiraID, jiraKey, name, description, ql, workspaceID, createdByUserID, metadata)
}

func (h *JiraImportHandler) existingMappedEntity(jobID, entityType, jiraID string) int {
	if id, ok := h.imports.MappedEntity(jobID, entityType, jiraID); ok {
		return id
	}
	return 0
}

func (h *JiraImportHandler) jiraBoardColumns(config *jira.JiraBoardConfiguration, statusMap map[string]int) (columns []models.BoardColumnRequest, backlogStatusIDs []int, unsupported []string) {
	if config == nil || config.ColumnConfig == nil || len(config.ColumnConfig.Columns) == 0 {
		return nil, nil, nil
	}
	columns = make([]models.BoardColumnRequest, 0, len(config.ColumnConfig.Columns))
	unsupported = []string{}
	backlogStatusIDs = []int{}
	for i, jiraColumn := range config.ColumnConfig.Columns {
		statusIDs := make([]int, 0, len(jiraColumn.Statuses))
		for _, st := range jiraColumn.Statuses {
			if id, ok := statusMap[st.ID]; ok {
				statusIDs = append(statusIDs, id)
			} else if strings.TrimSpace(st.ID) != "" {
				unsupported = append(unsupported, st.ID)
			}
		}
		statusIDs = dedupeInts(statusIDs)
		if len(statusIDs) == 0 {
			continue
		}
		name := strings.TrimSpace(jiraColumn.Name)
		if name == "" {
			name = fmt.Sprintf("Column %d", i+1)
		}
		if strings.EqualFold(name, "backlog") {
			backlogStatusIDs = append(backlogStatusIDs, statusIDs...)
			continue
		}
		columns = append(columns, models.BoardColumnRequest{
			Name:         name,
			DisplayOrder: len(columns),
			WIPLimit:     jiraColumn.Max,
			Color:        boardColumnColor(len(columns)),
			StatusIDs:    statusIDs,
		})
	}
	return columns, dedupeInts(backlogStatusIDs), unsupported
}

func (h *JiraImportHandler) defaultBoardColumnsFromMappedStatuses(statusMap map[string]int) []models.BoardColumnRequest {
	statusIDs := make([]int, 0, len(statusMap))
	seen := map[int]struct{}{}
	for _, id := range statusMap {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		statusIDs = append(statusIDs, id)
	}
	sort.Ints(statusIDs)
	groups := map[int][]int{1: {}, 2: {}, 3: {}}
	categoryIDs, _ := h.imports.StatusCategoryIDs(statusIDs)
	for _, id := range statusIDs {
		categoryID := categoryIDs[id]
		groups[categoryID] = append(groups[categoryID], id)
	}
	defs := []struct {
		categoryID int
		name       string
	}{
		{1, "To Do"},
		{2, "In Progress"},
		{3, "Done"},
	}
	columns := make([]models.BoardColumnRequest, 0, len(defs))
	for _, def := range defs {
		ids := dedupeInts(groups[def.categoryID])
		if len(ids) == 0 {
			continue
		}
		columns = append(columns, models.BoardColumnRequest{Name: def.name, DisplayOrder: len(columns), Color: boardColumnColor(len(columns)), StatusIDs: ids})
	}
	return columns
}

func (h *JiraImportHandler) ensureJiraBoardConfiguration(jobID string, board jira.JiraBoard, collectionID int, columns []models.BoardColumnRequest, backlogStatusIDs []int, metadata map[string]any) (int, bool) {
	jiraID := fmt.Sprintf("board:%d", board.ID)
	if existingID := h.existingMappedEntity(jobID, "board_configuration", jiraID); existingID > 0 {
		return existingID, true
	}
	if len(columns) == 0 {
		slog.Warn("Skipping Jira board configuration with no mapped columns", slog.String("component", "jira"), slog.Int("boardID", board.ID), slog.String("board", board.Name))
		return 0, false
	}
	listColumns := defaultImportedBoardListColumns()
	cardFields := defaultImportedBoardCardFields()
	return h.imports.EnsureBoardConfiguration(jobID, jiraID, board.Name, collectionID, &models.BoardConfigurationRequest{
		Columns: columns, BacklogStatusIDs: dedupeInts(backlogStatusIDs),
		ListColumns: listColumns, CardFields: cardFields, RoadmapConfig: &models.RoadmapConfig{},
	}, metadata)
}

func defaultImportedBoardListColumns() []models.ListColumn {
	fields := []string{"key", "title", "status", "priority", "assignee"}
	cols := make([]models.ListColumn, 0, len(fields))
	for i, field := range fields {
		cols = append(cols, models.ListColumn{FieldIdentifier: field, FieldType: "system", DisplayOrder: i, Width: 1})
	}
	return cols
}

func defaultImportedBoardCardFields() []models.ListColumn {
	fields := []string{"key", "title", "priority", "assignee"}
	cols := make([]models.ListColumn, 0, len(fields))
	for i, field := range fields {
		cols = append(cols, models.ListColumn{FieldIdentifier: field, FieldType: "system", DisplayOrder: i, Width: 1})
	}
	return cols
}

func boardColumnColor(index int) string {
	colors := []string{"#64748b", "#3b82f6", "#22c55e", "#f59e0b", "#a855f7", "#ef4444"}
	return colors[index%len(colors)]
}

func dedupeInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}
