package logger

import (
	"context"
	"net/http"
	"sync"

	"windshift/internal/database"
	"windshift/internal/utils"
)

// AuditRecorder collects the audit events recorded while one HTTP request is
// handled. Middleware installs it in the request context, handlers append to
// it with Record, and the middleware flushes it exactly once after the handler
// returns. That keeps the HTTP layer as the single audit writer and gives the
// response outcome one owner instead of scattering writes through handlers.
type AuditRecorder struct {
	mu     sync.Mutex
	events []AuditEvent
}

type auditRecorderContextKey struct{}

// NewAuditRecorder returns an empty recorder.
func NewAuditRecorder() *AuditRecorder {
	return &AuditRecorder{}
}

// WithAuditRecorder installs rec in ctx.
func WithAuditRecorder(ctx context.Context, rec *AuditRecorder) context.Context {
	return context.WithValue(ctx, auditRecorderContextKey{}, rec)
}

// AuditRecorderFrom returns the recorder installed in ctx, or nil.
func AuditRecorderFrom(ctx context.Context) *AuditRecorder {
	rec, _ := ctx.Value(auditRecorderContextKey{}).(*AuditRecorder)
	return rec
}

// Record queues a successful audit event built from the request's
// authenticated actor. It is a no-op when no recorder is installed, so a
// handler can call it without knowing whether it runs under the HTTP audit
// middleware.
func Record(r *http.Request, actionType, resourceType string, resourceID *int, resourceName string, details map[string]any) {
	record(r, actionType, resourceType, resourceID, resourceName, details, true, "")
}

// RecordFailure queues a failed mutation. Failure events are flushed even when
// the response is not 2xx.
func RecordFailure(r *http.Request, actionType, resourceType string, resourceID *int, resourceName, errorMessage string, details map[string]any) {
	record(r, actionType, resourceType, resourceID, resourceName, details, false, errorMessage)
}

// RecordDenied queues an authorization denial for a mutating request.
func RecordDenied(r *http.Request, actionType, resourceType string, resourceID *int, resourceName, requiredPermission string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	if requiredPermission != "" {
		details["required_permission"] = requiredPermission
	}
	details["method"] = r.Method
	details["route"] = r.URL.Path
	RecordFailure(r, actionType, resourceType, resourceID, resourceName, "permission_denied", details)
}

func record(r *http.Request, actionType, resourceType string, resourceID *int, resourceName string, details map[string]any, success bool, errorMessage string) {
	rec := AuditRecorderFrom(r.Context())
	if rec == nil {
		return
	}
	event := AuditEvent{
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   actionType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      details,
		Success:      success,
		ErrorMessage: errorMessage,
	}
	if user := utils.GetCurrentUser(r); user != nil {
		event.UserID = user.ID
		event.Username = user.Username
	} else {
		event.Username = "unknown"
	}
	rec.append(event)
}

func (rec *AuditRecorder) append(event AuditEvent) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.events = append(rec.events, event)
}

// flush writes the queued events. Success events are written only when the
// request succeeded; failure events are always written so a denial or error is
// never lost because of the response status.
func (rec *AuditRecorder) flush(db database.Database, success bool) {
	rec.mu.Lock()
	events := rec.events
	rec.events = nil
	rec.mu.Unlock()
	for _, event := range events {
		if event.Success && !success {
			continue
		}
		_ = LogAudit(db, event)
	}
}

// Flush writes the recorder's queued events through the auditor's database.
// It is the middleware's single call into the recorder.
func (a *Auditor) Flush(rec *AuditRecorder, success bool) {
	if a == nil || rec == nil {
		return
	}
	rec.flush(a.db, success)
}
