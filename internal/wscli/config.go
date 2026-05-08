package wscli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		if warning := validateAliasValue(k, v); warning != "" {
			_, _ = fmt.Fprintf(stderr, "warning: %s in %s — alias ignored\n", warning, path)
			continue
		}
		cfg.StatusAliases[k] = v
	}
}

// validateAliasValue rejects obviously malformed alias values (e.g. multiple
// aliases packed into one TOML value). Returns an empty string when the value
// is acceptable, otherwise a human-readable reason.
//
// Numeric IDs are the canonical form. Bare names (e.g. "Done") are accepted
// because ResolveStatusWithFallback handles the lookup. Anything containing
// "," or "=" is almost certainly a hand-edit mistake of the form
// `done = "Done, progress=In Progress"` — those are rejected loudly.
func validateAliasValue(key, value string) string {
	if strings.ContainsAny(value, ",=") {
		return fmt.Sprintf("status_aliases.%s = %q looks malformed (contains , or =) — split into separate keys", key, value)
	}
	return ""
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

// ResolveStatusWithFallback resolves a status input, falling back to the completed-statuses
// endpoint when the alias is non-numeric (stale) or when "done" has no alias.
// Returns comma-separated IDs for completed statuses, or the resolved value.
func (c *Config) ResolveStatusWithFallback(input string, client *Client) string {
	resolved := c.ResolveStatus(input)

	// If already numeric, use it directly
	if _, err := fmt.Sscanf(resolved, "%d", new(int)); err == nil {
		return resolved
	}

	// Non-numeric resolution — try completed-statuses endpoint for "done" alias
	if input == "done" || resolved == "done" {
		wsKey := c.GetEffectiveWorkspace()
		if wsKey == "" {
			return resolved
		}
		wsID, err := client.ResolveWorkspaceID(wsKey)
		if err != nil {
			return resolved
		}
		statuses, err := client.GetCompletedStatuses(wsID)
		if err != nil || len(statuses) == 0 {
			return resolved
		}
		ids := make([]string, 0, len(statuses))
		for _, s := range statuses {
			ids = append(ids, fmt.Sprintf("%d", s.ID))
		}
		return strings.Join(ids, ",")
	}

	return resolved
}

// GetEffectiveWorkspace returns the workspace key to use for queries
func (c *Config) GetEffectiveWorkspace() string {
	return c.Defaults.WorkspaceKey
}
