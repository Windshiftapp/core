package wscli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var assetSetCmd = &cobra.Command{
	Use:   "asset-set",
	Short: "Browse asset sets",
	Long: `Read-only commands for asset sets. Mutations (create / edit / archive,
roles, everyone-role, and set CRUD) remain available through the admin UI.`,
}

var assetSetListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List asset sets visible to the caller",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		sets, err := client.ListAssetSets()
		if err != nil {
			return fmt.Errorf("failed to list asset sets: %w", err)
		}
		NewOutput().Print(sets)
		return nil
	},
}

var assetSetGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get an asset set",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid asset set id %q: %w", args[0], err)
		}
		s, err := client.GetAssetSet(id)
		if err != nil {
			return fmt.Errorf("failed to get asset set: %w", err)
		}
		NewOutput().Print(s)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(assetSetCmd)
	assetSetCmd.AddCommand(assetSetListCmd)
	assetSetCmd.AddCommand(assetSetGetCmd)
}
