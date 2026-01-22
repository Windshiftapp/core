package middleware

import (
	"compress/gzip"
	"net/http"

	"github.com/klauspost/compress/gzhttp"
)

// CreateCompressionMiddleware returns a middleware that gzips responses.
// Only enabled when not behind a trusted proxy (proxies typically handle compression).
func CreateCompressionMiddleware(useProxy bool) func(http.Handler) http.Handler {
	if useProxy {
		// Behind a proxy - let the proxy handle compression
		return func(h http.Handler) http.Handler {
			return h
		}
	}

	// Not behind proxy - enable gzip compression
	wrapper, _ := gzhttp.NewWrapper(
		gzhttp.MinSize(1024), // Only compress responses > 1KB
		gzhttp.CompressionLevel(gzip.DefaultCompression),
	)

	return func(h http.Handler) http.Handler {
		return wrapper(h)
	}
}
