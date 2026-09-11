package wscli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var assetCmd = &cobra.Command{
	Use:   "asset",
	Short: "Manage assets",
	Long: `Commands for listing, getting, creating, editing, and deleting assets.

Assets live inside an asset management set. Use --set / -s to target a
specific set; defaults to the user's primary set when unset.`,
}

var assetListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List assets in a set",
	Long: `List assets in the target set, with optional filters.

Examples:
  ws asset ls --set 1
  ws asset ls -s 1 --type 4 --status 2
  ws asset ls -s 1 -q laptop                     # Free-text title search
  ws asset ls -s 1 --limit 200 --page 2          # Pagination`,
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		setID, err := resolveAssetSetID(client)
		if err != nil {
			return err
		}
		filters := map[string]string{
			"type_id":     assetListType,
			"category_id": assetListCategory,
			"status_id":   assetListStatus,
			"q":           assetListSearch,
		}
		if assetListLimit > 0 {
			filters["limit"] = fmt.Sprintf("%d", assetListLimit)
		}
		if assetListPage > 0 {
			filters["page"] = fmt.Sprintf("%d", assetListPage)
		}
		resp, err := client.ListAssets(setID, filters)
		if err != nil {
			return fmt.Errorf("failed to list assets: %w", err)
		}
		NewOutput().Print(resp)
		return nil
	},
}

var assetGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get an asset",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid asset id %q: %w", args[0], err)
		}
		a, err := client.GetAsset(id)
		if err != nil {
			return fmt.Errorf("failed to get asset: %w", err)
		}
		NewOutput().Print(a)
		return nil
	},
}

var assetCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new asset",
	Long: `Create a new asset in the target set.

Examples:
  ws asset create -s 1 --type 4 -t "Lenovo X1"
  ws asset create -s 1 --type 4 -t "Lenovo X1" --tag LAP-001 --status 2`,
	RunE: func(_ *cobra.Command, _ []string) error {
		if assetCreateTitle == "" {
			return fmt.Errorf("title is required: use -t or --title")
		}
		if assetCreateTypeID == 0 {
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
		req := AssetCreateRequest{
			Title:       assetCreateTitle,
			Description: assetCreateDesc,
			AssetTag:    assetCreateTag,
			AssetTypeID: assetCreateTypeID,
		}
		if assetCreateCategoryID > 0 {
			cid := assetCreateCategoryID
			req.CategoryID = &cid
		}
		if assetCreateStatusID > 0 {
			sid := assetCreateStatusID
			req.StatusID = &sid
		}
		a, err := client.CreateAsset(setID, req)
		if err != nil {
			return fmt.Errorf("failed to create asset: %w", err)
		}
		if outputFormat == "" || outputFormat == "table" {
			_, _ = fmt.Fprintf(stdout, "Created asset %d (%s)\n", a.ID, a.Title)
			return nil
		}
		NewOutput().Print(a)
		return nil
	},
}

var assetEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit an asset",
	Long: `Partial-update an asset. Only flags you pass are written; everything else
is preserved. Pass --category 0 / --status 0 to clear the column.

Examples:
  ws asset edit 42 -t "Renamed"
  ws asset edit 42 --status 3
  ws asset edit 42 --category 0           # Clear the category`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid asset id %q: %w", args[0], err)
		}
		req := AssetUpdateRequest{}
		hasUpdate := false
		if cmd.Flags().Changed("title") {
			req.Title = &assetEditTitle
			hasUpdate = true
		}
		if cmd.Flags().Changed("description") {
			req.Description = &assetEditDesc
			hasUpdate = true
		}
		if cmd.Flags().Changed("tag") {
			req.AssetTag = &assetEditTag
			hasUpdate = true
		}
		if cmd.Flags().Changed("type") {
			req.AssetTypeID = &assetEditTypeID
			hasUpdate = true
		}
		if cmd.Flags().Changed("category") {
			req.CategoryID = &assetEditCategoryID
			hasUpdate = true
		}
		if cmd.Flags().Changed("status") {
			req.StatusID = &assetEditStatusID
			hasUpdate = true
		}
		if !hasUpdate {
			return fmt.Errorf("no updates specified. Use --title, --description, --tag, --type, --category, or --status")
		}
		a, err := client.UpdateAsset(id, req)
		if err != nil {
			return fmt.Errorf("failed to update asset: %w", err)
		}
		if outputFormat == "" || outputFormat == "table" {
			_, _ = fmt.Fprintf(stdout, "Updated asset %d (%s)\n", a.ID, a.Title)
			return nil
		}
		NewOutput().Print(a)
		return nil
	},
}

