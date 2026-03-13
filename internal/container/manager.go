// Package container manages the full lifecycle of development containers,
// including creation, starting, stopping, removal, exec, and log streaming.
package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	// LabelManaged marks containers created by this tool.
	LabelManaged = "devenv.managed"

	// LabelProject stores the project directory for identification.
	LabelProject = "devenv.project"

	// LabelConfigName stores the devcontainer config name.
	LabelConfigName = "devenv.config.name"

	// DefaultStopTimeout is the graceful shutdown timeout in seconds.
	DefaultStopTimeout = 10
)

// Manager handles container lifecycle operations.
type Manager struct {
	cli *client.Client
}

// NewManager creates a new container manager from a Docker client.
func NewManager(cli *client.Client) *Manager {
	return &Manager{cli: cli}
}

// CreateOptions defines parameters for creating a new container.
type CreateOptions struct {
	Name           string
	Image          string
	WorkspaceDir   string            // Host directory to mount as workspace
	WorkspaceMount string            // Container mount point
	Env            map[string]string // Environment variables
	Mounts         []mount.Mount     // Additional mounts
	Ports          nat.PortMap       // Port bindings
	ExposedPorts   nat.PortSet       // Exposed ports
	User           string            // Container user
	Labels         map[string]string // Container labels
	Entrypoint     []string          // Custom entrypoint
	Cmd            []string          // Custom command
	RunArgs        []string          // Additional docker run arguments
	ProjectDir     string            // Host project directory (for labeling)
	ConfigName     string            // devcontainer config name
}

// ContainerInfo holds information about a managed container.
type ContainerInfo struct {
	ID         string
	Name       string
	Image      string
	Status     string
	State      string
	ProjectDir string
	ConfigName string
	Created    time.Time
	Ports      []string
}

// Create creates a new development container.
func (m *Manager) Create(ctx context.Context, opts CreateOptions) (string, error) {
	fmt.Printf("📦 Creating container: %s\n", opts.Name)

	// Build environment variables
	envList := make([]string, 0, len(opts.Env))
	for k, v := range opts.Env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	// Set up all mounts including workspace
	allMounts := make([]mount.Mount, 0, len(opts.Mounts)+1)

	// Workspace mount
	if opts.WorkspaceDir != "" && opts.WorkspaceMount != "" {
		allMounts = append(allMounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: opts.WorkspaceDir,
			Target: opts.WorkspaceMount,
		})
	}
	allMounts = append(allMounts, opts.Mounts...)

	// Set up labels
	labels := map[string]string{
		LabelManaged: "true",
	}
	if opts.ProjectDir != "" {
		labels[LabelProject] = opts.ProjectDir
	}
	if opts.ConfigName != "" {
		labels[LabelConfigName] = opts.ConfigName
	}
	for k, v := range opts.Labels {
		labels[k] = v
	}

	// Determine entrypoint and command
	entrypoint := opts.Entrypoint
	cmd := opts.Cmd
	if entrypoint == nil {
		// Keep the container running with a sleep command
		entrypoint = []string{"sh", "-c"}
		cmd = []string{"while sleep 1000; do :; done"}
	}

	containerConfig := &container.Config{
		Image:        opts.Image,
		Env:          envList,
		Labels:       labels,
		User:         opts.User,
		Entrypoint:   entrypoint,
		Cmd:          cmd,
		ExposedPorts: opts.ExposedPorts,
		Tty:          true,
		OpenStdin:    true,
	}

	hostConfig := &container.HostConfig{
		Mounts:       allMounts,
		PortBindings: opts.Ports,
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}

	networkConfig := &network.NetworkingConfig{}

	resp, err := m.cli.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, opts.Name)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	if len(resp.Warnings) > 0 {
		for _, w := range resp.Warnings {
			fmt.Printf("⚠️  Warning: %s\n", w)
		}
	}

	fmt.Printf("✅ Container created: %s (%s)\n", opts.Name, resp.ID[:12])
	return resp.ID, nil
}

// Start starts a container by ID.
func (m *Manager) Start(ctx context.Context, containerID string) error {
	fmt.Printf("🚀 Starting container: %s\n", containerID[:12])

	if err := m.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Wait a moment and verify the container is running
	time.Sleep(500 * time.Millisecond)

	inspect, err := m.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to inspect container after start: %w", err)
	}

	if !inspect.State.Running {
		return fmt.Errorf("container failed to start, status: %s", inspect.State.Status)
	}

	fmt.Printf("✅ Container started and running\n")
	return nil
}

