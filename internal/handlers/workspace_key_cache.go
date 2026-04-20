package handlers

import (
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"windshift/internal/database"
)

// WorkspaceKeyCache provides an in-memory cache mapping workspace keys to IDs.
// This avoids DB lookups when resolving workspace keys in URL path parameters.
type WorkspaceKeyCache struct {
	mu sync.RWMutex
	m  map[string]int // lowercase key → workspace ID
	db database.Database
}

// NewWorkspaceKeyCache creates and populates a new workspace key cache.
func NewWorkspaceKeyCache(db database.Database) *WorkspaceKeyCache {
	c := &WorkspaceKeyCache{db: db, m: make(map[string]int)}
	c.Load()
	return c
}

// Load queries all workspaces and rebuilds the cache.
func (c *WorkspaceKeyCache) Load() {
	rows, err := c.db.Query("SELECT id, key FROM workspaces")
	if err != nil {
		slog.Error("failed to load workspace key cache", slog.Any("error", err))
		return
	}
	defer func() { _ = rows.Close() }()

	m := make(map[string]int)
	for rows.Next() {
		var id int
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			slog.Error("failed to scan workspace row for key cache", slog.Any("error", err))
			continue
		}
		m[strings.ToLower(key)] = id
		// Also map the string representation of the ID so numeric strings resolve without Atoi
		m[strconv.Itoa(id)] = id
	}

	c.mu.Lock()
	c.m = m
	c.mu.Unlock()
}

// Resolve converts an ID-or-key string into a numeric workspace ID.
// It tries numeric parsing first, then falls back to a case-insensitive key lookup.
func (c *WorkspaceKeyCache) Resolve(idOrKey string) (int, bool) {
	if id, err := strconv.Atoi(idOrKey); err == nil {
		return id, true
	}
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	id, ok := c.m[strings.ToLower(idOrKey)]
	c.mu.RUnlock()
	return id, ok
}

// Invalidate rebuilds the cache from the database.
func (c *WorkspaceKeyCache) Invalidate() {
	c.Load()
}
