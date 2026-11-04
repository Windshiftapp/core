// Package v1 OpenAPI metadata.
//
// Deprecated: this file describes the superseded v1 API. New integrations must
// target v2 (/rest/api/v2); the canonical contract is core/api/openapi-v2.json.
//
// This file hosts swaggo directives for the API as a whole. Route annotations
// live beside the v1, health, and metrics handlers. The generated spec is
// committed to core/api/openapi.json and re-emitted by `make openapi`.

// @title                       Windshift HTTP API
// @version                     1
// @description                 Public API reference for the deprecated v1 API. REST operations use the `/rest/api/v1` base path and bearer tokens (`Authorization: Bearer crw_*`). Superseded by v2 (`/rest/api/v2`, contract at `/rest/api/v2/openapi.json`); new integrations must not use v1. Operational health and metrics endpoints are unauthenticated and use root-level paths.
// @BasePath                    /rest/api/v1
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 API bearer token in the form `Bearer crw_*`. Token scopes are checked per route.
package v1
