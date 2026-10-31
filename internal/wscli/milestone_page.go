package wscli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Milestone page links. Only workspace milestones have page
// links, and only to pages in the milestone's workspace. Anyone who can view
// the milestone lists the pages they may view; linking and unlinking need edit
// rights on the milestone, which the server enforces.

var milestonePageCmd = &cobra.Command{
	Use:   "page",
	Short: "Manage pages linked to milestones",
	Long:  `Commands for listing, linking, and unlinking the pages of a workspace milestone.`,
}

var milestonePageListCmd = &cobra.Command{
	Use:   "list <milestone-id>",
	Short: "List pages linked to a milestone",
	Long: `List the pages linked to a workspace milestone that you may view.

Examples:
  ws milestone page list 5
  ws milestone page list 5 -o table`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		milestoneID, err := parseMilestoneArgID("milestone", args[0])
		if err != nil {
			return err
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		links, err := client.ListMilestonePageLinks(milestoneID)
		if err != nil {
			return fmt.Errorf("failed to get milestone pages: %w", err)
		}

		NewOutput().Print(links)
		return nil
	},
}

var milestonePageAddCmd = &cobra.Command{
	Use:   "add <milestone-id> <page-id>",
	Short: "Link a page to a milestone",
	Long: `Link a page from the milestone's workspace to a workspace milestone.

Examples:
  ws milestone page add 5 66`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		milestoneID, pageID, err := parseMilestonePageTarget(args)
		if err != nil {
			return err
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		link, err := client.CreateMilestonePageLink(milestoneID, pageID)
		if err != nil {
			return fmt.Errorf("failed to link page: %w", err)
		}

		NewOutput().Print(link)
		return nil
	},
}

var milestonePageRemoveCmd = &cobra.Command{
	Use:   "remove <milestone-id> <page-id>",
	Short: "Unlink a page from a milestone",
	Long: `Unlink a page from a workspace milestone. The page is identified by its
page ID; the link ID is looked up from the milestone's page links.

Examples:
  ws milestone page remove 5 66`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		milestoneID, pageID, err := parseMilestonePageTarget(args)
		if err != nil {
			return err
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		links, err := client.ListMilestonePageLinks(milestoneID)
		if err != nil {
			return fmt.Errorf("failed to get milestone pages: %w", err)
		}
		linkID := 0
		for _, link := range links {
			if link.PageID == pageID {
				linkID = link.ID
				break
			}
		}
		if linkID == 0 {
			return fmt.Errorf("page %d is not linked to milestone %d", pageID, milestoneID)
		}

		if err := client.DeleteMilestonePageLink(milestoneID, linkID); err != nil {
			return fmt.Errorf("failed to unlink page: %w", err)
		}

		if outputFormat == "table" {
			_, _ = fmt.Fprintln(stdout, "Page unlinked")
		} else {
			NewOutput().Print(map[string]any{
				"deleted":      true,
				"milestone_id": milestoneID,
				"page_id":      pageID,
				"link_id":      linkID,
			})
		}
		return nil
	},
}

func parseMilestonePageTarget(args []string) (milestoneID, pageID int, err error) {
	if milestoneID, err = parseMilestoneArgID("milestone", args[0]); err != nil {
		return 0, 0, err
	}
	if pageID, err = parseMilestoneArgID("page", args[1]); err != nil {
		return 0, 0, err
	}
	return milestoneID, pageID, nil
}

func init() {
	milestoneCmd.AddCommand(milestonePageCmd)
	milestonePageCmd.AddCommand(milestonePageListCmd)
	milestonePageCmd.AddCommand(milestonePageAddCmd)
	milestonePageCmd.AddCommand(milestonePageRemoveCmd)
}
