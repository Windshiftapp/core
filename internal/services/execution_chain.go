package services

import (
	"log/slog"
	"sync"
	"time"
)

// ExecutionChain tracks state for cycle detection during action cascades.
// The chain is stored in memory and keyed by ExecutionChainID.
type ExecutionChain struct {
	ExecutedActions map[string]bool // Set of action keys already executed (e.g. "workspace:5", "asset:3")
	CreatedAt       time.Time       // For TTL cleanup
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
