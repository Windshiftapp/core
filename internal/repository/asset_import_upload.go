package repository

import (
	"errors"
	"fmt"
	"time"
)

func (r *AssetRepository) CreateImportUpload(id string, setID, userID int, createdAt time.Time) error {
	_, err := r.db.ExecWrite("INSERT INTO asset_import_uploads (id, set_id, created_by, created_at) VALUES (?, ?, ?, ?)", id, setID, userID, createdAt.Unix())
	return err
}

func (r *AssetRepository) ImportUploadOwnedBy(id string, setID, userID int) (bool, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM asset_import_uploads WHERE id = ? AND set_id = ? AND created_by = ?", id, setID, userID).Scan(&count)
	return count == 1, err
}

var ErrAssetImportConfigConflict = errors.New("upload has already been started with different import settings")

// ClaimImportUpload uses the upload ID as the job ID, so its primary key elects one worker.
func (r *AssetRepository) ClaimImportUpload(uploadID string, setID, userID int, filePath, configJSON string, createdAt time.Time) (bool, error) {
	result, err := r.db.ExecWrite(`
  INSERT INTO asset_import_jobs (id, set_id, status, phase, file_path, config_json, created_by, created_at, lease_expires_at)
  SELECT ?, ?, 'queued', 'initializing', ?, ?, ?, ?, ? FROM asset_import_uploads
  WHERE id = ? AND set_id = ? AND created_by = ?
  ON CONFLICT (id) DO NOTHING`, uploadID, setID, filePath, configJSON, userID, createdAt, createdAt.Add(assetImportLeaseDuration).Unix(), uploadID, setID, userID)
	if err != nil {
		return false, fmt.Errorf("claim import upload: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count == 1 {
		return true, nil
	}
	var existing string
	if err := r.db.QueryRow("SELECT config_json FROM asset_import_jobs WHERE id = ? AND set_id = ? AND created_by = ?", uploadID, setID, userID).Scan(&existing); err != nil {
		return false, notFoundOrWrap(err, "find claimed import")
	}
	if existing != configJSON {
		return false, ErrAssetImportConfigConflict
	}
	return false, nil
}
