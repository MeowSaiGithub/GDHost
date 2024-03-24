package deployment

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	ct "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/go-connections/nat"
	"github.com/rs/zerolog"
	"io"
	"strings"
)

type container struct {
	cli *client.Client
}

// newContainerController() initializes a new container
func newContainerController() (*container, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	return &container{
		cli: cli,
	}, err
}

// deleteImage remove image from the docker
func (c *container) deleteImage(ctx context.Context, imageId string) error {
	opts := types.ImageRemoveOptions{
		Force:         true,
		PruneChildren: true,
	}
	_, err := c.cli.ImageRemove(ctx, imageId, opts)
	return err
}

// buildImage build an image from the docker file with tar.
// TODO: need to remove dangling images
func (c *container) buildImage(ctx context.Context, name string, path string, logger zerolog.Logger) (io.ReadCloser, error) {
	tar, err := archive.TarWithOptions(path, &archive.TarOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to archive tar: %w", err)
	}
	defer func() {
		if err2 := tar.Close(); err2 != nil {
			logger.Error().Err(err).Msg("failed to close tar archive")
		}
	}()

	opts := types.ImageBuildOptions{
		Context:     tar,
		Dockerfile:  "Dockerfile",
		Tags:        []string{name},
		Remove:      true,
		ForceRemove: true,
	}

	resp, err := c.cli.ImageBuild(ctx, tar, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build image: %w", err)
	}
	return resp.Body, nil
}

// getImageId get the image ID from the docker
func (c *container) getImageId(ctx context.Context, name string) (string, error) {
	inspect, _, err := c.cli.ImageInspectWithRaw(ctx, name)
	if err != nil {
		return "", err
	}
	imageId := strings.Split(inspect.ID, ":")

	return imageId[1], nil
}

// createContainer create a docker-container from an image.
func (c *container) createContainer(ctx context.Context, name string, hport string, cport string) (string, error) {
	port, err := nat.NewPort("tcp", cport)
	if err != nil {
		return "", fmt.Errorf("failed to parse container port: %w", err)
	}

	containerConfig := &ct.Config{
		Image: name,
		ExposedPorts: nat.PortSet{
			port: struct{}{},
		},
	}
	hostConfig := &ct.HostConfig{
		PortBindings: nat.PortMap{
			port: []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: hport,
				},
			},
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}
	return resp.ID, nil

}

// stopContainer stops docker-container
func (c *container) stopContainer(ctx context.Context, containerId string) error {
	err := c.cli.ContainerStop(ctx, containerId, ct.StopOptions{})
	return err
}

// removeContainer removes docker-container
func (c *container) removeContainer(ctx context.Context, containerId string) error {
	err := c.cli.ContainerRemove(ctx, containerId, ct.RemoveOptions{})
	return err
}

// getContainerLogs retrieves docker-container-logs and follow.
// TODO: need to retrieve the first initialized logs too
func (c *container) getContainerLogs(ctx context.Context, containerId string) (io.ReadCloser, error) {
	opts := ct.LogsOptions{
		ShowStdout: true,
		Follow:     true,
	}
	clogs, err := c.cli.ContainerLogs(ctx, containerId, opts)
	if err != nil {
		return nil, err
	}
	return clogs, err
}

// startContainer starts a docker-container
func (c *container) startContainer(ctx context.Context, containerId string) error {
	err := c.cli.ContainerStart(ctx, containerId, ct.StartOptions{})
	return err
}

// isContainerRunning checks if a docker-container is running or not.
func (c *container) isContainerRunning(ctx context.Context, containerId string) (bool, error) {
	inspect, err := c.cli.ContainerInspect(ctx, containerId)
	if err != nil {
		return false, err
	}
	return inspect.State.Running, nil
}