// Note on ws asset rm: deliberately not exposed. Asset delete requires
// assets:delete, which isn't in DefaultAgentScopes — a CLI verb that
// 403s out of the box for the default token is a footgun. Operators who
// have minted a token with `--scopes assets:delete` can call
// DELETE /rest/api/v2/assets/{id} via curl, or use the cookie-auth
// admin UI. Client.DeleteAsset stays available for embedders.

// Flags for asset commands.
var (
	assetSetIDFlag    int
	assetListType     string
	assetListCategory string
	assetListStatus   string
	assetListSearch   string
	assetListLimit    int
	assetListPage     int

	assetCreateTitle      string
	assetCreateDesc       string
	assetCreateTag        string
	assetCreateTypeID     int
	assetCreateCategoryID int
	assetCreateStatusID   int

	assetEditTitle      string
	assetEditDesc       string
	assetEditTag        string
	assetEditTypeID     int
	assetEditCategoryID int
	assetEditStatusID   int
)

// resolveAssetSetID picks the set the asset commands should operate on:
// the --set / -s flag wins; otherwise the user's primary asset set; if
// neither is set and exactly one set is visible, that one. Errors if
// ambiguous so the user has to make the choice explicit.
func resolveAssetSetID(client *Client) (int, error) {
	if assetSetIDFlag > 0 {
		return assetSetIDFlag, nil
	}
	sets, err := client.ListAssetSets()
	if err != nil {
		return 0, fmt.Errorf("failed to list asset sets: %w", err)
	}
	if len(sets) == 0 {
		return 0, fmt.Errorf("no asset sets visible to this token")
	}
	if len(sets) == 1 {
		return sets[0].ID, nil
	}
	// Multiple sets — prefer the one flagged is_default.
	for _, s := range sets {
		if s.IsDefault {
			return s.ID, nil
		}
	}
	return 0, fmt.Errorf("multiple asset sets visible; pass --set <id> to pick one (use 'ws asset-set ls' to see them)")
}

func init() {
	rootCmd.AddCommand(assetCmd)
	assetCmd.AddCommand(assetListCmd)
	assetCmd.AddCommand(assetGetCmd)
	assetCmd.AddCommand(assetCreateCmd)
	assetCmd.AddCommand(assetEditCmd)

	// --set / -s applies to every verb that addresses a set.
	for _, c := range []*cobra.Command{assetListCmd, assetCreateCmd} {
		c.Flags().IntVarP(&assetSetIDFlag, "set", "s", 0, "asset set id (default: only visible set, or is_default set)")
	}

	// List filters
	assetListCmd.Flags().StringVar(&assetListType, "type", "", "filter by asset type id")
	assetListCmd.Flags().StringVar(&assetListCategory, "category", "", "filter by category id")
	assetListCmd.Flags().StringVar(&assetListStatus, "status", "", "filter by status id")
	assetListCmd.Flags().StringVarP(&assetListSearch, "q", "q", "", "free-text title search")
	assetListCmd.Flags().IntVar(&assetListLimit, "limit", 0, "page size")
	assetListCmd.Flags().IntVar(&assetListPage, "page", 0, "1-indexed page number")

	// Create flags
	assetCreateCmd.Flags().StringVarP(&assetCreateTitle, "title", "t", "", "asset title (required)")
	assetCreateCmd.Flags().StringVarP(&assetCreateDesc, "description", "d", "", "asset description")
	assetCreateCmd.Flags().StringVar(&assetCreateTag, "tag", "", "asset tag (e.g. LAP-001)")
	assetCreateCmd.Flags().IntVar(&assetCreateTypeID, "type", 0, "asset type id (required)")
	assetCreateCmd.Flags().IntVar(&assetCreateCategoryID, "category", 0, "category id")
	assetCreateCmd.Flags().IntVar(&assetCreateStatusID, "status", 0, "status id")

	// Edit flags
	assetEditCmd.Flags().StringVarP(&assetEditTitle, "title", "t", "", "new title")
	assetEditCmd.Flags().StringVarP(&assetEditDesc, "description", "d", "", "new description")
	assetEditCmd.Flags().StringVar(&assetEditTag, "tag", "", "new asset tag")
	assetEditCmd.Flags().IntVar(&assetEditTypeID, "type", 0, "new asset type id")
	assetEditCmd.Flags().IntVar(&assetEditCategoryID, "category", 0, "new category id (0 clears)")
	assetEditCmd.Flags().IntVar(&assetEditStatusID, "status", 0, "new status id (0 clears)")
}
