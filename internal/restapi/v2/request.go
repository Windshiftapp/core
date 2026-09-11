package v2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
)

const defaultBodyLimit = 2 << 20

// Optional records whether a merge-patch field was present and whether it was null.
type Optional[T any] struct {
	Value T
	Set   bool
	Null  bool
}

func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Null = true
		var zero T
		o.Value = zero
		return nil
	}
	o.Null = false
	return json.Unmarshal(data, &o.Value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, patch bool) error {
	expected := "application/json"
	if patch {
		expected = "application/merge-patch+json"
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != expected {
		return newError(http.StatusUnsupportedMediaType, "unsupported_media_type", fmt.Sprintf("Content-Type must be %s", expected))
	}
	if r.Body == nil {
		return newError(http.StatusBadRequest, "invalid_request", "Request body is required")
	}

	r.Body = http.MaxBytesReader(w, r.Body, defaultBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return newError(http.StatusRequestEntityTooLarge, "payload_too_large", "Request body exceeds 2 MiB")
		}
		if errors.Is(err, io.EOF) {
			return newError(http.StatusBadRequest, "invalid_request", "Request body is required")
		}
		return newError(http.StatusBadRequest, "invalid_request", "Request body is not valid JSON")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return newError(http.StatusBadRequest, "invalid_request", "Request body must contain one JSON value")
	}
	return nil
}
