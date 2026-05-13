package engine

import (
	"context"
)

type ContainerEngine interface {
    PullImage(ctx context.Context, imageRef string) error
	ImageExists(ctx context.Context, imageRef string) (bool, error)
	RunContainer(ctx context.Context, imageRef string, entrypoint []string, cmd []string) error
}