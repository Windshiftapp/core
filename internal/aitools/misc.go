package aitools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// oneYearAgoCutoff returns a dialect-appropriate SQL fragment that evaluates
// to "one year before now". Postgres uses INTERVAL syntax; SQLite uses
// datetime() with relative modifiers. Both are returned as bare expressions
// suitable for inlining into a WHERE clause.
func oneYearAgoCutoff(driver string) string {
	if driver == "postgres" {
		return "(NOW() - INTERVAL '1 year')"
	}
	return "datetime('now', '-1 year')"
}

// ----------------------------------------------------------------------------
// list_milestones
// ----------------------------------------------------------------------------

type listMilestonesArgs struct {
	WorkspaceID   int    `json:"workspace_id,omitempty" jsonschema:"Filter to a specific workspace"`
	Status        string `json:"status,omitempty" jsonschema:"Filter by status: planning, in-progress, completed, canceled"`
	IncludeGlobal *bool  `json:"include_global,omitempty" jsonschema:"Include cross-workspace milestones (default true)"`
}

type milestoneDTO struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Status        string `json:"status"`
	TargetDate    string `json:"target_date,omitempty"`
	CategoryName  string `json:"category_name,omitempty"`
	WorkspaceID   int    `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

type listMilestonesOut struct {
	Milestones []milestoneDTO `json:"milestones"`
}

// ----------------------------------------------------------------------------
// list_iterations
// ----------------------------------------------------------------------------

type listIterationsArgs struct {
	WorkspaceID   int    `json:"workspace_id,omitempty" jsonschema:"Filter to a specific workspace"`
	Status        string `json:"status,omitempty" jsonschema:"Filter by status: planned, active, completed, canceled"`
	IncludeGlobal *bool  `json:"include_global,omitempty" jsonschema:"Include cross-workspace iterations (default true)"`
}

type iterationDTO struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Status        string `json:"status"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	TypeName      string `json:"type_name,omitempty"`
	WorkspaceID   int    `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

type listIterationsOut struct {
	Iterations []iterationDTO `json:"iterations"`
}

// ----------------------------------------------------------------------------
// list_custom_fields
// ----------------------------------------------------------------------------

type listCustomFieldsArgs struct{}

type customFieldDTO struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	FieldType   string `json:"field_type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Options     string `json:"options,omitempty"`
}

type listCustomFieldsOut struct {
	CustomFields []customFieldDTO `json:"custom_fields"`
}

// ----------------------------------------------------------------------------
// list_recent_activity
// ----------------------------------------------------------------------------

type listRecentActivityArgs struct {
	SinceDate   string `json:"since_date,omitempty" jsonschema:"Start date (YYYY-MM-DD), defaults to yesterday"`
	WorkspaceID int    `json:"workspace_id,omitempty" jsonschema:"Filter to a specific workspace"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Max items (default 50, max 100)"`
}

type recentChangeDTO struct {
	FieldName string `json:"field"`
	OldValue  string `json:"old_value,omitempty"`
	NewValue  string `json:"new_value,omitempty"`
	ChangedAt string `json:"changed_at"`
	ItemKey   string `json:"item_key"`
	ItemTitle string `json:"item_title"`
	ChangedBy string `json:"changed_by"`
}

type recentCommentDTO struct {
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	ItemKey   string `json:"item_key"`
	ItemTitle string `json:"item_title"`
	Author    string `json:"author"`
}

type listRecentActivityOut struct {
	Changes  []recentChangeDTO  `json:"changes"`
	Comments []recentCommentDTO `json:"comments"`
}

