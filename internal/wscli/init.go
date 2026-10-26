package wscli

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

//go:embed templates/windshift.md.tmpl
var windshiftMDTemplateSrc string

// windshiftMDTemplate is the precompiled `WINDSHIFT.md` template. Parsing at
// package init means a malformed template fails fast on binary startup rather
// than at the first `ws init` call.
var windshiftMDTemplate = template.Must(template.New("windshift.md").Parse(windshiftMDTemplateSrc))

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
	reader := bufio.NewReader(stdin)
	emitConfigWarnings()

	// Short-circuit when there's already a working global token and the
	// user didn't ask for a refresh.
	if !initManual && !initNewAgent && globalTokenConfigured() {
		agentName := loadGlobalAgentName()
		if agentName != "" {
			_, _ = fmt.Fprintf(stdout, "Already connected as %s. Use --manual to reconfigure.\n", agentName)
		} else {
			_, _ = fmt.Fprintln(stdout, "CLI is already configured globally. Use --manual to reconfigure.")
		}
		return nil
	}

	instanceURL := strings.TrimSpace(cfg.Server.URL)
	if instanceURL == "" {
		if !stdinIsTTY() {
			return fmt.Errorf("--url is required (also accepts WS_URL)")
		}
		_, _ = fmt.Fprint(stdout, "Windshift server URL (e.g., https://windshift.example.com): ")
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
	_, _ = fmt.Fprintf(stdout, "Saved global config at %s\n", getGlobalConfigPath())

	cfg.Server.URL = instanceURL
	cfg.Server.Token = token

	// Sanity check — call /me so we fail loudly if the token doesn't work.
	client, clientErr := NewClient()
	if clientErr == nil {
		if user, uerr := client.GetCurrentUser(); uerr == nil {
			_, _ = fmt.Fprintf(stdout, "Connected as: %s (%s)\n", user.FullName, user.Email)
		}
	}

	if agentUsername != "" {
		_, _ = fmt.Fprintf(stdout, "Agent for this machine: %s\n", agentUsername)
	}
	_, _ = fmt.Fprintln(stdout, "Run `ws init` inside a project directory to set up its workspace.")
	return nil
}

