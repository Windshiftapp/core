package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"windshift/internal/objecttranslation"
	"windshift/internal/sanitize"
)

const maxTranslationResolveTargets = 500

// ObjectTranslationHandler exposes administrator-managed instance translations.
type ObjectTranslationHandler struct {
	service *objecttranslation.Service
}

// NewObjectTranslationHandler creates the instance translation handler.
func NewObjectTranslationHandler(service *objecttranslation.Service) *ObjectTranslationHandler {
	return &ObjectTranslationHandler{service: service}
}

// ListDefinitions returns the allowlisted configurable object types and fields.
func (h *ObjectTranslationHandler) ListDefinitions(w http.ResponseWriter, _ *http.Request) {
	respondJSONOK(w, objecttranslation.Definitions())
}

// List returns all locale rows for one configurable object.
func (h *ObjectTranslationHandler) List(w http.ResponseWriter, r *http.Request) {
	objectID, ok := requireIDParam(w, r, "object_id")
	if !ok {
		return
	}
	translations, err := h.service.List(r.Context(), r.PathValue("object_type"), objectID)
	if err != nil {
		h.respondError(w, r, err)
		return
	}
	respondJSONOK(w, translations)
}

type objectTranslationUpsertRequest struct {
	Value string `json:"value"`
}

// Upsert creates or replaces one instance translation.
func (h *ObjectTranslationHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	objectID, ok := requireIDParam(w, r, "object_id")
	if !ok {
		return
	}
	request, ok := decodeJSON[objectTranslationUpsertRequest](w, r)
	if !ok {
		return
	}
	field := r.PathValue("field")
	if field == objecttranslation.FieldDescription {
		request.Value = sanitize.RichText.Sanitize(request.Value)
	} else {
		request.Value = sanitize.PlainTextField.Sanitize(request.Value)
	}
	translation, err := h.service.UpsertInstance(
		r.Context(), r.PathValue("object_type"), objectID, field, r.PathValue("locale"), request.Value,
	)
	if err != nil {
		h.respondError(w, r, err)
		return
	}
	respondJSONOK(w, translation)
}

// Delete removes one instance translation without affecting shipped system rows.
func (h *ObjectTranslationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	objectID, ok := requireIDParam(w, r, "object_id")
	if !ok {
		return
	}
	err := h.service.DeleteInstance(
		r.Context(), r.PathValue("object_type"), objectID, r.PathValue("field"), r.PathValue("locale"),
	)
	if err != nil {
		h.respondError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type objectTranslationResolveRequest struct {
	Locale  string                     `json:"locale"`
	Targets []objecttranslation.Target `json:"targets"`
}

// Resolve performs bounded bulk display-value resolution.
func (h *ObjectTranslationHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[objectTranslationResolveRequest](w, r)
	if !ok {
		return
	}
	if len(request.Targets) > maxTranslationResolveTargets {
		respondValidationError(w, r, fmt.Sprintf("targets must contain at most %d objects", maxTranslationResolveTargets))
		return
	}
	resolved, err := h.service.Resolve(r.Context(), request.Locale, request.Targets)
	if err != nil {
		h.respondError(w, r, err)
		return
	}
	respondJSONOK(w, resolved)
}

// FindOrphans reports corruption left by bypassing validated writes or owner cleanup.
func (h *ObjectTranslationHandler) FindOrphans(w http.ResponseWriter, r *http.Request) {
	orphans, err := h.service.FindOrphans(r.Context())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, orphans)
}

// FindCanonicalDifferences reports built-in values that need administrator review.
func (h *ObjectTranslationHandler) FindCanonicalDifferences(w http.ResponseWriter, r *http.Request) {
	differences, err := h.service.FindCanonicalDifferences(r.Context(), objecttranslation.ShippedSystemTranslations())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, differences)
}

func (h *ObjectTranslationHandler) respondError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, objecttranslation.ErrObjectNotFound), errors.Is(err, objecttranslation.ErrTranslationNotFound):
		respondNotFound(w, r, "translation")
	case errors.Is(err, objecttranslation.ErrUnsupportedObjectType),
		errors.Is(err, objecttranslation.ErrUnsupportedField),
		errors.Is(err, objecttranslation.ErrInvalidLocale),
		errors.Is(err, objecttranslation.ErrInvalidValue):
		respondValidationError(w, r, err.Error())
	default:
		respondInternalError(w, r, err)
	}
}
