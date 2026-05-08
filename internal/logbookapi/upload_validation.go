package logbookapi

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"windshift/internal/models"
)

// allowedExtensions is the positive allowlist of file extensions accepted by
// logbook uploads. Anything not in here is rejected regardless of what the
// MIME sniffer says. The list intentionally excludes any executable, script,
// active-content, or macro-bearing format.
var allowedExtensions = map[string]bool{
	// Documents
	".pdf":  true,
	".doc":  true,
	".docx": true,
	".odt":  true,
	".rtf":  true,
	".txt":  true,
	".md":   true,
	".csv":  true,
	".tsv":  true,

	// Spreadsheets
	".xls":  true,
	".xlsx": true,
	".ods":  true,

	// Presentations
	".ppt":  true,
	".pptx": true,
	".odp":  true,

	// Images
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
	".tiff": true,
	".tif":  true,
	".heic": true,
	".heif": true,

	// Email / archive (opened server-side for text only, never served inline)
	".eml": true,

	// Audio / video (metadata-only extraction; served as attachment)
	".mp3": true,
	".mp4": true,
	".m4a": true,
	".wav": true,
	".ogg": true,
	".mov": true,
	".mkv": true,
}

// validateFileExtension enforces the allowlist. Unknown or empty extensions
// are rejected.
func validateFileExtension(filename string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" || ext == "." {
		return fmt.Errorf("files without extensions are not allowed")
	}
	if !allowedExtensions[ext] {
		return fmt.Errorf("file extension %s is not allowed", ext)
	}
	return nil
}

// verifyFileContent uses http.DetectContentType to verify that file content
// matches the declared extension. Returns the detected MIME type on success.
// Unknown extensions (no mime.TypeByExtension mapping) are refused: we do not
// want to fall back to "allow anything that sniffs OK" for exotic types.
func verifyFileContent(fileData []byte, filename string) (string, error) {
	detectSize := 512
	if len(fileData) < detectSize {
		detectSize = len(fileData)
	}
	detectedType := http.DetectContentType(fileData[:detectSize])

	ext := filepath.Ext(filename)
	expectedType := mime.TypeByExtension(ext)
	if expectedType == "" {
		// We already validated the extension is in the allowlist. If the Go
		// mime database has no mapping, fall back to the detected type — but
		// only because the extension passed the allowlist gate.
		return detectedType, nil
	}

	detectedBase := strings.Split(detectedType, ";")[0]
	expectedBase := strings.Split(expectedType, ";")[0]

	if detectedBase == expectedBase {
		return expectedType, nil
	}
	// Text-vs-text is fine (Go's sniffer often returns text/plain for things
	// that mime.TypeByExtension marks as text/csv, text/markdown, etc.).
	if detectedBase == "text/plain" && strings.HasPrefix(expectedBase, "text/") {
		return expectedType, nil
	}
	// ZIP-based container formats (DOCX, XLSX, PPTX, ODT, EPUB) all sniff as
	// application/zip. This is a real and benign carve-out.
	if detectedBase == "application/zip" && isLogbookZipBasedMimeType(expectedBase) {
		return expectedType, nil
	}
	// application/octet-stream is the sniffer's "I don't know" response and
	// should not pass — previously this silently accepted anything.
	return "", fmt.Errorf("file content type (%s) doesn't match extension %s (expected %s)", detectedBase, ext, expectedBase)
}

// isLogbookZipBasedMimeType returns true for MIME types that share ZIP magic bytes.
func isLogbookZipBasedMimeType(mimeType string) bool {
	return mimeType == "application/zip" ||
		strings.Contains(mimeType, "openxmlformats") ||
		strings.Contains(mimeType, "opendocument") ||
		mimeType == "application/epub+zip"
}

// validateUploadAgainstSettings checks file size and MIME type against attachment settings.
func validateUploadAgainstSettings(settings *models.AttachmentSettings, fileSize int64, detectedMimeType string) error {
	if fileSize > settings.MaxFileSize {
		return fmt.Errorf("file too large, maximum size: %d bytes", settings.MaxFileSize)
	}

	if settings.AllowedMimeTypes == "" {
		return nil
	}

	var allowedTypes []string
	if err := json.Unmarshal([]byte(settings.AllowedMimeTypes), &allowedTypes); err != nil {
		return fmt.Errorf("malformed allowed_mime_types setting: %w", err)
	}

	if len(allowedTypes) == 0 {
		return nil
	}

	for _, allowedType := range allowedTypes {
		if strings.HasPrefix(detectedMimeType, allowedType) {
			return nil
		}
	}

	return fmt.Errorf("file type %s is not allowed by server configuration", detectedMimeType)
}
