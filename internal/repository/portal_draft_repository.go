package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
)

// PortalRequestDraft is the persisted in-progress state of a multi-step portal
// request form. Exactly one of PortalCustomerID / UserID is set, identifying
// whoever owns the draft (magic-link customer or internal user).
type PortalRequestDraft struct {
	ID                int
	ChannelID         int
	RequestTypeID     int
	PortalCustomerID  *int
	UserID            *int
	Title             string
	Description       string
	CustomFieldValues map[string]interface{}
	CurrentStep       int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PortalRequestDraftSummary is a list-view row joining the draft with its
// request type so the Drafts UI can render an icon, name, and progress chip
// without follow-up queries.
type PortalRequestDraftSummary struct {
	ID              int       `json:"id"`
	ChannelID       int       `json:"channel_id"`
	RequestTypeID   int       `json:"request_type_id"`
	RequestTypeName string    `json:"request_type_name"`
	RequestTypeIcon string    `json:"request_type_icon"`
	Title           string    `json:"title"`
	CurrentStep     int       `json:"current_step"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DraftIdentity is whichever auth principal owns a draft. Callers populate
// exactly one of the two fields; the repository routes queries accordingly.
type DraftIdentity struct {
	PortalCustomerID *int
	UserID           *int
}

// PortalRequestDraftPayload carries the editable fields of a draft.
type PortalRequestDraftPayload struct {
	Title             string
	Description       string
	CustomFieldValues map[string]interface{}
	CurrentStep       int
}

// PortalDraftRepository persists portal request form drafts. There is at most
// one draft per (identity, request_type), enforced by partial unique indexes.
type PortalDraftRepository struct {
	db database.Database
}

// NewPortalDraftRepository creates a PortalDraftRepository.
func NewPortalDraftRepository(db database.Database) *PortalDraftRepository {
	return &PortalDraftRepository{db: db}
}

const portalDraftColumns = "id, channel_id, request_type_id, portal_customer_id, user_id, title, description, custom_field_values, current_step, created_at, updated_at"

// Upsert inserts or updates the single draft row for (identity, request_type).
// Implemented as SELECT-then-UPDATE-or-INSERT rather than ON CONFLICT to keep
// the partial-index targeting portable across SQLite and Postgres.
func (r *PortalDraftRepository) Upsert(
	ctx context.Context,
	channelID, requestTypeID int,
	identity DraftIdentity,
	payload PortalRequestDraftPayload,
) (*PortalRequestDraft, error) {
	if identity.PortalCustomerID == nil && identity.UserID == nil {
		return nil, fmt.Errorf("portal draft upsert: identity is required")
	}

	customJSON, err := marshalDraftCustomFields(payload.CustomFieldValues)
	if err != nil {
		return nil, fmt.Errorf("portal draft upsert: marshal custom fields: %w", err)
	}

	now := time.Now()

	existingID, err := r.lookupID(ctx, requestTypeID, identity)
	if err != nil {
		return nil, err
	}

	if existingID > 0 {
		if _, err := r.db.ExecWriteContext(ctx, `
			UPDATE portal_request_drafts
			SET channel_id = ?, title = ?, description = ?, custom_field_values = ?,
			    current_step = ?, updated_at = ?
			WHERE id = ?
		`, channelID, payload.Title, payload.Description, customJSON, payload.CurrentStep, now, existingID); err != nil {
			return nil, fmt.Errorf("update draft %d: %w", existingID, err)
		}
		return r.getByID(ctx, existingID)
	}

	res, err := r.db.ExecWriteContext(ctx, `
		INSERT INTO portal_request_drafts
		    (channel_id, request_type_id, portal_customer_id, user_id, title, description, custom_field_values, current_step, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		channelID, requestTypeID,
		draftNullableInt(identity.PortalCustomerID), draftNullableInt(identity.UserID),
		payload.Title, payload.Description, customJSON, payload.CurrentStep,
		now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert draft: %w", err)
	}
	newID, err := res.LastInsertId()
	if err != nil || newID == 0 {
		// Postgres lib/pq doesn't support LastInsertId; fall back to a lookup.
		id, lerr := r.lookupID(ctx, requestTypeID, identity)
		if lerr != nil {
			return nil, lerr
		}
		newID = int64(id)
	}
	return r.getByID(ctx, int(newID))
}

// GetByIdentity returns the draft for the given identity + request type, or
// (nil, nil) if none exists.
func (r *PortalDraftRepository) GetByIdentity(
	ctx context.Context,
	requestTypeID int,
	identity DraftIdentity,
) (*PortalRequestDraft, error) {
	id, err := r.lookupID(ctx, requestTypeID, identity)
	if err != nil {
		return nil, err
	}
	if id == 0 {
		return nil, nil
	}
	return r.getByID(ctx, id)
}

// ListByIdentityForChannel returns every draft the identity owns within this
// channel (one per request type), newest-first.
func (r *PortalDraftRepository) ListByIdentityForChannel(
	ctx context.Context,
	channelID int,
	identity DraftIdentity,
) ([]PortalRequestDraftSummary, error) {
	identityCol, identityVal, err := identityColumn(identity)
	if err != nil {
		return nil, err
	}

	q := fmt.Sprintf(`
		SELECT d.id, d.channel_id, d.request_type_id,
		       rt.name, COALESCE(rt.icon, ''),
		       d.title, d.current_step, d.updated_at
		FROM portal_request_drafts d
		JOIN request_types rt ON rt.id = d.request_type_id
		WHERE d.channel_id = ? AND d.%s = ?
		ORDER BY d.updated_at DESC
	`, identityCol)

	rows, err := r.db.QueryContext(ctx, q, channelID, identityVal)
	if err != nil {
		return nil, fmt.Errorf("list portal drafts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PortalRequestDraftSummary, 0)
	for rows.Next() {
		var s PortalRequestDraftSummary
		if err := rows.Scan(
			&s.ID, &s.ChannelID, &s.RequestTypeID,
			&s.RequestTypeName, &s.RequestTypeIcon,
			&s.Title, &s.CurrentStep, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan portal draft summary: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteByIdentity removes the draft for (identity, request_type). Returns
// ErrNotFound if there was no row to delete.
func (r *PortalDraftRepository) DeleteByIdentity(
	ctx context.Context,
	requestTypeID int,
	identity DraftIdentity,
) error {
	identityCol, identityVal, err := identityColumn(identity)
	if err != nil {
		return err
	}

	q := fmt.Sprintf(`DELETE FROM portal_request_drafts WHERE request_type_id = ? AND %s = ?`, identityCol)
	res, err := r.db.ExecWriteContext(ctx, q, requestTypeID, identityVal)
	if err != nil {
		return fmt.Errorf("delete portal draft: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PortalDraftRepository) lookupID(ctx context.Context, requestTypeID int, identity DraftIdentity) (int, error) {
	identityCol, identityVal, err := identityColumn(identity)
	if err != nil {
		return 0, err
	}
	q := fmt.Sprintf(`SELECT id FROM portal_request_drafts WHERE request_type_id = ? AND %s = ?`, identityCol)
	var id int
	err = r.db.QueryRowContext(ctx, q, requestTypeID, identityVal).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("lookup portal draft id: %w", err)
	}
	return id, nil
}

func (r *PortalDraftRepository) getByID(ctx context.Context, id int) (*PortalRequestDraft, error) {
	var d PortalRequestDraft
	var customJSON sql.NullString
	var pcID, userID sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		"SELECT "+portalDraftColumns+" FROM portal_request_drafts WHERE id = ?", id,
	).Scan(
		&d.ID, &d.ChannelID, &d.RequestTypeID,
		&pcID, &userID,
		&d.Title, &d.Description, &customJSON, &d.CurrentStep,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load portal draft %d: %w", id, err)
	}
	if pcID.Valid {
		v := int(pcID.Int64)
		d.PortalCustomerID = &v
	}
	if userID.Valid {
		v := int(userID.Int64)
		d.UserID = &v
	}
	d.CustomFieldValues = unmarshalDraftCustomFields(customJSON)
	return &d, nil
}

func identityColumn(identity DraftIdentity) (column string, value int, err error) {
	if identity.PortalCustomerID != nil && identity.UserID != nil {
		return "", 0, fmt.Errorf("portal draft identity: only one of portal_customer_id / user_id may be set")
	}
	if identity.PortalCustomerID != nil {
		return "portal_customer_id", *identity.PortalCustomerID, nil
	}
	if identity.UserID != nil {
		return "user_id", *identity.UserID, nil
	}
	return "", 0, fmt.Errorf("portal draft identity: one of portal_customer_id / user_id is required")
}

func marshalDraftCustomFields(v map[string]interface{}) (interface{}, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func unmarshalDraftCustomFields(n sql.NullString) map[string]interface{} {
	if !n.Valid || n.String == "" {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(n.String), &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func draftNullableInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}
