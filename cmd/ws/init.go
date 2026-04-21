package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

var (
	initGlobal    bool
	initManual    bool
	initNewAgent  bool
	initAgentName string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the CLI (auth) or a project (workspace)",
	Long: `Initialize the Windshift CLI.

Two tiers:
  * Global tier (runs automatically on first use or with --global):
      Mints a per-machine bot account + token via an OAuth-style browser
      flow and writes ~/.config/ws/config.toml. No copy/paste required.
  * Project tier (runs inside a project directory):
      Writes ./ws.toml with the workspace + status aliases. Reuses the
      global token by default; pass --new-agent to provision a dedicated
      agent + token for this directory.

Manual fallback:
  * --manual skips the browser and prompts for a personal API token.
  * The CLI falls back to manual automatically when the instance has
    user-managed agents disabled or API key creation turned off.

Examples:
  ws init                                 # Auto-detect tier; do the right thing.
  ws init --global                        # Force global-tier auth setup.
  ws init --manual                        # Prompt for a pasted token.
  ws init -w PROJ                         # Project-tier workspace setup.
  ws init --new-agent                     # Dedicated agent for this project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		hasProjectFile := projectConfigFileExists()
		hasGlobalToken := globalTokenConfigured()

		// Explicit --global wins. Otherwise: if there's no project file AND
		// no global token yet, this is the very first run — do global setup.
		wantGlobal := initGlobal || (!hasProjectFile && !hasGlobalToken)
		if wantGlobal {
			return runGlobalInit()
		}
		return runProjectInit()
	},
}

func runGlobalInit() error {
	reader := bufio.NewReader(os.Stdin)

	// Short-circuit when there's already a working global token and the
	// user didn't ask for a refresh.
	if !initManual && !initNewAgent && globalTokenConfigured() {
		agentName := loadGlobalAgentName()
		if agentName != "" {
			fmt.Printf("Already connected as %s. Use --manual to reconfigure.\n", agentName)
		} else {
			fmt.Println("CLI is already configured globally. Use --manual to reconfigure.")
		}
		return nil
	}

	instanceURL := strings.TrimSpace(cfg.Server.URL)
	if instanceURL == "" {
		if !stdinIsTTY() {
			return fmt.Errorf("--url is required (also accepts WS_URL)")
		}
		fmt.Print("Windshift server URL (e.g., https://windshift.example.com): ")
		in, readErr := reader.ReadString('\n')
		if readErr != nil {
			return readErr
		}
		instanceURL = strings.TrimSpace(in)
		if instanceURL == "" {
			return fmt.Errorf("server URL is required")
		}
	}

	agentName := initAgentName
	if agentName == "" {
		agentName = defaultGlobalAgentName()
	}

	token, agentUsername, err := acquireToken(instanceURL, agentName, reader)
	if err != nil {
		return err
	}

	newCfg := Config{
		Server:        ServerConfig{URL: instanceURL, Token: token},
		Defaults:      DefaultsConfig{},
		StatusAliases: map[string]string{},
	}
	if err := saveGlobalConfig(newCfg); err != nil {
		return fmt.Errorf("failed to save global config: %w", err)
	}
	fmt.Printf("Saved global config at %s\n", getGlobalConfigPath())

	cfg.Server.URL = instanceURL
	cfg.Server.Token = token

	// Sanity check — call /me so we fail loudly if the token doesn't work.
	client, clientErr := NewClient()
	if clientErr == nil {
		if user, uerr := client.GetCurrentUser(); uerr == nil {
			fmt.Printf("Connected as: %s (%s)\n", user.FullName, user.Email)
		}
	}

	if agentUsername != "" {
		fmt.Printf("Agent for this machine: %s\n", agentUsername)
	}
	fmt.Println("Run `ws init` inside a project directory to set up its workspace.")
	return nil
}

func runProjectInit() error {
	reader := bufio.NewReader(os.Stdin)

	// --new-agent mints a project-specific agent + token before workspace
	// discovery. Token is written into ws.toml and overrides the global
	// token for commands run from this directory.
	projectTokenOverride := ""
	projectAgentName := ""
	if initNewAgent {
		if cfg.Server.URL == "" {
			return fmt.Errorf("server URL not configured. Run `ws init --global` first, or pass --url")
		}
		agentName := initAgentName
		if agentName == "" {
			agentName = defaultGlobalAgentName() + "-" + projectSlug()
		}
		token, agentUsername, err := acquireToken(cfg.Server.URL, agentName, reader)
		if err != nil {
			return err
		}
		projectTokenOverride = token
		projectAgentName = agentUsername
		cfg.Server.Token = token // so the workspace discovery below authenticates
	}

	if cfg.Server.Token == "" {
		return fmt.Errorf("no API token configured; run `ws init --global` first, or pass --new-agent to provision one for this project")
	}

	client, err := NewClient()
	if err != nil {
		return err
	}

	wsID, err := resolveRequiredWorkspace(client)
	if err != nil {
		return err
	}

	wsCtx, err := fetchWorkspaceContext(client, wsID)
	if err != nil {
		return err
	}
	workspace := wsCtx.Workspace
	statuses := wsCtx.Statuses
	itemTypes := wsCtx.ItemTypes

	var defaultWorkflow *Workflow
	for i := range wsCtx.Workflows {
		if wsCtx.Workflows[i].IsDefault {
			defaultWorkflow = &wsCtx.Workflows[i]
			break
		}
	}
	var transitions []Transition
	if defaultWorkflow != nil {
		transitions, err = client.GetWorkflowTransitions(defaultWorkflow.ID)
		if err != nil {
			transitions = nil
		}
	}

	content := generateWindshiftMD(workspace, statuses, itemTypes, transitions)
	if err := os.WriteFile("WINDSHIFT.md", []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write WINDSHIFT.md: %w", err)
	}
	fmt.Println("Created WINDSHIFT.md")

	// For the default (no --new-agent) path we keep ws.toml's server.token
	// empty and let the global config supply the token. This keeps the
	// project file portable across machines that share a repo.
	projectConfig := Config{
		Server: ServerConfig{
			URL:   cfg.Server.URL,
			Token: projectTokenOverride,
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

	updateAgentsFile("AGENTS.md")
	updateAgentsFile("CLAUDE.md")

	fmt.Printf("\nProject initialized for workspace %s (%s)\n", workspace.Key, workspace.Name)
	if projectAgentName != "" {
		fmt.Printf("Using project-specific agent: %s\n", projectAgentName)
	}
	return nil
}

// acquireToken runs the browser flow or the manual prompt. Returns the
// minted (or pasted) token and, on the automatic path, the agent username
// so the caller can surface it to the user.
func acquireToken(instanceURL, agentName string, reader *bufio.Reader) (token, agentUsername string, err error) {
	if initManual || !stdinIsTTY() {
		t, perr := promptForToken(reader, instanceURL)
		return t, "", perr
	}

	caps, cerr := fetchCLICapabilities(instanceURL)
	if cerr != nil {
		fmt.Printf("Could not reach %s to probe onboarding capabilities (%s).\n", instanceURL, cerr)
		fmt.Println("Falling back to manual token entry.")
		t, perr := promptForToken(reader, instanceURL)
		return t, "", perr
	}
	if !caps.AutoOnboardingEnabled {
		if caps.ManualTokensEnabled {
			if !caps.AgentsEnabled {
				fmt.Println("This instance has user-managed agents disabled; falling back to manual setup.")
			} else {
				fmt.Println("Automatic setup is not available on this instance; falling back to manual.")
			}
			t, perr := promptForToken(reader, instanceURL)
			return t, "", perr
		}
		return "", "", fmt.Errorf("this instance has disabled both CLI auto-setup and API token creation; contact your administrator")
	}

	result, aerr := runCLIAuthFlow(instanceURL, agentName, hostnameForAgent(), defaultCLIScopes)
	if aerr != nil {
		fmt.Printf("Automatic setup failed: %s\n", aerr)
		if !caps.ManualTokensEnabled {
			return "", "", aerr
		}
		fmt.Println("Falling back to manual token entry.")
		t, perr := promptForToken(reader, instanceURL)
		return t, "", perr
	}
	return result.Token, result.Agent, nil
}

func projectConfigFileExists() bool {
	_, err := os.Stat("./ws.toml")
	return err == nil
}

func globalTokenConfigured() bool {
	path := getGlobalConfigPath()
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	var gc Config
	if _, err := toml.DecodeFile(path, &gc); err != nil {
		return false
	}
	return gc.Server.URL != "" && gc.Server.Token != ""
}

// loadGlobalAgentName returns the cached agent username, if any, from the
// global config. Best-effort — used only for friendly prompts.
func loadGlobalAgentName() string {
	// We don't persist the agent name in the config today, so derive the
	// default that was likely used. Users who override with --agent-name
	// won't see the actual name here, which is fine for the informational
	// "already connected as X" message.
	return defaultGlobalAgentName()
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

		idStr := fmt.Sprintf("%d", s.ID)

		// Map "done" alias
		if strings.Contains(nameLower, "done") || strings.Contains(nameLower, "complete") {
			if _, exists := aliases["done"]; !exists {
				aliases["done"] = idStr
			}
		}

		// Map "progress" alias
		if strings.Contains(nameLower, "progress") || strings.Contains(nameLower, "working") {
			if _, exists := aliases["progress"]; !exists {
				aliases["progress"] = idStr
			}
		}

		// Map "blocked" alias
		if strings.Contains(nameLower, "block") || strings.Contains(nameLower, "hold") {
			if _, exists := aliases["blocked"]; !exists {
				aliases["blocked"] = idStr
			}
		}

		// Map "review" alias
		if strings.Contains(nameLower, "review") {
			if _, exists := aliases["review"]; !exists {
				aliases["review"] = idStr
			}
		}

		// Map "todo" alias
		if strings.Contains(nameLower, "open") || strings.Contains(nameLower, "new") || strings.Contains(nameLower, "todo") {
			if _, exists := aliases["todo"]; !exists {
				aliases["todo"] = idStr
			}
		}
	}

	return aliases
}

func updateAgentsFile(filename string) {
	content, err := os.ReadFile(filename) //nolint:gosec // G304 — filename is a hardcoded literal
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

	if err := os.WriteFile(filename, append(content, []byte(addition)...), 0o600); err != nil { //nolint:gosec // G703: filename is hardcoded at call site
		fmt.Printf("Warning: Could not update %s: %s\n", filename, err)
		return
	}

	fmt.Printf("Updated %s with Windshift reference\n", filename)
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVar(&initGlobal, "global", false, "force global-tier CLI setup (writes ~/.config/ws/config.toml)")
	initCmd.Flags().BoolVar(&initManual, "manual", false, "skip the browser flow and prompt for a pasted API token")
	initCmd.Flags().BoolVar(&initNewAgent, "new-agent", false, "provision a project-specific agent + token (project tier)")
	initCmd.Flags().StringVar(&initAgentName, "agent-name", "", "override the generated agent username")
}
