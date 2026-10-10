package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// IntakeRepository persists email intake routing (WI-1644).
type IntakeRepository struct {
	db database.Database
}

func NewIntakeRepository(db database.Database) *IntakeRepository {
	return &IntakeRepository{db: db}
}

const intakeSelectColumns = `
	i.id, i.mailbox_id, i.folder, i.workspace_id, i.portal_channel_id,
	i.request_type_id, i.item_type_id, i.rate_limit_per_hour,
	i.processing_disposition, i.status, i.status_reason, i.created_at, i.updated_at,
	COALESCE(c.name, '') AS mailbox_name,
	COALESCE(s.last_uid, 0), COALESCE(s.uid_validity, 0), s.updated_at,
	COALESCE((
		SELECT COUNT(*) FROM email_message_tracking emt
		WHERE emt.intake_id = i.id AND emt.rate_limited_at IS NOT NULL
	), 0)`

const intakeFromJoins = `
	FROM intakes i
	LEFT JOIN channels c ON c.id = i.mailbox_id
	LEFT JOIN email_intake_state s ON s.intake_id = i.id`

func scanIntake(scanner interface {
	Scan(dest ...any) error
}) (models.Intake, error) {
	var in models.Intake
	var portalChannelID, requestTypeID, itemTypeID, rateLimit sql.NullInt64
	var lastPolledAt sql.NullTime
	if err := scanner.Scan(
		&in.ID, &in.MailboxID, &in.Folder, &in.WorkspaceID, &portalChannelID,
		&requestTypeID, &itemTypeID, &rateLimit,
		&in.ProcessingDisposition, &in.Status, &in.StatusReason, &in.CreatedAt, &in.UpdatedAt,
		&in.MailboxName,
		&in.LastUID, &in.UIDValidity, &lastPolledAt, &in.RateLimitedCount,
	); err != nil {
		return in, err
	}
	if portalChannelID.Valid {
		v := int(portalChannelID.Int64)
		in.PortalChannelID = &v
	}
	if requestTypeID.Valid {
		v := int(requestTypeID.Int64)
		in.RequestTypeID = &v
	}
	if itemTypeID.Valid {
		v := int(itemTypeID.Int64)
		in.ItemTypeID = &v
	}
	if rateLimit.Valid {
		v := int(rateLimit.Int64)
		in.RateLimitPerHour = &v
	}
	if lastPolledAt.Valid {
		t := lastPolledAt.Time
		in.LastPolledAt = &t
	}
	return in, nil
}

// ListByMailbox returns every intake reading from a mailbox, with the
// mailbox's monitored address attached for display.
func (r *IntakeRepository) ListByMailbox(ctx context.Context, mailboxID int) ([]models.Intake, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT`+intakeSelectColumns+intakeFromJoins+`
		WHERE i.mailbox_id = ?
		ORDER BY i.folder, i.id`, mailboxID)
	if err != nil {
		return nil, fmt.Errorf("list intakes for mailbox %d: %w", mailboxID, err)
	}
	defer func() { _ = rows.Close() }()
	intakes, err := scanIntakes(rows)
	if err != nil {
		return nil, err
	}
	if err := r.attachMailboxAddress(ctx, mailboxID, intakes); err != nil {
		return nil, err
	}
	return intakes, nil
}

// attachMailboxAddress fills in the monitored address every intake on the
// mailbox shares. The address lives on the mailbox channel config, so it is
// resolved once rather than joined per row.
func (r *IntakeRepository) attachMailboxAddress(ctx context.Context, mailboxID int, intakes []models.Intake) error {
	if len(intakes) == 0 {
		return nil
	}
	var configJSON string
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(config, '{}') FROM channels WHERE id = ? AND type IN ('email', 'imap')`,
		mailboxID,
	).Scan(&configJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load mailbox %d config: %w", mailboxID, err)
	}
	var cfg models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		// A malformed legacy config must not hide the intake list.
		return nil //nolint:nilerr // display-only address
	}
	address := strings.TrimSpace(cfg.EmailOAuthEmail)
	if address == "" {
		address = strings.TrimSpace(cfg.IMAPUsername)
	}
	for i := range intakes {
		intakes[i].MailboxAddress = address
	}
	return nil
}

