// Package blobstore abstracts attachment blob storage. The local backend
// preserves the historical ATTACHMENT_PATH filesystem behavior; the S3 backend
// stores the same objects in an S3-compatible bucket.
//
// Stored paths are the values persisted in attachments.file_path and
// attachments.thumbnail_path. The local backend accepts the historical mix of
// absolute and root-relative paths; the S3 backend maps them to object keys.
package blobstore

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path"
	"path/filepath"
	"sync"
	"time"
)

var (
	// ErrNotFound reports a blob that does not exist in the backend.
	ErrNotFound = errors.New("blob not found")
	// ErrOutsideRoot reports a stored path that escapes the configured root.
	ErrOutsideRoot = errors.New("blob path is outside the configured storage root")
)

// Object is an open blob. It supports seeking so http.ServeContent can honor
// range requests without buffering the whole object.
type Object interface {
	io.ReadSeekCloser
	Size() int64
	ModTime() time.Time
}

type backend interface {
	put(ctx context.Context, storedPath string, r io.Reader, size int64, contentType string) error
	open(ctx context.Context, storedPath string) (Object, error)
	remove(ctx context.Context, storedPath string) error
	probe(ctx context.Context) error
	health(ctx context.Context) error
}

// Store is the configured attachment blob backend. A nil *Store is disabled;
// all methods are nil-safe so callers can treat a missing store as "attachments
// are not enabled".
type Store struct {
	backend  backend
	enabled  bool
	local    bool
	root     string
	describe string

	healthMu  sync.Mutex
	healthErr error
	healthAt  time.Time
	healthTTL time.Duration
}

const (
	// defaultHealthCacheTTL keeps readiness ticks and status/bootstrap calls
	// from issuing one object-store request per request handler.
	defaultHealthCacheTTL = 10 * time.Second
	healthProbeTimeout    = 2 * time.Second
)

// NewLocal builds a filesystem-backed store rooted at root. An empty root
// produces a disabled store, preserving the historical "no ATTACHMENT_PATH
// means attachments are off" behavior.
func NewLocal(root string) *Store {
	if root == "" {
		return &Store{}
	}
	return &Store{
		backend:  &localBackend{root: root},
		enabled:  true,
		local:    true,
		root:     root,
		describe: root,
	}
}

// Disabled returns a store that reports itself as unavailable.
func Disabled() *Store {
	return &Store{}
}

// Enabled reports whether attachment storage is configured.
func (s *Store) Enabled() bool {
	return s != nil && s.enabled
}

// Backend names the active backend ("local" or "s3").
func (s *Store) Backend() string {
	if s == nil || !s.enabled {
		return ""
	}
	if s.local {
		return "local"
	}
	return "s3"
}

// IsS3 reports whether the store is backed by an S3-compatible bucket.
func (s *Store) IsS3() bool {
	return s != nil && s.enabled && !s.local
}

// Describe returns a non-sensitive descriptor for admin status output.
func (s *Store) Describe() string {
	if s == nil {
		return ""
	}
	return s.describe
}

// Path builds the stored path for a new object. The local backend keeps the
// historical absolute shape (<root>/<parts...>); the S3 backend stores a
// root-relative key so rows stay portable between buckets.
func (s *Store) Path(parts ...string) string {
	if s == nil || !s.enabled {
		return ""
	}
	if s.local {
		all := make([]string, 0, len(parts)+1)
		all = append(all, s.root)
		all = append(all, parts...)
		return filepath.Join(all...)
	}
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			clean = append(clean, part)
		}
	}
	return path.Join(clean...)
}

// Put stores an object at storedPath.
func (s *Store) Put(ctx context.Context, storedPath string, r io.Reader, size int64, contentType string) error {
	if s == nil || !s.enabled {
		return errors.New("attachment storage is not enabled")
	}
	ctx, cancel := withDefaultTimeout(ctx, 5*time.Minute)
	defer cancel()
	return s.backend.put(ctx, storedPath, r, size, contentType)
}

// Open returns a readable object for storedPath.
func (s *Store) Open(ctx context.Context, storedPath string) (Object, error) {
	if s == nil || !s.enabled {
		return nil, ErrNotFound
	}
	return s.backend.open(ctx, storedPath)
}

// Remove deletes storedPath. Missing objects are not an error.
func (s *Store) Remove(ctx context.Context, storedPath string) error {
	if s == nil || !s.enabled {
		return nil
	}
	ctx, cancel := withDefaultTimeout(ctx, time.Minute)
	defer cancel()
	return s.backend.remove(ctx, storedPath)
}

// HealthCheck performs a single lightweight reachability probe for readiness
// endpoints. It neither retries nor writes: callers are expected to cache the
// result so a health-check tick does not turn into a network call every time.
// Disabled storage is healthy (attachments simply are not enabled).
func (s *Store) HealthCheck(ctx context.Context) error {
	if s == nil || !s.enabled {
		return nil
	}
	return s.backend.health(ctx)
}

// CachedHealthCheck returns a shared, TTL-cached readiness result. The probe is
// detached from the caller's cancellation (context.WithoutCancel) so a canceled
// HTTP readiness request cannot poison the cache; the probe keeps its own
// timeout. Caller/system cancellation errors are not cached.
func (s *Store) CachedHealthCheck(ctx context.Context) error {
	if s == nil || !s.enabled {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.healthMu.Lock()
	defer s.healthMu.Unlock()

	ttl := s.healthTTL
	if ttl <= 0 {
		ttl = defaultHealthCacheTTL
	}
	if !s.healthAt.IsZero() && time.Since(s.healthAt) < ttl {
		return s.healthErr
	}

	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), healthProbeTimeout)
	defer cancel()

	previousErr := s.healthErr
	err := s.backend.health(probeCtx)
	if errors.Is(err, context.Canceled) {
		// A canceled caller/system is not a storage-health result.
		return err
	}
	s.healthErr = err
	s.healthAt = time.Now()

	switch {
	case err != nil && previousErr == nil:
		slog.Warn("attachment storage readiness check failed",
			slog.String("component", "blobstore"), slog.Any("error", err))
	case err == nil && previousErr != nil:
		slog.Info("attachment storage recovered", slog.String("component", "blobstore"))
	}
	return err
}

// Probe checks that the backend is reachable and writable where applicable.
func (s *Store) Probe(ctx context.Context) error {
	if s == nil || !s.enabled {
		return errors.New("attachment storage is not enabled")
	}
	return s.backend.probe(ctx)
}

func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}
