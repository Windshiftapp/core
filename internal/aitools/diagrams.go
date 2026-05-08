package aitools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// Storage contract: item_diagrams.diagram_data is opaque TEXT. Existing rows
// hold a JSON-serialized Excalidraw scene ({elements, appState, files}).
// Agent-facing tools may also write a tiny seed wrapper:
//
//	{"type":"mermaid","source":"graph TD; A-->B"}
//
// DiagramModal.svelte detects the wrapper on open, calls
// parseMermaidToExcalidraw + convertToExcalidrawElements client-side, and
// uses the result as the editor's initialData. Saving the editor replaces
// the wrapper with the converted Excalidraw scene; the source string is
// not preserved — the seed is one-shot, matching how Excalidraw's own
// mermaid panel behaves and avoiding dual-write complexity.

const (
	diagramKindMermaid    = "mermaid"
	diagramKindExcalidraw = "excalidraw"
)

type diagramSummaryDTO struct {
	ID        int    `json:"id"`
	ItemID    int    `json:"item_id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type diagramDetailDTO struct {
	diagramSummaryDTO
	DiagramData string `json:"diagram_data"`
}

type listDiagramsArgs struct {
	ItemID  int    `json:"item_id,omitempty" jsonschema:"Numeric item ID. Provide either this or item_key."`
	ItemKey string `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42. Provide either this or item_id."`
}

type listDiagramsOut struct {
	Diagrams []diagramSummaryDTO `json:"diagrams"`
}

type getDiagramArgs struct {
	ID int `json:"id" jsonschema:"Numeric diagram ID."`
}

type createDiagramArgs struct {
	ItemID     int             `json:"item_id,omitempty" jsonschema:"Numeric item ID. Provide either this or item_key."`
	ItemKey    string          `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42. Provide either this or item_id."`
	Name       string          `json:"name" jsonschema:"Diagram name (required)."`
	Mermaid    string          `json:"mermaid,omitempty" jsonschema:"Mermaid source. Stored as a {type:mermaid,source} seed; the editor converts it to an Excalidraw scene the first time the diagram is opened. Provide either this or excalidraw."`
	Excalidraw json.RawMessage `json:"excalidraw,omitempty" jsonschema:"Pre-built Excalidraw scene JSON object ({elements, appState, files}). Stored as-is. Provide either this or mermaid."`
}

type updateDiagramArgs struct {
	ID         int             `json:"id" jsonschema:"Numeric diagram ID."`
	Name       string          `json:"name,omitempty" jsonschema:"New name. Omit to keep existing."`
	Mermaid    string          `json:"mermaid,omitempty" jsonschema:"Replace diagram_data with a mermaid seed. Omit (and omit excalidraw) to keep existing data."`
	Excalidraw json.RawMessage `json:"excalidraw,omitempty" jsonschema:"Replace diagram_data with an Excalidraw scene. Omit (and omit mermaid) to keep existing data."`
}

type deleteDiagramArgs struct {
	ID int `json:"id" jsonschema:"Numeric diagram ID."`
}

// buildDiagramData validates the mermaid/excalidraw inputs and returns the
// string to persist in diagram_data plus a kind label. Exactly one of the
// two inputs must be set; both empty or both populated is a tool error.
func buildDiagramData(mermaid string, excalidraw json.RawMessage) (data, kind string, err error) {
	mermaidSet := strings.TrimSpace(mermaid) != ""
	excalidrawSet := len(excalidraw) > 0 && string(excalidraw) != "null"
	switch {
	case mermaidSet && excalidrawSet:
		return "", "", errors.New("provide either mermaid or excalidraw, not both")
	case mermaidSet:
		wrapper, mErr := json.Marshal(map[string]string{
			"type":   diagramKindMermaid,
			"source": strings.TrimSpace(mermaid),
		})
		if mErr != nil {
			return "", "", fmt.Errorf("encode mermaid wrapper: %w", mErr)
		}
		return string(wrapper), diagramKindMermaid, nil
	case excalidrawSet:
		// json.RawMessage is already valid JSON because it came through the
		// adapter's Unmarshal. Reject the explicit `null` literal above.
		return string(excalidraw), diagramKindExcalidraw, nil
	default:
		return "", "", errors.New("provide either mermaid or excalidraw")
	}
}

