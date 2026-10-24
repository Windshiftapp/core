package services

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
)

// CustomerErasurePolicyVersion identifies the GDPR retention-aware customer
// erasure policy this execution applied. Bump when the customer data-category
// matrix changes so erasure evidence stays interpretable after the fact.
const CustomerErasurePolicyVersion = "2026-10-05.1"

// CustomerErasureInput carries the DSAR intake evidence supplied at filing time.
type CustomerErasureInput struct {
	// RequestedBy records where the erasure request came from — the data
	// subject's email address or an intake-channel reference.
	RequestedBy string
	// RequestedAt is when the controller received the request. Zero defaults
	// to execution time.
	RequestedAt time.Time
	// Notes optionally records the controller's decision context.
	Notes string
}

// CustomerErasureEvidence is the completion evidence persisted for every
// portal-customer erasure.
type CustomerErasureEvidence struct {
	CustomerID    int       `json:"customer_id"`
	RequestedBy   string    `json:"requested_by"`
	RequestedAt   time.Time `json:"requested_at"`
	ApprovedBy    int       `json:"approved_by"`
	ExecutedAt    time.Time `json:"executed_at"`
	PolicyVersion string    `json:"policy_version"`
}

// ErrCustomerAlreadyErased refuses a second erasure — Article 17 execution is
// irreversible and recorded once.
var ErrCustomerAlreadyErased = errors.New("portal customer has already been erased")

// CustomerTicketedError refuses a cleanup erasure because ticket content
// appeared for the customer (WI-1555). Reason names the marker that excluded
// them: has_items, has_comments, or has_attachments.
type CustomerTicketedError struct {
	Reason string
}

func (e *CustomerTicketedError) Error() string {
	return fmt.Sprintf("portal customer holds ticket content (%s)", e.Reason)
}

// EraseOptions tunes a single erasure execution.
type EraseOptions struct {
	// RefuseWhenTicketed is the bulk-cleanup guard (WI-1555): the erasure
	// transaction re-checks ticket content inside itself, after taking the
	// customer row lock, and refuses with *CustomerTicketedError when any
	// items, comments, or attachments exist. The explicit DSAR path keeps
	// this off — an approved erasure intentionally covers ticket holders.
	RefuseWhenTicketed bool
}

// EraseCustomer executes an Article 17 erasure against a portal customer.
// Unlike the historical hard delete, the portal_customers row is never
// deleted: it is pseudonymized (deleted-customer-N) so customer-authored
// comments, items, item history, attachments, and approval decisions stay
// interpretable under a stable pseudonym. Authentication state (sessions,
// magic links, passkeys), in-progress drafts, and channel grants are
// hard-deleted; approval-routing assignments are released; retained
// email-reply outbox rows and email tracking rows are de-identified. The
// erasure decision itself is recorded as DSAR evidence.
func EraseCustomer(db database.Database, customerID int, actor AuditActor, input CustomerErasureInput) (CustomerErasureEvidence, error) {
	return EraseCustomerWithOptions(db, customerID, actor, input, EraseOptions{})
}

