package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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

func init() {
	cobra.OnInitialize(initConfig)

	// Global persistent flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ./ws.toml or ~/.config/ws/config.toml)")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "json", "output format: json, table")
	rootCmd.PersistentFlags().StringVar(&serverURL, "url", "", "Windshift server URL")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "API token")
	rootCmd.PersistentFlags().StringVarP(&workspaceKey, "workspace", "w", "", "workspace key")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