// ListEnabledForMailbox returns the enabled intakes a poll should process.
func (r *IntakeRepository) ListEnabledForMailbox(ctx context.Context, mailboxID int) ([]models.Intake, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT`+intakeSelectColumns+intakeFromJoins+`
		WHERE i.mailbox_id = ? AND i.status = ?
		ORDER BY i.folder, i.id`, mailboxID, models.IntakeStatusEnabled)
	if err != nil {
		return nil, fmt.Errorf("list enabled intakes for mailbox %d: %w", mailboxID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntakes(rows)
}

// ListByPortal returns the intakes that expose requests to a portal.
func (r *IntakeRepository) ListByPortal(ctx context.Context, portalChannelID int) ([]models.Intake, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT`+intakeSelectColumns+intakeFromJoins+`
		WHERE i.portal_channel_id = ?
		ORDER BY i.folder, i.id`, portalChannelID)
	if err != nil {
		return nil, fmt.Errorf("list intakes for portal %d: %w", portalChannelID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntakes(rows)
}

// ListByWorkspace returns the intakes routing into a workspace.
func (r *IntakeRepository) ListByWorkspace(ctx context.Context, workspaceID int) ([]models.Intake, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT`+intakeSelectColumns+intakeFromJoins+`
		WHERE i.workspace_id = ?
		ORDER BY i.folder, i.id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list intakes for workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntakes(rows)
}

func scanIntakes(rows *sql.Rows) ([]models.Intake, error) {
	var out []models.Intake
	for rows.Next() {
		in, scanErr := scanIntake(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan intake: %w", scanErr)
		}
		out = append(out, in)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate intakes: %w", err)
	}
	if out == nil {
		out = []models.Intake{}
	}
	return out, nil
}

// GetByID returns an intake, or ErrNotFound.
func (r *IntakeRepository) GetByID(ctx context.Context, id int) (*models.Intake, error) {
	row := r.db.QueryRowContext(ctx, `SELECT`+intakeSelectColumns+intakeFromJoins+`
		WHERE i.id = ?`, id)
	in, err := scanIntake(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get intake %d: %w", id, err)
	}
	return &in, nil
}

// MailboxFolderTaken reports whether another intake already reads this folder
// on the mailbox. excludeID > 0 excludes that row.
func (r *IntakeRepository) MailboxFolderTaken(ctx context.Context, mailboxID int, folder string, excludeID int) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM intakes
			WHERE mailbox_id = ? AND folder = ? AND id != ?
		)
	`, mailboxID, folder, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check intake folder: %w", err)
	}
	return exists, nil
}

// Create inserts an intake and returns its id.
func (r *IntakeRepository) Create(ctx context.Context, in *models.Intake) (int, error) {
	now := time.Now()
	folder := in.Folder
	if folder == "" {
		folder = "INBOX"
	}
	status := in.Status
	if status == "" {
		status = models.IntakeStatusEnabled
	}
	var id int
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO intakes (
			mailbox_id, folder, workspace_id, portal_channel_id, request_type_id, item_type_id,
			rate_limit_per_hour, processing_disposition, status, status_reason, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, in.MailboxID, folder, in.WorkspaceID, in.PortalChannelID, in.RequestTypeID, in.ItemTypeID,
		in.RateLimitPerHour, in.ProcessingDisposition, status, in.StatusReason, now, now).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("create intake: %w", err)
	}
	return id, nil
}

// Update replaces the editable fields of an intake.
func (r *IntakeRepository) Update(ctx context.Context, in *models.Intake) error {
	folder := in.Folder
	if folder == "" {
		folder = "INBOX"
	}
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE intakes SET
			folder = ?, workspace_id = ?, portal_channel_id = ?, request_type_id = ?, item_type_id = ?,
			rate_limit_per_hour = ?, processing_disposition = ?, status = ?, status_reason = ?, updated_at = ?
		WHERE id = ?
	`, folder, in.WorkspaceID, in.PortalChannelID, in.RequestTypeID, in.ItemTypeID,
		in.RateLimitPerHour, in.ProcessingDisposition, in.Status, in.StatusReason, time.Now(), in.ID)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update intake %d: %w", in.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkNeedsAttention parks intakes whose routing a configuration change
// invalidated, keeping their config for repair. reasons maps intake id to the
// explanation shown in the admin UI.
func (r *IntakeRepository) MarkNeedsAttention(ctx context.Context, reasons map[int]string) error {
	if len(reasons) == 0 {
		return nil
	}
	return database.WithTx(r.db, func(tx database.Tx) error {
		for id, reason := range reasons {
			if _, err := tx.Exec(`
				UPDATE intakes SET status = ?, status_reason = ?, updated_at = ?
				WHERE id = ?
			`, models.IntakeStatusNeedsAttention, reason, time.Now(), id); err != nil {
				return fmt.Errorf("mark intake %d needs attention: %w", id, err)
			}
		}
		return nil
	})
}

// DeleteByWorkspaceTx removes every intake routing into a deleted workspace so
// the mailbox stops polling a target that no longer exists.
func (r *IntakeRepository) DeleteByWorkspaceTx(tx database.Tx, workspaceID int) error {
	if _, err := tx.Exec(
		`DELETE FROM intakes WHERE workspace_id = ?`, workspaceID,
	); err != nil {
		return fmt.Errorf("delete intakes for workspace %d: %w", workspaceID, err)
	}
	return nil
}

// DeleteByPortalTx removes every intake exposing requests to a deleted portal.
func (r *IntakeRepository) DeleteByPortalTx(tx database.Tx, portalChannelID int) error {
	if _, err := tx.Exec(
		`DELETE FROM intakes WHERE portal_channel_id = ?`, portalChannelID,
	); err != nil {
		return fmt.Errorf("delete intakes for portal %d: %w", portalChannelID, err)
	}
	return nil
}

// Delete removes an intake and its watermark.
func (r *IntakeRepository) Delete(ctx context.Context, id int) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		if _, err := tx.Exec(`DELETE FROM email_intake_state WHERE intake_id = ?`, id); err != nil {
			return fmt.Errorf("delete intake state %d: %w", id, err)
		}
		res, err := tx.Exec(`DELETE FROM intakes WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("delete intake %d: %w", id, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
}
