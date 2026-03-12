package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize project for Claude Code",
	Long: `Initialize the project with Windshift CLI integration.

This command:
  1. Queries workspace for item types, statuses, and workflow configuration
  2. Generates WINDSHIFT.md documenting available commands and workspace config
  3. Updates AGENTS.md or CLAUDE.md (if they exist) to include WINDSHIFT.md

Examples:
  ws init                                 # Initialize with default workspace
  ws init -w PROJ                         # Initialize with specific workspace`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		// Resolve workspace
		wsKey := cfg.GetEffectiveWorkspace()
		if wsKey == "" {
			return fmt.Errorf("workspace is required: use -w flag or set defaults.workspace_key in config")
		}

		wsID, err := client.ResolveWorkspaceID(wsKey)
		if err != nil {
			return fmt.Errorf("failed to resolve workspace: %w", err)
		}

		// Get workspace details
		workspace, err := client.GetWorkspace(wsID)
		if err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}

		// Get statuses
		statuses, err := client.GetWorkspaceStatuses(wsID)
		if err != nil {
			return fmt.Errorf("failed to get statuses: %w", err)
		}

		// Get item types
		itemTypes, err := client.ListItemTypes()
		if err != nil {
			return fmt.Errorf("failed to get item types: %w", err)
		}

		// Get workflows and transitions
		workflows, err := client.ListWorkflows()
		if err != nil {
			return fmt.Errorf("failed to get workflows: %w", err)
		}

		// Find default workflow and get its transitions
		var defaultWorkflow *Workflow
		for i := range workflows {
			if workflows[i].IsDefault {
				defaultWorkflow = &workflows[i]
				break
			}
		}

		var transitions []Transition
		if defaultWorkflow != nil {
			transitions, err = client.GetWorkflowTransitions(defaultWorkflow.ID)
			if err != nil {
				// Non-fatal, continue without transitions
				transitions = nil
			}
		}

		// Generate WINDSHIFT.md
		content := generateWindshiftMD(workspace, statuses, itemTypes, transitions)

		if err := os.WriteFile("WINDSHIFT.md", []byte(content), 0o600); err != nil {
			return fmt.Errorf("failed to write WINDSHIFT.md: %w", err)
		}
		fmt.Println("Created WINDSHIFT.md")

		// Update ws.toml with workspace settings (preserves existing server config)
		projectConfig := Config{
			Server: ServerConfig{
				URL:   cfg.Server.URL,
				Token: cfg.Server.Token,
			},
			Defaults: DefaultsConfig{
				WorkspaceKey: workspace.Key,
			},
			StatusAliases: generateDefaultAliases(statuses),
		}
		if err := saveProjectConfig(projectConfig, "./ws.toml"); err != nil {
			return fmt.Errorf("failed to save ws.toml: %w", err)
		}
		fmt.Println("Updated ws.toml")

		// Update AGENTS.md or CLAUDE.md if they exist
		updateAgentsFile("AGENTS.md")
		updateAgentsFile("CLAUDE.md")

		fmt.Printf("\nProject initialized for workspace %s (%s)\n", workspace.Key, workspace.Name)
		return nil
	},
}

