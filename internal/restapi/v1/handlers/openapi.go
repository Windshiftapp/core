package handlers

import (
	"net/http"

	apispec "windshift/api"
)

// OpenAPISpecJSON serves the embedded OpenAPI 3.0 spec as JSON.
//
// Public route — discovery documents are meant to be readable before
// the caller has a bearer token. Individual endpoints described by the
// spec still enforce auth and scope checks normally.
//
// @Summary      OpenAPI 3.0 spec (JSON)
// @Description  Returns the OpenAPI 3.0 description of this API. Public; no auth required.
// @Tags         meta
// @Produce      json
// @Success      200  {string}  string  "OpenAPI 3.0 document"
// @Router       /openapi.json [get]
func OpenAPISpecJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(apispec.SpecJSON)
}