// detectKind classifies an existing diagram_data string. Used in responses so
// callers can tell whether a row is still a seed or has been converted.
func detectKind(data string) string {
	if data == "" {
		return diagramKindExcalidraw
	}
	var probe struct {
		Type   string `json:"type"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(data), &probe); err == nil &&
		probe.Type == diagramKindMermaid && probe.Source != "" {
		return diagramKindMermaid
	}
	return diagramKindExcalidraw
}

func diagramToSummary(d *models.ItemDiagram) diagramSummaryDTO {
	return diagramSummaryDTO{
		ID:        d.ID,
		ItemID:    d.ItemID,
		Name:      d.Name,
		Kind:      detectKind(d.DiagramData),
		CreatedAt: d.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: d.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// itemWorkspaceForDiagram resolves an item by ID/key and returns its workspace
// ID for permission gating. toolErr is a tool-level error map for caller-visible
// problems (item not found / no access); when toolErr is non-nil the caller
// should return it directly. Unexpected internal errors are folded into the
// "item not found" toolErr to avoid leaking item existence to unauthorized callers.
func itemWorkspaceForDiagram(env *Env, itemID int, itemKey string) (resolvedItemID, workspaceID int, toolErr any) {
	id, rerr := resolveItemID(env.DB, itemID, itemKey)
	if rerr != nil {
		return 0, 0, map[string]string{"error": rerr.Error()}
	}
	wsID, gerr := services.NewItemCRUDService(env.DB).GetWorkspaceID(id)
	if gerr != nil {
		return 0, 0, map[string]string{"error": "item not found"}
	}
	if !env.HasWorkspaceAccess(wsID) {
		return 0, 0, map[string]string{"error": "item not found"}
	}
	return id, wsID, nil
}

func init() {
	// ------------------------------------------------------------------------
	// list_diagrams
	// ------------------------------------------------------------------------
	Register(Default, Tool[listDiagramsArgs]{
		Name:        "list_diagrams",
		Description: "List diagrams attached to a work item. Returns summaries (id, name, kind, timestamps); use get_diagram to fetch the full Excalidraw/mermaid payload.",
		Run: func(_ context.Context, env *Env, args listDiagramsArgs) (any, error) {
			itemID, wsID, toolErr := itemWorkspaceForDiagram(env, args.ItemID, args.ItemKey)
			if toolErr != nil {
				return toolErr, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, wsID, models.PermissionItemView)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "item not found"}, nil
			}
			diagrams, err := repository.NewDiagramRepository(env.DB).ListByItem(itemID)
			if err != nil {
				return nil, err
			}
			out := listDiagramsOut{Diagrams: make([]diagramSummaryDTO, 0, len(diagrams))}
			for i := range diagrams {
				out.Diagrams = append(out.Diagrams, diagramToSummary(&diagrams[i]))
			}
			return out, nil
		},
	})

	// ------------------------------------------------------------------------
	// get_diagram
	// ------------------------------------------------------------------------
	Register(Default, Tool[getDiagramArgs]{
		Name:        "get_diagram",
		Description: "Get a single diagram by ID, including its full diagram_data payload (an Excalidraw scene JSON or a {type:mermaid,source} seed wrapper).",
		Run: func(_ context.Context, env *Env, args getDiagramArgs) (any, error) {
			if args.ID <= 0 {
				return map[string]string{"error": "id is required"}, nil
			}
			repo := repository.NewDiagramRepository(env.DB)
			d, err := repo.GetByID(args.ID)
			if errors.Is(err, repository.ErrNotFound) {
				return map[string]string{"error": "diagram not found"}, nil
			}
			if err != nil {
				return nil, err
			}
			wsID, err := services.NewItemCRUDService(env.DB).GetWorkspaceID(d.ItemID)
			if err != nil {
				return map[string]string{"error": "diagram not found"}, nil //nolint:nilerr // hide diagram existence from unauthorized callers
			}
			if !env.HasWorkspaceAccess(wsID) {
				return map[string]string{"error": "diagram not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, wsID, models.PermissionItemView)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "diagram not found"}, nil
			}
			return diagramDetailDTO{
				diagramSummaryDTO: diagramToSummary(d),
				DiagramData:       d.DiagramData,
			}, nil
		},
	})

	// ------------------------------------------------------------------------
	// create_diagram
	// ------------------------------------------------------------------------
	Register(Default, Tool[createDiagramArgs]{
		Name:        "create_diagram",
		Description: "Attach a new diagram to a work item. Provide either `mermaid` (a mermaid source string, stored as a seed and converted on first open) or `excalidraw` (a fully-formed Excalidraw scene JSON, stored as-is).",
		Run: func(_ context.Context, env *Env, args createDiagramArgs) (any, error) {
			name := utils.SanitizeName(args.Name)
			if name == "" {
				return map[string]string{"error": "name is required"}, nil
			}
			data, kind, berr := buildDiagramData(args.Mermaid, args.Excalidraw)
			if berr != nil {
				return map[string]string{"error": berr.Error()}, nil //nolint:nilerr // surface validation errors to the caller as a tool error map
			}
			itemID, wsID, toolErr := itemWorkspaceForDiagram(env, args.ItemID, args.ItemKey)
			if toolErr != nil {
				return toolErr, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, wsID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			repo := repository.NewDiagramRepository(env.DB)
			userID := env.UserID
			id, now, err := repo.Create(itemID, name, data, &userID)
			if err != nil {
				return nil, err
			}
			if herr := repo.RecordHistory(itemID, env.UserID, "diagram_created", nil,
				fmt.Sprintf("diagram:%d:%s", id, name)); herr != nil {
				// History is best-effort, mirroring the handler.
				_ = herr
			}
			return diagramSummaryDTO{
				ID:        int(id),
				ItemID:    itemID,
				Name:      name,
				Kind:      kind,
				CreatedAt: now.UTC().Format("2006-01-02T15:04:05Z"),
				UpdatedAt: now.UTC().Format("2006-01-02T15:04:05Z"),
			}, nil
		},
	})

	// ------------------------------------------------------------------------
	// update_diagram
	// ------------------------------------------------------------------------
	Register(Default, Tool[updateDiagramArgs]{
		Name:        "update_diagram",
		Description: "Update a diagram. Each of name / mermaid / excalidraw is optional; omit to keep the existing value. Mermaid and excalidraw are mutually exclusive.",
		Run: func(_ context.Context, env *Env, args updateDiagramArgs) (any, error) {
			if args.ID <= 0 {
				return map[string]string{"error": "id is required"}, nil
			}
			repo := repository.NewDiagramRepository(env.DB)
			existing, err := repo.GetByID(args.ID)
			if errors.Is(err, repository.ErrNotFound) {
				return map[string]string{"error": "diagram not found"}, nil
			}
			if err != nil {
				return nil, err
			}
			wsID, err := services.NewItemCRUDService(env.DB).GetWorkspaceID(existing.ItemID)
			if err != nil {
				return map[string]string{"error": "diagram not found"}, nil //nolint:nilerr // hide diagram existence from unauthorized callers
			}
			if !env.HasWorkspaceAccess(wsID) {
				return map[string]string{"error": "diagram not found"}, nil
			}
			canEdit, err := env.PermService.HasWorkspacePermission(env.UserID, wsID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !canEdit {
				return map[string]string{"error": "permission denied"}, nil
			}

			newName := existing.Name
			if strings.TrimSpace(args.Name) != "" {
				newName = utils.SanitizeName(args.Name)
				if newName == "" {
					return map[string]string{"error": "name cannot be blank"}, nil
				}
			}

			newData := existing.DiagramData
			newKind := detectKind(existing.DiagramData)
			mermaidSet := strings.TrimSpace(args.Mermaid) != ""
			excalidrawSet := len(args.Excalidraw) > 0 && string(args.Excalidraw) != "null"
			if mermaidSet || excalidrawSet {
				data, kind, berr := buildDiagramData(args.Mermaid, args.Excalidraw)
				if berr != nil {
					return map[string]string{"error": berr.Error()}, nil //nolint:nilerr // surface validation errors to the caller as a tool error map
				}
				newData = data
				newKind = kind
			}

			userID := env.UserID
			if err := repo.Update(args.ID, newName, newData, &userID); err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return map[string]string{"error": "diagram not found"}, nil
				}
				return nil, err
			}
			var oldName *string
			if existing.Name != newName {
				old := existing.Name
				oldName = &old
			}
			_ = repo.RecordHistory(existing.ItemID, env.UserID, "diagram_updated", oldName,
				fmt.Sprintf("diagram:%d:%s", args.ID, newName))

			return diagramSummaryDTO{
				ID:     args.ID,
				ItemID: existing.ItemID,
				Name:   newName,
				Kind:   newKind,
			}, nil
		},
	})

	// ------------------------------------------------------------------------
	// delete_diagram
	// ------------------------------------------------------------------------
	Register(Default, Tool[deleteDiagramArgs]{
		Name:        "delete_diagram",
		Description: "Delete a diagram by ID.",
		Run: func(_ context.Context, env *Env, args deleteDiagramArgs) (any, error) {
			if args.ID <= 0 {
				return map[string]string{"error": "id is required"}, nil
			}
			repo := repository.NewDiagramRepository(env.DB)
			name, itemID, err := repo.GetNameAndItemID(args.ID)
			if errors.Is(err, repository.ErrNotFound) {
				return map[string]string{"error": "diagram not found"}, nil
			}
			if err != nil {
				return nil, err
			}
			wsID, err := services.NewItemCRUDService(env.DB).GetWorkspaceID(itemID)
			if err != nil {
				return map[string]string{"error": "diagram not found"}, nil //nolint:nilerr // hide diagram existence from unauthorized callers
			}
			if !env.HasWorkspaceAccess(wsID) {
				return map[string]string{"error": "diagram not found"}, nil
			}
			canEdit, err := env.PermService.HasWorkspacePermission(env.UserID, wsID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !canEdit {
				return map[string]string{"error": "permission denied"}, nil
			}
			_ = repo.RecordHistory(itemID, env.UserID, "diagram_deleted", &name, name)
			if err := repo.Delete(args.ID); err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return map[string]string{"error": "diagram not found"}, nil
				}
				return nil, err
			}
			return map[string]any{
				"success": true,
				"id":      args.ID,
			}, nil
		},
	})
}
