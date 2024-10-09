package dockerhelper

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

func DeployService(
	ctx context.Context,
	out io.Writer,
	docker *client.Client,
	name, image string,
	replicas uint64,
	networks []string,
	env, labels map[string]string,
	secrets map[string][]byte,
	healthcheck *container.HealthConfig,
	mounts []mount.Mount,
	order string,
) error {
	if order != swarm.UpdateOrderStartFirst && order != swarm.UpdateOrderStopFirst {
		return fmt.Errorf("invalid order: %s", order)
	}

	secretRefs, err := createSecrets(ctx, docker, secrets)
	if err != nil {
		return err
	}

	spec := swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name:   name,
			Labels: labels,
		},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image:       image,
				Env:         mapToSlice(env),
				Secrets:     secretRefs,
				Healthcheck: healthcheck,
				Mounts:      mounts,
			},
			Networks: parseNetworks(networks),
			LogDriver: &swarm.Driver{
				Name: "local",
				Options: map[string]string{
					"max-size": "100m",
					"max-file": "3",
				},
			},
		},
		Mode: swarm.ServiceMode{
			Replicated: &swarm.ReplicatedService{
				Replicas: &replicas,
			},
		},
		UpdateConfig: &swarm.UpdateConfig{
			Parallelism:     1,
			FailureAction:   swarm.UpdateFailureActionRollback,
			MaxFailureRatio: 0.0,
			Monitor:         10 * time.Second,
			Order:           order,
		},
		RollbackConfig: &swarm.UpdateConfig{
			Parallelism:     1,
			FailureAction:   swarm.UpdateFailureActionPause,
			MaxFailureRatio: 0.0,
			Monitor:         10 * time.Second,
			Order:           order,
		},
	}

	existingService, err := FindService(ctx, docker, name)
	if err != nil {
		return err
	}

	if existingService != nil {
		fmt.Fprintf(out, "dockboy: updating service '%s'...\n", name)
		_, err = docker.ServiceUpdate(ctx, existingService.ID, existingService.Version, spec, types.ServiceUpdateOptions{})
		if err != nil {
			return fmt.Errorf("service update failed: %w", err)
		}
	} else {
		fmt.Fprintf(out, "dockboy: creating service '%s'...\n", name)
		resp, err := docker.ServiceCreate(ctx, spec, types.ServiceCreateOptions{})
		if err != nil {
			return fmt.Errorf("service creation failed: %w", err)
		}
		existingService = &swarm.Service{ID: resp.ID}
	}

	return WaitForService(ctx, out, docker, existingService.ID)
}

func WaitForService(ctx context.Context, out io.Writer, docker *client.Client, serviceID string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	msgs, errs := docker.Events(ctx, events.ListOptions{
		Filters: filters.NewArgs(filters.Arg("type", "container")),
	})

	go monitorEvents(ctx, out, docker, msgs, errs, serviceID)
	go checkService(ctx, out, docker, serviceID, done)

	select {
	case <-done:
		return nil
	case <-signalChan:
		cancel()
		return fmt.Errorf("deployment cancelled")
	case <-ctx.Done():
		return fmt.Errorf("context cancelled")
	}
}