func runProjectInit() error {
	reader := bufio.NewReader(stdin)
	emitConfigWarnings()

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
	statusAliases := generateDefaultAliases(statuses)

	if err := writeWindshiftMD(client, wsCtx, statusAliases, "WINDSHIFT.md"); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "Created WINDSHIFT.md")

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
		StatusAliases: statusAliases,
	}
	if err := saveProjectConfig(projectConfig, "./ws.toml"); err != nil {
		return fmt.Errorf("failed to save ws.toml: %w", err)
	}
	_, _ = fmt.Fprintln(stdout, "Updated ws.toml")

	updateAgentsFiles()

	_, _ = fmt.Fprintf(stdout, "\nProject initialized for workspace %s (%s)\n", workspace.Key, workspace.Name)
	if projectAgentName != "" {
		_, _ = fmt.Fprintf(stdout, "Using project-specific agent: %s\n", projectAgentName)
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
		_, _ = fmt.Fprintf(stdout, "Could not reach %s to probe onboarding capabilities (%s).\n", instanceURL, cerr)
		_, _ = fmt.Fprintln(stdout, "Falling back to manual token entry.")
		t, perr := promptForToken(reader, instanceURL)
		return t, "", perr
	}
	if !caps.AutoOnboardingEnabled {
		if caps.ManualTokensEnabled {
			if !caps.AgentsEnabled {
				_, _ = fmt.Fprintln(stdout, "This instance has user-managed agents disabled; falling back to manual setup.")
			} else {
				_, _ = fmt.Fprintln(stdout, "Automatic setup is not available on this instance; falling back to manual.")
			}
			t, perr := promptForToken(reader, instanceURL)
			return t, "", perr
		}
		return "", "", fmt.Errorf("this instance has disabled both CLI auto-setup and API token creation; contact your administrator")
	}

	result, aerr := runCLIAuthFlow(instanceURL, agentName, hostnameForAgent(), defaultCLIScopes)
	if aerr != nil {
		_, _ = fmt.Fprintf(stdout, "Automatic setup failed: %s\n", aerr)
		if !caps.ManualTokensEnabled {
			return "", "", aerr
		}
		_, _ = fmt.Fprintln(stdout, "Falling back to manual token entry.")
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

// windshiftMDData is the input bound to templates/windshift.md.tmpl. Fields
// are flattened / pre-rendered so the template stays purely structural — no
// helper funcs needed at execute time.
type windshiftMDData struct {
	Workspace       *Workspace
	StatusAliases   map[string]string // template ranges in sorted key order
	ItemTypes       []ItemType
	Statuses        []windshiftStatusRow
	HasTransitions  bool
	InitialStatuses string // pre-joined ", "
	Transitions     []windshiftTransitionRow
	CLIVersion      string
	GeneratedAt     string
}

type windshiftStatusRow struct {
	ID           int
	Name         string
	CategoryName string
	IsDefault    string // "Yes" or ""
	IsCompleted  string // "Yes" or ""
}

type windshiftTransitionRow struct {
	From string
	To   string // pre-joined ", "
}

func generateWindshiftMD(ws *Workspace, statuses []Status, itemTypes []ItemType, transitions []Transition, statusAliases map[string]string, generatedAt time.Time) string {
	data := windshiftMDData{
		Workspace:     ws,
		StatusAliases: statusAliases,
		ItemTypes:     itemTypes,
		Statuses:      make([]windshiftStatusRow, 0, len(statuses)),
		CLIVersion:    version,
		GeneratedAt:   generatedAt.UTC().Format(time.RFC3339),
	}
	for _, s := range statuses {
		data.Statuses = append(data.Statuses, windshiftStatusRow{
			ID:           s.ID,
			Name:         s.Name,
			CategoryName: s.CategoryName,
			IsDefault:    yesIf(s.IsDefault),
			IsCompleted:  yesIf(s.IsCompleted),
		})
	}

	if len(transitions) > 0 {
		// transitionMap: from-status ID -> list of to-status names. Built in
		// Go so the template can range over a pre-sorted slice instead of
		// having to look up by ID.
		transitionMap := map[int][]string{}
		var initials []string
		for _, t := range transitions {
			if t.FromStatusID == nil {
				if t.ToStatus != nil {
					initials = append(initials, t.ToStatus.Name)
				}
				continue
			}
			if t.ToStatus != nil {
				transitionMap[*t.FromStatusID] = append(transitionMap[*t.FromStatusID], t.ToStatus.Name)
			}
		}
		data.InitialStatuses = strings.Join(initials, ", ")
		for _, s := range statuses {
			targets := transitionMap[s.ID]
			if len(targets) == 0 {
				continue
			}
			data.Transitions = append(data.Transitions, windshiftTransitionRow{
				From: s.Name,
				To:   strings.Join(targets, ", "),
			})
		}
		data.HasTransitions = len(data.Transitions) > 0 || data.InitialStatuses != ""
	}

	var sb strings.Builder
	if err := windshiftMDTemplate.Execute(&sb, data); err != nil {
		// Template is embedded and data shape is owned by this file — any
		// error here is a programmer mistake, not a runtime condition.
		panic(fmt.Sprintf("render WINDSHIFT.md: %v", err))
	}
	return sb.String()
}

func yesIf(b bool) string {
	if b {
		return "Yes"
	}
	return ""
}

// writeWindshiftMD renders WINDSHIFT.md from the workspace context and
// writes it to path. Used by both `ws init` (during project setup) and
// `ws config docs` (refresh-only). A transition lookup failure aborts the
// refresh so an unavailable workflow cannot be rendered as an empty one.
func writeWindshiftMD(client *Client, wsCtx *WorkspaceContext, statusAliases map[string]string, path string) error {
	var defaultWorkflow *Workflow
	for i := range wsCtx.Workflows {
		if wsCtx.Workflows[i].IsDefault {
			defaultWorkflow = &wsCtx.Workflows[i]
			break
		}
	}
	var transitions []Transition
	if defaultWorkflow != nil {
		var err error
		transitions, err = client.GetWorkflowTransitions(defaultWorkflow.ID)
		if err != nil {
			return fmt.Errorf("failed to get transitions for default workflow %q: %w", defaultWorkflow.Name, err)
		}
	}

	content := generateWindshiftMD(wsCtx.Workspace, wsCtx.Statuses, wsCtx.ItemTypes, transitions, statusAliases, time.Now())
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // G306: project doc, group-readable is fine
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
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

func updateAgentsFiles() {
	files := []struct {
		name   string
		update func(string) (string, bool)
	}{
		{
			name: "AGENTS.md",
			update: func(content string) (string, bool) {
				if strings.Contains(content, "WINDSHIFT.md") {
					return content, false
				}
				return content + "\n\n## Windshift Integration\n\nRead [WINDSHIFT.md](./WINDSHIFT.md) before using the `ws` CLI.\n", true
			},
		},
		{
			name: "CLAUDE.md",
			update: func(content string) (string, bool) {
				imports := make([]string, 0, 2)
				if !containsLine(content, "@AGENTS.md") {
					imports = append(imports, "@AGENTS.md")
				}
				if !containsLine(content, "@WINDSHIFT.md") {
					imports = append(imports, "@WINDSHIFT.md")
				}
				if len(imports) == 0 {
					return content, false
				}
				return content + "\n\n" + strings.Join(imports, "\n") + "\n", true
			},
		},
	}

	for _, file := range files {
		content, err := os.ReadFile(file.name)
		if err != nil {
			continue
		}
		updated, changed := file.update(string(content))
		if !changed {
			continue
		}
		if err := os.WriteFile(file.name, []byte(updated), 0o600); err != nil {
			_, _ = fmt.Fprintf(stdout, "Warning: Could not update %s: %s\n", file.name, err)
			continue
		}
		_, _ = fmt.Fprintf(stdout, "Updated %s with agent context\n", file.name)
	}
}

func containsLine(content, want string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVar(&initGlobal, "global", false, "force global-tier CLI setup (writes ~/.config/ws/config.toml)")
	initCmd.Flags().BoolVar(&initManual, "manual", false, "skip the browser flow and prompt for a pasted API token")
	initCmd.Flags().BoolVar(&initNewAgent, "new-agent", false, "provision a project-specific agent + token (project tier)")
	initCmd.Flags().StringVar(&initAgentName, "agent-name", "", "override the generated agent username")
}
