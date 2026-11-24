package wscli

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

var attachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "Upload, list, and download work item attachments",
	Long:  `Commands for attaching files to work items and inspecting or downloading them.`,
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

		// Download to a temporary file before resolving and renaming the destination.
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
			NewOutput().Print(map[string]any{
				"attachment_id": id,
				"path":          finalPath,
			})
		}
		return nil
	},
}

// attachmentUploadCmd uploads one or more local files to a work item. Size
// limits are enforced server-side: the route bounds the body by the effective
// attachment setting, so a client-side ceiling would reject files the server
// accepts. Only cheap local checks remain (see checkUploadableFiles).
var attachmentUploadCmd = &cobra.Command{
	Use:   "upload <id|KEY-123> <file> [file...]",
	Short: "Upload one or more files to a work item",
	Long: `Upload local files as attachments on a work item.

Every file is checked (exists, readable, not a directory, non-empty) before
the first upload is attempted, so a typo in the last path does not leave you
with a partial batch. Files upload in the order given; if one fails mid-batch
the command reports which files did land and exits non-zero.

Your server enforces the size limit (an administrator-configured maximum with
an operator ceiling) plus restrictions this command cannot check up front:
allowed MIME types, a fixed blocklist of executable extensions (.exe, .sh,
.js, .svg, ...), a requirement that files have an extension matching their
actual content, and a global switch that can turn attachments off entirely.
Those are reported with the server's own explanation.

Examples:
  ws attachment upload PROJ-45 mockup.png
  ws attachment upload 123 before.png after.png notes.pdf
  ws attachment upload PROJ-45 ./shots/*.png       # shell expands the glob
  ws attachment upload PROJ-45 report.pdf -o json`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := args[1:]
		if err := checkUploadableFiles(paths); err != nil {
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

		uploaded := make([]Attachment, 0, len(paths))
		for _, path := range paths {
			att, err := uploadOneAttachment(client, itemID, path)
			if err != nil {
				// Report completed uploads so the remainder can be retried.
				if len(uploaded) > 0 {
					NewOutput().Print(uploaded)
				}
				return err
			}
			uploaded = append(uploaded, *att)
		}

		NewOutput().Print(uploaded)
		return nil
	},
}

// uploadOneAttachment streams a single local file to the item, translating
// the server's errors into CLI-appropriate ones.
func uploadOneAttachment(client *Client, itemID int, path string) (*Attachment, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path supplied by the user on the command line, intentional
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	att, err := client.UploadItemAttachment(itemID, filepath.Base(path), f)
	if err != nil {
		return nil, translateItemAttachmentError(err, path)
	}
	return att, nil
}

// checkUploadableFiles validates every path up front so the command fails
// before touching the network when any of them is missing, a directory, or
// empty. File size is left to the server: the effective attachment limit is
// server state, and a stale local copy would reject files the server accepts.
func checkUploadableFiles(paths []string) error {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot upload %s: %w", path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("cannot upload %s: is a directory", path)
		}
		if info.Size() == 0 {
			return fmt.Errorf("cannot upload %s: file is empty", path)
		}
		// Verify readability before the batch starts.
		f, err := os.Open(path) //nolint:gosec // G304: path supplied by the user on the command line, intentional
		if err != nil {
			return fmt.Errorf("cannot upload %s: %w", path, err)
		}
		_ = f.Close()
	}
	return nil
}

// translateItemAttachmentError adds the file being uploaded as context and
// expands the upload route's 404 into the two things it can mean. The
// handler deliberately returns the same 404 for "no such item" and "no
// item.edit here", so the message must not imply the item exists.
func translateItemAttachmentError(err error, path string) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return fmt.Errorf("upload %s: item not found, or you lack item.edit in its workspace (Editor role required)", path)
	}
	return fmt.Errorf("upload %s: %w", path, err)
}

func init() {
	rootCmd.AddCommand(attachmentCmd)
	attachmentCmd.AddCommand(attachmentListCmd)
	attachmentCmd.AddCommand(attachmentDownloadCmd)
	attachmentCmd.AddCommand(attachmentUploadCmd)

	attachmentDownloadCmd.Flags().StringVar(&attachmentDownloadOutput, "to", "", "output path: file, directory, or - for stdout (default: current directory using the server-supplied filename)")
}
