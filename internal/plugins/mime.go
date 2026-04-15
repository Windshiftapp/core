package plugins

import (
	"mime"
	"path/filepath"
)

// mimeTypeForExt returns the MIME type for the given file extension.
// It first checks the system MIME database, then falls back to common types.
func mimeTypeForExt(name string) string {
	ext := filepath.Ext(name)
	mimeType := mime.TypeByExtension(ext)
	if mimeType != "" {
		return mimeType
	}
	switch ext {
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".json":
		return "application/json"
	case ".html":
		return "text/html"
	default:
		return "application/octet-stream"
	}
}
