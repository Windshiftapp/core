package wscli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	searchLimit int
	searchQL    bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search work items by text or CQL filter",
	Long: `Search items the caller can view via the v2 API.
Multiple arguments are joined into a single query string.

The query may be free text or a structured CQL filter. A query that parses as
a CQL filter (e.g. "milestone = '0.8.2'") is evaluated as such; otherwise it is
matched as free text. Pass --ql to force CQL evaluation and surface parse
errors instead of silently falling back to text.

The server searches across every accessible workspace; when a workspace is
configured (via -w, $WS_WORKSPACE, or defaults.workspace_key in ws.toml)
	the server scopes the query to that workspace.

Examples:
  ws search "login bug"
  ws search login bug --limit 5
  ws search "rate limit" -w PROJ
  ws search "milestone = '0.8.2' AND status != Done"
  ws search --ql "assignee = currentUser() AND status != Done"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runItemSearch(strings.Join(args, " "), searchLimit, searchQL)
	},
}

// runItemSearch is the shared body of the item search, used by both the
// top-level `ws search` and the `ws task search` alias. limit <= 0 falls back
// to the server default page size. When asCQL is true the query is forced
// through the CQL filter path.
func runItemSearch(query string, limit int, asCQL bool) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("search query must not be empty")
	}

	wsID, err := resolveOptionalWorkspace(client)
	if err != nil {
		return err
	}
	resp, err := client.SearchItems(query, limit, asCQL, wsID)
	if err != nil {
		return fmt.Errorf("failed to search items: %w", err)
	}
	NewOutput().Print(resp)
	return nil
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().IntVar(&searchLimit, "limit", 0, "maximum results per page (server default if omitted, max 100)")
	searchCmd.Flags().BoolVar(&searchQL, "ql", false, "treat the query as a CQL filter (surfaces parse errors instead of full-text fallback)")
}
