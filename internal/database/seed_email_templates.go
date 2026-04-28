package database

import (
	"fmt"

	"windshift/internal/emailutil"
)

// seedDefaultEmailTemplates inserts the built-in transactional email
// templates if rows with their names don't already exist. Idempotent — safe
// to re-run on every boot, and won't overwrite admin edits.
func seedDefaultEmailTemplates(tx Tx) error {
	for _, t := range emailutil.DefaultTemplates() {
		_, err := tx.Exec(
			`INSERT INTO notification_templates (name, subject, content, text_body, description, is_system, is_active)
			 SELECT ?, ?, ?, ?, ?, ?, ?
			 WHERE NOT EXISTS (SELECT 1 FROM notification_templates WHERE name = ?)`,
			t.Name, t.Subject, t.HTMLBody, t.TextBody, t.Description, true, true, t.Name,
		)
		if err != nil {
			return fmt.Errorf("seed email template %q: %w", t.Name, err)
		}
	}
	return nil
}
