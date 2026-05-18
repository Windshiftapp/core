package services

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os/exec"
	"strings"
	"sync"
	"time"

	"windshift/internal/models"
)

// ContainerService manages ephemeral Docker containers for action execution.
type ContainerService struct {
	mu         sync.Mutex
	containers map[string]*managedContainer // containerID -> managed info
}

type managedContainer struct {
	containerID string
	cancelFn    context.CancelFunc
	startedAt   time.Time
}

// buildDockerRunArgs assembles the argv passed to `docker` for an ephemeral
// container start. Pulled out as a pure function so the policy bits — memory,
// cpu, network isolation — can be unit-tested without invoking docker.
//
// Network mode defaults to "none" and is always passed explicitly. The earlier
// "skip --network when mode is none" shortcut caused Docker to fall back to
// its default bridge, defeating isolation.
func buildDockerRunArgs(envConfig models.DockerEnvironmentConfig, hostPort int) []string {
	// Fixed args: run, -d, --memory <v>, --cpus <v>, -p <v>, --network <v> (10)
	// + 2 per env var (-e <k=v>) + 1 for the image at the tail.
	args := make([]string, 0, 10+2*len(envConfig.EnvVars)+1)
	args = append(args,
		"run", "-d",
		"--memory", envConfig.ResourceLimits.Memory,
		"--cpus", envConfig.ResourceLimits.CPUs,
		"-p", fmt.Sprintf("%d:8080", hostPort),
	)

	networkMode := envConfig.NetworkMode
	if networkMode == "" {
		networkMode = "none"
	}
	args = append(args, "--network", networkMode)

	for k, v := range envConfig.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, envConfig.Image)
	return args
}

// StartContainer starts an ephemeral Docker container from the given environment config.
// Returns container info including the assigned host port.
func (cs *ContainerService) StartContainer(ctx context.Context, envConfig models.DockerEnvironmentConfig, timeoutSecs int) (*models.ContainerInfo, error) {
	if timeoutSecs <= 0 {
		timeoutSecs = 300
	}

	// Pick a random high port for the container
	hostPort := 30000 + rand.IntN(30000) //nolint:gosec // non-security random port selection

	args := buildDockerRunArgs(envConfig, hostPort)

	slog.Debug("starting container",
		slog.String("component", "container_service"),
		slog.String("image", envConfig.Image),
		slog.Int("host_port", hostPort),
	)

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // admin-configured Docker image
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w: %s", err, string(output))
	}

	containerID := strings.TrimSpace(string(output))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}

	// Set up auto-teardown
	teardownCtx, cancelFn := context.WithCancel(context.Background()) //nolint:gosec // cancelFn stored in managedContainer and called during teardown
	cs.mu.Lock()
	cs.containers[containerID] = &managedContainer{
		containerID: containerID,
		cancelFn:    cancelFn,
		startedAt:   time.Now(),
	}
	cs.mu.Unlock()

	go cs.autoTeardown(teardownCtx, containerID, time.Duration(timeoutSecs)*time.Second)

	// Wait for health check if configured
	if envConfig.HealthCheck != nil {
		if err := cs.waitForHealthy(ctx, containerID, hostPort, envConfig.HealthCheck); err != nil {
			// Clean up on health check failure
			cs.StopContainer(containerID) //nolint:errcheck // best-effort cleanup
			return nil, fmt.Errorf("container health check failed: %w", err)
		}
	}

	return &models.ContainerInfo{
		ContainerID: containerID,
		Host:        "localhost",
		Port:        hostPort,
	}, nil
}

// StopContainer stops and removes a container.
func (cs *ContainerService) StopContainer(containerID string) error {
	cs.mu.Lock()
	mc, exists := cs.containers[containerID]
	if exists {
		mc.cancelFn()
		delete(cs.containers, containerID)
	}
	cs.mu.Unlock()

	slog.Debug("stopping container",
		slog.String("component", "container_service"),
		slog.String("container_id", containerID),
	)

	cmd := exec.Command("docker", "rm", "-f", containerID) //nolint:gosec // internal container ID
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop container: %w: %s", err, string(output))
	}
	return nil
}

// StopAll stops all managed containers. Called on shutdown.
func (cs *ContainerService) StopAll() {
	cs.mu.Lock()
	ids := make([]string, 0, len(cs.containers))
	for id := range cs.containers {
		ids = append(ids, id)
	}
	cs.mu.Unlock()

	for _, id := range ids {
		if err := cs.StopContainer(id); err != nil {
			slog.Warn("failed to stop container on shutdown",
				slog.String("component", "container_service"),
				slog.String("container_id", id),
				slog.Any("error", err),
			)
		}
	}
}

func (cs *ContainerService) autoTeardown(ctx context.Context, containerID string, timeout time.Duration) {
	select {
	case <-ctx.Done():
		return // Manually stopped
	case <-time.After(timeout):
		slog.Info("auto-tearing down container after timeout",
			slog.String("component", "container_service"),
			slog.String("container_id", containerID),
			slog.Duration("timeout", timeout),
		)
		if err := cs.StopContainer(containerID); err != nil {
			slog.Warn("failed to auto-teardown container",
				slog.String("component", "container_service"),
				slog.String("container_id", containerID),
				slog.Any("error", err),
			)
		}
	}
}

func (cs *ContainerService) waitForHealthy(ctx context.Context, containerID string, hostPort int, hc *models.HealthCheckConfig) error {
	interval := time.Duration(hc.IntervalSec) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	timeout := time.Duration(hc.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	deadline := time.Now().Add(timeout)
	endpoint := fmt.Sprintf("http://localhost:%d%s", hostPort, hc.Endpoint)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Use curl inside the context since we may not have direct network access
		checkCmd := exec.CommandContext(ctx, "curl", "-sf", "--max-time", "2", endpoint) //nolint:gosec // admin-configured health check URL
		if err := checkCmd.Run(); err == nil {
			slog.Debug("container health check passed",
				slog.String("component", "container_service"),
				slog.String("container_id", containerID),
			)
			return nil
		}

		time.Sleep(interval)
	}

	return fmt.Errorf("container %s did not become healthy within %v", containerID, timeout)
}
