package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/i18n"
	"windshift/internal/models"
)

// EmailTemplateRepository persists and reads admin-editable email templates.
// Reads go through a small in-process TTL cache so the SMTP path doesn't hit
// the DB on every send.
type EmailTemplateRepository struct {
	db database.Database

	mu       sync.RWMutex
	cache    map[string]cachedTemplate
	resolved map[string]cachedResolved
	cacheTTL time.Duration
	clockNow func() time.Time
}

type cachedTemplate struct {
	tmpl      *models.EmailTemplate
	fetchedAt time.Time
}

// cachedResolved caches the effective template for a (name, locale) pair so the
// digest and transactional senders don't re-resolve translations per email.
type cachedResolved struct {
	tmpl      *models.EmailTemplate
	fromDB    bool
	fetchedAt time.Time
}

// NewEmailTemplateRepository creates a new repository with a 60-second cache.
func NewEmailTemplateRepository(db database.Database) *EmailTemplateRepository {
	return &EmailTemplateRepository{
		db:       db,
		cache:    make(map[string]cachedTemplate),
		resolved: make(map[string]cachedResolved),
		cacheTTL: 60 * time.Second,
		clockNow: time.Now,
	}
}

const emailTemplateColumns = "id, name, COALESCE(subject, '') AS subject, COALESCE(content, '') AS content, COALESCE(text_body, '') AS text_body, COALESCE(description, '') AS description, COALESCE(is_active, true) AS is_active, COALESCE(is_system, false) AS is_system, COALESCE(is_customized, false) AS is_customized, created_at, updated_at"

const emailTemplateTranslationColumns = "id, template_id, locale, COALESCE(subject, '') AS subject, COALESCE(content, '') AS content, COALESCE(text_body, '') AS text_body, created_at, updated_at"

// GetByName returns the active template with the given name. Returns
// ErrNotFound if no row exists or the row is inactive — senders treat that
// as "use the embedded fallback".
func (r *EmailTemplateRepository) GetByName(name string) (*models.EmailTemplate, error) {
	r.mu.RLock()
	if entry, ok := r.cache[name]; ok && r.clockNow().Sub(entry.fetchedAt) < r.cacheTTL {
		r.mu.RUnlock()
		if entry.tmpl == nil {
			return nil, ErrNotFound
		}
		out := *entry.tmpl
		return &out, nil
	}
	r.mu.RUnlock()

	tmpl, err := r.fetchByName(name)
	if err != nil && err != ErrNotFound {
		return nil, err
	}

	r.mu.Lock()
	r.cache[name] = cachedTemplate{tmpl: tmpl, fetchedAt: r.clockNow()}
	r.mu.Unlock()

	if tmpl == nil {
		return nil, ErrNotFound
	}
	out := *tmpl
	return &out, nil
}

func (r *EmailTemplateRepository) fetchByName(name string) (*models.EmailTemplate, error) {
	row := r.db.QueryRow(
		"SELECT "+emailTemplateColumns+" FROM notification_templates WHERE name = ? AND is_active = true",
		name,
	)
	var t models.EmailTemplate
	if err := row.Scan(&t.ID, &t.Name, &t.Subject, &t.HTMLBody, &t.TextBody, &t.Description, &t.IsActive, &t.IsSystem, &t.IsCustomized, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("fetch email template %q: %w", name, err)
	}
	return &t, nil
}

// ResolveForLocale returns the effective template for a recipient locale and
// whether it came from the database (an admin override) rather than the
// embedded default. Resolution order: an exact-locale translation, then the
// base row when an admin has customized it; otherwise the embedded default
// applies, which is already localized at render time.
func (r *EmailTemplateRepository) ResolveForLocale(name, locale string) (*models.EmailTemplate, bool, error) {
	normalized := i18n.Normalize(locale)
	cacheKey := name + "\x00" + normalized

	r.mu.RLock()
	if entry, ok := r.resolved[cacheKey]; ok && r.clockNow().Sub(entry.fetchedAt) < r.cacheTTL {
		r.mu.RUnlock()
		if entry.tmpl == nil {
			return nil, false, nil
		}
		out := *entry.tmpl
		return &out, entry.fromDB, nil
	}
	r.mu.RUnlock()

	tmpl, fromDB, err := r.resolveForLocale(name, normalized)
	if err != nil {
		return nil, false, err
	}

	r.mu.Lock()
	r.resolved[cacheKey] = cachedResolved{tmpl: tmpl, fromDB: fromDB, fetchedAt: r.clockNow()}
	r.mu.Unlock()

	if tmpl == nil {
		return nil, false, nil
	}
	out := *tmpl
	return &out, fromDB, nil
}

