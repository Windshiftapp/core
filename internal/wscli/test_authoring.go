package wscli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"windshift/internal/agentskills"
)

func newTestCaseWriteCommand(edit bool) *cobra.Command {
	var file, title, status, preconditions, priority string
	name := "create"
	if edit {
		name = "edit <id>"
	}
	cmd := &cobra.Command{
		Use: name, Short: "Author a test case using the v2 API",
		Long: `Create or edit a test case. Use --file for a JSON object containing title,
preconditions, priority, status, estimated_duration, folder_id, steps, label_ids,
and (for BDD) gherkin. Create also accepts format. Use --file - for stdin.
Flags override file values. Omitted edit fields are preserved; explicit steps
and label_ids arrays replace their collections (empty clears). folder_id: null
moves to the root. Archive reversibly with --status inactive. Never store secrets.

Examples:
  ws test case create --title "Reject expired credentials" --status draft
  ws test case create --file case.json
  ws test case edit 42 --file repaired.json
  ws test case edit 42 --status inactive`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			caseID := 0
			if edit {
				var err error
				caseID, err = strconv.Atoi(args[0])
				if err != nil || caseID <= 0 {
					return fmt.Errorf("test case ID must be a positive integer")
				}
			}
			body := map[string]any{}
			if file != "" {
				var raw []byte
				var err error
				if file == "-" {
					raw, err = io.ReadAll(io.LimitReader(cmd.InOrStdin(), 2*1024*1024+1))
				} else {
					//nolint:gosec // operator-selected local payload file
					raw, err = os.ReadFile(file)
				}
				if err != nil {
					return err
				}
				if len(raw) > 2*1024*1024 {
					return fmt.Errorf("case JSON must not exceed 2 MiB")
				}
				if err := json.Unmarshal(raw, &body); err != nil || body == nil {
					return fmt.Errorf("--file must contain a JSON object")
				}
			}
			for name, value := range map[string]string{"title": title, "status": status, "preconditions": preconditions, "priority": priority} {
				if cmd.Flags().Changed(name) {
					body[name] = value
				}
			}
			if len(body) == 0 {
				return fmt.Errorf("provide --file or at least one field flag")
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			workspaceID, err := resolveRequiredWorkspace(client)
			if err != nil {
				return err
			}
			result, err := client.WriteTestCase(workspaceID, caseID, body)
			if err != nil {
				return err
			}
			NewOutput().Print(result)
			return nil
		},
	}
	if edit {
		cmd.Args = cobra.ExactArgs(1)
	}
	cmd.Flags().StringVar(&file, "file", "", "JSON payload file, or - for stdin")
	cmd.Flags().StringVarP(&title, "title", "t", "", "test case title")
	cmd.Flags().StringVar(&status, "status", "", "active, draft, or inactive (archive)")
	cmd.Flags().StringVar(&preconditions, "preconditions", "", "setup and synthetic data/reference; no secrets")
	cmd.Flags().StringVar(&priority, "priority", "", "low, medium, high, or critical")
	return cmd
}

func init() {
	var page, setup bool
	cmd := &cobra.Command{Use: "standard", Short: "Print the portable two-pass QA skill pack", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := agentskills.TestCaseStandardBody
			if page {
				body = agentskills.TestCaseStandardPage
			}
			if setup {
				body = agentskills.TestCaseStandardSetup
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), body)
			return err
		}}
	cmd.Flags().BoolVar(&page, "page", false, "print the starter standard for a workspace page")
	cmd.Flags().BoolVar(&setup, "setup", false, "print binding setup and the worked example")
	cmd.MarkFlagsMutuallyExclusive("page", "setup")
	testCmd.AddCommand(cmd)
}

func (c *Client) WriteTestCase(workspaceID, caseID int, body map[string]any) (*TestCase, error) {
	path := fmt.Sprintf("/rest/api/v2/workspaces/%d/test-cases", workspaceID)
	var result TestCase
	var err error
	if caseID == 0 {
		err = c.POST(path, body, &result)
	} else {
		err = c.MERGEPATCH(fmt.Sprintf("%s/%d", path, caseID), body, &result)
	}
	return &result, err
}
