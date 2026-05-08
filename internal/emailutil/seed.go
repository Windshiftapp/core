package emailutil

import (
	"fmt"

	"windshift/internal/database"
)

// SeedTemplates inserts the built-in transactional email templates if rows
// with their names don't already exist. Idempotent — safe to re-run on every
// boot, and won't overwrite admin edits.
//
// Lives here (not in internal/database) so the database layer doesn't need
// to import the email domain. Run from the server bootstrap after
// database.Initialize succeeds; the WHERE-NOT-EXISTS guard makes the
// per-template inserts safe outside the init transaction.
func SeedTemplates(db database.Database) error {
	return database.WithTx(db, func(tx database.Tx) error {
		for _, t := range DefaultTemplates() {
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
	})
}
