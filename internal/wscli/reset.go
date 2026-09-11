package wscli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetFlagState prevents command flags and config from leaking across Run calls.
func resetFlagState() {
	resetCommandFlags(rootCmd)
	// pflag's string-array value keeps its own changed bit after Flag.Changed
	// is reset, so clear the backing slice explicitly between invocations.
	apiHeaders = nil
	cfg = Config{StatusAliases: map[string]string{}}
}

func resetCommandFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(resetFlag)
	cmd.PersistentFlags().VisitAll(resetFlag)
	for _, child := range cmd.Commands() {
		resetCommandFlags(child)
	}
}

// resetFlag restores only mutated flags: reapplying a slice's "[]" default creates a bogus value.
func resetFlag(f *pflag.Flag) {
	if !f.Changed {
		return
	}
	_ = f.Value.Set(f.DefValue)
	f.Changed = false
}
