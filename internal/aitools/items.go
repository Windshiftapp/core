package aitools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"windshift/internal/auth"
	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
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
		Group:       CapabilityReadComment,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List work items in one or all accessible workspaces. Filter by status, milestone, iteration, assignee, priority, labels, and more with CQL.",
		Scopes:      []string{auth.ScopeItemsRead},
		Run: func(ctx context.Context, env *Env, args listItemsArgs) (any, error) {
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
			var qlArgs []any
			if args.Status != "" {
				qlParts = append(qlParts, "st.name = ?")
				qlArgs = append(qlArgs, args.Status)
			}
			if args.Filter != "" {
				wsMap := workspaceLookupMap(env.DB)
				customFieldMap, cfErr := repository.NewItemRepository(env.DB).GetCQLCustomFieldMap()
				if cfErr != nil {
					return map[string]string{"error": fmt.Sprintf("failed to build custom field map: %s", cfErr.Error())}, nil
				}
				evaluator := cql.NewEvaluator(wsMap, customFieldMap, env.DB.GetDriverName())
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
			if err := repository.NewMilestoneAttachRepository(env.DB).LoadForItemsContext(ctx, items); err != nil {
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
		Group:       CapabilityReadComment,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "Get details of a single work item by numeric ID or key (e.g. PROJ-42). Long descriptions are truncated to 500 characters with an explicit marker unless full_description=true.",
		Scopes:      []string{auth.ScopeItemsRead},
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
				if !args.FullDescription && len(desc) > 500 {
					// Cut on a rune boundary so the truncated text stays valid UTF-8.
					cut := 500
					for cut > 0 && !utf8.RuneStart(desc[cut]) {
						cut--
					}
					desc = fmt.Sprintf("%s... [truncated, %d chars total — pass full_description=true for the full text]",
						desc[:cut], utf8.RuneCountInString(item.Description))
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
		Group:       CapabilityReadComment,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "Full-text search for work items by title or description across accessible workspaces.",
		Scopes:      []string{auth.ScopeItemsRead},
		Run: func(ctx context.Context, env *Env, args searchItemsArgs) (any, error) {
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
			if err := repository.NewMilestoneAttachRepository(env.DB).LoadForItemsContext(ctx, items); err != nil {
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
		Group:       CapabilityIssueManagement,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Create a new work item in a workspace.",
		Scopes:      []string{auth.ScopeItemsWrite},
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
			title := sanitize.PlainTextField.Sanitize(args.Title)
			desc := sanitize.Comment.Sanitize(args.Description)
			// Centralized creation validation (parent hierarchy and cross-workspace
			// parent visibility) mirrors the cookie-auth and v1
			// create paths. Permission-shaped parent failures surface as "Parent
			// item not found" so existence isn't leaked.
			validationResult := services.ValidateItemCreation(env.DB, services.ItemValidationParams{
				WorkspaceID: args.WorkspaceID,
				Title:       title,
				ItemTypeID:  args.ItemTypeID,
				ParentID:    args.ParentID,
				UserID:      env.UserID,
				PermService: env.PermService,
			})
			if !validationResult.Valid {
				return map[string]string{"error": validationResult.Error}, nil
			}
			itemID, err := services.CreateItem(env.DB, services.ItemCreationParams{
				WorkspaceID:      args.WorkspaceID,
				Title:            title,
				Description:      desc,
				StatusID:         args.StatusID,
				PriorityID:       args.PriorityID,
				AssigneeID:       args.AssigneeID,
				ParentID:         args.ParentID,
				ItemTypeID:       args.ItemTypeID,
				CreatorID:        &env.UserID,
				ValidatingUserID: env.UserID,
				PermService:      env.PermService,
				EventMetadata:    itemevents.Agent(fmt.Sprintf("user:%d", env.UserID), "agent"),
			})
			if err != nil {
				return map[string]string{"error": fmt.Sprintf("create failed: %s", err.Error())}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			env.AuditWrite(logger.ResourceItem, int(itemID), "create_item", title)
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
		Group:       CapabilityIssueManagement,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Update fields on an existing work item. Identifies the item by numeric ID or key. Use transition_item to change status (workflow + condition rules apply).",
		Scopes:      []string{auth.ScopeItemsWrite},
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
			env.AuditWrite(logger.ResourceItem, itemID, "update_item", result.Item.Title)
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
		Group:       CapabilityIssueManagement,
		Access:      AccessDestructive,
		Risk:        RiskHigh,
		Description: "Delete a work item and all its descendants. Identifies the item by numeric ID or key (e.g. PROJ-42).",
		Scopes:      []string{auth.ScopeItemsDelete},
		Run: func(_ context.Context, env *Env, args deleteItemArgs) (any, error) {
			itemID, err := resolveItemID(env.DB, args.ItemID, args.ItemKey)
			if err != nil {
				return map[string]string{"error": err.Error()}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			deletionService := env.ItemDeletionService
			if deletionService == nil {
				deletionService = services.NewItemDeletionApplicationService(env.DB, env.PermService)
			}
			result, err := deletionService.Delete(services.ItemDeletionRequest{
				ItemID:        itemID,
				ActorUserID:   env.UserID,
				ActorUsername: env.Username,
				Mode:          services.ItemDeletionCascade,
				CanAccessWorkspace: func(workspaceID int) (bool, error) {
					return env.HasWorkspaceAccess(workspaceID), nil
				},
			})
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return map[string]string{"error": "item not found"}, nil
				}
				if errors.Is(err, services.ErrItemDeletionForbidden) {
					return map[string]string{"error": "permission denied"}, nil
				}
				return nil, err
			}
			env.AuditWrite(logger.ResourceItem, itemID, "delete_item", result.Item.Title)
			return map[string]any{"deleted": true, "deleted_count": result.DeletedCount}, nil
		},
	})

	// ------------------------------------------------------------------------
	// get_item_children
	// ------------------------------------------------------------------------
	Register(Default, Tool[getItemChildrenArgs]{
		Name:        "get_item_children",
		Group:       CapabilityReadComment,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "Get the direct children of a work item. Identifies the parent by numeric ID or key (e.g. PROJ-42).",
		Scopes:      []string{auth.ScopeItemsRead},
		Run: func(_ context.Context, env *Env, args getItemChildrenArgs) (any, error) {
			itemID, err := resolveItemID(env.DB, args.ItemID, args.ItemKey)
			if err != nil {
				return map[string]string{"error": err.Error()}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			crudSvc := services.NewItemCRUDService(env.DB)
			item, err := crudSvc.GetByID(itemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if !env.HasWorkspaceAccess(item.WorkspaceID) {
				return map[string]string{"error": "item not found"}, nil
			}
			children, err := crudSvc.GetChildren(itemID)
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
		Group:       CapabilityIssueManagement,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Perform a workflow status transition on an item. Identifies the item by ID or key, and the target status by ID or name. Workflow + condition rules are enforced.",
		Scopes:      []string{auth.ScopeItemsWrite},
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
					return map[string]string{"error": fmt.Sprintf("could not resolve status name %q: %s", args.ToStatusName, err.Error())}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
				}
				toStatusID = id
			default:
				return map[string]string{"error": "must provide to_status_id or to_status_name"}, nil
			}
			workflowSvc := services.NewWorkflowService(env.DB)
			conditionSvc := services.NewConditionService(env.DB, env.PermService, services.NewScriptEngine())
			approvalSvc := services.NewApprovalService(env.DB, repository.NewLeaveRepository(env.DB), workflowSvc)
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
			env.AuditWrite(logger.ResourceItem, itemID, "transition_item", result.Item.Title)
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
	ItemID          int    `json:"item_id,omitempty" jsonschema:"Item ID (numeric)"`
	ItemKey         string `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42"`
	FullDescription bool   `json:"full_description,omitempty" jsonschema:"Return the complete description. By default descriptions longer than 500 characters are truncated with a marker showing the total length."`
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
	ItemID            int            `json:"item_id,omitempty" jsonschema:"Item ID"`
	ItemKey           string         `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42"`
	Title             *string        `json:"title,omitempty" jsonschema:"New title"`
	Description       *string        `json:"description,omitempty" jsonschema:"New description"`
	PriorityID        *int           `json:"priority_id,omitempty" jsonschema:"New priority ID"`
	PriorityName      *string        `json:"priority_name,omitempty" jsonschema:"New priority name (alternative to ID)"`
	AssigneeID        *int           `json:"assignee_id,omitempty" jsonschema:"New assignee user ID (0 to unassign)"`
	AssigneeName      *string        `json:"assignee_name,omitempty" jsonschema:"New assignee full name (alternative to ID)"`
	DueDate           *string        `json:"due_date,omitempty" jsonschema:"Due date YYYY-MM-DD (empty string to clear)"`
	MilestoneID       *int           `json:"milestone_id,omitempty" jsonschema:"Milestone ID"`
	MilestoneName     *string        `json:"milestone_name,omitempty" jsonschema:"Milestone name (alternative to ID)"`
	IterationID       *int           `json:"iteration_id,omitempty" jsonschema:"Iteration ID"`
	IterationName     *string        `json:"iteration_name,omitempty" jsonschema:"Iteration name (alternative to ID)"`
	ProjectID         *int           `json:"project_id,omitempty" jsonschema:"Project ID"`
	ParentID          *int           `json:"parent_id,omitempty" jsonschema:"Parent item ID"`
	CustomFieldValues map[string]any `json:"custom_field_values,omitempty" jsonschema:"Custom field values map"`
}

type deleteItemArgs struct {
	ItemID  int    `json:"item_id,omitempty" jsonschema:"Item ID to delete (also deletes descendants). Provide either this or item_key."`
	ItemKey string `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42. Provide either this or item_id."`
}

type getItemChildrenArgs struct {
	ItemID  int    `json:"item_id,omitempty" jsonschema:"Parent item ID. Provide either this or item_key."`
	ItemKey string `json:"item_key,omitempty" jsonschema:"Parent item key like PROJ-42. Provide either this or item_id."`
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
func buildUpdateData(env *Env, args updateItemArgs, wsID int) (data map[string]any, changed []string, err error) {
	data = map[string]any{}
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
		id, err := services.NewIDResolverService(env.DB).ResolvePriorityIDByName(*args.PriorityName)
		if err != nil {
			return nil, nil, fmt.Errorf("could not resolve priority name %q: %w", *args.PriorityName, err)
		}
		out["priority_id"] = *id
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
		id, err := resolveAssigneeName(env, *args.AssigneeName, wsID)
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
			out["milestone_ids"] = []int{}
		} else {
			out["milestone_ids"] = []int{*args.MilestoneID}
		}
		changed = append(changed, "milestone")
	case args.MilestoneName != nil:
		id, err := resolveMilestoneName(env, *args.MilestoneName, wsID)
		if err != nil {
			return nil, nil, fmt.Errorf("could not resolve milestone name %q: %w", *args.MilestoneName, err)
		}
		out["milestone_ids"] = []int{id}
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
		id, err := resolveIterationName(env, *args.IterationName, wsID)
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
	out, err := repository.NewWorkspaceRepository(db).ListNameKeyToIDMap()
	if err != nil {
		return map[string]int{}
	}
	return out
}

// resolveStatusName resolves a status name to an ID, scoped to the statuses
// actually configured for the target workspace (via its configuration set's
// workflow — same source the workspace status endpoints use). A globally
// existing status that isn't part of the workspace's workflow is not a match;
// the error lists the workspace's valid statuses so the caller can pick one.
func resolveStatusName(db database.Database, name string, workspaceID int) (int, error) {
	statuses, err := services.NewWorkspaceService(db).GetStatuses(workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to load workspace statuses: %w", err)
	}
	var matches []models.Status
	for _, s := range statuses {
		if strings.EqualFold(s.Name, name) {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].ID, nil
	case 0:
		return 0, fmt.Errorf("status not found in this workspace; valid statuses: %s", statusCandidateList(statuses))
	default:
		return 0, fmt.Errorf("status name is ambiguous in this workspace; candidates: %s — pass to_status_id instead", statusCandidateList(matches))
	}
}

// statusCandidateList renders statuses as "Name (id N), ..." for tool errors.
func statusCandidateList(statuses []models.Status) string {
	if len(statuses) == 0 {
		return "(none configured)"
	}
	parts := make([]string, 0, len(statuses))
	for _, s := range statuses {
		parts = append(parts, fmt.Sprintf("%s (id %d)", s.Name, s.ID))
	}
	return strings.Join(parts, ", ")
}

// resolveAssigneeName resolves a user's full name to an ID, restricted to
// users visible in the item's workspace. Visibility reuses the canonical
// gated-aware check the HTTP layer builds workspace access from (item.view
// permission on the workspace, see PermissionService.AccessibleWorkspaceIDs)
// so a name match outside the workspace never resolves silently. Ambiguous
// or out-of-workspace matches return an error listing candidates so the
// caller can disambiguate (e.g. by passing assignee_id).
func resolveAssigneeName(env *Env, name string, workspaceID int) (int, error) {
	rows, err := repository.NewUserRepository(env.DB).FindIDsByFullName(name)
	if err != nil {
		return 0, fmt.Errorf("failed to look up user: %w", err)
	}
	matches := make([]userCandidate, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, userCandidate{id: row.ID, fullName: row.FullName})
	}
	if len(matches) == 0 {
		return 0, fmt.Errorf("user not found")
	}

	var inWorkspace []userCandidate
	for _, c := range matches {
		visible, err := env.PermService.HasWorkspacePermission(c.id, workspaceID, models.PermissionItemView)
		if err != nil {
			return 0, fmt.Errorf("failed to check workspace membership: %w", err)
		}
		if visible {
			inWorkspace = append(inWorkspace, c)
		}
	}
	switch len(inWorkspace) {
	case 1:
		return inWorkspace[0].id, nil
	case 0:
		return 0, fmt.Errorf("no matching user is a member of this workspace; matches elsewhere: %s", userCandidateList(matches))
	default:
		return 0, fmt.Errorf("name is ambiguous in this workspace; candidates: %s — pass assignee_id instead", userCandidateList(inWorkspace))
	}
}

// userCandidate is a (id, full name) pair used for assignee disambiguation
// messages.
type userCandidate struct {
	id       int
	fullName string
}

// userCandidateList renders user candidates as "Name (id N), ..." for tool errors.
func userCandidateList(users []userCandidate) string {
	parts := make([]string, 0, len(users))
	for _, u := range users {
		parts = append(parts, fmt.Sprintf("%s (id %d)", u.fullName, u.id))
	}
	return strings.Join(parts, ", ")
}

func resolveMilestoneName(env *Env, name string, workspaceID int) (int, error) {
	id, err := services.NewPlanningService(env.DB).FindMilestoneIDByName(workspaceID, name)
	if err != nil || id == nil {
		return 0, fmt.Errorf("milestone not found")
	}
	return *id, nil
}

func resolveIterationName(env *Env, name string, workspaceID int) (int, error) {
	id, err := services.NewPlanningService(env.DB).FindIterationIDByName(workspaceID, name)
	if err != nil || id == nil {
		return 0, fmt.Errorf("iteration not found")
	}
	return *id, nil
}
