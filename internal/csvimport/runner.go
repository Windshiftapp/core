package csvimport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

func newUploadID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func nowUTC() time.Time { return time.Now().UTC() }

// Progress is the persisted, client-visible state of a running import.
type Progress struct {
	Phase         string   `json:"phase"`
	TotalRows     int      `json:"total_rows"`
	ImportedCount int      `json:"imported_count"`
	FailedCount   int      `json:"failed_count"`
	SkippedCount  int      `json:"skipped_count"`
	Errors        []string `json:"errors,omitempty"`
}

// ErrorCap bounds the per-job row error list; overflow is summarized.
const ErrorCap = 50

// progressSink persists progress for a job. Store satisfies it.
type progressSink interface {
	StartRunning(jobID, phase, progressJSON string) error
	UpdateStatus(jobID, status, phase, progressJSON string) error
	UpdateProgress(jobID, phase, progressJSON string) error
	Finish(jobID, status, phase, progressJSON, errorMessage string) error
}

// RunRows streams the staged file through rowFn, collecting row-level errors
// (capped at ErrorCap), persisting progress every progressInterval rows, and
// finishing the job. It returns when the file is exhausted, the row callback
// fails with a lease loss (the worker was superseded), or a fatal setup error
// occurs — per-row failures are absorbed into the progress report.
func RunRows(store progressSink, jobID, path string, delimiter rune, hasHeader bool, rowFn func(rowNumber int, record []string) error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			progress := &Progress{Phase: "completed"}
			_ = store.Finish(jobID, "failed", "", marshalProgress(progress), fmt.Sprintf("Import crashed: %v", recovered))
		}
	}()
	progress := &Progress{Phase: "importing"}
	if err := store.StartRunning(jobID, "importing", marshalProgress(progress)); err != nil {
		return
	}
	// path was built from the configured storage root and a validated upload ID.
	//nolint:gosec // G304 cannot follow validation across the asynchronous boundary.
	file, err := os.Open(path)
	if err != nil {
		_ = store.Finish(jobID, "failed", "", marshalProgress(progress), "Failed to open CSV file")
		return
	}
	defer func() { _ = file.Close() }()
	reader := NewReader(file, delimiter)
	if hasHeader {
		if _, err := reader.Read(); err != nil {
			_ = store.Finish(jobID, "failed", "", marshalProgress(progress), "Failed to read CSV header")
			return
		}
	}

	const progressInterval = 100
	errorsTruncated := false
	appendError := func(message string) {
		if len(progress.Errors) < ErrorCap {
			progress.Errors = append(progress.Errors, message)
		} else {
			errorsTruncated = true
		}
	}
	for row := 1; ; row++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		progress.TotalRows = row
		if readErr != nil {
			progress.FailedCount++
			appendError(fmt.Sprintf("Row %d: %v", row, readErr))
		} else if err := rowFn(row, record); err != nil {
			if isLeaseLost(err) {
				return // the worker was superseded; reconciliation owns the job now
			}
			if errors.Is(err, ErrSkipRow) {
				progress.SkippedCount++
			} else {
				progress.FailedCount++
				appendError(fmt.Sprintf("Row %d: %v", row, err))
			}
		} else {
			progress.ImportedCount++
		}
		if row%progressInterval == 0 {
			if err := store.UpdateProgress(jobID, progress.Phase, marshalProgress(progress)); err != nil {
				return
			}
		}
	}
	if errorsTruncated {
		progress.Errors = append(progress.Errors, fmt.Sprintf("additional errors omitted; only the first %d are shown", ErrorCap))
	}
	progress.Phase = "completed"
	if err := store.Finish(jobID, "completed", "completed", marshalProgress(progress), ""); err != nil {
		return
	}
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		slog.Warn("failed to clean import upload", "path", path, "error", err)
	}
}

// MarshalProgress encodes progress for the job row's progress_json column.
func MarshalProgress(progress *Progress) string { return marshalProgress(progress) }

func marshalProgress(progress *Progress) string {
	data, err := json.Marshal(progress)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func isLeaseLost(err error) bool {
	return errors.Is(err, ErrLeaseLost)
}
