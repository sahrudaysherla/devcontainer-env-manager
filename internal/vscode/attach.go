// Package vscode provides integration with Visual Studio Code Remote Containers,
// enabling seamless attachment to running dev containers for debugging and editing.
package vscode

import (
	"encoding/hex"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Attach opens VS Code attached to a running dev container.
// It uses the Remote - Containers extension's URI scheme to connect
// VS Code to the container for inline editing and debugging.
func Attach(containerID, workspaceFolder string) error {
	codePath, err := findVSCode()
	if err != nil {
		return err
	}

	// Encode container ID as hex for the URI
	hexID := hex.EncodeToString([]byte(containerID))

	// Build the vscode-remote URI
	// Format: vscode-remote://attached-container+<hex_container_id><workspace_folder>
	uri := fmt.Sprintf("vscode-remote://attached-container+%s%s", hexID, workspaceFolder)

	fmt.Printf("🔗 Attaching VS Code to container %s...\n", containerID[:12])
	fmt.Printf("   URI: %s\n", uri)

	cmd := exec.Command(codePath, "--folder-uri", uri)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch VS Code: %w", err)
	}

	fmt.Printf("✅ VS Code launched and attaching to container\n")
	return nil
}

// IsInstalled checks if VS Code CLI (code command) is available in PATH.
func IsInstalled() bool {
	_, err := findVSCode()
	return err == nil
}

// findVSCode locates the VS Code CLI executable.
func findVSCode() (string, error) {
	// Try common names for the VS Code CLI
	names := []string{"code"}

	if runtime.GOOS == "windows" {
		names = append(names, "code.cmd", "code.exe")
	}

	for _, name := range names {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf(
		"VS Code CLI not found in PATH. Please install VS Code and ensure " +
			"the 'code' command is available. You can install it from the " +
			"VS Code Command Palette: 'Shell Command: Install 'code' command in PATH'",
	)
}

// GetContainerURI generates the VS Code Remote Container URI for a container.
func GetContainerURI(containerID, workspaceFolder string) string {
	hexID := hex.EncodeToString([]byte(containerID))
	return fmt.Sprintf("vscode-remote://attached-container+%s%s", hexID, workspaceFolder)
}

// GetVSCodeExtensions extracts VS Code extension IDs from devcontainer customizations.
func GetVSCodeExtensions(customizations map[string]interface{}) []string {
	if customizations == nil {
		return nil
	}

	vscodeConfig, ok := customizations["vscode"]
	if !ok {
		return nil
	}

	vsMap, ok := vscodeConfig.(map[string]interface{})
	if !ok {
		return nil
	}

	extList, ok := vsMap["extensions"]
	if !ok {
		return nil
	}

	extensions, ok := extList.([]interface{})
	if !ok {
		return nil
	}

	result := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		if s, ok := ext.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// InstallExtensionsInContainer generates commands to install VS Code extensions
// inside a dev container. These are run as part of lifecycle setup.
func InstallExtensionsInContainer(extensions []string) [][]string {
	if len(extensions) == 0 {
		return nil
	}

	// Build a single command to install all extensions
	args := []string{"code-server", "--install-extension"}
	var commands [][]string

	for _, ext := range extensions {
		cmd := make([]string, len(args)+1)
		copy(cmd, args)
		cmd[len(args)] = ext
		commands = append(commands, cmd)
	}

	return commands
}

// FormatContainerInfo returns a formatted string with container connection details.
func FormatContainerInfo(containerID, containerName, workspaceFolder string) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("╔══════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║              Dev Container Ready! 🚀                        ║\n")
	sb.WriteString("╠══════════════════════════════════════════════════════════════╣\n")
	sb.WriteString(fmt.Sprintf("║  Container:  %-46s ║\n", containerName))
	sb.WriteString(fmt.Sprintf("║  ID:         %-46s ║\n", containerID[:12]))
	sb.WriteString(fmt.Sprintf("║  Workspace:  %-46s ║\n", workspaceFolder))
	sb.WriteString("╠══════════════════════════════════════════════════════════════╣\n")
	sb.WriteString("║  Attach VS Code:  devenv attach                            ║\n")
	sb.WriteString("║  Run command:     devenv exec -- <command>                  ║\n")
	sb.WriteString("║  View logs:       devenv logs                              ║\n")
	sb.WriteString("║  Stop:            devenv down                              ║\n")
	sb.WriteString("╚══════════════════════════════════════════════════════════════╝\n")
	return sb.String()
}
