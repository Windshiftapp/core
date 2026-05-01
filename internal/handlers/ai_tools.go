package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// ToolExecutor executes tool calls on behalf of the agentic chat loop.
// It enforces workspace access via a pre-computed list of accessible workspace IDs.
type ToolExecutor struct {
	db                     database.Database
	accessibleWorkspaceIDs []int
	userID                 int
	timePermService        *services.TimePermissionService
	permService            *services.PermissionService
	commentService         *services.CommentService
}

// NewToolExecutor creates a tool executor scoped to the given user's accessible workspaces.
func NewToolExecutor(db database.Database, accessibleWorkspaceIDs []int, userID int, timePermService *services.TimePermissionService, permService *services.PermissionService, commentService *services.CommentService) *ToolExecutor {
	return &ToolExecutor{
		db:                     db,
		accessibleWorkspaceIDs: accessibleWorkspaceIDs,
		userID:                 userID,
		timePermService:        timePermService,
		permService:            permService,
		commentService:         commentService,
	}
}

// Execute dispatches a tool call by name and returns the JSON result.
func (e *ToolExecutor) Execute(_ context.Context, name, arguments string) (string, error) {
	switch name {
	case "list_workspaces":
		return e.listWorkspaces()
	case "get_workspace":
		return e.getWorkspace(arguments)
	case "list_items":
		return e.listItems(arguments)
	case "get_item":
		return e.getItem(arguments)
	case "search_items":
		return e.searchItems(arguments)
	case "list_milestones":
		return e.listMilestones(arguments)
	case "list_iterations":
		return e.listIterations(arguments)
	case "list_custom_fields":
		return e.listCustomFields()
	case "list_time_projects":
		return e.listTimeProjects(arguments)
	case "list_worklogs":
		return e.listWorklogs(arguments)
	case "list_recent_activity":
		return e.listRecentActivity(arguments)
	case "update_item":
		return e.updateItem(arguments)
	case "transition_item":
		return e.transitionItem(arguments)
	case "log_time":
		return e.logTime(arguments)
	case "list_comments":
		return e.listComments(arguments)
	case "add_comment":
		return e.addComment(arguments)
	default:
		return `{"error": "unknown tool"}`, nil
	}
}

func (e *ToolExecutor) hasWorkspaceAccess(workspaceID int) bool {
	for _, id := range e.accessibleWorkspaceIDs {
		if id == workspaceID {
			return true
		}
	}
	return false
}

