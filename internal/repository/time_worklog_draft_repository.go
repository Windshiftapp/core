package repository

import (
	"database/sql"
	"errors"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ErrConflict reports an optimistic-concurrency conflict: the caller's version
// no longer matches the stored row. Callers map it to HTTP 409.
var ErrConflict = errors.New("conflict")

// TimeWorklogDraftRepository persists time_worklog_drafts rows: incomplete,
// user-owned time entries produced when a timer stops without booking
// information, or when a manual entry is saved incomplete. Drafts are separate
// from time_worklogs so reporting and billing never see unbooked time.
type TimeWorklogDraftRepository struct {
	db database.Database
}

// NewTimeWorklogDraftRepository creates a TimeWorklogDraftRepository.
func NewTimeWorklogDraftRepository(db database.Database) *TimeWorklogDraftRepository {
	return &TimeWorklogDraftRepository{db: db}
}

// CreateDraftInput captures the fields of a new draft. Every booking field is
// optional because the draft exists precisely because booking information is
// incomplete.
type CreateDraftInput struct {
	UserID          int
	ProjectID       *int
	ItemID          *int
	WorkspaceID     *int
	Description     string
	DateUnix        *int64
	StartTimeUnix   *int64
	EndTimeUnix     *int64
	DurationMinutes *int
	Source          string
	SourceTimerID   *int
	NowUnix         int64
}

const insertDraftQuery = `
	INSERT INTO time_worklog_drafts (
		user_id, project_id, item_id, workspace_id, description, date,
		start_time, end_time, duration_minutes, source, source_timer_id,
		created_at, updated_at, version
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
	RETURNING id`

// CreateDraft inserts a draft and returns its id. A unique violation on
// (user_id, source_timer_id) means a concurrent or retried stop already
// produced the draft; it is mapped to ErrDuplicateEntry so the caller can
// recover the existing draft.
func (r *TimeWorklogDraftRepository) CreateDraft(in CreateDraftInput) (int, error) {
	var id int64
	err := r.db.QueryRow(insertDraftQuery,
		in.UserID, in.ProjectID, in.ItemID, in.WorkspaceID, in.Description, in.DateUnix,
		in.StartTimeUnix, in.EndTimeUnix, in.DurationMinutes, in.Source, in.SourceTimerID,
		in.NowUnix, in.NowUnix,
	).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, err
	}
	return int(id), nil
}

// draftJoinedSelect returns the draft plus its linked labels. Joins are LEFT
// because every association is optional. The caller is responsible for
// redacting labels the user may no longer be allowed to see.
const draftJoinedSelect = `
	SELECT
		d.id, d.user_id, d.project_id, d.item_id, d.workspace_id, d.description,
		d.date, d.start_time, d.end_time, d.duration_minutes, d.source,
		d.source_timer_id, d.created_at, d.updated_at, d.version,
		COALESCE(tp.name, ''), COALESCE(tc.name, ''), COALESCE(tc.builtin_key, ''),
		COALESCE(i.title, ''), COALESCE(ws.key, ''), COALESCE(ws.name, '')
	FROM time_worklog_drafts d
	LEFT JOIN time_projects tp ON d.project_id = tp.id
	LEFT JOIN customer_organisations tc ON tp.customer_id = tc.id
	LEFT JOIN items i ON d.item_id = i.id
	LEFT JOIN workspaces ws ON d.workspace_id = ws.id`

