package handlers

import (
	"errors"
	"net/http"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

// SiteTitleSettingKey is the system_settings key holding the instance-wide
// product name shown in the sidebar and on pre-auth surfaces.
const SiteTitleSettingKey = "site_title"

// DefaultSiteTitle is returned when the site_title setting is absent.
const DefaultSiteTitle = "Windshift"

// GeneralSettingsHandler owns instance-wide general configuration. The site
// title is its first field; the surface is meant to grow.
type GeneralSettingsHandler struct {
	settings *repository.SystemSettingRepository
	themes   *repository.ThemeRepository
	auditor  *logger.Auditor
}

// NewGeneralSettingsHandler creates a GeneralSettingsHandler.
func NewGeneralSettingsHandler(settings *repository.SystemSettingRepository, themes *repository.ThemeRepository, auditor *logger.Auditor) *GeneralSettingsHandler {
	return &GeneralSettingsHandler{settings: settings, themes: themes, auditor: auditor}
}

// GeneralSettings is the admin read/write shape for general configuration.
type GeneralSettings struct {
	SiteTitle string `json:"site_title"`
}

// BrandingResponse is the public, render-facing projection of instance
// branding. It carries only logos, layout, and the title.
type BrandingResponse struct {
	SiteTitle       string `json:"site_title"`
	BrandLayout     string `json:"brand_layout"`
	LogoURL         string `json:"logo_url,omitempty"`
	LogoURLDark     string `json:"logo_url_dark,omitempty"`
	LogoFullURL     string `json:"logo_full_url,omitempty"`
	LogoFullURLDark string `json:"logo_full_url_dark,omitempty"`
}

// siteTitle reads the persisted title, falling back to the product default.
func (h *GeneralSettingsHandler) siteTitle() string {
	if value, ok, err := h.settings.GetValue(SiteTitleSettingKey); err == nil && ok {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return DefaultSiteTitle
}

// GetSettings returns the admin general configuration.
func (h *GeneralSettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	respondJSONOK(w, GeneralSettings{SiteTitle: h.siteTitle()})
}

// UpdateSettings persists the admin general configuration.
func (h *GeneralSettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	settings, ok := decodeJSON[GeneralSettings](w, r)
	if !ok {
		return
	}
	sanitize.Apply(&settings.SiteTitle, sanitize.PlainTextField)
	settings.SiteTitle = strings.TrimSpace(settings.SiteTitle)
	if settings.SiteTitle == "" {
		settings.SiteTitle = DefaultSiteTitle
	}

	if err := h.settings.Upsert(
		SiteTitleSettingKey, settings.SiteTitle,
		"string", "Instance site title", "branding",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionGeneralSettingsUpdate, logger.ResourceGeneralSettings, nil, "site_title")
	}
	respondJSONOK(w, settings)
}

// GetBranding returns the public branding projection used by the login,
// boot, and authenticated shell surfaces. It exposes no admin data.
func (h *GeneralSettingsHandler) GetBranding(w http.ResponseWriter, r *http.Request) {
	response := BrandingResponse{
		SiteTitle:   h.siteTitle(),
		BrandLayout: models.BrandLayoutMarkText,
	}

	theme, err := h.themes.GetActive()
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		respondInternalError(w, r, err)
		return
	}
	if err == nil {
		response.LogoURL = theme.LogoURL
		response.LogoURLDark = theme.LogoURLDark
		response.LogoFullURL = theme.LogoFullURL
		response.LogoFullURLDark = theme.LogoFullURLDark
		if models.IsValidBrandLayout(theme.BrandLayout) {
			response.BrandLayout = theme.BrandLayout
		}
	}

	respondJSONOK(w, response)
}
