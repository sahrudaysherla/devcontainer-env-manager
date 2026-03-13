// Package lifecycle handles the execution of devcontainer lifecycle hooks
// in the correct order, both on the host and inside containers.
package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sahru/devcontainer-env-manager/internal/config"
	ctr "github.com/sahru/devcontainer-env-manager/internal/container"
)

// Phase represents a lifecycle phase.
type Phase string

const (
	PhaseInitialize    Phase = "initializeCommand"
	PhaseOnCreate      Phase = "onCreateCommand"
	PhaseUpdateContent Phase = "updateContentCommand"
	PhasePostCreate    Phase = "postCreateCommand"
	PhasePostStart     Phase = "postStartCommand"
	PhasePostAttach    Phase = "postAttachCommand"
)

// Executor handles running lifecycle hooks.
type Executor struct {
	manager     *ctr.Manager
	containerID string
	user        string
	projectDir  string
}

// NewExecutor creates a lifecycle executor for a given container.
func NewExecutor(manager *ctr.Manager, containerID, user, projectDir string) *Executor {
	return &Executor{
		manager:     manager,
		containerID: containerID,
		user:        user,
		projectDir:  projectDir,
	}
}

// ExecuteAll runs the full lifecycle sequence based on the devcontainer config.
// The execution order follows the Dev Container specification:
//  1. initializeCommand  (runs on HOST)
//  2. onCreateCommand    (runs in CONTAINER)
//  3. updateContentCommand (runs in CONTAINER)
//  4. postCreateCommand  (runs in CONTAINER)
//  5. postStartCommand   (runs in CONTAINER)
//  6. postAttachCommand  (runs in CONTAINER)
func (e *Executor) ExecuteAll(ctx context.Context, cfg *config.DevContainerConfig) error {
	hooks := []struct {
		phase   Phase
		hook    interface{}
		onHost  bool
	}{
		{PhaseInitialize, cfg.InitializeCommand, true},
		{PhaseOnCreate, cfg.OnCreateCommand, false},
		{PhaseUpdateContent, cfg.UpdateContentCommand, false},
		{PhasePostCreate, cfg.PostCreateCommand, false},
		{PhasePostStart, cfg.PostStartCommand, false},
		{PhasePostAttach, cfg.PostAttachCommand, false},
	}

	for _, h := range hooks {
		if h.hook == nil {
			continue
		}

		commands := config.ResolveLifecycleCommands(h.hook)
		if len(commands) == 0 {
			continue
		}

		fmt.Printf("🔄 Running %s...\n", h.phase)

		for _, cmd := range commands {
			if h.onHost {
				if err := e.runOnHost(ctx, cmd); err != nil {
					return fmt.Errorf("%s failed: %w", h.phase, err)
				}
			} else {
				if err := e.runInContainer(ctx, cmd); err != nil {
					return fmt.Errorf("%s failed: %w", h.phase, err)
				}
			}
		}

		fmt.Printf("✅ %s completed\n", h.phase)
	}

	return nil
}

// ExecutePhase runs commands for a specific lifecycle phase.
func (e *Executor) ExecutePhase(ctx context.Context, phase Phase, hook interface{}) error {
	if hook == nil {
		return nil
	}

	commands := config.ResolveLifecycleCommands(hook)
	if len(commands) == 0 {
		return nil
	}

	onHost := phase == PhaseInitialize
	fmt.Printf("🔄 Running %s...\n", phase)

	for _, cmd := range commands {
		if onHost {
			if err := e.runOnHost(ctx, cmd); err != nil {
				return fmt.Errorf("%s failed: %w", phase, err)
			}
		} else {
			if err := e.runInContainer(ctx, cmd); err != nil {
				return fmt.Errorf("%s failed: %w", phase, err)
			}
		}
	}

	fmt.Printf("✅ %s completed\n", phase)
	return nil
}

// RunPostStart runs only the postStartCommand phase.
// Used when restarting an existing container.
func (e *Executor) RunPostStart(ctx context.Context, cfg *config.DevContainerConfig) error {
	return e.ExecutePhase(ctx, PhasePostStart, cfg.PostStartCommand)
}

// runOnHost executes a command on the host machine.
func (e *Executor) runOnHost(ctx context.Context, cmd []string) error {
	if len(cmd) == 0 {
		return nil
	}

	fmt.Printf("  → [host] %s\n", strings.Join(cmd, " "))

	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	command.Dir = e.projectDir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin

	return command.Run()
}

// runInContainer executes a command inside the container.
func (e *Executor) runInContainer(ctx context.Context, cmd []string) error {
	if len(cmd) == 0 {
		return nil
	}

	fmt.Printf("  → [container] %s\n", strings.Join(cmd, " "))

	exitCode, err := e.manager.Exec(ctx, e.containerID, cmd, e.user)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("command exited with code %d", exitCode)
	}
	return nil
}
