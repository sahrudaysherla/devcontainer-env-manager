// Package config handles parsing and validation of devcontainer.json configuration files.
// It supports the Dev Container specification including image-based and Dockerfile-based
// configurations, lifecycle hooks, port forwarding, mounts, and feature definitions.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DevContainerConfig represents the full devcontainer.json specification.
type DevContainerConfig struct {
	// General properties
	Name string `json:"name,omitempty"`

	// Image-based configuration
	Image string `json:"image,omitempty"`

	// Dockerfile-based configuration
	Build *BuildConfig `json:"build,omitempty"`

	// Runtime configuration
	ForwardPorts  []interface{}     `json:"forwardPorts,omitempty"`
	AppPort       []interface{}     `json:"appPort,omitempty"`
	ContainerEnv  map[string]string `json:"containerEnv,omitempty"`
	RemoteEnv     map[string]string `json:"remoteEnv,omitempty"`
	ContainerUser string            `json:"containerUser,omitempty"`
	RemoteUser    string            `json:"remoteUser,omitempty"`
	Mounts        []interface{}     `json:"mounts,omitempty"`
	RunArgs       []string          `json:"runArgs,omitempty"`
	WorkspaceMount string           `json:"workspaceMount,omitempty"`
	WorkspaceFolder string          `json:"workspaceFolder,omitempty"`

	// Features
	Features map[string]interface{} `json:"features,omitempty"`

	// Customizations (VS Code settings, extensions, etc.)
	Customizations map[string]interface{} `json:"customizations,omitempty"`

	// Lifecycle hooks - can be string, []string, or map[string]string/[]string
	InitializeCommand   interface{} `json:"initializeCommand,omitempty"`
	OnCreateCommand     interface{} `json:"onCreateCommand,omitempty"`
	UpdateContentCommand interface{} `json:"updateContentCommand,omitempty"`
	PostCreateCommand   interface{} `json:"postCreateCommand,omitempty"`
	PostStartCommand    interface{} `json:"postStartCommand,omitempty"`
	PostAttachCommand   interface{} `json:"postAttachCommand,omitempty"`

	// Metadata
	HostRequirements *HostRequirements `json:"hostRequirements,omitempty"`
	OverrideCommand  *bool             `json:"overrideCommand,omitempty"`
	ShutdownAction   string            `json:"shutdownAction,omitempty"`
	UserEnvProbe     string            `json:"userEnvProbe,omitempty"`

	// Internal: the directory where the config was loaded from
	configDir   string
	projectDir  string
}

