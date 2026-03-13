// DevContainer Environment Manager (devenv) is a CLI-based platform tool for managing
// reproducible development environments based on the Dev Container specification.
//
// It uses the Docker SDK to automatically provision, configure, and tear down
// containerized workspaces, ensuring consistency between local development and
// CI pipelines. It integrates with VS Code Remote Containers for seamless
// attachment to running environments.
//
// Usage:
//
//	devenv init          - Initialize a project with devcontainer.json
//	devenv up            - Build and start a dev environment
//	devenv down          - Stop and tear down an environment
//	devenv status        - Show environment status
//	devenv exec          - Execute a command in a container
//	devenv attach        - Attach VS Code to a container
//	devenv list          - List all managed environments
//	devenv logs          - Stream container logs
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sahru/devcontainer-env-manager/internal/config"
	ctr "github.com/sahru/devcontainer-env-manager/internal/container"
	"github.com/sahru/devcontainer-env-manager/internal/docker"
	"github.com/sahru/devcontainer-env-manager/internal/lifecycle"
	"github.com/sahru/devcontainer-env-manager/internal/vscode"
	"github.com/sahru/devcontainer-env-manager/internal/workspace"
)

var version = "1.0.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "devenv",
		Short: "DevContainer Environment Manager",
		Long: `DevContainer Environment Manager (devenv) is a CLI tool for managing 
reproducible development environments based on the Dev Container specification.

It uses the Docker SDK to provision, configure, and tear down containerized 
workspaces with VS Code Remote Containers integration.`,
		Version: version,
	}

	rootCmd.AddCommand(
		newInitCmd(),
		newUpCmd(),
		newDownCmd(),
		newStatusCmd(),
		newExecCmd(),
		newAttachCmd(),
		newListCmd(),
		newLogsCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// newInitCmd creates the 'init' subcommand to scaffold a devcontainer.json.
func newInitCmd() *cobra.Command {
	var template string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a project with a devcontainer configuration",
		Long:  `Creates a .devcontainer/devcontainer.json file in the current directory with a starter configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(template)
		},
	}

	cmd.Flags().StringVarP(&template, "template", "t", "default", "Template to use (default, go, python, node)")
	return cmd
}

// newUpCmd creates the 'up' subcommand to build and start a dev environment.
func newUpCmd() *cobra.Command {
	var projectDir string
	var rebuild bool

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Build and start a development environment",
		Long: `Reads the devcontainer.json configuration, pulls or builds the container image, 
creates and starts the container, binds workspace mounts, forwards ports, 
and executes lifecycle hooks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(projectDir, rebuild)
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project-dir", "p", ".", "Path to the project directory")
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Force rebuild of the container image")
	return cmd
}

// newDownCmd creates the 'down' subcommand to stop and remove a container.
func newDownCmd() *cobra.Command {
	var projectDir string
	var removeVolumes bool

	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop and tear down a development environment",
		Long:  `Stops the running container and removes it along with associated resources.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDown(projectDir, removeVolumes)
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project-dir", "p", ".", "Path to the project directory")
	cmd.Flags().BoolVar(&removeVolumes, "volumes", false, "Also remove associated volumes")
	return cmd
}

// newStatusCmd creates the 'status' subcommand to show environment status.
func newStatusCmd() *cobra.Command {
	var projectDir string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the status of the current development environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(projectDir)
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project-dir", "p", ".", "Path to the project directory")
	return cmd
}

// newExecCmd creates the 'exec' subcommand to run commands in a container.
func newExecCmd() *cobra.Command {
	var projectDir string
	var user string

	cmd := &cobra.Command{
		Use:   "exec [flags] -- <command> [args...]",
		Short: "Execute a command in the running development container",
		Long:  `Runs a command inside the running dev container. Use -- to separate devenv flags from the command.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExec(projectDir, user, args)
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project-dir", "p", ".", "Path to the project directory")
	cmd.Flags().StringVarP(&user, "user", "u", "", "User to run the command as (default: from config)")
	return cmd
}

// newAttachCmd creates the 'attach' subcommand to open VS Code.
func newAttachCmd() *cobra.Command {
	var projectDir string

	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach VS Code to the running development container",
		Long:  `Opens VS Code with the Remote - Containers extension attached to the running dev container.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttach(projectDir)
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project-dir", "p", ".", "Path to the project directory")
	return cmd
}

// newListCmd creates the 'list' subcommand to list all managed environments.
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all managed development environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList()
		},
	}

	return cmd
}

// newLogsCmd creates the 'logs' subcommand to stream container logs.
func newLogsCmd() *cobra.Command {
	var projectDir string
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Stream logs from the development container",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(projectDir, follow)
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project-dir", "p", ".", "Path to the project directory")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	return cmd
}

// ─────────────────────────────────────────────────────────────────────────────
// Command implementations
// ─────────────────────────────────────────────────────────────────────────────

func runInit(template string) error {
	devcontainerDir := filepath.Join(".", ".devcontainer")
	configPath := filepath.Join(devcontainerDir, "devcontainer.json")

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("devcontainer.json already exists at %s", configPath)
	}

	// Create directory
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		return fmt.Errorf("failed to create .devcontainer directory: %w", err)
	}

	// Get template config
	configData := getTemplate(template)

	// Write config
	data, err := json.MarshalIndent(configData, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("✅ Created %s with '%s' template\n", configPath, template)
	fmt.Println("\n📝 Next steps:")
	fmt.Println("   1. Edit .devcontainer/devcontainer.json to customize your environment")
	fmt.Println("   2. Run 'devenv up' to start your development container")
	return nil
}

func runUp(projectDir string, rebuild bool) error {
	ctx, cancel := contextWithSignal()
	defer cancel()

	// Resolve project directory
	absDir, err := workspace.ResolveProjectDir(projectDir)
	if err != nil {
		return err
	}

	// Load devcontainer configuration
	fmt.Printf("📋 Loading configuration from %s\n", absDir)
	cfg, err := config.Load(absDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	fmt.Printf("   Name: %s\n", cfg.Name)

	// Create Docker client
	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	// Verify Docker connectivity
	if err := dockerClient.Ping(ctx); err != nil {
		return err
	}

	// Create container manager
	manager := ctr.NewManager(dockerClient.Inner())

	// Check if container already exists
	existing, err := manager.FindByProject(ctx, absDir)
	if err != nil {
		return err
	}

	if existing != nil && !rebuild {
		if existing.State == "running" {
			fmt.Printf("ℹ️  Container '%s' is already running\n", existing.Name)
			fmt.Print(vscode.FormatContainerInfo(existing.ID, existing.Name, cfg.GetWorkspaceFolder()))
			return nil
		}

		// Restart existing stopped container
		fmt.Printf("🔄 Restarting existing container '%s'\n", existing.Name)
		if err := manager.Start(ctx, existing.ID); err != nil {
			return err
		}

		// Run post-start hooks
		executor := lifecycle.NewExecutor(manager, existing.ID, cfg.GetEffectiveUser(), absDir)
		if err := executor.RunPostStart(ctx, cfg); err != nil {
			fmt.Printf("⚠️  Post-start command failed: %v\n", err)
		}

		fmt.Print(vscode.FormatContainerInfo(existing.ID, existing.Name, cfg.GetWorkspaceFolder()))
		return nil
	}

	// If rebuilding, remove existing container
	if existing != nil && rebuild {
		fmt.Println("🔄 Rebuilding: removing existing container...")
		if existing.State == "running" {
			_ = manager.Stop(ctx, existing.ID)
		}
		_ = manager.Remove(ctx, existing.ID, true)
	}

	// Ensure image is available (pull or build)
	imageName := cfg.Image
	if cfg.Build != nil {
		imageName = cfg.GetContainerName() + ":latest"
		buildCfg := &docker.BuildConfig{
			ContextDir: cfg.GetBuildContext(),
			Dockerfile: cfg.Build.Dockerfile,
			Args:       cfg.Build.Args,
		}
		if err := dockerClient.EnsureImage(ctx, imageName, buildCfg); err != nil {
			return fmt.Errorf("failed to prepare image: %w", err)
		}
	} else {
		if err := dockerClient.EnsureImage(ctx, imageName, nil); err != nil {
			return fmt.Errorf("failed to pull image: %w", err)
		}
	}

	// Prepare workspace configuration
	wsConfig, err := workspace.Prepare(cfg)
	if err != nil {
		return err
	}

	// Create container
	containerID, err := manager.Create(ctx, ctr.CreateOptions{
		Name:           wsConfig.ContainerName,
		Image:          imageName,
		WorkspaceDir:   wsConfig.WorkspaceDir,
		WorkspaceMount: wsConfig.WorkspaceMount,
		Env:            wsConfig.Env,
		Mounts:         wsConfig.Mounts,
		Ports:          wsConfig.Ports,
		ExposedPorts:   wsConfig.ExposedPorts,
		User:           wsConfig.User,
		ProjectDir:     absDir,
		ConfigName:     cfg.Name,
	})
	if err != nil {
		return err
	}

	// Start container
	if err := manager.Start(ctx, containerID); err != nil {
		return err
	}

	// Execute lifecycle hooks
	executor := lifecycle.NewExecutor(manager, containerID, cfg.GetEffectiveUser(), absDir)
	if err := executor.ExecuteAll(ctx, cfg); err != nil {
		fmt.Printf("⚠️  Lifecycle hook failed: %v\n", err)
		fmt.Println("   Container is still running. You may need to run hooks manually.")
	}

	// Display connection info
	fmt.Print(vscode.FormatContainerInfo(containerID, wsConfig.ContainerName, wsConfig.WorkspaceMount))
	return nil
}

func runDown(projectDir string, removeVolumes bool) error {
	ctx, cancel := contextWithSignal()
	defer cancel()

	absDir, err := workspace.ResolveProjectDir(projectDir)
	if err != nil {
		return err
	}

	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	manager := ctr.NewManager(dockerClient.Inner())

	existing, err := manager.FindByProject(ctx, absDir)
	if err != nil {
		return err
	}
	if existing == nil {
		fmt.Println("ℹ️  No development environment found for this project")
		return nil
	}

	fmt.Printf("🛑 Tearing down environment '%s'\n", existing.Name)

	// Stop if running
	if existing.State == "running" {
		if err := manager.Stop(ctx, existing.ID); err != nil {
			return err
		}
	}

	// Remove container
	if err := manager.Remove(ctx, existing.ID, true); err != nil {
		return err
	}

	fmt.Println("✅ Development environment removed")
	return nil
}

func runStatus(projectDir string) error {
	ctx, cancel := contextWithSignal()
	defer cancel()

	absDir, err := workspace.ResolveProjectDir(projectDir)
	if err != nil {
		return err
	}

	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	manager := ctr.NewManager(dockerClient.Inner())

	existing, err := manager.FindByProject(ctx, absDir)
	if err != nil {
		return err
	}

	if existing == nil {
		fmt.Println("ℹ️  No development environment found for this project")
		fmt.Println("   Run 'devenv up' to start one")
		return nil
	}

	// Get detailed status
	info, err := manager.GetStatus(ctx, existing.ID)
	if err != nil {
		return err
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║              Environment Status                            ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Name:       %-46s ║\n", info.Name)
	fmt.Printf("║  ID:         %-46s ║\n", info.ID[:12])
	fmt.Printf("║  Image:      %-46s ║\n", truncate(info.Image, 46))
	fmt.Printf("║  Status:     %-46s ║\n", info.Status)
	fmt.Printf("║  Created:    %-46s ║\n", info.Created.Format("2006-01-02 15:04:05"))

	if info.ProjectDir != "" {
		fmt.Printf("║  Project:    %-46s ║\n", truncate(info.ProjectDir, 46))
	}
	if len(info.Ports) > 0 {
		fmt.Printf("║  Ports:      %-46s ║\n", truncate(strings.Join(info.Ports, ", "), 46))
	}

	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	return nil
}

func runExec(projectDir, user string, command []string) error {
	ctx, cancel := contextWithSignal()
	defer cancel()

	absDir, err := workspace.ResolveProjectDir(projectDir)
	if err != nil {
		return err
	}

	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	manager := ctr.NewManager(dockerClient.Inner())

	existing, err := manager.FindByProject(ctx, absDir)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("no running environment found. Run 'devenv up' first")
	}
	if existing.State != "running" {
		return fmt.Errorf("container is not running (status: %s). Run 'devenv up' first", existing.State)
	}

	// If no user specified, try to get from config
	if user == "" {
		cfg, err := config.Load(absDir)
		if err == nil {
			user = cfg.GetEffectiveUser()
		}
	}

	exitCode, err := manager.Exec(ctx, existing.ID, command, user)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

func runAttach(projectDir string) error {
	ctx, cancel := contextWithSignal()
	defer cancel()

	absDir, err := workspace.ResolveProjectDir(projectDir)
	if err != nil {
		return err
	}

	// Check VS Code is installed
	if !vscode.IsInstalled() {
		return fmt.Errorf("VS Code CLI (code) not found in PATH. Install VS Code and enable the 'code' shell command")
	}

	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	manager := ctr.NewManager(dockerClient.Inner())

	existing, err := manager.FindByProject(ctx, absDir)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("no running environment found. Run 'devenv up' first")
	}
	if existing.State != "running" {
		return fmt.Errorf("container is not running (status: %s). Run 'devenv up' first", existing.State)
	}

	// Get workspace folder from config
	cfg, err := config.Load(absDir)
	if err != nil {
		return err
	}

	return vscode.Attach(existing.ID, cfg.GetWorkspaceFolder())
}

func runList() error {
	ctx, cancel := contextWithSignal()
	defer cancel()

	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	manager := ctr.NewManager(dockerClient.Inner())

	containers, err := manager.ListAll(ctx)
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		fmt.Println("ℹ️  No managed development environments found")
		fmt.Println("   Run 'devenv up' in a project directory to start one")
		return nil
	}

	fmt.Printf("\n📦 Managed Development Environments (%d)\n\n", len(containers))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tSTATUS\tIMAGE\tPROJECT\tCREATED\n")
	fmt.Fprintf(w, "────\t──────\t─────\t───────\t───────\n")

	for _, c := range containers {
		created := c.Created.Format("2006-01-02 15:04")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			c.Name,
			formatState(c.State),
			truncate(c.Image, 30),
			truncate(c.ProjectDir, 40),
			created,
		)
	}
	w.Flush()
	fmt.Println()
	return nil
}

func runLogs(projectDir string, follow bool) error {
	ctx, cancel := contextWithSignal()
	defer cancel()

	absDir, err := workspace.ResolveProjectDir(projectDir)
	if err != nil {
		return err
	}

	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	manager := ctr.NewManager(dockerClient.Inner())

	existing, err := manager.FindByProject(ctx, absDir)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("no environment found for this project")
	}

	return manager.StreamLogs(ctx, existing.ID, follow)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// contextWithSignal creates a context that is cancelled on SIGINT or SIGTERM.
func contextWithSignal() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatState(state string) string {
	switch state {
	case "running":
		return "🟢 running"
	case "exited":
		return "🔴 exited"
	case "paused":
		return "🟡 paused"
	case "created":
		return "⚪ created"
	default:
		return state
	}
}

func getTemplate(name string) map[string]interface{} {
	templates := map[string]map[string]interface{}{
		"default": {
			"name":  filepath.Base(mustCwd()),
			"image": "mcr.microsoft.com/devcontainers/base:ubuntu",
			"customizations": map[string]interface{}{
				"vscode": map[string]interface{}{
					"extensions": []string{},
				},
			},
			"forwardPorts":    []int{},
			"postCreateCommand": "",
		},
		"go": {
			"name":  filepath.Base(mustCwd()),
			"image": "mcr.microsoft.com/devcontainers/go:1.22",
			"customizations": map[string]interface{}{
				"vscode": map[string]interface{}{
					"extensions": []string{
						"golang.Go",
					},
					"settings": map[string]interface{}{
						"go.toolsManagement.checkForUpdates": "local",
						"go.useLanguageServer":               true,
					},
				},
			},
			"forwardPorts":      []int{8080},
			"postCreateCommand": "go mod download",
			"remoteUser":        "vscode",
		},
		"python": {
			"name":  filepath.Base(mustCwd()),
			"image": "mcr.microsoft.com/devcontainers/python:3.12",
			"customizations": map[string]interface{}{
				"vscode": map[string]interface{}{
					"extensions": []string{
						"ms-python.python",
						"ms-python.vscode-pylance",
					},
				},
			},
			"forwardPorts":      []int{8000, 5000},
			"postCreateCommand": "pip install -r requirements.txt",
			"remoteUser":        "vscode",
		},
		"node": {
			"name":  filepath.Base(mustCwd()),
			"image": "mcr.microsoft.com/devcontainers/javascript-node:22",
			"customizations": map[string]interface{}{
				"vscode": map[string]interface{}{
					"extensions": []string{
						"dbaeumer.vscode-eslint",
						"esbenp.prettier-vscode",
					},
				},
			},
			"forwardPorts":      []int{3000, 5173},
			"postCreateCommand": "npm install",
			"remoteUser":        "node",
		},
	}

	if t, ok := templates[name]; ok {
		return t
	}
	return templates["default"]
}

func mustCwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "project"
	}
	return dir
}
