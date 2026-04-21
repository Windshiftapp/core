package services

import (
	"log/slog"
	"sync"
	"time"
)

// ExecutionChain tracks state for cycle detection during action cascades.
// The chain is stored in memory and keyed by ExecutionChainID.
//
// ExecutedActions is accessed by multiple goroutines: ActionService and
// AssetActionService share a single ExecutionChainStore, and a workspace
// action that emits an asset-action event can cause both services to read
// and write the same chain concurrently. Callers MUST go through MarkExecuted
// and HasExecuted rather than touching the map directly.
type ExecutionChain struct {
	mu              sync.Mutex
	ExecutedActions map[string]bool // Set of action keys already executed (e.g. "workspace:5", "asset:3")
	CreatedAt       time.Time       // For TTL cleanup
}

// HasExecuted reports whether an action key has already run in this chain.
func (c *ExecutionChain) HasExecuted(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ExecutedActions[key]
}

// MarkExecuted records an action key as having run in this chain.
func (c *ExecutionChain) MarkExecuted(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ExecutedActions[key] = true
}

// ExecutionChainStore provides a shared, thread-safe store for execution chains
// across workspace, asset, and logbook action services.
type ExecutionChainStore struct {
	cache sync.Map
}

// NewExecutionChainStore creates a new shared execution chain store.
func NewExecutionChainStore() *ExecutionChainStore {
	return &ExecutionChainStore{}
}

// GetChain retrieves an execution chain from cache by its ID.
// Returns nil if the chain doesn't exist.
func (s *ExecutionChainStore) GetChain(chainID string) *ExecutionChain {
	if chainID == "" {
		return nil
	}
	if chain, ok := s.cache.Load(chainID); ok {
		return chain.(*ExecutionChain) //nolint:errcheck // type assertion always succeeds for cached chains
	}
	return nil
}

// CreateChain creates a new execution chain and stores it in the cache.
// Returns the newly created chain.
func (s *ExecutionChainStore) CreateChain(chainID string) *ExecutionChain {
	chain := &ExecutionChain{
		ExecutedActions: make(map[string]bool),
		CreatedAt:       time.Now(),
	}
	s.cache.Store(chainID, chain)
	return chain
}

// Cleanup removes stale execution chains older than 5 minutes.
func (s *ExecutionChainStore) Cleanup() {
	threshold := time.Now().Add(-5 * time.Minute)
	cleaned := 0
	s.cache.Range(func(key, value interface{}) bool {
		chain := value.(*ExecutionChain) //nolint:errcheck // type assertion always succeeds for cached chains
		if chain.CreatedAt.Before(threshold) {
			s.cache.Delete(key)
			cleaned++
		}
		return true
	})
	if cleaned > 0 {
		slog.Debug("cleaned up stale execution chains",
			slog.String("component", "chain-store"),
			slog.Int("count", cleaned),
		)
	}
}
