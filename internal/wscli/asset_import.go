package wscli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var assetImportCmd = &cobra.Command{
	Use:   "import <csv-path>",
	Short: "Queue an asset import from a CSV file",
	Long: `Stream a CSV file into the target set. The first row is treated as a
header. Columns named "title", "description", or "asset_tag"/"tag" map to
built-in fields; every other column is matched case-insensitively against
the asset type's declared custom field names. Rows missing a non-empty
title are counted as errors but don't abort the import — partial-success
is reported in the response (error_rows > 0, status=partial).

Examples:
  ws asset import laptops.csv --set 1 --type 4
  ws asset import laptops.csv -s 1 --type 4 --status 2 --category 7`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if assetImportTypeID == 0 {
			return fmt.Errorf("asset type id is required: use --type")
		}
		client, err := NewClient()
		if err != nil {
			return err
		}
		setID, err := resolveAssetSetID(client)
		if err != nil {
			return err
		}
		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", args[0], err)
		}
		defer func() { _ = f.Close() }()

		var statusID, categoryID *int
		if assetImportStatusID > 0 {
			v := assetImportStatusID
			statusID = &v
		}
		if assetImportCategoryID > 0 {
			v := assetImportCategoryID
			categoryID = &v
		}
		job, err := client.ImportAssetsCSV(setID, assetImportTypeID, statusID, categoryID, f.Name(), f)
		if err != nil {
			return fmt.Errorf("import failed: %w", err)
		}
		NewOutput().Print(job)
		return nil
	},
}

var (
	assetImportTypeID     int
	assetImportStatusID   int
	assetImportCategoryID int
)

func init() {
	assetCmd.AddCommand(assetImportCmd)
	assetImportCmd.Flags().IntVarP(&assetSetIDFlag, "set", "s", 0, "asset set id (default: only visible set, or is_default set)")
	assetImportCmd.Flags().IntVar(&assetImportTypeID, "type", 0, "asset type id to create all rows as (required)")
	assetImportCmd.Flags().IntVar(&assetImportStatusID, "status", 0, "default status id for every row")
	assetImportCmd.Flags().IntVar(&assetImportCategoryID, "category", 0, "default category id for every row")
}
