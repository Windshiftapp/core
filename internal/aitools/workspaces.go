package aitools

import (
	"context"
	"database/sql"
	"errors"

	"windshift/internal/repository"
)

type workspaceDTO struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Key           string `json:"key"`
	Description   string `json:"description,omitempty"`
	Icon          string `json:"icon,omitempty"`
	Color         string `json:"color,omitempty"`
	Active        bool   `json:"active"`
	IsPersonal    bool   `json:"is_personal"`
	OwnerID       *int   `json:"owner_id,omitempty"`
	TimeProjectID *int   `json:"time_project_id,omitempty"`
}

type listWorkspacesArgs struct{}

type listWorkspacesOut struct {
	Workspaces []workspaceDTO `json:"workspaces"`
}

type getWorkspaceArgs struct {
	WorkspaceID int `json:"workspace_id" jsonschema:"Workspace ID to retrieve"`
}

func init() {
	Register(Default, Tool[listWorkspacesArgs]{
		Name:        "list_workspaces",
		Description: "List all workspaces the authenticated user has access to.",
		Run: func(_ context.Context, env *Env, _ listWorkspacesArgs) (any, error) {
			out := listWorkspacesOut{Workspaces: []workspaceDTO{}}
			if len(env.AccessibleWorkspaceIDs) == 0 {
				return out, nil
			}
			repo := repository.NewWorkspaceRepository(env.DB)
			for _, id := range env.AccessibleWorkspaceIDs {
				ws, err := repo.FindByID(id)
				if err != nil {
					continue
				}
				out.Workspaces = append(out.Workspaces, mapWorkspaceDTO(ws.ID, ws.Name, ws.Key, ws.Description, ws.Icon, ws.Color, ws.Active, ws.IsPersonal, ws.OwnerID, ws.TimeProjectID))
			}
			return out, nil
		},
	})

	Register(Default, Tool[getWorkspaceArgs]{
		Name:        "get_workspace",
		Description: "Get detailed information about a specific workspace.",
		Run: func(_ context.Context, env *Env, args getWorkspaceArgs) (any, error) {
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			ws, err := repository.NewWorkspaceRepository(env.DB).FindByID(args.WorkspaceID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) || errors.Is(err, repository.ErrNotFound) {
					return map[string]string{"error": "workspace not found"}, nil
				}
				return nil, err
			}
			return mapWorkspaceDTO(ws.ID, ws.Name, ws.Key, ws.Description, ws.Icon, ws.Color, ws.Active, ws.IsPersonal, ws.OwnerID, ws.TimeProjectID), nil
		},
	})
}

func mapWorkspaceDTO(id int, name, key, desc, icon, color string, active, isPersonal bool, owner, timeProj *int) workspaceDTO {
	return workspaceDTO{
		ID: id, Name: name, Key: key, Description: desc, Icon: icon, Color: color,
		Active: active, IsPersonal: isPersonal, OwnerID: owner, TimeProjectID: timeProj,
	}
}
