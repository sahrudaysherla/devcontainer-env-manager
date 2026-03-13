package container

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
)

func TestCreateOptionsDefaults(t *testing.T) {
	opts := CreateOptions{
		Name:  "test-container",
		Image: "ubuntu:22.04",
	}

	if opts.Name != "test-container" {
		t.Errorf("expected name 'test-container', got %q", opts.Name)
	}
	if opts.Image != "ubuntu:22.04" {
		t.Errorf("expected image 'ubuntu:22.04', got %q", opts.Image)
	}
}

func TestCreateOptionsWithPorts(t *testing.T) {
	portBindings := nat.PortMap{
		"8080/tcp": []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: "8080"},
		},
		"3000/tcp": []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: "3000"},
		},
	}

	exposedPorts := nat.PortSet{
		"8080/tcp": struct{}{},
		"3000/tcp": struct{}{},
	}

	opts := CreateOptions{
		Name:         "test-ports",
		Image:        "node:18",
		Ports:        portBindings,
		ExposedPorts: exposedPorts,
	}

	if len(opts.Ports) != 2 {
		t.Errorf("expected 2 port bindings, got %d", len(opts.Ports))
	}
	if len(opts.ExposedPorts) != 2 {
		t.Errorf("expected 2 exposed ports, got %d", len(opts.ExposedPorts))
	}
}

func TestCreateOptionsWithMounts(t *testing.T) {
	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: "/host/path",
			Target: "/container/path",
		},
		{
			Type:   mount.TypeVolume,
			Source: "my-volume",
			Target: "/data",
		},
	}

	opts := CreateOptions{
		Name:   "test-mounts",
		Image:  "ubuntu:22.04",
		Mounts: mounts,
	}

	if len(opts.Mounts) != 2 {
		t.Errorf("expected 2 mounts, got %d", len(opts.Mounts))
	}

	if opts.Mounts[0].Source != "/host/path" {
		t.Errorf("expected source '/host/path', got %q", opts.Mounts[0].Source)
	}
}

func TestCreateOptionsWithEnv(t *testing.T) {
	opts := CreateOptions{
		Name:  "test-env",
		Image: "ubuntu:22.04",
		Env: map[string]string{
			"NODE_ENV": "development",
			"PORT":     "3000",
		},
	}

	if len(opts.Env) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(opts.Env))
	}
	if opts.Env["NODE_ENV"] != "development" {
		t.Error("expected NODE_ENV=development")
	}
}

func TestCreateOptionsLabels(t *testing.T) {
	opts := CreateOptions{
		Name:       "test-labels",
		Image:      "ubuntu:22.04",
		ProjectDir: "/home/user/myproject",
		ConfigName: "My Project",
		Labels: map[string]string{
			"custom.label": "value",
		},
	}

	if opts.ProjectDir != "/home/user/myproject" {
		t.Errorf("expected project dir '/home/user/myproject', got %q", opts.ProjectDir)
	}
	if opts.ConfigName != "My Project" {
		t.Errorf("expected config name 'My Project', got %q", opts.ConfigName)
	}
	if opts.Labels["custom.label"] != "value" {
		t.Error("expected custom.label=value")
	}
}

func TestContainerInfoFields(t *testing.T) {
	info := ContainerInfo{
		ID:         "abc123def456",
		Name:       "devenv-myproject",
		Image:      "ubuntu:22.04",
		Status:     "Up 5 minutes",
		State:      "running",
		ProjectDir: "/home/user/myproject",
		ConfigName: "My Project",
		Ports:      []string{"0.0.0.0:8080->8080/tcp"},
	}

	if info.ID != "abc123def456" {
		t.Errorf("unexpected ID: %q", info.ID)
	}
	if info.State != "running" {
		t.Errorf("expected state 'running', got %q", info.State)
	}
	if len(info.Ports) != 1 {
		t.Errorf("expected 1 port, got %d", len(info.Ports))
	}
}