// listWorkspaces returns all accessible workspaces.
func (e *ToolExecutor) listWorkspaces() (string, error) {
	if len(e.accessibleWorkspaceIDs) == 0 {
		return `{"workspaces": []}`, nil
	}

	placeholders := make([]string, len(e.accessibleWorkspaceIDs))
	args := make([]interface{}, len(e.accessibleWorkspaceIDs))
	for i, id := range e.accessibleWorkspaceIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := e.db.Query(
		fmt.Sprintf("SELECT id, name, key, description FROM workspaces WHERE id IN (%s) AND active = true ORDER BY name",
			strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return "", fmt.Errorf("failed to list workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type ws struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Key         string `json:"key"`
		Description string `json:"description,omitempty"`
	}
	var workspaces []ws
	for rows.Next() {
		var w ws
		var desc sql.NullString
		if err := rows.Scan(&w.ID, &w.Name, &w.Key, &desc); err != nil {
			continue
		}
		if desc.Valid {
			w.Description = desc.String
		}
		workspaces = append(workspaces, w)
	}
	if workspaces == nil {
		workspaces = []ws{}
	}

	b, _ := json.Marshal(map[string]interface{}{"workspaces": workspaces})
	return string(b), nil
}

// getWorkspace returns details for a single workspace.
func (e *ToolExecutor) getWorkspace(arguments string) (string, error) {
	var args struct {
		WorkspaceID int `json:"workspace_id"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}
	if !e.hasWorkspaceAccess(args.WorkspaceID) {
		return `{"error": "workspace not found"}`, nil
	}

	wsSvc := services.NewWorkspaceService(e.db)
	ws, err := wsSvc.GetByID(args.WorkspaceID)
	if err != nil {
		return `{"error": "workspace not found"}`, nil
	}

	type result struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Key         string `json:"key"`
		Description string `json:"description,omitempty"`
		Active      bool   `json:"active"`
		IsPersonal  bool   `json:"is_personal"`
	}
	b, _ := json.Marshal(result{
		ID:          ws.ID,
		Name:        ws.Name,
		Key:         ws.Key,
		Description: ws.Description,
		Active:      ws.Active,
		IsPersonal:  ws.IsPersonal,
	})
	return string(b), nil
}

// listItems returns items in a workspace with optional filters.
func (e *ToolExecutor) listItems(arguments string) (string, error) {
	var args struct {
		WorkspaceID int    `json:"workspace_id"`
		Status      string `json:"status"`
		AssigneeID  int    `json:"assignee_id"`
		Limit       int    `json:"limit"`
		Filter      string `json:"filter"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}

	// Determine which workspaces to query
	var wsIDs []int
	if args.WorkspaceID > 0 {
		if !e.hasWorkspaceAccess(args.WorkspaceID) {
			return `{"error": "workspace not found"}`, nil
		}
		wsIDs = []int{args.WorkspaceID}
	} else {
		wsIDs = e.accessibleWorkspaceIDs
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	// Build QL filter for optional status and assignee
	var qlParts []string
	var qlArgs []interface{}
	if args.Status != "" {
		qlParts = append(qlParts, "st.name = ?")
		qlArgs = append(qlArgs, args.Status)
	}
	if args.AssigneeID > 0 {
		qlParts = append(qlParts, "i.assignee_id = ?")
		qlArgs = append(qlArgs, args.AssigneeID)
	}

	// Evaluate CQL filter expression if provided
	if args.Filter != "" {
		workspaceMap := make(map[string]int)
		wsRows, err := e.db.Query("SELECT id, name, key FROM workspaces")
		if err == nil {
			defer func() { _ = wsRows.Close() }()
			for wsRows.Next() {
				var id int
				var name, key string
				if err := wsRows.Scan(&id, &name, &key); err == nil {
					idStr := strconv.Itoa(id)
					workspaceMap[idStr] = id
					workspaceMap[strings.ToLower(name)] = id
					workspaceMap[strings.ToLower(key)] = id
				}
			}
		}

		evaluator := cql.NewEvaluator(workspaceMap, e.db.GetDriverName())
		resolvedFilter := cql.SubstituteFunctions(args.Filter, cql.UserContext(e.userID))
		cqlSQL, cqlArgs, err := evaluator.EvaluateToSQL(resolvedFilter)
		if err != nil {
			return fmt.Sprintf(`{"error": "invalid filter expression: %s"}`, err.Error()), nil
		}
		if cqlSQL != "" {
			qlParts = append(qlParts, cqlSQL)
			qlArgs = append(qlArgs, cqlArgs...)
		}
	}

	var filters services.ItemFilters
	if len(qlParts) > 0 {
		filters.QLQuery = strings.Join(qlParts, " AND ")
		filters.QLArgs = qlArgs
	}

	crudSvc := services.NewItemCRUDService(e.db)
	items, total, err := crudSvc.List(services.ItemListParams{
		WorkspaceIDs: wsIDs,
		Filters:      filters,
		SortBy:       "created_at",
		SortAsc:      false,
		Pagination:   services.PaginationParams{Limit: limit},
	})
	if err != nil {
		return `{"error": "failed to list items"}`, nil
	}

	type itemSummary struct {
		ID                  int    `json:"id"`
		Key                 string `json:"key"`
		Title               string `json:"title"`
		Status              string `json:"status,omitempty"`
		Priority            string `json:"priority,omitempty"`
		Assignee            string `json:"assignee,omitempty"`
		DueDate             string `json:"due_date,omitempty"`
		Type                string `json:"type,omitempty"`
		MilestoneName       string `json:"milestone_name,omitempty"`
		MilestoneTargetDate string `json:"milestone_target_date,omitempty"`
		IterationName       string `json:"iteration_name,omitempty"`
		IterationEndDate    string `json:"iteration_end_date,omitempty"`
		WorkspaceID         int    `json:"workspace_id"`
	}
	results := make([]itemSummary, 0, len(items))
	for _, item := range items {
		s := itemSummary{
			ID:                  item.ID,
			Key:                 fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber),
			Title:               item.Title,
			Status:              item.StatusName,
			Priority:            item.PriorityName,
			Assignee:            item.AssigneeName,
			Type:                item.ItemTypeName,
			MilestoneName:       item.MilestoneName,
			MilestoneTargetDate: item.MilestoneTargetDate,
			IterationName:       item.IterationName,
			IterationEndDate:    item.IterationEndDate,
			WorkspaceID:         item.WorkspaceID,
		}
		if item.DueDate != nil {
			s.DueDate = item.DueDate.Format("2006-01-02")
		}
		results = append(results, s)
	}

	b, _ := json.Marshal(map[string]interface{}{"items": results, "total": total})
	return string(b), nil
}

// getItem returns details for a single item by ID or key.
func (e *ToolExecutor) getItem(arguments string) (string, error) {
	var args struct {
		ItemID  int    `json:"item_id"`
		ItemKey string `json:"item_key"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}

	crudSvc := services.NewItemCRUDService(e.db)
	var itemID int

	switch {
	case args.ItemID > 0:
		itemID = args.ItemID
	case args.ItemKey != "":
		// Parse key like "PROJ-42"
		parts := strings.SplitN(strings.ToUpper(args.ItemKey), "-", 2)
		if len(parts) != 2 {
			return `{"error": "invalid item key format, expected KEY-NUMBER"}`, nil
		}
		num, err := strconv.Atoi(parts[1])
		if err != nil {
			return `{"error": "invalid item key format, expected KEY-NUMBER"}`, nil
		}
		// Look up by workspace key + item number
		itemID, err = repository.NewItemRepository(e.db).FindIDByKeyAndNumber(parts[0], num)
		if err != nil {
			return `{"error": "item not found"}`, nil
		}
	default:
		return `{"error": "must provide item_id or item_key"}`, nil
	}

	// Check workspace access
	wsID, err := crudSvc.GetWorkspaceID(itemID)
	if err != nil {
		return `{"error": "item not found"}`, nil
	}
	if !e.hasWorkspaceAccess(wsID) {
		return `{"error": "item not found"}`, nil
	}

	item, err := crudSvc.GetByID(itemID)
	if err != nil {
		return `{"error": "item not found"}`, nil
	}

	type itemDetail struct {
		ID                  int      `json:"id"`
		Key                 string   `json:"key"`
		Title               string   `json:"title"`
		Description         string   `json:"description,omitempty"`
		Status              string   `json:"status,omitempty"`
		Priority            string   `json:"priority,omitempty"`
		Assignee            string   `json:"assignee,omitempty"`
		Creator             string   `json:"creator,omitempty"`
		DueDate             string   `json:"due_date,omitempty"`
		Type                string   `json:"type,omitempty"`
		MilestoneName       string   `json:"milestone_name,omitempty"`
		MilestoneTargetDate string   `json:"milestone_target_date,omitempty"`
		IterationName       string   `json:"iteration_name,omitempty"`
		IterationEndDate    string   `json:"iteration_end_date,omitempty"`
		Workspace           string   `json:"workspace"`
		WorkspaceID         int      `json:"workspace_id"`
		Labels              []string `json:"labels,omitempty"`
	}

	d := itemDetail{
		ID:                  item.ID,
		Key:                 fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber),
		Title:               item.Title,
		Status:              item.StatusName,
		Priority:            item.PriorityName,
		Assignee:            item.AssigneeName,
		Creator:             item.CreatorName,
		Type:                item.ItemTypeName,
		MilestoneName:       item.MilestoneName,
		MilestoneTargetDate: item.MilestoneTargetDate,
		IterationName:       item.IterationName,
		IterationEndDate:    item.IterationEndDate,
		Workspace:           item.WorkspaceName,
		WorkspaceID:         wsID,
	}
	if item.Description != "" {
		desc := item.Description
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}
		d.Description = desc
	}
	if item.DueDate != nil {
		d.DueDate = item.DueDate.Format("2006-01-02")
	}
	for _, l := range item.Labels {
		d.Labels = append(d.Labels, l.Name)
	}

	b, _ := json.Marshal(d)
	return string(b), nil
}

// searchItems searches for items by text across accessible workspaces.
func (e *ToolExecutor) searchItems(arguments string) (string, error) {
	var args struct {
		Query        string `json:"query"`
		WorkspaceIDs []int  `json:"workspace_ids"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}
	if args.Query == "" {
		return `{"error": "query is required"}`, nil
	}

	// Filter workspace IDs to only accessible ones
	var searchWSIDs []int
	if len(args.WorkspaceIDs) > 0 {
		for _, id := range args.WorkspaceIDs {
			if e.hasWorkspaceAccess(id) {
				searchWSIDs = append(searchWSIDs, id)
			}
		}
		if len(searchWSIDs) == 0 {
			return `{"items": [], "total": 0}`, nil
		}
	} else {
		searchWSIDs = e.accessibleWorkspaceIDs
	}

	crudSvc := services.NewItemCRUDService(e.db)
	items, total, err := crudSvc.Search(args.Query, searchWSIDs, services.PaginationParams{Limit: 20})
	if err != nil {
		return `{"error": "search failed"}`, nil
	}

	type itemSummary struct {
		ID          int    `json:"id"`
		Key         string `json:"key"`
		Title       string `json:"title"`
		Status      string `json:"status,omitempty"`
		Priority    string `json:"priority,omitempty"`
		Assignee    string `json:"assignee,omitempty"`
		WorkspaceID int    `json:"workspace_id"`
	}
	results := make([]itemSummary, 0, len(items))
	for _, item := range items {
		// Filter results to accessible workspaces
		if !e.hasWorkspaceAccess(item.WorkspaceID) {
			continue
		}
		results = append(results, itemSummary{
			ID:          item.ID,
			Key:         fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber),
			Title:       item.Title,
			Status:      item.StatusName,
			Priority:    item.PriorityName,
			Assignee:    item.AssigneeName,
			WorkspaceID: item.WorkspaceID,
		})
	}

	b, _ := json.Marshal(map[string]interface{}{"items": results, "total": total})
	return string(b), nil
}

// listMilestones returns milestones the user can see.
func (e *ToolExecutor) listMilestones(arguments string) (string, error) {
	var args struct {
		WorkspaceID   int    `json:"workspace_id"`
		Status        string `json:"status"`
		IncludeGlobal *bool  `json:"include_global"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}

	includeGlobal := true
	if args.IncludeGlobal != nil {
		includeGlobal = *args.IncludeGlobal
	}

	query := `SELECT m.id, m.name, COALESCE(m.description, ''), m.status,
		COALESCE(CAST(m.target_date AS TEXT), ''),
		COALESCE(mc.name, ''),
		COALESCE(m.workspace_id, 0), COALESCE(w.name, '')
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN workspaces w ON m.workspace_id = w.id
		WHERE NOT (m.status IN ('completed', 'cancelled') AND m.updated_at < NOW() - INTERVAL '1 year')`

	var queryArgs []interface{}

	// Access control: global milestones always visible; workspace-scoped only if user has access
	var accessParts []string
	if includeGlobal {
		accessParts = append(accessParts, "m.is_global = true")
	}
	if args.WorkspaceID > 0 {
		if !e.hasWorkspaceAccess(args.WorkspaceID) {
			return `{"error": "workspace not found"}`, nil
		}
		accessParts = append(accessParts, "m.workspace_id = ?")
		queryArgs = append(queryArgs, args.WorkspaceID)
	} else if len(e.accessibleWorkspaceIDs) > 0 {
		placeholders := make([]string, len(e.accessibleWorkspaceIDs))
		for i, id := range e.accessibleWorkspaceIDs {
			placeholders[i] = "?"
			queryArgs = append(queryArgs, id)
		}
		accessParts = append(accessParts, fmt.Sprintf("m.workspace_id IN (%s)", strings.Join(placeholders, ",")))
	}
	if len(accessParts) > 0 {
		query += " AND (" + strings.Join(accessParts, " OR ") + ")"
	}

	if args.Status != "" {
		query += " AND m.status = ?"
		queryArgs = append(queryArgs, args.Status)
	}

	query += " ORDER BY m.status, m.target_date NULLS LAST, m.name"

	rows, err := e.db.Query(query, queryArgs...)
	if err != nil {
		return `{"error": "failed to list milestones"}`, nil
	}
	defer func() { _ = rows.Close() }()

	type milestone struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		Status        string `json:"status"`
		TargetDate    string `json:"target_date,omitempty"`
		CategoryName  string `json:"category_name,omitempty"`
		WorkspaceID   int    `json:"workspace_id,omitempty"`
		WorkspaceName string `json:"workspace_name,omitempty"`
	}
	var milestones []milestone
	for rows.Next() {
		var ms milestone
		if err := rows.Scan(&ms.ID, &ms.Name, &ms.Description, &ms.Status, &ms.TargetDate, &ms.CategoryName, &ms.WorkspaceID, &ms.WorkspaceName); err != nil {
			continue
		}
		milestones = append(milestones, ms)
	}
	if milestones == nil {
		milestones = []milestone{}
	}

	b, _ := json.Marshal(map[string]interface{}{"milestones": milestones})
	return string(b), nil
}

// listIterations returns iterations the user can see.
func (e *ToolExecutor) listIterations(arguments string) (string, error) {
	var args struct {
		WorkspaceID   int    `json:"workspace_id"`
		Status        string `json:"status"`
		IncludeGlobal *bool  `json:"include_global"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}

	includeGlobal := true
	if args.IncludeGlobal != nil {
		includeGlobal = *args.IncludeGlobal
	}

	query := `SELECT iter.id, iter.name, COALESCE(iter.description, ''), iter.status,
		CAST(iter.start_date AS TEXT), CAST(iter.end_date AS TEXT),
		COALESCE(it.name, ''),
		COALESCE(iter.workspace_id, 0), COALESCE(w.name, '')
		FROM iterations iter
		LEFT JOIN iteration_types it ON iter.type_id = it.id
		LEFT JOIN workspaces w ON iter.workspace_id = w.id
		WHERE NOT (iter.status IN ('completed', 'cancelled') AND iter.end_date < NOW() - INTERVAL '1 year')`

	var queryArgs []interface{}

	// Access control: global iterations always visible; workspace-scoped only if user has access
	var accessParts []string
	if includeGlobal {
		accessParts = append(accessParts, "iter.is_global = true")
	}
	if args.WorkspaceID > 0 {
		if !e.hasWorkspaceAccess(args.WorkspaceID) {
			return `{"error": "workspace not found"}`, nil
		}
		accessParts = append(accessParts, "iter.workspace_id = ?")
		queryArgs = append(queryArgs, args.WorkspaceID)
	} else if len(e.accessibleWorkspaceIDs) > 0 {
		placeholders := make([]string, len(e.accessibleWorkspaceIDs))
		for i, id := range e.accessibleWorkspaceIDs {
			placeholders[i] = "?"
			queryArgs = append(queryArgs, id)
		}
		accessParts = append(accessParts, fmt.Sprintf("iter.workspace_id IN (%s)", strings.Join(placeholders, ",")))
	}
	if len(accessParts) > 0 {
		query += " AND (" + strings.Join(accessParts, " OR ") + ")"
	}

	if args.Status != "" {
		query += " AND iter.status = ?"
		queryArgs = append(queryArgs, args.Status)
	}

	query += " ORDER BY iter.status, iter.start_date, iter.name"

	rows, err := e.db.Query(query, queryArgs...)
	if err != nil {
		return `{"error": "failed to list iterations"}`, nil
	}
	defer func() { _ = rows.Close() }()

	type iteration struct {
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
	var iterations []iteration
	for rows.Next() {
		var it iteration
		if err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.Status, &it.StartDate, &it.EndDate, &it.TypeName, &it.WorkspaceID, &it.WorkspaceName); err != nil {
			continue
		}
		iterations = append(iterations, it)
	}
	if iterations == nil {
		iterations = []iteration{}
	}

	b, _ := json.Marshal(map[string]interface{}{"iterations": iterations})
	return string(b), nil
}

// listCustomFields returns all custom field definitions.
func (e *ToolExecutor) listCustomFields() (string, error) {
	rows, err := e.db.Query(
		"SELECT id, name, field_type, COALESCE(description, ''), required, COALESCE(options, '') FROM custom_field_definitions ORDER BY display_order, name",
	)
	if err != nil {
		return `{"error": "failed to list custom fields"}`, nil
	}
	defer func() { _ = rows.Close() }()

	type customField struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		FieldType   string `json:"field_type"`
		Description string `json:"description,omitempty"`
		Required    bool   `json:"required"`
		Options     string `json:"options,omitempty"`
	}
	var fields []customField
	for rows.Next() {
		var cf customField
		if err := rows.Scan(&cf.ID, &cf.Name, &cf.FieldType, &cf.Description, &cf.Required, &cf.Options); err != nil {
			continue
		}
		fields = append(fields, cf)
	}
	if fields == nil {
		fields = []customField{}
	}

	b, _ := json.Marshal(map[string]interface{}{"custom_fields": fields})
	return string(b), nil
}

