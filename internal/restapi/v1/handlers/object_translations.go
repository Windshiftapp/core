package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"windshift/internal/objecttranslation"
	"windshift/internal/restapi"
	"windshift/internal/sanitize"
)

const maxObjectTranslationResolveTargets = 500

// ObjectTranslationHandler exposes instance translation administration to API tokens.
type ObjectTranslationHandler struct {
	BaseHandler
	service *objecttranslation.Service
}

// NewObjectTranslationHandler creates a REST v1 object translation handler.
func NewObjectTranslationHandler(base BaseHandler, service *objecttranslation.Service) *ObjectTranslationHandler {
	return &ObjectTranslationHandler{BaseHandler: base, service: service}
}

// TranslationUpsertRequest replaces one localized field value.
type TranslationUpsertRequest struct {
	Value string `json:"value"`
}

// TranslationResolveRequest requests localized display values in bulk.
type TranslationResolveRequest struct {
	Locale  string                     `json:"locale"`
	Targets []objecttranslation.Target `json:"targets"`
}

// ListDefinitions handles GET /rest/api/v1/admin/object-translations/definitions.
//
// @Summary      List configurable object translation definitions
// @Description  System-admin only. Returns the allowlisted object types and translatable fields.
// @Tags         admin,object-translations
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   objecttranslation.ObjectDefinition
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Router       /admin/object-translations/definitions [get]
func (h *ObjectTranslationHandler) ListDefinitions(w http.ResponseWriter, _ *http.Request) {
	h.RespondOK(w, objecttranslation.Definitions())
}

// List handles GET /rest/api/v1/admin/object-translations/{objectType}/{objectId}.
//
// @Summary      List translations for a configurable object
// @Description  System-admin only. Returns all locale and source rows for one object.
// @Tags         admin,object-translations
// @Produce      json
// @Security     BearerAuth
// @Param        objectType  path      string  true  "Allowlisted object type"
// @Param        objectId    path      int     true  "Object ID"
// @Success      200         {array}   objecttranslation.Translation
// @Failure      400         {object}  handlers.ErrorResponse
// @Failure      401         {object}  handlers.ErrorResponse
// @Failure      403         {object}  handlers.ErrorResponse
// @Failure      404         {object}  handlers.ErrorResponse
// @Router       /admin/object-translations/{objectType}/{objectId} [get]
func (h *ObjectTranslationHandler) List(w http.ResponseWriter, r *http.Request) {
	objectID, ok := h.ParsePathID(w, r, "objectId", "object ID")
	if !ok {
		return
	}
	translations, err := h.service.List(r.Context(), r.PathValue("objectType"), objectID)
	if err != nil {
		h.respondTranslationError(w, r, err)
		return
	}
	h.RespondOK(w, translations)
}

// Upsert handles PUT /rest/api/v1/admin/object-translations/{objectType}/{objectId}/{field}/{locale}.
//
// @Summary      Create or replace an instance object translation
// @Description  System-admin only. Writes the instance source and never overwrites shipped system rows.
// @Tags         admin,object-translations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        objectType  path      string                    true  "Allowlisted object type"
// @Param        objectId    path      int                       true  "Object ID"
// @Param        field       path      string                    true  "Translatable field"
// @Param        locale      path      string                    true  "BCP 47 locale"
// @Param        body        body      TranslationUpsertRequest  true  "Translation value"
// @Success      200         {object}  objecttranslation.Translation
// @Failure      400         {object}  handlers.ErrorResponse
// @Failure      401         {object}  handlers.ErrorResponse
// @Failure      403         {object}  handlers.ErrorResponse
// @Failure      404         {object}  handlers.ErrorResponse
// @Router       /admin/object-translations/{objectType}/{objectId}/{field}/{locale} [put]
func (h *ObjectTranslationHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	objectID, ok := h.ParsePathID(w, r, "objectId", "object ID")
	if !ok {
		return
	}
	var request TranslationUpsertRequest
	if !h.DecodeBodyOrRespond(w, r, &request) {
		return
	}
	field := r.PathValue("field")
	if field == objecttranslation.FieldDescription {
		request.Value = sanitize.RichText.Sanitize(request.Value)
	} else {
		request.Value = sanitize.PlainTextField.Sanitize(request.Value)
	}
	translation, err := h.service.UpsertInstance(
		r.Context(), r.PathValue("objectType"), objectID, field, r.PathValue("locale"), request.Value,
	)
	if err != nil {
		h.respondTranslationError(w, r, err)
		return
	}
	h.RespondOK(w, translation)
}

