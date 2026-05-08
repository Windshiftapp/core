package wscli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetFlagState zeros every flag-backing global so that a previous Run's
// --status, --workspace, etc. cannot bleed into the next one. It walks the
// cobra tree and writes each flag's DefValue back into its Value, which
// covers both PersistentFlags on rootCmd and per-subcommand flags.
//
// initConfig is registered via cobra.OnInitialize and re-runs on every
// Execute, so config flows through the precedence chain afresh each call.
func resetFlagState() {
	resetCommandFlags(rootCmd)
	cfg = Config{StatusAliases: map[string]string{}}
}

func resetCommandFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	for _, child := range cmd.Commands() {
		resetCommandFlags(child)
	}
}
