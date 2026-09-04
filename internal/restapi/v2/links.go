package v2

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/models"
	"windshift/internal/services"
)

func registerLinkRoutes(builder *routeBuilder, links linkApplication, catalogs catalogMutationApplication) {
	builder.Read("/link-types", AuthAuthenticated, []string{"links:read"}, listLinkTypes(links))
	builder.JSON(http.MethodPost, "/link-types", http.StatusCreated, false, AuthAuthenticated, []string{"links:write"}, createLinkType(catalogs))
	builder.Read("/link-types/{link_type_id}", AuthAuthenticated, []string{"links:read"}, getLinkType(catalogs))
	builder.JSON(http.MethodPatch, "/link-types/{link_type_id}", http.StatusOK, true, AuthAuthenticated, []string{"links:write"}, patchLinkType(catalogs))
	builder.Command(http.MethodDelete, "/link-types/{link_type_id}", AuthAuthenticated, []string{"links:write"}, deleteLinkType(catalogs))
	builder.Read("/items/{item_id}/links", AuthAuthenticated, []string{"items:read"}, listEntityLinks(links, "item", "item_id"))
	builder.Read("/pages/{page_id}/links", AuthAuthenticated, []string{"pages:read"}, listEntityLinks(links, "page", "page_id"))
	builder.Read("/test-cases/{test_case_id}/links", AuthAuthenticated, []string{"tests:read"}, listEntityLinks(links, "test_case", "test_case_id"))
	builder.Page("/links/batch", AuthAuthenticated, []string{"items:read"}, listLinksBatch(links))
	builder.JSON(http.MethodPost, "/links", http.StatusCreated, false, AuthAuthenticated, []string{"links:write"}, createLink(links))
	builder.Command(http.MethodDelete, "/links/{link_id}", AuthAuthenticated, []string{"links:write"}, deleteLink(links))
	builder.Read("/links/search", AuthAuthenticated, []string{"links:read"}, searchLinks(links))
	builder.Read("/items/{item_id}/fields/{field_id}/links", AuthAuthenticated, []string{"items:read"}, listFieldLinks(links))
}

func getLinkType(catalogs catalogMutationApplication) readOperation[models.LinkType] {
	return func(r *http.Request) (models.LinkType, error) {
		id, err := pathID(r, "link_type_id")
		if err != nil {
			return models.LinkType{}, err
		}
		item, err := catalogs.GetLinkType(id)
		if err != nil {
			return models.LinkType{}, catalogMutationError(err)
		}
		return *item, nil
	}
}

func createLinkType(catalogs catalogMutationApplication) jsonOperation[models.LinkType, models.LinkType] {
	return func(r *http.Request, input models.LinkType) (models.LinkType, error) {
		user, err := principal(r)
		if err != nil {
			return models.LinkType{}, err
		}
		item, err := catalogs.CreateLinkType(auditActor(r, user), input)
		if err != nil {
			return models.LinkType{}, catalogMutationError(err)
		}
		return *item, nil
	}
}

func patchLinkType(catalogs catalogMutationApplication) jsonOperation[linkTypePatchRequest, models.LinkType] {
	return func(r *http.Request, input linkTypePatchRequest) (models.LinkType, error) {
		user, id, err := catalogPrincipalAndID(r, "link_type_id")
		if err != nil {
			return models.LinkType{}, err
		}
		item, err := catalogs.PatchLinkType(auditActor(r, user), id, services.LinkTypePatch{
			Name: optionalValue(input.Name), Description: optionalValue(input.Description),
			ForwardLabel: optionalValue(input.ForwardLabel), ReverseLabel: optionalValue(input.ReverseLabel),
			Color: optionalValue(input.Color), Active: optionalValue(input.Active), AllowedEntityTypes: optionalSlice(input.AllowedEntityTypes),
		})
		if err != nil {
			return models.LinkType{}, catalogMutationError(err)
		}
		return *item, nil
	}
}

func deleteLinkType(catalogs catalogMutationApplication) commandOperation {
	return catalogDelete(catalogs, "link_type_id", func(app catalogMutationApplication, actor services.AuditActor, id int) error {
		return app.DeleteLinkType(actor, id)
	})
}

func listLinkTypes(links linkApplication) readOperation[[]models.LinkType] {
	return func(r *http.Request) ([]models.LinkType, error) {
		includeInactive := false
		if value := r.URL.Query().Get("include_inactive"); value != "" {
			var err error
			includeInactive, err = strconv.ParseBool(value)
			if err != nil {
				return nil, newError(http.StatusBadRequest, "invalid_request", "include_inactive must be a boolean")
			}
		}
		result, err := links.ListLinkTypes(includeInactive)
		return result, linkError(err)
	}
}

func listFieldLinks(links linkApplication) readOperation[[]models.ItemLink] {
	return func(r *http.Request) ([]models.ItemLink, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		itemID, err := pathID(r, "item_id")
		if err != nil {
			return nil, err
		}
		fieldID, err := pathID(r, "field_id")
		if err != nil {
			return nil, err
		}
		result, err := links.ListFieldLinks(user.ID, itemID, fieldID)
		return result, linkError(err)
	}
}

