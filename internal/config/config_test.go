package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	devcontainerDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatal(err)
	}

	configJSON := `{
		// This is a comment
		"name": "Test Project",
		"image": "mcr.microsoft.com/devcontainers/go:1.22",
		"forwardPorts": [8080, 3000],
		"containerEnv": {
			"GO111MODULE": "on"
		},
		"remoteUser": "vscode",
		"postCreateCommand": "go mod download",
		"customizations": {
			"vscode": {
				"extensions": ["golang.Go"]
			}
		},
	}`

	configPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := Load(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if config.Name != "Test Project" {
		t.Errorf("expected name 'Test Project', got %q", config.Name)
	}
	if config.Image != "mcr.microsoft.com/devcontainers/go:1.22" {
		t.Errorf("unexpected image: %q", config.Image)
	}
	if config.RemoteUser != "vscode" {
		t.Errorf("expected remoteUser 'vscode', got %q", config.RemoteUser)
	}
	if len(config.ForwardPorts) != 2 {
		t.Errorf("expected 2 forward ports, got %d", len(config.ForwardPorts))
	}
	if config.ContainerEnv["GO111MODULE"] != "on" {
		t.Error("expected GO111MODULE=on in containerEnv")
	}
}

func TestLoadBuildConfig(t *testing.T) {
	dir := t.TempDir()
	devcontainerDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatal(err)
	}

	configJSON := `{
		"name": "Build Test",
		"build": {
			"dockerfile": "Dockerfile",
			"context": "..",
			"args": {
				"VARIANT": "3.11"
			}
		}
	}`

	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := Load(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if config.Build == nil {
		t.Fatal("expected build config to be set")
	}
	if config.Build.Dockerfile != "Dockerfile" {
		t.Errorf("expected dockerfile 'Dockerfile', got %q", config.Build.Dockerfile)
	}
	if config.Build.Args["VARIANT"] != "3.11" {
		t.Error("expected VARIANT=3.11 in build args")
	}
}

func TestLoadMissingConfig(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for missing config")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	devcontainerDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(devcontainerDir, "devcontainer.json"),
		[]byte(`{invalid json}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadMissingImageAndBuild(t *testing.T) {
	dir := t.TempDir()
	devcontainerDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatal(err)
	}

	configJSON := `{"name": "No Image"}`
	if err := os.WriteFile(
		filepath.Join(devcontainerDir, "devcontainer.json"),
		[]byte(configJSON),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Error("expected validation error for missing image/build")
	}
}

func TestLoadRootDevcontainerJSON(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"name": "Root Config",
		"image": "ubuntu:22.04"
	}`

	if err := os.WriteFile(filepath.Join(dir, ".devcontainer.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := Load(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if config.Name != "Root Config" {
		t.Errorf("expected name 'Root Config', got %q", config.Name)
	}
}

func TestStripJSONComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single line comment",
			input:    `{"key": "value" // comment\n}`,
			expected: `{"key": "value" `,
		},
		{
			name:     "multi-line comment",
			input:    `{"key": /* comment */ "value"}`,
			expected: `{"key":  "value"}`,
		},
		{
			name:     "comment-like string",
			input:    `{"key": "value // not a comment"}`,
			expected: `{"key": "value // not a comment"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripJSONComments(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestResolveLifecycleCommands(t *testing.T) {
	// String command
	cmds := ResolveLifecycleCommands("echo hello")
	if len(cmds) != 1 || cmds[0][2] != "echo hello" {
		t.Error("unexpected result for string command")
	}

	// Nil command
	cmds = ResolveLifecycleCommands(nil)
	if cmds != nil {
		t.Error("expected nil for nil hook")
	}

	// Empty string
	cmds = ResolveLifecycleCommands("")
	if cmds != nil {
		t.Error("expected nil for empty string")
	}
}

func TestGetContainerName(t *testing.T) {
	config := &DevContainerConfig{
		Name: "My Test Project",
	}
	name := config.GetContainerName()
	if name != "devenv-my-test-project" {
		t.Errorf("expected 'devenv-my-test-project', got %q", name)
	}
}

func TestGetEffectiveUser(t *testing.T) {
	config := &DevContainerConfig{}
	if config.GetEffectiveUser() != "root" {
		t.Error("expected default user 'root'")
	}

	config.ContainerUser = "dev"
	if config.GetEffectiveUser() != "dev" {
		t.Error("expected containerUser 'dev'")
	}

	config.RemoteUser = "vscode"
	if config.GetEffectiveUser() != "vscode" {
		t.Error("expected remoteUser 'vscode' to take priority")
	}
}

func TestGetWorkspaceFolder(t *testing.T) {
	config := &DevContainerConfig{
		projectDir: "/home/user/myproject",
	}
	if config.GetWorkspaceFolder() != "/workspaces/myproject" {
		t.Errorf("expected '/workspaces/myproject', got %q", config.GetWorkspaceFolder())
	}

	config.WorkspaceFolder = "/custom/workspace"
	if config.GetWorkspaceFolder() != "/custom/workspace" {
		t.Errorf("expected '/custom/workspace' override, got %q", config.GetWorkspaceFolder())
	}
}
