package mcp

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

func (ms *MCPServer) registerItemTools() {
	type listItemsInput struct {
		WorkspaceID *int `json:"workspace_id,omitempty" jsonschema:"Filter by workspace ID"`
		StatusID    *int `json:"status_id,omitempty" jsonschema:"Filter by status ID"`
		AssigneeID  *int `json:"assignee_id,omitempty" jsonschema:"Filter by assignee user ID"`
		ParentID    *int `json:"parent_id,omitempty" jsonschema:"Filter by parent item ID (0 for root items only)"`
		Limit       int  `json:"limit,omitempty" jsonschema:"Max items to return (default 50)"`
		Offset      int  `json:"offset,omitempty" jsonschema:"Offset for pagination"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "list_items",
		Description: "List work items. Requires workspace_id or returns items across all accessible workspaces.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listItemsInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		wsIDs, err := ms.accessibleWorkspaceIDs(user.ID)
		if err != nil {
			return errInternal("workspaces", err)
		}
		if len(wsIDs) == 0 {
			return emptyList()
		}

		limit := args.Limit
		if limit <= 0 || limit > 200 {
			limit = 50
		}

		filters := services.ItemFilters{}
		if args.WorkspaceID != nil {
			filters.WorkspaceID = args.WorkspaceID
		}
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

		items, total, err := services.NewItemCRUDService(ms.deps.DB).List(services.ItemListParams{
			WorkspaceIDs: wsIDs,
			Filters:      filters,
			Pagination:   services.PaginationParams{Limit: limit, Offset: args.Offset},
			SortBy:       "created_at",
		})
		if err != nil {
			return errInternal("list items", err)
		}

		return toolJSON(map[string]any{"items": mapItems(items), "total": total})
	})

	type getItemInput struct {
		ItemID int `json:"item_id" jsonschema:"The item ID to retrieve"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "get_item",
		Description: "Get a work item by ID with full details including labels.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getItemInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		crud := services.NewItemCRUDService(ms.deps.DB)
		item, err := crud.GetByID(args.ItemID)
		if err != nil {
			return toolError("item not found")
		}

		if ok, _ := ms.canViewItem(user.ID, item.WorkspaceID); !ok {
			return toolError("item not found")
		}

		return toolJSON(mapItem(*item))
	})

	type createItemInput struct {
		WorkspaceID int    `json:"workspace_id" jsonschema:"Workspace to create item in"`
		Title       string `json:"title" jsonschema:"Item title"`
		Description string `json:"description,omitempty" jsonschema:"Item description (supports TipTap JSON or plain text)"`
		StatusID    *int   `json:"status_id,omitempty" jsonschema:"Status ID (uses workflow default if omitted)"`
		PriorityID  *int   `json:"priority_id,omitempty" jsonschema:"Priority ID"`
		AssigneeID  *int   `json:"assignee_id,omitempty" jsonschema:"Assignee user ID"`
		ParentID    *int   `json:"parent_id,omitempty" jsonschema:"Parent item ID for sub-items"`
		ItemTypeID  *int   `json:"item_type_id,omitempty" jsonschema:"Item type ID"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "create_item",
		Description: "Create a new work item in a workspace.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createItemInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		if strings.TrimSpace(args.Title) == "" {
			return toolError("title is required")
		}

		if ok, _ := ms.canEditItem(user.ID, args.WorkspaceID); !ok {
			return toolError("workspace not found or no edit permission")
		}

		title := utils.StripHTMLTags(args.Title)
		desc := utils.SanitizeCommentContent(args.Description)

		itemID, err := services.CreateItem(ms.deps.DB, services.ItemCreationParams{
			WorkspaceID: args.WorkspaceID,
			Title:       title,
			Description: desc,
			StatusID:    args.StatusID,
			PriorityID:  args.PriorityID,
			AssigneeID:  args.AssigneeID,
			ParentID:    args.ParentID,
			ItemTypeID:  args.ItemTypeID,
			CreatorID:   &user.ID,
		})
		if err != nil {
			return errInternal("create item", err)
		}

		crud := services.NewItemCRUDService(ms.deps.DB)
		created, err := crud.GetByID(int(itemID))
		if err != nil {
			return toolJSON(map[string]any{"id": itemID})
		}

		return toolJSON(mapItem(*created))
	})

	type updateItemInput struct {
		ItemID      int     `json:"item_id" jsonschema:"Item ID to update"`
		Title       *string `json:"title,omitempty" jsonschema:"New title"`
		Description *string `json:"description,omitempty" jsonschema:"New description"`
		StatusID    *int    `json:"status_id,omitempty" jsonschema:"New status ID"`
		PriorityID  *int    `json:"priority_id,omitempty" jsonschema:"New priority ID"`
		AssigneeID  *int    `json:"assignee_id,omitempty" jsonschema:"New assignee user ID (0 to unassign)"`
		DueDate     *string `json:"due_date,omitempty" jsonschema:"Due date in YYYY-MM-DD format (empty string to clear)"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "update_item",
		Description: "Update fields on an existing work item.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateItemInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		crud := services.NewItemCRUDService(ms.deps.DB)
		item, err := crud.GetByID(args.ItemID)
		if err != nil {
			return toolError("item not found")
		}

		if ok, _ := ms.canEditItem(user.ID, item.WorkspaceID); !ok {
			return toolError("item not found")
		}

		updateData := make(map[string]interface{})
		if args.Title != nil {
			updateData["title"] = utils.StripHTMLTags(*args.Title)
		}
		if args.Description != nil {
			updateData["description"] = utils.SanitizeCommentContent(*args.Description)
		}
		if args.StatusID != nil {
			updateData["status_id"] = *args.StatusID
		}
		if args.PriorityID != nil {
			updateData["priority_id"] = *args.PriorityID
		}
		if args.AssigneeID != nil {
			updateData["assignee_id"] = *args.AssigneeID
		}
		if args.DueDate != nil {
			if *args.DueDate == "" {
				updateData["due_date"] = nil
			} else {
				t, parseErr := time.Parse("2006-01-02", *args.DueDate)
				if parseErr != nil {
					return toolError("invalid due_date format, use YYYY-MM-DD")
				}
				updateData["due_date"] = t
			}
		}

		if len(updateData) == 0 {
			return toolError("no fields to update")
		}

		svc := services.NewItemUpdateService(ms.deps.DB)
		result, err := svc.UpdateItem(services.UpdateItemRequest{
			ItemID:     args.ItemID,
			UpdateData: updateData,
			UserID:     user.ID,
		})
		if err != nil {
			return errInternal("update item", err)
		}

		return toolJSON(mapItem(*result.Item))
	})

	type deleteItemInput struct {
		ItemID int `json:"item_id" jsonschema:"Item ID to delete (also deletes descendants)"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "delete_item",
		Description: "Delete a work item and all its descendants.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteItemInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		crud := services.NewItemCRUDService(ms.deps.DB)
		item, err := crud.GetByID(args.ItemID)
		if err != nil {
			return toolError("item not found")
		}

		if ok, _ := ms.canDeleteItem(user.ID, item.WorkspaceID); !ok {
			return toolError("item not found")
		}

		result, err := crud.Delete(args.ItemID)
		if err != nil {
			return errInternal("delete item", err)
		}

		return toolJSON(map[string]any{
			"deleted":       true,
			"deleted_count": result.DeletedCount,
		})
	})

	type searchItemsInput struct {
		Query       string `json:"query" jsonschema:"Search query (full-text search on title and description)"`
		WorkspaceID *int   `json:"workspace_id,omitempty" jsonschema:"Limit search to a specific workspace"`
		Limit       int    `json:"limit,omitempty" jsonschema:"Max results (default 20)"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "search_items",
		Description: "Full-text search for work items by title or description.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchItemsInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		if strings.TrimSpace(args.Query) == "" {
			return toolError("query is required")
		}

		wsIDs, err := ms.accessibleWorkspaceIDs(user.ID)
		if err != nil {
			return errInternal("workspaces", err)
		}
		if len(wsIDs) == 0 {
			return emptyList()
		}

		// If workspace_id specified, filter to just that workspace
		if args.WorkspaceID != nil {
			found := false
			for _, id := range wsIDs {
				if id == *args.WorkspaceID {
					found = true
					break
				}
			}
			if !found {
				return emptyList()
			}
			wsIDs = []int{*args.WorkspaceID}
		}

		limit := args.Limit
		if limit <= 0 || limit > 100 {
			limit = 20
		}

		items, total, err := services.NewItemCRUDService(ms.deps.DB).Search(
			args.Query, wsIDs, services.PaginationParams{Limit: limit},
		)
		if err != nil {
			return errInternal("search", err)
		}

		return toolJSON(map[string]any{"items": mapItems(items), "total": total})
	})

	type getItemChildrenInput struct {
		ItemID int `json:"item_id" jsonschema:"Parent item ID"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "get_item_children",
		Description: "Get the direct children of a work item.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getItemChildrenInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		crud := services.NewItemCRUDService(ms.deps.DB)
		item, err := crud.GetByID(args.ItemID)
		if err != nil {
			return toolError("item not found")
		}

		if ok, _ := ms.canViewItem(user.ID, item.WorkspaceID); !ok {
			return toolError("item not found")
		}

		children, err := crud.GetChildren(args.ItemID)
		if err != nil {
			return errInternal("get children", err)
		}

		items := make([]models.Item, len(children))
		for i, c := range children {
			items[i] = *c
		}

		return toolJSON(map[string]any{"children": mapItems(items)})
	})
}

// Permission helpers

func (ms *MCPServer) accessibleWorkspaceIDs(userID int) ([]int, error) {
	rows, err := ms.deps.DB.Query(`
		SELECT DISTINCT w.id
		FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE w.active = true
		   OR (w.active = false AND uwr.role_id IS NOT NULL)
		   OR (w.active = false AND grp.workspace_id IS NOT NULL)
		   OR (w.is_personal = true AND w.owner_id = ?)
	`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (ms *MCPServer) canViewItem(userID, workspaceID int) (bool, error) {
	return ms.deps.PermissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
}

func (ms *MCPServer) canEditItem(userID, workspaceID int) (bool, error) {
	return ms.deps.PermissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemEdit)
}

func (ms *MCPServer) canDeleteItem(userID, workspaceID int) (bool, error) {
	return ms.deps.PermissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemDelete)
}

// Mapping helpers

func mapItem(item models.Item) map[string]any {
	m := map[string]any{
		"id":           item.ID,
		"workspace_id": item.WorkspaceID,
		"title":        item.Title,
		"created_at":   item.CreatedAt.Format(time.RFC3339),
		"updated_at":   item.UpdatedAt.Format(time.RFC3339),
	}
	if item.WorkspaceKey != "" {
		m["key"] = item.WorkspaceKey + "-" + itoa(item.WorkspaceItemNumber)
	}
	if item.Description != "" {
		m["description"] = item.Description
	}
	if item.StatusID != nil {
		m["status_id"] = *item.StatusID
	}
	if item.StatusName != "" {
		m["status_name"] = item.StatusName
	}
	if item.PriorityID != nil {
		m["priority_id"] = *item.PriorityID
	}
	if item.PriorityName != "" {
		m["priority_name"] = item.PriorityName
	}
	if item.AssigneeID != nil {
		m["assignee_id"] = *item.AssigneeID
	}
	if item.AssigneeName != "" {
		m["assignee_name"] = item.AssigneeName
	}
	if item.ParentID != nil {
		m["parent_id"] = *item.ParentID
	}
	if item.DueDate != nil {
		m["due_date"] = item.DueDate.Format("2006-01-02")
	}
	if item.ItemTypeID != nil {
		m["item_type_id"] = *item.ItemTypeID
	}
	if item.ItemTypeName != "" {
		m["item_type_name"] = item.ItemTypeName
	}
	if item.WorkspaceName != "" {
		m["workspace_name"] = item.WorkspaceName
	}
	if len(item.Labels) > 0 {
		labels := make([]map[string]any, len(item.Labels))
		for i, l := range item.Labels {
			labels[i] = map[string]any{"id": l.ID, "name": l.Name, "color": l.Color}
		}
		m["labels"] = labels
	}
	return m
}

func mapItems(items []models.Item) []map[string]any {
	result := make([]map[string]any, len(items))
	for i, item := range items {
		result[i] = mapItem(item)
	}
	return result
}

// errNoAuth returns a standard auth error for tool handlers.
func errNoAuth() (*mcp.CallToolResult, any, error) {
	return toolError("authentication required")
}

// errInternal returns a tool error for internal failures.
func errInternal(op string, err error) (*mcp.CallToolResult, any, error) {
	return toolErrorf("failed to %s: %v", op, err)
}

// emptyList returns an empty items list.
func emptyList() (*mcp.CallToolResult, any, error) {
	return toolJSON(map[string]any{"items": []any{}, "total": 0})
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
