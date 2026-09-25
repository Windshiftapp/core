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
	return assetImportWriteResult(tx.ExecWrite(`UPDATE import_jobs SET lease_expires_at = ?
		WHERE id = ? AND scope_id = ? AND status = 'running' AND lease_expires_at > ?`,
		now.Add(assetImportLeaseDuration).Unix(), jobID, setID, now.Unix()))
}
