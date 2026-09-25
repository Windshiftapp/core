// Package csvimport is the shared CSV import pipeline: upload staging,
// delimiter-aware parsing, durable job tracking with lease semantics, and a
// row runner with progress and row-level error reporting. Domain code supplies
// the per-row callback; everything else lives here.
package csvimport

import "errors"

// Kind discriminates the owning domain in the shared import tables.
const (
	KindAsset  = "asset"
	KindTicket = "ticket"
)

var (
	// ErrLeaseLost reports that the current worker lost the job lease (a
	// reconciliation pass expired it) and must stop writing immediately.
	ErrLeaseLost = errors.New("import lease expired or job is no longer active")
	// ErrConfigConflict reports that the upload was already started with
	// different import settings.
	ErrConfigConflict = errors.New("upload has already been started with different import settings")
	// ErrUploadNotFound reports that the upload does not exist for the caller.
	ErrUploadNotFound = errors.New("import upload not found")
	// ErrSkipRow marks a row the callback declined benignly — typically a
	// duplicate that was already imported. Counted as skipped, not failed.
	ErrSkipRow = errors.New("skip row")
)
