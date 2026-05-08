package logbookapi

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"windshift/internal/models"
	"windshift/internal/restapi"
)

// multipartMemoryThreshold is the boundary ParseMultipartForm uses to decide
// whether to keep a part in memory vs spill to a temp file on disk. Set
// intentionally low so the mime/multipart layer does not buffer large uploads
// in RAM; our own streaming writer then copies straight to the final storage
// location without re-buffering the whole file.
const multipartMemoryThreshold = 1 << 20 // 1 MiB

// storedUpload captures the outcome of writing a user upload to the storage
// directory in streaming fashion.
type storedUpload struct {
	Path     string
	MimeType string
	Hash     string
	Size     int64
}

// writeUploadToStorage streams a multipart file to disk under dstDir, hashing
// and content-sniffing along the way. It validates the extension up front,
// checks the sniffed MIME against the extension and the configured allowlist,
// and enforces the size limit from settings.
//
// The written file has a random hex filename (plus the original extension)
// and is only renamed into place after validation succeeds; failures leave
// nothing behind. dstDir must already exist with mode 0o750.
func writeUploadToStorage(
	src io.Reader,
	originalFilename string,
	dstDir string,
	settings *models.AttachmentSettings,
) (*storedUpload, error) {
	if err := validateFileExtension(originalFilename); err != nil {
		return nil, err
	}

	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("generate stored filename: %w", err)
	}
	ext := filepath.Ext(originalFilename)
	storedName := hex.EncodeToString(randomBytes) + ext
	finalPath := filepath.Join(dstDir, storedName)

	// Write to a sibling .tmp file first so a mid-stream abort or validation
	// failure never produces a half-written file at finalPath.
	tmpPath := finalPath + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304/G703: tmpPath = dstDir + hex-random filename, caller-provided dstDir is storagePath+validated-UUIDs
	if err != nil {
		return nil, fmt.Errorf("open upload tempfile: %w", err)
	}
	// On any return path other than success, remove the tempfile. We set
	// `success` at the very end.
	success := false
	defer func() {
		_ = out.Close()
		if !success {
			_ = os.Remove(tmpPath) //nolint:gosec // G304/G703: tmpPath constructed from validated parts (see OpenFile above)
		}
	}()

	hasher := sha256.New()
	// Buffer the first 512 bytes for http.DetectContentType. This is the same
	// size limit DetectContentType itself uses, so there's no point collecting
	// more.
	var sniff bytes.Buffer
	sniff.Grow(512)

	// Cap the copy at the configured max size; LimitReader stops at limit+1
	// so we can distinguish "exactly limit" from "overran". MaxBytesReader is
	// also in effect at the request-body layer — this is defense in depth
	// against a sneaky multipart encoding that slips past it.
	limit := settings.MaxFileSize
	limited := io.LimitReader(src, limit+1)

	written, err := copyWithSniff(out, hasher, &sniff, limited)
	if err != nil {
		return nil, fmt.Errorf("write upload: %w", err)
	}
	if written > limit {
		return nil, fmt.Errorf("file too large, maximum size: %d bytes", limit)
	}

	// Close file before rename to flush buffers. The deferred Close on an
	// already-closed *os.File returns an error we ignore.
	if err := out.Close(); err != nil {
		return nil, fmt.Errorf("close upload tempfile: %w", err)
	}

	detectedMimeType, err := verifyFileContent(sniff.Bytes(), originalFilename)
	if err != nil {
		return nil, err
	}
	if err := validateUploadAgainstSettings(settings, written, detectedMimeType); err != nil {
		return nil, err
	}

	if err := os.Rename(tmpPath, finalPath); err != nil { //nolint:gosec // G304/G703: paths constructed from validated parts (see OpenFile above)
		return nil, fmt.Errorf("rename upload into place: %w", err)
	}
	success = true

	return &storedUpload{
		Path:     finalPath,
		MimeType: detectedMimeType,
		Hash:     hex.EncodeToString(hasher.Sum(nil)),
		Size:     written,
	}, nil
}

// copyWithSniff copies src to dst while simultaneously feeding the stream
// into a hasher and collecting up to 512 bytes into sniff for MIME detection.
func copyWithSniff(dst, hasher io.Writer, sniff *bytes.Buffer, src io.Reader) (int64, error) {
	sniffNeed := 512 - sniff.Len()
	var total int64
	buf := make([]byte, 32*1024)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			chunk := buf[:n]

			if sniffNeed > 0 {
				take := sniffNeed
				if take > n {
					take = n
				}
				sniff.Write(chunk[:take])
				sniffNeed -= take
			}

			if _, err := hasher.Write(chunk); err != nil {
				return total, err
			}
			wn, werr := dst.Write(chunk)
			total += int64(wn)
			if werr != nil {
				return total, werr
			}
			if wn != n {
				return total, io.ErrShortWrite
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return total, nil
			}
			return total, rerr
		}
	}
}

// respondUploadError turns validation/storage errors into an appropriate HTTP
// response. Size-limit overflows and extension/MIME mismatches are 400s; any
// other error is treated as an internal server error.
func respondUploadError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if hasAnyPrefix(msg,
		"file extension",
		"files without extensions are not allowed",
		"file too large",
		"file content type",
		"file type",
	) {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, msg)
		return
	}
	respondInternalError(w, r, err)
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}
