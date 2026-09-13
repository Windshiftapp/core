package services

import "strings"

const iterationSelectQuery = `
	SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
	       i.type_id, it.name as type_name, it.color as type_color,
	       i.is_global, i.workspace_id, w.name as workspace_name,
	       i.created_at, i.updated_at
	FROM iterations i
	LEFT JOIN iteration_types it ON i.type_id = it.id
	LEFT JOIN workspaces w ON i.workspace_id = w.id`

const milestoneSelectQuery = `
	SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
	       mc.name as category_name, mc.color as category_color,
	       m.is_global, m.workspace_id, w.name as workspace_name,
	       m.external_key, m.position,
	       mr.id, mr.tag_name, mr.name, mr.body, mr.is_draft, mr.is_prerelease,
	       mr.target_commitish, mr.workspace_repository_id, mr.tag_url, mr.release_status,
	       CAST(mr.released_at AS TEXT), CAST(mr.assets_json AS TEXT), CAST(mr.last_synced_at AS TEXT),
	       mr.scm_connection_id, mr.scm_repository,
	       mr.scm_release_id, mr.scm_release_url, mr.created_by, mr.created_at,
	       m.created_at, m.updated_at
	FROM milestones m
	LEFT JOIN milestone_categories mc ON m.category_id = mc.id
	LEFT JOIN workspaces w ON m.workspace_id = w.id
	LEFT JOIN (
		SELECT * FROM milestone_releases
		WHERE state = 'created' AND id IN (
			SELECT MAX(id) FROM milestone_releases WHERE state = 'created' GROUP BY milestone_id
		)
	) mr ON mr.milestone_id = m.id`

type planningListQuery struct {
	query      string
	countQuery string
	args       []any
	countArgs  []any
}

func newPlanningListQuery(query, countQuery string) *planningListQuery {
	return &planningListQuery{query: query, countQuery: countQuery}
}

func (q *planningListQuery) addFilter(clause string, args ...any) {
	q.query += " AND " + clause
	q.countQuery += " AND " + clause
	q.args = append(q.args, args...)
	q.countArgs = append(q.countArgs, args...)
}

func (q *planningListQuery) addWorkspaceScope(
	workspaceColumn, globalColumn string,
	workspaceID *int,
	workspaceIDs []int,
	includeGlobal bool,
) {
	switch {
	case workspaceID != nil && includeGlobal:
		q.addFilter("("+workspaceColumn+" = ? OR "+globalColumn+" = ?)", *workspaceID, true)
	case workspaceID != nil:
		q.addFilter(workspaceColumn+" = ?", *workspaceID)
	case len(workspaceIDs) > 0:
		workspaceClause, workspaceArgs := planningWorkspaceFilter(workspaceColumn, workspaceIDs)
		workspaceClause = strings.TrimPrefix(workspaceClause, " AND ")
		if includeGlobal {
			q.addFilter("("+globalColumn+" = ? OR "+workspaceClause+")", append([]any{true}, workspaceArgs...)...)
			return
		}
		q.addFilter(workspaceClause, workspaceArgs...)
	case includeGlobal:
		q.addFilter(globalColumn+" = ?", true)
	default:
		q.addFilter("1=0")
	}
}

func (q *planningListQuery) addNullableIDFilter(column string, id *int) {
	if id == nil {
		return
	}
	if *id == 0 {
		q.addFilter(column + " IS NULL")
		return
	}
	q.addFilter(column+" = ?", *id)
}

func (q *planningListQuery) addStringFilter(column, value string) {
	if value != "" {
		q.addFilter(column+" = ?", value)
	}
}

func (q *planningListQuery) paginated(orderBy string, limit, offset int) (query string, args []any) {
	return q.query + orderBy + " LIMIT ? OFFSET ?", append(q.args, limit, offset)
}
