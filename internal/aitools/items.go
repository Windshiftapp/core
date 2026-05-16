package aitools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// itemSummaryDTO is the trimmed shape used in list responses.
type itemSummaryDTO struct {
	ID               int      `json:"id"`
	Key              string   `json:"key,omitempty"`
	Title            string   `json:"title"`
	Status           string   `json:"status,omitempty"`
	StatusID         *int     `json:"status_id,omitempty"`
	Priority         string   `json:"priority,omitempty"`
	PriorityID       *int     `json:"priority_id,omitempty"`
	Assignee         string   `json:"assignee,omitempty"`
	AssigneeID       *int     `json:"assignee_id,omitempty"`
	DueDate          string   `json:"due_date,omitempty"`
	Type             string   `json:"type,omitempty"`
	Milestones       []string `json:"milestones,omitempty"`
	IterationName    string   `json:"iteration_name,omitempty"`
	IterationEndDate string   `json:"iteration_end_date,omitempty"`
	WorkspaceID      int      `json:"workspace_id"`
	Labels           []string `json:"labels,omitempty"`
}

// itemDetailDTO is the richer shape for get_item.
type itemDetailDTO struct {
	itemSummaryDTO
	Description string `json:"description,omitempty"`
	Creator     string `json:"creator,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	ParentID    *int   `json:"parent_id,omitempty"`
}

func itemToSummary(item *models.Item) itemSummaryDTO {
	s := itemSummaryDTO{
		ID:               item.ID,
		Title:            item.Title,
		Status:           item.StatusName,
		StatusID:         item.StatusID,
		Priority:         item.PriorityName,
		PriorityID:       item.PriorityID,
		Assignee:         item.AssigneeName,
		AssigneeID:       item.AssigneeID,
		Type:             item.ItemTypeName,
		IterationName:    item.IterationName,
		IterationEndDate: item.IterationEndDate,
		WorkspaceID:      item.WorkspaceID,
	}
	if len(item.Milestones) > 0 {
		names := make([]string, 0, len(item.Milestones))
		for _, m := range item.Milestones {
			if m.TargetDate != nil && *m.TargetDate != "" {
				names = append(names, fmt.Sprintf("%s (target: %s)", m.Name, *m.TargetDate))
			} else {
				names = append(names, m.Name)
			}
		}
		s.Milestones = names
	}
	if item.WorkspaceKey != "" {
		s.Key = fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
	}
	if item.DueDate != nil {
		s.DueDate = item.DueDate.Format("2006-01-02")
	}
	for _, l := range item.Labels {
		s.Labels = append(s.Labels, l.Name)
	}
	return s
}

// ----------------------------------------------------------------------------
// list_items
// ----------------------------------------------------------------------------

type listItemsArgs struct {
	WorkspaceID *int   `json:"workspace_id,omitempty" jsonschema:"Workspace ID. If omitted, queries all accessible workspaces."`
	StatusID    *int   `json:"status_id,omitempty" jsonschema:"Filter by status ID"`
	Status      string `json:"status,omitempty" jsonschema:"Filter by status name"`
	AssigneeID  *int   `json:"assignee_id,omitempty" jsonschema:"Filter by assignee user ID"`
	ParentID    *int   `json:"parent_id,omitempty" jsonschema:"Filter by parent item ID (0 for root items only)"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Max items to return (default 20, max 200)"`
	Offset      int    `json:"offset,omitempty" jsonschema:"Offset for pagination"`
	Filter      string `json:"filter,omitempty" jsonschema:"CQL filter expression. Supported fields: status, priority, assignee, creator, due_date, label, milestone, iteration, project, itemtype, cf_<name>. Operators: =, !=, <, <=, >, >=, ~ (contains), IN, NOT IN. Logical: AND, OR, NOT. Functions: currentUser(), now(), startOfDay(), endOfDay()."`
}

type listItemsOut struct {
	Items []itemSummaryDTO `json:"items"`
	Total int              `json:"total"`
}