func (r *EmailTemplateRepository) resolveForLocale(name, locale string) (*models.EmailTemplate, bool, error) {
	base, err := r.GetByName(name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	translation, err := r.getTranslation(base.ID, locale)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	if translation != nil {
		out := *base
		out.Subject = translation.Subject
		out.HTMLBody = translation.HTMLBody
		out.TextBody = translation.TextBody
		return &out, true, nil
	}

	// An unmodified seed defers to the embedded default, which is localized.
	if base.IsCustomized {
		return base, true, nil
	}
	return nil, false, nil
}

// List returns all email templates ordered by name, each with its locale
// overrides attached. Used by the admin UI.
func (r *EmailTemplateRepository) List() ([]models.EmailTemplate, error) {
	rows, err := r.db.Query("SELECT " + emailTemplateColumns + " FROM notification_templates ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list email templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var templates []models.EmailTemplate
	for rows.Next() {
		var t models.EmailTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Subject, &t.HTMLBody, &t.TextBody, &t.Description, &t.IsActive, &t.IsSystem, &t.IsCustomized, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email template: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email templates: %w", err)
	}

	translations, err := r.listAllTranslations()
	if err != nil {
		return nil, err
	}
	byTemplate := make(map[int][]models.EmailTemplateTranslation, len(translations))
	for _, translation := range translations {
		byTemplate[translation.TemplateID] = append(byTemplate[translation.TemplateID], translation)
	}
	for i := range templates {
		templates[i].Translations = byTemplate[templates[i].ID]
	}
	return templates, nil
}

func (r *EmailTemplateRepository) listAllTranslations() ([]models.EmailTemplateTranslation, error) {
	rows, err := r.db.Query("SELECT " + emailTemplateTranslationColumns + " FROM notification_template_translations ORDER BY template_id, locale")
	if err != nil {
		return nil, fmt.Errorf("list email template translations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var translations []models.EmailTemplateTranslation
	for rows.Next() {
		var t models.EmailTemplateTranslation
		if err := rows.Scan(&t.ID, &t.TemplateID, &t.Locale, &t.Subject, &t.HTMLBody, &t.TextBody, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email template translation: %w", err)
		}
		translations = append(translations, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email template translations: %w", err)
	}
	return translations, nil
}

// GetByID returns a single template by primary key, with its locale overrides.
func (r *EmailTemplateRepository) GetByID(id int) (*models.EmailTemplate, error) {
	row := r.db.QueryRow("SELECT "+emailTemplateColumns+" FROM notification_templates WHERE id = ?", id)
	var t models.EmailTemplate
	if err := row.Scan(&t.ID, &t.Name, &t.Subject, &t.HTMLBody, &t.TextBody, &t.Description, &t.IsActive, &t.IsSystem, &t.IsCustomized, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get email template %d: %w", id, err)
	}
	translations, err := r.ListTranslations(t.ID)
	if err != nil {
		return nil, err
	}
	t.Translations = translations
	return &t, nil
}

// GetTranslation returns the admin override for one locale. The locale is
// normalized to a shipped tag; ErrNotFound means no override exists.
func (r *EmailTemplateRepository) GetTranslation(templateID int, locale string) (*models.EmailTemplateTranslation, error) {
	return r.getTranslation(templateID, i18n.Normalize(locale))
}

func (r *EmailTemplateRepository) getTranslation(templateID int, locale string) (*models.EmailTemplateTranslation, error) {
	row := r.db.QueryRow(
		"SELECT "+emailTemplateTranslationColumns+" FROM notification_template_translations WHERE template_id = ? AND locale = ?",
		templateID, locale,
	)
	var t models.EmailTemplateTranslation
	if err := row.Scan(&t.ID, &t.TemplateID, &t.Locale, &t.Subject, &t.HTMLBody, &t.TextBody, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get email template %d translation %q: %w", templateID, locale, err)
	}
	return &t, nil
}

// ListTranslations returns every locale override for one template.
func (r *EmailTemplateRepository) ListTranslations(templateID int) ([]models.EmailTemplateTranslation, error) {
	rows, err := r.db.Query(
		"SELECT "+emailTemplateTranslationColumns+" FROM notification_template_translations WHERE template_id = ? ORDER BY locale",
		templateID,
	)
	if err != nil {
		return nil, fmt.Errorf("list email template %d translations: %w", templateID, err)
	}
	defer func() { _ = rows.Close() }()

	var translations []models.EmailTemplateTranslation
	for rows.Next() {
		var t models.EmailTemplateTranslation
		if err := rows.Scan(&t.ID, &t.TemplateID, &t.Locale, &t.Subject, &t.HTMLBody, &t.TextBody, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email template translation: %w", err)
		}
		translations = append(translations, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email template translations: %w", err)
	}
	return translations, nil
}

// UpsertTranslation writes or replaces the override for one locale.
func (r *EmailTemplateRepository) UpsertTranslation(templateID int, locale, subject, htmlBody, textBody string) (*models.EmailTemplateTranslation, error) {
	normalized := i18n.Normalize(locale)
	now := time.Now()
	if _, err := r.db.ExecWrite(`
		INSERT INTO notification_template_translations (template_id, locale, subject, content, text_body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(template_id, locale) DO UPDATE SET
		    subject = excluded.subject,
		    content = excluded.content,
		    text_body = excluded.text_body,
		    updated_at = excluded.updated_at
	`, templateID, normalized, subject, htmlBody, textBody, now, now); err != nil {
		return nil, fmt.Errorf("upsert email template translation: %w", err)
	}
	r.invalidateAll()
	return r.getTranslation(templateID, normalized)
}

// DeleteTranslation removes the override for one locale, reverting to the base
// row and then the embedded default. ErrNotFound means no override existed.
func (r *EmailTemplateRepository) DeleteTranslation(templateID int, locale string) error {
	res, err := r.db.ExecWrite(
		"DELETE FROM notification_template_translations WHERE template_id = ? AND locale = ?",
		templateID, i18n.Normalize(locale),
	)
	if err != nil {
		return fmt.Errorf("delete email template translation: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	r.invalidateAll()
	return nil
}

// Update writes editable fields back to the row, refreshing updated_at and
// invalidating every cached template.
func (r *EmailTemplateRepository) Update(id int, subject, htmlBody, textBody, description string, isActive bool) (*models.EmailTemplate, error) {
	now := time.Now()
	res, err := r.db.ExecWrite(
		`UPDATE notification_templates
		 SET subject = ?, content = ?, text_body = ?, description = ?, is_active = ?, is_customized = true, updated_at = ?
		 WHERE id = ?`,
		subject, htmlBody, textBody, description, isActive, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update email template %d: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("update email template rowsAffected: %w", err)
	}
	if affected == 0 {
		return nil, ErrNotFound
	}

	r.invalidateAll()
	return r.GetByID(id)
}

func (r *EmailTemplateRepository) invalidateAll() {
	r.mu.Lock()
	r.cache = make(map[string]cachedTemplate)
	r.resolved = make(map[string]cachedResolved)
	r.mu.Unlock()
}

// SeedDefault inserts a row if no template with the given name exists yet.
// Idempotent: re-running it will not overwrite admin edits to existing rows.
func (r *EmailTemplateRepository) SeedDefault(name, description, subject, htmlBody, textBody string) error {
	now := time.Now()
	_, err := r.db.ExecWrite(
		`INSERT INTO notification_templates (name, subject, content, text_body, description, is_system, is_active, created_at, updated_at)
		 SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (SELECT 1 FROM notification_templates WHERE name = ?)`,
		name, subject, htmlBody, textBody, description, true, true, now, now, name,
	)
	if err != nil {
		return fmt.Errorf("seed email template %q: %w", name, err)
	}
	return nil
}
