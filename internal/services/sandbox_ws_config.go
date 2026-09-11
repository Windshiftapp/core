package services

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// pinSandboxWSEnvironment gives managed runs a destination that repository
// configuration cannot override. WS_TOKEN remains in the process environment.
func pinSandboxWSEnvironment(env map[string]string) error {
	trustedURL, ok, err := sandboxWSURL(env)
	if err != nil || !ok {
		return err
	}
	env["WS_URL"] = trustedURL
	if workspace := env["WS_WORKSPACE_KEY"]; workspace != "" {
		env["WS_WORKSPACE"] = workspace
	}
	return nil
}

func sandboxWSURL(env map[string]string) (trustedURL string, configured bool, err error) {
	raw := strings.TrimSpace(env["WS_URL"])
	if raw == "" {
		raw = strings.TrimSpace(env["WS_API_URL"])
		if raw == "" {
			return "", false, nil
		}
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("sandbox ws config: invalid trusted URL %q", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if env["WS_URL"] == "" && strings.HasSuffix(parsed.Path, "/api") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/api")
	}
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), true, nil
}

// writeSandboxWSConfig replaces repository-supplied root configuration after
// checkout. The environment still pins nested invocations to the same URL.
func writeSandboxWSConfig(root string, env map[string]string) error {
	trustedURL, ok, err := sandboxWSURL(env)
	if err != nil || !ok {
		return err
	}

	var content strings.Builder
	content.WriteString("# Managed by the Windshift agent sandbox.\n[server]\nurl = ")
	content.WriteString(strconv.Quote(trustedURL))
	content.WriteByte('\n')
	if workspace := env["WS_WORKSPACE_KEY"]; workspace != "" {
		content.WriteString("\n[defaults]\nworkspace_key = ")
		content.WriteString(strconv.Quote(workspace))
		content.WriteByte('\n')
	}

	tmp, err := os.CreateTemp(root, ".ws.toml-*")
	if err != nil {
		return fmt.Errorf("sandbox ws config: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sandbox ws config: chmod temporary file: %w", err)
	}
	if _, err := tmp.WriteString(content.String()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sandbox ws config: write temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("sandbox ws config: close temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(root, "ws.toml")); err != nil {
		return fmt.Errorf("sandbox ws config: replace ws.toml: %w", err)
	}
	return nil
}
