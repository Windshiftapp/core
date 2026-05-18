package wscli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

var attachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "List and download work item attachments",
	Long: `Commands for inspecting and downloading files attached to work items.

Uploading is not supported from the CLI (use the web UI for that).`,
}

var attachmentListCmd = &cobra.Command{
	Use:   "list <id|KEY-123>",
	Short: "List attachments on a work item",
	Long: `List attachments on a work item, including each attachment's ID,
filename, size, MIME type, uploader, and creation time. The ID column is
what you pass to "ws attachment download".

Examples:
  ws attachment list PROJ-45
  ws attachment list 123`,
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
		atts, err := client.ListAttachments(itemID)
		if err != nil {
			return fmt.Errorf("failed to list attachments: %w", err)
		}
		NewOutput().Print(atts)
		return nil
	},
}

var attachmentDownloadOutput string

var attachmentDownloadCmd = &cobra.Command{
	Use:   "download <attachment-id>",
	Short: "Download an attachment by ID",
	Long: `Download an attachment by its numeric ID (find IDs via
"ws attachment list <KEY-123>").

By default, the file is written to the current directory using the
attachment's original filename. With --to:
  --to <file>   write to that exact path
  --to <dir>/   write into that directory using the server's filename
  --to -        stream raw bytes to stdout (useful with pipes)

Examples:
  ws attachment download 42                      # ./<original-filename>
  ws attachment download 42 --to /tmp/spec.pdf   # exact path
  ws attachment download 42 --to /tmp/           # /tmp/<original-filename>
  ws attachment download 42 --to - > out.bin     # stream to stdout`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid attachment ID: %s", args[0])
		}
		client, err := NewClient()
		if err != nil {
			return err
		}

		// Stdout streaming bypasses filename resolution entirely.
		if attachmentDownloadOutput == "-" {
			if _, err := client.DownloadAttachment(id, stdout); err != nil {
				return fmt.Errorf("failed to download attachment: %w", err)
			}
			return nil
		}

		// Resolve the destination path. We need the server's filename for the
		// default-CWD and directory-target cases, so download into a temp file
		// first, then rename. This also avoids leaving a half-written file at
		// the final destination if the transfer fails mid-stream.
		destDir := "."
		destName := ""
		if attachmentDownloadOutput != "" {
			info, statErr := os.Stat(attachmentDownloadOutput)
			switch {
			case statErr == nil && info.IsDir():
				destDir = attachmentDownloadOutput
			default:
				// Either the path doesn't exist (treat as file path) or it's
				// an existing file (overwrite). Split into dir + name.
				destDir = filepath.Dir(attachmentDownloadOutput)
				destName = filepath.Base(attachmentDownloadOutput)
			}
		}

		tmp, err := os.CreateTemp(destDir, ".ws-attachment-*.partial")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmp.Name()
		// Best-effort cleanup if we bail before the rename.
		cleanup := func() { _ = os.Remove(tmpPath) }

		serverName, err := client.DownloadAttachment(id, tmp)
		if cerr := tmp.Close(); err == nil && cerr != nil {
			err = cerr
		}
		if err != nil {
			cleanup()
			return fmt.Errorf("failed to download attachment: %w", err)
		}

		if destName == "" {
			destName = serverName
		}
		finalPath := filepath.Join(destDir, destName)
		if err := os.Rename(tmpPath, finalPath); err != nil {
			cleanup()
			return fmt.Errorf("failed to write file: %w", err)
		}

		if outputFormat == "table" {
			_, _ = fmt.Fprintf(stdout, "Downloaded %s\n", finalPath)
		} else {
			NewOutput().Print(map[string]interface{}{
				"attachment_id": id,
				"path":          finalPath,
			})
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(attachmentCmd)
	attachmentCmd.AddCommand(attachmentListCmd)
	attachmentCmd.AddCommand(attachmentDownloadCmd)

	attachmentDownloadCmd.Flags().StringVar(&attachmentDownloadOutput, "to", "", "output path: file, directory, or - for stdout (default: current directory using the server-supplied filename)")
}
