// Package main is the ws CLI binary. The command tree itself lives in
// internal/wscli so it can be driven in-process from tests.
package main

import (
	"os"

	"windshift/internal/wscli"
)

// Build info — set via ldflags from the VERSION file.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	wscli.SetBuildInfo(version, commit, date)
	os.Exit(wscli.Execute())
}
