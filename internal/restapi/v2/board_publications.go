package v2

import (
	"net/http"

	"windshift/internal/services"
)

type boardSharingApplication interface {
	GetBoardSharing(int, int) (*services.BoardSharing, error)
	SaveBoardSharing(services.AuditActor, int, services.BoardSharingSave) (*services.BoardSharing, error)
	RevokeBoardSharing(services.AuditActor, int, int, string, bool) (*services.BoardSharing, error)
	BoardSharingHistory(int, int) ([]services.BoardSharingEvent, error)
	ListBoardPublications(int, services.BoardPublicationFilter) ([]services.BoardSharing, int, error)
}

func registerBoardPublicationRoutes(b *routeBuilder, collections collectionApplication) {
	application := func() (boardSharingApplication, error) {
		service, ok := collections.(boardSharingApplication)
		if !ok {
			return nil, newError(http.StatusServiceUnavailable, "unavailable", "Board sharing is unavailable")
		}
		return service, nil
	}
	list := func(adminOnly bool) pageOperation[services.BoardSharing] {
		return func(r *http.Request) ([]services.BoardSharing, Pagination, int, error) {
			user, err := principal(r)
			if err != nil {
				return nil, Pagination{}, 0, err
			}
			page, err := ParsePage(r)
			if err != nil {
				return nil, page, 0, err
			}
			service, err := application()
			if err != nil {
				return nil, page, 0, err
			}
			workspace, err := optionalPositiveQueryID(r, "workspace_id")
			if err != nil {
				return nil, page, 0, err
			}
			owner, err := optionalPositiveQueryID(r, "owner_id")
			if err != nil {
				return nil, page, 0, err
			}
			organisation, err := optionalPositiveQueryID(r, "organisation_id")
			if err != nil {
				return nil, page, 0, err
			}
			items, total, err := service.ListBoardPublications(user.ID, services.BoardPublicationFilter{
				SystemAdminOnly: adminOnly,
				Query:           r.URL.Query().Get("q"), State: r.URL.Query().Get("state"), AccessMode: r.URL.Query().Get("access_mode"),
				WorkspaceID: workspace, OwnerID: owner, OrganisationID: organisation, Global: r.URL.Query().Get("scope") == "global", Limit: page.PageSize, Offset: page.Offset,
			})
			return items, page, total, collectionError(err)
		}
	}
	b.Page("/board-publications", AuthAuthenticated, []string{"collections:read"}, list(false))
	b.Page("/admin/board-publications", AuthAuthenticated, []string{"collections:read"}, list(true))
	b.Read("/board-publications/{collection_id}", AuthAuthenticated, []string{"collections:read"}, func(r *http.Request) (*services.BoardSharing, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		id, err := pathID(r, "collection_id")
		if err != nil {
			return nil, err
		}
		service, err := application()
		if err != nil {
			return nil, err
		}
		result, err := service.GetBoardSharing(user.ID, id)
		return result, collectionError(err)
	})
	b.JSON(http.MethodPut, "/board-publications/{collection_id}", http.StatusOK, false, AuthAuthenticated, []string{"collections:write"}, func(r *http.Request, input services.BoardSharingSave) (*services.BoardSharing, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		id, err := pathID(r, "collection_id")
		if err != nil {
			return nil, err
		}
		service, err := application()
		if err != nil {
			return nil, err
		}
		result, err := service.SaveBoardSharing(auditActor(r, user), id, input)
		return result, collectionError(err)
	})
	b.Read("/board-publications/{collection_id}/history", AuthAuthenticated, []string{"collections:read"}, func(r *http.Request) ([]services.BoardSharingEvent, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		id, err := pathID(r, "collection_id")
		if err != nil {
			return nil, err
		}
		service, err := application()
		if err != nil {
			return nil, err
		}
		result, err := service.BoardSharingHistory(user.ID, id)
		return result, collectionError(err)
	})
	for _, operation := range []string{"revoke", "allow-republish"} {
		b.JSON(http.MethodPost, "/board-publications/{collection_id}/"+operation, http.StatusOK, false, AuthAuthenticated, []string{"collections:write"}, func(r *http.Request, input struct {
			Revision int    `json:"revision"`
			Reason   string `json:"reason"`
		}) (*services.BoardSharing, error) {
			user, err := principal(r)
			if err != nil {
				return nil, err
			}
			id, err := pathID(r, "collection_id")
			if err != nil {
				return nil, err
			}
			service, err := application()
			if err != nil {
				return nil, err
			}
			result, err := service.RevokeBoardSharing(auditActor(r, user), id, input.Revision, input.Reason, operation == "allow-republish")
			return result, collectionError(err)
		})
	}
}
