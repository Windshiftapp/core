package v2

import (
	"net/http"
	"slices"
	"strings"
)

func applyBoardContractMetadata(route *Route) {
	summary, found := map[string]string{
		"GET /board-publications":                                            "List owned board publications",
		"GET /admin/board-publications":                                      "Inventory board publications across all owners",
		"GET /board-publications/{collection_id}":                            "Read board sharing settings",
		"PUT /board-publications/{collection_id}":                            "Save board publication and audience atomically",
		"GET /board-publications/{collection_id}/history":                    "Read board sharing and placement audit history",
		"POST /board-publications/{collection_id}/revoke":                    "Revoke and lock a board publication",
		"POST /board-publications/{collection_id}/allow-republish":           "Clear a publication lock without republishing",
		"GET /channels/{channel_id}/board-placements":                        "List linked board placements and effective audiences",
		"POST /channels/{channel_id}/board-placements":                       "Link a board publication to a portal",
		"PUT /channels/{channel_id}/board-placements/{placement_id}":         "Update a portal board placement",
		"DELETE /channels/{channel_id}/board-placements/{placement_id}":      "Unlink a portal board without deleting its source",
		"GET /channels/{channel_id}/experience":                              "Read portal presentation and capability settings",
		"PUT /channels/{channel_id}/experience":                              "Save revisioned portal experience settings",
		"GET /portal/{slug}/experience":                                      "Read the policy-filtered customer experience",
		"GET /portal/{slug}/boards/{placement_id}":                           "Read a board in its linked portal context",
		"GET /portal/{slug}/boards/{placement_id}/items/{key}":               "Read externally projected board item detail",
		"GET /portal/{slug}/boards/{placement_id}/attachments/{id}/download": "Download a visible image in its portal board context",
	}[route.Method+" "+route.Path]
	if !found {
		return
	}
	route.Tag, route.Summary = "Governance", summary
	if strings.HasPrefix(route.Path, "/channels/") {
		route.Tag = "Administration"
	}
	route.Description = summary + ". Management is scoped to the collection owner or portal manager; system administrators retain cross-owner sharing governance. Sharing does not grant source editing or requester privileges. Revision conflicts return 409 without changing stored state."
	if strings.HasPrefix(route.Path, "/portal/") {
		route.Tag = "API"
		route.Description = summary + ". Customer sessions remain bound to the portal. Admission enforces manual registration and email-domain restrictions, intersecting portal policy, board baseline and placement policy on every request. Missing, disabled, revoked, paused or cross-portal placements are masked with 404; restricted admission or audience without a portal customer session returns 401, and a signed-in customer outside admission or audience returns 403. Responses are private and non-cacheable. Internal sessions are not board audience evidence."
	}
	if route.Path == "/admin/board-publications" {
		route.Description += " Requires system administrator permission; a publishing permission alone does not grant cross-owner inventory access."
	}
	for _, name := range []string{"collection_id", "channel_id", "id"} {
		if strings.Contains(route.Path, "{"+name+"}") {
			upsertParameter(route, ParameterMetadata{Name: name, In: "path", Required: true, Description: "Positive numeric resource identifier.", Schema: map[string]any{"type": "integer", "minimum": 1}})
		}
	}
	for _, name := range []string{"slug", "placement_id", "key"} {
		if strings.Contains(route.Path, "{"+name+"}") {
			upsertParameter(route, ParameterMetadata{Name: name, In: "path", Required: true, Description: "Resource key resolved within the requested context.", Schema: map[string]any{"type": "string", "minLength": 1}})
		}
	}
	if route.Path == "/board-publications" || route.Path == "/admin/board-publications" {
		upsertParameter(route, stringQuery("q", "Case-insensitive board name or slug substring."))
		for _, name := range []string{"workspace_id", "owner_id", "organisation_id"} {
			upsertParameter(route, positiveIDQuery(name, "Restricts publication metadata to this resource."))
		}
		upsertParameter(route, booleanQuery("global", "Only globally scoped collections.", false))
		upsertParameter(route, enumQuery("state", "Publication state.", "all", "enabled", "disabled", "revoked"))
		upsertParameter(route, enumQuery("access_mode", "Board baseline audience mode.", "public", "organisations"))
	}
	if route.Method == http.MethodDelete {
		parameter := positiveIDQuery("revision", "Current placement revision; stale revisions return 409.")
		parameter.Required = true
		upsertParameter(route, parameter)
	}
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		if !slices.Contains(route.DocumentedErrors, status) {
			route.DocumentedErrors = append(route.DocumentedErrors, status)
		}
	}
	if route.Method != http.MethodGet && !slices.Contains(route.DocumentedErrors, http.StatusConflict) {
		route.DocumentedErrors = append(route.DocumentedErrors, http.StatusConflict)
	}
	slices.Sort(route.DocumentedErrors)
}
