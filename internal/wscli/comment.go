package wscli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var commentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Manage comments on tasks",
	Long:  `Commands for viewing, creating, and managing comments on work items.`,
}

var commentAddCmd = &cobra.Command{
	Use:   "add <KEY-123>",
	Short: "Add a comment to a task",
	Long: `Add a comment to a task. Supports markdown content.

Examples:
  ws comment add PROJ-45 -m "This is a simple comment"
  ws comment add PROJ-45 -m "## Status Update\n\n- Fixed bug\n- Added tests"
  ws comment add PROJ-45 --file body.md
  ws comment add PROJ-45 --file - < body.md

--file posts verbatim bytes. Prefer it (or a single-quoted -m) when the body
contains backticks: inside double quotes the shell runs backticks as command
substitution and strips the code from the comment before ws sees it.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := resolveCommentBody(commentMessage, commentFile)
		if err != nil {
			return err
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}

		comment, err := client.CreateComment(itemID, body)
		if err != nil {
			return fmt.Errorf("failed to create comment: %w", err)
		}

		output := NewOutput()
		output.Print(comment)
		return nil
	},
}

var commentListCmd = &cobra.Command{
	Use:   "list <KEY-123>",
	Short: "List comments on a task",
	Long: `List all comments on a task.

Examples:
  ws comment list PROJ-45
  ws comment list PROJ-45 -o table`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}

		comments, err := client.GetComments(itemID)
		if err != nil {
			return fmt.Errorf("failed to get comments: %w", err)
		}

		output := NewOutput()
		output.Print(comments)
		return nil
	},
}

var commentEditCmd = &cobra.Command{
	Use:   "edit <comment_id>",
	Short: "Edit a comment",
	Long: `Edit an existing comment. Supports markdown content.

Examples:
  ws comment edit 123 -m "Updated comment content"
  ws comment edit 123 --file body.md`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := resolveCommentBody(commentMessage, commentFile)
		if err != nil {
			return err
		}

		commentID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid comment ID: %s", args[0])
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		comment, err := client.UpdateComment(commentID, body)
		if err != nil {
			return fmt.Errorf("failed to update comment: %w", err)
		}

		output := NewOutput()
		output.Print(comment)
		return nil
	},
}

var commentDeleteCmd = &cobra.Command{
	Use:   "delete <comment_id>",
	Short: "Delete a comment",
	Long: `Delete an existing comment.

Examples:
  ws comment delete 123`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		commentID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid comment ID: %s", args[0])
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		if err := client.DeleteComment(commentID); err != nil {
			return fmt.Errorf("failed to delete comment: %w", err)
		}

		if outputFormat == "table" {
			_, _ = fmt.Fprintln(stdout, "Comment deleted")
		} else {
			output := NewOutput()
			output.Print(map[string]any{
				"deleted":    true,
				"comment_id": commentID,
			})
		}
		return nil
	},
}

// Flags for comment commands
var (
	commentMessage string
	commentFile    string
)

// resolveCommentBody picks the comment body: --file wins over -m and is used
// verbatim ("-" = stdin), matching the page commands. -m goes through
// ParseCLIEscapes; file bytes never do, so shell quoting cannot corrupt the
// posted content.
func resolveCommentBody(message, file string) (string, error) {
	if file != "" {
		body, _, err := readMarkdownFile(file)
		if err != nil {
			return "", err
		}
		return body, nil
	}
	if message == "" {
		return "", fmt.Errorf("message is required: use -m or --message, or pass --file (- for stdin)")
	}
	return ParseCLIEscapes(message), nil
}

func init() {
	rootCmd.AddCommand(commentCmd)
	commentCmd.AddCommand(commentAddCmd)
	commentCmd.AddCommand(commentListCmd)
	commentCmd.AddCommand(commentEditCmd)
	commentCmd.AddCommand(commentDeleteCmd)

	// Message and file flags for add and edit
	commentAddCmd.Flags().StringVarP(&commentFile, "file", "f", "", "path to a Markdown file (use - for stdin; verbatim bytes, wins over -m)")
	commentAddCmd.Flags().StringVarP(&commentMessage, "message", "m", "", "comment content (Markdown; supports \\n / \\t / \\\\)")
	commentEditCmd.Flags().StringVarP(&commentFile, "file", "f", "", "path to a Markdown file (use - for stdin; verbatim bytes, wins over -m)")
	commentEditCmd.Flags().StringVarP(&commentMessage, "message", "m", "", "new comment content (Markdown; supports \\n / \\t / \\\\)")
}