func generateWindshiftMD(ws *Workspace, statuses []Status, itemTypes []ItemType, transitions []Transition) string {
	var sb strings.Builder

	sb.WriteString("# Windshift CLI\n\n")
	fmt.Fprintf(&sb, "This project is connected to Windshift workspace **%s** (%s).\n\n", ws.Key, ws.Name)

	// Quick Commands section
	sb.WriteString("## Quick Commands\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("# My work\n")
	sb.WriteString("ws task mine              # Tasks assigned to me\n")
	sb.WriteString("ws task created           # Tasks I created\n")
	sb.WriteString("\n")
	sb.WriteString("# Create & manage\n")
	sb.WriteString("ws task create -t \"Title\" [-d \"Description\"]\n")
	sb.WriteString("ws task move <KEY-123> <status>\n")
	sb.WriteString("ws task get <KEY-123>\n")
	sb.WriteString("\n")
	sb.WriteString("# Test execution\n")
	sb.WriteString("ws test run mine          # My test runs\n")
	sb.WriteString("ws test run start <set>   # Start test run\n")
	sb.WriteString("ws test result <run> <case> passed|failed|blocked|skipped\n")
	sb.WriteString("```\n\n")

	// Status Aliases section (if any are configured)
	if len(cfg.StatusAliases) > 0 {
		sb.WriteString("## Status Aliases\n\n")
		sb.WriteString("Use these consistent commands regardless of actual workspace statuses:\n\n")
		sb.WriteString("| Alias | Maps To | Usage |\n")
		sb.WriteString("|-------|---------|-------|\n")
		for alias, status := range cfg.StatusAliases {
			fmt.Fprintf(&sb, "| `%s` | %s | `ws task move X %s` |\n", alias, status, alias)
		}
		sb.WriteString("\n")
	}

	// Item Types section
	sb.WriteString("## Available Item Types\n\n")
	for _, t := range itemTypes {
		icon := ""
		if t.Icon != "" {
			icon = t.Icon + " "
		}
		fmt.Fprintf(&sb, "- %s%s\n", icon, t.Name)
	}
	sb.WriteString("\n")

	// Statuses section
	sb.WriteString("## Available Statuses\n\n")
	sb.WriteString("| ID | Status | Category | Default | Completed |\n")
	sb.WriteString("|----|--------|----------|---------|------------|\n")
	for _, s := range statuses {
		isDefault := ""
		if s.IsDefault {
			isDefault = "Yes"
		}
		isCompleted := ""
		if s.IsCompleted {
			isCompleted = "Yes"
		}
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s |\n", s.ID, s.Name, s.CategoryName, isDefault, isCompleted)
	}
	sb.WriteString("\n")

	// Workflow Rules section (if we have transitions)
	if len(transitions) > 0 {
		sb.WriteString("## Workflow Transitions\n\n")

		// Build transition map
		transitionMap := make(map[int][]string) // from status ID -> list of to status names
		initialStatuses := []string{}

		for _, t := range transitions {
			if t.FromStatusID == nil {
				// Initial status (can be set when creating)
				if t.ToStatus != nil {
					initialStatuses = append(initialStatuses, t.ToStatus.Name)
				}
			} else {
				if t.ToStatus != nil {
					transitionMap[*t.FromStatusID] = append(transitionMap[*t.FromStatusID], t.ToStatus.Name)
				}
			}
		}

		if len(initialStatuses) > 0 {
			fmt.Fprintf(&sb, "**Initial statuses:** %s\n\n", strings.Join(initialStatuses, ", "))
		}

		sb.WriteString("| From Status | Can Move To |\n")
		sb.WriteString("|-------------|-------------|\n")
		for _, s := range statuses {
			targets := transitionMap[s.ID]
			if len(targets) > 0 {
				fmt.Fprintf(&sb, "| %s | %s |\n", s.Name, strings.Join(targets, ", "))
			}
		}
		sb.WriteString("\n")
	}

	// Test Management section
	sb.WriteString("## Test Management\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("# Test Cases\n")
	sb.WriteString("ws test case ls                    # List all test cases\n")
	sb.WriteString("ws test case get <id>              # Get case with steps\n")
	sb.WriteString("\n")
	sb.WriteString("# Test Runs\n")
	sb.WriteString("ws test run mine                   # My assigned runs\n")
	sb.WriteString("ws test run ls                     # List all runs\n")
	sb.WriteString("ws test run get <id>               # Get run with results\n")
	sb.WriteString("ws test run start <set-id>         # Start new run from set\n")
	sb.WriteString("ws test run end <id>               # End/complete a run\n")
	sb.WriteString("\n")
	sb.WriteString("# Recording Results\n")
	sb.WriteString("ws test result <run-id> <case-id> passed\n")
	sb.WriteString("ws test result <run-id> <case-id> failed --notes \"Issue description\"\n")
	sb.WriteString("```\n\n")

	// Configuration section
	sb.WriteString("## Configuration\n\n")
	sb.WriteString("Project config is stored in `ws.toml`. Global config is at `~/.config/ws/config.toml`.\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("ws config show                     # Show effective config\n")
	sb.WriteString("ws config init                     # Initialize config\n")
	sb.WriteString("```\n")

	return sb.String()
}

func generateDefaultAliases(statuses []Status) map[string]string {
	aliases := make(map[string]string)

	// Try to find common status mappings
	for _, s := range statuses {
		nameLower := strings.ToLower(s.Name)

		// Map "done" alias
		if strings.Contains(nameLower, "done") || strings.Contains(nameLower, "complete") {
			if _, exists := aliases["done"]; !exists {
				aliases["done"] = s.Name
			}
		}

		// Map "progress" alias
		if strings.Contains(nameLower, "progress") || strings.Contains(nameLower, "working") {
			if _, exists := aliases["progress"]; !exists {
				aliases["progress"] = s.Name
			}
		}

		// Map "blocked" alias
		if strings.Contains(nameLower, "block") || strings.Contains(nameLower, "hold") {
			if _, exists := aliases["blocked"]; !exists {
				aliases["blocked"] = s.Name
			}
		}

		// Map "review" alias
		if strings.Contains(nameLower, "review") {
			if _, exists := aliases["review"]; !exists {
				aliases["review"] = s.Name
			}
		}

		// Map "todo" alias
		if strings.Contains(nameLower, "open") || strings.Contains(nameLower, "new") || strings.Contains(nameLower, "todo") {
			if _, exists := aliases["todo"]; !exists {
				aliases["todo"] = s.Name
			}
		}
	}

	return aliases
}

func updateAgentsFile(filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		// File doesn't exist, skip
		return
	}

	// Check if already has Windshift reference
	if strings.Contains(string(content), "WINDSHIFT.md") {
		return
	}

	// Append Windshift section
	addition := "\n\n## Windshift Integration\n\nSee [WINDSHIFT.md](./WINDSHIFT.md) for task management commands.\n"

	if err := os.WriteFile(filename, append(content, []byte(addition)...), 0o600); err != nil {
		fmt.Printf("Warning: Could not update %s: %s\n", filename, err)
		return
	}

	fmt.Printf("Updated %s with Windshift reference\n", filename)
}

func init() {
	rootCmd.AddCommand(initCmd)
}
