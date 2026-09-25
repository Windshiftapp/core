package csvimport

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ParseDelimiter maps a client-supplied delimiter name to its rune. Comma is
// the default; an unknown name falls back to comma rather than erroring.
func ParseDelimiter(name string) rune {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "semicolon", ";":
		return ';'
	case "tab", "\\t", "\t":
		return '\t'
	case "pipe", "|":
		return '|'
	default:
		return ','
	}
}

// DelimiterName renders the canonical client-side name for a delimiter rune.
func DelimiterName(delimiter rune) string {
	switch delimiter {
	case ';':
		return "semicolon"
	case '\t':
		return "tab"
	case '|':
		return "pipe"
	default:
		return "comma"
	}
}

// NewReader builds a delimiter-aware CSV reader with the pipeline's shared
// tolerances (variable field counts, lenient quotes).
func NewReader(source io.Reader, delimiter rune) *csv.Reader {
	reader := csv.NewReader(source)
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	return reader
}

// DetectDelimiter picks the delimiter whose first line yields the most
// fields. Defaults to comma.
func DetectDelimiter(path string) rune {
	file, err := os.Open(path) //nolint:gosec // G304 cannot follow validation across the asynchronous boundary.
	if err != nil {
		return ','
	}
	defer func() { _ = file.Close() }()

	sniff := make([]byte, 4096)
	n, _ := file.Read(sniff)
	firstLine := strings.SplitN(string(sniff[:n]), "\n", 2)[0]

	best, bestCount := ',', strings.Count(firstLine, ",")
	for _, candidate := range []rune{';', '\t', '|'} {
		if count := strings.Count(firstLine, string(candidate)); count > bestCount {
			best, bestCount = candidate, count
		}
	}
	return best
}

// ParsePreview returns the header row, up to limit preview rows, and the
// total row count of the staged file.
func ParsePreview(path string, delimiter rune, hasHeader bool, limit int) (headers []string, rows [][]string, total int, err error) {
	file, err := os.Open(path) //nolint:gosec // G304 cannot follow validation across the asynchronous boundary.
	if err != nil {
		return nil, nil, 0, err
	}
	defer func() { _ = file.Close() }()

	reader := NewReader(file, delimiter)
	if hasHeader {
		if headers, err = reader.Read(); err != nil {
			return nil, nil, 0, err
		}
	}
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, nil, 0, err
		}
		total++
		if len(rows) < limit {
			rows = append(rows, record)
		}
	}
	return headers, rows, total, nil
}

// DetectHeaderMismatch warns when the first data row has a different field
// count than the header — the most common malformed-spreadsheet symptom.
func DetectHeaderMismatch(headers []string, rows [][]string, hasHeader bool) string {
	if !hasHeader || len(headers) == 0 || len(rows) == 0 {
		return ""
	}
	if len(rows[0]) != len(headers) {
		return fmt.Sprintf("header has %d columns but the first data row has %d", len(headers), len(rows[0]))
	}
	return ""
}

// StageUpload persists an uploaded CSV/TSV under storageRoot, enforces the
// byte cap, records the upload row, and returns the staged state. The upload
// directory is removed on any failure.
func StageUpload(store *Store, storageRoot, kind string, scopeID, userID int, filename string, hasHeader bool, delimiterName string, source io.Reader, maxBytes int64) (staged *StagedUpload, path string, err error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".csv" && ext != ".tsv" {
		return nil, "", fmt.Errorf("only CSV and TSV files are accepted")
	}

	uploadID, err := newUploadID()
	if err != nil {
		return nil, "", fmt.Errorf("generate upload id: %w", err)
	}
	dir := filepath.Join(storageRoot, "imports", uploadID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, "", fmt.Errorf("create import directory: %w", err)
	}
	path = filepath.Join(dir, "upload.csv")
	// path is derived from the configured storage root and a server-generated UUID.
	//nolint:gosec // G304 cannot infer that the path has no user-controlled segment.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", fmt.Errorf("create import upload: %w", err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(source, maxBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written > maxBytes {
		_ = os.RemoveAll(dir)
		if written > maxBytes {
			return nil, "", fmt.Errorf("CSV upload exceeds %d MiB", maxBytes>>20)
		}
		return nil, "", errors.Join(copyErr, closeErr)
	}

	delimiter := ParseDelimiter(delimiterName)
	if delimiterName == "" {
		delimiter = DetectDelimiter(path)
	}
	headers, rows, total, err := ParsePreview(path, delimiter, hasHeader, 5)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", fmt.Errorf("parse CSV: %v", err)
	}
	if err := store.CreateUpload(kind, scopeID, userID, uploadID, nowUTC()); err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", err
	}
	return &StagedUpload{
		UploadID:      uploadID,
		Headers:       headers,
		PreviewRows:   rows,
		TotalRows:     total,
		Delimiter:     DelimiterName(delimiter),
		HeaderWarning: DetectHeaderMismatch(headers, rows, hasHeader),
	}, path, nil
}

// StagedUpload is the client-visible result of staging one file.
type StagedUpload struct {
	UploadID      string     `json:"upload_id"`
	Headers       []string   `json:"headers"`
	PreviewRows   [][]string `json:"preview_rows"`
	TotalRows     int        `json:"total_rows"`
	Delimiter     string     `json:"delimiter"`
	HeaderWarning string     `json:"header_warning,omitempty"`
}

// UploadPath renders the staged file path for one upload ID.
func UploadPath(storageRoot, uploadID string) string {
	return filepath.Join(storageRoot, "imports", uploadID, "upload.csv")
}

// RemoveUpload deletes the staged directory for one upload ID.
func RemoveUpload(storageRoot, uploadID string) {
	_ = os.RemoveAll(filepath.Join(storageRoot, "imports", uploadID))
}
