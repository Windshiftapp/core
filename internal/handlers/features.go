package handlers

import (
	"net/http"

	"windshift/internal/plugins"
)

// FeaturesHandler handles the feature discovery endpoint.
type FeaturesHandler struct {
	pluginManager *plugins.Manager
	sshEnabled    bool
}

// NewFeaturesHandler creates a new features handler.
func NewFeaturesHandler(pluginManager *plugins.Manager, sshEnabled bool) *FeaturesHandler {
	return &FeaturesHandler{pluginManager: pluginManager, sshEnabled: sshEnabled}
}

// Snapshot returns feature discovery data for callers that need to compose it
// into a larger response without making an internal HTTP request.
func (h *FeaturesHandler) Snapshot() FeaturesResponse {
	resp := FeaturesResponse{
		Edition:       "community",
		SAMLAvailable: true,
		LDAPAvailable: true,
		SCIMAvailable: true,
		SSHAvailable:  h.sshEnabled,
		Plugins:       make([]string, 0),
		Capabilities:  make([]string, 0),
	}

	if h.pluginManager != nil {
		for _, p := range h.pluginManager.ListPlugins() {
			resp.Plugins = append(resp.Plugins, p.Manifest.Name)
		}
		if caps := h.pluginManager.GetCapabilities(); len(caps) > 0 {
			resp.Capabilities = caps
		}
	}
	return resp
}

// FeaturesResponse represents the available features and installed plugins.
type FeaturesResponse struct {
	Edition       string   `json:"edition"`
	SAMLAvailable bool     `json:"saml_available"`
	LDAPAvailable bool     `json:"ldap_available"`
	SCIMAvailable bool     `json:"scim_available"`
	SSHAvailable  bool     `json:"ssh_available"`
	Plugins       []string `json:"plugins"`
	Capabilities  []string `json:"capabilities"`
}

// GetFeatures handles GET /api/features (public, no auth required).
func (h *FeaturesHandler) GetFeatures(w http.ResponseWriter, r *http.Request) {
	respondJSONOK(w, h.Snapshot())
}
