package wscli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Conformance checking (WI-1334): compare a configuration set against a
// canonical template document and optionally repair drifted configuration
// back to it. The template is the same document the import flow consumes.

var configSetCmd = &cobra.Command{
	Use:   "config-set",
	Short: "Configuration set conformance",
	Long: `Check a configuration set against its canonical template and repair drift.

The template is the portable configuration-set document produced by export
or shipped with a framework pack.`,
}

var configSetCheckCmd = &cobra.Command{
	Use:   "check <config-set-id>",
	Short: "Check a configuration set against a canonical template",
	Long: `Compare the live configuration of a configuration set with the template
document and print one drift row per concrete difference. Read-only.

Examples:
  ws config-set check 12 --template pack-template.json`,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("exactly one configuration set ID is required")
		}
		configSetID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid configuration set ID %q", args[0])
		}
		template, err := loadConformanceTemplate(configSetTemplateFile)
		if err != nil {
			return err
		}
		client, err := NewClient()
		if err != nil {
			return err
		}
		var doc struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.POST(fmt.Sprintf("/rest/api/v2/configuration-sets/%d/conformance/check", configSetID), template, &doc); err != nil {
			return fmt.Errorf("conformance check failed: %w", err)
		}
		return printConformanceJSON(doc.Data)
	},
}

var configSetRepairCmd = &cobra.Command{
	Use:   "repair <config-set-id>",
	Short: "Repair drifted configuration back to the canonical template",
	Long: `Restore a configuration set's configuration to the canonical template.
Repairs run in one transaction and never touch item data. Without --only,
every repairable drift row is repaired.

Examples:
  ws config-set repair 12 --template pack-template.json
  ws config-set repair 12 --template pack-template.json --only workflows/missing/rt-workflow`,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("exactly one configuration set ID is required")
		}
		configSetID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid configuration set ID %q", args[0])
		}
		template, err := loadConformanceTemplate(configSetTemplateFile)
		if err != nil {
			return err
		}
		body := map[string]any{"template": template}
		if strings.TrimSpace(configSetRepairOnly) != "" {
			var ids []string
			for _, id := range strings.Split(configSetRepairOnly, ",") {
				if trimmed := strings.TrimSpace(id); trimmed != "" {
					ids = append(ids, trimmed)
				}
			}
			body["ids"] = ids
		}
		client, err := NewClient()
		if err != nil {
			return err
		}
		var doc struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.POST(fmt.Sprintf("/rest/api/v2/configuration-sets/%d/conformance/repair", configSetID), body, &doc); err != nil {
			return fmt.Errorf("conformance repair failed: %w", err)
		}
		return printConformanceJSON(doc.Data)
	},
}

var (
	configSetTemplateFile string
	configSetRepairOnly   string
)

func init() {
	rootCmd.AddCommand(configSetCmd)
	configSetCmd.AddCommand(configSetCheckCmd)
	configSetCmd.AddCommand(configSetRepairCmd)

	configSetCheckCmd.Flags().StringVar(&configSetTemplateFile, "template", "", "path to the canonical template JSON document (required)")
	_ = configSetCheckCmd.MarkFlagRequired("template")
	configSetRepairCmd.Flags().StringVar(&configSetTemplateFile, "template", "", "path to the canonical template JSON document (required)")
	_ = configSetRepairCmd.MarkFlagRequired("template")
	configSetRepairCmd.Flags().StringVar(&configSetRepairOnly, "only", "", "comma-separated drift row IDs to repair (default: all repairable rows)")
}

func loadConformanceTemplate(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's own flag
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	var template map[string]any
	if err := json.Unmarshal(raw, &template); err != nil {
		return nil, fmt.Errorf("template %q is not valid JSON: %w", path, err)
	}
	return template, nil
}

func printConformanceJSON(data json.RawMessage) error {
	var pretty map[string]any
	if err := json.Unmarshal(data, &pretty); err == nil {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pretty)
	}
	_, err := fmt.Fprintln(stdout, string(data))
	return err
}
