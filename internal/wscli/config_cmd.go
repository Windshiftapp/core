package wscli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration commands",
	Long:  `Commands for managing CLI configuration.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration file",
	Long: `Initialize a new configuration file.

By default, creates a project-local config (./ws.toml).
Use --global to create the global config (~/.config/ws/config.toml).

Examples:
  ws config init                          # Create ./ws.toml
  ws config init --global                 # Create ~/.config/ws/config.toml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(stdin)

		// Flags win over the environment, which is a documented config source.
		// Read into locals so repeated in-process runs do not leak values into
		// the flag-bound globals.
		instanceURL := serverURL
		if instanceURL == "" {
			instanceURL = os.Getenv("WS_URL")
		}
		apiToken := token
		if apiToken == "" {
			apiToken = os.Getenv("WS_TOKEN")
		}
		workspace := workspaceKey
		if workspace == "" {
			workspace = os.Getenv("WS_WORKSPACE")
		}

		// Non-interactive mode kicks in when explicitly requested, when stdin
		// is not a TTY (CI / piped input), or when both required fields were
		// already supplied. Any prompt in non-interactive mode is a fatal
		// error rather than a stdin hang.
		nonInteractive := configInitNonInteractive || !stdinIsTTY() || (instanceURL != "" && apiToken != "")

		// Determine config path
		var configPath string
		if configInitGlobal {
			configPath = getGlobalConfigPath()
		} else {
			configPath = "./ws.toml"
		}

		// Check if config already exists
		if _, err := os.Stat(configPath); err == nil {
			if nonInteractive {
				// Auto-overwrite in non-interactive mode
				_, _ = fmt.Fprintf(stdout, "Overwriting config at %s\n", configPath)
			} else {
				_, _ = fmt.Fprintf(stdout, "Config already exists at %s. Overwrite? [y/N]: ", configPath)
				input, _ := reader.ReadString('\n') //nolint:errcheck // interactive user input
				input = strings.TrimSpace(strings.ToLower(input))
				if input != "y" && input != "yes" {
					_, _ = fmt.Fprintln(stdout, "Aborted.")
					return nil
				}
			}
		}

		// Prompt for server URL (skip if provided via flag or environment)
		if instanceURL == "" {
			if nonInteractive {
				return fmt.Errorf("--url is required in non-interactive mode (also accepts WS_URL env var)")
			}
			_, _ = fmt.Fprint(stdout, "Windshift server URL (e.g., https://windshift.example.com): ")
			instanceURL, _ = reader.ReadString('\n') //nolint:errcheck // interactive user input
			instanceURL = strings.TrimSpace(instanceURL)
		}

		// Prompt for token (skip if provided via flag or environment)
		if apiToken == "" {
			if nonInteractive {
				return fmt.Errorf("--token is required in non-interactive mode (also accepts WS_TOKEN env var)")
			}
			_, _ = fmt.Fprint(stdout, "API token (crw_...): ")
			apiToken, _ = reader.ReadString('\n') //nolint:errcheck // interactive user input
			apiToken = strings.TrimSpace(apiToken)
		}

		// Prompt for default workspace (skip if provided via flag or environment)
		if workspace == "" && !nonInteractive {
			_, _ = fmt.Fprint(stdout, "Default workspace key (optional, press Enter to skip): ")
			workspace, _ = reader.ReadString('\n') //nolint:errcheck // interactive user input
			workspace = strings.TrimSpace(workspace)
		}

		newConfig := Config{
			Server: ServerConfig{
				URL:   instanceURL,
				Token: apiToken,
			},
			Defaults: DefaultsConfig{
				WorkspaceKey: workspace,
			},
			StatusAliases: map[string]string{},
		}

		// Add default status aliases if this is a project config
		if !configInitGlobal && workspace != "" && !nonInteractive {
			_, _ = fmt.Fprintln(stdout, "\nWould you like to configure status aliases? (These let you use shortcuts like 'done' instead of full status names)")
			_, _ = fmt.Fprint(stdout, "Configure aliases? [y/N]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			if input == "y" || input == "yes" {
				_, _ = fmt.Fprintln(stdout, "\nEnter aliases in format: alias=Status Name (press Enter when done)")
				_, _ = fmt.Fprintln(stdout, "Examples: done=To Review, progress=In Progress, blocked=On Hold")
				for {
					_, _ = fmt.Fprint(stdout, "Alias: ")
					alias, _ := reader.ReadString('\n')
					alias = strings.TrimSpace(alias)
					if alias == "" {
						break
					}
					parts := strings.SplitN(alias, "=", 2)
					if len(parts) == 2 {
						newConfig.StatusAliases[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
					} else {
						_, _ = fmt.Fprintln(stdout, "Invalid format. Use: alias=Status Name")
					}
				}
			}
		}

		// Save config
		var err error
		if configInitGlobal {
			err = saveGlobalConfig(newConfig)
		} else {
			err = saveProjectConfig(newConfig, configPath)
		}
		if err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		_, _ = fmt.Fprintf(stdout, "Config saved to %s\n", configPath)

		// Verify connection
		verify := true
		if !nonInteractive {
			_, _ = fmt.Fprint(stdout, "\nVerify connection? [Y/n]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			verify = input != "n" && input != "no"
		}
		if verify {
			// Temporarily apply new config
			cfg = newConfig
			client, err := NewClient()
			if err != nil {
				_, _ = fmt.Fprintf(stdout, "Warning: %s\n", err)
				return nil
			}
			user, err := client.GetCurrentUser()
			if err != nil {
				_, _ = fmt.Fprintf(stdout, "Warning: Could not verify connection: %s\n", err)
				return nil
			}
			_, _ = fmt.Fprintf(stdout, "Connected as: %s (%s)\n", user.FullName, user.Email)
		}

		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show effective configuration",
	Long: `Display the current effective configuration.

This shows the merged configuration from all sources:
  1. CLI flags (highest priority)
  2. Environment variables
  3. Project config (nearest ws.toml walking up from cwd)
  4. Global config (~/.config/ws/config.toml)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		emitConfigWarnings()

		// Mask token for display
		maskedToken := cfg.Server.Token
		if len(maskedToken) > 8 {
			maskedToken = maskedToken[:4] + "..." + maskedToken[len(maskedToken)-4:]
		}

		if outputFormat == "json" {
			result := struct {
				Server struct {
					URL   string `json:"url"`
					Token string `json:"token"`
				} `json:"server"`
				Defaults struct {
					WorkspaceKey string `json:"workspace_key"`
				} `json:"defaults"`
				Cache struct {
					UserID int `json:"user_id,omitempty"`
				} `json:"cache,omitempty"`
				StatusAliases map[string]string `json:"status_aliases,omitempty"`
				Sources       struct {
					GlobalConfig  string `json:"global_config"`
					ProjectConfig string `json:"project_config"`
				} `json:"sources"`
			}{
				StatusAliases: cfg.StatusAliases,
			}
			result.Server.URL = cfg.Server.URL
			result.Server.Token = maskedToken
			result.Defaults.WorkspaceKey = cfg.Defaults.WorkspaceKey
			result.Cache.UserID = cfg.Cache.UserID
			result.Sources.GlobalConfig = getGlobalConfigPath()
			result.Sources.ProjectConfig = discoveredProjectConfig
			if result.Sources.ProjectConfig == "" {
				result.Sources.ProjectConfig = "(none found)"
			}

			output := NewOutput()
			output.Print(result)
		} else {
			_, _ = fmt.Fprintln(stdout, "=== Effective Configuration ===")
			_, _ = fmt.Fprintf(stdout, "Server URL:        %s\n", cfg.Server.URL)
			_, _ = fmt.Fprintf(stdout, "Token:             %s\n", maskedToken)
			_, _ = fmt.Fprintf(stdout, "Default Workspace: %s\n", cfg.Defaults.WorkspaceKey)
			if cfg.Cache.UserID > 0 {
				_, _ = fmt.Fprintf(stdout, "Cached User ID:    %d\n", cfg.Cache.UserID)
			}
			_, _ = fmt.Fprintln(stdout, "\n=== Config Sources ===")
			_, _ = fmt.Fprintf(stdout, "Global:  %s\n", getGlobalConfigPath())
			projectDisplay := discoveredProjectConfig
			if projectDisplay == "" {
				projectDisplay = "(none found)"
			}
			_, _ = fmt.Fprintf(stdout, "Project: %s\n", projectDisplay)
			if len(cfg.StatusAliases) > 0 {
				_, _ = fmt.Fprintln(stdout, "\n=== Status Aliases ===")
				for alias, status := range cfg.StatusAliases {
					_, _ = fmt.Fprintf(stdout, "  %s -> %s\n", alias, status)
				}
			}
		}
		return nil
	},
}

var configRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh status aliases from workspace",
	Long: `Re-fetch workspace statuses and regenerate status aliases with numeric IDs.

This is useful when statuses have been renamed on the server or when aliases
contain stale name-based values instead of numeric IDs.

Examples:
  ws config refresh                       # Refresh aliases in ./ws.toml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsKey := cfg.GetEffectiveWorkspace()
		if wsKey == "" {
			return fmt.Errorf("workspace is required: use -w flag or set defaults.workspace_key in config")
		}

		wsID, err := client.ResolveWorkspaceID(wsKey)
		if err != nil {
			return fmt.Errorf("failed to resolve workspace: %w", err)
		}

		statuses, err := client.GetWorkspaceStatuses(wsID)
		if err != nil {
			return fmt.Errorf("failed to get statuses: %w", err)
		}

		// Regenerate aliases with numeric IDs
		cfg.StatusAliases = generateDefaultAliases(statuses)

		// Write back to the same ws.toml we loaded so refresh from a
		// subdirectory updates the repo-root file rather than creating a
		// stray one in cwd. Fall back to ./ws.toml if none was discovered.
		targetPath := discoveredProjectConfig
		if targetPath == "" {
			targetPath = "./ws.toml"
		}

		// Keep the file's own [server] section. Writing cfg.Server would copy
		// the global or environment token into the project file.
		var existing Config
		if _, err := toml.DecodeFile(targetPath, &existing); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to read %s: %w", targetPath, err)
		}

		projectConfig := Config{
			Server:        existing.Server,
			Defaults:      cfg.Defaults,
			Cache:         cfg.Cache,
			StatusAliases: cfg.StatusAliases,
		}
		if err := saveProjectConfig(projectConfig, targetPath); err != nil {
			return fmt.Errorf("failed to save %s: %w", targetPath, err)
		}

		_, _ = fmt.Fprintf(stdout, "Refreshed status aliases in %s:\n", targetPath)
		for alias, id := range cfg.StatusAliases {
			_, _ = fmt.Fprintf(stdout, "  %s -> %s\n", alias, id)
		}
		return nil
	},
}

