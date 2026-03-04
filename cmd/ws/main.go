package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Build info (set via ldflags)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	// Global flags
	cfgFile      string
	outputFormat string
	serverURL    string
	token        string
	workspaceKey string
)

var rootCmd = &cobra.Command{
	Use:   "ws",
	Short: "Windshift CLI - Task and test management from the command line",
	Long: `Windshift CLI (ws) provides efficient task and test management
for developers and Claude Code integration.

Configuration priority:
  1. CLI flags (--url, --token, --workspace)
  2. Environment variables (WS_URL, WS_TOKEN, WS_WORKSPACE)
  3. Project config (./ws.toml)
  4. Global config (~/.config/ws/config.toml)`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ws %s (commit: %s, built: %s)\n", version, commit, date)
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for ws.

To load completions:

Bash:
  $ source <(ws completion bash)
  # To persist, add to ~/.bashrc or install system-wide:
  $ ws completion bash > /etc/bash_completion.d/ws

Zsh:
  $ ws completion zsh > "${fpath[1]}/_ws"

Fish:
  $ ws completion fish > ~/.config/fish/completions/ws.fish
  # Fish completions include descriptions for each command

PowerShell:
  PS> ws completion powershell | Out-String | Invoke-Expression`,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			_ = rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			_ = rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			_ = rootCmd.GenFishCompletion(os.Stdout, true) // true = include descriptions
		case "powershell":
			_ = rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global persistent flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ./ws.toml or ~/.config/ws/config.toml)")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "json", "output format: json, table, csv")
	rootCmd.PersistentFlags().StringVar(&serverURL, "url", "", "Windshift server URL")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "API token")
	rootCmd.PersistentFlags().StringVarP(&workspaceKey, "workspace", "w", "", "workspace key")

	// Add version and completion commands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(completionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
