package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/moby/term"
)

type DockerEngine struct {
	cli *client.Client
}

func NewDockerEngine() (*DockerEngine, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &DockerEngine{cli: cli}, nil
}

func (d *DockerEngine) PullImage(ctx context.Context, imageRef string) error {
	pullStream, err := d.cli.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer pullStream.Close()

	termFd, isTerm := term.GetFdInfo(os.Stdout)
	return jsonmessage.DisplayJSONMessagesStream(pullStream, os.Stdout, termFd, isTerm, nil)
}

func (d *DockerEngine) ImageExists(ctx context.Context, imageRef string) (bool, error) {
	_, _, err := d.cli.ImageInspectWithRaw(ctx, imageRef)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d *DockerEngine) RunContainer(ctx context.Context, imageRef string, entrypoint []string, cmdArgs []string, binds []string) error {
	resp, err := d.cli.ContainerCreate(ctx, &container.Config{
		Image:      imageRef,
		Entrypoint: entrypoint,
		Cmd:        cmdArgs,
		Tty:        false,
	}, &container.HostConfig{
		Binds: binds,
	}, nil, nil, "")
	if err != nil {
		return err
	}

	defer d.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return err
	}

	statusCh, errCh := d.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)

	out, err := d.cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
	if err == nil {
		defer out.Close()
		_, _ = stdcopy.StdCopy(os.Stdout, os.Stderr, out)
	}

	select {
	case err := <-errCh:
		return err
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return fmt.Errorf("container exited with non-zero status: %d", status.StatusCode)
		}
	}
	return nil
}

// RunContainerWithOutput runs a container and captures stdout/stderr as strings.
func (d *DockerEngine) RunContainerWithOutput(ctx context.Context, imageRef string, entrypoint []string, cmdArgs []string, binds []string) (stdout string, stderr string, err error) {
	return d.runContainerWithOutput(ctx, imageRef, entrypoint, cmdArgs, binds, false)
}

// RunContainerWithOutputPolicy runs a container with the restrictive policy used
// by inspect. No host paths are mounted and network access is disabled
// unless allowInternet is explicitly requested.
func (d *DockerEngine) RunContainerWithOutputPolicy(ctx context.Context, imageRef string, entrypoint []string, cmdArgs []string, allowInternet bool) (stdout string, stderr string, err error) {
	return d.runContainerWithOutput(ctx, imageRef, entrypoint, cmdArgs, nil, allowInternet)
}

func (d *DockerEngine) runContainerWithOutput(ctx context.Context, imageRef string, entrypoint []string, cmdArgs []string, binds []string, allowInternet bool) (stdout string, stderr string, err error) {
	networkMode := container.NetworkMode("none")
	if allowInternet {
		networkMode = container.NetworkMode("default")
	}

	resp, err := d.cli.ContainerCreate(ctx, &container.Config{
		Image:      imageRef,
		Entrypoint: entrypoint,
		Cmd:        cmdArgs,
		Tty:        false,
	}, &container.HostConfig{
		Binds:          binds,
		NetworkMode:    networkMode,
		ReadonlyRootfs: true,
		SecurityOpt:    []string{"no-new-privileges:true"},
		CapDrop:        []string{"ALL"},
		Resources: container.Resources{
			PidsLimit: int64Ptr(128),
			Memory:    512 * 1024 * 1024,
			NanoCPUs:  2 * 1e9,
		},
	}, nil, nil, "")
	if err != nil {
		return "", "", fmt.Errorf("container create: %w", err)
	}

	defer func() {
		_ = d.cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
	}()

	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", "", fmt.Errorf("container start: %w", err)
	}

	statusCh, errCh := d.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)

	out, err := d.cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
	if err != nil {
		return "", "", fmt.Errorf("container logs: %w", err)
	}
	defer out.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	_, _ = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, out)

	select {
	case waitErr := <-errCh:
		return stdoutBuf.String(), stderrBuf.String(), waitErr
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("container exited with status %d", status.StatusCode)
		}
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

func int64Ptr(value int64) *int64 {
	return &value
}

// BuildImage builds a Docker image from a Dockerfile in the given context directory.
// Uses the docker CLI (subprocess) to avoid SDK archive compatibility issues.
func (d *DockerEngine) BuildImage(ctx context.Context, contextDir string, tag string) error {
	args := []string{"build", "-t", tag, "-f", contextDir + "/Dockerfile", contextDir}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	return nil
}

// RemoveImage removes a Docker image by reference.
func (d *DockerEngine) RemoveImage(ctx context.Context, imageRef string) error {
	_, err := d.cli.ImageRemove(ctx, imageRef, image.RemoveOptions{Force: true})
	return err
}

// Close closes the Docker client connection.
func (d *DockerEngine) Close() error {
	return d.cli.Close()
}