// Stop stops a running container with a graceful timeout.
func (m *Manager) Stop(ctx context.Context, containerID string) error {
	fmt.Printf("🛑 Stopping container: %s\n", containerID[:12])

	timeout := DefaultStopTimeout
	stopOpts := container.StopOptions{
		Timeout: &timeout,
	}

	if err := m.cli.ContainerStop(ctx, containerID, stopOpts); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	fmt.Printf("✅ Container stopped\n")
	return nil
}

// Remove removes a container and its associated resources.
func (m *Manager) Remove(ctx context.Context, containerID string, force bool) error {
	fmt.Printf("🗑️  Removing container: %s\n", containerID[:12])

	removeOpts := container.RemoveOptions{
		Force:         force,
		RemoveVolumes: true,
	}

	if err := m.cli.ContainerRemove(ctx, containerID, removeOpts); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	fmt.Printf("✅ Container removed\n")
	return nil
}

// Exec executes a command inside a running container and returns the exit code.
func (m *Manager) Exec(ctx context.Context, containerID string, cmd []string, user string) (int, error) {
	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		User:         user,
	}

	execResp, err := m.cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return -1, fmt.Errorf("failed to create exec: %w", err)
	}

	attachResp, err := m.cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{Tty: true})
	if err != nil {
		return -1, fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer attachResp.Close()

	// Stream output to stdout
	io.Copy(os.Stdout, attachResp.Reader)

	// Get exit code
	inspectResp, err := m.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return -1, fmt.Errorf("failed to inspect exec: %w", err)
	}

	return inspectResp.ExitCode, nil
}

// GetStatus returns information about a container.
func (m *Manager) GetStatus(ctx context.Context, containerID string) (*ContainerInfo, error) {
	inspect, err := m.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	ports := make([]string, 0)
	for p, bindings := range inspect.NetworkSettings.Ports {
		for _, b := range bindings {
			ports = append(ports, fmt.Sprintf("%s:%s->%s", b.HostIP, b.HostPort, p))
		}
	}

	created, _ := time.Parse(time.RFC3339Nano, inspect.Created)

	return &ContainerInfo{
		ID:         inspect.ID,
		Name:       strings.TrimPrefix(inspect.Name, "/"),
		Image:      inspect.Config.Image,
		Status:     inspect.State.Status,
		State:      inspect.State.Status,
		ProjectDir: inspect.Config.Labels[LabelProject],
		ConfigName: inspect.Config.Labels[LabelConfigName],
		Created:    created,
		Ports:      ports,
	}, nil
}

// FindByProject finds a managed container for a given project directory.
func (m *Manager) FindByProject(ctx context.Context, projectDir string) (*ContainerInfo, error) {
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", LabelManaged+"=true")
	filterArgs.Add("label", LabelProject+"="+projectDir)

	containers, err := m.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		return nil, nil
	}

	c := containers[0]
	created := time.Unix(c.Created, 0)

	return &ContainerInfo{
		ID:         c.ID,
		Name:       strings.TrimPrefix(c.Names[0], "/"),
		Image:      c.Image,
		Status:     c.Status,
		State:      c.State,
		ProjectDir: c.Labels[LabelProject],
		ConfigName: c.Labels[LabelConfigName],
		Created:    created,
	}, nil
}

// ListAll lists all containers managed by devenv.
func (m *Manager) ListAll(ctx context.Context) ([]ContainerInfo, error) {
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", LabelManaged+"=true")

	containers, err := m.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	infos := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		ports := make([]string, 0)
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				ports = append(ports, fmt.Sprintf("%d->%d/%s", p.PublicPort, p.PrivatePort, p.Type))
			}
		}

		created := time.Unix(c.Created, 0)

		infos = append(infos, ContainerInfo{
			ID:         c.ID,
			Name:       name,
			Image:      c.Image,
			Status:     c.Status,
			State:      c.State,
			ProjectDir: c.Labels[LabelProject],
			ConfigName: c.Labels[LabelConfigName],
			Created:    created,
			Ports:      ports,
		})
	}

	return infos, nil
}

// StreamLogs streams container logs to stdout.
func (m *Manager) StreamLogs(ctx context.Context, containerID string, follow bool) error {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: true,
		Tail:       "100",
	}

	reader, err := m.cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return fmt.Errorf("failed to get container logs: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(os.Stdout, reader)
	return err
}

// IsRunning checks if a container is currently running.
func (m *Manager) IsRunning(ctx context.Context, containerID string) (bool, error) {
	inspect, err := m.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, err
	}
	return inspect.State.Running, nil
}

// WaitForReady waits for a container to be in a running state.
func (m *Manager) WaitForReady(ctx context.Context, containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, err := m.IsRunning(ctx, containerID)
		if err != nil {
			return err
		}
		if running {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("container did not become ready within %s", timeout)
}
