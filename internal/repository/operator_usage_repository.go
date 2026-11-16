package repository

import (
	"context"
	"database/sql"
	"errors"

	"windshift/internal/database"
	"windshift/internal/operatorcontrols"
)

type OperatorUsageRepository struct{ db database.Database }

func NewOperatorUsageRepository(db database.Database) *OperatorUsageRepository {
	return &OperatorUsageRepository{db: db}
}

func (r *OperatorUsageRepository) Reserve(ctx context.Context, resource string, period, amount, ceiling int64) error {
	if amount < 0 || amount > ceiling {
		return &operatorcontrols.LimitError{Resource: resource}
	}
	result, err := r.db.ExecWriteContext(ctx, `INSERT INTO operator_usage (resource, period, used) VALUES (?, ?, ?)
		ON CONFLICT (resource, period) DO UPDATE SET used = operator_usage.used + excluded.used
		WHERE operator_usage.used <= ?`, resource, period, amount, ceiling-amount)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return &operatorcontrols.LimitError{Resource: resource}
	}
	return nil
}

func (r *OperatorUsageRepository) Refund(ctx context.Context, resource string, period, amount int64) error {
	if amount < 0 {
		return errors.New("negative operator usage refund")
	}
	result, err := r.db.ExecWriteContext(ctx, `UPDATE operator_usage SET used = used - ? WHERE resource = ? AND period = ? AND used >= ?`, amount, resource, period, amount)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("operator usage reservation missing")
	}
	return nil
}

func (r *OperatorUsageRepository) Used(ctx context.Context, resource string, period int64) (int64, error) {
	var used int64
	err := r.db.QueryRowContext(ctx, `SELECT used FROM operator_usage WHERE resource = ? AND period = ?`, resource, period).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return used, err
}