// listTimeProjects returns time tracking projects the user has access to.
func (e *ToolExecutor) listTimeProjects(arguments string) (string, error) {
	var args struct {
		Status string `json:"status"`
	}
	if arguments != "" {
		_ = json.Unmarshal([]byte(arguments), &args)
	}

	// Get accessible project IDs for the user
	accessibleIDs, err := e.timePermService.GetAccessibleProjects(e.userID)
	if err != nil {
		return `{"error": "failed to check project access"}`, nil
	}
	// accessibleIDs == nil means all projects are accessible
	// empty slice means no access
	if accessibleIDs != nil && len(accessibleIDs) == 0 {
		return `{"projects": []}`, nil
	}

	query := `SELECT tp.id, tp.name, tp.status, COALESCE(tp.description, ''),
		COALESCE(co.name, ''), COALESCE(tpc.name, '')
		FROM time_projects tp
		LEFT JOIN customer_organisations co ON tp.customer_id = co.id
		LEFT JOIN time_project_categories tpc ON tp.category_id = tpc.id
		WHERE 1=1`

	var queryArgs []interface{}

	if accessibleIDs != nil {
		placeholders := make([]string, len(accessibleIDs))
		for i, id := range accessibleIDs {
			placeholders[i] = "?"
			queryArgs = append(queryArgs, id)
		}
		query += fmt.Sprintf(" AND tp.id IN (%s)", strings.Join(placeholders, ","))
	}

	if args.Status != "" {
		query += " AND tp.status = ?"
		queryArgs = append(queryArgs, args.Status)
	}

	query += " ORDER BY tp.name"

	rows, err := e.db.Query(query, queryArgs...)
	if err != nil {
		return `{"error": "failed to list time projects"}`, nil
	}
	defer func() { _ = rows.Close() }()

	type project struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Status       string `json:"status"`
		Description  string `json:"description,omitempty"`
		CustomerName string `json:"customer_name,omitempty"`
		CategoryName string `json:"category_name,omitempty"`
	}
	var projects []project
	for rows.Next() {
		var p project
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.Description, &p.CustomerName, &p.CategoryName); err != nil {
			continue
		}
		projects = append(projects, p)
	}
	if projects == nil {
		projects = []project{}
	}

	b, _ := json.Marshal(map[string]interface{}{"projects": projects})
	return string(b), nil
}

