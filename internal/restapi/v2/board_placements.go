package v2

import (
	"context"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/services"
)

type boardPlacementApplication interface {
	ListBoardPlacements(context.Context, int, int) ([]models.BoardPlacement, error)
	SaveBoardPlacement(context.Context, services.AuditActor, int, string, models.BoardPlacement) (*models.BoardPlacement, error)
	DeleteBoardPlacement(context.Context, services.AuditActor, int, string, int) error
	GetPortalExperienceSettings(context.Context, int, int) (*services.PortalExperienceSettings, error)
	SavePortalExperienceSettings(context.Context, services.AuditActor, int, services.PortalExperienceSettings) (*services.PortalExperienceSettings, error)
}

func registerBoardPlacementRoutes(b *routeBuilder, collections collectionApplication) {
	application := func() (boardPlacementApplication, error) {
		service, ok := collections.(boardPlacementApplication)
		if !ok {
			return nil, newError(http.StatusServiceUnavailable, "unavailable", "Board placements are unavailable")
		}
		return service, nil
	}
	b.Read("/channels/{channel_id}/experience", AuthAuthenticated, []string{"collections:read"}, func(r *http.Request) (*services.PortalExperienceSettings, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		id, err := pathID(r, "channel_id")
		if err != nil {
			return nil, err
		}
		service, err := application()
		if err != nil {
			return nil, err
		}
		result, err := service.GetPortalExperienceSettings(r.Context(), user.ID, id)
		return result, collectionError(err)
	})
	b.JSON(http.MethodPut, "/channels/{channel_id}/experience", http.StatusOK, false, AuthAuthenticated, []string{"collections:write"}, func(r *http.Request, input services.PortalExperienceSettings) (*services.PortalExperienceSettings, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		id, err := pathID(r, "channel_id")
		if err != nil {
			return nil, err
		}
		service, err := application()
		if err != nil {
			return nil, err
		}
		result, err := service.SavePortalExperienceSettings(r.Context(), auditActor(r, user), id, input)
		return result, collectionError(err)
	})
	b.Read("/channels/{channel_id}/board-placements", AuthAuthenticated, []string{"collections:read"}, func(r *http.Request) ([]models.BoardPlacement, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		id, err := pathID(r, "channel_id")
		if err != nil {
			return nil, err
		}
		service, err := application()
		if err != nil {
			return nil, err
		}
		result, err := service.ListBoardPlacements(r.Context(), user.ID, id)
		return result, collectionError(err)
	})
	for _, entry := range []struct {
		method, path string
		status       int
	}{
		{http.MethodPost, "/channels/{channel_id}/board-placements", http.StatusCreated},
		{http.MethodPut, "/channels/{channel_id}/board-placements/{placement_id}", http.StatusOK},
	} {
		b.JSON(entry.method, entry.path, entry.status, false, AuthAuthenticated, []string{"collections:write"}, func(r *http.Request, input models.BoardPlacement) (*models.BoardPlacement, error) {
			user, err := principal(r)
			if err != nil {
				return nil, err
			}
			id, err := pathID(r, "channel_id")
			if err != nil {
				return nil, err
			}
			service, err := application()
			if err != nil {
				return nil, err
			}
			result, err := service.SaveBoardPlacement(r.Context(), auditActor(r, user), id, r.PathValue("placement_id"), input)
			return result, collectionError(err)
		})
	}
	b.Command(http.MethodDelete, "/channels/{channel_id}/board-placements/{placement_id}", AuthAuthenticated, []string{"collections:write"}, func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		id, err := pathID(r, "channel_id")
		if err != nil {
			return err
		}
		revision, err := strconv.Atoi(r.URL.Query().Get("revision"))
		if err != nil || revision < 1 {
			return newError(http.StatusBadRequest, "invalid_request", "revision is required")
		}
		service, err := application()
		if err != nil {
			return err
		}
		return collectionError(service.DeleteBoardPlacement(r.Context(), auditActor(r, user), id, r.PathValue("placement_id"), revision))
	})
}