// BuildConfig represents the "build" section of devcontainer.json.
type BuildConfig struct {
	Dockerfile string            `json:"dockerfile,omitempty"`
	Context    string            `json:"context,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Target     string            `json:"target,omitempty"`
	CacheFrom  []string          `json:"cacheFrom,omitempty"`
}

// HostRequirements specifies minimum host requirements.
type HostRequirements struct {
	CPUs    int    `json:"cpus,omitempty"`
	Memory  string `json:"memory,omitempty"`
	Storage string `json:"storage,omitempty"`
	GPU     interface{} `json:"gpu,omitempty"`
}

// Load discovers and parses a devcontainer.json from the given project path.
// It searches in the following order:
//  1. .devcontainer/devcontainer.json
//  2. .devcontainer.json
//  3. .devcontainer/<subdirectory>/devcontainer.json (first found)
func Load(projectPath string) (*DevContainerConfig, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project path: %w", err)
	}

	searchPaths := []string{
		filepath.Join(absPath, ".devcontainer", "devcontainer.json"),
		filepath.Join(absPath, ".devcontainer.json"),
	}

	// Also search for subdirectories in .devcontainer/
	devcontainerDir := filepath.Join(absPath, ".devcontainer")
	if entries, err := os.ReadDir(devcontainerDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				subPath := filepath.Join(devcontainerDir, entry.Name(), "devcontainer.json")
				searchPaths = append(searchPaths, subPath)
			}
		}
	}

	for _, configPath := range searchPaths {
		if _, err := os.Stat(configPath); err == nil {
			config, err := loadFromFile(configPath)
			if err != nil {
				return nil, err
			}
			config.configDir = filepath.Dir(configPath)
			config.projectDir = absPath
			return config, nil
		}
	}

	return nil, fmt.Errorf("no devcontainer.json found in %s", absPath)
}

// loadFromFile reads and parses a devcontainer.json file, handling JSONC (comments).
func loadFromFile(path string) (*DevContainerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// Strip JSONC comments (// and /* */ style)
	cleaned := stripJSONComments(string(data))

	// Strip trailing commas before } or ]
	cleaned = stripTrailingCommas(cleaned)

	var config DevContainerConfig
	if err := json.Unmarshal([]byte(cleaned), &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid devcontainer config in %s: %w", path, err)
	}

	return &config, nil
}

// stripJSONComments removes single-line (//) and multi-line (/* */) comments
// from a JSON string, respecting string literals.
func stripJSONComments(input string) string {
	var result strings.Builder
	inString := false
	escaped := false
	i := 0

	for i < len(input) {
		ch := input[i]

		if escaped {
			result.WriteByte(ch)
			escaped = false
			i++
			continue
		}

		if ch == '\\' && inString {
			result.WriteByte(ch)
			escaped = true
			i++
			continue
		}

		if ch == '"' {
			inString = !inString
			result.WriteByte(ch)
			i++
			continue
		}

		if !inString {
			// Check for single-line comment
			if i+1 < len(input) && ch == '/' && input[i+1] == '/' {
				// Skip until end of line
				for i < len(input) && input[i] != '\n' {
					i++
				}
				continue
			}
			// Check for multi-line comment
			if i+1 < len(input) && ch == '/' && input[i+1] == '*' {
				i += 2
				for i+1 < len(input) {
					if input[i] == '*' && input[i+1] == '/' {
						i += 2
						break
					}
					i++
				}
				continue
			}
		}

		result.WriteByte(ch)
		i++
	}

	return result.String()
}

// stripTrailingCommas removes trailing commas before } and ].
func stripTrailingCommas(input string) string {
	re := regexp.MustCompile(`,\s*([}\]])`)
	return re.ReplaceAllString(input, "$1")
}

// validate checks that the config has at least an image or build config specified.
func (c *DevContainerConfig) validate() error {
	if c.Image == "" && c.Build == nil {
		return fmt.Errorf("either 'image' or 'build' must be specified")
	}

	if c.Build != nil && c.Build.Dockerfile == "" {
		return fmt.Errorf("'build.dockerfile' is required when 'build' is specified")
	}

	return nil
}

// GetDockerfilePath returns the absolute path to the Dockerfile for build-based configs.
func (c *DevContainerConfig) GetDockerfilePath() string {
	if c.Build == nil || c.Build.Dockerfile == "" {
		return ""
	}
	return filepath.Join(c.configDir, c.Build.Dockerfile)
}

// GetBuildContext returns the build context directory.
func (c *DevContainerConfig) GetBuildContext() string {
	if c.Build == nil {
		return ""
	}
	if c.Build.Context != "" {
		return filepath.Join(c.configDir, c.Build.Context)
	}
	return c.configDir
}

// GetWorkspaceFolder returns the workspace folder inside the container.
func (c *DevContainerConfig) GetWorkspaceFolder() string {
	if c.WorkspaceFolder != "" {
		return c.WorkspaceFolder
	}
	return "/workspaces/" + filepath.Base(c.projectDir)
}

// GetProjectDir returns the host project directory.
func (c *DevContainerConfig) GetProjectDir() string {
	return c.projectDir
}

// GetConfigDir returns the directory containing the devcontainer.json.
func (c *DevContainerConfig) GetConfigDir() string {
	return c.configDir
}

// GetEffectiveUser returns the remote user, falling back to containerUser or "root".
func (c *DevContainerConfig) GetEffectiveUser() string {
	if c.RemoteUser != "" {
		return c.RemoteUser
	}
	if c.ContainerUser != "" {
		return c.ContainerUser
	}
	return "root"
}

// GetContainerName returns a deterministic container name based on project path.
func (c *DevContainerConfig) GetContainerName() string {
	name := c.Name
	if name == "" {
		name = filepath.Base(c.projectDir)
	}
	// Sanitize for Docker container naming
	sanitized := strings.ToLower(name)
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	re := regexp.MustCompile(`[^a-z0-9\-_.]`)
	sanitized = re.ReplaceAllString(sanitized, "")
	return "devenv-" + sanitized
}

// ResolveLifecycleCommands extracts commands from a lifecycle hook value.
// Lifecycle hooks can be: string, []string, or map[string](string|[]string).
func ResolveLifecycleCommands(hook interface{}) [][]string {
	if hook == nil {
		return nil
	}

	switch v := hook.(type) {
	case string:
		if v == "" {
			return nil
		}
		return [][]string{{"sh", "-c", v}}
	case []interface{}:
		cmd := make([]string, len(v))
		for i, arg := range v {
			cmd[i] = fmt.Sprintf("%v", arg)
		}
		return [][]string{cmd}
	case map[string]interface{}:
		var commands [][]string
		for _, val := range v {
			sub := ResolveLifecycleCommands(val)
			commands = append(commands, sub...)
		}
		return commands
	default:
		return nil
	}
}
