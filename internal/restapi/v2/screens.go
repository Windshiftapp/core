package v2

import (
	"errors"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/services"
)

type screenDTO struct {
	ID                 int                  `json:"id"`
	BuiltinKey         *string              `json:"builtin_key"`
	Name               string               `json:"name"`
	DisplayName        string               `json:"display_name"`
	Description        string               `json:"description"`
	DisplayDescription string               `json:"display_description,omitempty"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
	Fields             []models.ScreenField `json:"fields,omitempty"`
	SystemFields       []string             `json:"system_fields,omitempty"`
}

func screenDTOFromModel(screen *models.Screen) screenDTO {
	builtinKey := nullableString(screen.BuiltinKey)
	return screenDTO{
		ID:                 screen.ID,
		BuiltinKey:         builtinKey,
		Name:               screen.Name,
		DisplayName:        screen.DisplayName,
		Description:        screen.Description,
		DisplayDescription: screen.DisplayDescription,
		CreatedAt:          screen.CreatedAt,
		UpdatedAt:          screen.UpdatedAt,
		Fields:             screen.Fields,
		SystemFields:       screen.SystemFields,
	}
}

type screenCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type screenPatchRequest struct {
	Name        Optional[string] `json:"name"`
	Description Optional[string] `json:"description"`
}

type screenFieldsRequest struct {
	Fields []models.ScreenField `json:"fields"`
}

type screenSystemFieldsRequest struct {
	SystemFields []string `json:"system_fields"`
}

// screenMutationError maps shared screen provisioning errors onto v2 error
// semantics. ServiceError values already carry the intended HTTP status.
func screenMutationError(err error) error {
	if err == nil {
		return nil
	}
	var serviceErr *services.ServiceError
	if errors.As(err, &serviceErr) {
		return newError(serviceErr.StatusCode, catalogErrorCode(serviceErr.StatusCode), serviceErr.Message)
	}
	return internalError(err)
}

func registerScreenRoutes(b *routeBuilder, deps Deps) {
	read := []string{"screens:read"}
	write := []string{"screens:write"}

	b.Page("/screens", AuthAuthenticated, read, func(r *http.Request) ([]screenDTO, Pagination, int, error) {
		page, err := ParsePagination(r, map[string]bool{"name": true, "created_at": true, "updated_at": true}, "name")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		screens, total, err := deps.Screens.ListScreensPage(
			r.URL.Query().Get("include_fields") == "true",
			clampSearch(r.URL.Query().Get("search")),
			page.Sort, page.Desc, page.PageSize, page.Offset,
		)
		if err != nil {
			return nil, Pagination{}, 0, screenMutationError(err)
		}
		items := make([]screenDTO, len(screens))
		for i := range screens {
			items[i] = screenDTOFromModel(&screens[i])
		}
		if err := localizeCatalog(r, deps.ObjectTranslations, "screen", &items); err != nil {
			return nil, Pagination{}, 0, err
		}
		return items, page, total, nil
	})

	b.Read("/screens/{screen_id}", AuthAuthenticated, read, func(r *http.Request) (screenDTO, error) {
		id, err := pathID(r, "screen_id")
		if err != nil {
			return screenDTO{}, err
		}
		screen, err := deps.Screens.GetScreen(id)
		if err != nil {
			return screenDTO{}, screenMutationError(err)
		}
		item := screenDTOFromModel(screen)
		if err := localizeCatalog(r, deps.ObjectTranslations, "screen", &item); err != nil {
			return screenDTO{}, err
		}
		return item, nil
	})

	b.JSON(http.MethodPost, "/screens", http.StatusCreated, false, AuthAuthenticated, write, func(r *http.Request, input screenCreateRequest) (screenDTO, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return screenDTO{}, err
		}
		screen, warnings, err := deps.Screens.Create(auditActorFromRequest(r), &models.Screen{Name: input.Name, Description: input.Description})
		if err != nil {
			return screenDTO{}, screenMutationError(err)
		}
		_ = warnings
		return screenDTOFromModel(screen), nil
	})

	b.JSON(http.MethodPatch, "/screens/{screen_id}", http.StatusOK, true, AuthAuthenticated, write, func(r *http.Request, input screenPatchRequest) (screenDTO, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return screenDTO{}, err
		}
		id, err := pathID(r, "screen_id")
		if err != nil {
			return screenDTO{}, err
		}
		current, err := deps.Screens.GetScreen(id)
		if err != nil {
			return screenDTO{}, screenMutationError(err)
		}
		if input.Name.Set {
			current.Name = input.Name.Value
		}
		if input.Description.Set {
			current.Description = input.Description.Value
		}
		updated, _, err := deps.Screens.Update(auditActorFromRequest(r), id, &models.Screen{Name: current.Name, Description: current.Description})
		if err != nil {
			return screenDTO{}, screenMutationError(err)
		}
		return screenDTOFromModel(updated), nil
	})

	b.Command(http.MethodDelete, "/screens/{screen_id}", AuthAuthenticated, write, func(r *http.Request) error {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return err
		}
		id, err := pathID(r, "screen_id")
		if err != nil {
			return err
		}
		return screenMutationError(deps.Screens.Delete(auditActorFromRequest(r), id))
	})

	b.Read("/screens/{screen_id}/fields", AuthAuthenticated, read, func(r *http.Request) ([]models.ScreenField, error) {
		id, err := pathID(r, "screen_id")
		if err != nil {
			return nil, err
		}
		fields, err := deps.Screens.GetScreenFields(id)
		if err != nil {
			return nil, screenMutationError(err)
		}
		return fields, nil
	})

	b.JSON(http.MethodPut, "/screens/{screen_id}/fields", http.StatusOK, false, AuthAuthenticated, write, func(r *http.Request, input screenFieldsRequest) ([]models.ScreenField, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		id, err := pathID(r, "screen_id")
		if err != nil {
			return nil, err
		}
		fields, err := deps.Screens.ReplaceScreenFields(auditActorFromRequest(r), id, input.Fields)
		if err != nil {
			return nil, screenMutationError(err)
		}
		return fields, nil
	})

	b.JSON(http.MethodPut, "/screens/{screen_id}/system-fields", http.StatusOK, false, AuthAuthenticated, write, func(r *http.Request, input screenSystemFieldsRequest) ([]string, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		id, err := pathID(r, "screen_id")
		if err != nil {
			return nil, err
		}
		systemFields, err := deps.Screens.ReplaceScreenSystemFields(auditActorFromRequest(r), id, input.SystemFields)
		if err != nil {
			return nil, screenMutationError(err)
		}
		return systemFields, nil
	})
}
