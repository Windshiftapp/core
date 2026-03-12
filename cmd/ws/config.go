package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config represents the CLI configuration
type Config struct {
	Server        ServerConfig      `toml:"server"`
	Defaults      DefaultsConfig    `toml:"defaults"`
	Cache         CacheConfig       `toml:"cache"`
	StatusAliases map[string]string `toml:"status_aliases"`
}

type ServerConfig struct {
	URL   string `toml:"url"`
	Token string `toml:"token"`
}

type DefaultsConfig struct {
	WorkspaceKey string `toml:"workspace_key"`
}

type CacheConfig struct {
	UserID int `toml:"user_id"`
}

var cfg Config

func initConfig() {
	// Initialize config with defaults
	cfg = Config{
		StatusAliases: make(map[string]string),
	}

	// 1. Load global config first (lowest priority)
	globalConfigPath := getGlobalConfigPath()
	if _, err := os.Stat(globalConfigPath); err == nil {
		loadConfigFile(globalConfigPath)
	}

	// 2. Load project config (overrides global)
	projectConfigPath := "./ws.toml"
	if cfgFile != "" {
		projectConfigPath = cfgFile
	}
	if _, err := os.Stat(projectConfigPath); err == nil {
		loadConfigFile(projectConfigPath)
	}

	// 3. Override with environment variables
	if envURL := os.Getenv("WS_URL"); envURL != "" {
		cfg.Server.URL = envURL
	}
	if envToken := os.Getenv("WS_TOKEN"); envToken != "" {
		cfg.Server.Token = envToken
	}
	if envWorkspace := os.Getenv("WS_WORKSPACE"); envWorkspace != "" {
		cfg.Defaults.WorkspaceKey = envWorkspace
	}

	// 4. Override with CLI flags (highest priority)
	if serverURL != "" {
		cfg.Server.URL = serverURL
	}
	if token != "" {
		cfg.Server.Token = token
	}
	if workspaceKey != "" {
		cfg.Defaults.WorkspaceKey = workspaceKey
	}
}

func loadConfigFile(path string) {
	var fileCfg Config
	if _, err := toml.DecodeFile(path, &fileCfg); err != nil {
		return
	}

	// Merge file config into main config
	if fileCfg.Server.URL != "" {
		cfg.Server.URL = fileCfg.Server.URL
	}
	if fileCfg.Server.Token != "" {
		cfg.Server.Token = fileCfg.Server.Token
	}
	if fileCfg.Defaults.WorkspaceKey != "" {
		cfg.Defaults.WorkspaceKey = fileCfg.Defaults.WorkspaceKey
	}
	if fileCfg.Cache.UserID != 0 {
		cfg.Cache.UserID = fileCfg.Cache.UserID
	}
	for k, v := range fileCfg.StatusAliases {
		cfg.StatusAliases[k] = v
	}
}

func getGlobalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "ws", "config.toml")
}

func saveGlobalConfig(config Config) error {
	path := getGlobalConfigPath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	f, err := os.Create(path) //nolint:gosec // G304 — path from getGlobalConfigPath() (user home dir)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer func() { _ = f.Close() }()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(config)
}

func saveProjectConfig(config Config, path string) error {
	f, err := os.Create(path) //nolint:gosec // G304 — path from CLI user's own config args
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer func() { _ = f.Close() }()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(config)
}

// ResolveStatus resolves a status input using aliases, falling back to the input itself
func (c *Config) ResolveStatus(input string) string {
	if resolved, ok := c.StatusAliases[input]; ok {
		return resolved
	}
	return input
}

// GetEffectiveWorkspace returns the workspace key to use for queries
func (c *Config) GetEffectiveWorkspace() string {
	return c.Defaults.WorkspaceKey
}
