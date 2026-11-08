package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"windshift/internal/emailutil"
	"windshift/internal/i18n"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

// EmailTemplateHandler exposes admin CRUD for built-in transactional email
// templates (magic_link, email_verification, invitation, mention,
// notification_batch, portal_reply, approval_requested, intake_confirmation).
// Only update + read + preview are exposed — the rows are seeded by the
// system and admins are not allowed to add or remove them.
type EmailTemplateHandler struct {
	repo    *repository.EmailTemplateRepository
	auditor *logger.Auditor
}

// NewEmailTemplateHandler creates a new email template handler.
func NewEmailTemplateHandler(repo *repository.EmailTemplateRepository, auditor *logger.Auditor) *EmailTemplateHandler {
	return &EmailTemplateHandler{
		repo:    repo,
		auditor: auditor,
	}
}

// List handles GET /email-templates.
func (h *EmailTemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	templates, err := h.repo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if templates == nil {
		templates = []models.EmailTemplate{}
	}
	respondJSONOK(w, templates)
}

// Get handles GET /email-templates/{id}.
func (h *EmailTemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	template, err := h.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "email template")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, template)
}

// emailTemplateUpdateRequest is the editable subset of an email template.
type emailTemplateUpdateRequest struct {
	Subject     string `json:"subject"`
	HTMLBody    string `json:"html_body"`
	TextBody    string `json:"text_body"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
}

// Update handles PUT /email-templates/{id}.
func (h *EmailTemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[emailTemplateUpdateRequest](w, r)
	if !ok {
		return
	}
	// Sanitize rendered subject/description, but preserve template bodies: this
	// admin-only trusted surface intentionally authors outbound HTML/text.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Subject, Policy: sanitize.PlainTextField, Label: "Subject"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	if req.Subject == "" || req.HTMLBody == "" {
		respondValidationError(w, r, "subject and html_body are required")
		return
	}

	existing, err := h.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "email template")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	// Reject a save that would break the email family: a template that cannot
	// render against its own sample data would fail every send of that type.
	if emailutil.HasSampleData(existing.Name) {
		if _, _, _, err := renderEmailTemplate(existing.Name, req.Subject, req.HTMLBody, req.TextBody); err != nil {
			respondValidationError(w, r, err.Error())
			return
		}
	}

	updated, err := h.repo.Update(id, req.Subject, req.HTMLBody, req.TextBody, req.Description, req.IsActive)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "email template")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if user := utils.GetCurrentUser(r); user != nil {
		idCopy := updated.ID
		h.auditor.Log(r, user, logger.ActionEmailTemplateUpdate, logger.ResourceEmailTemplate, &idCopy, updated.Name)
	}

	respondJSONOK(w, struct {
		*models.EmailTemplate
		Warnings []string `json:"warnings,omitempty"`
	}{updated, warnings})
}

// previewRequest carries the template sources to render plus the name of a
// sample-data preset to pull canned values from.
type previewRequest struct {
	Subject  string `json:"subject"`
	HTMLBody string `json:"html_body"`
	TextBody string `json:"text_body"`
	Name     string `json:"name"`
	// Locale renders the preview in a specific locale (the `t` helper bound to
	// it). Empty uses the default locale.
	Locale string `json:"locale"`
}

// previewResponse mirrors what the preview UI renders into the iframe.
type previewResponse struct {
	Subject  string `json:"subject"`
	HTMLBody string `json:"html_body"`
	TextBody string `json:"text_body"`
}

// Preview handles POST /email-templates/preview. It renders the supplied
// template sources against canned sample data so admins can see the result
// before saving.
func (h *EmailTemplateHandler) Preview(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[previewRequest](w, r)
	if !ok {
		return
	}
	// Same rationale as Update: Subject + Name are user-facing labels and
	// run through sanitize; HTMLBody + TextBody are template sources the
	// admin is composing and are passed through unchanged so the preview
	// reflects what would actually be sent.
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Subject, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField},
	)

	subjectOut, htmlOut, textOut, err := renderEmailTemplateLocale(req.Name, req.Locale, req.Subject, req.HTMLBody, req.TextBody)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	resp := previewResponse{Subject: subjectOut, HTMLBody: htmlOut, TextBody: textOut}
	respondJSONOK(w, resp)
}

// UpdateTranslation handles PUT /email-templates/{id}/translations/{locale}.
// It stores a per-locale override of the template's subject/body; the base row
// and the localized built-in default remain the fallback for other locales.
func (h *EmailTemplateHandler) UpdateTranslation(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	locale := r.PathValue("locale")
	if !i18n.IsSupported(locale) {
		respondValidationError(w, r, "Unsupported locale")
		return
	}

	req, ok := decodeJSON[emailTemplateUpdateRequest](w, r)
	if !ok {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Subject, Policy: sanitize.PlainTextField, Label: "Subject"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
	)
	if req.Subject == "" || req.HTMLBody == "" {
		respondValidationError(w, r, "subject and html_body are required")
		return
	}

	existing, err := h.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "email template")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	// Reject a translation that cannot render against the sample data, so a
	// broken override cannot take down that locale's email family.
	if emailutil.HasSampleData(existing.Name) {
		if _, _, _, err := renderEmailTemplateLocale(existing.Name, locale, req.Subject, req.HTMLBody, req.TextBody); err != nil {
			respondValidationError(w, r, err.Error())
			return
		}
	}

	updated, err := h.repo.UpsertTranslation(id, locale, req.Subject, req.HTMLBody, req.TextBody)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "email template")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if user := utils.GetCurrentUser(r); user != nil {
		idCopy := existing.ID
		h.auditor.Log(r, user, logger.ActionEmailTemplateUpdate, logger.ResourceEmailTemplate, &idCopy, existing.Name+":"+updated.Locale)
	}

	respondJSONOK(w, struct {
		*models.EmailTemplateTranslation
		Warnings []string `json:"warnings,omitempty"`
	}{updated, warnings})
}

// DeleteTranslation handles DELETE /email-templates/{id}/translations/{locale}.
// Removing an override reverts that locale to the base row and then the
// localized built-in default.
func (h *EmailTemplateHandler) DeleteTranslation(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	locale := r.PathValue("locale")
	if !i18n.IsSupported(locale) {
		respondValidationError(w, r, "Unsupported locale")
		return
	}

	existing, err := h.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "email template")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if err := h.repo.DeleteTranslation(id, locale); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "email template translation")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if user := utils.GetCurrentUser(r); user != nil {
		idCopy := existing.ID
		h.auditor.Log(r, user, logger.ActionEmailTemplateUpdate, logger.ResourceEmailTemplate, &idCopy, existing.Name+":"+i18n.Normalize(locale))
	}

	respondJSONOK(w, map[string]any{"deleted": true})
}

// renderEmailTemplate renders the supplied sources against the sample data for
// the named template using the default locale. It is the single validation
// path shared by admin saves and previews so both agree on what a valid
// template is.
func renderEmailTemplate(name, subject, htmlBody, textBody string) (subjectOut, htmlOut, textOut string, err error) {
	return renderEmailTemplateLocale(name, i18n.Default, subject, htmlBody, textBody)
}

// renderEmailTemplateLocale is renderEmailTemplate with the `t` helper bound to
// locale, so per-locale overrides validate against the copy they will render.
func renderEmailTemplateLocale(name, locale, subject, htmlBody, textBody string) (subjectOut, htmlOut, textOut string, err error) {
	data := emailutil.SampleData(name)
	_, subjectOut, err = emailutil.RenderTemplatesLocale(locale, subject, subject, data)
	if err != nil {
		return "", "", "", fmt.Errorf("subject render error: %w", err)
	}
	enriched := emailutil.EnrichWithSubjectLocale(data, subjectOut, locale)
	htmlOut, textOut, err = emailutil.RenderTemplatesLocale(locale, htmlBody, textBody, enriched)
	if err != nil {
		return "", "", "", fmt.Errorf("template render error: %w", err)
	}
	return subjectOut, htmlOut, textOut, nil
}