func init() {
	Register(Default, Tool[listMilestonesArgs]{
		Name:        "list_milestones",
		Description: "List milestones the user can see, with optional workspace, status and global-include filters.",
		Run: func(_ context.Context, env *Env, args listMilestonesArgs) (any, error) {
			includeGlobal := true
			if args.IncludeGlobal != nil {
				includeGlobal = *args.IncludeGlobal
			}
			oneYearAgo := oneYearAgoCutoff(env.DB.GetDriverName())
			query := `SELECT m.id, m.name, COALESCE(m.description, ''), m.status,
			       COALESCE(CAST(m.target_date AS TEXT), ''),
			       COALESCE(mc.name, ''),
			       COALESCE(m.workspace_id, 0), COALESCE(w.name, '')
			       FROM milestones m
			       LEFT JOIN milestone_categories mc ON m.category_id = mc.id
			       LEFT JOIN workspaces w ON m.workspace_id = w.id
			       WHERE NOT (m.status IN ('completed', 'cancelled') AND m.updated_at < ` + oneYearAgo + `)`
			var qa []interface{}
			var accessParts []string
			if includeGlobal {
				accessParts = append(accessParts, "m.is_global = true")
			}
			if args.WorkspaceID > 0 {
				if !env.HasWorkspaceAccess(args.WorkspaceID) {
					return map[string]string{"error": "workspace not found"}, nil
				}
				accessParts = append(accessParts, "m.workspace_id = ?")
				qa = append(qa, args.WorkspaceID)
			} else if len(env.AccessibleWorkspaceIDs) > 0 {
				ph := make([]string, len(env.AccessibleWorkspaceIDs))
				for i, id := range env.AccessibleWorkspaceIDs {
					ph[i] = "?"
					qa = append(qa, id)
				}
				accessParts = append(accessParts, fmt.Sprintf("m.workspace_id IN (%s)", strings.Join(ph, ",")))
			}
			if len(accessParts) > 0 {
				query += " AND (" + strings.Join(accessParts, " OR ") + ")"
			}
			if args.Status != "" {
				query += " AND m.status = ?"
				qa = append(qa, args.Status)
			}
			query += " ORDER BY m.status, m.target_date NULLS LAST, m.name"
			rows, err := env.DB.Query(query, qa...)
			if err != nil {
				return nil, err
			}
			defer func() { _ = rows.Close() }()
			out := listMilestonesOut{Milestones: []milestoneDTO{}}
			for rows.Next() {
				var m milestoneDTO
				if err := rows.Scan(&m.ID, &m.Name, &m.Description, &m.Status, &m.TargetDate, &m.CategoryName, &m.WorkspaceID, &m.WorkspaceName); err != nil {
					continue
				}
				out.Milestones = append(out.Milestones, m)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return out, nil
		},
	})

	Register(Default, Tool[listIterationsArgs]{
		Name:        "list_iterations",
		Description: "List iterations (sprints, PIs, releases) the user can see.",
		Run: func(_ context.Context, env *Env, args listIterationsArgs) (any, error) {
			includeGlobal := true
			if args.IncludeGlobal != nil {
				includeGlobal = *args.IncludeGlobal
			}
			oneYearAgo := oneYearAgoCutoff(env.DB.GetDriverName())
			// Iterations created without dates have NULL start_date/end_date.
			// `null < timestamp` is NULL (≈ false) in both Postgres and SQLite,
			// which would silently drop them from the result. Treat NULL
			// end_date as "not stale" so newly seeded completed iterations
			// still surface.
			query := `SELECT iter.id, iter.name, COALESCE(iter.description, ''), iter.status,
			       CAST(iter.start_date AS TEXT), CAST(iter.end_date AS TEXT),
			       COALESCE(it.name, ''),
			       COALESCE(iter.workspace_id, 0), COALESCE(w.name, '')
			       FROM iterations iter
			       LEFT JOIN iteration_types it ON iter.type_id = it.id
			       LEFT JOIN workspaces w ON iter.workspace_id = w.id
			       WHERE NOT (iter.status IN ('completed', 'cancelled') AND iter.end_date IS NOT NULL AND iter.end_date < ` + oneYearAgo + `)`
			var qa []interface{}
			var accessParts []string
			if includeGlobal {
				accessParts = append(accessParts, "iter.is_global = true")
			}
			if args.WorkspaceID > 0 {
				if !env.HasWorkspaceAccess(args.WorkspaceID) {
					return map[string]string{"error": "workspace not found"}, nil
				}
				accessParts = append(accessParts, "iter.workspace_id = ?")
				qa = append(qa, args.WorkspaceID)
			} else if len(env.AccessibleWorkspaceIDs) > 0 {
				ph := make([]string, len(env.AccessibleWorkspaceIDs))
				for i, id := range env.AccessibleWorkspaceIDs {
					ph[i] = "?"
					qa = append(qa, id)
				}
				accessParts = append(accessParts, fmt.Sprintf("iter.workspace_id IN (%s)", strings.Join(ph, ",")))
			}
			if len(accessParts) > 0 {
				query += " AND (" + strings.Join(accessParts, " OR ") + ")"
			}
			if args.Status != "" {
				query += " AND iter.status = ?"
				qa = append(qa, args.Status)
			}
			query += " ORDER BY iter.status, iter.start_date, iter.name"
			rows, err := env.DB.Query(query, qa...)
			if err != nil {
				return nil, err
			}
			defer func() { _ = rows.Close() }()
			out := listIterationsOut{Iterations: []iterationDTO{}}
			for rows.Next() {
				var it iterationDTO
				if err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.Status, &it.StartDate, &it.EndDate, &it.TypeName, &it.WorkspaceID, &it.WorkspaceName); err != nil {
					continue
				}
				out.Iterations = append(out.Iterations, it)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return out, nil
		},
	})

	Register(Default, Tool[listCustomFieldsArgs]{
		Name:        "list_custom_fields",
		Description: "List available custom field definitions. Use this to discover what custom fields exist before filtering items with cf_<name> in the filter parameter of list_items.",
		Run: func(_ context.Context, env *Env, _ listCustomFieldsArgs) (any, error) {
			rows, err := env.DB.Query(
				"SELECT id, name, field_type, COALESCE(description, ''), required, COALESCE(options, '') FROM custom_field_definitions ORDER BY display_order, name",
			)
			if err != nil {
				return nil, err
			}
			defer func() { _ = rows.Close() }()
			out := listCustomFieldsOut{CustomFields: []customFieldDTO{}}
			for rows.Next() {
				var cf customFieldDTO
				if err := rows.Scan(&cf.ID, &cf.Name, &cf.FieldType, &cf.Description, &cf.Required, &cf.Options); err != nil {
					continue
				}
				out.CustomFields = append(out.CustomFields, cf)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return out, nil
		},
	})

	Register(Default, Tool[listRecentActivityArgs]{
		Name:        "list_recent_activity",
		Description: "List recent changes and comments across accessible workspaces. Useful for understanding what happened recently.",
		Run: func(_ context.Context, env *Env, args listRecentActivityArgs) (any, error) {
			sinceDate := args.SinceDate
			if sinceDate == "" {
				sinceDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
			}
			if _, err := time.Parse("2006-01-02", sinceDate); err != nil {
				return map[string]string{"error": "invalid since_date format, use YYYY-MM-DD"}, nil
			}
			limit := args.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 100 {
				limit = 100
			}
			var wsIDs []int
			if args.WorkspaceID > 0 {
				if !env.HasWorkspaceAccess(args.WorkspaceID) {
					return map[string]string{"error": "workspace not found"}, nil
				}
				wsIDs = []int{args.WorkspaceID}
			} else {
				wsIDs = env.AccessibleWorkspaceIDs
			}
			out := listRecentActivityOut{Changes: []recentChangeDTO{}, Comments: []recentCommentDTO{}}
			if len(wsIDs) == 0 {
				return out, nil
			}
			ph := make([]string, len(wsIDs))
			wsArgs := make([]interface{}, len(wsIDs))
			for i, id := range wsIDs {
				ph[i] = "?"
				wsArgs[i] = id
			}
			wsIn := strings.Join(ph, ",")

			changeQuery := fmt.Sprintf(`SELECT ih.field_name, COALESCE(ih.old_value, ''), COALESCE(ih.new_value, ''), ih.changed_at,
				w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
				COALESCE(u.first_name || ' ' || u.last_name, 'Unknown') as changed_by
				FROM item_history ih
				JOIN items i ON ih.item_id = i.id
				JOIN workspaces w ON i.workspace_id = w.id
				LEFT JOIN users u ON ih.user_id = u.id
				WHERE i.workspace_id IN (%s) AND ih.changed_at >= ?
				ORDER BY ih.changed_at DESC LIMIT ?`, wsIn)
			cArgs := append(append([]interface{}{}, wsArgs...), sinceDate, limit)
			rows, err := env.DB.Query(changeQuery, cArgs...)
			if err != nil {
				return nil, err
			}
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var c recentChangeDTO
				var changedAt time.Time
				if err := rows.Scan(&c.FieldName, &c.OldValue, &c.NewValue, &changedAt, &c.ItemKey, &c.ItemTitle, &c.ChangedBy); err != nil {
					continue
				}
				c.ChangedAt = changedAt.Format(time.RFC3339)
				out.Changes = append(out.Changes, c)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}

			commentQuery := fmt.Sprintf(`SELECT c.content, c.created_at,
				w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
				COALESCE(u.first_name || ' ' || u.last_name, 'Unknown') as author
				FROM comments c
				JOIN items i ON c.item_id = i.id
				JOIN workspaces w ON i.workspace_id = w.id
				LEFT JOIN users u ON c.author_id = u.id
				WHERE i.workspace_id IN (%s) AND c.created_at >= ? AND c.is_private = false
				ORDER BY c.created_at DESC LIMIT ?`, wsIn)
			ccArgs := append(append([]interface{}{}, wsArgs...), sinceDate, 30)
			cRows, err := env.DB.Query(commentQuery, ccArgs...)
			if err != nil {
				return nil, err
			}
			defer func() { _ = cRows.Close() }()
			for cRows.Next() {
				var cm recentCommentDTO
				var createdAt time.Time
				if err := cRows.Scan(&cm.Content, &createdAt, &cm.ItemKey, &cm.ItemTitle, &cm.Author); err != nil {
					continue
				}
				cm.CreatedAt = createdAt.Format(time.RFC3339)
				if len(cm.Content) > 200 {
					cm.Content = cm.Content[:200] + "..."
				}
				out.Comments = append(out.Comments, cm)
			}
			if err := cRows.Err(); err != nil {
				return nil, err
			}
			return out, nil
		},
	})
}
