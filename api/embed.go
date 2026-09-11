// Package api embeds the generated OpenAPI spec for Windshift's public REST
// API and the separately maintained session-auth Agent Studio contract so the
// running binary can serve both without a filesystem dependency.
package api

import _ "embed"

// SpecJSON is the OpenAPI 3.0 spec serialized as JSON.
//
//go:embed openapi.json
var SpecJSON []byte

// V2SpecJSON is the canonical dual-mount API v2 contract.
//
//go:embed openapi-v2.json
var V2SpecJSON []byte

// AgentStudioSpecYAML documents the cookie/session-header /api surface used by
// Agent Studio. It is deliberately separate from the bearer-token v1 spec.
//
//go:embed agent-studio.openapi.yaml
var AgentStudioSpecYAML []byte
