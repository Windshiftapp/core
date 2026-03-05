package handlers

import (
	"encoding/json"
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

// FeaturesResponse represents the available features and installed plugins.
type FeaturesResponse struct {
	Edition       string   `json:"edition"`
	SAMLAvailable bool     `json:"saml_available"`
	LDAPAvailable bool     `json:"ldap_available"`
	SCIMAvailable bool     `json:"scim_available"`
	SSHAvailable  bool     `json:"ssh_available"`
	Plugins       []string `json:"plugins"`
}

// GetFeatures handles GET /api/features (public, no auth required).
func (h *FeaturesHandler) GetFeatures(w http.ResponseWriter, r *http.Request) {
	resp := FeaturesResponse{
		Edition:       "community",
		SAMLAvailable: true,
		LDAPAvailable: true,
		SCIMAvailable: true,
		SSHAvailable:  h.sshEnabled,
		Plugins:       make([]string, 0),
	}

	// List installed plugin names
	if h.pluginManager != nil {
		for _, p := range h.pluginManager.ListPlugins() {
			resp.Plugins = append(resp.Plugins, p.Manifest.Name)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
