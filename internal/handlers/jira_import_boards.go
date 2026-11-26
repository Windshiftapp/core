package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"windshift/internal/itemorder"
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

// jiraFilterResolution is the structured outcome of translating saved-filter or
// board JQL for one workspace. Membership and ordering are separate: a filter
// can match the right issues while its source order cannot be reproduced, and
// the importer must say which happened.
type jiraFilterResolution struct {
	QL                  string
	Unsupported         []string
	MembershipComplete  bool
	Scope               jira.JQLProjectScope
	Decision            jira.JQLProjectScopeDecision
	Order               itemorder.Sort
	OrderingDiagnostics []string
}

// resolveJiraFilterQL translates saved-filter or board JQL for one workspace,
// honoring the project predicate. A JQLScopeUnrelated result means the filter
// targets only other projects.
func resolveJiraFilterQL(jql string, workspaceID int, workspaceProjectKeys []string) jiraFilterResolution {
	scope := jira.AnalyzeJQLProjectScope(jql)
	decision := jira.DecideJQLProjectScope(scope, workspaceProjectKeys)
	resolution := jiraFilterResolution{Scope: scope, Decision: decision, MembershipComplete: true}
	if decision == jira.JQLScopeUnrelated {
		resolution.MembershipComplete = false
		return resolution
	}
	translation := jira.TranslateJQL(jql, workspaceID)
	resolution.Unsupported = translation.Unsupported
	resolution.MembershipComplete = translation.MembershipComplete
	resolution.Order = translation.Order
	resolution.OrderingDiagnostics = translation.OrderingDiagnostics
	if decision == jira.JQLScopeNarrowed || decision == jira.JQLScopeIndeterminate {
		// A project predicate that workspace-scoped QL cannot express would
		// otherwise widen the collection to the whole workspace.
		resolution.MembershipComplete = false
	}
	resolution.QL = translation.QL
	if !resolution.MembershipComplete {
		resolution.QL = jira.UnsupportedJQLSelector(workspaceID)
	}
	return resolution
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
		resolution := resolveJiraFilterQL(filter.JQL, workspaceID, workspaceProjectKeys)
		if resolution.Decision == jira.JQLScopeUnrelated {
			slog.Debug("Skipping Jira saved filter scoped to other projects",
				slog.String("component", "jira"),
				slog.String("project", projectKey),
				slog.String("filterID", filter.ID),
				slog.Any("filter_projects", resolution.Scope.Include))
			continue
		}
		description := jiraFilterCollectionDescription(filter, resolution)
		metadata := jiraFilterMetadata(resolution)
		metadata["jira_entity"] = "filter"
		metadata["jira_jql"] = filter.JQL
		if filter.ViewURL != "" {
			metadata["view_url"] = filter.ViewURL
		}
		if resolution.Scope.HasClause {
			metadata["jira_project_predicate"] = append(append([]string{}, resolution.Scope.Include...), resolution.Scope.Exclude...)
		}
		if resolution.Decision == jira.JQLScopeNarrowed || resolution.Decision == jira.JQLScopeIndeterminate {
			metadata["jira_project_scope"] = jiraProjectScopeLabel(resolution.Decision)
			h.recordJiraFilterScopeFinding(jobID, filter.ID, workspaceID, resolution.Scope, resolution.Decision)
		}
		// Identity includes the workspace: the same Jira filter can legitimately
		// back one collection per workspace it targets.
		collectionID, ok := h.ensureJiraCollection(jobID, jiraFilterCollectionID(filter.ID, workspaceID), filter.ID, jiraCollectionName("Jira Filter", filter.Name, filter.ID), description, resolution.QL, resolution.Order, workspaceID, createdByUserID, metadata)
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
		filterReadFailed := false
		if config != nil && config.Filter != nil && strings.TrimSpace(string(config.Filter.ID)) != "" {
			filter, filterErr := client.GetFilter(ctx, string(config.Filter.ID))
			if filterErr != nil {
				filterReadFailed = true
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
		if strings.TrimSpace(jql) == "" && !filterReadFailed {
			jql = fmt.Sprintf("project = %s", projectKey)
		}

		// A board whose filter exists but could not be read has unknown
		// membership. Treat it as empty for review instead of widening it to
		// project = … .
		resolution := jiraFilterResolution{MembershipComplete: true}
		if filterReadFailed {
			resolution = jiraFilterResolution{
				QL:                 jira.UnsupportedJQLSelector(workspaceID),
				MembershipComplete: false,
				Unsupported:        []string{"board filter could not be read; membership is unknown"},
				Decision:           jira.JQLScopeIndeterminate,
			}
			h.recordJiraBoardFilterFinding(jobID, board.ID, workspaceID, config)
		} else {
			resolution = resolveJiraFilterQL(jql, workspaceID, workspaceProjectKeys)
		}

		// A sub-query further restricts the board. Apply it when representable;
		// otherwise the board stays empty for review. A board with a sub-query
		// gets its own collection so the standalone filter keeps its membership.
		boardSpecific := filterReadFailed
		if config != nil && config.SubQuery != nil && strings.TrimSpace(config.SubQuery.Query) != "" {
			boardSpecific = true
			subQuery := strings.TrimSpace(config.SubQuery.Query)
			subResolution := resolveJiraFilterQL(subQuery, workspaceID, workspaceProjectKeys)
			if resolution.MembershipComplete && subResolution.MembershipComplete &&
				resolution.Decision != jira.JQLScopeUnrelated && subResolution.Decision != jira.JQLScopeUnrelated {
				resolution.QL = "(" + resolution.QL + ") AND (" + subResolution.QL + ")"
				resolution.Unsupported = append(resolution.Unsupported, subResolution.Unsupported...)
				resolution.OrderingDiagnostics = append(resolution.OrderingDiagnostics, subResolution.OrderingDiagnostics...)
			} else {
				resolution.QL = jira.UnsupportedJQLSelector(workspaceID)
				resolution.MembershipComplete = false
				resolution.Unsupported = append(resolution.Unsupported, "board sub-query could not be fully translated: "+subQuery)
			}
		}

		metadata := jiraFilterMetadata(resolution)
		metadata["jira_entity"] = "board"
		metadata["jira_board_id"] = board.ID
		metadata["jira_type"] = board.Type
		metadata["jira_jql"] = jql
		for k, v := range filterMeta {
			metadata[k] = v
		}

		// A board's saved filter may already be imported as a standalone
		// collection; reuse it when the membership is identical.
		collectionID := 0
		if !boardSpecific {
			if filterID, ok := filterMeta["jira_filter_id"].(string); ok && strings.TrimSpace(filterID) != "" {
				collectionID = h.existingMappedEntity(jobID, "collection", jiraFilterCollectionID(filterID, workspaceID))
			}
		}
		if collectionID == 0 {
			if resolution.Decision == jira.JQLScopeUnrelated {
				// The board's filter targets other projects, so a collection
				// scoped to this workspace would have the wrong membership.
				slog.Debug("Skipping Jira board whose filter is scoped to other projects",
					slog.String("component", "jira"),
					slog.String("project", projectKey),
					slog.Int("boardID", board.ID))
				continue
			}
			var ok bool
			collectionID, ok = h.ensureJiraCollection(jobID, fmt.Sprintf("board:%d", board.ID), strconv.Itoa(board.ID), jiraCollectionName("Jira Board", board.Name, strconv.Itoa(board.ID)), jiraBoardCollectionDescription(board, config, jql, resolution), resolution.QL, resolution.Order, workspaceID, createdByUserID, metadata)
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

// jiraFilterMetadata records the import fidelity of a translated filter or
// board. Membership loss and ordering loss are reported separately so a
// correctly matched collection is never described as fully faithful when its
// source order could not be reproduced.
func jiraFilterMetadata(resolution jiraFilterResolution) map[string]any {
	metadata := map[string]any{}
	if len(resolution.Unsupported) > 0 {
		metadata["unsupported_jql"] = resolution.Unsupported
	}
	if len(resolution.OrderingDiagnostics) > 0 {
		metadata["unsupported_ordering"] = resolution.OrderingDiagnostics
		metadata["ordering_needs_review"] = true
	}
	if !resolution.MembershipComplete {
		metadata["needs_review"] = true
		metadata["unsupported_membership"] = true
	}
	return metadata
}

func jiraFilterCollectionDescription(filter jira.JiraFilter, resolution jiraFilterResolution) string {
	parts := []string{"Imported from Jira saved filter."}
	if strings.TrimSpace(filter.Description) != "" {
		parts = append(parts, strings.TrimSpace(filter.Description))
	}
	if strings.TrimSpace(filter.JQL) != "" {
		parts = append(parts, "Original JQL:\n```jql\n"+strings.TrimSpace(filter.JQL)+"\n```")
	}
	parts = appendJiraResolutionNotes(parts, resolution)
	return strings.Join(parts, "\n\n")
}

func jiraBoardCollectionDescription(board jira.JiraBoard, config *jira.JiraBoardConfiguration, jql string, resolution jiraFilterResolution) string {
	parts := []string{fmt.Sprintf("Imported from Jira %s board %d.", strings.TrimSpace(board.Type), board.ID)}
	if config != nil && config.SubQuery != nil && strings.TrimSpace(config.SubQuery.Query) != "" {
		parts = append(parts, "Board sub-query:\n```jql\n"+strings.TrimSpace(config.SubQuery.Query)+"\n```")
	}
	if strings.TrimSpace(jql) != "" {
		parts = append(parts, "Original board/filter JQL:\n```jql\n"+strings.TrimSpace(jql)+"\n```")
	}
	parts = appendJiraResolutionNotes(parts, resolution)
	return strings.Join(parts, "\n\n")
}

// appendJiraResolutionNotes records unsupported membership clauses, an
// unavailable source order, and the non-executable state of the collection.
func appendJiraResolutionNotes(parts []string, resolution jiraFilterResolution) []string {
	if len(resolution.Unsupported) > 0 {
		parts = append(parts, "Unsupported JQL clauses not translated into Windshift QL:\n- "+strings.Join(resolution.Unsupported, "\n- "))
	}
	if len(resolution.OrderingDiagnostics) > 0 {
		parts = append(parts, "The source ordering could not be reproduced, so this collection keeps the native order until an administrator reviews it:\n- "+strings.Join(resolution.OrderingDiagnostics, "\n- "))
	}
	if !resolution.MembershipComplete {
		parts = append(parts, "This collection is intentionally empty and needs review: its Jira predicate tree could not be translated in full, so matching every workspace issue would be incorrect.")
	}
	return parts
}

func (h *JiraImportHandler) ensureJiraCollection(jobID, jiraID, jiraKey, name, description, ql string, defaultSort itemorder.Sort, workspaceID, createdByUserID int, metadata map[string]any) (int, bool) {
	return h.imports.EnsureCollection(jobID, jiraID, jiraKey, name, description, ql, defaultSort, workspaceID, createdByUserID, metadata)
}

// recordJiraBoardFilterFinding makes an unreadable board filter explicit. The
// board is imported empty, and the diagnostic names the source filter so an
// administrator can restore the predicate.
func (h *JiraImportHandler) recordJiraBoardFilterFinding(jobID string, boardID, workspaceID int, config *jira.JiraBoardConfiguration) {
	filterID := ""
	if config != nil && config.Filter != nil {
		filterID = string(config.Filter.ID)
	}
	_ = h.recordMapping(jobID, "fidelity_finding", fmt.Sprintf("board-filter:%d:ws%d", boardID, workspaceID), "The board's saved filter could not be read, so its membership is unknown and the board was imported empty for review.", 0, map[string]any{
		"code":           "jira_board_filter_unavailable",
		"severity":       "warning",
		"disposition":    "unavailable",
		"jira_board_id":  boardID,
		"jira_filter_id": filterID,
		"workspace_id":   workspaceID,
	})
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