// Delete handles DELETE /rest/api/v1/admin/object-translations/{objectType}/{objectId}/{field}/{locale}.
//
// @Summary      Delete an instance object translation
// @Description  System-admin only. Shipped system translations are unaffected.
// @Tags         admin,object-translations
// @Security     BearerAuth
// @Param        objectType  path  string  true  "Allowlisted object type"
// @Param        objectId    path  int     true  "Object ID"
// @Param        field       path  string  true  "Translatable field"
// @Param        locale      path  string  true  "BCP 47 locale"
// @Success      204
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse
// @Router       /admin/object-translations/{objectType}/{objectId}/{field}/{locale} [delete]
func (h *ObjectTranslationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	objectID, ok := h.ParsePathID(w, r, "objectId", "object ID")
	if !ok {
		return
	}
	err := h.service.DeleteInstance(
		r.Context(), r.PathValue("objectType"), objectID, r.PathValue("field"), r.PathValue("locale"),
	)
	if err != nil {
		h.respondTranslationError(w, r, err)
		return
	}
	h.RespondNoContent(w)
}

// Resolve handles POST /rest/api/v1/admin/object-translations/resolve.
//
// @Summary      Resolve configurable object display values in bulk
// @Description  System-admin only. Query count is bounded by object type and field, not target count.
// @Tags         admin,object-translations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      TranslationResolveRequest       true  "Locale and canonical targets"
// @Success      200   {array}   objecttranslation.ResolvedValue
// @Failure      400   {object}  handlers.ErrorResponse
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse
// @Router       /admin/object-translations/resolve [post]
func (h *ObjectTranslationHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	var request TranslationResolveRequest
	if !h.DecodeBodyOrRespond(w, r, &request) {
		return
	}
	if len(request.Targets) > maxObjectTranslationResolveTargets {
		h.RespondError(w, r, restapi.NewAPIError(
			http.StatusBadRequest, restapi.ErrCodeInvalidInput,
			fmt.Sprintf("targets must contain at most %d objects", maxObjectTranslationResolveTargets),
		))
		return
	}
	resolved, err := h.service.Resolve(r.Context(), request.Locale, request.Targets)
	if err != nil {
		h.respondTranslationError(w, r, err)
		return
	}
	h.RespondOK(w, resolved)
}

// FindOrphans handles GET /rest/api/v1/admin/object-translations/orphans.
//
// @Summary      Detect orphaned configurable object translations
// @Description  System-admin only. Reports corruption without mutating it.
// @Tags         admin,object-translations
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   objecttranslation.Translation
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Router       /admin/object-translations/orphans [get]
func (h *ObjectTranslationHandler) FindOrphans(w http.ResponseWriter, r *http.Request) {
	orphans, err := h.service.FindOrphans(r.Context())
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, orphans)
}

// FindCanonicalDifferences handles GET /rest/api/v1/admin/object-translations/canonical-differences.
//
// @Summary      List built-in object values needing translation review
// @Description  System-admin only. Compares canonical values with shipped base-locale copy without changing either.
// @Tags         admin,object-translations
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   objecttranslation.CanonicalDifference
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Router       /admin/object-translations/canonical-differences [get]
func (h *ObjectTranslationHandler) FindCanonicalDifferences(w http.ResponseWriter, r *http.Request) {
	differences, err := h.service.FindCanonicalDifferences(r.Context(), objecttranslation.ShippedSystemTranslations())
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, differences)
}

func (h *ObjectTranslationHandler) respondTranslationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, objecttranslation.ErrObjectNotFound), errors.Is(err, objecttranslation.ErrTranslationNotFound):
		h.RespondNotFound(w, r)
	case errors.Is(err, objecttranslation.ErrUnsupportedObjectType),
		errors.Is(err, objecttranslation.ErrUnsupportedField),
		errors.Is(err, objecttranslation.ErrInvalidLocale),
		errors.Is(err, objecttranslation.ErrInvalidValue):
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, err.Error()))
	default:
		h.RespondInternalError(w, r)
	}
}