func listEntityLinks(links linkApplication, entityType, pathName string) readOperation[services.EntityLinks] {
	return func(r *http.Request) (services.EntityLinks, error) {
		user, err := principal(r)
		if err != nil {
			return services.EntityLinks{}, err
		}
		id, err := pathID(r, pathName)
		if err != nil {
			return services.EntityLinks{}, err
		}
		outgoing, incoming, err := links.ListLinksForEntityWithChecks(user.ID, entityType, id)
		return services.EntityLinks{Outgoing: outgoing, Incoming: incoming}, linkError(err)
	}
}

func listLinksBatch(links linkApplication) pageOperation[services.BatchItemLinks] {
	return func(r *http.Request) ([]services.BatchItemLinks, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		ids, err := parseLinkIDs(r.URL.Query().Get("ids"))
		if err != nil {
			return nil, page, 0, err
		}
		afterID := 0
		if raw := r.URL.Query().Get("after_id"); raw != "" {
			afterID, err = strconv.Atoi(raw)
			if err != nil || afterID < 0 {
				return nil, page, 0, newError(http.StatusBadRequest, "invalid_request", "after_id must be non-negative")
			}
		}
		sortBy := strings.TrimPrefix(r.URL.Query().Get("sort"), "-")
		sortAsc := !strings.HasPrefix(r.URL.Query().Get("sort"), "-")
		result, total, err := links.ListBatch(r.Context(), user.ID, services.BatchLinkParams{
			QLQuery: r.URL.Query().Get("ql"), ItemIDs: ids,
			Page: services.PaginationParams{Limit: page.PageSize, Offset: page.Offset}, SortBy: sortBy, SortAsc: sortAsc,
			AfterID: afterID, IncludeCustomFields: r.URL.Query().Get("include_custom_fields") == "true",
		})
		return result, page, total, linkError(err)
	}
}

func createLink(links linkApplication) jsonOperation[models.ItemLink, models.ItemLink] {
	return func(r *http.Request, input models.ItemLink) (models.ItemLink, error) {
		user, err := principal(r)
		if err != nil {
			return models.ItemLink{}, err
		}
		if input.SourceType == "" || input.SourceID <= 0 || input.TargetType == "" || input.TargetID <= 0 || (input.LinkTypeID <= 0 && input.CustomFieldID == nil) {
			return models.ItemLink{}, newError(http.StatusBadRequest, "invalid_request", "link_type_id or custom_field_id and both entity refs are required")
		}
		result, err := links.CreateManagedLink(user.ID, services.CreateItemLinkParams{
			LinkTypeID: input.LinkTypeID, SourceType: input.SourceType, SourceID: input.SourceID,
			TargetType: input.TargetType, TargetID: input.TargetID, CustomFieldID: input.CustomFieldID,
		})
		if result == nil {
			return models.ItemLink{}, linkError(err)
		}
		return *result, linkError(err)
	}
}

func deleteLink(links linkApplication) commandOperation {
	return func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		id, err := pathID(r, "link_id")
		if err != nil {
			return err
		}
		return linkError(links.DeleteLinkWithChecks(user.ID, id))
	}
}

func searchLinks(links linkApplication) readOperation[[]models.LinkableItem] {
	return func(r *http.Request) ([]models.LinkableItem, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		limit, err := parsePositiveInt(r, "limit", 20, 100)
		if err != nil {
			return nil, err
		}
		itemTypeIDs, err := parseLinkIDs(r.URL.Query().Get("item_type_ids"))
		if err != nil {
			return nil, err
		}
		result, err := links.SearchLinkable(user.ID, r.URL.Query().Get("q"), r.URL.Query().Get("type"), limit, itemTypeIDs)
		return result, linkError(err)
	}
}

func parseLinkIDs(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := map[int]struct{}{}
	result := []int{}
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			return nil, newError(http.StatusBadRequest, "invalid_request", "ids must contain positive integers")
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result, nil
}

func linkError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrLinkSelfReference), errors.Is(err, services.ErrLinkInvalidEntityType), errors.Is(err, services.ErrInvalidLinkTypeForEntities), errors.Is(err, services.ErrQLQuery):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, services.ErrLinkExists):
		return newError(http.StatusConflict, "conflict", "Link already exists")
	case errors.Is(err, services.ErrLinkNotFound), errors.Is(err, services.ErrLinkCrossWorkspacePage), services.IsEntityNotAccessible(err):
		return newError(http.StatusNotFound, "not_found", "Link was not found")
	default:
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "ids must") || strings.Contains(err.Error(), "custom field") || strings.Contains(err.Error(), "field options") || strings.Contains(err.Error(), "not allowed") || strings.Contains(err.Error(), "does not match") {
			return newError(http.StatusBadRequest, "invalid_request", err.Error())
		}
		return internalError(err)
	}
}
