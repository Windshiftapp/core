package wscli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var assetTypeCmd = &cobra.Command{
	Use:   "asset-type",
	Short: "Browse asset types",
	Long: `Read-only commands for asset types (the schemas that asset rows in a set
follow). Mutations remain available through the admin UI.`,
}

var assetTypeListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List asset types in a set",
	Long: `List asset types defined on the target set.

Examples:
  ws asset-type ls --set 1`,
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		setID, err := resolveAssetSetID(client)
		if err != nil {
			return err
		}
		types, err := client.ListAssetTypes(setID)
		if err != nil {
			return fmt.Errorf("failed to list asset types: %w", err)
		}
		NewOutput().Print(types)
		return nil
	},
}

var assetTypeGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get an asset type",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid asset type id %q: %w", args[0], err)
		}
		t, err := client.GetAssetType(id)
		if err != nil {
			return fmt.Errorf("failed to get asset type: %w", err)
		}
		NewOutput().Print(t)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(assetTypeCmd)
	assetTypeCmd.AddCommand(assetTypeListCmd)
	assetTypeCmd.AddCommand(assetTypeGetCmd)
	assetTypeListCmd.Flags().IntVarP(&assetSetIDFlag, "set", "s", 0, "asset set id (default: only visible set, or is_default set)")
}
