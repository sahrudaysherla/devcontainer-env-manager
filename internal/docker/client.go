// Package docker provides a wrapper around the Docker SDK client for managing
// images and interacting with the Docker daemon.
package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
)

// Client wraps the Docker SDK client with higher-level operations.
type Client struct {
	cli *client.Client
}

// NewClient creates a new Docker client from environment variables.
// It connects to the Docker daemon using DOCKER_HOST or the default socket.
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &Client{cli: cli}, nil
}

// Inner returns the underlying Docker SDK client for direct API access.
func (c *Client) Inner() *client.Client {
	return c.cli
}

// Close closes the Docker client connection.
func (c *Client) Close() error {
	return c.cli.Close()
}

// Ping verifies connectivity to the Docker daemon.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("cannot connect to Docker daemon: %w", err)
	}
	return nil
}

// PullImage pulls an image from a registry, streaming progress to stdout.
func (c *Client) PullImage(ctx context.Context, ref string) error {
	fmt.Printf("📦 Pulling image: %s\n", ref)

	reader, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", ref, err)
	}
	defer reader.Close()

	// Stream pull progress
	if err := jsonmessage.DisplayJSONMessagesStream(reader, os.Stdout, os.Stdout.Fd(), true, nil); err != nil {
		// Fallback: just consume the reader
		io.Copy(io.Discard, reader)
	}

	fmt.Printf("✅ Image pulled: %s\n", ref)
	return nil
}

// BuildImage builds a Docker image from a Dockerfile.
func (c *Client) BuildImage(ctx context.Context, contextDir, dockerfile, tag string, buildArgs map[string]string) error {
	fmt.Printf("🔨 Building image from %s (context: %s)\n", dockerfile, contextDir)

	// Create tar archive of the build context
	tar, err := archive.TarWithOptions(contextDir, &archive.TarOptions{})
	if err != nil {
		return fmt.Errorf("failed to create build context archive: %w", err)
	}
	defer tar.Close()

	// Convert build args to *string map
	args := make(map[string]*string)
	for k, v := range buildArgs {
		val := v
		args[k] = &val
	}

	opts := types.ImageBuildOptions{
		Dockerfile: dockerfile,
		Tags:       []string{tag},
		BuildArgs:  args,
		Remove:     true,
		ForceRemove: true,
	}

	resp, err := c.cli.ImageBuild(ctx, tar, opts)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}
	defer resp.Body.Close()

	// Stream build output
	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, os.Stdout, os.Stdout.Fd(), true, nil); err != nil {
		io.Copy(io.Discard, resp.Body)
	}

	fmt.Printf("✅ Image built: %s\n", tag)
	return nil
}

// EnsureImage either pulls or builds an image based on the configuration.
func (c *Client) EnsureImage(ctx context.Context, imageName string, buildConfig *BuildConfig) error {
	if buildConfig != nil {
		return c.BuildImage(
			ctx,
			buildConfig.ContextDir,
			buildConfig.Dockerfile,
			imageName,
			buildConfig.Args,
		)
	}
	return c.PullImage(ctx, imageName)
}

// ImageExists checks if an image exists locally.
func (c *Client) ImageExists(ctx context.Context, ref string) (bool, error) {
	images, err := c.cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == ref || strings.HasPrefix(tag, ref+":") {
				return true, nil
			}
		}
	}
	return false, nil
}

// BuildConfig holds the resolved build configuration for EnsureImage.
type BuildConfig struct {
	ContextDir string
	Dockerfile string
	Args       map[string]string
}