// listWorklogs returns the current user's worklogs with optional filters.
func (e *ToolExecutor) listWorklogs(arguments string) (string, error) {
	var args struct {
		DateFrom  string `json:"date_from"`
		DateTo    string `json:"date_to"`
		ProjectID int    `json:"project_id"`
	}
	if arguments != "" {
		_ = json.Unmarshal([]byte(arguments), &args)
	}

	query := `SELECT tw.id, tp.name, COALESCE(co.name, ''), tw.description, tw.date,
		tw.duration_minutes, COALESCE(tw.item_id, 0),
		COALESCE(i.workspace_item_number, 0), COALESCE(w.key, ''), COALESCE(i.workspace_id, 0)
		FROM time_worklogs tw
		JOIN time_projects tp ON tw.project_id = tp.id
		LEFT JOIN customer_organisations co ON tw.customer_id = co.id
		LEFT JOIN items i ON tw.item_id = i.id
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		WHERE tw.user_id = ?`

	queryArgs := []interface{}{e.userID}

	if args.DateFrom != "" {
		dateFrom, err := time.Parse("2006-01-02", args.DateFrom)
		if err != nil {
			return `{"error": "invalid date_from format, use YYYY-MM-DD"}`, nil
		}
		query += " AND tw.date >= ?"
		queryArgs = append(queryArgs, dateFrom.Unix())
	}

	if args.DateTo != "" {
		dateTo, err := time.Parse("2006-01-02", args.DateTo)
		if err != nil {
			return `{"error": "invalid date_to format, use YYYY-MM-DD"}`, nil
		}
		// End of day
		query += " AND tw.date <= ?"
		queryArgs = append(queryArgs, dateTo.Add(24*time.Hour-time.Second).Unix())
	}

	if args.ProjectID > 0 {
		query += " AND tw.project_id = ?"
		queryArgs = append(queryArgs, args.ProjectID)
	}

	query += " ORDER BY tw.date DESC LIMIT 50"

	rows, err := e.db.Query(query, queryArgs...)
	if err != nil {
		return `{"error": "failed to list worklogs"}`, nil
	}
	defer func() { _ = rows.Close() }()

	type worklog struct {
		ID              int    `json:"id"`
		ProjectName     string `json:"project_name"`
		CustomerName    string `json:"customer_name,omitempty"`
		Description     string `json:"description"`
		Date            string `json:"date"`
		DurationMinutes int    `json:"duration_minutes"`
		ItemKey         string `json:"item_key,omitempty"`
	}
	var worklogs []worklog
	for rows.Next() {
		var wl worklog
		var dateUnix int64
		var itemID, itemNumber, wsID int
		var wsKey string
		if err := rows.Scan(&wl.ID, &wl.ProjectName, &wl.CustomerName, &wl.Description,
			&dateUnix, &wl.DurationMinutes, &itemID, &itemNumber, &wsKey, &wsID); err != nil {
			continue
		}
		wl.Date = time.Unix(dateUnix, 0).Format("2006-01-02")
		// Only include item key if user has workspace access
		if itemID > 0 && wsKey != "" && e.hasWorkspaceAccess(wsID) {
			wl.ItemKey = fmt.Sprintf("%s-%d", wsKey, itemNumber)
		}
		worklogs = append(worklogs, wl)
	}
	if worklogs == nil {
		worklogs = []worklog{}
	}

	b, _ := json.Marshal(map[string]interface{}{"worklogs": worklogs})
	return string(b), nil
}

