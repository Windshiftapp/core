// Package v1 OpenAPI metadata.
//
// This file exists solely to host swaggo `// @...` directives that describe
// the v1 REST API as a whole. Per-route annotations live above each handler
// method in internal/restapi/v1/handlers/. The generated spec is committed
// to core/api/openapi.{yaml,json} and re-emitted by `make openapi`.

// @title                       Windshift REST API
// @version                     1
// @description                 Public REST API for Windshift work-management. Authenticate every request with a bearer token (`Authorization: Bearer crw_*`). Tokens are scoped per route — see each operation's `security` block for the required scopes.
// @BasePath                    /rest/api/v1
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 API bearer token in the form `Bearer crw_*`. Token scopes are checked per route.
package v1
