package wscli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var diagramCmd = &cobra.Command{
	Use:   "diagram",
	Short: "Manage Excalidraw / mermaid diagrams attached to work items",
	Long: `Commands for creating, viewing, and managing diagrams attached
to work items. Diagrams are stored as Excalidraw scene JSON; mermaid
sources are stored as a tiny seed wrapper and converted to a real scene
the first time the diagram is opened in the editor.`,
}

var (
	diagramName       string
	diagramMermaid    string
	diagramExcalidraw string
	diagramFromFile   string
)

var diagramCreateCmd = &cobra.Command{
	Use:   "create <KEY-123>",
	Short: "Create a new diagram on a work item",
	Long: `Create a new diagram attached to a work item. Provide the diagram
content via exactly one of:
  --mermaid "graph TD; A-->B"   inline mermaid source (stored as a seed)
  --excalidraw '{"elements":...}'   inline Excalidraw scene JSON
  --from-file scene.json        read Excalidraw scene JSON from file
  --from-file - / --from-file /dev/stdin   read from stdin

Examples:
  ws diagram create PROJ-45 --name "Auth flow" --mermaid "graph TD; A-->B"
  ws diagram create PROJ-45 --name scene --from-file scene.json
  cat scene.json | ws diagram create PROJ-45 --name scene --from-file -`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(diagramName) == "" {
			return fmt.Errorf("--name is required")
		}
		data, err := buildDiagramPayload(diagramMermaid, diagramExcalidraw, diagramFromFile)
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

		d, err := client.CreateDiagram(itemID, diagramName, data)
		if err != nil {
			return fmt.Errorf("failed to create diagram: %w", err)
		}
		NewOutput().Print(d)
		return nil
	},
}

var diagramListCmd = &cobra.Command{
	Use:   "list <KEY-123>",
	Short: "List diagrams on a work item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}
		diagrams, err := client.ListDiagrams(itemID)
		if err != nil {
			return fmt.Errorf("failed to list diagrams: %w", err)
		}
		NewOutput().Print(diagrams)
		return nil
	},
}

var diagramGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a single diagram by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid diagram ID: %s", args[0])
		}
		client, err := NewClient()
		if err != nil {
			return err
		}
		d, err := client.GetDiagram(id)
		if err != nil {
			return fmt.Errorf("failed to get diagram: %w", err)
		}
		NewOutput().Print(d)
		return nil
	},
}

var diagramUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a diagram's name and/or data",
	Long: `Update an existing diagram. Pass --name to rename. To replace the
diagram content, provide exactly one of --mermaid / --excalidraw / --from-file.

If you only pass --name, the existing diagram_data is preserved (the CLI
fetches the current value and re-sends it).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid diagram ID: %s", args[0])
		}
		client, err := NewClient()
		if err != nil {
			return err
		}

		current, err := client.GetDiagram(id)
		if err != nil {
			return fmt.Errorf("failed to fetch diagram: %w", err)
		}

		newName := current.Name
		if strings.TrimSpace(diagramName) != "" {
			newName = diagramName
		}

		newData := current.DiagramData
		if diagramMermaid != "" || diagramExcalidraw != "" || diagramFromFile != "" {
			newData, err = buildDiagramPayload(diagramMermaid, diagramExcalidraw, diagramFromFile)
			if err != nil {
				return err
			}
		}

		d, err := client.UpdateDiagram(id, newName, newData)
		if err != nil {
			return fmt.Errorf("failed to update diagram: %w", err)
		}
		NewOutput().Print(d)
		return nil
	},
}

var diagramDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a diagram by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid diagram ID: %s", args[0])
		}
		client, err := NewClient()
		if err != nil {
			return err
		}
		if err := client.DeleteDiagram(id); err != nil {
			return fmt.Errorf("failed to delete diagram: %w", err)
		}
		if outputFormat == "table" {
			_, _ = fmt.Fprintln(stdout, "Diagram deleted")
		} else {
			NewOutput().Print(map[string]interface{}{
				"deleted":    true,
				"diagram_id": id,
			})
		}
		return nil
	},
}

// buildDiagramPayload turns the mermaid / excalidraw / file inputs into the
// raw string the API stores in diagram_data. Exactly one source must be set.
func buildDiagramPayload(mermaid, excalidraw, fromFile string) (string, error) {
	count := 0
	if strings.TrimSpace(mermaid) != "" {
		count++
	}
	if strings.TrimSpace(excalidraw) != "" {
		count++
	}
	if fromFile != "" {
		count++
	}
	if count == 0 {
		return "", fmt.Errorf("provide one of --mermaid, --excalidraw, or --from-file")
	}
	if count > 1 {
		return "", fmt.Errorf("--mermaid, --excalidraw, and --from-file are mutually exclusive")
	}

	if mermaid != "" {
		wrapper, err := json.Marshal(map[string]string{
			"type":   "mermaid",
			"source": strings.TrimSpace(mermaid),
		})
		if err != nil {
			return "", fmt.Errorf("encode mermaid wrapper: %w", err)
		}
		return string(wrapper), nil
	}

	var raw string
	if excalidraw != "" {
		raw = excalidraw
	} else {
		raw = readDiagramFile(fromFile)
		if raw == "" {
			return "", fmt.Errorf("file %s is empty", fromFile)
		}
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("excalidraw payload is not valid JSON")
	}
	return raw, nil
}

func readDiagramFile(path string) string {
	var (
		data []byte
		err  error
	)
	if path == "-" || path == "/dev/stdin" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(path) //nolint:gosec // path supplied by user via CLI flag
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to read %s: %v\n", path, err)
		return ""
	}
	return string(data)
}

func init() {
	rootCmd.AddCommand(diagramCmd)
	diagramCmd.AddCommand(diagramCreateCmd)
	diagramCmd.AddCommand(diagramListCmd)
	diagramCmd.AddCommand(diagramGetCmd)
	diagramCmd.AddCommand(diagramUpdateCmd)
	diagramCmd.AddCommand(diagramDeleteCmd)

	for _, c := range []*cobra.Command{diagramCreateCmd, diagramUpdateCmd} {
		c.Flags().StringVar(&diagramName, "name", "", "diagram name")
		c.Flags().StringVar(&diagramMermaid, "mermaid", "", "mermaid source (stored as a seed wrapper)")
		c.Flags().StringVar(&diagramExcalidraw, "excalidraw", "", "Excalidraw scene JSON (inline)")
		c.Flags().StringVar(&diagramFromFile, "from-file", "", "path to a JSON file containing an Excalidraw scene (use - for stdin)")
	}
}
