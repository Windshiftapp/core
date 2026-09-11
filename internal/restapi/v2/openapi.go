package v2

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

func serveOpenAPI(spec []byte) Handler {
	digest := sha256.Sum256(spec)
	etag := `"` + hex.EncodeToString(digest[:]) + `"`
	return func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return nil
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(spec)
		return nil
	}
}
