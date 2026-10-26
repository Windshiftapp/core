package wscli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

// discoveredProjectConfig is the ws.toml path actually loaded for this
// invocation (either an explicit --config, or the nearest ws.toml found by
// walking up from cwd). Empty when no project config was found. Other
// subcommands (config show, config refresh) read this so they target the
// real file instead of a stray ./ws.toml in a subdirectory.
var discoveredProjectConfig string

// configFileTrust tells loadConfigFile whether a file may set the server
// destination, or must first be classified by git.
type configFileTrust int

const (
	trustConfigFile configFileTrust = iota // global config or explicit --config
	trustDiscovered                        // found by walking up from cwd
)

// configWarning records server fields dropped from a git-tracked project
// config so they can be reported from the command that needs them.
type configWarning struct {
	path         string
	droppedURL   string
	droppedToken bool
}

var (
	cfgURLSource          string
	cfgWarnings           []configWarning
	configWarningsEmitted bool
)

func initConfig() {
	// Initialize config with defaults
	cfg = Config{
		StatusAliases: make(map[string]string),
	}
	discoveredProjectConfig = ""
	cfgURLSource = ""
	cfgWarnings = nil
	configWarningsEmitted = false

	// 1. Load global config first (lowest priority). It lives outside any
	// checkout, so it is always trusted.
	globalConfigPath := getGlobalConfigPath()
	if _, err := os.Stat(globalConfigPath); err == nil {
		loadConfigFile(globalConfigPath, trustConfigFile)
	}

	// 2. Load project config (overrides global). Walk up from cwd so that
	// running `ws` from a subdirectory of the repo still picks up the
	// repo-root ws.toml instead of silently falling back to the global token.
	// An explicit --config is trusted; a discovered file is only trusted when
	// it was authored locally rather than shipped with the checkout.
	projectConfigPath := cfgFile
	projectTrust := trustConfigFile
	if projectConfigPath == "" {
		projectConfigPath = findProjectConfigPath()
		projectTrust = trustDiscovered
	}
	if projectConfigPath != "" {
		if _, err := os.Stat(projectConfigPath); err == nil {
			loadConfigFile(projectConfigPath, projectTrust)
			discoveredProjectConfig = projectConfigPath
		}
	}

	// 3. Override with environment variables
	if envURL := os.Getenv("WS_URL"); envURL != "" {
		cfg.Server.URL = envURL
		cfgURLSource = "WS_URL"
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
		cfgURLSource = "--url"
	}
	if token != "" {
		cfg.Server.Token = token
	}
	if workspaceKey != "" {
		cfg.Defaults.WorkspaceKey = workspaceKey
	}
}

func loadConfigFile(path string, trust configFileTrust) {
	var fileCfg Config
	if _, err := toml.DecodeFile(path, &fileCfg); err != nil {
		return
	}

	// A discovered file may only set the server destination when it was
	// authored locally. Classify lazily so the common tokenless project file
	// never shells out to git.
	trusted := true
	if trust == trustDiscovered && (fileCfg.Server.URL != "" || fileCfg.Server.Token != "") {
		trusted = discoveredFileTrusted(path)
	}

	// Merge file config into main config
	if fileCfg.Server.URL != "" {
		if trusted {
			cfg.Server.URL = fileCfg.Server.URL
			cfgURLSource = path
		} else {
			recordIgnoredServerFields(path, fileCfg.Server.URL, false)
		}
	}
	if fileCfg.Server.Token != "" {
		if trusted {
			cfg.Server.Token = fileCfg.Server.Token
		} else {
			recordIgnoredServerFields(path, "", true)
		}
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

// discoveredFileTrusted reports whether a discovered ws.toml was authored
// locally. A file tracked by git arrived with the checkout and is untrusted.
// An untracked file in a worktree is the user's own. Outside a worktree, or
// when git is unavailable, the file is trusted so local directories keep
// working.
func discoveredFileTrusted(path string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := filepath.Dir(path)
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--is-inside-work-tree").Output() //nolint:gosec // git is the intended subprocess; the arguments are fixed
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return true
	}

	tracked := exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "--error-unmatch", "--", filepath.Base(path)) //nolint:gosec // git is the intended subprocess; the path is the local config file
	tracked.Stdout = io.Discard
	tracked.Stderr = io.Discard
	return tracked.Run() != nil
}

