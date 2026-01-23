package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
	Long:  `Commands for listing and viewing workspace information.`,
}

var workspaceListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List accessible workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		workspaces, err := client.ListWorkspaces()
		if err != nil {
			return fmt.Errorf("failed to list workspaces: %w", err)
		}

		output := NewOutput()
		output.Print(workspaces)
		return nil
	},
}

var workspaceInfoCmd = &cobra.Command{
	Use:   "info [workspace]",
	Short: "Show workspace configuration",
	Long: `Show detailed workspace information including statuses, item types, and workflow.

Examples:
  ws workspace info                       # Show info for default workspace
  ws workspace info PROJ                  # Show info for workspace PROJ
  ws workspace info --key PROJ            # Same as above`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		// Determine workspace key
		wsKey := workspaceInfoKey
		if wsKey == "" && len(args) > 0 {
			wsKey = args[0]
		}
		if wsKey == "" {
			wsKey = cfg.GetEffectiveWorkspace()
		}
		if wsKey == "" {
			return fmt.Errorf("workspace is required: use -w flag, provide argument, or set defaults.workspace_key in config")
		}

		wsID, err := client.ResolveWorkspaceID(wsKey)
		if err != nil {
			return fmt.Errorf("failed to resolve workspace: %w", err)
		}

		// Get workspace details
		workspace, err := client.GetWorkspace(wsID)
		if err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}

		// Get statuses
		statuses, err := client.GetWorkspaceStatuses(wsID)
		if err != nil {
			return fmt.Errorf("failed to get statuses: %w", err)
		}

		// Get item types
		itemTypes, err := client.ListItemTypes()
		if err != nil {
			return fmt.Errorf("failed to get item types: %w", err)
		}

		// Get workflows
		workflows, err := client.ListWorkflows()
		if err != nil {
			return fmt.Errorf("failed to get workflows: %w", err)
		}

		if outputFormat == "json" {
			result := struct {
				Workspace *Workspace  `json:"workspace"`
				Statuses  []Status    `json:"statuses"`
				ItemTypes []ItemType  `json:"item_types"`
				Workflows []Workflow  `json:"workflows"`
			}{
				Workspace: workspace,
				Statuses:  statuses,
				ItemTypes: itemTypes,
				Workflows: workflows,
			}
			output := NewOutput()
			output.Print(result)
		} else {
			output := NewOutput()
			fmt.Println("=== Workspace ===")
			output.Print(workspace)
			fmt.Println("\n=== Item Types ===")
			output.Print(itemTypes)
			fmt.Println("\n=== Statuses ===")
			output.Print(statuses)
			fmt.Printf("\n=== Workflows (%d) ===\n", len(workflows))
			for _, w := range workflows {
				defaultStr := ""
				if w.IsDefault {
					defaultStr = " (default)"
				}
				fmt.Printf("  - %s%s\n", w.Name, defaultStr)
			}
		}
		return nil
	},
}

var workspaceInfoKey string

func init() {
	rootCmd.AddCommand(workspaceCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceInfoCmd)

	workspaceInfoCmd.Flags().StringVar(&workspaceInfoKey, "key", "", "workspace key")
}