// EraseCustomerWithOptions executes an erasure with the given options.
func EraseCustomerWithOptions(db database.Database, customerID int, actor AuditActor, input CustomerErasureInput, options EraseOptions) (CustomerErasureEvidence, error) {
	var evidence CustomerErasureEvidence

	if input.RequestedBy == "" {
		return evidence, NewServiceError(400, "requested_by is required (DSAR intake reference)")
	}
	requestedAt := input.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = time.Now()
	}

	var erasedAt sql.NullTime
	if err := db.QueryRow(`SELECT erased_at FROM portal_customers WHERE id = ?`, customerID).Scan(&erasedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return evidence, NewServiceError(404, "Portal customer not found")
		}
		return evidence, fmt.Errorf("load customer erasure state: %w", err)
	}
	if erasedAt.Valid {
		return evidence, ErrCustomerAlreadyErased
	}

	// Snapshot the pre-erasure identity for the audit trail; the row is
	// pseudonymized below and the original attributes survive only in the
	// audit details and the DSAR intake record.
	var priorName, priorEmail string
	var priorPhone sql.NullString
	if err := db.QueryRow(`SELECT COALESCE(name, ''), COALESCE(email, ''), phone FROM portal_customers WHERE id = ?`, customerID).
		Scan(&priorName, &priorEmail, &priorPhone); err != nil {
		return evidence, fmt.Errorf("load customer identity: %w", err)
	}

	executedAt := time.Now()
	pseudonymName := fmt.Sprintf("deleted-customer-%d", customerID)
	pseudonymEmail := fmt.Sprintf("deleted-customer-%d@erased.invalid", customerID)

	tx, err := db.Begin()
	if err != nil {
		return evidence, fmt.Errorf("failed to begin erasure transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize with other lifecycle writes on the same customer. On
	// PostgreSQL the FOR UPDATE lock also conflicts with the FOR KEY SHARE
	// locks that FK checks take on items/comments/attachments inserts, so an
	// in-flight ticket creation for this customer blocks this transaction
	// until it commits and is then seen by the footprint check below — the
	// cleanup eligibility decision and the erasure are atomic with respect
	// to content creation (WI-1555). SQLite's single writer serializes the
	// same way.
	customerLockQuery := `SELECT id FROM portal_customers WHERE id = ?`
	if db.GetDriverName() == "postgres" {
		customerLockQuery += ` FOR UPDATE`
	}
	var lockedID int
	if err := tx.QueryRow(customerLockQuery, customerID).Scan(&lockedID); err != nil {
		return evidence, fmt.Errorf("lock customer for erasure: %w", err)
	}

	// Cleanup guard: refuse when ticket content exists, checked inside the
	// transaction so a ticket that landed after selection still excludes the
	// customer (WI-1555).
	if options.RefuseWhenTicketed {
		var items, comments, attachments int
		if err := tx.QueryRow(`
			SELECT
				(SELECT COUNT(*) FROM items WHERE creator_portal_customer_id = ?),
				(SELECT COUNT(*) FROM comments WHERE portal_customer_id = ?),
				(SELECT COUNT(*) FROM attachments WHERE uploaded_by_portal_customer_id = ?)
		`, customerID, customerID, customerID).Scan(&items, &comments, &attachments); err != nil {
			return evidence, fmt.Errorf("check ticket content: %w", err)
		}
		switch {
		case items > 0:
			return evidence, &CustomerTicketedError{Reason: "has_items"}
		case comments > 0:
			return evidence, &CustomerTicketedError{Reason: "has_comments"}
		case attachments > 0:
			return evidence, &CustomerTicketedError{Reason: "has_attachments"}
		}
	}

	// Pseudonymize the row in place. The guarded WHERE makes a concurrent
	// second erasure a no-op instead of a second evidence row. Booleans are
	// bound as parameters: SQLite stores is_primary as INTEGER while Postgres
	// uses BOOLEAN and rejects integer comparisons. Deactivation rides along
	// (erasure implies access loss) so every auth path's deactivated_at
	// check covers erased customers even before their sessions are reaped.
	if res, err := tx.Exec(`
		UPDATE portal_customers SET
			name = ?,
			email = ?,
			phone = NULL,
			custom_field_values = NULL,
			user_id = NULL,
			customer_organisation_id = NULL,
			is_primary = ?,
			dismissed_passkey_prompt_at = NULL,
			deactivated_at = COALESCE(deactivated_at, ?),
			erased_at = ?,
			updated_at = ?
		WHERE id = ? AND erased_at IS NULL
	`, pseudonymName, pseudonymEmail, false, executedAt, executedAt, executedAt, customerID); err != nil {
		return evidence, fmt.Errorf("failed to pseudonymize customer: %w", err)
	} else if rows, rowsErr := res.RowsAffected(); rowsErr == nil && rows == 0 {
		// The guarded WHERE means zero rows affected can only mean a
		// concurrent erasure committed between the state check and this
		// transaction.
		return evidence, ErrCustomerAlreadyErased
	}

	// Hard-delete authentication state, drafts, and channel grants. The
	// customer row survives, so the schema-level CASCADEs never fire and each
	// category must be removed explicitly. Expired and live rows are deleted
	// alike — erasure is not scoped by validity.
	for _, stmt := range []struct {
		query string
		desc  string
	}{
		{`DELETE FROM portal_customer_sessions WHERE portal_customer_id = ?`, "portal sessions"},
		{`DELETE FROM portal_customer_magic_links WHERE portal_customer_id = ?`, "magic links"},
		{`DELETE FROM portal_webauthn_credentials WHERE portal_customer_id = ?`, "portal passkey credentials"},
		{`DELETE FROM portal_webauthn_sessions WHERE portal_customer_id = ?`, "portal passkey sessions"},
		{`DELETE FROM portal_request_drafts WHERE portal_customer_id = ?`, "portal request drafts"},
		{`DELETE FROM portal_customer_channels WHERE portal_customer_id = ?`, "portal channel grants"},
		{`DELETE FROM portal_customer_roles WHERE portal_customer_id = ?`, "portal contact roles"},
	} {
		if _, err := tx.Exec(stmt.query, customerID); err != nil {
			return evidence, fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
		}
	}

	// Release pending approval-routing assignments. The pool snapshot rows
	// keep their FK into the pseudonymized customer (history stays
	// interpretable), but the erased identity must never be an active
	// approver again. is_active is bound as a boolean for the same
	// engine-type reason as above.
	if _, err := tx.Exec(`UPDATE approval_step_approvers SET is_active = ? WHERE portal_customer_id = ? AND is_active = ?`, false, customerID, true); err != nil {
		return evidence, fmt.Errorf("failed to release approval assignments: %w", err)
	}

	// Outbound-reply rows addressed to the customer carry the erased identity
	// as the RECIPIENT (staff replies), not as the comment author, so match by
	// address. Pending rows are canceled first — mail must never go out to
	// an erased identity after erasure — then every retained row is
	// pseudonymized; the rows are third-party-visible ticket history kept
	// under the Art. 17(3)(b) exception (WI-1579).
	if priorEmail != "" {
		if _, err := tx.Exec(`
			UPDATE email_reply_outbox SET
				discarded_at = ?, last_error = 'canceled: recipient erased', updated_at = ?
			WHERE UPPER(to_email) = UPPER(?) AND delivered_at IS NULL AND discarded_at IS NULL
		`, executedAt, executedAt, priorEmail); err != nil {
			return evidence, fmt.Errorf("failed to cancel queued replies to the erased customer: %w", err)
		}
		if _, err := tx.Exec(`
			UPDATE email_reply_outbox SET
				to_email = ?, to_name = ?, updated_at = ?
			WHERE UPPER(to_email) = UPPER(?)
		`, pseudonymEmail, pseudonymName, executedAt, priorEmail); err != nil {
			return evidence, fmt.Errorf("failed to de-identify reply outbox rows: %w", err)
		}
	}

	// Email tracking rows keep the ticket thread intact but lose the erased
	// sender identity; subject lines remain part of the retained ticket
	// record. Match case-insensitively: the address is stored as it arrived
	// in the From header, which may differ in case from portal_customers.email.
	if priorEmail != "" {
		if _, err := tx.Exec(`
			UPDATE email_message_tracking SET from_email = ?, from_name = ?
			WHERE UPPER(from_email) = UPPER(?)
		`, pseudonymEmail, pseudonymName, priorEmail); err != nil {
			return evidence, fmt.Errorf("failed to de-identify email tracking rows: %w", err)
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO customer_erasure_records (portal_customer_id, requested_by, requested_at, approved_by, executed_at, policy_version, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, customerID, input.RequestedBy, requestedAt, actor.UserID, executedAt, CustomerErasurePolicyVersion, nullErasureNotes(input.Notes)); err != nil {
		return evidence, fmt.Errorf("failed to record erasure evidence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return evidence, fmt.Errorf("failed to commit erasure transaction: %w", err)
	}

	auditDetails := map[string]any{
		"requested_by":   input.RequestedBy,
		"requested_at":   requestedAt.Format(time.RFC3339),
		"policy_version": CustomerErasurePolicyVersion,
		"policy_summary": "ticket_content_retained_deidentified; sessions_magic_links_passkeys_drafts_deleted; approval_assignments_released; backups_age_out_30d",
		"prior_name":     priorName,
		"prior_email":    priorEmail,
	}
	if priorPhone.Valid && priorPhone.String != "" {
		auditDetails["prior_phone"] = priorPhone.String
	}
	_ = logger.LogAudit(db, logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		ActionType:   logger.ActionPortalCustomerErase,
		ResourceType: logger.ResourcePortalCustomer,
		ResourceID:   &customerID,
		Success:      true,
		Details:      auditDetails,
	})

	return CustomerErasureEvidence{
		CustomerID:    customerID,
		RequestedBy:   input.RequestedBy,
		RequestedAt:   requestedAt,
		ApprovedBy:    actor.UserID,
		ExecutedAt:    executedAt,
		PolicyVersion: CustomerErasurePolicyVersion,
	}, nil
}
