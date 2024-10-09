package dockerhelper

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func ExecContainer(ctx context.Context, remote *client.Client, containerID string, cmd []string) error {
	execConfig := container.ExecOptions{
		Cmd: cmd,
	}

	execID, err := remote.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec: %w", err)
	}

	err = remote.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start exec: %w", err)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			inspectResp, err := remote.ContainerExecInspect(ctx, execID.ID)
			if err != nil {
				return fmt.Errorf("failed to inspect exec: %w", err)
			}
			if !inspectResp.Running {
				if inspectResp.ExitCode != 0 {
					return fmt.Errorf("command exited with code %d", inspectResp.ExitCode)
				}
				return nil
			}
		}
	}
}
