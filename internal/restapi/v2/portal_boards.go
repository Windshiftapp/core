package v2

import (
	"net/http"

	"windshift/internal/handlers"
	"windshift/internal/models"
)

type portalBoardHTTP interface {
	GetPortalExperience(http.ResponseWriter, *http.Request)
	GetPortalBoard(http.ResponseWriter, *http.Request)
	GetPortalBoardItem(http.ResponseWriter, *http.Request)
	DownloadPortalBoardAttachment(http.ResponseWriter, *http.Request)
}

func registerPortalBoardRoutes(b *routeBuilder, deps Deps) {
	wrap := func(operation func(portalBoardHTTP, http.ResponseWriter, *http.Request)) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			h, ok := deps.CustomerAccess.(portalBoardHTTP)
			if !ok {
				return newError(http.StatusNotFound, "not_found", "Board was not found")
			}
			return adaptCustomerContent(w, r, func(writer http.ResponseWriter, request *http.Request) { operation(h, writer, request) })
		}
	}
	b.sessionOnly(func() {
		b.RawResponse[models.CustomerExperience](http.MethodGet, "/portal/{slug}/experience", http.StatusOK, "application/json", AuthPublic, nil,
			wrap(func(h portalBoardHTTP, w http.ResponseWriter, r *http.Request) { h.GetPortalExperience(w, r) }))
		b.RawResponse[handlers.PublicBoardResponse](http.MethodGet, "/portal/{slug}/boards/{placement_id}", http.StatusOK, "application/json", AuthPublic, nil,
			wrap(func(h portalBoardHTTP, w http.ResponseWriter, r *http.Request) { h.GetPortalBoard(w, r) }))
		b.RawResponse[handlers.PublicBoardItemResponse](http.MethodGet, "/portal/{slug}/boards/{placement_id}/items/{key}", http.StatusOK, "application/json", AuthPublic, nil,
			wrap(func(h portalBoardHTTP, w http.ResponseWriter, r *http.Request) { h.GetPortalBoardItem(w, r) }))
		b.RawHandler(http.MethodGet, "/portal/{slug}/boards/{placement_id}/attachments/{id}/download", "application/octet-stream", http.StatusOK, AuthPublic, nil,
			wrap(func(h portalBoardHTTP, w http.ResponseWriter, r *http.Request) { h.DownloadPortalBoardAttachment(w, r) }))
	})
}
