package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
)

const assetImportLeaseDuration = time.Minute

var ErrAssetImportLeaseLost = errors.New("asset import lease expired or job is no longer active")

func assetImportWriteResult(result sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("write asset import job: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrAssetImportLeaseLost
	}
	return nil
}

// RenewImportJobLeaseInTx fences row insertion against recovery. The job lock
// remains held until the asset and its events commit.
func (r *AssetRepository) RenewImportJobLeaseInTx(tx database.Tx, jobID string, setID int) error {
	now := time.Now().UTC()
	return assetImportWriteResult(tx.ExecWrite(`UPDATE asset_import_jobs SET lease_expires_at = ?
		WHERE id = ? AND set_id = ? AND status = 'running' AND lease_expires_at > ?`,
		now.Add(assetImportLeaseDuration).Unix(), jobID, setID, now.Unix()))
}

// ReconcileExpiredAssetImports atomically claims and rolls back abandoned jobs.
// A live worker renewing its lease wins over recovery of a stale candidate.
func (r *AssetRepository) ReconcileExpiredAssetImports(now time.Time) (int, error) {
	rows, err := r.db.Query(`SELECT id FROM asset_import_jobs
		WHERE status IN ('queued', 'running') AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`, now.Unix())
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	count := 0
	for _, id := range ids {
		claimed := false
		err := database.WithTx(r.db, func(tx database.Tx) error {
			result, err := tx.ExecWrite(`UPDATE asset_import_jobs
				SET status = 'failed', phase = '', error_message = 'Import worker lease expired', completed_at = ?
				WHERE id = ? AND status IN ('queued', 'running') AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`, now, id, now.Unix())
			if err := assetImportWriteResult(result, err); err != nil {
				if errors.Is(err, ErrAssetImportLeaseLost) {
					return nil
				}
				return err
			}
			claimed = true
			return r.deleteAssetsFromImportJobInTx(tx, id)
		})
		if err != nil {
			return count, fmt.Errorf("recover import %s: %w", id, err)
		}
		if claimed {
			count++
		}
	}
	return count, nil
}
