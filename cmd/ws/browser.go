package main

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// openBrowser opens the specified URL in the default browser
func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL) //nolint:gosec // G204: URL is constructed by buildItemURL, not user input
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", rawURL) //nolint:gosec // G204: URL is constructed by buildItemURL, not user input
	default:
		cmd = exec.Command("xdg-open", rawURL) //nolint:gosec // G204: URL is constructed by buildItemURL, not user input
	}
	return cmd.Start()
}

// buildItemURL constructs the web URL for an item. The configured server URL
// is the API base (which may live under any subpath, e.g. /api or /svc/api).
// We strip a trailing /api segment from the path and append the workspace +
// item segments. Falls back to naive concatenation only if the URL won't parse.
func buildItemURL(workspaceKey string, itemNumber int) string {
	suffix := fmt.Sprintf("/workspace/%s/item/%d", workspaceKey, itemNumber)

	u, err := url.Parse(cfg.Server.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		base := strings.TrimSuffix(strings.TrimSuffix(cfg.Server.URL, "/"), "/api")
		return base + suffix
	}

	path := strings.TrimSuffix(u.Path, "/")
	path = strings.TrimSuffix(path, "/api")
	u.Path = path + suffix
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
