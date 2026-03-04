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
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// buildItemURL constructs the web URL for an item
func buildItemURL(workspaceKey string, itemNumber int) string {
	baseURL := strings.TrimSuffix(cfg.Server.URL, "/api")
	baseURL = strings.TrimSuffix(baseURL, "/")
	return fmt.Sprintf("%s/workspace/%s/item/%d", baseURL, workspaceKey, itemNumber)
}
