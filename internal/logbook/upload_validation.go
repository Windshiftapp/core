package logbook

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"windshift/internal/models"
)

// dangerousExtensions lists file extensions that are blocked for security reasons.
var dangerousExtensions = []string{
	".exe", ".bat", ".cmd", ".com", ".pif", ".scr", ".msi", // Windows executables
	".js", ".jsx", ".ts", ".tsx", // JavaScript/TypeScript (XSS risk)
	".html", ".htm", ".svg", // HTML/SVG (XSS risk)
	".sh", ".bash", ".zsh", ".fish", // Shell scripts
	".py", ".rb", ".pl", ".php", ".asp", ".aspx", ".jsp", // Server-side scripts
	".jar", ".class", ".dex", // Java/Android executables
	".app", ".dmg", ".pkg", // macOS executables/installers
	".deb", ".rpm", // Linux packages
	".apk", ".ipa", // Mobile app packages
}

// validateFileExtension blocks dangerous extensions and files without extensions.
func validateFileExtension(filename string) error {
	ext := strings.ToLower(filepath.Ext(filename))

	for _, dangerous := range dangerousExtensions {
		if ext == dangerous {
			return fmt.Errorf("file extension %s is not allowed for security reasons", ext)
		}
	}

	if ext == "" || ext == "." {
		return fmt.Errorf("files without extensions are not allowed")
	}

	return nil
}

// verifyFileContent uses http.DetectContentType to verify that file content
// matches the declared extension. Returns the detected MIME type on success.
func verifyFileContent(fileData []byte, filename string) (string, error) {
	detectSize := 512
	if len(fileData) < detectSize {
		detectSize = len(fileData)
	}

	detectedType := http.DetectContentType(fileData[:detectSize])

	ext := filepath.Ext(filename)
	expectedType := mime.TypeByExtension(ext)

	if expectedType != "" {
		detectedBase := strings.Split(detectedType, ";")[0]
		expectedBase := strings.Split(expectedType, ";")[0]

		if detectedBase != expectedBase && detectedBase != "application/octet-stream" &&
			(detectedBase != "text/plain" || !strings.HasPrefix(expectedBase, "text/")) {
			// Allow application/zip for known ZIP-based container formats.
			if detectedBase == "application/zip" && isLogbookZipBasedMimeType(expectedBase) {
				return expectedType, nil
			}
			return "", fmt.Errorf("file content type (%s) doesn't match extension %s (expected %s)", detectedBase, ext, expectedBase)
		}
	}

	return detectedType, nil
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
		return nil // malformed setting = no restriction
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
