package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeDB struct{ err error }

func (f fakeDB) PingContext(context.Context) error { return f.err }

type fakeAttachment struct {
	enabled bool
	err     error
}

func (f *fakeAttachment) Enabled() bool { return f.enabled }

func (f *fakeAttachment) CachedHealthCheck(context.Context) error { return f.err }

func ready(h *Handler) (*httptest.ResponseRecorder, readinessResponse) {
	rec := httptest.NewRecorder()
	h.Readiness(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var body readinessResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestReadinessHealthy(t *testing.T) {
	h := NewHandler(fakeDB{}, &fakeAttachment{enabled: true})
	rec, body := ready(h)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body.Database != "ok" || body.Attachments != "ok" {
		t.Fatalf("body = %+v, want database/attachments ok", body)
	}
}

func TestReadinessAttachmentUnavailable(t *testing.T) {
	h := NewHandler(fakeDB{}, &fakeAttachment{enabled: true, err: errors.New("bucket down")})
	rec, body := ready(h)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body.Attachments != "unavailable" {
		t.Fatalf("attachments = %q, want unavailable", body.Attachments)
	}
}

func TestReadinessDatabaseUnavailable(t *testing.T) {
	h := NewHandler(fakeDB{err: errors.New("db down")}, &fakeAttachment{enabled: true})
	rec, body := ready(h)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body.Database != "unavailable" {
		t.Fatalf("database = %q, want unavailable", body.Database)
	}
}

func TestReadinessDisabledAttachments(t *testing.T) {
	probe := &fakeAttachment{enabled: false, err: errors.New("should not be called")}
	h := NewHandler(fakeDB{}, probe)
	rec, body := ready(h)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body.Attachments != "disabled" {
		t.Fatalf("attachments = %q, want disabled", body.Attachments)
	}
}

func TestSetAttachmentProbe(t *testing.T) {
	h := NewHandler(fakeDB{})
	if rec, body := ready(h); rec.Code != http.StatusOK || body.Attachments != "disabled" {
		t.Fatalf("before set: status=%d body=%+v", rec.Code, body)
	}
	h.SetAttachmentProbe(&fakeAttachment{enabled: true, err: errors.New("down")})
	if rec, body := ready(h); rec.Code != http.StatusServiceUnavailable || body.Attachments != "unavailable" {
		t.Fatalf("after set: status=%d body=%+v", rec.Code, body)
	}
}

func TestLivenessDoesNotDependOnDependencies(t *testing.T) {
	h := NewHandler(fakeDB{err: errors.New("db down")}, &fakeAttachment{enabled: true, err: errors.New("s3 down")})
	rec := httptest.NewRecorder()
	h.Liveness(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("liveness status = %d, want 200", rec.Code)
	}
}
