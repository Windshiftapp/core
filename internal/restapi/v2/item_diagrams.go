package v2

import (
	"errors"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerItemDiagramRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/items/{item_id}/diagrams", AuthAuthenticated, []string{"items:read"}, listItemDiagrams(deps))
	builder.JSON(http.MethodPost, "/items/{item_id}/diagrams", http.StatusCreated, false, AuthAuthenticated, []string{"items:write"}, createItemDiagram(deps))
	builder.Read("/item-diagrams/{diagram_id}", AuthAuthenticated, []string{"items:read"}, getItemDiagram(deps))
	builder.JSON(http.MethodPatch, "/item-diagrams/{diagram_id}", http.StatusOK, true, AuthAuthenticated, []string{"items:write"}, updateItemDiagram(deps))
	builder.Command(http.MethodDelete, "/item-diagrams/{diagram_id}", AuthAuthenticated, []string{"items:write"}, deleteItemDiagram(deps))
}

type itemDiagramRequest struct {
	Name        string `json:"name"`
	DiagramData string `json:"diagram_data"`
}

type itemDiagramPatchRequest struct {
	Name        Optional[string] `json:"name"`
	DiagramData Optional[string] `json:"diagram_data"`
}

type itemDiagramDTO struct {
	ID             int       `json:"id"`
	ItemID         int       `json:"item_id"`
	Name           string    `json:"name"`
	DiagramData    string    `json:"diagram_data"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedBy      *int      `json:"created_by"`
	UpdatedBy      *int      `json:"updated_by"`
	CreatorName    string    `json:"creator_name"`
	CreatorEmail   string    `json:"creator_email"`
	UpdatedByName  string    `json:"updated_by_name"`
	UpdatedByEmail string    `json:"updated_by_email"`
}

func listItemDiagrams(deps Deps) readOperation[[]itemDiagramDTO] {
	return func(r *http.Request) ([]itemDiagramDTO, error) {
		item, err := requireItem(r, deps, deps.Access.CanViewWorkspace)
		if err != nil {
			return nil, err
		}
		diagrams, err := deps.ItemDiagrams.List(item.ID)
		if err != nil {
			return nil, internalError(err)
		}
		result := make([]itemDiagramDTO, len(diagrams))
		for i := range diagrams {
			result[i] = itemDiagramFromModel(&diagrams[i])
		}
		return result, nil
	}
}

func createItemDiagram(deps Deps) jsonOperation[itemDiagramRequest, itemDiagramDTO] {
	return func(r *http.Request, input itemDiagramRequest) (itemDiagramDTO, error) {
		user, err := principal(r)
		if err != nil {
			return itemDiagramDTO{}, err
		}
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return itemDiagramDTO{}, err
		}
		diagram, err := deps.ItemDiagrams.Create(item.ID, input.Name, input.DiagramData, user.ID)
		if err != nil {
			return itemDiagramDTO{}, itemDiagramError(err)
		}
		return itemDiagramFromModel(diagram), nil
	}
}

func getItemDiagram(deps Deps) readOperation[itemDiagramDTO] {
	return func(r *http.Request) (itemDiagramDTO, error) {
		diagram, err := itemDiagramTarget(r, deps, deps.Access.CanViewWorkspace)
		if err != nil {
			return itemDiagramDTO{}, err
		}
		return itemDiagramFromModel(diagram), nil
	}
}

func updateItemDiagram(deps Deps) jsonOperation[itemDiagramPatchRequest, itemDiagramDTO] {
	return func(r *http.Request, input itemDiagramPatchRequest) (itemDiagramDTO, error) {
		if input.Name.Null || input.DiagramData.Null {
			return itemDiagramDTO{}, newError(http.StatusBadRequest, "invalid_request", "Diagram fields cannot be null")
		}
		user, err := principal(r)
		if err != nil {
			return itemDiagramDTO{}, err
		}
		diagram, err := itemDiagramTarget(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return itemDiagramDTO{}, err
		}
		updated, err := deps.ItemDiagrams.Patch(diagram.ID, optionalValue(input.Name), optionalValue(input.DiagramData), user.ID)
		if err != nil {
			return itemDiagramDTO{}, itemDiagramError(err)
		}
		return itemDiagramFromModel(updated), nil
	}
}

func deleteItemDiagram(deps Deps) commandOperation {
	return func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		diagram, err := itemDiagramTarget(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return err
		}
		_, err = deps.ItemDiagrams.Delete(diagram.ID, user.ID)
		return itemDiagramError(err)
	}
}

func itemDiagramTarget(r *http.Request, deps Deps, allowed func(int, int) (bool, error)) (*models.ItemDiagram, error) {
	id, err := pathID(r, "diagram_id")
	if err != nil {
		return nil, err
	}
	diagram, err := deps.ItemDiagrams.Get(id)
	if err != nil {
		return nil, itemDiagramError(err)
	}
	if _, err := requireItemID(r, deps, diagram.ItemID, allowed); err != nil {
		return nil, err
	}
	return diagram, nil
}

func itemDiagramFromModel(diagram *models.ItemDiagram) itemDiagramDTO {
	return itemDiagramDTO{
		ID: diagram.ID, ItemID: diagram.ItemID, Name: diagram.Name, DiagramData: diagram.DiagramData,
		CreatedAt: diagram.CreatedAt, UpdatedAt: diagram.UpdatedAt, CreatedBy: diagram.CreatedBy,
		UpdatedBy: diagram.UpdatedBy, CreatorName: diagram.CreatorName,
		CreatorEmail: diagram.CreatorEmail, UpdatedByName: diagram.UpdatedByName,
		UpdatedByEmail: diagram.UpdatedByEmail,
	}
}

func itemDiagramError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Item diagram was not found")
	case errors.Is(err, services.ErrDiagramNameRequired), errors.Is(err, services.ErrDiagramDataRequired):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
