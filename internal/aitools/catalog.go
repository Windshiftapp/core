package aitools

import (
	"context"

	"windshift/internal/auth"
	"windshift/internal/services"
)

// ----------------------------------------------------------------------------
// list_item_types
// ----------------------------------------------------------------------------

type listItemTypesArgs struct {
	WorkspaceID int `json:"workspace_id" jsonschema:"Workspace ID whose available item types to list"`
}

type itemTypeDTO struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	BuiltinKey     string `json:"builtin_key,omitempty"`
	Description    string `json:"description,omitempty"`
	Icon           string `json:"icon,omitempty"`
	Color          string `json:"color,omitempty"`
	HierarchyLevel int    `json:"hierarchy_level"`
	IsDefault      bool   `json:"is_default"`
}

type listItemTypesOut struct {
	ItemTypes []itemTypeDTO `json:"item_types"`
}

// ----------------------------------------------------------------------------
// list_statuses
// ----------------------------------------------------------------------------

type listStatusesArgs struct {
	WorkspaceID int `json:"workspace_id" jsonschema:"Workspace ID whose workflow statuses to list"`
}

type statusDTO struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	BuiltinKey    string `json:"builtin_key,omitempty"`
	Description   string `json:"description,omitempty"`
	CategoryID    int    `json:"category_id"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	IsDefault     bool   `json:"is_default"`
	IsCompleted   bool   `json:"is_completed"`
}

type listStatusesOut struct {
	Statuses []statusDTO `json:"statuses"`
}

func init() {
	Register(Default, Tool[listItemTypesArgs]{
		Name:        "list_item_types",
		Group:       CapabilityIssueManagement,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List the item types available in a workspace with their IDs and names. Use this to discover the item_type_id to pass to create_item.",
		// Workspace-scoped catalog — mirrors GET /workspaces/{id}/item-types.
		Scopes: []string{auth.ScopeWorkspacesRead},
		Run: func(_ context.Context, env *Env, args listItemTypesArgs) (any, error) {
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			types, err := services.NewWorkspaceService(env.DB).GetItemTypes(args.WorkspaceID)
			if err != nil {
				return nil, err
			}
			out := listItemTypesOut{ItemTypes: make([]itemTypeDTO, 0, len(types))}
			for _, t := range types {
				out.ItemTypes = append(out.ItemTypes, itemTypeDTO{
					ID: t.ID, Name: t.Name, BuiltinKey: t.BuiltinKey, Description: t.Description,
					Icon: t.Icon, Color: t.Color, HierarchyLevel: t.HierarchyLevel, IsDefault: t.IsDefault,
				})
			}
			return out, nil
		},
	})

	Register(Default, Tool[listStatusesArgs]{
		Name:        "list_statuses",
		Group:       CapabilityIssueManagement,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List the workflow statuses available in a workspace with their IDs and names. Use this to discover the status_id to pass to create_item, or a valid to_status_name/to_status_id for transition_item.",
		// Workspace-scoped catalog — mirrors GET /workspaces/{id}/statuses.
		Scopes: []string{auth.ScopeWorkspacesRead},
		Run: func(_ context.Context, env *Env, args listStatusesArgs) (any, error) {
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			statuses, err := services.NewWorkspaceService(env.DB).GetStatuses(args.WorkspaceID)
			if err != nil {
				return nil, err
			}
			out := listStatusesOut{Statuses: make([]statusDTO, 0, len(statuses))}
			for _, s := range statuses {
				out.Statuses = append(out.Statuses, statusDTO{
					ID: s.ID, Name: s.Name, BuiltinKey: s.BuiltinKey, Description: s.Description,
					CategoryID: s.CategoryID, CategoryName: s.CategoryName, CategoryColor: s.CategoryColor,
					IsDefault: s.IsDefault, IsCompleted: s.IsCompleted,
				})
			}
			return out, nil
		},
	})
}
