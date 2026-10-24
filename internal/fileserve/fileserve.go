// Package fileserve safely confines stored files and formats safe disposition
// headers for cookie-auth and bearer-token handlers.
package fileserve

import (
	"errors"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ErrOutsideRoot is returned when a stored path cannot be resolved to a
// location inside the configured storage root.
var ErrOutsideRoot = errors.New("path is outside the configured storage root")

// OpenUnderRoot resolves legacy absolute, CWD-relative, and root-relative paths
// through os.Root, rejecting traversal and escaping symlinks. Callers close the
// returned file; escapes return ErrOutsideRoot.
func OpenUnderRoot(root, storedPath string) (*os.File, error) {
	if root == "" {
		return nil, ErrOutsideRoot
	}
	rel, err := relWithinRoot(root, storedPath)
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return r.Open(rel)
}

// RemoveUnderRoot mirrors OpenUnderRoot confinement and removes only the named file.
func RemoveUnderRoot(root, storedPath string) error {
	if root == "" {
		return ErrOutsideRoot
	}
	rel, err := relWithinRoot(root, storedPath)
	if err != nil {
		return err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	return r.Remove(rel)
}

// relWithinRoot preserves historical path resolution while requiring a safe
// root-relative result for os.Root.
func relWithinRoot(root, storedPath string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	var candidates []string
	if filepath.IsAbs(storedPath) {
		candidates = []string{storedPath}
	} else {
		candidates = []string{storedPath, filepath.Join(root, storedPath)}
	}

	for _, candidate := range candidates {
		absCandidate, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absRoot, absCandidate)
		if err != nil {
			continue
		}
		if rel == "." || rel == ".." ||
			strings.HasPrefix(rel, ".."+string(os.PathSeparator)) ||
			filepath.IsAbs(rel) {
			continue
		}
		return rel, nil
	}
	return "", ErrOutsideRoot
}

// inlineSafeMimeTypes is the allowlist of Content-Types a browser may render
// inline. Every entry is a non-executable media type; HTML/XHTML/SVG/MathML and
// any unknown type are absent and therefore forced to download.
var inlineSafeMimeTypes = map[string]struct{}{
	"image/png":                {},
	"image/jpeg":               {},
	"image/gif":                {},
	"image/webp":               {},
	"image/bmp":                {},
	"image/x-icon":             {},
	"image/vnd.microsoft.icon": {},
	"application/pdf":          {},
	"text/plain":               {},
	"audio/mpeg":               {},
	"audio/ogg":                {},
	"audio/wav":                {},
	"audio/x-wav":              {},
	"audio/webm":               {},
	"video/mp4":                {},
	"video/webm":               {},
	"video/ogg":                {},
}

// IsInlineSafeMimeType reports whether a stored Content-Type may be served with
// an inline Content-Disposition. The declared type (before any parameters such
// as "; charset=") is matched case-insensitively; SVG is deliberately absent
// because it can carry script.
func IsInlineSafeMimeType(mimeType string) bool {
	base := strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(base, ';'); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	_, ok := inlineSafeMimeTypes[base]
	return ok
}

// ContentDisposition builds a safe Content-Disposition header value for the
// given disposition ("inline" or "attachment") and filename.
//
// The filename is formatted with mime.FormatMediaType, which quotes/escapes
// special characters (quotes, semicolons, backslashes, spaces) and emits an
// RFC 2231 filename* parameter for non-ASCII names — so a filename cannot
// inject extra parameters or break out of the quoted string. Control
// characters (including CR/LF) are stripped first so the value can never
// contribute to header splitting.
//
// If formatting fails (e.g. an unexpected disposition), the bare disposition is
// returned without a filename rather than an attacker-influenced string.
func ContentDisposition(disposition, filename string) string {
	clean := sanitizeFilename(filename)
	if v := mime.FormatMediaType(disposition, map[string]string{"filename": clean}); v != "" {
		return v
	}
	return disposition
}

// sanitizeFilename drops control characters (CR, LF, NUL, and the rest) that
// have no place in a header value. Everything else — including Unicode — is
// preserved for mime.FormatMediaType to encode.
func sanitizeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
}
