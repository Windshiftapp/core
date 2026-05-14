package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type optionalJSONFixture struct {
	Field string `json:"field"`
	Flag  bool   `json:"flag"`
}

func TestDecodeOptionalJSON_NilBody(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Body = nil

	v, ok := decodeOptionalJSON[optionalJSONFixture](rec, r)
	if !ok {
		t.Fatalf("expected ok=true for nil body")
	}
	if v != (optionalJSONFixture{}) {
		t.Fatalf("expected zero value, got %+v", v)
	}
	if rec.Code != 200 {
		t.Fatalf("expected default 200, got %d", rec.Code)
	}
}

func TestDecodeOptionalJSON_EmptyBody(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))

	v, ok := decodeOptionalJSON[optionalJSONFixture](rec, r)
	if !ok {
		t.Fatalf("expected ok=true for empty body, got %d", rec.Code)
	}
	if v != (optionalJSONFixture{}) {
		t.Fatalf("expected zero value, got %+v", v)
	}
}

func TestDecodeOptionalJSON_ValidBody(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"field":"hello","flag":true}`))

	v, ok := decodeOptionalJSON[optionalJSONFixture](rec, r)
	if !ok {
		t.Fatalf("expected ok=true for valid body")
	}
	if v.Field != "hello" || !v.Flag {
		t.Fatalf("expected {hello,true}, got %+v", v)
	}
}

func TestDecodeOptionalJSON_MalformedBody(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"field":`))

	_, ok := decodeOptionalJSON[optionalJSONFixture](rec, r)
	if ok {
		t.Fatalf("expected ok=false for malformed body")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDecodeOptionalJSON_TypeMismatch(t *testing.T) {
	rec := httptest.NewRecorder()
	// flag is bool — string should fail to decode
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"flag":"not-a-bool"}`))

	_, ok := decodeOptionalJSON[optionalJSONFixture](rec, r)
	if ok {
		t.Fatalf("expected ok=false for type mismatch")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// decodeOptionalJSON should also handle a request whose body has already
// been drained — common in handler chains. Reading after the EOF returns
// io.EOF and must be treated as "no body provided".
func TestDecodeOptionalJSON_AlreadyDrained(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"field":"x"}`))
	_, _ = io.ReadAll(r.Body)

	_, ok := decodeOptionalJSON[optionalJSONFixture](rec, r)
	if !ok {
		t.Fatalf("expected ok=true after body drained, got %d", rec.Code)
	}
}
