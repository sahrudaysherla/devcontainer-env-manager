# DevContainer Environment Manager

[![CI](https://github.com/sahru/devcontainer-env-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/sahru/devcontainer-env-manager/.github/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/sahru/devcontainer-env-manager)](https://goreportcard.com/report/github.com/sahru/devcontainer-env-manager)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A CLI-based platform tool built with **Go** and **Docker SDK** for managing reproducible development environments based on the [Dev Container specification](https://containers.dev/). Automatically provisions, configures, and tears down containerized workspaces, ensuring consistency between local development and CI pipelines.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    CLI Layer (cobra)                      │
│  init │ up │ down │ status │ exec │ attach │ list │ logs │
├──────────────────────────────────────────────────────────┤
│                   Orchestration Layer                     │
│  ┌────────────┐ ┌───────────┐ ┌────────────────────────┐ │
│  │  Lifecycle  │ │ Workspace │ │   VS Code Integration  │ │
│  │  Executor   │ │ Manager   │ │   (Remote Containers)  │ │
│  └────────────┘ └───────────┘ └────────────────────────┘ │
├──────────────────────────────────────────────────────────┤
│                    Core Layer                             │
│  ┌──────────────────┐  ┌──────────────────────────────┐  │
│  │  Config Parser    │  │  Container Manager           │  │
│  │  (JSONC support)  │  │  (create/start/stop/remove)  │  │
│  └──────────────────┘  └──────────────────────────────┘  │
├──────────────────────────────────────────────────────────┤
│              Docker SDK (github.com/docker/docker)        │
└──────────────────────────────────────────────────────────┘
```

## Features

- **Full Dev Container Spec Support** — Parses `devcontainer.json` with JSONC (comments & trailing commas), supporting `image`, `build`, `forwardPorts`, `mounts`, `remoteUser`, `features`, and `customizations`
- **Lifecycle Hook Execution** — Runs all 6 lifecycle phases (`initializeCommand` → `onCreateCommand` → `updateContentCommand` → `postCreateCommand` → `postStartCommand` → `postAttachCommand`) in spec order
- **VS Code Remote Integration** — Seamlessly attach VS Code to running containers via the Remote - Containers extension
- **Container Management** — Create, start, stop, remove containers with deterministic naming and label-based tracking
- **Port Forwarding** — Automatic host-to-container port forwarding from config
- **Workspace Mounting** — Bind-mount project directories into containers with configurable mount points
- **Multi-Project Support** — Manage environments across multiple projects simultaneously
- **Signal Handling** — Graceful shutdown on SIGINT/SIGTERM

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/sahru/devcontainer-env-manager.git
cd devcontainer-env-manager

# Build the binary
go build -o bin/devenv ./cmd/devenv

# Optional: move to PATH
sudo mv bin/devenv /usr/local/bin/
```

### Prerequisites

- **Go 1.22+** (for building from source)
- **Docker** (daemon must be running)
- **VS Code** with [Remote - Containers](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) extension (for `attach` command)

## Quick Start

```bash
# 1. Initialize a new project with a devcontainer configuration
devenv init --template go

# 2. Start the development environment
devenv up

# 3. Execute commands inside the container
devenv exec -- go version
devenv exec -- go test ./...

# 4. Attach VS Code for editing and debugging
devenv attach

# 5. View container logs
devenv logs -f

# 6. Tear down when done
devenv down
```

## CLI Reference

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `devenv init` | Generate a `.devcontainer/devcontainer.json` | `--template` (`default`, `go`, `python`, `node`) |
| `devenv up` | Build/pull image, create container, run lifecycle hooks | `--project-dir`, `--rebuild` |
| `devenv down` | Stop and remove the container | `--project-dir`, `--volumes` |
| `devenv status` | Show environment status and details | `--project-dir` |
| `devenv exec` | Run a command in the container | `--user`, `-- <command>` |
| `devenv attach` | Open VS Code attached to the container | `--project-dir` |
| `devenv list` | List all managed environments | — |
| `devenv logs` | Stream container logs | `--follow` |

## devcontainer.json Support

The tool supports the following `devcontainer.json` fields:

```jsonc
{
  // Identity
  "name": "My Project",

  // Image (use one of image or build)
  "image": "mcr.microsoft.com/devcontainers/go:1.22",

  // Build from Dockerfile
  "build": {
    "dockerfile": "Dockerfile",
    "context": "..",
    "args": { "VARIANT": "1.22" }
  },

  // Runtime
  "forwardPorts": [8080, 3000],
  "mounts": ["type=volume,source=cache,target=/cache"],
  "containerEnv": { "GO111MODULE": "on" },
  "remoteUser": "vscode",

  // Lifecycle Hooks
  "initializeCommand": "echo 'Setting up...'",       // Runs on HOST
  "onCreateCommand": "apt-get update",                // Runs in CONTAINER
  "postCreateCommand": "go mod download",             // Runs in CONTAINER
  "postStartCommand": "echo 'Ready!'",                // Runs in CONTAINER

  // VS Code
  "customizations": {
    "vscode": {
      "extensions": ["golang.Go"],
      "settings": { "go.useLanguageServer": true }
    }
  }
}
```

## Project Structure

```
├── cmd/devenv/main.go           # CLI entry point with cobra commands
├── internal/
│   ├── config/                  # devcontainer.json parser (JSONC support)
│   ├── container/               # Container lifecycle management
│   ├── docker/                  # Docker SDK client wrapper
│   ├── lifecycle/               # Lifecycle hook executor
│   ├── vscode/                  # VS Code Remote integration
│   └── workspace/               # Workspace provisioning (mounts, ports, env)
├── examples/                    # Example devcontainer configurations
│   ├── go-project/
│   ├── python-project/
│   └── node-project/
├── .github/workflows/ci.yml     # GitHub Actions CI pipeline
├── go.mod
└── README.md
```

## Development

```bash
# Run tests
go test -v ./...

# Run tests with race detection
go test -race ./...

# Lint
go vet ./...

# Build for all platforms
GOOS=linux   GOARCH=amd64 go build -o bin/devenv-linux-amd64 ./cmd/devenv
GOOS=darwin  GOARCH=arm64 go build -o bin/devenv-darwin-arm64 ./cmd/devenv
GOOS=windows GOARCH=amd64 go build -o bin/devenv-windows-amd64.exe ./cmd/devenv
```

## How It Works

1. **Configuration Discovery** — Searches for `.devcontainer/devcontainer.json` or `.devcontainer.json` with JSONC parsing (comments, trailing commas)
2. **Image Preparation** — Pulls the specified image or builds from a Dockerfile using the Docker SDK
3. **Container Creation** — Creates a container with workspace mounts, port forwarding, environment variables, and tracking labels (`devenv.managed=true`)
4. **Lifecycle Execution** — Runs all configured lifecycle hooks in specification order: host commands first (`initializeCommand`), then container commands
5. **VS Code Attachment** — Generates a `vscode-remote://` URI and launches VS Code connected to the container

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes (`git commit -am 'Add my feature'`)
4. Push to the branch (`git push origin feature/my-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
