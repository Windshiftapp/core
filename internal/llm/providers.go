package llm

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ProviderType identifies an LLM provider.
type ProviderType string

//go:embed llm_providers.json
var defaultProvidersJSON []byte

// providerRegistry holds the loaded provider list.
var (
	providerMu       sync.RWMutex
	providerRegistry []ProviderInfo
)

// ModelInfo describes a model offered by a provider.
type ModelInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MaxTokens int    `json:"max_tokens"`
}

// ProviderInfo describes a known LLM provider and its available models.
type ProviderInfo struct {
	Type      ProviderType `json:"type"`
	Name      string       `json:"name"`
	APIFormat string       `json:"api_format"`
	ChatPath  string       `json:"chat_path,omitempty"`
	BaseURL   string       `json:"base_url"`
	Models    []ModelInfo  `json:"models"`
}

// providersFile is the JSON structure for the providers file.
type providersFile struct {
	Providers []ProviderInfo `json:"providers"`
}

// LoadProviders reads and parses an LLM providers JSON file.
func LoadProviders(filePath string) error {
	data, err := os.ReadFile(filePath) //nolint:gosec // G304 — filePath from trusted CLI flag (-llm-providers)
	if err != nil {
		return fmt.Errorf("read providers file: %w", err)
	}
	return loadProvidersFromJSON(data)
}

// LoadDefaultProviders loads providers from the embedded default JSON.
func LoadDefaultProviders() {
	if err := loadProvidersFromJSON(defaultProvidersJSON); err != nil {
		// This should never happen since the embedded JSON is compiled in.
		panic(fmt.Sprintf("failed to parse embedded llm_providers.json: %v", err))
	}
}

// loadProvidersFromJSON parses JSON bytes into the provider registry.
func loadProvidersFromJSON(data []byte) error {
	var f providersFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse providers JSON: %w", err)
	}
	providerMu.Lock()
	providerRegistry = f.Providers
	providerMu.Unlock()
	return nil
}

// GetProviders returns the loaded list of providers.
func GetProviders() []ProviderInfo {
	providerMu.RLock()
	defer providerMu.RUnlock()
	return providerRegistry
}

// GetProvider looks up a provider by type. Returns nil if not found.
func GetProvider(pt ProviderType) *ProviderInfo {
	providerMu.RLock()
	defer providerMu.RUnlock()
	for i := range providerRegistry {
		if providerRegistry[i].Type == pt {
			return &providerRegistry[i]
		}
	}
	return nil
}

// KnownProviders returns the list of supported LLM providers.
// Kept for backward compatibility; delegates to GetProviders.
func KnownProviders() []ProviderInfo {
	return GetProviders()
}
