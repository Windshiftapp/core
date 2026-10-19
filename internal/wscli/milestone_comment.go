package wscli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Milestone comments. Anyone who can view a milestone can list its
// comments; the server enforces who may add, edit, or delete them.

var milestoneCommentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Manage comments on milestones",
	Long:  `Commands for viewing, adding, and managing Markdown comments on a milestone.`,
}

var milestoneCommentListCmd = &cobra.Command{
	Use:   "list <milestone-id>",
	Short: "List comments on a milestone",
	Long: `List all comments on a milestone, newest first.

Examples:
  ws milestone comment list 5
  ws milestone comment list 5 -o table`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		milestoneID, err := parseMilestoneCommentID("milestone", args[0])
		if err != nil {
			return err
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		comments, err := client.ListMilestoneComments(milestoneID)
		if err != nil {
			return fmt.Errorf("failed to get milestone comments: %w", err)
		}

		NewOutput().Print(comments)
		return nil
	},
}

var milestoneCommentAddCmd = &cobra.Command{
	Use:   "add <milestone-id>",
	Short: "Add a comment to a milestone",
	Long: `Add a Markdown comment to a milestone. Pass the body with -m, or read it
verbatim from a file with --file (use - for stdin) for long Markdown.

Examples:
  ws milestone comment add 5 -m "Scope agreed; see the spec page"
  ws milestone comment add 5 --file update.md
  cat update.md | ws milestone comment add 5 --file -`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		milestoneID, err := parseMilestoneCommentID("milestone", args[0])
		if err != nil {
			return err
		}
		body, err := milestoneCommentBody(cmd)
		if err != nil {
			return err
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		comment, err := client.CreateMilestoneComment(milestoneID, body)
		if err != nil {
			return fmt.Errorf("failed to create milestone comment: %w", err)
		}

		NewOutput().Print(comment)
		return nil
	},
}

var milestoneCommentEditCmd = &cobra.Command{
	Use:   "edit <milestone-id> <comment-id>",
	Short: "Edit a milestone comment",
	Long: `Replace the body of a milestone comment you wrote. Pass the new body with
-m, or read it verbatim from a file with --file (use - for stdin).

Examples:
  ws milestone comment edit 5 12 -m "Updated comment"
  ws milestone comment edit 5 12 --file update.md`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		milestoneID, commentID, err := parseMilestoneCommentTarget(args)
		if err != nil {
			return err
		}
		body, err := milestoneCommentBody(cmd)
		if err != nil {
			return err
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		comment, err := client.UpdateMilestoneComment(milestoneID, commentID, body)
		if err != nil {
			return fmt.Errorf("failed to update milestone comment: %w", err)
		}

		NewOutput().Print(comment)
		return nil
	},
}

var milestoneCommentDeleteCmd = &cobra.Command{
	Use:   "delete <milestone-id> <comment-id>",
	Short: "Delete a milestone comment",
	Long: `Delete a milestone comment you wrote.

Examples:
  ws milestone comment delete 5 12`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		milestoneID, commentID, err := parseMilestoneCommentTarget(args)
		if err != nil {
			return err
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		if err := client.DeleteMilestoneComment(milestoneID, commentID); err != nil {
			return fmt.Errorf("failed to delete milestone comment: %w", err)
		}

		if outputFormat == "table" {
			_, _ = fmt.Fprintln(stdout, "Comment deleted")
		} else {
			NewOutput().Print(map[string]any{
				"deleted":      true,
				"milestone_id": milestoneID,
				"comment_id":   commentID,
			})
		}
		return nil
	},
}

func parseMilestoneCommentID(kind, raw string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid %s ID: %s", kind, raw)
	}
	return id, nil
}

func parseMilestoneCommentTarget(args []string) (milestoneID, commentID int, err error) {
	if milestoneID, err = parseMilestoneCommentID("milestone", args[0]); err != nil {
		return 0, 0, err
	}
	if commentID, err = parseMilestoneCommentID("comment", args[1]); err != nil {
		return 0, 0, err
	}
	return milestoneID, commentID, nil
}

// milestoneCommentBody reads the comment body from --file (verbatim) or -m
// (with \n / \t / \\ escapes, like `ws comment add`).
func milestoneCommentBody(cmd *cobra.Command) (string, error) {
	var body string
	if cmd.Flags().Changed("file") {
		content, _, err := readMarkdownFile(milestoneCommentFile)
		if err != nil {
			return "", err
		}
		body = content
	} else {
		body = ParseCLIEscapes(milestoneCommentMessage)
	}
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("comment body is required: use -m/--message or -f/--file")
	}
	return body, nil
}

// Flags for milestone comment commands
var (
	milestoneCommentMessage string
	milestoneCommentFile    string
)

func init() {
	milestoneCmd.AddCommand(milestoneCommentCmd)
	milestoneCommentCmd.AddCommand(milestoneCommentListCmd)
	milestoneCommentCmd.AddCommand(milestoneCommentAddCmd)
	milestoneCommentCmd.AddCommand(milestoneCommentEditCmd)
	milestoneCommentCmd.AddCommand(milestoneCommentDeleteCmd)

	for _, c := range []*cobra.Command{milestoneCommentAddCmd, milestoneCommentEditCmd} {
		c.Flags().StringVarP(&milestoneCommentMessage, "message", "m", "", "comment content (Markdown; supports \\n / \\t / \\\\)")
		c.Flags().StringVarP(&milestoneCommentFile, "file", "f", "", "path to a Markdown file with the comment body (use - for stdin)")
		c.MarkFlagsMutuallyExclusive("message", "file")
	}
}
