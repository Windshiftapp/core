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

		// Determine config path
		var configPath string
		if configInitGlobal {
			configPath = getGlobalConfigPath()
		} else {
			configPath = "./ws.toml"
		}

		// Check if config already exists
		if _, err := os.Stat(configPath); err == nil {
			fmt.Printf("Config already exists at %s. Overwrite? [y/N]: ", configPath)
			input, _ := reader.ReadString('\n') //nolint:errcheck // interactive user input
			input = strings.TrimSpace(strings.ToLower(input))
			if input != "y" && input != "yes" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		// Prompt for server URL
		fmt.Print("Windshift server URL (e.g., https://windshift.example.com): ")
		serverURL, _ = reader.ReadString('\n') //nolint:errcheck // interactive user input
		serverURL = strings.TrimSpace(serverURL)

		// Prompt for token
		fmt.Print("API token (crw_...): ")
		token, _ = reader.ReadString('\n') //nolint:errcheck // interactive user input
		token = strings.TrimSpace(token)

		// Prompt for default workspace (optional)
		fmt.Print("Default workspace key (optional, press Enter to skip): ")
		workspaceKey, _ = reader.ReadString('\n') //nolint:errcheck // interactive user input
		workspaceKey = strings.TrimSpace(workspaceKey)

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
		if !configInitGlobal && workspaceKey != "" {
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
		fmt.Print("\nVerify connection? [Y/n]: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "n" && input != "no" {
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

var configInitGlobal bool

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)

	configInitCmd.Flags().BoolVar(&configInitGlobal, "global", false, "create global config instead of project config")
}
