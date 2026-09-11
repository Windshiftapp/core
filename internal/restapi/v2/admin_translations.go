package v2

import (
	"errors"
	"net/http"

	"windshift/internal/objecttranslation"
)

type translationUpsertRequest struct {
	Value string `json:"value"`
}
type translationResolveRequest struct {
	Locale  string                     `json:"locale"`
	Targets []objecttranslation.Target `json:"targets"`
}

func registerAdminTranslationRoutes(b *routeBuilder, deps Deps) {
	read := []string{"admin:object-translations:read"}
	write := []string{"admin:object-translations:write"}
	b.Read("/admin/object-translations/definitions", AuthAuthenticated, read, func(r *http.Request) ([]objecttranslation.ObjectDefinition, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		return objecttranslation.Definitions(), nil
	})
	b.Read("/admin/object-translations/orphans", AuthAuthenticated, read, func(r *http.Request) ([]objecttranslation.Translation, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		result, err := deps.AdminTranslations.FindOrphans(r.Context())
		return result, translationError(err)
	})
	b.Read("/admin/object-translations/canonical-differences", AuthAuthenticated, read, func(r *http.Request) ([]objecttranslation.CanonicalDifference, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		result, err := deps.AdminTranslations.FindCanonicalDifferences(r.Context(), objecttranslation.ShippedSystemTranslations())
		return result, translationError(err)
	})
	b.Read("/admin/object-translations/{object_type}/{object_id}", AuthAuthenticated, read, func(r *http.Request) ([]objecttranslation.Translation, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		id, err := pathID(r, "object_id")
		if err != nil {
			return nil, err
		}
		result, err := deps.AdminTranslations.List(r.Context(), r.PathValue("object_type"), id)
		return result, translationError(err)
	})
	b.JSON(http.MethodPost, "/admin/object-translations/resolve", http.StatusOK, false, AuthAuthenticated, read, func(r *http.Request, input translationResolveRequest) ([]objecttranslation.ResolvedValue, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		result, err := deps.AdminTranslations.ResolveBounded(r.Context(), input.Locale, input.Targets)
		return result, translationError(err)
	})
	b.JSON(http.MethodPut, "/admin/object-translations/{object_type}/{object_id}/{field}/{locale}", http.StatusOK, false, AuthAuthenticated, write, func(r *http.Request, input translationUpsertRequest) (objecttranslation.Translation, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return objecttranslation.Translation{}, err
		}
		id, err := pathID(r, "object_id")
		if err != nil {
			return objecttranslation.Translation{}, err
		}
		result, err := deps.AdminTranslations.UpsertInstance(r.Context(), r.PathValue("object_type"), id, r.PathValue("field"), r.PathValue("locale"), input.Value)
		return result, translationError(err)
	})
	b.Command(http.MethodDelete, "/admin/object-translations/{object_type}/{object_id}/{field}/{locale}", AuthAuthenticated, write, func(r *http.Request) error {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return err
		}
		id, err := pathID(r, "object_id")
		if err != nil {
			return err
		}
		return translationError(deps.AdminTranslations.DeleteInstance(r.Context(), r.PathValue("object_type"), id, r.PathValue("field"), r.PathValue("locale")))
	})
}

func translationError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, objecttranslation.ErrObjectNotFound), errors.Is(err, objecttranslation.ErrTranslationNotFound):
		return newError(http.StatusNotFound, "not_found", "Object or translation was not found")
	case errors.Is(err, objecttranslation.ErrUnsupportedObjectType), errors.Is(err, objecttranslation.ErrUnsupportedField), errors.Is(err, objecttranslation.ErrInvalidLocale), errors.Is(err, objecttranslation.ErrInvalidValue), errors.Is(err, objecttranslation.ErrTooManyTargets):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
