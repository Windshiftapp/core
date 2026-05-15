package repository

import (
	"database/sql"
	"errors"
	"time"

	"windshift/internal/database"
)

// CLIAuthRepository persists browser-to-CLI one-time auth codes.
type CLIAuthRepository struct {
	db database.Database
}

func NewCLIAuthRepository(db database.Database) *CLIAuthRepository {
	return &CLIAuthRepository{db: db}
}

type ApprovedCLIAuthCode struct {
	Code             string
	State            string
	CallbackURL      string
	Hostname         string
	AgentName        string
	RequestedScopes  string
	ApprovedByUserID int
	AgentID          int
	TokenID          int
	TokenPlaintext   string
	ExpiresAt        time.Time
}

type CLIAuthCode struct {
	ID              int64
	State           string
	Status          string
	TokenPlaintext  *string
	AgentID         *int64
	AgentName       string
	Hostname        string
	RequestedScopes string
	ExpiresAt       time.Time
	ConsumedAt      *time.Time
	ApprovedBy      *int64
	CallbackURL     string
}

func (r *CLIAuthRepository) StoreApproved(code ApprovedCLIAuthCode) error {
	_, err := r.db.Exec(`
		INSERT INTO cli_auth_codes (code, state, callback_url, hostname, agent_name, requested_scopes, status, approved_by_user_id, agent_id, token_id, token_plaintext, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, 'approved', ?, ?, ?, ?, ?)
	`, code.Code, code.State, code.CallbackURL, code.Hostname, code.AgentName, code.RequestedScopes, code.ApprovedByUserID, code.AgentID, code.TokenID, code.TokenPlaintext, code.ExpiresAt)
	return err
}

func (r *CLIAuthRepository) FindByCode(code string) (*CLIAuthCode, error) {
	var row CLIAuthCode
	var tokenText sql.NullString
	var agentID sql.NullInt64
	var consumedAt sql.NullTime
	var approvedBy sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, state, status, token_plaintext, agent_id, agent_name, hostname, requested_scopes, expires_at, consumed_at, approved_by_user_id, callback_url
		FROM cli_auth_codes
		WHERE code = ?
	`, code).Scan(&row.ID, &row.State, &row.Status, &tokenText, &agentID, &row.AgentName, &row.Hostname, &row.RequestedScopes, &row.ExpiresAt, &consumedAt, &approvedBy, &row.CallbackURL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if tokenText.Valid {
		row.TokenPlaintext = &tokenText.String
	}
	if agentID.Valid {
		row.AgentID = &agentID.Int64
	}
	if consumedAt.Valid {
		row.ConsumedAt = &consumedAt.Time
	}
	if approvedBy.Valid {
		row.ApprovedBy = &approvedBy.Int64
	}
	return &row, nil
}

func (r *CLIAuthRepository) MarkExpired(id int64) error {
	_, err := r.db.Exec(`UPDATE cli_auth_codes SET status = 'expired', token_plaintext = NULL WHERE id = ?`, id)
	return err
}

func (r *CLIAuthRepository) ConsumeApproved(id int64) (bool, error) {
	res, err := r.db.Exec(`
		UPDATE cli_auth_codes
		SET status = 'consumed', consumed_at = CURRENT_TIMESTAMP, token_plaintext = NULL
		WHERE id = ? AND status = 'approved' AND consumed_at IS NULL
	`, id)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}
