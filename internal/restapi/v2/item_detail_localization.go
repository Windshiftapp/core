package v2

import (
	"net/http"

	"windshift/internal/services"
)

func localizeItemDetail(r *http.Request, localizer objectLocalizer, result services.ItemDetailSummary, err error) (services.ItemDetailSummary, error) {
	if err != nil {
		return services.ItemDetailSummary{}, itemError(err)
	}
	for _, catalog := range []struct {
		objectType string
		value      any
	}{
		{"priority", &result.Priorities},
		{"link_type", &result.LinkTypes},
	} {
		if err := localizeCatalog(r, localizer, catalog.objectType, catalog.value); err != nil {
			return services.ItemDetailSummary{}, err
		}
	}
	if result.CurrentHierarchyLevel != nil {
		if err := localizeCatalog(r, localizer, "hierarchy_level", result.CurrentHierarchyLevel); err != nil {
			return services.ItemDetailSummary{}, err
		}
	}
	return result, nil
}
