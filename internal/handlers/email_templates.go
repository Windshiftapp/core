package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/emailutil"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

// EmailTemplateHandler exposes admin CRUD for built-in transactional email
// templates (magic_link, email_verification, invitation, notification_batch).
// Only update + read + preview are exposed — the rows are seeded by the
// system and admins are not allowed to add or remove them.
type EmailTemplateHandler struct {
	*BaseHandler
	repo *repository.EmailTemplateRepository
}

// NewEmailTemplateHandler creates a new email template handler.
func NewEmailTemplateHandler(db database.Database) *EmailTemplateHandler {
	return &EmailTemplateHandler{
		BaseHandler: NewBaseHandler(db),
		repo:        repository.NewEmailTemplateRepository(db),
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

	if req.Subject == "" || req.HTMLBody == "" {
		respondValidationError(w, r, "subject and html_body are required")
		return
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
		logAudit(h.db, r, user, logger.ActionEmailTemplateUpdate, logger.ResourceEmailTemplate, &idCopy, updated.Name)
	}

	respondJSONOK(w, updated)
}

// previewRequest carries the template sources to render plus the name of a
// sample-data preset to pull canned values from.
type previewRequest struct {
	Subject  string `json:"subject"`
	HTMLBody string `json:"html_body"`
	TextBody string `json:"text_body"`
	Name     string `json:"name"`
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

	data := emailutil.SampleData(req.Name)

	htmlOut, textOut, err := emailutil.RenderTemplates(req.HTMLBody, req.TextBody, data)
	if err != nil {
		respondValidationError(w, r, "template render error: "+err.Error())
		return
	}
	subjectOut, _, err := emailutil.RenderTemplates(req.Subject, req.Subject, data)
	if err != nil {
		respondValidationError(w, r, "subject render error: "+err.Error())
		return
	}

	resp := previewResponse{Subject: subjectOut, HTMLBody: htmlOut, TextBody: textOut}
	respondJSONOK(w, resp)
}
