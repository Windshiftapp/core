package aitools

import (
	"context"
	"encoding/json"
	"errors"

	"windshift/internal/auth"
	"windshift/internal/services"
)

type createPageDiagramArgs struct {
	PageID              int             `json:"page_id" jsonschema:"Page ID to embed the diagram in"`
	Name                string          `json:"name" jsonschema:"Diagram name"`
	Mermaid             string          `json:"mermaid,omitempty" jsonschema:"Mermaid source; provide either this or excalidraw"`
	Excalidraw          json.RawMessage `json:"excalidraw,omitempty" jsonschema:"Excalidraw scene object; provide either this or mermaid"`
	Placement           string          `json:"placement" jsonschema:"Insertion placement: start or end"`
	ExpectedContentHash *string         `json:"expected_content_hash,omitempty" jsonschema:"Page content hash used as an optimistic concurrency precondition"`
}

type listPageDiagramsArgs struct {
	PageID int `json:"page_id" jsonschema:"Page ID"`
}

type getPageDiagramArgs struct {
	PageID       int `json:"page_id" jsonschema:"Page ID"`
	AttachmentID int `json:"attachment_id" jsonschema:"Diagram attachment ID embedded in the Page"`
}

type updatePageDiagramArgs struct {
	PageID              int             `json:"page_id" jsonschema:"Page ID"`
	AttachmentID        int             `json:"attachment_id" jsonschema:"Current diagram attachment ID embedded in the Page"`
	Name                string          `json:"name,omitempty" jsonschema:"Optional replacement diagram name"`
	Mermaid             string          `json:"mermaid,omitempty" jsonschema:"Replacement Mermaid source; provide either this or excalidraw"`
	Excalidraw          json.RawMessage `json:"excalidraw,omitempty" jsonschema:"Replacement Excalidraw scene; provide either this or mermaid"`
	ExpectedContentHash *string         `json:"expected_content_hash,omitempty" jsonschema:"Page content hash used as an optimistic concurrency precondition"`
}

func init() {
	Register(Default, Tool[createPageDiagramArgs]{
		Name:        "create_page_diagram",
		Group:       CapabilityKnowledgeDiagrams,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Create an editable Mermaid-seeded or Excalidraw diagram and embed it at the explicit start/end of a Page.",
		Scopes:      []string{auth.ScopePagesWrite},
		Run: func(ctx context.Context, env *Env, args createPageDiagramArgs) (any, error) {
			service, page, result := authorizedPageDiagramService(env, args.PageID)
			if result != nil {
				return result, nil
			}
			diagram, err := service.Create(ctx, pageAuditActor(env), services.CreatePageDiagramInput{
				PageID:              page.ID,
				Name:                args.Name,
				Mermaid:             args.Mermaid,
				Excalidraw:          args.Excalidraw,
				Placement:           args.Placement,
				ExpectedContentHash: args.ExpectedContentHash,
			})
			if err != nil {
				return pageDiagramToolError(err), nil
			}
			return diagram, nil
		},
	})

	Register(Default, Tool[listPageDiagramsArgs]{
		Name:        "list_page_diagrams",
		Group:       CapabilityKnowledgeDiagrams,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List editable diagrams embedded in a Page, including their attachment IDs, kinds, payloads, and current Page content hash.",
		Scopes:      []string{auth.ScopePagesRead},
		Run: func(ctx context.Context, env *Env, args listPageDiagramsArgs) (any, error) {
			service, page, result := authorizedPageDiagramService(env, args.PageID)
			if result != nil {
				return result, nil
			}
			diagrams, err := service.List(ctx, pageAuditActor(env), page.ID)
			if err != nil {
				return pageDiagramToolError(err), nil
			}
			return map[string]any{"diagrams": diagrams}, nil
		},
	})

	Register(Default, Tool[getPageDiagramArgs]{
		Name:        "get_page_diagram",
		Group:       CapabilityKnowledgeDiagrams,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "Fetch one editable Page diagram by its Page and attachment IDs, including the Mermaid seed or Excalidraw scene.",
		Scopes:      []string{auth.ScopePagesRead},
		Run: func(ctx context.Context, env *Env, args getPageDiagramArgs) (any, error) {
			service, page, result := authorizedPageDiagramService(env, args.PageID)
			if result != nil {
				return result, nil
			}
			diagram, err := service.Get(ctx, pageAuditActor(env), page.ID, args.AttachmentID)
			if err != nil {
				return pageDiagramToolError(err), nil
			}
			return diagram, nil
		},
	})

	Register(Default, Tool[updatePageDiagramArgs]{
		Name:        "update_page_diagram",
		Group:       CapabilityKnowledgeDiagrams,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Replace exactly one embedded Page diagram with a new immutable Mermaid-seeded or Excalidraw attachment.",
		Scopes:      []string{auth.ScopePagesWrite},
		Run: func(ctx context.Context, env *Env, args updatePageDiagramArgs) (any, error) {
			service, page, result := authorizedPageDiagramService(env, args.PageID)
			if result != nil {
				return result, nil
			}
			diagram, err := service.Update(ctx, pageAuditActor(env), services.UpdatePageDiagramInput{
				PageID:              page.ID,
				AttachmentID:        args.AttachmentID,
				Name:                args.Name,
				Mermaid:             args.Mermaid,
				Excalidraw:          args.Excalidraw,
				ExpectedContentHash: args.ExpectedContentHash,
			})
			if err != nil {
				return pageDiagramToolError(err), nil
			}
			return diagram, nil
		},
	})
}

func authorizedPageDiagramService(env *Env, pageID int) (*services.PageDiagramService, pageDTO, any) {
	if env.PageDiagramService == nil {
		return nil, pageDTO{}, map[string]string{"error": "page diagram storage is unavailable"}
	}
	page, ok := loadWorkspacePage(env, pageApplicationService(env).PageService(), pageID)
	if !ok {
		return nil, pageDTO{}, map[string]string{"error": "page not found"}
	}
	return env.PageDiagramService, pageToDTO(page), nil
}

func pageDiagramToolError(err error) map[string]string {
	switch {
	case errors.Is(err, services.ErrPageContentConflict):
		return map[string]string{"error": "page content changed since it was read"}
	case errors.Is(err, services.ErrPageDiagramNotFound):
		return map[string]string{"error": "page diagram not found"}
	case errors.Is(err, services.ErrPageDiagramReferenceConflict):
		return map[string]string{"error": "diagram attachment must be referenced exactly once in the Page"}
	case errors.Is(err, services.ErrPageAttachmentUploadDisabled):
		return map[string]string{"error": "page diagram storage is disabled"}
	default:
		return map[string]string{"error": err.Error()}
	}
}
