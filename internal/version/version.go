// Package version exposes build-time metadata about the running binary.
// Values are injected by the release script via -ldflags -X, with the git
// tag as the source of truth. Defaults below are used for `go run` / `go
// build` outside the release pipeline.
package version

var (
	Version     = "dev"
	Commit      = "none"
	Date        = "unknown"
	ReleaseName = ""
)
