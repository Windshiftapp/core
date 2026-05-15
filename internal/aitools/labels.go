package aitools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/models"
	"windshift/internal/services"
)

type listLabelsArgs struct {
	WorkspaceID int `json:"workspace_id" jsonschema:"Workspace ID to list labels for"`
}

type labelDTO struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	WorkspaceID int    `json:"workspace_id"`
}

type listLabelsOut struct {
	Labels []labelDTO `json:"labels"`
}

type setItemLabelsArgs struct {
	ItemID   int   `json:"item_id" jsonschema:"Item ID to set labels on"`
	LabelIDs []int `json:"label_ids" jsonschema:"Label IDs to set (replaces all existing labels)"`
}

type setItemLabelsOut struct {
	ItemID   int   `json:"item_id"`
	LabelIDs []int `json:"label_ids"`
	Updated  bool  `json:"updated"`
}

func init() {
	Register(Default, Tool[listLabelsArgs]{
		Name:        "list_labels",
		Description: "List all labels in a workspace.",
		Run: func(_ context.Context, env *Env, args listLabelsArgs) (any, error) {
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			rows, err := env.DB.Query(
				"SELECT id, name, color, workspace_id FROM labels WHERE workspace_id = ? ORDER BY name",
				args.WorkspaceID,
			)
			if err != nil {
				return nil, err
			}
			defer func() { _ = rows.Close() }()
			out := listLabelsOut{Labels: []labelDTO{}}
			for rows.Next() {
				var l labelDTO
				if err := rows.Scan(&l.ID, &l.Name, &l.Color, &l.WorkspaceID); err != nil {
					continue
				}
				out.Labels = append(out.Labels, l)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return out, nil
		},
	})

	Register(Default, Tool[setItemLabelsArgs]{
		Name:        "set_item_labels",
		Description: "Set labels on a work item (replaces existing labels).",
		Run: func(_ context.Context, env *Env, args setItemLabelsArgs) (any, error) {
			item, err := services.NewItemCRUDService(env.DB).GetByID(args.ItemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if !env.HasWorkspaceAccess(item.WorkspaceID) {
				return map[string]string{"error": "item not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, item.WorkspaceID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			tx, err := env.DB.Begin()
			if err != nil {
				return nil, err
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.Exec("DELETE FROM item_labels WHERE item_id = ?", args.ItemID); err != nil {
				return nil, err
			}
			for _, labelID := range args.LabelIDs {
				var wsID int
				err := tx.QueryRow("SELECT workspace_id FROM labels WHERE id = ?", labelID).Scan(&wsID)
				if errors.Is(err, sql.ErrNoRows) {
					return map[string]string{"error": fmt.Sprintf("label %d not found", labelID)}, nil
				}
				if err != nil {
					return nil, err
				}
				if wsID != item.WorkspaceID {
					return map[string]string{"error": fmt.Sprintf("label %d belongs to a different workspace", labelID)}, nil
				}
				if _, err := tx.Exec("INSERT INTO item_labels (item_id, label_id) VALUES (?, ?)", args.ItemID, labelID); err != nil {
					if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
						continue
					}
					return nil, err
				}
			}
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return setItemLabelsOut{ItemID: args.ItemID, LabelIDs: args.LabelIDs, Updated: true}, nil
		},
	})
}