func init() {
	Register(Default, Tool[listItemsArgs]{
		Name:        "list_items",
		Description: "List work items in one or all accessible workspaces, with optional filters and CQL.",
		Run: func(_ context.Context, env *Env, args listItemsArgs) (any, error) {
			var wsIDs []int
			if args.WorkspaceID != nil && *args.WorkspaceID > 0 {
				if !env.HasWorkspaceAccess(*args.WorkspaceID) {
					return map[string]string{"error": "workspace not found"}, nil
				}
				wsIDs = []int{*args.WorkspaceID}
			} else {
				wsIDs = env.AccessibleWorkspaceIDs
			}
			if len(wsIDs) == 0 {
				return listItemsOut{Items: []itemSummaryDTO{}, Total: 0}, nil
			}

			limit := args.Limit
			if limit <= 0 {
				limit = 20
			}
			if limit > 200 {
				limit = 200
			}

			filters := services.ItemFilters{}
			if args.StatusID != nil {
				filters.StatusID = args.StatusID
			}
			if args.AssigneeID != nil {
				filters.AssigneeID = args.AssigneeID
			}
			if args.ParentID != nil {
				filters.ParentID = args.ParentID
				filters.ParentIDIsSet = true
			}

			var qlParts []string
			var qlArgs []interface{}
			if args.Status != "" {
				qlParts = append(qlParts, "st.name = ?")
				qlArgs = append(qlArgs, args.Status)
			}
			if args.Filter != "" {
				wsMap := workspaceLookupMap(env.DB)
				evaluator := cql.NewEvaluator(wsMap, nil, env.DB.GetDriverName())
				resolved := cql.SubstituteFunctions(args.Filter, cql.UserContext(env.UserID))
				cqlSQL, cqlArgs, err := evaluator.EvaluateToSQL(resolved)
				if err != nil {
					return map[string]string{"error": fmt.Sprintf("invalid filter expression: %s", err.Error())}, nil
				}
				if cqlSQL != "" {
					qlParts = append(qlParts, cqlSQL)
					qlArgs = append(qlArgs, cqlArgs...)
				}
			}
			if len(qlParts) > 0 {
				filters.QLQuery = strings.Join(qlParts, " AND ")
				filters.QLArgs = qlArgs
			}

			items, total, err := services.NewItemCRUDService(env.DB).List(services.ItemListParams{
				WorkspaceIDs: wsIDs,
				Filters:      filters,
				SortBy:       "created_at",
				SortAsc:      false,
				Pagination:   services.PaginationParams{Limit: limit, Offset: args.Offset},
			})
			if err != nil {
				return nil, err
			}

			out := listItemsOut{Items: make([]itemSummaryDTO, 0, len(items)), Total: total}
			for _, item := range items {
				out.Items = append(out.Items, itemToSummary(&item))
			}
			return out, nil
		},
	})

	// ------------------------------------------------------------------------
	// get_item
	// ------------------------------------------------------------------------
	Register(Default, Tool[getItemArgs]{
		Name:        "get_item",
		Description: "Get details of a single work item by numeric ID or key (e.g. PROJ-42).",
		Run: func(_ context.Context, env *Env, args getItemArgs) (any, error) {
			itemID, err := resolveItemID(env.DB, args.ItemID, args.ItemKey)
			if err != nil {
				return map[string]string{"error": err.Error()}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			crudSvc := services.NewItemCRUDService(env.DB)
			wsID, err := crudSvc.GetWorkspaceID(itemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if !env.HasWorkspaceAccess(wsID) {
				return map[string]string{"error": "item not found"}, nil
			}
			item, err := crudSvc.GetByID(itemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			d := itemDetailDTO{
				itemSummaryDTO: itemToSummary(item),
				Creator:        item.CreatorName,
				Workspace:      item.WorkspaceName,
				ParentID:       item.ParentID,
			}
			if item.Description != "" {
				desc := item.Description
				if len(desc) > 500 {
					desc = desc[:500] + "..."
				}
				d.Description = desc
			}
			return d, nil
		},
	})

	// ------------------------------------------------------------------------
	// search_items
	// ------------------------------------------------------------------------
	Register(Default, Tool[searchItemsArgs]{
		Name:        "search_items",
		Description: "Full-text search for work items by title or description across accessible workspaces.",
		Run: func(_ context.Context, env *Env, args searchItemsArgs) (any, error) {
			if strings.TrimSpace(args.Query) == "" {
				return map[string]string{"error": "query is required"}, nil
			}
			searchWS := env.AccessibleWorkspaceIDs
			if len(args.WorkspaceIDs) > 0 {
				searchWS = nil
				for _, id := range args.WorkspaceIDs {
					if env.HasWorkspaceAccess(id) {
						searchWS = append(searchWS, id)
					}
				}
			}
			if args.WorkspaceID != nil && *args.WorkspaceID > 0 {
				if !env.HasWorkspaceAccess(*args.WorkspaceID) {
					return listItemsOut{Items: []itemSummaryDTO{}, Total: 0}, nil
				}
				searchWS = []int{*args.WorkspaceID}
			}
			if len(searchWS) == 0 {
				return listItemsOut{Items: []itemSummaryDTO{}, Total: 0}, nil
			}
			limit := args.Limit
			if limit <= 0 || limit > 100 {
				limit = 20
			}
			items, total, err := services.NewItemCRUDService(env.DB).Search(args.Query, searchWS, services.PaginationParams{Limit: limit})
			if err != nil {
				return nil, err
			}
			out := listItemsOut{Items: make([]itemSummaryDTO, 0, len(items)), Total: total}
			for _, item := range items {
				if !env.HasWorkspaceAccess(item.WorkspaceID) {
					continue
				}
				out.Items = append(out.Items, itemToSummary(&item))
			}
			return out, nil
		},
	})

	// ------------------------------------------------------------------------
	// create_item
	// ------------------------------------------------------------------------
	Register(Default, Tool[createItemArgs]{
		Name:        "create_item",
		Description: "Create a new work item in a workspace.",
		Run: func(_ context.Context, env *Env, args createItemArgs) (any, error) {
			if strings.TrimSpace(args.Title) == "" {
				return map[string]string{"error": "title is required"}, nil
			}
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, args.WorkspaceID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			title := utils.StripHTMLTags(args.Title)
			desc := utils.SanitizeCommentContent(args.Description)
			itemID, err := services.CreateItem(env.DB, services.ItemCreationParams{
				WorkspaceID: args.WorkspaceID,
				Title:       title,
				Description: desc,
				StatusID:    args.StatusID,
				PriorityID:  args.PriorityID,
				AssigneeID:  args.AssigneeID,
				ParentID:    args.ParentID,
				ItemTypeID:  args.ItemTypeID,
				CreatorID:   &env.UserID,
			})
			if err != nil {
				return nil, err
			}
			created, err := services.NewItemCRUDService(env.DB).GetByID(int(itemID))
			if err != nil {
				return map[string]any{"id": itemID}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			return itemToSummary(created), nil
		},
	})

	// ------------------------------------------------------------------------
	// update_item
	// ------------------------------------------------------------------------
	Register(Default, Tool[updateItemArgs]{
		Name:        "update_item",
		Description: "Update fields on an existing work item. Identifies the item by numeric ID or key. Use transition_item to change status (workflow + condition rules apply).",
		Run: func(_ context.Context, env *Env, args updateItemArgs) (any, error) {
			itemID, err := resolveItemID(env.DB, args.ItemID, args.ItemKey)
			if err != nil {
				return map[string]string{"error": err.Error()}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			crudSvc := services.NewItemCRUDService(env.DB)
			wsID, err := crudSvc.GetWorkspaceID(itemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if !env.HasWorkspaceAccess(wsID) {
				return map[string]string{"error": "item not found"}, nil
			}
			canEdit, err := env.PermService.HasWorkspacePermission(env.UserID, wsID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !canEdit {
				return map[string]string{"error": "permission denied"}, nil
			}
			updateData, changed, herr := buildUpdateData(env, args, wsID)
			if herr != nil {
				return map[string]string{"error": herr.Error()}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if len(updateData) == 0 {
				return map[string]string{"error": "no fields to update"}, nil
			}
			result, err := services.NewItemUpdateService(env.DB).
				WithPermissionService(env.PermService).
				UpdateItem(services.UpdateItemRequest{
					ItemID:     itemID,
					UpdateData: updateData,
					UserID:     env.UserID,
				})
			if err != nil {
				return map[string]string{"error": fmt.Sprintf("update failed: %s", err.Error())}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			out := map[string]any{
				"item":           itemToSummary(result.Item),
				"changed_fields": changed,
			}
			return out, nil
		},
	})

	// ------------------------------------------------------------------------
	// delete_item
	// ------------------------------------------------------------------------
	Register(Default, Tool[deleteItemArgs]{
		Name:        "delete_item",
		Description: "Delete a work item and all its descendants.",
		Run: func(_ context.Context, env *Env, args deleteItemArgs) (any, error) {
			crudSvc := services.NewItemCRUDService(env.DB)
			item, err := crudSvc.GetByID(args.ItemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if !env.HasWorkspaceAccess(item.WorkspaceID) {
				return map[string]string{"error": "item not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, item.WorkspaceID, models.PermissionItemDelete)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			result, err := crudSvc.Delete(args.ItemID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"deleted": true, "deleted_count": result.DeletedCount}, nil
		},
	})

	// ------------------------------------------------------------------------
	// get_item_children
	// ------------------------------------------------------------------------
	Register(Default, Tool[getItemChildrenArgs]{
		Name:        "get_item_children",
		Description: "Get the direct children of a work item.",
		Run: func(_ context.Context, env *Env, args getItemChildrenArgs) (any, error) {
			crudSvc := services.NewItemCRUDService(env.DB)
			item, err := crudSvc.GetByID(args.ItemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if !env.HasWorkspaceAccess(item.WorkspaceID) {
				return map[string]string{"error": "item not found"}, nil
			}
			children, err := crudSvc.GetChildren(args.ItemID)
			if err != nil {
				return nil, err
			}
			out := make([]itemSummaryDTO, 0, len(children))
			for _, c := range children {
				out = append(out, itemToSummary(c))
			}
			return map[string]any{"children": out}, nil
		},
	})

	// ------------------------------------------------------------------------
	// transition_item
	// ------------------------------------------------------------------------
	Register(Default, Tool[transitionItemArgs]{
		Name:        "transition_item",
		Description: "Perform a workflow status transition on an item. Identifies the item by ID or key, and the target status by ID or name. Workflow + condition rules are enforced.",
		Run: func(ctx context.Context, env *Env, args transitionItemArgs) (any, error) {
			itemID, err := resolveItemID(env.DB, args.ItemID, args.ItemKey)
			if err != nil {
				return map[string]string{"error": err.Error()}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			crudSvc := services.NewItemCRUDService(env.DB)
			wsID, err := crudSvc.GetWorkspaceID(itemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if !env.HasWorkspaceAccess(wsID) {
				return map[string]string{"error": "item not found"}, nil
			}
			canEdit, err := env.PermService.HasWorkspacePermission(env.UserID, wsID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !canEdit {
				return map[string]string{"error": "permission denied"}, nil
			}
			var toStatusID int
			switch {
			case args.ToStatusID != nil:
				toStatusID = *args.ToStatusID
			case args.ToStatusName != "":
				id, err := resolveStatusName(env.DB, args.ToStatusName, wsID)
				if err != nil {
					return map[string]string{"error": fmt.Sprintf("could not resolve status name %q", args.ToStatusName)}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
				}
				toStatusID = id
			default:
				return map[string]string{"error": "must provide to_status_id or to_status_name"}, nil
			}
			workflowSvc := services.NewWorkflowService(env.DB)
			conditionSvc := services.NewConditionService(env.DB, env.PermService, services.NewScriptEngine())
			approvalSvc := services.NewApprovalService(env.DB, env.PermService, repository.NewLeaveRepository(env.DB), workflowSvc)
			result, err := workflowSvc.PerformTransition(ctx, services.PerformTransitionRequest{
				ItemID:      itemID,
				ToStatusID:  toStatusID,
				ActorUserID: env.UserID,
				Modes:       []string{"validator", "condition"},
			}, repository.NewItemRepository(env.DB), conditionSvc, approvalSvc)
			if err != nil {
				if rej := services.IsTransitionRejection(err); rej != nil {
					return map[string]string{"error": fmt.Sprintf("transition rejected: %s", rej.Message)}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
				}
				return map[string]string{"error": fmt.Sprintf("transition failed: %s", err.Error())}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			out := map[string]any{
				"item":          itemToSummary(result.Item),
				"old_status_id": result.OldStatusID,
				"new_status_id": result.NewStatusID,
				"no_op":         result.NoOp,
			}
			return out, nil
		},
	})
}

// ----------------------------------------------------------------------------
// Args types
// ----------------------------------------------------------------------------

type getItemArgs struct {
	ItemID  int    `json:"item_id,omitempty" jsonschema:"Item ID (numeric)"`
	ItemKey string `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42"`
}

type searchItemsArgs struct {
	Query        string `json:"query" jsonschema:"Search query (full-text on title/description)"`
	WorkspaceID  *int   `json:"workspace_id,omitempty" jsonschema:"Limit search to a specific workspace"`
	WorkspaceIDs []int  `json:"workspace_ids,omitempty" jsonschema:"Limit to a list of workspaces"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Max results (default 20)"`
}

type createItemArgs struct {
	WorkspaceID int    `json:"workspace_id" jsonschema:"Workspace to create item in"`
	Title       string `json:"title" jsonschema:"Item title"`
	Description string `json:"description,omitempty" jsonschema:"Item description (TipTap JSON or plain text)"`
	StatusID    *int   `json:"status_id,omitempty" jsonschema:"Status ID (uses workflow default if omitted)"`
	PriorityID  *int   `json:"priority_id,omitempty" jsonschema:"Priority ID"`
	AssigneeID  *int   `json:"assignee_id,omitempty" jsonschema:"Assignee user ID"`
	ParentID    *int   `json:"parent_id,omitempty" jsonschema:"Parent item ID for sub-items"`
	ItemTypeID  *int   `json:"item_type_id,omitempty" jsonschema:"Item type ID (uses workspace default if omitted)"`
}

type updateItemArgs struct {
	ItemID            int                    `json:"item_id,omitempty" jsonschema:"Item ID"`
	ItemKey           string                 `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42"`
	Title             *string                `json:"title,omitempty" jsonschema:"New title"`
	Description       *string                `json:"description,omitempty" jsonschema:"New description"`
	PriorityID        *int                   `json:"priority_id,omitempty" jsonschema:"New priority ID"`
	PriorityName      *string                `json:"priority_name,omitempty" jsonschema:"New priority name (alternative to ID)"`
	AssigneeID        *int                   `json:"assignee_id,omitempty" jsonschema:"New assignee user ID (0 to unassign)"`
	AssigneeName      *string                `json:"assignee_name,omitempty" jsonschema:"New assignee full name (alternative to ID)"`
	DueDate           *string                `json:"due_date,omitempty" jsonschema:"Due date YYYY-MM-DD (empty string to clear)"`
	MilestoneID       *int                   `json:"milestone_id,omitempty" jsonschema:"Milestone ID"`
	MilestoneName     *string                `json:"milestone_name,omitempty" jsonschema:"Milestone name (alternative to ID)"`
	IterationID       *int                   `json:"iteration_id,omitempty" jsonschema:"Iteration ID"`
	IterationName     *string                `json:"iteration_name,omitempty" jsonschema:"Iteration name (alternative to ID)"`
	ProjectID         *int                   `json:"project_id,omitempty" jsonschema:"Project ID"`
	ParentID          *int                   `json:"parent_id,omitempty" jsonschema:"Parent item ID"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty" jsonschema:"Custom field values map"`
}

type deleteItemArgs struct {
	ItemID int `json:"item_id" jsonschema:"Item ID to delete (also deletes descendants)"`
}

type getItemChildrenArgs struct {
	ItemID int `json:"item_id" jsonschema:"Parent item ID"`
}

type transitionItemArgs struct {
	ItemID       int    `json:"item_id,omitempty" jsonschema:"Item ID"`
	ItemKey      string `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42"`
	ToStatusID   *int   `json:"to_status_id,omitempty" jsonschema:"Target status ID"`
	ToStatusName string `json:"to_status_name,omitempty" jsonschema:"Target status name (alternative to ID)"`
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// buildUpdateData translates updateItemArgs into the map expected by
// services.UpdateItemRequest, resolving names to IDs where applicable.
// Returns the update map, the list of changed field names, and any
// resolution error.
func buildUpdateData(env *Env, args updateItemArgs, wsID int) (data map[string]interface{}, changed []string, err error) {
	data = map[string]interface{}{}
	out := data

	if args.Title != nil {
		out["title"] = *args.Title
		changed = append(changed, "title")
	}
	if args.Description != nil {
		out["description"] = *args.Description
		changed = append(changed, "description")
	}
	switch {
	case args.PriorityID != nil:
		out["priority_id"] = *args.PriorityID
		changed = append(changed, "priority")
	case args.PriorityName != nil:
		id, err := resolvePriorityName(env.DB, *args.PriorityName)
		if err != nil {
			return nil, nil, fmt.Errorf("could not resolve priority name %q: %w", *args.PriorityName, err)
		}
		out["priority_id"] = id
		changed = append(changed, "priority")
	}
	switch {
	case args.AssigneeID != nil:
		if *args.AssigneeID == 0 {
			out["assignee_id"] = nil
		} else {
			out["assignee_id"] = *args.AssigneeID
		}
		changed = append(changed, "assignee")
	case args.AssigneeName != nil:
		id, err := resolveAssigneeName(env.DB, *args.AssigneeName)
		if err != nil {
			return nil, nil, fmt.Errorf("could not resolve assignee name %q: %w", *args.AssigneeName, err)
		}
		out["assignee_id"] = id
		changed = append(changed, "assignee")
	}
	if args.DueDate != nil {
		if *args.DueDate == "" {
			out["due_date"] = nil
		} else {
			if _, err := time.Parse("2006-01-02", *args.DueDate); err != nil {
				return nil, nil, fmt.Errorf("invalid due_date format, use YYYY-MM-DD")
			}
			out["due_date"] = *args.DueDate
		}
		changed = append(changed, "due_date")
	}
	switch {
	case args.MilestoneID != nil:
		if *args.MilestoneID == 0 {
			out["milestone_id"] = nil
		} else {
			out["milestone_id"] = *args.MilestoneID
		}
		changed = append(changed, "milestone")
	case args.MilestoneName != nil:
		id, err := resolveMilestoneName(env.DB, *args.MilestoneName, wsID)
		if err != nil {
			return nil, nil, fmt.Errorf("could not resolve milestone name %q: %w", *args.MilestoneName, err)
		}
		out["milestone_id"] = id
		changed = append(changed, "milestone")
	}
	switch {
	case args.IterationID != nil:
		if *args.IterationID == 0 {
			out["iteration_id"] = nil
		} else {
			out["iteration_id"] = *args.IterationID
		}
		changed = append(changed, "iteration")
	case args.IterationName != nil:
		id, err := resolveIterationName(env.DB, *args.IterationName, wsID)
		if err != nil {
			return nil, nil, fmt.Errorf("could not resolve iteration name %q: %w", *args.IterationName, err)
		}
		out["iteration_id"] = id
		changed = append(changed, "iteration")
	}
	if args.ProjectID != nil {
		if *args.ProjectID == 0 {
			out["project_id"] = nil
		} else {
			out["project_id"] = *args.ProjectID
		}
		changed = append(changed, "project")
	}
	if args.ParentID != nil {
		if *args.ParentID == 0 {
			out["parent_id"] = nil
		} else {
			out["parent_id"] = *args.ParentID
		}
		changed = append(changed, "parent")
	}
	if args.CustomFieldValues != nil {
		out["custom_field_values"] = args.CustomFieldValues
		changed = append(changed, "custom_fields")
	}
	return out, changed, nil
}

func workspaceLookupMap(db database.Database) map[string]int {
	out := map[string]int{}
	rows, err := db.Query("SELECT id, name, key FROM workspaces")
	if err != nil {
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var name, key string
		if err := rows.Scan(&id, &name, &key); err != nil {
			continue
		}
		out[fmt.Sprintf("%d", id)] = id
		out[strings.ToLower(name)] = id
		out[strings.ToLower(key)] = id
	}
	if err := rows.Err(); err != nil {
		return out
	}
	return out
}

func resolveStatusName(db database.Database, name string, workspaceID int) (int, error) {
	var id int
	err := db.QueryRow("SELECT id FROM statuses WHERE LOWER(name) = LOWER(?) AND workspace_id = ?", name, workspaceID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("status not found in this workspace")
	}
	return id, nil
}

func resolvePriorityName(db database.Database, name string) (int, error) {
	var id int
	err := db.QueryRow("SELECT id FROM priorities WHERE LOWER(name) = LOWER(?)", name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("priority not found")
	}
	return id, nil
}

func resolveAssigneeName(db database.Database, name string) (int, error) {
	var id int
	err := db.QueryRow("SELECT id FROM users WHERE LOWER(first_name || ' ' || last_name) = LOWER(?)", name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("user not found")
	}
	return id, nil
}

func resolveMilestoneName(db database.Database, name string, workspaceID int) (int, error) {
	var id int
	err := db.QueryRow(
		"SELECT id FROM milestones WHERE LOWER(name) = LOWER(?) AND (workspace_id = ? OR is_global = true) ORDER BY CASE WHEN workspace_id = ? THEN 0 ELSE 1 END LIMIT 1",
		name, workspaceID, workspaceID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("milestone not found")
	}
	return id, nil
}

func resolveIterationName(db database.Database, name string, workspaceID int) (int, error) {
	var id int
	err := db.QueryRow(
		"SELECT id FROM iterations WHERE LOWER(name) = LOWER(?) AND (workspace_id = ? OR is_global = true) ORDER BY CASE WHEN workspace_id = ? THEN 0 ELSE 1 END LIMIT 1",
		name, workspaceID, workspaceID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("iteration not found")
	}
	return id, nil
}
