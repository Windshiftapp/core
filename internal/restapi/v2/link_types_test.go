package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"windshift/internal/contextkeys"
	"windshift/internal/models"
	"windshift/internal/services"
)

type linkTypeApplicationStub struct {
	linkApplication
	catalogMutationApplication
	item models.LinkType
}

func (s linkTypeApplicationStub) ListLinkTypes(bool) ([]models.LinkType, error) {
	return []models.LinkType{s.item}, nil
}

func (s linkTypeApplicationStub) GetLinkType(int) (*models.LinkType, error) {
	return &s.item, nil
}

func (s linkTypeApplicationStub) CreateLinkType(services.AuditActor, models.LinkType) (*models.LinkType, error) {
	return &s.item, nil
}

func (s linkTypeApplicationStub) PatchLinkType(services.AuditActor, int, services.LinkTypePatch) (*models.LinkType, error) {
	return &s.item, nil
}

type linkTypeLocalizerStub struct{ t *testing.T }

func (s linkTypeLocalizerStub) LocalizeResponse(_ context.Context, locale, objectType string, value any) error {
	if locale != "ru-RU" || objectType != "link_type" {
		s.t.Fatalf("unexpected localization target %s / %s", locale, objectType)
	}
	localize := func(item *models.LinkType) {
		item.DisplayForwardLabel = "Реализует"
		item.DisplayReverseLabel = "Реализуется через"
	}
	switch items := value.(type) {
	case *models.LinkType:
		localize(items)
	case *[]models.LinkType:
		for i := range *items {
			localize(&(*items)[i])
		}
	default:
		s.t.Fatalf("unexpected localization value %T", value)
	}
	return nil
}

func TestLinkTypeResponsesLocalizeDirectionLabels(t *testing.T) {
	app := linkTypeApplicationStub{item: models.LinkType{
		ID: 2, Name: "Implements", ForwardLabel: "implements", ReverseLabel: "implemented by",
	}}
	localizer := linkTypeLocalizerStub{t}
	for name, operation := range map[string]readOperation[models.LinkType]{
		"list": func(r *http.Request) (models.LinkType, error) {
			items, err := listLinkTypes(app, localizer)(r)
			if err != nil || len(items) == 0 {
				return models.LinkType{}, err
			}
			return items[0], nil
		},
		"get": getLinkType(app, localizer),
		"create": func(r *http.Request) (models.LinkType, error) {
			return createLinkType(app, localizer)(r, app.item)
		},
		"patch": func(r *http.Request) (models.LinkType, error) {
			return patchLinkType(app, localizer)(r, linkTypePatchRequest{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/link-types/2", nil)
			req.Header.Set("Accept-Language", "ru-RU, en;q=0.8")
			req.SetPathValue("link_type_id", "2")
			req = req.WithContext(context.WithValue(req.Context(), contextkeys.User, &models.User{ID: 1}))
			item, err := operation(req)
			if err != nil {
				t.Fatal(err)
			}
			if item.DisplayForwardLabel != "Реализует" || item.DisplayReverseLabel != "Реализуется через" {
				t.Fatalf("missing localized direction labels: %+v", item)
			}
			if item.ForwardLabel != "implements" || item.ReverseLabel != "implemented by" {
				t.Fatalf("canonical direction labels changed: %+v", item)
			}
		})
	}
}
