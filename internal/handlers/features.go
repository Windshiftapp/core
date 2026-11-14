package handlers

import (
	"net/http"

	"windshift/internal/operatorcontrols"
	"windshift/internal/plugins"
)

// FeaturesHandler handles the feature discovery endpoint.
type FeaturesHandler struct {
	pluginManager *plugins.Manager
	sshEnabled    bool
	controls      *operatorcontrols.Controls
}

// NewFeaturesHandler creates a new features handler.
func NewFeaturesHandler(pluginManager *plugins.Manager, sshEnabled bool, controls ...*operatorcontrols.Controls) *FeaturesHandler {
	h := &FeaturesHandler{pluginManager: pluginManager, sshEnabled: sshEnabled}
	if len(controls) > 0 {
		h.controls = controls[0]
	}
	return h
}

// Snapshot returns feature discovery data for callers that need to compose it
// into a larger response without making an internal HTTP request.
func (h *FeaturesHandler) Snapshot() FeaturesResponse {
	resp := FeaturesResponse{
		Edition:          "community",
		SAMLAvailable:    true,
		LDAPAvailable:    h.controls.Allowed(operatorcontrols.LDAP),
		SCIMAvailable:    true,
		SSHAvailable:     h.sshEnabled,
		Plugins:          make([]string, 0),
		Capabilities:     make([]string, 0),
		OperatorControls: h.controls.Snapshot(),
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
	OperatorControls operatorcontrols.Snapshot `json:"operator_controls"`
	Edition          string                    `json:"edition"`
	SAMLAvailable    bool                      `json:"saml_available"`
	LDAPAvailable    bool                      `json:"ldap_available"`
	SCIMAvailable    bool                      `json:"scim_available"`
	SSHAvailable     bool                      `json:"ssh_available"`
	Plugins          []string                  `json:"plugins"`
	Capabilities     []string                  `json:"capabilities"`
}

// GetFeatures handles GET /api/features (public, no auth required).
func (h *FeaturesHandler) GetFeatures(w http.ResponseWriter, r *http.Request) {
	respondJSONOK(w, h.Snapshot())
}
