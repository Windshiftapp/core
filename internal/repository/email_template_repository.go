package repository

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// EmailTemplateRepository persists and reads admin-editable email templates.
// Reads go through a small in-process TTL cache so the SMTP path doesn't hit
// the DB on every send.
type EmailTemplateRepository struct {
	db database.Database

	mu       sync.RWMutex
	cache    map[string]cachedTemplate
	cacheTTL time.Duration
	clockNow func() time.Time
}

type cachedTemplate struct {
	tmpl      *models.EmailTemplate
	fetchedAt time.Time
}

// NewEmailTemplateRepository creates a new repository with a 60-second cache.
func NewEmailTemplateRepository(db database.Database) *EmailTemplateRepository {
	return &EmailTemplateRepository{
		db:       db,
		cache:    make(map[string]cachedTemplate),
		cacheTTL: 60 * time.Second,
		clockNow: time.Now,
	}
}

const emailTemplateColumns = "id, name, COALESCE(subject, '') AS subject, COALESCE(content, '') AS content, COALESCE(text_body, '') AS text_body, COALESCE(description, '') AS description, COALESCE(is_active, true) AS is_active, COALESCE(is_system, false) AS is_system, created_at, updated_at"

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
	if err := row.Scan(&t.ID, &t.Name, &t.Subject, &t.HTMLBody, &t.TextBody, &t.Description, &t.IsActive, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("fetch email template %q: %w", name, err)
	}
	return &t, nil
}

// List returns all email templates ordered by name. Used by the admin UI.
func (r *EmailTemplateRepository) List() ([]models.EmailTemplate, error) {
	rows, err := r.db.Query("SELECT " + emailTemplateColumns + " FROM notification_templates ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list email templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var templates []models.EmailTemplate
	for rows.Next() {
		var t models.EmailTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Subject, &t.HTMLBody, &t.TextBody, &t.Description, &t.IsActive, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email template: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email templates: %w", err)
	}
	return templates, nil
}

// GetByID returns a single template by primary key.
func (r *EmailTemplateRepository) GetByID(id int) (*models.EmailTemplate, error) {
	row := r.db.QueryRow("SELECT "+emailTemplateColumns+" FROM notification_templates WHERE id = ?", id)
	var t models.EmailTemplate
	if err := row.Scan(&t.ID, &t.Name, &t.Subject, &t.HTMLBody, &t.TextBody, &t.Description, &t.IsActive, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get email template %d: %w", id, err)
	}
	return &t, nil
}

// Update writes editable fields back to the row, refreshing updated_at and
// invalidating the cache entry for that name.
func (r *EmailTemplateRepository) Update(id int, subject, htmlBody, textBody, description string, isActive bool) (*models.EmailTemplate, error) {
	now := time.Now()
	res, err := r.db.ExecWrite(
		`UPDATE notification_templates
		 SET subject = ?, content = ?, text_body = ?, description = ?, is_active = ?, updated_at = ?
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

	updated, err := r.GetByID(id)
	if err != nil {
		return nil, err
	}

	r.invalidateCache(updated.Name)
	return updated, nil
}

func (r *EmailTemplateRepository) invalidateCache(name string) {
	r.mu.Lock()
	delete(r.cache, name)
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