// logTime creates a new time worklog entry.
func (e *ToolExecutor) logTime(arguments string) (string, error) {
	var args struct {
		ProjectID   int    `json:"project_id"`
		Description string `json:"description"`
		Date        string `json:"date"`
		Duration    string `json:"duration"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		ItemID      int    `json:"item_id"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}
	if args.ProjectID == 0 || args.Description == "" || args.Date == "" {
		return `{"error": "project_id, description, and date are required"}`, nil
	}

	// Check permission
	canBook, err := e.timePermService.CanBookTimeOnProject(e.userID, args.ProjectID)
	if err != nil {
		return `{"error": "failed to check booking permission"}`, nil
	}
	if !canBook {
		return `{"error": "you do not have permission to log time on this project"}`, nil
	}

	// Validate project exists and is active
	var projectName, projectStatus string
	var customerID sql.NullInt64
	err = e.db.QueryRow(
		"SELECT name, status, customer_id FROM time_projects WHERE id = ?", args.ProjectID,
	).Scan(&projectName, &projectStatus, &customerID)
	if err != nil {
		return `{"error": "project not found"}`, nil
	}
	if projectStatus != "Active" {
		return fmt.Sprintf(`{"error": "project %q is not active (status: %s)"}`, projectName, projectStatus), nil
	}
	if !customerID.Valid {
		return `{"error": "project has no customer assigned, cannot log time"}`, nil
	}

	// Parse date
	date, err := time.Parse("2006-01-02", args.Date)
	if err != nil {
		return `{"error": "invalid date format, use YYYY-MM-DD"}`, nil
	}

	var durationMins int
	var startTimeUnix, endTimeUnix int64

	switch {
	case args.Duration != "":
		// Parse duration string
		dur, err := ParseDuration(args.Duration)
		if err != nil {
			return fmt.Sprintf(`{"error": "invalid duration: %s"}`, err.Error()), nil
		}
		durationMins = int(dur.Minutes())
		// Set start/end to beginning of day with computed end
		startTimeUnix = date.Unix()
		endTimeUnix = date.Add(dur).Unix()
	case args.StartTime != "" && args.EndTime != "":
		// Parse HH:MM times
		startParts := strings.SplitN(args.StartTime, ":", 2)
		endParts := strings.SplitN(args.EndTime, ":", 2)
		if len(startParts) != 2 || len(endParts) != 2 {
			return `{"error": "start_time and end_time must be in HH:MM format"}`, nil
		}
		startH, err1 := strconv.Atoi(startParts[0])
		startM, err2 := strconv.Atoi(startParts[1])
		endH, err3 := strconv.Atoi(endParts[0])
		endM, err4 := strconv.Atoi(endParts[1])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			return `{"error": "start_time and end_time must be in HH:MM format"}`, nil
		}
		startT := date.Add(time.Duration(startH)*time.Hour + time.Duration(startM)*time.Minute)
		endT := date.Add(time.Duration(endH)*time.Hour + time.Duration(endM)*time.Minute)
		if !endT.After(startT) {
			return `{"error": "end_time must be after start_time"}`, nil
		}
		durationMins = int(endT.Sub(startT).Minutes())
		startTimeUnix = startT.Unix()
		endTimeUnix = endT.Unix()
	default:
		return `{"error": "provide either duration or both start_time and end_time"}`, nil
	}

	if durationMins <= 0 {
		return `{"error": "duration must be positive"}`, nil
	}

	// If item_id provided, verify it exists and user has workspace access
	var itemIDVal interface{} = nil
	if args.ItemID > 0 {
		wsID, err := repository.NewItemRepository(e.db).GetWorkspaceID(args.ItemID)
		if err != nil {
			return `{"error": "item not found"}`, nil
		}
		if !e.hasWorkspaceAccess(wsID) {
			return `{"error": "item not found"}`, nil
		}
		itemIDVal = args.ItemID
	}

	custID := customerID.Int64

	now := time.Now().Unix()
	dateUnix := date.Unix()

	var id int64
	err = e.db.QueryRow(`
		INSERT INTO time_worklogs (project_id, customer_id, user_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, args.ProjectID, custID, e.userID, itemIDVal, args.Description, dateUnix, startTimeUnix, endTimeUnix, durationMins, now, now).Scan(&id)
	if err != nil {
		return `{"error": "failed to create worklog"}`, nil
	}

	result := map[string]interface{}{
		"id":               id,
		"project_name":     projectName,
		"date":             args.Date,
		"duration_minutes": durationMins,
		"description":      args.Description,
		"message":          "Time logged successfully",
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

// listRecentActivity returns recent item changes and comments across accessible workspaces.
func (e *ToolExecutor) listRecentActivity(arguments string) (string, error) {
	var args struct {
		SinceDate   string `json:"since_date"`
		WorkspaceID int    `json:"workspace_id"`
		Limit       int    `json:"limit"`
	}
	if arguments != "" {
		_ = json.Unmarshal([]byte(arguments), &args)
	}

	// Default to yesterday
	sinceDate := args.SinceDate
	if sinceDate == "" {
		sinceDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", sinceDate); err != nil {
		return `{"error": "invalid since_date format, use YYYY-MM-DD"}`, nil
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	// Determine workspace scope
	var wsIDs []int
	if args.WorkspaceID > 0 {
		if !e.hasWorkspaceAccess(args.WorkspaceID) {
			return `{"error": "workspace not found"}`, nil
		}
		wsIDs = []int{args.WorkspaceID}
	} else {
		wsIDs = e.accessibleWorkspaceIDs
	}

	if len(wsIDs) == 0 {
		return `{"changes": [], "comments": []}`, nil
	}

	placeholders := make([]string, len(wsIDs))
	wsArgs := make([]interface{}, len(wsIDs))
	for i, id := range wsIDs {
		placeholders[i] = "?"
		wsArgs[i] = id
	}
	wsIn := strings.Join(placeholders, ",")

	// Query 1: Recent item_history changes
	changeQuery := fmt.Sprintf(`SELECT ih.field_name, COALESCE(ih.old_value, ''), COALESCE(ih.new_value, ''), ih.changed_at,
		w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
		COALESCE(u.first_name || ' ' || u.last_name, 'Unknown') as changed_by
		FROM item_history ih
		JOIN items i ON ih.item_id = i.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN users u ON ih.user_id = u.id
		WHERE i.workspace_id IN (%s) AND ih.changed_at >= ?
		ORDER BY ih.changed_at DESC LIMIT ?`, wsIn)

	changeArgs := append(append([]interface{}{}, wsArgs...), sinceDate, limit)
	rows, err := e.db.Query(changeQuery, changeArgs...)
	if err != nil {
		return `{"error": "failed to query recent changes"}`, nil
	}
	defer func() { _ = rows.Close() }()

	type change struct {
		FieldName string `json:"field"`
		OldValue  string `json:"old_value,omitempty"`
		NewValue  string `json:"new_value,omitempty"`
		ChangedAt string `json:"changed_at"`
		ItemKey   string `json:"item_key"`
		ItemTitle string `json:"item_title"`
		ChangedBy string `json:"changed_by"`
	}
	var changes []change
	for rows.Next() {
		var c change
		var changedAt time.Time
		if err := rows.Scan(&c.FieldName, &c.OldValue, &c.NewValue, &changedAt, &c.ItemKey, &c.ItemTitle, &c.ChangedBy); err != nil {
			continue
		}
		c.ChangedAt = changedAt.Format(time.RFC3339)
		changes = append(changes, c)
	}
	if changes == nil {
		changes = []change{}
	}

	// Query 2: Recent comments
	commentQuery := fmt.Sprintf(`SELECT c.content, c.created_at,
		w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
		COALESCE(u.first_name || ' ' || u.last_name, 'Unknown') as author
		FROM comments c
		JOIN items i ON c.item_id = i.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN users u ON c.author_id = u.id
		WHERE i.workspace_id IN (%s) AND c.created_at >= ? AND c.is_private = false
		ORDER BY c.created_at DESC LIMIT ?`, wsIn)

	commentArgs := append(append([]interface{}{}, wsArgs...), sinceDate, 30)
	cRows, err := e.db.Query(commentQuery, commentArgs...)
	if err != nil {
		return `{"error": "failed to query recent comments"}`, nil
	}
	defer func() { _ = cRows.Close() }()

	type comment struct {
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
		ItemKey   string `json:"item_key"`
		ItemTitle string `json:"item_title"`
		Author    string `json:"author"`
	}
	var comments []comment
	for cRows.Next() {
		var cm comment
		var createdAt time.Time
		if err := cRows.Scan(&cm.Content, &createdAt, &cm.ItemKey, &cm.ItemTitle, &cm.Author); err != nil {
			continue
		}
		cm.CreatedAt = createdAt.Format(time.RFC3339)
		if len(cm.Content) > 200 {
			cm.Content = cm.Content[:200] + "..."
		}
		comments = append(comments, cm)
	}
	if comments == nil {
		comments = []comment{}
	}

	b, _ := json.Marshal(map[string]interface{}{"changes": changes, "comments": comments})
	return string(b), nil
}

// updateItem updates an item's fields with name-based resolution support.
func (e *ToolExecutor) updateItem(arguments string) (string, error) {
	// Parse into raw map first to distinguish null from absent fields
	var rawArgs map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &rawArgs); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}

	var args struct {
		ItemID            int                    `json:"item_id"`
		ItemKey           string                 `json:"item_key"`
		Title             *string                `json:"title"`
		Description       *string                `json:"description"`
		PriorityID        *int                   `json:"priority_id"`
		PriorityName      *string                `json:"priority_name"`
		AssigneeID        *int                   `json:"assignee_id"`
		AssigneeName      *string                `json:"assignee_name"`
		DueDate           *string                `json:"due_date"`
		MilestoneName     *string                `json:"milestone_name"`
		IterationName     *string                `json:"iteration_name"`
		CustomFieldValues map[string]interface{} `json:"custom_field_values"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}

	// Helper: check if a raw field is explicitly null
	isExplicitNull := func(key string) bool {
		raw, ok := rawArgs[key]
		return ok && string(raw) == "null"
	}
	// Helper: check if a raw field is present (null or otherwise)
	hasField := func(key string) bool {
		_, ok := rawArgs[key]
		return ok
	}

	// Resolve item ID (same pattern as getItem)
	crudSvc := services.NewItemCRUDService(e.db)
	var itemID int

	switch {
	case args.ItemID > 0:
		itemID = args.ItemID
	case args.ItemKey != "":
		parts := strings.SplitN(strings.ToUpper(args.ItemKey), "-", 2)
		if len(parts) != 2 {
			return `{"error": "invalid item key format, expected KEY-NUMBER"}`, nil
		}
		num, err := strconv.Atoi(parts[1])
		if err != nil {
			return `{"error": "invalid item key format, expected KEY-NUMBER"}`, nil
		}
		itemID, err = repository.NewItemRepository(e.db).FindIDByKeyAndNumber(parts[0], num)
		if err != nil {
			return `{"error": "item not found"}`, nil
		}
	default:
		return `{"error": "must provide item_id or item_key"}`, nil
	}

	// Check workspace access
	wsID, err := crudSvc.GetWorkspaceID(itemID)
	if err != nil {
		return `{"error": "item not found"}`, nil
	}
	if !e.hasWorkspaceAccess(wsID) {
		return `{"error": "item not found"}`, nil
	}

	// Check edit permission
	canEdit, err := e.permService.HasWorkspacePermission(e.userID, wsID, models.PermissionItemEdit)
	if err != nil {
		return `{"error": "failed to check permissions"}`, nil
	}
	if !canEdit {
		return `{"error": "you do not have permission to edit items in this workspace"}`, nil
	}

	// Build update data map, resolving names to IDs where needed
	updateData := make(map[string]interface{})
	var changed []string

	if args.Title != nil {
		updateData["title"] = *args.Title
		changed = append(changed, "title")
	}
	if args.Description != nil {
		updateData["description"] = *args.Description
		changed = append(changed, "description")
	}

	// Status changes are handled by the separate transition_item tool so that
	// workflow + condition rules are always enforced.

	// Priority: prefer priority_id, fall back to priority_name
	if args.PriorityID != nil {
		updateData["priority_id"] = *args.PriorityID
		changed = append(changed, "priority")
	} else if args.PriorityName != nil {
		id, err := e.resolvePriorityName(*args.PriorityName)
		if err != nil {
			return fmt.Sprintf(`{"error": "could not resolve priority name %q: %s"}`, *args.PriorityName, err.Error()), nil
		}
		updateData["priority_id"] = id
		changed = append(changed, "priority")
	}

	// Assignee: prefer assignee_id, fall back to assignee_name; null clears
	switch {
	case isExplicitNull("assignee_id"):
		updateData["assignee_id"] = nil
		changed = append(changed, "assignee")
	case args.AssigneeID != nil:
		updateData["assignee_id"] = *args.AssigneeID
		changed = append(changed, "assignee")
	case args.AssigneeName != nil:
		id, err := e.resolveAssigneeName(*args.AssigneeName)
		if err != nil {
			return fmt.Sprintf(`{"error": "could not resolve assignee name %q: %s"}`, *args.AssigneeName, err.Error()), nil
		}
		updateData["assignee_id"] = id
		changed = append(changed, "assignee")
	}

	// Due date: null clears, string sets
	if hasField("due_date") {
		if isExplicitNull("due_date") {
			updateData["due_date"] = nil
			changed = append(changed, "due_date")
		} else if args.DueDate != nil {
			if _, err := time.Parse("2006-01-02", *args.DueDate); err != nil {
				return `{"error": "invalid due_date format, use YYYY-MM-DD"}`, nil
			}
			updateData["due_date"] = *args.DueDate
			changed = append(changed, "due_date")
		}
	}

	// Milestone: null clears, prefer milestone_id, fall back to milestone_name
	switch {
	case isExplicitNull("milestone_id"):
		updateData["milestone_id"] = nil
		changed = append(changed, "milestone")
	case hasField("milestone_id"):
		var mid int
		if err := json.Unmarshal(rawArgs["milestone_id"], &mid); err == nil {
			updateData["milestone_id"] = mid
			changed = append(changed, "milestone")
		}
	case args.MilestoneName != nil:
		id, err := e.resolveMilestoneName(*args.MilestoneName, wsID)
		if err != nil {
			return fmt.Sprintf(`{"error": "could not resolve milestone name %q: %s"}`, *args.MilestoneName, err.Error()), nil
		}
		updateData["milestone_id"] = id
		changed = append(changed, "milestone")
	}

	// Iteration: null clears, prefer iteration_id, fall back to iteration_name
	switch {
	case isExplicitNull("iteration_id"):
		updateData["iteration_id"] = nil
		changed = append(changed, "iteration")
	case hasField("iteration_id"):
		var iid int
		if err := json.Unmarshal(rawArgs["iteration_id"], &iid); err == nil {
			updateData["iteration_id"] = iid
			changed = append(changed, "iteration")
		}
	case args.IterationName != nil:
		id, err := e.resolveIterationName(*args.IterationName, wsID)
		if err != nil {
			return fmt.Sprintf(`{"error": "could not resolve iteration name %q: %s"}`, *args.IterationName, err.Error()), nil
		}
		updateData["iteration_id"] = id
		changed = append(changed, "iteration")
	}

	// Project: null clears
	if isExplicitNull("project_id") {
		updateData["project_id"] = nil
		changed = append(changed, "project")
	} else if hasField("project_id") {
		var pid int
		if err := json.Unmarshal(rawArgs["project_id"], &pid); err == nil {
			updateData["project_id"] = pid
			changed = append(changed, "project")
		}
	}

	// Parent: null clears
	if isExplicitNull("parent_id") {
		updateData["parent_id"] = nil
		changed = append(changed, "parent")
	} else if hasField("parent_id") {
		var pid int
		if err := json.Unmarshal(rawArgs["parent_id"], &pid); err == nil {
			updateData["parent_id"] = pid
			changed = append(changed, "parent")
		}
	}

	if args.CustomFieldValues != nil {
		updateData["custom_field_values"] = args.CustomFieldValues
		changed = append(changed, "custom_fields")
	}

	if len(updateData) == 0 {
		return `{"error": "no fields to update"}`, nil
	}

	// Perform the update
	updateSvc := services.NewItemUpdateService(e.db).WithPermissionService(e.permService)
	result, err := updateSvc.UpdateItem(services.UpdateItemRequest{
		ItemID:     itemID,
		UpdateData: updateData,
		UserID:     e.userID,
	})
	if err != nil {
		return fmt.Sprintf(`{"error": "update failed: %s"}`, err.Error()), nil
	}

	// Build response
	item := result.Item
	resp := map[string]interface{}{
		"message":        "Item updated successfully",
		"item_id":        item.ID,
		"key":            fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber),
		"title":          item.Title,
		"changed_fields": changed,
	}
	if item.StatusName != "" {
		resp["status"] = item.StatusName
	}
	if item.PriorityName != "" {
		resp["priority"] = item.PriorityName
	}
	if item.AssigneeName != "" {
		resp["assignee"] = item.AssigneeName
	}
	if item.MilestoneName != "" {
		resp["milestone"] = item.MilestoneName
	}

	b, _ := json.Marshal(resp)
	return string(b), nil
}

