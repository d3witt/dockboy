package app

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/d3witt/dockboy/cli/command"
	"github.com/d3witt/dockboy/dockerhelper"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/urfave/cli/v2"
)

func NewInfoCmd(dockboyCli *command.Cli) *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Display information about the app",
		Action: func(ctx *cli.Context) error {
			return runInfo(ctx.Context, dockboyCli)
		},
	}
}

type appInfo struct {
	Name        string
	Status      string
	HealthError string

	Image       string
	Env         map[string]string
	Labels      map[string]string
	Secrets     []string
	HealthCheck *healthCheckInfo
}

type healthCheckInfo struct {
	Test     string
	Interval string
	Timeout  string
	Retries  int
}

func runInfo(ctx context.Context, dockboyCli *command.Cli) error {
	conf, err := dockboyCli.AppConfig()
	if err != nil {
		return fmt.Errorf("get app config: %w", err)
	}

	info := &appInfo{
		Name:   conf.Name,
		Status: "Not Deployed",
	}

	sshClient, err := dockboyCli.DialMachine()
	if err != nil {
		return fmt.Errorf("dial machine: %w", err)
	}
	defer sshClient.Close()

	if !dockerhelper.IsDockerInstalled(sshClient) {
		printInfo(dockboyCli.Out, info)
		return nil
	}

	dockerClient, err := dockerhelper.DialSSH(sshClient)
	if err != nil {
		return fmt.Errorf("dial Docker: %w", err)
	}
	defer dockerClient.Close()

	inactive, err := dockerhelper.IsSwarmInactive(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("check if Swarm is inactive: %w", err)
	}
	if inactive {
		printInfo(dockboyCli.Out, info)
		return nil
	}

	service, err := dockerhelper.FindService(ctx, dockerClient, conf.Name)
	if err != nil {
		return fmt.Errorf("find service: %w", err)
	}

	if service == nil {
		printInfo(dockboyCli.Out, info)
		return nil
	}

	if err := populateAppInfo(ctx, dockerClient, service, info); err != nil {
		return err
	}

	printInfo(dockboyCli.Out, info)
	return nil
}

func populateAppInfo(ctx context.Context, dockerClient *client.Client, service *swarm.Service, info *appInfo) error {
	info.Status = "Deployed"
	info.Image = service.Spec.TaskTemplate.ContainerSpec.Image
	info.Env = formatEnv(service.Spec.TaskTemplate.ContainerSpec.Env)
	info.Labels = service.Spec.Labels
	info.Secrets = formatSecrets(service.Spec.TaskTemplate.ContainerSpec.Secrets)

	if hc := service.Spec.TaskTemplate.ContainerSpec.Healthcheck; hc != nil {
		info.HealthCheck = &healthCheckInfo{
			Test:     strings.Join(hc.Test, " "),
			Interval: hc.Interval.String(),
			Timeout:  hc.Timeout.String(),
			Retries:  int(hc.Retries),
		}
	}

	tasks, err := dockerhelper.ListTasks(ctx, dockerClient, service.ID)
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}
	currentTasks := filterCurrentTasks(tasks)
	if len(currentTasks) == 0 {
		info.Status = "Deployed (no containers)"
		return nil
	} else {
		containerStatus, healthErr, err := getContainerStatus(ctx, dockerClient, currentTasks)
		if err != nil {
			return err
		}

		info.Status = "Deployed (" + containerStatus + ")"
		info.HealthError = healthErr
	}

	return nil
}

func filterCurrentTasks(tasks []swarm.Task) []swarm.Task {
	var currentTasks []swarm.Task
	for _, task := range tasks {
		if task.DesiredState == swarm.TaskStateRunning &&
			(task.Status.State == swarm.TaskStateRunning || task.Status.State == swarm.TaskStateStarting) {
			currentTasks = append(currentTasks, task)
		}
	}
	return currentTasks
}

func getContainerStatus(ctx context.Context, dockerClient *client.Client, tasks []swarm.Task) (string, string, error) {
	for _, task := range tasks {
		containerID := task.Status.ContainerStatus.ContainerID
		if containerID == "" {
			continue
		}

		containerJSON, err := dockerClient.ContainerInspect(ctx, containerID)
		if err != nil {
			return "", "", fmt.Errorf("inspect container %s: %w", containerID, err)
		}

		if hcState := containerJSON.State.Health; hcState != nil {
			if hcState.Status == "unhealthy" {
				var healthError string
				if len(hcState.Log) > 0 {
					var logs []string
					for _, log := range hcState.Log {
						logs = append(logs, log.Output)
					}
					healthError = strings.Join(logs, "\n")
				}
				return hcState.Status, healthError, nil
			}

			return hcState.Status, containerJSON.State.Error, nil
		}
	}

	return "unknown", "", nil
}

func printInfo(w io.Writer, info *appInfo) {
	fmt.Fprintf(w, "Name: %s\n", info.Name)
	fmt.Fprintf(w, "Status: %s\n", info.Status)

	if info.HealthError != "" {
		fmt.Fprintf(w, "\nHealth Check Error:\n%s\n", info.HealthError)
	}

	fmt.Fprintf(w, "\nImage: %s\n", info.Image)

	if len(info.Env) > 0 {
		fmt.Fprintf(w, "\nEnvironments:\n")
		for k, v := range info.Env {
			fmt.Fprintf(w, "  %s=%s\n", k, v)
		}
	}

	if len(info.Secrets) > 0 {
		fmt.Fprintf(w, "\nSecrets:\n")
		for _, s := range info.Secrets {
			fmt.Fprintf(w, "  %s\n", s)
		}
	}

	if info.HealthCheck != nil {
		fmt.Fprintf(w, "\nHealth Check:\n")
		fmt.Fprintf(w, "  Test: %s\n", info.HealthCheck.Test)
		fmt.Fprintf(w, "  Interval: %s\n", info.HealthCheck.Interval)
		fmt.Fprintf(w, "  Timeout: %s\n", info.HealthCheck.Timeout)
		fmt.Fprintf(w, "  Retries: %d\n", info.HealthCheck.Retries)
	}
}

func formatEnv(env []string) map[string]string {
	result := make(map[string]string)
	for _, e := range env {
		if parts := strings.SplitN(e, "=", 2); len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func formatSecrets(secrets []*swarm.SecretReference) []string {
	var result []string
	for _, secret := range secrets {
		result = append(result, secret.SecretName)
	}
	return result
}
