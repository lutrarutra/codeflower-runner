package engine

import (
	"context"
	"fmt"
	"os"

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
        Entrypoint: entrypoint, // Uses the invoke_cmd provided by the runner
        Cmd:        cmdArgs,
        Tty:        false,
    }, &container.HostConfig{
        Binds: binds, // Applies organized /cf/inputs and /cf/outputs mounts
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