// transitionItem performs a workflow status transition on an item. Identifies
// the item by item_id or item_key, and the target status by to_status_id or
// to_status_name. Enforces both validator-mode and condition-mode workflow
// rules via WorkflowService.PerformTransition.
func (e *ToolExecutor) transitionItem(arguments string) (string, error) {
	var args struct {
		ItemID       int    `json:"item_id"`
		ItemKey      string `json:"item_key"`
		ToStatusID   *int   `json:"to_status_id"`
		ToStatusName string `json:"to_status_name"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}

	var itemID int
	switch {
	case args.ItemID > 0:
		itemID = args.ItemID
	case args.ItemKey != "":
		parts := strings.SplitN(strings.ToUpper(args.ItemKey), "-", 2)
		if len(parts) != 2 {
			return `{"error": "invalid item key format, expected KEY-NUMBER"}`, nil
		}
		num, err := strconv.Atoi(parts[1])
		if err != nil {
			return `{"error": "invalid item key format, expected KEY-NUMBER"}`, nil
		}
		itemID, err = repository.NewItemRepository(e.db).FindIDByKeyAndNumber(parts[0], num)
		if err != nil {
			return `{"error": "item not found"}`, nil
		}
	default:
		return `{"error": "must provide item_id or item_key"}`, nil
	}

	crudSvc := services.NewItemCRUDService(e.db)
	wsID, err := crudSvc.GetWorkspaceID(itemID)
	if err != nil {
		return `{"error": "item not found"}`, nil
	}
	if !e.hasWorkspaceAccess(wsID) {
		return `{"error": "item not found"}`, nil
	}

	canEdit, err := e.permService.HasWorkspacePermission(e.userID, wsID, models.PermissionItemEdit)
	if err != nil {
		return `{"error": "failed to check permissions"}`, nil
	}
	if !canEdit {
		return `{"error": "you do not have permission to edit items in this workspace"}`, nil
	}

	var toStatusID int
	switch {
	case args.ToStatusID != nil:
		toStatusID = *args.ToStatusID
	case args.ToStatusName != "":
		id, err := e.resolveStatusName(args.ToStatusName, wsID)
		if err != nil {
			return fmt.Sprintf(`{"error": "could not resolve status name %q: %s"}`, args.ToStatusName, err.Error()), nil
		}
		toStatusID = id
	default:
		return `{"error": "must provide to_status_id or to_status_name"}`, nil
	}

	workflowSvc := services.NewWorkflowService(e.db)
	scriptEngine := services.NewScriptEngine()
	conditionSvc := services.NewConditionService(e.db, e.permService, scriptEngine)
	approvalSvc := services.NewApprovalService(e.db, e.permService, repository.NewLeaveRepository(e.db), workflowSvc)

	result, err := workflowSvc.PerformTransition(context.Background(), services.PerformTransitionRequest{
		ItemID:      itemID,
		ToStatusID:  toStatusID,
		ActorUserID: e.userID,
		Modes:       []string{"validator", "condition"},
	}, repository.NewItemRepository(e.db), conditionSvc, approvalSvc)
	if err != nil {
		if rej := services.IsTransitionRejection(err); rej != nil {
			return fmt.Sprintf(`{"error": "transition rejected: %s"}`, rej.Message), nil
		}
		return fmt.Sprintf(`{"error": "transition failed: %s"}`, err.Error()), nil
	}

	resp := map[string]interface{}{
		"message":       "Transition completed",
		"item_id":       result.Item.ID,
		"key":           fmt.Sprintf("%s-%d", result.Item.WorkspaceKey, result.Item.WorkspaceItemNumber),
		"title":         result.Item.Title,
		"old_status_id": result.OldStatusID,
		"new_status_id": result.NewStatusID,
		"no_op":         result.NoOp,
	}
	if result.Item.StatusName != "" {
		resp["status"] = result.Item.StatusName
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

// resolveStatusName resolves a status name to its ID within a workspace.
func (e *ToolExecutor) resolveStatusName(name string, workspaceID int) (int, error) {
	var id int
	err := e.db.QueryRow(
		"SELECT id FROM statuses WHERE LOWER(name) = LOWER(?) AND workspace_id = ?",
		name, workspaceID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("status not found in this workspace")
	}
	return id, nil
}

// resolvePriorityName resolves a priority name to its ID.
func (e *ToolExecutor) resolvePriorityName(name string) (int, error) {
	var id int
	err := e.db.QueryRow(
		"SELECT id FROM priorities WHERE LOWER(name) = LOWER(?)",
		name,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("priority not found")
	}
	return id, nil
}

// resolveAssigneeName resolves a user's full name to their user ID.
func (e *ToolExecutor) resolveAssigneeName(name string) (int, error) {
	var id int
	err := e.db.QueryRow(
		"SELECT id FROM users WHERE LOWER(first_name || ' ' || last_name) = LOWER(?)",
		name,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("user not found")
	}
	return id, nil
}

// resolveMilestoneName resolves a milestone name to its ID (workspace-scoped or global).
func (e *ToolExecutor) resolveMilestoneName(name string, workspaceID int) (int, error) {
	var id int
	err := e.db.QueryRow(
		"SELECT id FROM milestones WHERE LOWER(name) = LOWER(?) AND (workspace_id = ? OR is_global = true) ORDER BY CASE WHEN workspace_id = ? THEN 0 ELSE 1 END LIMIT 1",
		name, workspaceID, workspaceID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("milestone not found")
	}
	return id, nil
}

// resolveIterationName resolves an iteration name to its ID (workspace-scoped or global).
func (e *ToolExecutor) resolveIterationName(name string, workspaceID int) (int, error) {
	var id int
	err := e.db.QueryRow(
		"SELECT id FROM iterations WHERE LOWER(name) = LOWER(?) AND (workspace_id = ? OR is_global = true) ORDER BY CASE WHEN workspace_id = ? THEN 0 ELSE 1 END LIMIT 1",
		name, workspaceID, workspaceID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("iteration not found")
	}
	return id, nil
}

// listComments returns comments on a work item.
func (e *ToolExecutor) listComments(arguments string) (string, error) {
	var args struct {
		ItemID int `json:"item_id"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}
	if args.ItemID <= 0 {
		return `{"error": "item_id is required"}`, nil
	}

	// Check item exists and user has workspace access
	crudSvc := services.NewItemCRUDService(e.db)
	item, err := crudSvc.GetByID(args.ItemID)
	if err != nil {
		return `{"error": "item not found"}`, nil
	}
	if !e.hasWorkspaceAccess(item.WorkspaceID) {
		return `{"error": "item not found"}`, nil
	}

	comments, err := e.commentService.GetByItemID(args.ItemID)
	if err != nil {
		return `{"error": "failed to list comments"}`, nil
	}

	result := make([]map[string]interface{}, len(comments))
	for i, c := range comments {
		m := map[string]interface{}{
			"id":         c.ID,
			"item_id":    c.ItemID,
			"content":    c.Content,
			"created_at": c.CreatedAt.Format(time.RFC3339),
		}
		if c.AuthorID != nil {
			m["author_id"] = *c.AuthorID
		}
		if c.AuthorName != "" {
			m["author_name"] = c.AuthorName
		}
		result[i] = m
	}

	b, _ := json.Marshal(map[string]interface{}{"comments": result})
	return string(b), nil
}

