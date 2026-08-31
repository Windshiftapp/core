package restapi

import (
	"net/http"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/objecttranslation"
	"windshift/internal/services"
)

// Deps carries the dependencies v1 (and future versions) need so we can add
// services without churning every call site. New fields go at the end with
// nil-safe defaults so unrelated callers compile unchanged.
type Deps struct {
	Mux               *http.ServeMux
	DB                database.Database
	TokenManager      *auth.TokenManager
	PermissionService *services.PermissionService
	// ActionService is the optional cache-invalidation hook for the actions
	// surface. v1 falls back to "next periodic refresh" when nil, which is
	// fine for cold-start tooling but worth wiring for production.
	ActionService *services.ActionService
	// AttachmentPath is the base directory where attachment blobs are stored.
	// Empty when attachments are disabled — the v1 download route falls back
	// to a not-enabled response in that case.
	AttachmentPath string
	// ItemLinkService is the fully-wired link orchestration service
	// (asset/page permission checkers, notification + action emitters)
	// shared with the cookie-auth handler. Required for the v1 link
	// surface; the v1 router falls back to a bare service if nil so old
	// embedders that haven't wired this yet still work for everything
	// EXCEPT link endpoints.
	ItemLinkService *services.ItemLinkService
	// CommentService is the fully-wired comment pipeline (notifications,
	// mentions, webhooks, activity tracking, email replies, the coding-agent
	// @mention trigger). Shared with the cookie-auth handler so comments
	// created through the bearer-token surface (MCP, REST API, the coding
	// agent) trigger the same notifications as comments created in the web
	// UI — without this wiring a comment posted by the coding agent never
	// notified the item creator/assignee (WI-434). The v1 router constructs
	// a bare service when nil so embedders that haven't wired this yet still
	// persist comments, but those comments won't fire notifications.
	CommentService *services.CommentService
	// ItemCreationService is the fully wired user-facing creation pipeline.
	// Sharing it with the cookie handler keeps normalization, validation,
	// persistence, and committed-item side effects identical across surfaces.
	ItemCreationService *services.ItemCreationService
	// ItemUpdateApplicationService is the fully wired user-facing update
	// pipeline. Sharing it keeps activity tracking, cache invalidation,
	// committed-item events, and mention processing identical across surfaces.
	ItemUpdateApplicationService *services.ItemUpdateApplicationService
	// ItemDeletionApplicationService is the fully wired destructive pipeline.
	// It owns the exact item.delete permission, cascade result, cache
	// invalidation, and committed delete event shared by REST v1 and MCP.
	ItemDeletionApplicationService *services.ItemDeletionApplicationService
	// PageApplicationService is the permission-aware mutation/audit pipeline
	// shared by cookie, REST v1, and MCP page operations.
	PageApplicationService *services.PageApplicationService
	// PageDiagramService owns Page-scoped diagram attachment mutations.
	PageDiagramService *services.PageDiagramService
	// AssetPermissionService gates the v1 asset surface against the
	// per-set role model. Shared with the cookie-auth handler so both
	// surfaces consult one role-check pipeline. The v1 router constructs
	// a fresh instance when nil so embedders that haven't wired this yet
	// still serve asset routes correctly.
	AssetPermissionService *services.AssetPermissionService
	// AssetService owns the asset mutation pipeline: repo writes, audit
	// emission, automation-event emission, and custom-field schema
	// validation. Shared with the cookie-auth handler so the two surfaces
	// emit identical audit + automation events. The v1 router constructs
	// a fresh instance when nil so embedders boot, but a nil-AssetService
	// path produces orphaned audit rows (no automation hook); production
	// should always pass the cookie-auth handler's shared instance.
	AssetService *services.AssetService
	// ObjectTranslationService is shared with the cookie-auth admin surface so
	// writes invalidate the same translation cache for every transport.
	ObjectTranslationService *objecttranslation.Service
}

// SetupRoutesFunc is a function type for setting up v1 routes
// This breaks the import cycle by allowing main.go to wire the dependency
type SetupRoutesFunc func(deps Deps)

// SetupRoutes registers all REST API routes under /rest/api
// The v1Setup function is called to register v1 routes on the provided mux
func SetupRoutes(deps Deps, v1Setup SetupRoutesFunc) {
	// Register v1 routes (they handle their own prefix /rest/api/v1)
	if v1Setup != nil {
		v1Setup(deps)
	}

	// Future: v2 routes
	// v2Setup(deps)
}
