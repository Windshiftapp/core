package handlers

import (
	"errors"
	"net/http"
	"strings"

	"golang.org/x/text/currency"

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

// CurrencySettingKey is the system_settings key holding the instance-wide ISO
// 4217 currency code used to format monetary values such as time-tracking rates.
const CurrencySettingKey = "currency"

// DefaultCurrency is the ISO 4217 code used when the currency setting is absent
// or no longer recognized.
const DefaultCurrency = "USD"

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
	Currency  string `json:"currency"`
}

// BrandingResponse is the public, render-facing projection of instance
// branding. It carries only logos, layout, title, and the display currency.
type BrandingResponse struct {
	SiteTitle       string `json:"site_title"`
	Currency        string `json:"currency"`
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

// currency reads the persisted ISO 4217 code, falling back to the default when
// the setting is absent or no longer recognized.
func (h *GeneralSettingsHandler) currency() string {
	if value, ok, err := h.settings.GetValue(CurrencySettingKey); err == nil && ok {
		if normalized, err := normalizeCurrency(value); err == nil {
			return normalized
		}
	}
	return DefaultCurrency
}

// normalizeCurrency uppercases and validates an ISO 4217 code.
func normalizeCurrency(code string) (string, error) {
	unit, err := currency.ParseISO(strings.TrimSpace(code))
	if err != nil {
		return "", err
	}
	return unit.String(), nil
}

// GetSettings returns the admin general configuration.
func (h *GeneralSettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	respondJSONOK(w, GeneralSettings{SiteTitle: h.siteTitle(), Currency: h.currency()})
}

// UpdateSettings persists the admin general configuration. An omitted currency
// keeps the stored value; a present currency must be a valid ISO 4217 code.
func (h *GeneralSettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	settings, fields, ok := decodeJSONWithFields[GeneralSettings](w, r)
	if !ok {
		return
	}
	sanitize.Apply(&settings.SiteTitle, sanitize.PlainTextField)
	settings.SiteTitle = strings.TrimSpace(settings.SiteTitle)
	if settings.SiteTitle == "" {
		settings.SiteTitle = DefaultSiteTitle
	}

	currentCurrency := h.currency()
	incomingCurrency := settings.Currency
	settings.Currency = currentCurrency
	_, currencyPresent := fields["currency"]
	if currencyPresent {
		normalized, err := normalizeCurrency(incomingCurrency)
		if err != nil {
			respondValidationError(w, r, "currency must be a valid ISO 4217 code")
			return
		}
		settings.Currency = normalized
	}

	if err := h.settings.Upsert(
		SiteTitleSettingKey, settings.SiteTitle,
		"string", "Instance site title", "branding",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if currencyPresent {
		if err := h.settings.Upsert(
			CurrencySettingKey, settings.Currency,
			"string", "Instance currency (ISO 4217)", "branding",
		); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		resourceName := "site_title"
		if currencyPresent {
			resourceName = "site_title,currency"
		}
		h.auditor.Log(r, currentUser, logger.ActionGeneralSettingsUpdate, logger.ResourceGeneralSettings, nil, resourceName)
	}
	respondJSONOK(w, settings)
}

// GetBranding returns the public branding projection used by the login,
// boot, and authenticated shell surfaces. It exposes no admin data.
func (h *GeneralSettingsHandler) GetBranding(w http.ResponseWriter, r *http.Request) {
	response := BrandingResponse{
		SiteTitle:   h.siteTitle(),
		Currency:    h.currency(),
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
