package wscli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Build info — the binary wrapper sets these via SetBuildInfo before calling Run.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// SetBuildInfo lets the cmd/ws/main.go wrapper inject ldflags-injected build
// metadata. Tests do not need to call this.
func SetBuildInfo(v, c, d string) {
	version = v
	commit = c
	date = d
}

// Global flag-bound vars. They are reset to zero by Run before each cobra
// invocation so that tests can run sequentially without state bleeding
// between calls.
var (
	cfgFile      string
	outputFormat string
	serverURL    string
	token        string
	workspaceKey string
)

// stdout / stderr / stdin are package-level IO sinks. Run swaps them per
// invocation; everything else writes through these. Tests use this to
// capture output.
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
	stdin  io.Reader = os.Stdin
)

// debugHTTP enables one-line HTTP request/response logging to stderr. It is
// flipped on by setting WS_DEBUG_HTTP=1 in the env passed to Run.
var debugHTTP bool

var rootCmd = &cobra.Command{
	Use:   "ws",
	Short: "Windshift CLI - Task and test management from the command line",
	Long: `Windshift CLI (ws) provides efficient task and test management
for developers and Claude Code integration.

Configuration priority:
  1. CLI flags (--url, --token, --workspace)
  2. Environment variables (WS_URL, WS_TOKEN, WS_WORKSPACE)
  3. Project config (./ws.toml)
  4. Global config (~/.config/ws/config.toml)`,
	SilenceUsage: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprintf(stdout, "ws %s (commit: %s, built: %s)\n", version, commit, date)
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for ws.

To load completions:

Bash:
  $ source <(ws completion bash)
  # To persist, add to ~/.bashrc or install system-wide:
  $ ws completion bash > /etc/bash_completion.d/ws

Zsh:
  $ ws completion zsh > "${fpath[1]}/_ws"

Fish:
  $ ws completion fish > ~/.config/fish/completions/ws.fish

PowerShell:
  PS> ws completion powershell | Out-String | Invoke-Expression`,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			_ = rootCmd.GenBashCompletion(stdout)
		case "zsh":
			_ = rootCmd.GenZshCompletion(stdout)
		case "fish":
			_ = rootCmd.GenFishCompletion(stdout, true)
		case "powershell":
			_ = rootCmd.GenPowerShellCompletionWithDesc(stdout)
		}
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ./ws.toml or ~/.config/ws/config.toml)")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "json", "output format: json, table, csv")
	rootCmd.PersistentFlags().StringVar(&serverURL, "url", "", "Windshift server URL")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "API token")
	rootCmd.PersistentFlags().StringVarP(&workspaceKey, "workspace", "w", "", "workspace key")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(completionCmd)
}

// Run is the in-process entry point. The cmd/ws/main.go wrapper calls it with
// the real os.Args / os.Std*. Tests call it with captured buffers and
// per-invocation env so behavior is deterministic.
//
// envOverrides applies env vars only for the duration of the call (so tests
// don't leak into the process env). It is OK to pass nil; in that case the
// process env is used as-is.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, envOverrides map[string]string) int {
	// Snapshot+restore IO so a test reusing the package between calls gets
	// fresh wiring each time.
	prevStdin, prevStdout, prevStderr := stdin, stdout, stderr
	defer func() { stdin, stdout, stderr = prevStdin, prevStdout, prevStderr }()
	if in != nil {
		stdin = in
	}
	if out != nil {
		stdout = out
	}
	if errOut != nil {
		stderr = errOut
	}

	// Snapshot+restore env vars we'll mutate.
	if len(envOverrides) > 0 {
		restore := make(map[string]struct {
			val string
			set bool
		}, len(envOverrides))
		for k := range envOverrides {
			v, ok := os.LookupEnv(k)
			restore[k] = struct {
				val string
				set bool
			}{v, ok}
		}
		for k, v := range envOverrides {
			_ = os.Setenv(k, v)
		}
		defer func() {
			for k, prev := range restore {
				if prev.set {
					_ = os.Setenv(k, prev.val)
				} else {
					_ = os.Unsetenv(k)
				}
			}
		}()
	}

	// Reset flag-backing globals so a previous run's --status etc. doesn't
	// bleed into this one. cobra will re-populate from args during Execute.
	resetFlagState()
	debugHTTP = os.Getenv("WS_DEBUG_HTTP") == "1"

	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetIn(stdin)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// Execute is kept as a convenience for the binary wrapper. Equivalent to
// Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, nil).
func Execute() int {
	return Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, nil)
}