// addComment adds a comment to a work item.
func (e *ToolExecutor) addComment(arguments string) (string, error) {
	var args struct {
		ItemID  int    `json:"item_id"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"error": "invalid arguments"}`, nil
	}
	if args.ItemID <= 0 {
		return `{"error": "item_id is required"}`, nil
	}
	if strings.TrimSpace(args.Content) == "" {
		return `{"error": "content is required"}`, nil
	}

	// Check item exists and user has workspace access
	crudSvc := services.NewItemCRUDService(e.db)
	item, err := crudSvc.GetByID(args.ItemID)
	if err != nil {
		return `{"error": "item not found"}`, nil
	}
	if !e.hasWorkspaceAccess(item.WorkspaceID) {
		return `{"error": "item not found"}`, nil
	}

	result, err := e.commentService.Create(services.CreateCommentParams{
		ItemID:      args.ItemID,
		AuthorID:    e.userID,
		Content:     args.Content,
		ActorUserID: e.userID,
	})
	if err != nil {
		return `{"error": "failed to add comment"}`, nil
	}

	b, _ := json.Marshal(map[string]interface{}{
		"id":      result.CommentID,
		"item_id": args.ItemID,
		"content": args.Content,
		"message": "Comment added successfully",
	})
	return string(b), nil
}
