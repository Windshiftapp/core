package server

import (
	"context"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"windshift/internal/database"
)

// JiraHostAllowlist maintains an in-memory, periodically refreshed list of
// `scheme://host` origins derived from `jira_import_connections.instance_url`.
// The CSP middleware appends these to `img-src` so the SPA can load project
// avatars served directly from each configured Jira tenant.
type JiraHostAllowlist struct {
	db       database.Database
	interval time.Duration
	origins  atomic.Pointer[[]string]
}

// NewJiraHostAllowlist returns an allowlist that is safe to query immediately;
// it serves an empty origin list until the first refresh completes.
func NewJiraHostAllowlist(db database.Database, interval time.Duration) *JiraHostAllowlist {
	a := &JiraHostAllowlist{db: db, interval: interval}
	empty := []string{}
	a.origins.Store(&empty)
	return a
}

// Allowed returns the cached origin list. Callers must treat the slice as immutable.
func (a *JiraHostAllowlist) Allowed() []string {
	p := a.origins.Load()
	if p == nil {
		return nil
	}
	return *p
}

// Start performs an initial refresh and then refreshes on a ticker until stop closes.
func (a *JiraHostAllowlist) Start(stop <-chan struct{}) {
	if err := a.refresh(context.Background()); err != nil {
		slog.Warn("jira host allowlist initial refresh failed", "error", err)
	}
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	slog.Info("jira host allowlist scheduler started", "interval", a.interval)
	for {
		select {
		case <-ticker.C:
			if err := a.refresh(context.Background()); err != nil {
				slog.Warn("jira host allowlist refresh failed", "error", err)
			}
		case <-stop:
			slog.Info("jira host allowlist scheduler stopped")
			return
		}
	}
}

func (a *JiraHostAllowlist) refresh(ctx context.Context) error {
	rows, err := a.db.QueryContext(ctx, "SELECT instance_url FROM jira_import_connections")
	if err != nil {
		return err
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		origin, ok := normalizeOrigin(raw)
		if !ok {
			slog.Warn("jira host allowlist skipping malformed instance_url", "raw", raw)
			continue
		}
		seen[origin] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	origins := make([]string, 0, len(seen))
	for o := range seen {
		origins = append(origins, o)
	}
	sort.Strings(origins)
	a.origins.Store(&origins)
	slog.Debug("jira host allowlist refreshed", "count", len(origins))
	return nil
}

// normalizeOrigin reduces a stored instance_url to its CSP-compatible
// `scheme://host[:port]` form. Returns (origin, true) on success, ("", false)
// for malformed inputs or non-http(s) schemes.
func normalizeOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return "", false
	}
	// Drop default ports so the CSP entry matches the browser's canonical form.
	if h, port, ok := splitHostPort(host); ok && isDefaultPort(scheme, port) {
		host = h
	}
	return scheme + "://" + host, true
}

// splitHostPort splits "host:port" without surfacing errors for hosts that
// have no port (the common case). Returns ok=false when there's no port.
func splitHostPort(host string) (hostname, port string, ok bool) {
	i := strings.LastIndex(host, ":")
	if i < 0 {
		return host, "", false
	}
	// Skip cases like "[::1]" with no port.
	if strings.Contains(host[i:], "]") {
		return host, "", false
	}
	return host[:i], host[i+1:], true
}
