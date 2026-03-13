// Package workspace handles workspace provisioning, mount preparation,
// port binding resolution, and environment variable merging for dev containers.
package workspace

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"

	"github.com/sahru/devcontainer-env-manager/internal/config"
)

// PrepareConfig holds the resolved workspace configuration ready for container creation.
type PrepareConfig struct {
	Image          string
	ContainerName  string
	WorkspaceDir   string
	WorkspaceMount string
	Env            map[string]string
	Mounts         []mount.Mount
	Ports          nat.PortMap
	ExposedPorts   nat.PortSet
	User           string
}

// Prepare resolves a DevContainerConfig into a PrepareConfig suitable for container creation.
func Prepare(cfg *config.DevContainerConfig) (*PrepareConfig, error) {
	pc := &PrepareConfig{
		Image:          cfg.Image,
		ContainerName:  cfg.GetContainerName(),
		WorkspaceDir:   cfg.GetProjectDir(),
		WorkspaceMount: cfg.GetWorkspaceFolder(),
		User:           cfg.GetEffectiveUser(),
	}

	// Resolve environment variables
	pc.Env = PrepareEnvironment(cfg)

	// Resolve mounts
	mounts, err := PrepareMounts(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare mounts: %w", err)
	}
	pc.Mounts = mounts

	// Resolve port bindings
	portMap, exposedPorts, err := PreparePortBindings(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare port bindings: %w", err)
	}
	pc.Ports = portMap
	pc.ExposedPorts = exposedPorts

	return pc, nil
}

// PrepareEnvironment merges containerEnv and remoteEnv into a single map.
// containerEnv takes lower priority; remoteEnv overrides.
func PrepareEnvironment(cfg *config.DevContainerConfig) map[string]string {
	env := make(map[string]string)

	for k, v := range cfg.ContainerEnv {
		env[k] = v
	}
	for k, v := range cfg.RemoteEnv {
		env[k] = v
	}

	return env
}

// PrepareMounts converts devcontainer mount specifications to Docker mount configs.
// Supports both string format ("type=bind,source=...,target=...") and object format.
func PrepareMounts(cfg *config.DevContainerConfig) ([]mount.Mount, error) {
	var mounts []mount.Mount

	for _, m := range cfg.Mounts {
		switch v := m.(type) {
		case string:
			parsed, err := parseMountString(v)
			if err != nil {
				return nil, fmt.Errorf("invalid mount spec %q: %w", v, err)
			}
			mounts = append(mounts, parsed)

		case map[string]interface{}:
			parsed, err := parseMountObject(v)
			if err != nil {
				return nil, fmt.Errorf("invalid mount object: %w", err)
			}
			mounts = append(mounts, parsed)
		}
	}

	return mounts, nil
}

// parseMountString parses a Docker-style mount string (type=bind,source=/a,target=/b).
func parseMountString(spec string) (mount.Mount, error) {
	m := mount.Mount{}
	parts := strings.Split(spec, ",")

	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		switch key {
		case "type":
			m.Type = mount.Type(val)
		case "source", "src":
			m.Source = val
		case "target", "dst", "destination":
			m.Target = val
		case "readonly", "ro":
			m.ReadOnly = val == "true" || val == "1"
		}
	}

	if m.Type == "" {
		m.Type = mount.TypeBind
	}
	if m.Target == "" {
		return m, fmt.Errorf("mount target is required")
	}

	return m, nil
}

// parseMountObject parses a mount from a JSON object format.
func parseMountObject(obj map[string]interface{}) (mount.Mount, error) {
	m := mount.Mount{}

	if t, ok := obj["type"].(string); ok {
		m.Type = mount.Type(t)
	} else {
		m.Type = mount.TypeBind
	}

	if src, ok := obj["source"].(string); ok {
		m.Source = src
	}
	if tgt, ok := obj["target"].(string); ok {
		m.Target = tgt
	}

	if m.Target == "" {
		return m, fmt.Errorf("mount target is required")
	}

	return m, nil
}

// PreparePortBindings converts forwardPorts configuration to Docker port maps.
// forwardPorts can contain integers or "host:container" strings.
func PreparePortBindings(cfg *config.DevContainerConfig) (nat.PortMap, nat.PortSet, error) {
	portMap := nat.PortMap{}
	exposedPorts := nat.PortSet{}

	for _, p := range cfg.ForwardPorts {
		hostPort, containerPort, err := resolvePort(p)
		if err != nil {
			return nil, nil, err
		}

		port, err := nat.NewPort("tcp", containerPort)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid port %s: %w", containerPort, err)
		}

		portMap[port] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: hostPort,
			},
		}
		exposedPorts[port] = struct{}{}
	}

	// Also handle appPort
	for _, p := range cfg.AppPort {
		hostPort, containerPort, err := resolvePort(p)
		if err != nil {
			return nil, nil, err
		}

		port, err := nat.NewPort("tcp", containerPort)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid port %s: %w", containerPort, err)
		}

		portMap[port] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: hostPort,
			},
		}
		exposedPorts[port] = struct{}{}
	}

	return portMap, exposedPorts, nil
}

// resolvePort resolves a port specification to host:container port pair.
func resolvePort(p interface{}) (string, string, error) {
	switch v := p.(type) {
	case float64:
		port := strconv.Itoa(int(v))
		return port, port, nil
	case string:
		parts := strings.SplitN(v, ":", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
		return v, v, nil
	default:
		return "", "", fmt.Errorf("unsupported port format: %v", p)
	}
}

// ResolveProjectDir returns the absolute project directory path.
func ResolveProjectDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve project directory: %w", err)
	}
	return abs, nil
}