var (
	configInitGlobal         bool
	configInitNonInteractive bool
)

// stdinIsTTY reports whether stdin is connected to a terminal. Returns false
// when stdin is a pipe, file, or otherwise non-character-device — which is
// the heuristic for "this is automation, do not prompt".
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// promptForToken asks the user to paste a personal API token. Used by the
// manual onboarding fallback (both `ws config init` and `ws init --manual`).
// Returns the trimmed token or a clean error if the environment is not
// interactive.
func promptForToken(reader *bufio.Reader, instanceURL string) (string, error) {
	if reader == nil {
		return "", fmt.Errorf("internal error: no input reader")
	}
	if !stdinIsTTY() {
		return "", fmt.Errorf("no TTY; pass --token to provide the API token")
	}
	if instanceURL != "" {
		// #nosec G705 -- writing to a CLI terminal, not HTML; G705 is checking for an XSS sink that doesn't exist here
		_, _ = fmt.Fprintf(stdout, "Create a token at %s/profile and paste it here.\n", strings.TrimSuffix(instanceURL, "/"))
	}
	_, _ = fmt.Fprint(stdout, "API token (crw_...): ")
	raw, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(raw)
	if t == "" {
		return "", fmt.Errorf("empty token")
	}
	return t, nil
}

var configDocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Regenerate WINDSHIFT.md from the current workspace",
	Long: `Refetch workspace context (statuses, item types, transitions) and
rewrite WINDSHIFT.md with the live CLI surface plus this workspace's
taxonomy. Does not touch ws.toml, AGENTS.md, or CLAUDE.md — use
` + "`ws init`" + ` for full project setup.

Writes to the directory containing the active ws.toml (the one found by
walking up from cwd) so running this from a subdirectory still updates
the canonical WINDSHIFT.md at the repo root. Falls back to cwd when no
project config is loaded.

WINDSHIFT.md is workspace- and token-specific; gitignore it and load it
into agent context via a CLAUDE.md @WINDSHIFT.md import (or equivalent).

Examples:
  ws config docs                          # Rewrite WINDSHIFT.md next to ws.toml`,
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsKey := cfg.GetEffectiveWorkspace()
		if wsKey == "" {
			return fmt.Errorf("workspace is required: use -w flag or set defaults.workspace_key in config")
		}
		wsID, err := client.ResolveWorkspaceID(wsKey)
		if err != nil {
			return fmt.Errorf("failed to resolve workspace: %w", err)
		}
		wsCtx, err := fetchWorkspaceContext(client, wsID)
		if err != nil {
			return err
		}

		// Anchor the output next to the active ws.toml so running from a
		// subdir still rewrites the repo-root copy (mirrors `config refresh`).
		dir := "."
		if discoveredProjectConfig != "" {
			dir = filepath.Dir(discoveredProjectConfig)
		}
		path := filepath.Join(dir, "WINDSHIFT.md")
		if err := writeWindshiftMD(client, wsCtx, cfg.StatusAliases, path); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "Wrote %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configRefreshCmd)
	configCmd.AddCommand(configDocsCmd)

	configInitCmd.Flags().BoolVar(&configInitGlobal, "global", false, "create global config instead of project config")
	configInitCmd.Flags().BoolVar(&configInitNonInteractive, "non-interactive", false, "fail instead of prompting when required fields are missing (auto-detected when stdin is not a TTY)")
}
