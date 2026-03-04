package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec // G204: URL is constructed by buildItemURL, not user input
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url) //nolint:gosec // G204: URL is constructed by buildItemURL, not user input
	default:
		cmd = exec.Command("xdg-open", url) //nolint:gosec // G204: URL is constructed by buildItemURL, not user input
	}
	return cmd.Start()
}

// buildItemURL constructs the web URL for an item
func buildItemURL(workspaceKey string, itemNumber int) string {
	baseURL := strings.TrimSuffix(cfg.Server.URL, "/api")
	baseURL = strings.TrimSuffix(baseURL, "/")
	return fmt.Sprintf("%s/workspace/%s/item/%d", baseURL, workspaceKey, itemNumber)
}
