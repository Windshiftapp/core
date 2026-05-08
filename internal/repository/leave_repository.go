package repository

import (
	"database/sql"
	"errors"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type LeaveRepository struct {
	db database.Database
}

func NewLeaveRepository(db database.Database) *LeaveRepository {
	return &LeaveRepository{db: db}
}

func (r *LeaveRepository) GetByID(id int) (*models.UserLeavePeriod, error) {
	var leave models.UserLeavePeriod
	var substituteID sql.NullInt64
	var substituteName sql.NullString
	var userName sql.NullString

	err := r.db.QueryRow(`
		SELECT lp.id, lp.user_id, lp.substitute_user_id, lp.start_date, lp.end_date,
			lp.reason, lp.is_active, lp.created_at, lp.updated_at,
			sub.first_name || ' ' || sub.last_name as substitute_name,
			u.first_name || ' ' || u.last_name as user_name
		FROM user_leave_periods lp
		LEFT JOIN users sub ON sub.id = lp.substitute_user_id
		LEFT JOIN users u ON u.id = lp.user_id
		WHERE lp.id = ?
	`, id).Scan(
		&leave.ID, &leave.UserID, &substituteID, &leave.StartDate, &leave.EndDate,
		&leave.Reason, &leave.IsActive, &leave.CreatedAt, &leave.UpdatedAt,
		&substituteName, &userName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if substituteID.Valid {
		id := int(substituteID.Int64)
		leave.SubstituteUserID = &id
	}
	leave.SubstituteName = substituteName.String
	leave.UserName = userName.String

	return &leave, nil
}

func (r *LeaveRepository) GetForUser(userID int) ([]models.UserLeavePeriod, error) {
	rows, err := r.db.Query(`
		SELECT lp.id, lp.user_id, lp.substitute_user_id, lp.start_date, lp.end_date,
			lp.reason, lp.is_active, lp.created_at, lp.updated_at,
			sub.first_name || ' ' || sub.last_name as substitute_name
		FROM user_leave_periods lp
		LEFT JOIN users sub ON sub.id = lp.substitute_user_id
		WHERE lp.user_id = ?
		ORDER BY lp.start_date DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var periods []models.UserLeavePeriod
	for rows.Next() {
		var lp models.UserLeavePeriod
		var substituteID sql.NullInt64
		var substituteName sql.NullString

		err := rows.Scan(
			&lp.ID, &lp.UserID, &substituteID, &lp.StartDate, &lp.EndDate,
			&lp.Reason, &lp.IsActive, &lp.CreatedAt, &lp.UpdatedAt,
			&substituteName,
		)
		if err != nil {
			return nil, err
		}

		if substituteID.Valid {
			id := int(substituteID.Int64)
			lp.SubstituteUserID = &id
		}
		lp.SubstituteName = substituteName.String
		periods = append(periods, lp)
	}

	return periods, nil
}

func (r *LeaveRepository) GetActiveForUser(userID int) (*models.UserLeavePeriod, error) {
	var leave models.UserLeavePeriod
	var substituteID sql.NullInt64
	var substituteName sql.NullString

	err := r.db.QueryRow(`
		SELECT lp.id, lp.user_id, lp.substitute_user_id, lp.start_date, lp.end_date,
			lp.reason, lp.is_active, lp.created_at, lp.updated_at,
			sub.first_name || ' ' || sub.last_name as substitute_name
		FROM user_leave_periods lp
		LEFT JOIN users sub ON sub.id = lp.substitute_user_id
		WHERE lp.user_id = ? AND lp.is_active = true
			AND lp.start_date <= date('now') AND lp.end_date >= date('now')
		LIMIT 1
	`, userID).Scan(
		&leave.ID, &leave.UserID, &substituteID, &leave.StartDate, &leave.EndDate,
		&leave.Reason, &leave.IsActive, &leave.CreatedAt, &leave.UpdatedAt,
		&substituteName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // No active leave is normal
	}
	if err != nil {
		return nil, err
	}

	if substituteID.Valid {
		id := int(substituteID.Int64)
		leave.SubstituteUserID = &id
	}
	leave.SubstituteName = substituteName.String

	return &leave, nil
}

// IsUserOnLeave checks if user is currently on leave, returns substitute ID if set
func (r *LeaveRepository) IsUserOnLeave(userID int) (isOnLeave bool, substitutePtr *int, retErr error) {
	var substituteID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT substitute_user_id
		FROM user_leave_periods
		WHERE user_id = ? AND is_active = true
			AND start_date <= date('now') AND end_date >= date('now')
		LIMIT 1
	`, userID).Scan(&substituteID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}

	if substituteID.Valid {
		id := int(substituteID.Int64)
		return true, &id, nil
	}
	return true, nil, nil
}

func (r *LeaveRepository) Create(userID int, substituteUserID *int, startDate, endDate, reason string) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO user_leave_periods (user_id, substitute_user_id, start_date, end_date, reason, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, true, ?, ?) RETURNING id
	`, userID, substituteUserID, startDate, endDate, reason, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *LeaveRepository) Update(id int, substituteUserID *int, startDate, endDate, reason string) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE user_leave_periods
		SET substitute_user_id = ?, start_date = ?, end_date = ?, reason = ?, updated_at = ?
		WHERE id = ?
	`, substituteUserID, startDate, endDate, reason, now, id)
	return err
}

func (r *LeaveRepository) Delete(id int) error {
	_, err := r.db.Exec("DELETE FROM user_leave_periods WHERE id = ?", id)
	return err
}
