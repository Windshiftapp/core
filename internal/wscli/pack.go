package wscli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

// Framework pack commands (WI-1336): apply installs a pack archive into a
// workspace; verify validates the pack without applying. Both go over the
// same /packs API the UI uses.

var packCmd = &cobra.Command{
	Use:   "pack",
	Short: "Framework pack apply and verify",
	Long: `Apply or verify framework packs: versioned archives binding a configuration
set (schema), a workspace content bundle, and plugin references.`,
}

var packApplyCmd = &cobra.Command{
	Use:   "apply <pack.tar.gz>",
	Short: "Apply a framework pack to a workspace",
	Long: `Install a pack archive into a workspace. Target an existing workspace with
--workspace-id, or create/reuse one by name with --workspace-name. Apply is
idempotent: re-running converges without duplicating entities.

Examples:
  ws pack apply iso-27001-1.0.0.tar.gz --workspace-name "ISO 27001"
  ws pack apply iso-27001-1.0.0.tar.gz --workspace-id 12`,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("exactly one pack archive path is required")
		}
		data, err := os.ReadFile(args[0]) //nolint:gosec // the path comes from the operator's own flag
		if err != nil {
			return fmt.Errorf("read pack archive: %w", err)
		}
		fields := map[string]string{}
		if packWorkspaceID > 0 {
			fields["workspace_id"] = fmt.Sprint(packWorkspaceID)
		}
		if packWorkspaceName != "" {
			fields["workspace_name"] = packWorkspaceName
		}
		client, err := NewClient()
		if err != nil {
			return err
		}
		var doc struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Upload(http.MethodPost, "/rest/api/v2/packs/apply", "file", args[0], data, fields, &doc); err != nil {
			return fmt.Errorf("pack apply failed: %w", err)
		}
		return printConformanceJSON(doc.Data)
	},
}

var packVerifyCmd = &cobra.Command{
	Use:   "verify <pack.tar.gz>",
	Short: "Validate a framework pack without applying it",
	Long: `Check the pack archive's manifest, referenced files, and plugin
requirements against the connected instance, and print the apply report
that a real apply would start from.

Examples:
  ws pack verify iso-27001-1.0.0.tar.gz`,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("exactly one pack archive path is required")
		}
		data, err := os.ReadFile(args[0]) //nolint:gosec // the path comes from the operator's own flag
		if err != nil {
			return fmt.Errorf("read pack archive: %w", err)
		}
		fields := map[string]string{}
		if packWorkspaceID > 0 {
			fields["workspace_id"] = fmt.Sprint(packWorkspaceID)
		}
		if packWorkspaceName != "" {
			fields["workspace_name"] = packWorkspaceName
		}
		client, err := NewClient()
		if err != nil {
			return err
		}
		var doc struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Upload(http.MethodPost, "/rest/api/v2/packs/verify", "file", args[0], data, fields, &doc); err != nil {
			return fmt.Errorf("pack verify failed: %w", err)
		}
		return printConformanceJSON(doc.Data)
	},
}

var (
	packWorkspaceID   int
	packWorkspaceName string
)

func init() {
	rootCmd.AddCommand(packCmd)
	packCmd.AddCommand(packApplyCmd)
	packCmd.AddCommand(packVerifyCmd)

	packApplyCmd.Flags().IntVar(&packWorkspaceID, "workspace-id", 0, "apply into this existing workspace (by ID)")
	packApplyCmd.Flags().StringVar(&packWorkspaceName, "workspace-name", "", "apply into a workspace with this name, creating it when missing")
	packApplyCmd.MarkFlagsOneRequired("workspace-id", "workspace-name")
	packVerifyCmd.Flags().IntVar(&packWorkspaceID, "workspace-id", 0, "resolve plugins and target against this workspace")
	packVerifyCmd.Flags().StringVar(&packWorkspaceName, "workspace-name", "", "resolve the target workspace by name")
	packVerifyCmd.MarkFlagsOneRequired("workspace-id", "workspace-name")
}
