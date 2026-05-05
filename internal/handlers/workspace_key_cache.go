package handlers

import (
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"windshift/internal/repository"
)

// WorkspaceKeyCache provides an in-memory cache mapping workspace keys to IDs.
// This avoids DB lookups when resolving workspace keys in URL path parameters.
type WorkspaceKeyCache struct {
	mu   sync.RWMutex
	m    map[string]int // lowercase key → workspace ID
	repo *repository.WorkspaceRepository
}

// NewWorkspaceKeyCache creates and populates a new workspace key cache.
func NewWorkspaceKeyCache(repo *repository.WorkspaceRepository) *WorkspaceKeyCache {
	c := &WorkspaceKeyCache{repo: repo, m: make(map[string]int)}
	c.Load()
	return c
}

// Load queries all workspaces and rebuilds the cache.
func (c *WorkspaceKeyCache) Load() {
	pairs, err := c.repo.ListIDKeys()
	if err != nil {
		slog.Error("failed to load workspace key cache", slog.Any("error", err))
		return
	}

	m := make(map[string]int, len(pairs)*2)
	for _, p := range pairs {
		m[strings.ToLower(p.Key)] = p.ID
		// Also map the string representation of the ID so numeric strings resolve without Atoi
		m[strconv.Itoa(p.ID)] = p.ID
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
