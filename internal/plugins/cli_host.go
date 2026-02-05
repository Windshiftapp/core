package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	extism "github.com/extism/go-sdk"
)

const (
	// DefaultCLITimeoutMs is the default timeout for CLI execution (30 seconds)
	DefaultCLITimeoutMs = 30000
	// MaxCLITimeoutMs is the maximum allowed timeout (10 minutes)
	MaxCLITimeoutMs = 600000
	// MaxOutputBytes is the maximum bytes to capture from stdout/stderr
	MaxOutputBytes = 1024 * 1024 // 1MB
)

// cliExecHostFunction executes a CLI command and returns the result.
func (m *Manager) cliExecHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("cli_exec: failed to read payload", "error", err)
		m.writeHostResponse(plugin, stack, CLIExecResponse{
			Status: "error",
			Error:  "failed to read request payload",
		})
		return
	}

	var req CLIExecRequest
	if err = json.Unmarshal(payload, &req); err != nil {
		m.logger.Warn("cli_exec: failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, CLIExecResponse{
			Status: "error",
			Error:  "invalid request format: " + err.Error(),
		})
		return
	}

	// Validate command
	if req.Command == "" {
		m.writeHostResponse(plugin, stack, CLIExecResponse{
			Status: "error",
			Error:  "command is required",
		})
		return
	}

	// Set default timeout
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = DefaultCLITimeoutMs
	}
	if timeoutMs > MaxCLITimeoutMs {
		timeoutMs = MaxCLITimeoutMs
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(execCtx, req.Command, req.Args...) //nolint:gosec // G204: command path is from trusted plugin configuration

	// Set working directory if specified
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	// Set environment variables
	if len(req.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range req.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, limit: MaxOutputBytes}
	cmd.Stderr = &limitedWriter{w: &stderr, limit: MaxOutputBytes}

	m.logger.Debug("cli_exec: executing command",
		"command", req.Command,
		"args", req.Args,
		"working_dir", req.WorkingDir,
		"timeout_ms", timeoutMs,
	)

	// Execute command
	err = cmd.Run()

	response := CLIExecResponse{
		Status:   "ok",
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Command ran but exited with non-zero status
			response.ExitCode = exitErr.ExitCode()
		} else if execCtx.Err() == context.DeadlineExceeded {
			// Timeout
			response.Status = "error"
			response.Error = "command timed out"
			response.ExitCode = -1
		} else {
			// Other error (command not found, permission denied, etc.)
			response.Status = "error"
			response.Error = err.Error()
			response.ExitCode = -1
		}
	}

	m.logger.Debug("cli_exec: command completed",
		"command", req.Command,
		"exit_code", response.ExitCode,
		"stdout_len", len(response.Stdout),
		"stderr_len", len(response.Stderr),
	)

	m.writeHostResponse(plugin, stack, response)
}

// limitedWriter wraps a writer and limits how many bytes can be written.
type limitedWriter struct {
	w       *bytes.Buffer
	limit   int
	written int
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	remaining := lw.limit - lw.written
	if remaining <= 0 {
		return len(p), nil // Discard but don't error
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err = lw.w.Write(p)
	lw.written += n
	return len(p), err // Always report all bytes as written
}