func scanDraft(row interface{ Scan(...any) error }) (*models.TimeWorklogDraft, error) {
	draft := &models.TimeWorklogDraft{}
	var projectName, customerName, customerBuiltin, itemTitle, workspaceKey, workspaceName string
	var version int
	err := row.Scan(
		&draft.ID, &draft.UserID, &draft.ProjectID, &draft.ItemID, &draft.WorkspaceID, &draft.Description,
		&draft.Date, &draft.StartTime, &draft.EndTime, &draft.DurationMins, &draft.Source,
		&draft.SourceTimerID, &draft.CreatedAt, &draft.UpdatedAt, &version,
		&projectName, &customerName, &customerBuiltin, &itemTitle, &workspaceKey, &workspaceName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	draft.ProjectName = projectName
	draft.CustomerName = customerName
	draft.CustomerBuiltin = customerBuiltin
	draft.ItemTitle = itemTitle
	draft.WorkspaceKey = workspaceKey
	draft.WorkspaceName = workspaceName
	draft.Version = version
	return draft, nil
}

// GetDraft returns one draft owned by userID, joined with its labels.
func (r *TimeWorklogDraftRepository) GetDraft(id, userID int) (*models.TimeWorklogDraft, error) {
	return scanDraft(r.db.QueryRow(draftJoinedSelect+` WHERE d.id = ? AND d.user_id = ?`, id, userID))
}

// GetDraftBySourceTimer returns the draft a timer produced, if it still exists.
func (r *TimeWorklogDraftRepository) GetDraftBySourceTimer(userID, timerID int) (*models.TimeWorklogDraft, error) {
	return scanDraft(r.db.QueryRow(draftJoinedSelect+` WHERE d.user_id = ? AND d.source_timer_id = ?`, userID, timerID))
}

// ListDrafts returns all of a user's drafts, newest first.
func (r *TimeWorklogDraftRepository) ListDrafts(userID int) ([]models.TimeWorklogDraft, error) {
	rows, err := r.db.Query(draftJoinedSelect+` WHERE d.user_id = ? ORDER BY d.updated_at DESC, d.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []models.TimeWorklogDraft{}
	for rows.Next() {
		draft, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *draft)
	}
	return out, rows.Err()
}

// UpdateDraftInput fully replaces the editable fields of a draft. Nil pointers
// clear the corresponding field; the draft stays incomplete until booking.
type UpdateDraftInput struct {
	ProjectID       *int
	ItemID          *int
	Description     string
	DateUnix        *int64
	StartTimeUnix   *int64
	EndTimeUnix     *int64
	DurationMinutes *int
	NowUnix         int64
}

// UpdateDraft applies a full replace scoped to the owner and the caller's
// expected version, incrementing the version. It returns ErrConflict when the
// version is stale and ErrNotFound when the draft is missing or owned by
// someone else.
func (r *TimeWorklogDraftRepository) UpdateDraft(id, userID, expectedVersion int, in UpdateDraftInput) error {
	result, err := r.db.ExecWrite(`
		UPDATE time_worklog_drafts
		SET project_id = ?, item_id = ?, description = ?, date = ?, start_time = ?,
		    end_time = ?, duration_minutes = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND user_id = ? AND version = ?`,
		in.ProjectID, in.ItemID, in.Description, in.DateUnix, in.StartTimeUnix,
		in.EndTimeUnix, in.DurationMinutes, in.NowUnix, id, userID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return r.missingOrStale(id, userID)
	}
	return nil
}

// DeleteDraft removes an owned draft. It reports ErrNotFound when the draft is
// missing or owned by someone else.
func (r *TimeWorklogDraftRepository) DeleteDraft(id, userID int) error {
	result, err := r.db.ExecWrite(`DELETE FROM time_worklog_drafts WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// missingOrStale distinguishes a stale-version conflict from a missing/foreign
// draft for an update that matched no row.
func (r *TimeWorklogDraftRepository) missingOrStale(id, userID int) error {
	var exists int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM time_worklog_drafts WHERE id = ? AND user_id = ?`, id, userID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	return ErrConflict
}

// Booking is a recorded idempotent worklog booking.
type Booking struct {
	UserID         int
	IdempotencyKey string
	WorklogID      int
	DraftID        *int
}

// GetBooking returns the booking recorded for a caller-scoped idempotency key,
// or ErrNotFound.
func (r *TimeWorklogDraftRepository) GetBooking(userID int, key string) (*Booking, error) {
	booking := &Booking{}
	err := r.db.QueryRow(
		`SELECT user_id, idempotency_key, worklog_id, draft_id FROM time_worklog_bookings WHERE user_id = ? AND idempotency_key = ?`,
		userID, key,
	).Scan(&booking.UserID, &booking.IdempotencyKey, &booking.WorklogID, &booking.DraftID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return booking, nil
}

// BookDraft atomically materializes an owned draft as a worklog and deletes the
// draft. The delete is conditional on the expected version, so exactly one
// concurrent booking wins and a concurrent update cannot be silently lost. The
// booking is recorded in the same transaction for idempotent replay.
func (r *TimeWorklogDraftRepository) BookDraft(draftID, userID, expectedVersion int, in CreateWorklogInput, bookingKey string) (int, error) {
	var worklogID int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if err := tx.QueryRow(insertBookedWorklogQuery,
			in.ProjectID, in.UserID, in.ItemID, in.Description,
			in.DateUnix, in.StartTimeUnix, in.EndTimeUnix, in.DurationMinutes, in.NowUnix, in.NowUnix,
		).Scan(&worklogID); err != nil {
			return err
		}
		result, err := tx.ExecWrite(
			`DELETE FROM time_worklog_drafts WHERE id = ? AND user_id = ? AND version = ?`,
			draftID, userID, expectedVersion,
		)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrConflict
		}
		if bookingKey != "" {
			if _, err := tx.ExecWrite(
				`INSERT INTO time_worklog_bookings (user_id, idempotency_key, worklog_id, draft_id, created_at)
				 VALUES (?, ?, ?, ?, ?)`,
				userID, bookingKey, worklogID, draftID, in.NowUnix,
			); err != nil {
				if database.IsUniqueConstraintError(err) {
					return ErrDuplicateEntry
				}
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return worklogID, nil
}

// insertBookedWorklogQuery inserts a worklog and returns its id. It is shared by
// the draft-book and timer-stop paths so both produce the same shape.
const insertBookedWorklogQuery = `
	INSERT INTO time_worklogs (project_id, user_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING id`

// FinalizeTimerBooking atomically books a timer as a worklog, removes the timer,
// and records the idempotency key so a retried stop replays the same worklog
// instead of stopping a newer timer or duplicating booked time.
func (r *ActiveTimerRepository) FinalizeTimerBooking(timerID int, in CreateWorklogInput, bookingKey string) (int, error) {
	var worklogID int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if err := tx.QueryRow(insertBookedWorklogQuery,
			in.ProjectID, in.UserID, in.ItemID, in.Description,
			in.DateUnix, in.StartTimeUnix, in.EndTimeUnix, in.DurationMinutes, in.NowUnix, in.NowUnix,
		).Scan(&worklogID); err != nil {
			return err
		}
		result, err := tx.ExecWrite("DELETE FROM active_timers WHERE id = ?", timerID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrNotFound
		}
		if bookingKey != "" {
			if _, err := tx.ExecWrite(
				`INSERT INTO time_worklog_bookings (user_id, idempotency_key, worklog_id, draft_id, created_at)
				 VALUES (?, ?, ?, NULL, ?)`,
				in.UserID, bookingKey, worklogID, in.NowUnix,
			); err != nil {
				if database.IsUniqueConstraintError(err) {
					return ErrDuplicateEntry
				}
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return worklogID, nil
}

// FinalizeTimerDraft atomically records a draft produced by a stopping timer and
// removes the timer. The draft is returned so the caller can open the completion
// modal without a second round trip.
func (r *ActiveTimerRepository) FinalizeTimerDraft(timerID int, in CreateDraftInput) (int, error) {
	var draftID int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if err := tx.QueryRow(insertDraftQuery,
			in.UserID, in.ProjectID, in.ItemID, in.WorkspaceID, in.Description, in.DateUnix,
			in.StartTimeUnix, in.EndTimeUnix, in.DurationMinutes, in.Source, in.SourceTimerID,
			in.NowUnix, in.NowUnix,
		).Scan(&draftID); err != nil {
			if database.IsUniqueConstraintError(err) {
				return ErrDuplicateEntry
			}
			return err
		}
		result, err := tx.ExecWrite("DELETE FROM active_timers WHERE id = ?", timerID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return draftID, nil
}