func monitorEvents(ctx context.Context, out io.Writer, docker *client.Client, msgs <-chan events.Message, errs <-chan error, serviceID string) {
	for {
		select {
		case msg := <-msgs:
			handleEvent(ctx, out, docker, msg, serviceID)
		case err := <-errs:
			if err != nil {
				fmt.Fprintf(out, "dockboy: event error: %v\n", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func checkService(ctx context.Context, out io.Writer, docker *client.Client, serviceID string, done chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastState swarm.UpdateState

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service, _, err := docker.ServiceInspectWithRaw(ctx, serviceID, types.ServiceInspectOptions{})
			if err != nil {
				fmt.Fprintf(out, "dockboy: service inspection error: %v\n", err)
				continue
			}

			if service.UpdateStatus != nil && service.UpdateStatus.State != lastState {
				switch service.UpdateStatus.State {
				case swarm.UpdateStateCompleted:
					fmt.Fprintf(out, "dockboy: service '%s' updated successfully.\n", service.Spec.Name)
					close(done)
					return
				case swarm.UpdateStatePaused:
					fmt.Fprintf(out, "dockboy: service '%s' update paused.\n", service.Spec.Name)
					close(done)
					return
				case swarm.UpdateStateRollbackCompleted:
					// deploy might start with a rollback completed state if the previous update failed.
					if lastState == swarm.UpdateStateRollbackStarted || lastState == swarm.UpdateStateRollbackPaused {
						fmt.Fprintf(out, "dockboy: service '%s' rolled back successfully.\n", service.Spec.Name)
						close(done)
						return
					}
				case swarm.UpdateStateRollbackPaused:
					fmt.Fprintf(out, "dockboy: service '%s' rollback paused.\n", service.Spec.Name)
					close(done)
					return
				case swarm.UpdateStateRollbackStarted:
					fmt.Fprintf(out, "dockboy: service '%s' update failed, rolling back. message: %s\n", service.Spec.Name, service.UpdateStatus.Message)
					taskErr, err := getLatestTaskError(ctx, docker, serviceID)
					if err != nil {
						fmt.Fprintf(out, "dockboy: failed to get task error: %v\n", err)
					} else {
						fmt.Fprintf(out, "dockboy: task error: %s\n", taskErr)
					}
				}

				lastState = service.UpdateStatus.State
			}
		}
	}
}

func handleEvent(ctx context.Context, out io.Writer, docker *client.Client, event events.Message, serviceID string) {
	if event.Actor.Attributes["com.docker.swarm.service.id"] != serviceID {
		return
	}
	switch event.Action {
	case "create":
		fmt.Fprintf(out, "swarm: creating container %s...\n", event.Actor.Attributes["name"])
	case "start":
		fmt.Fprintf(out, "swarm: starting container %s...\n", event.Actor.Attributes["name"])
	case "die":
		fmt.Fprintf(out, "swarm: container %s stopped.\n", event.Actor.Attributes["name"])
	case "health_status: healthy":
		fmt.Fprintf(out, "swarm: container %s is healthy.\n", event.Actor.Attributes["name"])
	case "health_status: unhealthy":
		fmt.Fprintf(out, "swarm: container %s is unhealthy.\n", event.Actor.Attributes["name"])
		if message, err := getHealthCheckError(ctx, docker, event.Actor.ID); err == nil && message != "" {
			fmt.Fprintf(out, "dockboy: health check error:\n %s\n", message)
		}
	}
}

func getHealthCheckError(ctx context.Context, remote *client.Client, containerID string) (string, error) {
	container, err := remote.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspecting container: %w", err)
	}

	if container.State.Health == nil {
		return "", nil
	}

	if container.State.Health.Status != "unhealthy" {
		return "", nil
	}

	if len(container.State.Health.Log) == 0 {
		return "", nil
	}

	return container.State.Health.Log[len(container.State.Health.Log)-1].Output, nil
}

func getLatestTaskError(ctx context.Context, docker *client.Client, serviceID string) (string, error) {
	tasks, err := docker.TaskList(ctx, types.TaskListOptions{
		Filters: filters.NewArgs(
			filters.Arg("service", serviceID),
		),
	})
	if err != nil {
		return "", fmt.Errorf("listing tasks: %w", err)
	}

	var latestTask swarm.Task
	for _, task := range tasks {
		if task.Status.State == swarm.TaskStateFailed || task.Status.State == swarm.TaskStateRejected {
			if latestTask.ID == "" || task.CreatedAt.After(latestTask.CreatedAt) {
				latestTask = task
				break
			}
		}
	}

	return latestTask.Status.Err, nil
}

func createSecrets(ctx context.Context, docker *client.Client, secrets map[string][]byte) ([]*swarm.SecretReference, error) {
	var secretRefs []*swarm.SecretReference
	for name, data := range secrets {
		secretName := fmt.Sprintf("%s-%d", name, time.Now().Unix())
		secret, err := docker.SecretCreate(ctx, swarm.SecretSpec{
			Annotations: swarm.Annotations{Name: secretName},
			Data:        data,
		})
		if err != nil {
			return nil, fmt.Errorf("creating secret: %w", err)
		}

		secretRefs = append(secretRefs, &swarm.SecretReference{
			SecretID:   secret.ID,
			SecretName: secretName,
			File: &swarm.SecretReferenceFileTarget{
				Name: name,
				UID:  "0",
				GID:  "0",
				Mode: 0o444,
			},
		})
	}
	return secretRefs, nil
}

func mapToSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}

func parseNetworks(networks []string) []swarm.NetworkAttachmentConfig {
	out := make([]swarm.NetworkAttachmentConfig, len(networks))
	for i, net := range networks {
		out[i] = swarm.NetworkAttachmentConfig{Target: net}
	}
	return out
}