// recordIgnoredServerFields remembers that a git-tracked project config tried
// to set the server destination.
func recordIgnoredServerFields(path, droppedURL string, droppedToken bool) {
	for i := range cfgWarnings {
		if cfgWarnings[i].path != path {
			continue
		}
		if droppedURL != "" {
			cfgWarnings[i].droppedURL = droppedURL
		}
		cfgWarnings[i].droppedToken = cfgWarnings[i].droppedToken || droppedToken
		return
	}
	cfgWarnings = append(cfgWarnings, configWarning{path: path, droppedURL: droppedURL, droppedToken: droppedToken})
}

// emitConfigWarnings reports ignored server settings once per process. Call it
// where the settings would have been used, so `ws --help` stays quiet.
func emitConfigWarnings() {
	if configWarningsEmitted {
		return
	}
	configWarningsEmitted = true
	for _, w := range cfgWarnings {
		if w.droppedToken {
			_, _ = fmt.Fprintf(stderr, "warning: %s is committed to git and contains an API token, so anyone who can read the repo can use it. ws is ignoring that token. Remove it from ws.toml, add ws.toml to .gitignore, and keep the token on your machine with `ws init --global` or a password manager.\n", w.path)
		}
		if w.droppedURL == "" || w.droppedURL == cfg.Server.URL {
			continue
		}
		if cfg.Server.URL == "" {
			_, _ = fmt.Fprintf(stderr, "warning: %s is committed to git and sets the server URL to %s. A file that ships with a repo shouldn't decide where your API token is sent, so ws is ignoring it. Set WS_URL, pass --url, or run `ws init --global` to choose a server.\n", w.path, w.droppedURL)
			continue
		}
		_, _ = fmt.Fprintf(stderr, "warning: %s is committed to git and sets the server URL to %s. A file that ships with a repo shouldn't decide where your API token is sent, so ws is ignoring it and using %s from %s instead. To use %s, set WS_URL or edit ~/.config/ws/config.toml.\n", w.path, w.droppedURL, cfg.Server.URL, cfgURLSource, w.droppedURL)
	}
}

// validateAliasValue rejects likely packed TOML aliases. Numeric IDs and names
// are valid; comma/equal-sign values are reported as hand-edit mistakes.
func validateAliasValue(key, value string) string {
	if strings.ContainsAny(value, ",=") {
		return fmt.Sprintf("status_aliases.%s = %q looks malformed (contains , or =) — split into separate keys", key, value)
	}
	return ""
}

// findProjectConfigPath walks up from the current working directory looking
// for a ws.toml. Returns the absolute path of the first match, or "" if none
// is found before reaching the filesystem root.
func findProjectConfigPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, "ws.toml")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
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

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // private directories require owner execute permission
		return fmt.Errorf("failed to protect config directory: %w", err)
	}
	return writeConfigAtomically(path, config)
}

func saveProjectConfig(config Config, path string) error {
	return writeConfigAtomically(path, config)
}

func writeConfigAtomically(path string, config Config) error {
	return writeFileAtomically(path, func(w io.Writer) error {
		return toml.NewEncoder(w).Encode(config)
	})
}

func writeFileAtomically(path string, write func(io.Writer) error) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp") //nolint:gosec // destination directory is selected by the CLI user
	if err != nil {
		return fmt.Errorf("failed to create temporary config file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to protect temporary config file: %w", err)
	}
	if err := write(tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to encode config file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to sync temporary config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temporary config file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil { //nolint:gosec // both paths are in the caller-selected config directory
		return fmt.Errorf("failed to replace config file: %w", err)
	}
	return nil
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
