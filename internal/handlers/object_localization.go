package handlers

import (
	"net/http"

	"windshift/internal/objecttranslation"
)

func localizeObjectResponse(w http.ResponseWriter, r *http.Request, service *objecttranslation.Service, objectType string, value any) bool {
	if service == nil {
		return true
	}
	if err := service.LocalizeResponse(r.Context(), objecttranslation.RequestLocale(r), objectType, value); err != nil {
		respondInternalError(w, r, err)
		return false
	}
	return true
}
