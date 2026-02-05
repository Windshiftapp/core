package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Status management commands",
	Long:  `Commands for listing and viewing status information.`,
}

var statusListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List available statuses",
	Long: `List statuses available for items.

If workspace is specified, shows only statuses for that workspace.
Otherwise, shows all statuses in the system.

Examples:
  ws status ls                            # List all statuses
  ws status ls -w PROJ                    # List statuses for workspace PROJ`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		var statuses []Status

		// Check if workspace is specified
		wsKey := cfg.GetEffectiveWorkspace()
		if wsKey != "" {
			var wsID int
			wsID, err = client.ResolveWorkspaceID(wsKey)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			statuses, err = client.GetWorkspaceStatuses(wsID)
			if err != nil {
				return fmt.Errorf("failed to list workspace statuses: %w", err)
			}
		} else {
			statuses, err = client.ListStatuses()
			if err != nil {
				return fmt.Errorf("failed to list statuses: %w", err)
			}
		}

		output := NewOutput()
		output.Print(statuses)
		return nil
	},
}

var itemTypeCmd = &cobra.Command{
	Use:   "item-type",
	Short: "Item type commands",
	Long:  `Commands for listing and viewing item types.`,
}

var itemTypeListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List available item types",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		itemTypes, err := client.ListItemTypes()
		if err != nil {
			return fmt.Errorf("failed to list item types: %w", err)
		}

		output := NewOutput()
		output.Print(itemTypes)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.AddCommand(statusListCmd)

	rootCmd.AddCommand(itemTypeCmd)
	itemTypeCmd.AddCommand(itemTypeListCmd)
}
