package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
		reader := bufio.NewReader(os.Stdin)

		// Non-interactive mode kicks in when explicitly requested, when stdin
		// is not a TTY (CI / piped input), or when both required fields were
		// already supplied via flags. Any prompt in non-interactive mode is a
		// fatal error rather than a stdin hang.
		nonInteractive := configInitNonInteractive || !stdinIsTTY() || (serverURL != "" && token != "")

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
				fmt.Printf("Overwriting config at %s\n", configPath)
			} else {
				fmt.Printf("Config already exists at %s. Overwrite? [y/N]: ", configPath)
				input, _ := reader.ReadString('\n') //nolint:errcheck // interactive user input
				input = strings.TrimSpace(strings.ToLower(input))
				if input != "y" && input != "yes" {
					fmt.Println("Aborted.")
					return nil
				}
			}
		}

		// Prompt for server URL (skip if provided via flag)
		if serverURL == "" {
			if nonInteractive {
				return fmt.Errorf("--url is required in non-interactive mode (also accepts WS_URL env var)")
			}
			fmt.Print("Windshift server URL (e.g., https://windshift.example.com): ")
			serverURL, _ = reader.ReadString('\n') //nolint:errcheck // interactive user input
			serverURL = strings.TrimSpace(serverURL)
		}

		// Prompt for token (skip if provided via flag)
		if token == "" {
			if nonInteractive {
				return fmt.Errorf("--token is required in non-interactive mode (also accepts WS_TOKEN env var)")
			}
			fmt.Print("API token (crw_...): ")
			token, _ = reader.ReadString('\n') //nolint:errcheck // interactive user input
			token = strings.TrimSpace(token)
		}

		// Prompt for default workspace (skip if provided via flag)
		if workspaceKey == "" && !nonInteractive {
			fmt.Print("Default workspace key (optional, press Enter to skip): ")
			workspaceKey, _ = reader.ReadString('\n') //nolint:errcheck // interactive user input
			workspaceKey = strings.TrimSpace(workspaceKey)
		}

		newConfig := Config{
			Server: ServerConfig{
				URL:   serverURL,
				Token: token,
			},
			Defaults: DefaultsConfig{
				WorkspaceKey: workspaceKey,
			},
			StatusAliases: map[string]string{},
		}

		// Add default status aliases if this is a project config
		if !configInitGlobal && workspaceKey != "" && !nonInteractive {
			fmt.Println("\nWould you like to configure status aliases? (These let you use shortcuts like 'done' instead of full status names)")
			fmt.Print("Configure aliases? [y/N]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			if input == "y" || input == "yes" {
				fmt.Println("\nEnter aliases in format: alias=Status Name (press Enter when done)")
				fmt.Println("Examples: done=To Review, progress=In Progress, blocked=On Hold")
				for {
					fmt.Print("Alias: ")
					alias, _ := reader.ReadString('\n')
					alias = strings.TrimSpace(alias)
					if alias == "" {
						break
					}
					parts := strings.SplitN(alias, "=", 2)
					if len(parts) == 2 {
						newConfig.StatusAliases[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
					} else {
						fmt.Println("Invalid format. Use: alias=Status Name")
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

		fmt.Printf("Config saved to %s\n", configPath)

		// Verify connection
		verify := true
		if !nonInteractive {
			fmt.Print("\nVerify connection? [Y/n]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			verify = input != "n" && input != "no"
		}
		if verify {
			// Temporarily apply new config
			cfg = newConfig
			client, err := NewClient()
			if err != nil {
				fmt.Printf("Warning: %s\n", err)
				return nil
			}
			user, err := client.GetCurrentUser()
			if err != nil {
				fmt.Printf("Warning: Could not verify connection: %s\n", err)
				return nil
			}
			fmt.Printf("Connected as: %s (%s)\n", user.FullName, user.Email)
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
  3. Project config (./ws.toml)
  4. Global config (~/.config/ws/config.toml)`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			result.Sources.ProjectConfig = "./ws.toml"

			output := NewOutput()
			output.Print(result)
		} else {
			fmt.Println("=== Effective Configuration ===")
			fmt.Printf("Server URL:        %s\n", cfg.Server.URL)
			fmt.Printf("Token:             %s\n", maskedToken)
			fmt.Printf("Default Workspace: %s\n", cfg.Defaults.WorkspaceKey)
			if cfg.Cache.UserID > 0 {
				fmt.Printf("Cached User ID:    %d\n", cfg.Cache.UserID)
			}
			fmt.Println("\n=== Config Sources ===")
			fmt.Printf("Global:  %s\n", getGlobalConfigPath())
			fmt.Printf("Project: ./ws.toml\n")
			if len(cfg.StatusAliases) > 0 {
				fmt.Println("\n=== Status Aliases ===")
				for alias, status := range cfg.StatusAliases {
					fmt.Printf("  %s -> %s\n", alias, status)
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

		// Save back to project config
		projectConfig := Config{
			Server:        cfg.Server,
			Defaults:      cfg.Defaults,
			Cache:         cfg.Cache,
			StatusAliases: cfg.StatusAliases,
		}
		if err := saveProjectConfig(projectConfig, "./ws.toml"); err != nil {
			return fmt.Errorf("failed to save ws.toml: %w", err)
		}

		fmt.Println("Refreshed status aliases in ws.toml:")
		for alias, id := range cfg.StatusAliases {
			fmt.Printf("  %s -> %s\n", alias, id)
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
		fmt.Printf("Create a token at %s/profile and paste it here.\n", strings.TrimSuffix(instanceURL, "/"))
	}
	fmt.Print("API token (crw_...): ")
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

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configRefreshCmd)

	configInitCmd.Flags().BoolVar(&configInitGlobal, "global", false, "create global config instead of project config")
	configInitCmd.Flags().BoolVar(&configInitNonInteractive, "non-interactive", false, "fail instead of prompting when required fields are missing (auto-detected when stdin is not a TTY)")
}
