package handlers

import "windshift/internal/restapi"

// Type aliases so swag annotations of the form `{object} handlers.ErrorResponse`
// resolve from any file in this package, without each handler having to import
// windshift/internal/restapi. The handler bodies still import restapi where
// they need to construct API errors (NewAPIError, ErrXxx, RespondError) —
// these aliases exist purely so the swag annotation parser can resolve the
// response-shape references regardless of the per-file import list. New
// handlers added to this package can reference handlers.ErrorResponse /
// handlers.PaginatedResponse in their @Failure / @Success comments without
// touching imports.
//
// Side effect: the generated OpenAPI spec now exposes these schemas as
// `handlers.ErrorResponse` / `handlers.PaginatedResponse` instead of the
// previous `restapi.ErrorResponse` / `restapi.PaginatedResponse`. Wire shape
// is unchanged; only the component name differs.
type (
	ErrorResponse     = restapi.ErrorResponse
	PaginatedResponse = restapi.PaginatedResponse
)
